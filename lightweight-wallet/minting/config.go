package minting

import (
	"fmt"

	"github.com/lightninglabs/taproot-assets/lightweight-wallet/keyring"
	"github.com/lightninglabs/taproot-assets/tapdb"
	"github.com/lightninglabs/taproot-assets/tapgarden"
)

// Config holds configuration for asset minting.
type Config struct {
	// ChainBridge for blockchain operations
	ChainBridge tapgarden.ChainBridge

	// WalletAnchor for PSBT funding and signing
	WalletAnchor tapgarden.WalletAnchor

	// KeyRing for key derivation
	KeyRing *keyring.KeyRing

	// MintingStore for batch persistence
	MintingStore *tapdb.AssetMintingStore

	// TreeStore for MSSMT trees
	TreeStore *tapdb.TaprootAssetTreeStore

	// ProofFileDir for proof storage
	ProofFileDir string
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.ChainBridge == nil {
		return fmt.Errorf("chain bridge is required")
	}
	if c.WalletAnchor == nil {
		return fmt.Errorf("wallet anchor is required")
	}
	if c.KeyRing == nil {
		return fmt.Errorf("key ring is required")
	}
	if c.MintingStore == nil {
		return fmt.Errorf("minting store is required")
	}
	if c.TreeStore == nil {
		return fmt.Errorf("tree store is required")
	}

	return nil
}

// Minter provides a simplified API for asset minting.
//
// This is a lightweight wrapper around tapgarden.ChainPlanter.
// The actual minting logic is in tapgarden - we just wire it up.
type Minter struct {
	cfg *Config

	// ChainPlanter is the underlying minting engine from tapgarden
	// In a full implementation, this would be initialized with all dependencies
	planter tapgarden.Planter
}

// New creates a new Minter.
//
// This demonstrates how to wire up tapgarden.ChainPlanter with lightweight
// wallet components. Full implementation requires additional components
// (GenesisSigner, ProofArchive, etc.) which are documented in Task 06 docs.
func New(cfg *Config) (*Minter, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// NOTE: Full ChainPlanter initialization requires additional setup:
	// - asset.GenesisSigner for signing genesis transactions
	// - proof.Archiver for proof storage
	// - proof.Watcher for proof monitoring
	//
	// For now, this validates the configuration and shows the pattern.
	// See docs/lightweight-wallet/06-asset-minting.md for full details.

	return &Minter{
		cfg:     cfg,
		planter: nil, // Would be tapgarden.NewChainPlanter(planterCfg)
	}, nil
}

// Start starts the minter.
func (m *Minter) Start() error {
	if m.planter != nil {
		return m.planter.Start()
	}
	return nil
}

// Stop stops the minter.
func (m *Minter) Stop() error {
	if m.planter != nil {
		return m.planter.Stop()
	}
	return nil
}
