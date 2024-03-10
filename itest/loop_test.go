package itest

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/lightninglabs/taproot-assets/address"
	"github.com/lightninglabs/taproot-assets/asset"
	"github.com/lightninglabs/taproot-assets/commitment"
	"github.com/lightninglabs/taproot-assets/fn"
	"github.com/lightninglabs/taproot-assets/proof"
	"github.com/lightninglabs/taproot-assets/tappsbt"
	"github.com/lightninglabs/taproot-assets/taprpc"
	wrpc "github.com/lightninglabs/taproot-assets/taprpc/assetwalletrpc"
	"github.com/lightninglabs/taproot-assets/taprpc/mintrpc"
	"github.com/lightninglabs/taproot-assets/taprpc/tapdevrpc"
	"github.com/lightninglabs/taproot-assets/tapscript"
	"github.com/lightninglabs/taproot-assets/tapsend"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnrpc/chainrpc"
	"github.com/lightningnetwork/lnd/lnrpc/signrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/lntest/node"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/stretchr/testify/require"
)

func testLoopSwapV2(t *harnessTest) {
	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	// We mint some grouped assets to use in the test. These assets are
	// minted on the default tapd instance that is always created in the
	// integration test (connected to lnd "Alice").
	firstBatch := MintAssetsConfirmBatch(
		t.t, t.lndHarness.Miner.Client, t.tapd,
		[]*mintrpc.MintAssetRequest{issuableAssets[0]},
	)[0]

	var (
		firstBatchGenesis = firstBatch.AssetGenesis
		aliceTapd         = t.tapd
		aliceLnd          = t.lndHarness.Alice
		bobLnd            = t.lndHarness.Bob
	)
	// We create a second tapd node that will be used to simulate a second
	// party in the test. This tapd node is connected to lnd "Bob".
	bobTapd := setupTapdHarness(t.t, t, bobLnd, t.universeServer)
	defer func() {
		require.NoError(t.t, bobTapd.stop(!*noDelete))
	}()

	firstTransfer := uint64(1000)

	// First we'll just send some funds to bob so we know that he funds
	// the sweep with the htlc input, not other inputs he might have.
	// We'll send 1000 assets to Bob.
	bobAddr, err := bobTapd.NewAddr(
		ctxt, &taprpc.NewAddrRequest{
			AssetId:      firstBatchGenesis.AssetId,
			Amt:          firstTransfer,
			AssetVersion: issuableAssets[0].Asset.AssetVersion,
		},
	)
	require.NoError(t.t, err)

	_, err = t.tapd.SendAsset(
		ctxt, &taprpc.SendAssetRequest{
			TapAddrs: []string{bobAddr.Encoded},
		},
	)
	require.NoError(t.t, err)

	// Mine 10 blocks
	MineBlocks(t.t, t.lndHarness.Miner.Client, 10, 1)

	// ConfirmAndAssertOutboundTransfer(
	// 	t.t, t.lndHarness.Miner.Client, t.tapd, sendResp1,
	// 	firstBatchGenesis.AssetId, expectedAmounts, 0, len(expectedAmounts),
	// )
	// AssertNonInteractiveRecvComplete(t.t, bobTapd, 1)

	// Sleep a couple of seconds for proof transfer.
	<-time.After(5 * time.Second)

	// And now we prepare the multisig addresses for both levels. On the
	// BTC level we are going to do a 2-of-2 musig2 based multisig.
	// On the asset level we are going to do a OP_TRUE based anyonecanspend
	// scheme. The BTC level key is going to be called the "internal key".
	_, aliceInternalKey := deriveKeys(t.t, aliceTapd)
	_, bobInternalKey := deriveKeys(t.t, bobTapd)

	// We now create the loop htlc.
	blockRes := aliceLnd.RPC.GetBestBlock(&chainrpc.GetBestBlockRequest{})

	var (
		preimage = makeLndPreimage(t.t)
		hash     = preimage.Hash()
	)

	// Create the loop htlc.
	successPathScript, err := GenSuccessPathScript(
		bobInternalKey.PubKey, hash,
	)
	require.NoError(t.t, err)

	timeoutPathScript, err := GenTimeoutPathScript(
		aliceInternalKey.PubKey, int64(blockRes.BlockHeight+100),
	)
	require.NoError(t.t, err)

	// Assemble our taproot script tree from our leaves.
	// Calculate the internal aggregate key.
	aggregateKey, err := input.MuSig2CombineKeys(
		input.MuSig2Version100RC2,
		[]*btcec.PublicKey{
			aliceInternalKey.PubKey, bobInternalKey.PubKey,
		},
		true,
		&input.MuSig2Tweaks{},
	)
	require.NoError(t.t, err)

	btcInternalKey := aggregateKey.PreTweakedKey
	btcControlBlock := &txscript.ControlBlock{
		LeafVersion: txscript.BaseLeafVersion,
		InternalKey: btcInternalKey,
	}
	t.t.Logf("Internal key: %v", btcControlBlock)

	timeoutLeaf := txscript.NewBaseTapLeaf(timeoutPathScript)
	branch := txscript.NewTapBranch(
		txscript.NewBaseTapLeaf(successPathScript),
		timeoutLeaf,
	)
	siblingPreimage := commitment.NewPreimageFromBranch(branch)
	siblingPreimageBytes, _, err := commitment.MaybeEncodeTapscriptPreimage(
		&siblingPreimage,
	)
	require.NoError(t.t, err)
	t.t.Logf("Sibiling preimage: %x", siblingPreimageBytes)

	const assetsToSend = 1000
	tapScriptKey := createOpTrueScriptKey(t.t)
	t.t.Logf("Tapscript key: %v", tapScriptKey)

	// Create a new vPkt.
	assetId := asset.ID{}
	copy(assetId[:], firstBatchGenesis.AssetId)
	pkt := &tappsbt.VPacket{
		Inputs: []*tappsbt.VInput{{
			PrevID: asset.PrevID{
				ID: assetId,
			},
		}},
		Outputs:     make([]*tappsbt.VOutput, 0, 2),
		ChainParams: &address.RegressionNetTap,
	}
	pkt.Outputs = append(pkt.Outputs, &tappsbt.VOutput{
		Amount:            0,
		Type:              tappsbt.TypeSplitRoot,
		AnchorOutputIndex: 0,
		ScriptKey:         asset.NUMSScriptKey,
	})
	pkt.Outputs = append(pkt.Outputs, &tappsbt.VOutput{
		AssetVersion:      asset.Version(issuableAssets[0].Asset.AssetVersion),
		Amount:            assetsToSend,
		Interactive:       true,
		AnchorOutputIndex: 1,
		ScriptKey: asset.NewScriptKey(
			tapScriptKey.PubKey,
		),
		AnchorOutputInternalKey:      btcInternalKey,
		AnchorOutputTapscriptSibling: &siblingPreimage,
		//ProofDeliveryAddress:         &addr.ProofCourierAddr,
	})
	// pkt.Outputs[1].SetAnchorInternalKey(
	// 	keychain.KeyDescriptor{
	// 		PubKey: btcInternalKey,
	// 	}, address.RegressionNetTap.HDCoinType,
	// )

	// We now can fund the vpsbt.
	fundResp := fundPacket(t, aliceTapd, pkt)
	t.Logf("Funded packet: %v", toJSON(t.t, fundResp))
	signResp, err := aliceTapd.SignVirtualPsbt(
		ctxt, &wrpc.SignVirtualPsbtRequest{
			FundedPsbt: fundResp.FundedPsbt,
		},
	)
	require.NoError(t.t, err)
	t.Logf("Sign resp: %v", toJSON(t.t, signResp))

	fundedHtlcPkt := deserializeVPacket(
		t.t, signResp.SignedPsbt,
	)

	// for _, input := range fundedWithdrawPkt.Inputs {
	// 	t.Logf("vpkt input %v", input)
	// }

	// for _, output := range fundedWithdrawPkt.Outputs {
	// 	t.Logf("vpkt output %v", output)
	// }

	htlcVPackets := []*tappsbt.VPacket{fundedHtlcPkt}
	htlcBtcPkt, err := tapsend.PrepareAnchoringTemplate(htlcVPackets)
	require.NoError(t.t, err)

	btcPacket, activeAssets, passiveAssets, commitResp := commitVirtualPsbts(
		t.t, aliceTapd, htlcBtcPkt, htlcVPackets, nil, -1,
	)
	require.NoError(t.t, err)
	btcPacket = signPacket(t.t, aliceLnd, btcPacket)
	btcPacket = finalizePacket(t.t, aliceLnd, btcPacket)
	sendResp := logAndPublish(
		t.t, aliceTapd, btcPacket, activeAssets, passiveAssets,
		commitResp,
	)
	t.Logf("Logged transaction: %v", toJSON(t.t, sendResp))

	multiSigOutAnchor := sendResp.Transfer.Outputs[1].Anchor
	timeoutLeafHash := timeoutLeaf.TapHash()
	btcControlBlock.InclusionProof = append(
		timeoutLeafHash[:], multiSigOutAnchor.TaprootAssetRoot[:]...)

	rootHash := btcControlBlock.RootHash(successPathScript)
	tapKey := txscript.ComputeTaprootOutputKey(btcInternalKey, rootHash)

	if tapKey.SerializeCompressed()[0] ==
		secp256k1.PubKeyFormatCompressedOdd {

		btcControlBlock.OutputKeyYIsOdd = true
	}
	require.Equal(t.t, rootHash[:], multiSigOutAnchor.MerkleRoot)

	// Let's mine a transaction to make sure the transfer completes.
	expectedAmounts := []uint64{
		firstBatch.Amount - assetsToSend - firstTransfer, assetsToSend,
	}

	// Mine a block in order to be able to create the proof.
	ConfirmAndAssertOutboundTransferWithOutputs(
		t.t, t.lndHarness.Miner.Client, aliceTapd,
		sendResp, firstBatchGenesis.AssetId, expectedAmounts,
		1, 2, len(expectedAmounts),
	)

	// Get the proof for the vpsbt.
	// Parse the outpoint.
	outpoint, err := wire.NewOutPointFromString(multiSigOutAnchor.Outpoint)
	require.NoError(t.t, err)

	htlcProofRes, err := aliceTapd.ExportProof(
		ctxt, &taprpc.ExportProofRequest{
			AssetId:   firstBatchGenesis.AssetId,
			ScriptKey: tapScriptKey.PubKey.SerializeCompressed(),
			Outpoint: &taprpc.OutPoint{
				Txid:        outpoint.Hash[:],
				OutputIndex: outpoint.Index,
			},
		},
	)
	require.NoError(t.t, err)

	isValidProof, err := bobTapd.VerifyProof(
		ctxt, &taprpc.ProofFile{
			RawProofFile: htlcProofRes.RawProofFile,
		},
	)
	require.NoError(t.t, err)
	require.True(t.t, isValidProof.Valid)

	importRes, err := bobTapd.ImportProof(
		ctxt, &tapdevrpc.ImportProofRequest{
			ProofFile: htlcProofRes.RawProofFile,
		},
	)
	require.NoError(t.t, err)
	t.Logf("Imported proof: %v", toJSON(t.t, importRes))

	// Create a reader from the htlcProofFile.ProofFile bytes slice.
	htlcProofFile, err := proof.DecodeFile(htlcProofRes.RawProofFile)
	require.NoError(t.t, err)

	// Get the proofs.
	htlcProof, err := htlcProofFile.LastProof()
	require.NoError(t.t, err)

	t.Logf("HTLC proof: %v", htlcProof)

	// Do the taptree verification from the proof.
	assetCpy := htlcProof.Asset.Copy()
	assetCpy.PrevWitnesses[0].SplitCommitment = nil
	sendCommitment, err := commitment.NewAssetCommitment(
		assetCpy,
	)
	require.NoError(t.t, err)

	assetCommitment, err := commitment.NewTapCommitment(
		sendCommitment,
	)

	require.NoError(t.t, err)

	siblingHash, err := siblingPreimage.TapHash()
	require.NoError(t.t, err)

	anchorPkScript, err := tapscript.PayToAddrScript(
		*btcInternalKey, siblingHash, *assetCommitment,
	)
	require.NoError(t.t, err)

	// verify calculated pkscript matches the one in the proof.
	require.Equal(
		t.t, btcPacket.UnsignedTx.TxOut[1].PkScript, anchorPkScript,
	)

	// verify asset script key matches the op_true one.
	require.Equal(
		t.t, htlcProof.Asset.ScriptKey.PubKey, asset.NewScriptKey(tapScriptKey.PubKey).PubKey,
	)
	taprootAssetRoot := txscript.AssembleTaprootScriptTree(assetCommitment.TapLeaf()).RootNode.TapHash()
	merkleRoot := assetCommitment.TapscriptRoot(siblingHash)

	expectedTapasRoot, err := chainhash.NewHash(sendResp.Transfer.Outputs[1].Anchor.TaprootAssetRoot)
	require.NoError(t.t, err)

	expectedMerkleRoot, err := chainhash.NewHash(sendResp.Transfer.Outputs[1].Anchor.MerkleRoot)
	require.NoError(t.t, err)

	require.Equal(t.t, expectedTapasRoot, &taprootAssetRoot)
	require.Equal(t.t, expectedMerkleRoot, &merkleRoot)

	scriptKey, sweepInternalKey := deriveKeys(t.t, bobTapd)

	sweepVpkt, err := tappsbt.PacketFromProofs(
		[]*proof.Proof{htlcProof}, &address.RegressionNetTap,
	)
	require.NoError(t.t, err)

	sweepVpkt.Outputs = append(sweepVpkt.Outputs, &tappsbt.VOutput{
		AssetVersion:            asset.Version(issuableAssets[0].Asset.AssetVersion),
		Amount:                  assetsToSend,
		Interactive:             true,
		AnchorOutputIndex:       0,
		ScriptKey:               scriptKey,
		AnchorOutputInternalKey: sweepInternalKey.PubKey,
	})

	sweepVpkt.Outputs[0].SetAnchorInternalKey(
		sweepInternalKey, address.RegressionNetTap.HDCoinType,
	)

	t.Logf("input bip derivation %v", sweepVpkt.Inputs[0].TaprootBip32Derivation)

	for idx, input := range sweepVpkt.Inputs {
		t.Logf("vpkt %v input %v", idx, input)
	}
	for idx, output := range sweepVpkt.Outputs {
		t.Logf("vpkt %v output %v", idx, output)
	}

	err = tapsend.PrepareOutputAssets(ctxt, sweepVpkt)
	require.NoError(t.t, err)

	require.NoError(t.t, err)

	updateWitness(sweepVpkt.Outputs[0].Asset, getOpTrueWitness(t.t))

	sweepVPackets := []*tappsbt.VPacket{sweepVpkt}
	sweepBtcPkt, err := tapsend.PrepareAnchoringTemplate(sweepVPackets)
	require.NoError(t.t, err)
	for idx, input := range sweepBtcPkt.Inputs {
		t.Logf("btcpkt 1 %v input %v", idx, input.TaprootBip32Derivation)
	}
	for idx, output := range sweepBtcPkt.Outputs {
		t.Logf("btcpkt 1 %v output %v", idx, output)
	}

	sweepBtcPacket, sweepActiveAssets, sweepPassiveAssets, sweepCommitResp := commitVirtualPsbts(
		t.t, bobTapd, sweepBtcPkt, sweepVPackets, nil, -1,
	)
	require.NoError(t.t, err)

	feeTxOut := &wire.TxOut{
		PkScript: sweepBtcPacket.Inputs[1].WitnessUtxo.PkScript,
		Value:    sweepBtcPacket.Inputs[1].WitnessUtxo.Value,
	}

	assetTxOut := &wire.TxOut{
		PkScript: sweepBtcPacket.Inputs[0].WitnessUtxo.PkScript,
		Value:    sweepBtcPacket.Inputs[0].WitnessUtxo.Value,
	}

	sweepBtcPacket.UnsignedTx.TxIn[0].Sequence = 1
	txWitness := genSuccessWitness(
		t.t, bobLnd, *btcControlBlock, preimage, successPathScript,
		sweepBtcPacket.UnsignedTx, bobInternalKey, assetTxOut, feeTxOut,
	)

	var buf2 bytes.Buffer
	err = psbt.WriteTxWitness(&buf2, txWitness)
	require.NoError(t.t, err)
	sweepBtcPacket.Inputs[0].SighashType = txscript.SigHashDefault
	sweepBtcPacket.Inputs[0].FinalScriptWitness = buf2.Bytes()

	sweepBtcPacket = signPacket(t.t, bobLnd, sweepBtcPacket)
	sweepBtcPacket = finalizePacket(t.t, bobLnd, sweepBtcPacket)
	sweepSendResp := logAndPublish(
		t.t, bobTapd, sweepBtcPacket, sweepActiveAssets, sweepPassiveAssets,
		sweepCommitResp,
	)
	t.Logf("Logged transaction: %v", toJSON(t.t, sweepSendResp))

	// Mine a block to confirm the transfer.
	MineBlocks(t.t, t.lndHarness.Miner.Client, 6, 1)

	// _ = sendProof(
	// 	t.t, bobTapd, bobTapd, sweepSendResp,
	// )
	AssertBalanceByID(
		t.t, bobTapd, firstBatchGenesis.AssetId,
		assetsToSend+firstTransfer,
	)

	aliceAssets, err := aliceTapd.ListAssets(ctxb, &taprpc.ListAssetRequest{
		WithWitness: false,
	})
	require.NoError(t.t, err)
	t.Logf("Alice assets: %v", toJSON(t.t, aliceAssets))

	bobAssets, err := bobTapd.ListAssets(ctxb, &taprpc.ListAssetRequest{
		WithWitness: true,
	})
	require.NoError(t.t, err)
	t.Logf("Bob assets: %v", toJSON(t.t, bobAssets))
}

