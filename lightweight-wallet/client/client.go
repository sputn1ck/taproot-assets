package client

import (
	"context"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/taproot-assets/asset"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/chain/mempool"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/db"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/keyring"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/minting"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/proofconfig"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/receiving"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/sending"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/wallet/btcwallet"
	"github.com/lightninglabs/taproot-assets/tapdb"
)

// Config holds client configuration.
type Config struct {
	// Network parameters
	Network string // "mainnet", "testnet", "regtest"

	// Database path
	DBPath string

	// Wallet seed (32 bytes)
	Seed []byte

	// Mempool.space API URL
	MempoolURL string

	// Proof storage directory
	ProofDir string
}

// Client is the main lightweight tapd client for embedding in Go applications.
type Client struct {
	cfg *Config

	// Core components
	chainBridge  *mempool.ChainBridge
	walletAnchor *btcwallet.WalletAnchor
	keyRing      *keyring.KeyRing
	dbStore      *tapdb.SqliteStore
	stores       *db.Stores
	proofSystem  *proofconfig.ProofSystem

	// Operations
	minter   *minting.Minter
	sender   *sending.Sender
	receiver *receiving.Receiver
}

// New creates a new lightweight tapd client.
//
// This is the main entry point for embedding the lightweight wallet in Go applications.
func New(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config required")
	}

	// Determine network parameters
	var netParams *chaincfg.Params
	switch cfg.Network {
	case "mainnet":
		netParams = &chaincfg.MainNetParams
	case "testnet":
		netParams = &chaincfg.TestNet3Params
	case "regtest":
		netParams = &chaincfg.RegressionNetParams
	default:
		netParams = &chaincfg.TestNet3Params
	}

	// Task 01: Initialize chain backend
	mempoolCfg := mempool.DefaultConfig()
	if cfg.MempoolURL != "" {
		mempoolCfg.BaseURL = cfg.MempoolURL
	}
	mempoolClient := mempool.NewClient(mempoolCfg)
	chainBridgeCfg := mempool.DefaultChainBridgeConfig(mempoolClient)
	chainBridge := mempool.NewChainBridge(chainBridgeCfg)

	// Task 02: Initialize wallet
	walletCfg := btcwallet.DefaultConfig(chainBridge)
	walletCfg.DBPath = cfg.DBPath + ".wallet"
	walletCfg.Seed = cfg.Seed
	walletCfg.NetParams = netParams
	walletAnchor, err := btcwallet.New(walletCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	// Task 03: Initialize keyring
	keyRingCfg := keyring.DefaultConfig(cfg.Seed, netParams)
	keyRing, err := keyring.New(keyRingCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	// Task 04: Initialize database
	dbStore, err := db.InitDatabaseFromPath(cfg.DBPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to init database: %w", err)
	}
	sqliteStore, ok := dbStore.(*tapdb.SqliteStore)
	if !ok {
		return nil, fmt.Errorf("expected SqliteStore")
	}

	stores, err := db.InitAllStores(sqliteStore)
	if err != nil {
		return nil, fmt.Errorf("failed to init stores: %w", err)
	}

	// Task 05: Initialize proof system
	proofCfg := &proofconfig.Config{
		ProofFileDir: cfg.ProofDir,
		ChainBridge:  chainBridge,
		AssetStore:   stores.AssetStore,
	}
	proofSystem, err := proofconfig.New(proofCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to init proof system: %w", err)
	}

	// Task 06: Initialize minting
	mintingCfg := &minting.Config{
		ChainBridge:  chainBridge,
		WalletAnchor: walletAnchor,
		KeyRing:      keyRing,
		MintingStore: stores.MintingStore,
		TreeStore:    stores.TreeStore,
		ProofFileDir: cfg.ProofDir,
	}
	minter, err := minting.New(mintingCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to init minter: %w", err)
	}

	// Task 07: Initialize sending
	sendingCfg := &sending.Config{
		ChainBridge:  chainBridge,
		WalletAnchor: walletAnchor,
		KeyRing:      keyRing,
		AssetStore:   stores.AssetStore,
	}
	sender, err := sending.New(sendingCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to init sender: %w", err)
	}

	// Task 08: Initialize receiving
	receivingCfg := &receiving.Config{
		ChainBridge:  chainBridge,
		WalletAnchor: walletAnchor,
		KeyRing:      keyRing,
		AddrBook:     stores.AddrBook,
		AssetStore:   stores.AssetStore,
	}
	receiver, err := receiving.New(receivingCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to init receiver: %w", err)
	}

	return &Client{
		cfg:          cfg,
		chainBridge:  chainBridge,
		walletAnchor: walletAnchor,
		keyRing:      keyRing,
		dbStore:      sqliteStore,
		stores:       stores,
		proofSystem:  proofSystem,
		minter:       minter,
		sender:       sender,
		receiver:     receiver,
	}, nil
}

// Start starts the client.
func (c *Client) Start() error {
	if err := c.chainBridge.Start(); err != nil {
		return fmt.Errorf("failed to start chain bridge: %w", err)
	}
	if err := c.walletAnchor.Start(); err != nil {
		return fmt.Errorf("failed to start wallet: %w", err)
	}
	if err := c.minter.Start(); err != nil {
		return fmt.Errorf("failed to start minter: %w", err)
	}
	return nil
}

// Stop stops the client.
func (c *Client) Stop() error {
	_ = c.minter.Stop()
	_ = c.walletAnchor.Stop()
	_ = c.chainBridge.Stop()
	if c.dbStore != nil {
		c.dbStore.DB.Close()
	}
	return nil
}

// ListAssets lists all assets in the wallet.
func (c *Client) ListAssets(ctx context.Context) ([]*asset.ChainAsset, error) {
	return c.stores.AssetStore.FetchAllAssets(ctx, false, false, nil)
}
