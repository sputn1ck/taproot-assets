package proofconfig

import (
	"bytes"
	"context"

	"github.com/lightninglabs/taproot-assets/proof"
	"github.com/lightninglabs/taproot-assets/tapdb"
	"github.com/lightninglabs/taproot-assets/tapgarden"
)

// Config holds configuration for the proof system.
type Config struct {
	// ProofFileDir is the directory where proof files are stored.
	ProofFileDir string

	// ChainBridge is used for blockchain lookups during verification.
	ChainBridge tapgarden.ChainBridge

	// AssetStore is used for database proof archival.
	AssetStore *tapdb.AssetStore
}

// ProofSystem holds all proof-related components.
//
// This is a simple wrapper around existing proof components.
// All actual proof logic is in the proof/ package - we just wire it up.
type ProofSystem struct {
	// ChainBridge for proof verification
	ChainBridge tapgarden.ChainBridge

	// AssetStore for proof metadata
	AssetStore *tapdb.AssetStore

	// Verifier for proof verification
	Verifier proof.Verifier
}

// New creates a new ProofSystem.
//
// This wires up existing proof components without modification.
// The proof package is reused as-is.
func New(cfg *Config) (*ProofSystem, error) {
	if cfg == nil || cfg.ChainBridge == nil || cfg.AssetStore == nil {
		return nil, ErrInvalidConfig
	}

	// Use the BaseVerifier from proof package
	verifier := &proof.BaseVerifier{}

	return &ProofSystem{
		ChainBridge: cfg.ChainBridge,
		AssetStore:  cfg.AssetStore,
		Verifier:    verifier,
	}, nil
}

// VerifyProof verifies a proof blob using our ChainBridge.
//
// This is a convenience wrapper around proof.BaseVerifier.Verify
// that sets up the verification context with our lightweight implementations.
func (ps *ProofSystem) VerifyProof(ctx context.Context, proofBlob proof.Blob) (*proof.AssetSnapshot, error) {
	// Create verifier context using our ChainBridge
	// Note: Proof.Blob is []byte, need to wrap in reader
	vCtx := proof.VerifierCtx{
		ChainLookupGen: ps.ChainBridge,
		// Other verifiers can be added as needed
	}

	// Convert blob to reader
	reader := bytes.NewReader(proofBlob)

	return ps.Verifier.Verify(ctx, reader, vCtx)
}