func signMusig2Psbt(t *testing.T, ctx context.Context, aliceLnd, bobLnd *node.HarnessNode,
	aliceSigDesc, bobSigDesc keychain.KeyDescriptor, tx *wire.MsgTx, rootHash []byte,
	prevOut *wire.TxOut) []byte {
	signers := [][]byte{
		aliceSigDesc.PubKey.SerializeCompressed(),
		bobSigDesc.PubKey.SerializeCompressed(),
	}
	// Create the musig2 sessions
	aliceSession, err := aliceLnd.RPC.Signer.MuSig2CreateSession(
		ctx, &signrpc.MuSig2SessionRequest{
			Version: signrpc.MuSig2Version_MUSIG2_VERSION_V100RC2,
			KeyLoc: &signrpc.KeyLocator{
				KeyFamily: int32(aliceSigDesc.Family),
				KeyIndex:  int32(aliceSigDesc.Index),
			},
			AllSignerPubkeys: signers,
			TaprootTweak: &signrpc.TaprootTweakDesc{
				KeySpendOnly: true,
			},
		},
	)
	require.NoError(t, err)

	bobSession, err := bobLnd.RPC.Signer.MuSig2CreateSession(
		ctx, &signrpc.MuSig2SessionRequest{
			Version: signrpc.MuSig2Version_MUSIG2_VERSION_V100RC2,
			KeyLoc: &signrpc.KeyLocator{
				KeyFamily: int32(bobSigDesc.Family),
				KeyIndex:  int32(bobSigDesc.Index),
			},
			AllSignerPubkeys: signers,
			TaprootTweak: &signrpc.TaprootTweakDesc{
				KeySpendOnly: true,
			},
		},
	)
	require.NoError(t, err)

	// Register the nonces with each other.
	regNonceRes, err := aliceLnd.RPC.Signer.MuSig2RegisterNonces(
		ctx, &signrpc.MuSig2RegisterNoncesRequest{
			SessionId:               aliceSession.SessionId,
			OtherSignerPublicNonces: [][]byte{bobSession.LocalPublicNonces},
		},
	)
	require.NoError(t, err)
	require.True(t, regNonceRes.HaveAllNonces)

	_, err = bobLnd.RPC.Signer.MuSig2RegisterNonces(
		ctx, &signrpc.MuSig2RegisterNoncesRequest{
			SessionId:               bobSession.SessionId,
			OtherSignerPublicNonces: [][]byte{aliceSession.LocalPublicNonces},
		},
	)
	require.NoError(t, err)

	prevOutFetcher := txscript.NewCannedPrevOutputFetcher(
		prevOut.PkScript, prevOut.Value,
	)
	sigHashes := txscript.NewTxSigHashes(tx, prevOutFetcher)
	taprootSigHash, err := txscript.CalcTaprootSignatureHash(
		sigHashes, txscript.SigHashDefault,
		tx, 0, prevOutFetcher,
	)
	require.NoError(t, err)

	// Now we can sign the psbt.
	aliceSignRes, err := aliceLnd.RPC.Signer.MuSig2Sign(
		ctx, &signrpc.MuSig2SignRequest{
			SessionId:     aliceSession.SessionId,
			MessageDigest: taprootSigHash,
		},
	)
	require.NoError(t, err)

	_, err = bobLnd.RPC.Signer.MuSig2Sign(
		ctx, &signrpc.MuSig2SignRequest{
			SessionId:     bobSession.SessionId,
			MessageDigest: taprootSigHash,
		},
	)
	require.NoError(t, err)

	// combine the sigs at bob
	combineSigRes, err := bobLnd.RPC.Signer.MuSig2CombineSig(
		ctx, &signrpc.MuSig2CombineSigRequest{
			SessionId: bobSession.SessionId,
			OtherPartialSignatures: [][]byte{
				aliceSignRes.LocalPartialSignature,
			},
		},
	)
	require.NoError(t, err)
	require.True(t, combineSigRes.HaveAllSignatures)

	return combineSigRes.FinalSignature
}

