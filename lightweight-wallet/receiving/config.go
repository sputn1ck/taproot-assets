package receiving

import (
	"fmt"

	"github.com/lightninglabs/taproot-assets/address"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/keyring"
	"github.com/lightninglabs/taproot-assets/tapdb"
	"github.com/lightninglabs/taproot-assets/tapgarden"
)

// Config holds configuration for asset receiving.
type Config struct {
	// ChainBridge for blockchain monitoring
	ChainBridge tapgarden.ChainBridge

	// WalletAnchor for transaction monitoring
	WalletAnchor tapgarden.WalletAnchor

	// KeyRing for key derivation
	KeyRing *keyring.KeyRing

	// AddrBook for address management
	AddrBook *tapdb.TapAddressBook

	// AssetStore for asset storage
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
	if c.AddrBook == nil {
		return fmt.Errorf("address book required")
	}
	if c.AssetStore == nil {
		return fmt.Errorf("asset store required")
	}
	return nil
}

// Receiver handles asset receiving operations.
//
// Wraps tapgarden.Custodian and address.Book with lightweight components.
type Receiver struct {
	cfg *Config

	// AddrBook for address generation
	addrBook *address.Book

	// Custodian monitors incoming assets
	custodian *tapgarden.Custodian
}

// New creates a new Receiver.
//
// Demonstrates wiring address.Book and tapgarden.Custodian.
// Full implementation in docs/lightweight-wallet/08-asset-receiving.md
func New(cfg *Config) (*Receiver, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Receiver{
		cfg:       cfg,
		addrBook:  nil, // Would be address.NewBook(bookCfg)
		custodian: nil, // Would be tapgarden.NewCustodian(custodianCfg)
	}, nil
}
