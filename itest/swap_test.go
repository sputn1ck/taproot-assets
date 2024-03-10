package itest

import (
	"bytes"
	"context"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/lightninglabs/taproot-assets/address"
	"github.com/lightninglabs/taproot-assets/asset"
	"github.com/lightninglabs/taproot-assets/commitment"
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
	"github.com/lightningnetwork/lnd/lntest/node"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/stretchr/testify/require"
)

// testTrustlessSubmarine is an example test, which shows how to create an
// onchain htlc, that can be trustlessly claimed by a third party, via a
// preimage.
func testTrustlessSubmarineSwapPreimage(t *harnessTest) {
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
		assetsToSend      = uint64(1000)
	)

	// We create a second tapd node that will be used to simulate a second
	// party in the test. This tapd node is connected to lnd "Bob".
	bobTapd := setupTapdHarness(t.t, t, bobLnd, t.universeServer)
	defer func() {
		require.NoError(t.t, bobTapd.stop(!*noDelete))
	}()

	// Finally we'll get a block height 24 blocks from now.
	gi, err := aliceTapd.GetInfo(ctxt, &taprpc.GetInfoRequest{})
	require.NoError(t.t, err)

	// First we'll setup the contract that we'll use to create the bitcoin
	// onchain htlc.
	contract := setupBtcHtlcContract(
		t.t, ctxt, aliceTapd, bobTapd, assetsToSend, gi.BlockHeight+24,
	)

	// Next we'll create the vpacket that we'll use to create the htlc.
	assetId := asset.ID{}
	copy(assetId[:], firstBatchGenesis.AssetId)
	vpkt := createHtlcOutput(t.t, assetId, contract)

	// Now we'll fund and publish the vpacket and bitcoin tx.
	sendResp := commitAndPublishVpacket(t, ctxt, aliceTapd, aliceLnd, vpkt)

	// The expected outputs for the first transactions are the change
	// output for alice and the htlc output.
	expectedOutputs := []uint64{
		firstBatch.Amount - assetsToSend, assetsToSend,
	}

	ConfirmAndAssertOutboundTransferWithOutputs(
		t.t, t.lndHarness.Miner.Client, aliceTapd,
		sendResp, firstBatchGenesis.AssetId, expectedOutputs,
		0, 1, len(expectedOutputs),
	)

	// Now that the htlc is published, we can export the proof from alice
	// in order for bob to verify and extract the necessary infortmation
	// to claim the htlc.
	outpoint, err := wire.NewOutPointFromString(
		sendResp.Transfer.Outputs[1].Anchor.Outpoint,
	)
	require.NoError(t.t, err)

	htlcProofRes, err := aliceTapd.ExportProof(
		ctxt, &taprpc.ExportProofRequest{
			AssetId:   firstBatchGenesis.AssetId,
			ScriptKey: createOpTrueScriptKey(t.t).PubKey.SerializeCompressed(),
			Outpoint: &taprpc.OutPoint{
				Txid:        outpoint.Hash[:],
				OutputIndex: outpoint.Index,
			},
		},
	)
	require.NoError(t.t, err)

	// We'll now extract the necessary information from the proof to find
	// the htlc output onchain and claim it.
	proofInfo := verifyProofAndExtractInfo(
		t.t, ctxt, bobTapd, htlcProofRes.RawProofFile, contract,
	)

	// We'll now wait for the htlc to be confirmed onchain.
	confClient, err := bobLnd.RPC.ChainClient.RegisterConfirmationsNtfn(
		ctxt, &chainrpc.ConfRequest{
			Script:     proofInfo.pkScript,
			NumConfs:   1,
			HeightHint: gi.BlockHeight,
		},
	)
	require.NoError(t.t, err)

	// Receiver the confMsg.
	confMsg, err := confClient.Recv()
	require.NoError(t.t, err)

	t.Log("Received confirmation message: ", toJSON(t.t, confMsg))

	// Now that we know that the tx with the expected pkscript has been
	// confirmed onchain, we can claim the htlc output.
	sendResp = claimHtlcOutput(
		t.t, ctxt, bobTapd, bobLnd, contract, proofInfo,
	)

	// We now just expect 1 output with the sweep amount.
	expectedOutputs = []uint64{
		assetsToSend,
	}

	ConfirmAndAssertOutboundTransferWithOutputs(
		t.t, t.lndHarness.Miner.Client, bobTapd,
		sendResp, firstBatchGenesis.AssetId, expectedOutputs,
		0, 1, len(expectedOutputs),
	)

	AssertBalanceByID(
		t.t, bobTapd, firstBatchGenesis.AssetId,
		assetsToSend,
	)

}