func pubkeyTo33Byte(pubkey *btcec.PublicKey) [33]byte {
	var pub33 [33]byte
	copy(pub33[:], pubkey.SerializeCompressed())
	return pub33
}

func makeLndPreimage(t *testing.T) lntypes.Preimage {
	// Create a random preimage
	var preimage lntypes.Preimage
	_, err := rand.Read(preimage[:])
	require.NoError(t, err)
	return preimage
}

func partialSignWithKeyTopLevel(t *testing.T, lnd *node.HarnessNode, pkt *psbt.Packet,
	inputIndex uint32, key keychain.KeyDescriptor, tapLeaf txscript.TapLeaf) []byte {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	// The lnd SignPsbt RPC doesn't really understand multi-sig yet, we
	// cannot specify multiple keys that need to sign. So what we do here
	// is just replace the derivation path info for the input we want to
	// sign to the key we want to sign with. If we do this for every signing
	// participant, we'll get the correct signatures for OP_CHECKSIGADD.
	signInput := &pkt.Inputs[inputIndex]
	derivation, trDerivation := tappsbt.Bip32DerivationFromKeyDesc(
		key, lnd.Cfg.NetParams.HDCoinType,
	)
	trDerivation.LeafHashes = [][]byte{fn.ByteSlice(tapLeaf.TapHash())}
	signInput.Bip32Derivation = []*psbt.Bip32Derivation{derivation}
	signInput.TaprootBip32Derivation = []*psbt.TaprootBip32Derivation{
		trDerivation,
	}
	signInput.SighashType = txscript.SigHashDefault

	var buf bytes.Buffer
	err := pkt.Serialize(&buf)
	require.NoError(t, err)

	resp, err := lnd.RPC.WalletKit.SignPsbt(
		ctxt, &walletrpc.SignPsbtRequest{
			FundedPsbt: buf.Bytes(),
		},
	)
	require.NoError(t, err)

	result, err := psbt.NewFromRawBytes(
		bytes.NewReader(resp.SignedPsbt), false,
	)
	require.NoError(t, err)

	// Make sure the input we wanted to sign for was actually signed.
	require.Contains(t, resp.SignedInputs, inputIndex)

	return result.Inputs[inputIndex].TaprootScriptSpendSig[0].Signature
}

