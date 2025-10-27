package sending

import (
	"fmt"

	"github.com/lightninglabs/taproot-assets/lightweight-wallet/keyring"
	"github.com/lightninglabs/taproot-assets/tapdb"
	"github.com/lightninglabs/taproot-assets/tapfreighter"
	"github.com/lightninglabs/taproot-assets/tapgarden"
)

// Config holds configuration for asset sending.
type Config struct {
	// ChainBridge for blockchain operations
	ChainBridge tapgarden.ChainBridge

	// WalletAnchor for PSBT operations
	WalletAnchor tapfreighter.WalletAnchor

	// KeyRing for key derivation
	KeyRing *keyring.KeyRing

	// AssetStore for asset queries
	AssetStore *tapdb.AssetStore
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.ChainBridge == nil {
		return fmt.Errorf("chain bridge required")
	}
	if c.WalletAnchor == nil {
		return fmt.Errorf("wallet anchor required")
	}
	if c.KeyRing == nil {
		return fmt.Errorf("key ring required")
	}
	if c.AssetStore == nil {
		return fmt.Errorf("asset store required")
	}
	return nil
}

// Sender provides asset sending operations.
//
// Wraps tapfreighter.ChainPorter with lightweight components.
type Sender struct {
	cfg *Config

	// ChainPorter is the underlying transfer engine
	porter tapfreighter.Porter
}

// New creates a new Sender.
//
// Demonstrates wiring tapfreighter.ChainPorter with lightweight components.
// Full implementation in docs/lightweight-wallet/07-asset-sending.md
func New(cfg *Config) (*Sender, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Sender{
		cfg:    cfg,
		porter: nil, // Would be tapfreighter.NewChainPorter(porterCfg)
	}, nil
}