// btcHtlcContract is a struct that contains all the information needed to
// create a trustless onchain htlc.
type btcHtlcContract struct {
	amount            btcutil.Amount
	senderKeyDesc     keychain.KeyDescriptor
	receiverKeyDesc   keychain.KeyDescriptor
	musig2InternalKey *btcec.PublicKey
	expiry            uint32
	swapHash          lntypes.Hash
	swapPreimage      lntypes.Preimage
}

// genSuccesPathScript generates a script that can be used to claim the htlc
// output as the receiver using a signature and a preimage.
func (b *btcHtlcContract) genSuccesPathScript(t *testing.T) []byte {
	builder := txscript.NewScriptBuilder()
	builder.AddData(schnorr.SerializePubKey(b.receiverKeyDesc.PubKey))
	builder.AddOp(txscript.OP_CHECKSIGVERIFY)
	builder.AddOp(txscript.OP_SIZE)
	builder.AddInt64(32)
	builder.AddOp(txscript.OP_EQUALVERIFY)
	builder.AddOp(txscript.OP_HASH160)
	builder.AddData(input.Ripemd160H(b.swapHash[:]))
	builder.AddOp(txscript.OP_EQUALVERIFY)
	builder.AddInt64(1)
	builder.AddOp(txscript.OP_CHECKSEQUENCEVERIFY)

	script, err := builder.Script()
	require.NoError(t, err)

	return script
}

// genSuccessControlBlock generates a control block that can be used to claim
// the htlc output as the receiver using a signature and a preimage.
func (b *btcHtlcContract) genSuccessControlBlock(t *testing.T,
	taprootAssetRoot []byte) *txscript.ControlBlock {

	timeoutLeaf := txscript.NewBaseTapLeaf(b.genTimeoutPathScript(t))
	timeoutLeafHash := timeoutLeaf.TapHash()
	inclusionProof := append(timeoutLeafHash[:], taprootAssetRoot...)
	controlBlock := &txscript.ControlBlock{
		LeafVersion:    txscript.BaseLeafVersion,
		InternalKey:    b.musig2InternalKey,
		InclusionProof: inclusionProof,
	}

	rootHash := controlBlock.RootHash(b.genSuccesPathScript(t))
	tapKey := txscript.ComputeTaprootOutputKey(
		b.musig2InternalKey, rootHash,
	)

	if tapKey.SerializeCompressed()[0] ==
		secp256k1.PubKeyFormatCompressedOdd {

		controlBlock.OutputKeyYIsOdd = true
	}

	return controlBlock
}

// genTimeoutPathScript generates a script that can be used to claim the htlc
// output as the sender using a signature and a timeout.
func (b *btcHtlcContract) genTimeoutPathScript(t *testing.T) []byte {
	builder := txscript.NewScriptBuilder()
	builder.AddData(schnorr.SerializePubKey(b.senderKeyDesc.PubKey))
	builder.AddOp(txscript.OP_CHECKSIGVERIFY)
	builder.AddInt64(int64(b.expiry))
	builder.AddOp(txscript.OP_CHECKSEQUENCEVERIFY)
	builder.AddOp(txscript.OP_DROP)

	script, err := builder.Script()
	require.NoError(t, err)

	return script
}

