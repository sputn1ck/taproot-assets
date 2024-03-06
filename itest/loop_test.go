package itest

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	tap "github.com/lightninglabs/taproot-assets"
	"github.com/lightninglabs/taproot-assets/asset"
	"github.com/lightninglabs/taproot-assets/commitment"
	"github.com/lightninglabs/taproot-assets/fn"
	"github.com/lightninglabs/taproot-assets/proof"
	"github.com/lightninglabs/taproot-assets/tappsbt"
	"github.com/lightninglabs/taproot-assets/taprpc"
	wrpc "github.com/lightninglabs/taproot-assets/taprpc/assetwalletrpc"
	"github.com/lightninglabs/taproot-assets/taprpc/mintrpc"
	"github.com/lightninglabs/taproot-assets/taprpc/tapdevrpc"
	unirpc "github.com/lightninglabs/taproot-assets/taprpc/universerpc"
	"github.com/lightninglabs/taproot-assets/tapsend"
	"github.com/lightninglabs/taproot-assets/universe"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnrpc/chainrpc"
	"github.com/lightningnetwork/lnd/lnrpc/signrpc"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/lntest/node"
	"github.com/lightningnetwork/lnd/lntest/wait"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/stretchr/testify/require"
)

func testLoopSwap(t *harnessTest) {
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
	successPathScript, err := GenSuccessPathScript(bobInternalKey.PubKey, hash)
	require.NoError(t.t, err)

	timeoutPathScript, err := GenTimeoutPathScript(aliceInternalKey.PubKey, int64(blockRes.BlockHeight+100))
	require.NoError(t.t, err)

	// Assemble our taproot script tree from our leaves.
	branch := txscript.NewTapBranch(
		txscript.NewBaseTapLeaf(successPathScript),
		txscript.NewBaseTapLeaf(timeoutPathScript),
	)
	siblingPreimage := commitment.NewPreimageFromBranch(branch)
	siblingPreimageBytes, _, err := commitment.MaybeEncodeTapscriptPreimage(
		&siblingPreimage,
	)
	require.NoError(t.t, err)

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

	const assetsToSend = 1000
	tapScriptKey, _, _, _ := createOpTrueLeaf(
		t.t,
	)

	deliveryAddrStr := fmt.Sprintf(
		"%s://%s", proof.UniverseRpcCourierType,
		t.universeServer.ListenAddr,
	)
	//deliveryAddr, err := url.Parse(deliveryAddrStr)
	//require.NoError(t.t, err)

	tapdAddr, err := bobTapd.NewAddr(ctxt, &taprpc.NewAddrRequest{
		AssetId:   firstBatchGenesis.AssetId,
		Amt:       assetsToSend,
		ScriptKey: tap.MarshalScriptKey(tapScriptKey),
		InternalKey: &taprpc.KeyDescriptor{
			RawKeyBytes: pubKeyBytes(aggregateKey.PreTweakedKey),
		},
		ProofCourierAddr: deliveryAddrStr,
		TapscriptSibling: siblingPreimageBytes,
	})
	require.NoError(t.t, err)

	// Now we can create our virtual transaction and ask Alice's tapd to
	// fund it.
	sendResp, err := aliceTapd.SendAsset(ctxt, &taprpc.SendAssetRequest{
		TapAddrs: []string{tapdAddr.Encoded},
	})
	require.NoError(t.t, err)

	t.Logf("Initial transaction: %v", toJSON(t.t, sendResp))

	// Let's mine a transaction to make sure the transfer completes.
	expectedAmounts := []uint64{
		firstBatch.Amount - assetsToSend, assetsToSend,
	}
	ConfirmAndAssertOutboundTransferWithOutputs(
		t.t, t.lndHarness.Miner.Client, aliceTapd,
		sendResp, firstBatchGenesis.AssetId, expectedAmounts,
		0, 1, len(expectedAmounts),
	)

	// And now the event should be completed on both sides.
	AssertAddrEvent(t.t, bobTapd, tapdAddr, 1, statusCompleted)
	AssertNonInteractiveRecvComplete(t.t, bobTapd, 1)
	AssertBalanceByID(
		t.t, bobTapd, firstBatchGenesis.AssetId, assetsToSend,
	)

	// We have now stored our assets in a double-multisig protected TAP
	// address. Let's now try to spend them back to Alice. Let's create a
	// virtual transaction that sends half of the assets back to Alice.
	withdrawAddr2, err := bobTapd.NewAddr(ctxt, &taprpc.NewAddrRequest{
		AssetId: firstBatchGenesis.AssetId,
		Amt:     assetsToSend,
	})
	require.NoError(t.t, err)

	// We fund this withdrawal transaction from Bob's tapd which only has
	// the multisig locked assets currently.
	withdrawRecipients := map[string]uint64{
		withdrawAddr2.Encoded: withdrawAddr2.Amount,
	}
	withdrawFundResp, err := bobTapd.FundVirtualPsbt(
		ctxt, &wrpc.FundVirtualPsbtRequest{
			Template: &wrpc.FundVirtualPsbtRequest_Raw{
				Raw: &wrpc.TxTemplate{
					Recipients: withdrawRecipients,
				},
			},
		},
	)
	require.NoError(t.t, err)

	fundedWithdrawPkt := deserializeVPacket(
		t.t, withdrawFundResp.FundedPsbt,
	)

	for _, input := range fundedWithdrawPkt.Inputs {
		t.Logf("vpkt input %v", input)
	}

	for _, output := range fundedWithdrawPkt.Outputs {
		t.Logf("vpkt output %v", output)
	}

	vPackets := []*tappsbt.VPacket{fundedWithdrawPkt}
	withdrawBtcPkt, err := tapsend.PrepareAnchoringTemplate(vPackets)
	require.NoError(t.t, err)

	// By committing the virtual transaction to the BTC template we created,
	// Bob's lnd node will fund the BTC level transaction with an input to
	// pay for the fees (and it will also add a change output).
	btcWithdrawPkt, finalizedWithdrawPackets, _, commitResp := commitVirtualPsbts(
		t.t, bobTapd, withdrawBtcPkt, vPackets, nil, -1,
	)
	require.NoError(t.t, err)

	newRootHash := sendResp.Transfer.Outputs[1].Anchor.MerkleRoot

	// Get the prevout for bobs bitcoin fee input
	t.Logf("Fee Input: %v", spew.Sdump(btcWithdrawPkt.Inputs[1]))
	multiPrevOutFetcher := txscript.NewMultiPrevOutFetcher(nil)
	for idx := range btcWithdrawPkt.Inputs {
		prevOut := btcWithdrawPkt.Inputs[idx].WitnessUtxo
		multiPrevOutFetcher.AddPrevOut(
			btcWithdrawPkt.UnsignedTx.TxIn[idx].PreviousOutPoint, prevOut,
		)
	}

	// We now get the the signature for the anchor tx.
	sig := signMusig2Psbt(
		t.t, ctxt, aliceLnd, bobLnd, aliceInternalKey, bobInternalKey,
		btcWithdrawPkt.UnsignedTx.Copy(), newRootHash, multiPrevOutFetcher,
	)

	txWitness := wire.TxWitness{
		sig,
	}
	var buf2 bytes.Buffer
	err = psbt.WriteTxWitness(&buf2, txWitness)
	require.NoError(t.t, err)

	btcWithdrawPkt.Inputs[0].FinalScriptWitness = buf2.Bytes()

	// Finalize the packet.
	signedPkt := finalizePacket(t.t, bobLnd, btcWithdrawPkt)

	logResp := logAndPublish(
		t.t, bobTapd, signedPkt, finalizedWithdrawPackets, nil,
		commitResp,
	)

	t.Logf("Logged transaction: %v", toJSON(t.t, logResp))

	// Mine a block to confirm the transfer.
	MineBlocks(t.t, t.lndHarness.Miner.Client, 1, 1)

	AssertAddrEvent(t.t, bobTapd, withdrawAddr2, 1, statusCompleted)
	AssertBalanceByID(
		t.t, bobTapd, firstBatchGenesis.AssetId,
		assetsToSend,
	)

}