func GenSuccessPathScript(receiverHtlcKey *btcec.PublicKey,
	swapHash lntypes.Hash) ([]byte, error) {

	builder := txscript.NewScriptBuilder()

	builder.AddData(schnorr.SerializePubKey(receiverHtlcKey))
	builder.AddOp(txscript.OP_CHECKSIGVERIFY)
	builder.AddOp(txscript.OP_SIZE)
	builder.AddInt64(32)
	builder.AddOp(txscript.OP_EQUALVERIFY)
	builder.AddOp(txscript.OP_HASH160)
	builder.AddData(input.Ripemd160H(swapHash[:]))
	builder.AddOp(txscript.OP_EQUALVERIFY)
	builder.AddInt64(1)
	builder.AddOp(txscript.OP_CHECKSEQUENCEVERIFY)

	return builder.Script()
}

// GenTimeoutPathScript constructs an HtlcScript for the timeout payment path.
// Largest possible bytesize of the script is 32 + 1 + 2 + 1 = 36.
//
//	<senderHtlcKey> OP_CHECKSIGVERIFY <cltvExpiry> OP_CHECKLOCKTIMEVERIFY
func GenTimeoutPathScript(senderHtlcKey *btcec.PublicKey, cltvExpiry int64) (
	[]byte, error) {

	builder := txscript.NewScriptBuilder()
	builder.AddData(schnorr.SerializePubKey(senderHtlcKey))
	builder.AddOp(txscript.OP_CHECKSIGVERIFY)
	builder.AddInt64(cltvExpiry)
	builder.AddOp(txscript.OP_CHECKLOCKTIMEVERIFY)
	return builder.Script()
}

