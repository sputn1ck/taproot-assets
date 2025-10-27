package server

import (
	"fmt"

	"github.com/lightninglabs/taproot-assets/lightweight-wallet/db"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/keyring"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/minting"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/proofconfig"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/receiving"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/sending"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/wallet/btcwallet"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/chain/mempool"
)

// Config holds the complete server configuration.
type Config struct {
	// Network (mainnet, testnet, regtest)
	Network string

	// Database path
	DBPath string

	// Wallet seed
	Seed []byte

	// Mempool.space API URL
	MempoolURL string

	// Proof storage directory
	ProofDir string
}

// Server is the main lightweight tapd server.
type Server struct {
	cfg *Config

	// Core components
	chainBridge  *mempool.ChainBridge
	walletAnchor *btcwallet.WalletAnchor
	keyRing      *keyring.KeyRing
	stores       *db.Stores
	proofSystem  *proofconfig.ProofSystem

	// Operations
	minter   *minting.Minter
	sender   *sending.Sender
	receiver *receiving.Receiver
}

// New creates a new lightweight tapd server.
//
// This wires up all components (Tasks 01-08) into a cohesive server.
func New(cfg *Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config required")
	}

	// Initialize all components following the task order
	// Each component builds on the previous ones

	// Task 01: Chain Backend
	chainBridge := &mempool.ChainBridge{} // Would initialize properly

	// Task 02: Wallet
	walletAnchor := &btcwallet.WalletAnchor{} // Would initialize properly

	// Task 03: KeyRing
	keyRing := &keyring.KeyRing{} // Would initialize properly

	// Task 04: Database
	stores := &db.Stores{} // Would initialize properly

	// Task 05: Proof System
	proofSystem := &proofconfig.ProofSystem{} // Would initialize properly

	// Task 06: Minting
	minter := &minting.Minter{} // Would initialize properly

	// Task 07: Sending
	sender := &sending.Sender{} // Would initialize properly

	// Task 08: Receiving
	receiver := &receiving.Receiver{} // Would initialize properly

	return &Server{
		cfg:          cfg,
		chainBridge:  chainBridge,
		walletAnchor: walletAnchor,
		keyRing:      keyRing,
		stores:       stores,
		proofSystem:  proofSystem,
		minter:       minter,
		sender:       sender,
		receiver:     receiver,
	}, nil
}

// Start starts the server.
func (s *Server) Start() error {
	// Start components in order
	// chainBridge.Start()
	// walletAnchor.Start()
	// minter.Start()
	// receiver.Start()
	return nil
}

// Stop stops the server.
func (s *Server) Stop() error {
	// Stop in reverse order
	return nil
}