func signMusig2Psbt(t *testing.T, ctx context.Context, aliceLnd, bobLnd *node.HarnessNode,
	aliceSigDesc, bobSigDesc keychain.KeyDescriptor, tx *wire.MsgTx, rootHash []byte,
	prevOutFetcher txscript.PrevOutputFetcher) []byte {
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
				ScriptRoot: rootHash,
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
				ScriptRoot: rootHash,
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

	sigHashes := txscript.NewTxSigHashes(tx, prevOutFetcher)
	taprootSigHash, err := txscript.CalcTaprootSignatureHash(
		sigHashes, txscript.SigHashDefault,
		tx, 0, prevOutFetcher,
	)
	require.NoError(t, err)
	t.Logf("Taproot sig hash: %x", taprootSigHash)

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

func createOpTrueLeaf(t *testing.T) (asset.ScriptKey, txscript.TapLeaf,
	*txscript.IndexedTapScriptTree, *txscript.ControlBlock) {

	// Create the taproot OP_TRUE script.
	tapScript, err := txscript.NewScriptBuilder().AddOp(txscript.OP_TRUE).Script()
	require.NoError(t, err)

	tapLeaf := txscript.NewBaseTapLeaf(tapScript)
	tree := txscript.AssembleTaprootScriptTree(tapLeaf)
	rootHash := tree.RootNode.TapHash()
	tapKey := txscript.ComputeTaprootOutputKey(asset.NUMSPubKey, rootHash[:])

	merkleRootHash := tree.RootNode.TapHash()

	controlBlock := &txscript.ControlBlock{
		LeafVersion: txscript.BaseLeafVersion,
		InternalKey: asset.NUMSPubKey,
	}
	tapScriptKey := asset.ScriptKey{
		PubKey: tapKey,
		TweakedScriptKey: &asset.TweakedScriptKey{
			RawKey: keychain.KeyDescriptor{
				PubKey: asset.NUMSPubKey,
			},
			Tweak: merkleRootHash[:],
		},
	}
	if tapKey.SerializeCompressed()[0] ==
		secp256k1.PubKeyFormatCompressedOdd {

		controlBlock.OutputKeyYIsOdd = true
	}

	return tapScriptKey, tapLeaf, tree, controlBlock
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

// sendUniProof manually exports a proof from the given source using the
// universe RPCs and then imports it into the destination node.
func sendUniProof(t *harnessTest, src, dst *tapdHarness, scriptKey []byte,
	genInfo *taprpc.GenesisInfo, group *taprpc.AssetGroup,
	outpoint string) *tapdevrpc.ImportProofResponse {

	ctxb := context.Background()
	ctxt, cancel := context.WithTimeout(ctxb, defaultWaitTimeout)
	defer cancel()

	fetchUniProof := func(ctx context.Context,
		loc proof.Locator) (proof.Blob, error) {

		uniID := universe.Identifier{
			AssetID: *loc.AssetID,
		}
		if loc.GroupKey != nil {
			uniID.GroupKey = loc.GroupKey
		}

		rpcUniID, err := tap.MarshalUniID(uniID)
		require.NoError(t.t, err)

		op := &unirpc.Outpoint{
			HashStr: loc.OutPoint.Hash.String(),
			Index:   int32(loc.OutPoint.Index),
		}
		scriptKeyBytes := loc.ScriptKey.SerializeCompressed()

		uniProof, err := src.QueryProof(ctx, &unirpc.UniverseKey{
			Id: rpcUniID,
			LeafKey: &unirpc.AssetKey{
				Outpoint: &unirpc.AssetKey_Op{
					Op: op,
				},
				ScriptKey: &unirpc.AssetKey_ScriptKeyBytes{
					ScriptKeyBytes: scriptKeyBytes,
				},
			},
		})
		if err != nil {
			return nil, err
		}

		return uniProof.AssetLeaf.Proof, nil
	}

	var assetID asset.ID
	copy(assetID[:], genInfo.AssetId)

	scriptPubKey, err := btcec.ParsePubKey(scriptKey)
	require.NoError(t.t, err)

	op, err := wire.NewOutPointFromString(outpoint)
	require.NoError(t.t, err)

	loc := proof.Locator{
		AssetID:   &assetID,
		ScriptKey: *scriptPubKey,
		OutPoint:  op,
	}

	if group != nil {
		groupKey, err := btcec.ParsePubKey(group.TweakedGroupKey)
		require.NoError(t.t, err)

		loc.GroupKey = groupKey
	}

	var proofFile *proof.File
	err = wait.NoError(func() error {
		proofFile, err = proof.FetchProofProvenance(
			ctxt, nil, loc, fetchUniProof,
		)
		return err
	}, defaultWaitTimeout)
	require.NoError(t.t, err)

	var buf bytes.Buffer
	err = proofFile.Encode(&buf)
	require.NoError(t.t, err)

	t.Logf("Importing proof %x", buf.Bytes())

	importResp, err := dst.ImportProof(ctxb, &tapdevrpc.ImportProofRequest{
		ProofFile:    buf.Bytes(),
		GenesisPoint: genInfo.GenesisPoint,
	})
	require.NoError(t.t, err)

	return importResp
}