// genSuccessWitness returns the success script to spend this htlc with
// the preimage.
func genSuccessWitness(t *testing.T, lnd *node.HarnessNode,
	controlBlock txscript.ControlBlock, preimage lntypes.Preimage,
	successScript []byte, tx *wire.MsgTx, keyDesc keychain.KeyDescriptor,
	assetTxOut *wire.TxOut, feeInputTxOut *wire.TxOut) wire.TxWitness {

	var buf bytes.Buffer
	err := tx.Serialize(&buf)
	require.NoError(t, err)

	assetSignTxOut := &signrpc.TxOut{
		PkScript: assetTxOut.PkScript,
		Value:    assetTxOut.Value,
	}
	changeSignTxOut := &signrpc.TxOut{
		PkScript: feeInputTxOut.PkScript,
		Value:    feeInputTxOut.Value,
	}
	rawSig, err := lnd.RPC.Signer.SignOutputRaw(
		context.Background(), &signrpc.SignReq{
			RawTxBytes: buf.Bytes(),
			SignDescs: []*signrpc.SignDescriptor{
				{
					KeyDesc: &signrpc.KeyDescriptor{
						KeyLoc: &signrpc.KeyLocator{
							KeyFamily: int32(keyDesc.Family),
							KeyIndex:  int32(keyDesc.Index),
						},
					},
					SignMethod:    signrpc.SignMethod_SIGN_METHOD_TAPROOT_SCRIPT_SPEND,
					WitnessScript: successScript,
					Output:        assetSignTxOut,
					Sighash:       uint32(txscript.SigHashDefault),
					InputIndex:    0,
				},
			},
			PrevOutputs: []*signrpc.TxOut{
				assetSignTxOut, changeSignTxOut,
			},
		},
	)
	require.NoError(t, err)

	controlBlockBytes, err := controlBlock.ToBytes()
	require.NoError(t, err)

	return wire.TxWitness{
		preimage[:],
		rawSig.RawSigs[0],
		successScript,
		controlBlockBytes,
	}
}