// genSuccessWitness returns a witness that satisfies the succes path script.
func (b *btcHtlcContract) genSuccessWitness(t *testing.T, lnd *node.HarnessNode,
	sweepBtcPacket *psbt.Packet, proofInfo proofInfo) wire.TxWitness {

	// Set the sequence number to 1 to satisfy the CHECKSEQUENCEVERIFY opcode.
	sweepBtcPacket.UnsignedTx.TxIn[0].Sequence = 1

	var buf bytes.Buffer
	err := sweepBtcPacket.UnsignedTx.Serialize(&buf)
	require.NoError(t, err)

	assetSignTxOut := &signrpc.TxOut{
		PkScript: sweepBtcPacket.Inputs[0].WitnessUtxo.PkScript,
		Value:    sweepBtcPacket.Inputs[0].WitnessUtxo.Value,
	}
	changeSignTxOut := &signrpc.TxOut{
		PkScript: sweepBtcPacket.Inputs[1].WitnessUtxo.PkScript,
		Value:    sweepBtcPacket.Inputs[1].WitnessUtxo.Value,
	}

	successScript := b.genSuccesPathScript(t)
	rawSig, err := lnd.RPC.Signer.SignOutputRaw(
		context.Background(), &signrpc.SignReq{
			RawTxBytes: buf.Bytes(),
			SignDescs: []*signrpc.SignDescriptor{
				{
					KeyDesc: &signrpc.KeyDescriptor{
						KeyLoc: &signrpc.KeyLocator{
							KeyFamily: int32(b.receiverKeyDesc.Family),
							KeyIndex:  int32(b.receiverKeyDesc.Index),
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

	// Get the success path control block.
	controlBlock := b.genSuccessControlBlock(t, proofInfo.taprootAssetRoot)

	controlBlockBytes, err := controlBlock.ToBytes()
	require.NoError(t, err)

	return wire.TxWitness{
		b.swapPreimage[:],
		rawSig.RawSigs[0],
		successScript,
		controlBlockBytes,
	}
}

// getSiblingPreimage returns a tapscript.TapScriptPreimage for the internal
// tap branch.
func (b *btcHtlcContract) getSiblingPreimage(t *testing.T) commitment.TapscriptPreimage {
	branch := txscript.NewTapBranch(
		txscript.NewBaseTapLeaf(b.genSuccesPathScript(t)),
		txscript.NewBaseTapLeaf(b.genTimeoutPathScript(t)),
	)
	return commitment.NewPreimageFromBranch(branch)
}

// createOpScriptKey creates a script key that can be used to create a taproot
// output that is spendable by anyone.
func createOpTrueScriptKey(t *testing.T) asset.ScriptKey {
	// Create the taproot OP_TRUE script.
	tapScript := getOpTrueScript(t)

	tapLeaf := txscript.NewBaseTapLeaf(tapScript)
	tree := txscript.AssembleTaprootScriptTree(tapLeaf)
	rootHash := tree.RootNode.TapHash()
	tapKey := txscript.ComputeTaprootOutputKey(asset.NUMSPubKey, rootHash[:])

	merkleRootHash := tree.RootNode.TapHash()

	tapScriptKey := asset.ScriptKey{
		PubKey: tapKey,
		TweakedScriptKey: &asset.TweakedScriptKey{
			RawKey: keychain.KeyDescriptor{
				PubKey: asset.NUMSPubKey,
			},
			Tweak: merkleRootHash[:],
		},
	}

	return tapScriptKey
}

// getOpTrueScript returns a script that pushes the OP_TRUE opcode.
func getOpTrueScript(t *testing.T) []byte {
	builder := txscript.NewScriptBuilder()
	builder.AddOp(txscript.OP_TRUE)
	script, err := builder.Script()
	require.NoError(t, err)

	return script
}

// getOpTrueWitness returns a witness that satisfies the OP_TRUE script.
func getOpTrueWitness(t *testing.T) [][]byte {
	// The valid witness need the script and the control block bytes.
	script := getOpTrueScript(t)

	tapLeaf := txscript.NewBaseTapLeaf(script)
	tree := txscript.AssembleTaprootScriptTree(tapLeaf)
	rootHash := tree.RootNode.TapHash()
	tapKey := txscript.ComputeTaprootOutputKey(asset.NUMSPubKey, rootHash[:])

	controlBlock := &txscript.ControlBlock{
		LeafVersion: txscript.BaseLeafVersion,
		InternalKey: asset.NUMSPubKey,
	}

	if tapKey.SerializeCompressed()[0] ==
		secp256k1.PubKeyFormatCompressedOdd {

		controlBlock.OutputKeyYIsOdd = true
	}

	controlBlockBytes, err := controlBlock.ToBytes()
	require.NoError(t, err)

	return wire.TxWitness{
		script,
		controlBlockBytes,
	}
}

// createHtlcOutput creates a vpacket that contains the htlc contract.
func createHtlcOutput(t *testing.T, assetId asset.ID, contract *btcHtlcContract) *tappsbt.VPacket {
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

	// For the output we need the tapscript sibling preimage.
	siblingPreimage := contract.getSiblingPreimage(t)

	pkt.Outputs = append(pkt.Outputs, &tappsbt.VOutput{
		AssetVersion:      asset.Version(issuableAssets[0].Asset.AssetVersion),
		Amount:            uint64(contract.amount),
		Interactive:       true,
		AnchorOutputIndex: 1,
		// The script key for the output is the OP_TRUE script key,
		// meaning anyone can spend the tap output.
		ScriptKey: createOpTrueScriptKey(t),
		// The internal key is the musig2 internal (pretweaked) key.
		// This allows the btc output to be spent by the musig2 signature
		// of both parties.
		AnchorOutputInternalKey: contract.musig2InternalKey,
		// The tapscript sibling preimage is the preimage of the internal
		// tap branch for the htlc contract.
		AnchorOutputTapscriptSibling: &siblingPreimage,
	})

	return pkt
}

// setupBtcHtlcContract creates a btcHtlcContract with the given amount and
// parties.
func setupBtcHtlcContract(t *testing.T, ctx context.Context, senderTapd,
	receiverTapd *tapdHarness, amount uint64, expiry uint32,
) *btcHtlcContract {

	// First we'll create a preimage and hash.
	preimage := makeLndPreimage(t)
	pHash := preimage.Hash()

	// Next we'll create 2 internal Lnd keys for alice and bob.
	_, senderInternalKey := deriveKeys(t, senderTapd)
	_, receiverInternalKey := deriveKeys(t, receiverTapd)

	// Now we can create the musig2 internal key.
	aggregateKey, err := input.MuSig2CombineKeys(
		input.MuSig2Version100RC2,
		[]*btcec.PublicKey{
			senderInternalKey.PubKey, receiverInternalKey.PubKey,
		},
		true,
		&input.MuSig2Tweaks{},
	)
	require.NoError(t, err)

	// The musig2 key is the pre aggregate key, as tapd will tweak it with
	// the hash of the tapscript.
	musig2InternalKey := aggregateKey.PreTweakedKey

	return &btcHtlcContract{
		amount:            btcutil.Amount(amount),
		senderKeyDesc:     senderInternalKey,
		receiverKeyDesc:   receiverInternalKey,
		musig2InternalKey: musig2InternalKey,
		expiry:            expiry,
		swapHash:          pHash,
		swapPreimage:      preimage,
	}
}

func claimHtlcOutput(t *testing.T, ctx context.Context, tapd *tapdHarness,
	lnd *node.HarnessNode, contract *btcHtlcContract, proofInfo proofInfo,
) *taprpc.SendAssetResponse {

	// First we'll create the keys we'll send the funds to.
	scriptKey, sweepInternalKey := deriveKeys(t, tapd)

	// Next we'll create the vpkt from the proof.
	sweepVpkt, err := tappsbt.PacketFromProofs(
		[]*proof.Proof{proofInfo.proof}, &address.RegressionNetTap,
	)
	require.NoError(t, err)

	// Next we'll add the output to the packet.
	sweepVpkt.Outputs = append(sweepVpkt.Outputs, &tappsbt.VOutput{
		AssetVersion:            asset.Version(issuableAssets[0].Asset.AssetVersion),
		Amount:                  uint64(contract.amount),
		Interactive:             true,
		AnchorOutputIndex:       0,
		ScriptKey:               scriptKey,
		AnchorOutputInternalKey: sweepInternalKey.PubKey,
	})

	sweepVpkt.Outputs[0].SetAnchorInternalKey(
		sweepInternalKey, address.RegressionNetTap.HDCoinType,
	)

	// Now we'll prepare our vpacket. This fills in the asset.Asset struct
	// data we'll need to commit the transaction.
	err = tapsend.PrepareOutputAssets(ctx, sweepVpkt)
	require.NoError(t, err)

	// Now we'll update the witness of our taproot assets output.
	updateWitness(sweepVpkt.Outputs[0].Asset, getOpTrueWitness(t))

	// Next we'll prepare the anchoring of the vpacket.
	sweepVPackets := []*tappsbt.VPacket{sweepVpkt}
	sweepBtcPkt, err := tapsend.PrepareAnchoringTemplate(sweepVPackets)
	require.NoError(t, err)

	sweepBtcPacket, sweepActiveAssets, sweepPassiveAssets,
		sweepCommitResp := commitVirtualPsbts(

		t, tapd, sweepBtcPkt, sweepVPackets, nil, -1,
	)
	require.NoError(t, err)

	// We'll no get the successWitness
	successWitness := contract.genSuccessWitness(
		t, lnd, sweepBtcPacket, proofInfo,
	)

	var buf bytes.Buffer
	err = psbt.WriteTxWitness(&buf, successWitness)
	require.NoError(t, err)
	sweepBtcPacket.Inputs[0].SighashType = txscript.SigHashDefault
	sweepBtcPacket.Inputs[0].FinalScriptWitness = buf.Bytes()

	sweepBtcPacket = signPacket(t, lnd, sweepBtcPacket)
	sweepBtcPacket = finalizePacket(t, lnd, sweepBtcPacket)
	sweepSendResp := logAndPublish(
		t, tapd, sweepBtcPacket, sweepActiveAssets, sweepPassiveAssets,
		sweepCommitResp,
	)

	return sweepSendResp
}

// proofInfo return the required information to listen onchain for the htlc
// and claim it.
type proofInfo struct {
	// pkScript is the onchain script that the receiver can use to listen
	// for the htlc output.
	pkScript []byte
	// taprootAssetRoot is the root hash of the taproot asset tree.
	// This is later used to create the taproot control block to claim
	// the onchain htlc.
	taprootAssetRoot []byte

	// proof contains the deserialized proof that can be used to claim the
	// htlc output.
	proof *proof.Proof
}

// verifyProofAndExtractInfo verifies the htlc proof and extracts the necessary
// information to listen onchain for the htlc output.
func verifyProofAndExtractInfo(t *testing.T, ctx context.Context,
	tapd *tapdHarness, proofFile []byte, htlcContract *btcHtlcContract,
) proofInfo {

	// First we'll import the proof into the tapd instance. This will return
	// an error if the proof is invalid.
	_, err := tapd.ImportProof(
		ctx, &tapdevrpc.ImportProofRequest{
			ProofFile: proofFile,
		},
	)
	require.NoError(t, err)

	// Create a reader from the htlcProofFile.ProofFile bytes slice.
	htlcProofFile, err := proof.DecodeFile(proofFile)
	require.NoError(t, err)

	htlcProof, err := htlcProofFile.LastProof()
	require.NoError(t, err)

	// The pkscript of the htlc output can be calculated from the proof
	// and the known parameters of our htlc contract.
	assetCpy := htlcProof.Asset.Copy()
	assetCpy.PrevWitnesses[0].SplitCommitment = nil
	sendCommitment, err := commitment.NewAssetCommitment(
		assetCpy,
	)
	require.NoError(t, err)

	assetCommitment, err := commitment.NewTapCommitment(
		sendCommitment,
	)
	require.NoError(t, err)

	siblingPreimage := htlcContract.getSiblingPreimage(t)

	siblingHash, err := siblingPreimage.TapHash()
	require.NoError(t, err)

	// The anchorPkScript is based on the musig2 internal key, the sibling
	// hash from the htlc script and the asset commitment.
	anchorPkScript, err := tapscript.PayToAddrScript(
		*htlcContract.musig2InternalKey, siblingHash, *assetCommitment,
	)
	require.NoError(t, err)

	// The taproot asset root is the root hash of the tapscript tree.

	taprootAssetRoot := txscript.AssembleTaprootScriptTree(
		assetCommitment.TapLeaf(),
	).RootNode.TapHash()

	return proofInfo{
		pkScript:         anchorPkScript,
		taprootAssetRoot: taprootAssetRoot[:],
		proof:            htlcProof,
	}

}

// commitAndPublishVpacket commits a vpacket to a bitcoin anchor output and
// publishes the transaction to the network.
func commitAndPublishVpacket(t *harnessTest, ctx context.Context,
	tapd *tapdHarness, lnd *node.HarnessNode, htlcVpkt *tappsbt.VPacket,
) *taprpc.SendAssetResponse {
	// First we'll fund the vpacket and sign it.
	fundResp := fundPacket(t, tapd, htlcVpkt)
	signResp, err := tapd.SignVirtualPsbt(
		ctx, &wrpc.SignVirtualPsbtRequest{
			FundedPsbt: fundResp.FundedPsbt,
		},
	)
	require.NoError(t.t, err)

	fundedHtlcPkt := deserializeVPacket(
		t.t, signResp.SignedPsbt,
	)

	htlcVPackets := []*tappsbt.VPacket{fundedHtlcPkt}
	htlcBtcPkt, err := tapsend.PrepareAnchoringTemplate(htlcVPackets)
	require.NoError(t.t, err)

	btcPacket, activeAssets, passiveAssets, commitResp := commitVirtualPsbts(
		t.t, tapd, htlcBtcPkt, htlcVPackets, nil, -1,
	)
	require.NoError(t.t, err)
	btcPacket = signPacket(t.t, lnd, btcPacket)
	btcPacket = finalizePacket(t.t, lnd, btcPacket)
	sendResp := logAndPublish(
		t.t, tapd, btcPacket, activeAssets, passiveAssets,
		commitResp,
	)

	t.Logf("Logged transaction: %v", toJSON(t.t, sendResp))

	return sendResp
}
