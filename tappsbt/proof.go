package tappsbt

import (
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/lightninglabs/taproot-assets/address"
	"github.com/lightninglabs/taproot-assets/asset"
	"github.com/lightninglabs/taproot-assets/commitment"
	"github.com/lightninglabs/taproot-assets/proof"
)

// PacketFromProofs creates a packet from the given proofs that adds them as
// inputs to the packet.
func PacketFromProofs(proofs []*proof.Proof, params *address.ChainParams) (*VPacket, error) {

	pkt := &VPacket{
		ChainParams: params,
		Version:     0,
	}

	for idx := range proofs {
		p := proofs[idx]

		txOut := p.AnchorTx.TxOut[p.InclusionProof.OutputIndex]
		_, tapCommitment, err := p.InclusionProof.DeriveByAssetInclusion(
			&p.Asset,
		)
		if err != nil {
			return nil, fmt.Errorf("error deriving commitment: %w",
				err)
		}

		tapProof := p.InclusionProof.CommitmentProof
		siblingBytes, sibling, err := commitment.MaybeEncodeTapscriptPreimage(
			tapProof.TapSiblingPreimage,
		)
		if err != nil {
			return nil, fmt.Errorf("error encoding taproot "+
				"sibling: %w", err)
		}

		rootHash := tapCommitment.TapscriptRoot(sibling)
		pkt.Inputs = append(pkt.Inputs, &VInput{
			PrevID: asset.PrevID{
				OutPoint: p.OutPoint(),
				ID:       p.Asset.ID(),
				ScriptKey: asset.ToSerialized(
					p.Asset.ScriptKey.PubKey,
				),
			},
			Anchor: Anchor{
				Value:            btcutil.Amount(txOut.Value),
				PkScript:         txOut.PkScript,
				SigHashType:      txscript.SigHashDefault,
				InternalKey:      p.InclusionProof.InternalKey,
				MerkleRoot:       rootHash[:],
				TapscriptSibling: siblingBytes,
			},
			Proof: p,
		})
		pkt.SetInputAsset(len(pkt.Inputs)-1, &p.Asset)
	}

	return pkt, nil
}
