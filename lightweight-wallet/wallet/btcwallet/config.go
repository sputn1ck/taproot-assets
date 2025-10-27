package btcwallet

import (
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/chain/mempool"
)

// Config holds the configuration for the btcwallet-based WalletAnchor.
type Config struct {
	// NetParams is the network parameters (mainnet, testnet, etc.)
	NetParams *chaincfg.Params

	// DBPath is the path to the wallet database.
	// If empty, an in-memory wallet will be used.
	DBPath string

	// PrivatePass is the private passphrase for the wallet.
	PrivatePass []byte

	// PublicPass is the public passphrase for the wallet.
	PublicPass []byte

	// Seed is the wallet seed for key derivation.
	// If provided, will be used to initialize a new wallet.
	Seed []byte

	// Birthday is the wallet birthday (earliest time to scan for transactions).
	// If zero, will scan from genesis.
	Birthday time.Time

	// ChainBridge is the chain backend for transaction monitoring.
	ChainBridge *mempool.ChainBridge

	// RecoveryWindow is the number of addresses to generate during recovery.
	// Default: 250
	RecoveryWindow uint32

	// MinConfs is the minimum confirmations for coin selection.
	// Default: 1
	MinConfs uint32

	// AccountGapLimit is the gap limit for account discovery.
	// Default: 20
	AccountGapLimit uint32
}

// DefaultConfig returns a default configuration.
func DefaultConfig(chainBridge *mempool.ChainBridge) *Config {
	return &Config{
		NetParams:       &chaincfg.TestNet3Params,
		PrivatePass:     []byte("password"),
		PublicPass:      []byte(wallet.InsecurePubPassphrase),
		RecoveryWindow:  250,
		MinConfs:        1,
		AccountGapLimit: 20,
		ChainBridge:     chainBridge,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.NetParams == nil {
		return ErrInvalidNetParams
	}

	if c.ChainBridge == nil {
		return ErrChainBridgeRequired
	}

	if len(c.PrivatePass) == 0 {
		return ErrPrivatePassRequired
	}

	return nil
}
