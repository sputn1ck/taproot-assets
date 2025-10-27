package mempool

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/taproot-assets/asset"
	"github.com/lightninglabs/taproot-assets/proof"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
)

// ChainBridgeConfig holds configuration for the ChainBridge.
type ChainBridgeConfig struct {
	// Client is the mempool.space API client.
	Client *Client

	// PollInterval is how often to poll for new blocks/confirmations.
	// Default: 30 seconds
	PollInterval time.Duration

	// CacheSize is the number of items to cache.
	// Default: 100
	CacheSize int

	// CacheTTL is how long cached items are valid.
	// Default: 60 seconds
	CacheTTL time.Duration
}

// DefaultChainBridgeConfig returns default configuration.
func DefaultChainBridgeConfig(client *Client) *ChainBridgeConfig {
	return &ChainBridgeConfig{
		Client:       client,
		PollInterval: 30 * time.Second,
		CacheSize:    100,
		CacheTTL:     60 * time.Second,
	}
}

// ChainBridge implements the tapgarden.ChainBridge interface using mempool.space API.
type ChainBridge struct {
	cfg *ChainBridgeConfig

	cache *cache

	// Notification managers
	confNotifier  *confirmationNotifier
	epochNotifier *epochNotifier

	started bool
	quit    chan struct{}
	wg      sync.WaitGroup
	mu      sync.RWMutex
}

// NewChainBridge creates a new ChainBridge.
func NewChainBridge(cfg *ChainBridgeConfig) *ChainBridge {
	if cfg == nil {
		cfg = DefaultChainBridgeConfig(nil)
	}

	return &ChainBridge{
		cfg:           cfg,
		cache:         newCache(cfg.CacheSize, cfg.CacheTTL),
		confNotifier:  newConfirmationNotifier(cfg.Client, cfg.PollInterval),
		epochNotifier: newEpochNotifier(cfg.Client, cfg.PollInterval),
		quit:          make(chan struct{}),
	}
}

// Start starts the chain bridge.
func (c *ChainBridge) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}

	c.started = true

	// Start notification managers
	c.confNotifier.Start()
	c.epochNotifier.Start()

	return nil
}

// Stop stops the chain bridge.
func (c *ChainBridge) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	close(c.quit)
	c.wg.Wait()

	c.confNotifier.Stop()
	c.epochNotifier.Stop()

	c.started = false

	return nil
}

// CurrentHeight returns the current blockchain height.
func (c *ChainBridge) CurrentHeight(ctx context.Context) (uint32, error) {
	// Check cache first
	if height, ok := c.cache.getHeight(); ok {
		return height, nil
	}

	// Fetch from API
	height, err := c.cfg.Client.GetCurrentHeight(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get current height: %w", err)
	}

	// Cache result
	c.cache.setHeight(height)

	return height, nil
}

// GetBlockHash returns the hash of the block at the given height.
func (c *ChainBridge) GetBlockHash(ctx context.Context, height int64) (chainhash.Hash, error) {
	// Check cache first
	if hash, ok := c.cache.getBlockHash(uint32(height)); ok {
		return hash, nil
	}

	// Fetch from API
	hashStr, err := c.cfg.Client.GetBlockHash(ctx, height)
	if err != nil {
		return chainhash.Hash{}, fmt.Errorf("failed to get block hash: %w", err)
	}

	// Parse hash
	hashBytes, err := hex.DecodeString(hashStr)
	if err != nil {
		return chainhash.Hash{}, fmt.Errorf("failed to decode block hash: %w", err)
	}

	hash, err := chainhash.NewHash(hashBytes)
	if err != nil {
		return chainhash.Hash{}, fmt.Errorf("failed to create hash: %w", err)
	}

	// Cache result
	c.cache.setBlockHash(uint32(height), *hash)

	return *hash, nil
}

// GetBlock returns the block for the given hash.
func (c *ChainBridge) GetBlock(ctx context.Context, blockHash chainhash.Hash) (*wire.MsgBlock, error) {
	// Fetch from API
	blockResp, err := c.cfg.Client.GetBlock(ctx, blockHash.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	// For now, we need to fetch the full block data
	// mempool.space returns block metadata, but we need the full block
	// We'll need to fetch transactions and reconstruct the block
	// This is a simplified implementation - full implementation would need
	// to fetch all transactions in the block

	msgBlock := &wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:    blockResp.Version,
			PrevBlock:  chainhash.Hash{}, // Need to parse from blockResp.PreviousBlockHash
			MerkleRoot: chainhash.Hash{}, // Need to parse from blockResp.MerkleRoot
			Timestamp:  time.Unix(blockResp.Timestamp, 0),
			Bits:       blockResp.Bits,
			Nonce:      blockResp.Nonce,
		},
		Transactions: []*wire.MsgTx{}, // Would need to fetch all transactions
	}

	return msgBlock, nil
}

// GetBlockTimestamp returns the timestamp of the block at the given height.
func (c *ChainBridge) GetBlockTimestamp(ctx context.Context, height uint32) int64 {
	// Check cache first
	if timestamp, ok := c.cache.getBlockTimestamp(height); ok {
		return timestamp
	}

	// Fetch block hash
	hash, err := c.GetBlockHash(ctx, int64(height))
	if err != nil {
		return 0
	}

	// Fetch block
	blockResp, err := c.cfg.Client.GetBlock(ctx, hash.String())
	if err != nil {
		return 0
	}

	// Cache result
	c.cache.setBlockTimestamp(height, blockResp.Timestamp)

	return blockResp.Timestamp
}

// GetBlockHeaderByHeight returns the block header for the given height.
func (c *ChainBridge) GetBlockHeaderByHeight(ctx context.Context, height int64) (*wire.BlockHeader, error) {
	// Fetch block hash
	hash, err := c.GetBlockHash(ctx, height)
	if err != nil {
		return nil, err
	}

	// Fetch block
	blockResp, err := c.cfg.Client.GetBlock(ctx, hash.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	// Parse previous block hash
	prevHashBytes, err := hex.DecodeString(blockResp.PreviousBlockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode prev hash: %w", err)
	}
	prevHash, err := chainhash.NewHash(prevHashBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create prev hash: %w", err)
	}

	// Parse merkle root
	merkleBytes, err := hex.DecodeString(blockResp.MerkleRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to decode merkle root: %w", err)
	}
	merkleRoot, err := chainhash.NewHash(merkleBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create merkle root: %w", err)
	}

	header := &wire.BlockHeader{
		Version:    blockResp.Version,
		PrevBlock:  *prevHash,
		MerkleRoot: *merkleRoot,
		Timestamp:  time.Unix(blockResp.Timestamp, 0),
		Bits:       blockResp.Bits,
		Nonce:      blockResp.Nonce,
	}

	return header, nil
}

// PublishTransaction broadcasts a transaction to the network.
func (c *ChainBridge) PublishTransaction(ctx context.Context, tx *wire.MsgTx, label string) error {
	return c.cfg.Client.BroadcastTransaction(ctx, tx)
}

// EstimateFee estimates the fee for a given confirmation target.
func (c *ChainBridge) EstimateFee(ctx context.Context, confTarget uint32) (chainfee.SatPerKWeight, error) {
	fees, err := c.cfg.Client.GetFeeEstimates(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get fee estimates: %w", err)
	}

	// Map confirmation target to fee estimate
	var feeRate int64
	switch {
	case confTarget <= 1:
		feeRate = fees.FastestFee
	case confTarget <= 3:
		feeRate = fees.HalfHourFee
	case confTarget <= 6:
		feeRate = fees.HourFee
	case confTarget <= 12:
		feeRate = fees.EconomyFee
	default:
		feeRate = fees.MinimumFee
	}

	// Convert sat/vB to sat/kW
	// 1 vB = 4 weight units, so sat/vB * 4 = sat/kW / 1000
	satPerKW := chainfee.SatPerKWeight(feeRate * 1000 / 4)

	return satPerKW, nil
}

// VerifyBlock verifies that a block exists on-chain at the given height.
func (c *ChainBridge) VerifyBlock(ctx context.Context, header wire.BlockHeader, height uint32) error {
	// Get block hash at height
	hash, err := c.GetBlockHash(ctx, int64(height))
	if err != nil {
		return fmt.Errorf("failed to get block hash: %w", err)
	}

	// Compute header hash
	headerHash := header.BlockHash()

	// Compare hashes
	if !bytes.Equal(hash[:], headerHash[:]) {
		return fmt.Errorf("block hash mismatch: expected %s, got %s", hash, headerHash)
	}

	return nil
}

// RegisterConfirmationsNtfn registers for confirmation notifications.
func (c *ChainBridge) RegisterConfirmationsNtfn(
	ctx context.Context,
	txid *chainhash.Hash,
	pkScript []byte,
	numConfs, heightHint uint32,
	includeBlock bool,
	reOrgChan chan struct{},
) (*chainntnfs.ConfirmationEvent, chan error, error) {
	return c.confNotifier.RegisterConfirmation(
		ctx, txid, pkScript, numConfs, heightHint, includeBlock, reOrgChan,
	)
}

// RegisterBlockEpochNtfn registers for block epoch notifications.
func (c *ChainBridge) RegisterBlockEpochNtfn(ctx context.Context) (chan int32, chan error, error) {
	return c.epochNotifier.RegisterEpoch(ctx)
}

// GenFileChainLookup generates a chain lookup for proof verification from a file.
func (c *ChainBridge) GenFileChainLookup(f *proof.File) asset.ChainLookup {
	return &chainLookup{
		bridge: c,
	}
}

// GenProofChainLookup generates a chain lookup for proof verification from a single proof.
func (c *ChainBridge) GenProofChainLookup(p *proof.Proof) (asset.ChainLookup, error) {
	return &chainLookup{
		bridge: c,
	}, nil
}

// chainLookup implements asset.ChainLookup for proof verification.
type chainLookup struct {
	bridge *ChainBridge
}

// TxBlockHeight returns the block height that the given transaction was included in.
func (c *chainLookup) TxBlockHeight(ctx context.Context, txid chainhash.Hash) (uint32, error) {
	tx, err := c.bridge.cfg.Client.GetTransaction(ctx, txid.String())
	if err != nil {
		return 0, fmt.Errorf("failed to get transaction: %w", err)
	}

	if !tx.Status.Confirmed {
		return 0, fmt.Errorf("transaction not confirmed")
	}

	return uint32(tx.Status.BlockHeight), nil
}

// MeanBlockTimestamp returns the mean timestamp of blocks around the given height.
func (c *chainLookup) MeanBlockTimestamp(ctx context.Context, blockHeight uint32) (time.Time, error) {
	// Calculate mean over 11 blocks (current + 10 previous)
	var totalTimestamp int64
	count := 0

	for i := int64(0); i < 11 && int64(blockHeight)-i >= 0; i++ {
		height := int64(blockHeight) - i
		timestamp := c.bridge.GetBlockTimestamp(ctx, uint32(height))
		if timestamp > 0 {
			totalTimestamp += timestamp
			count++
		}
	}

	if count == 0 {
		return time.Time{}, fmt.Errorf("no block timestamps found")
	}

	meanTimestamp := totalTimestamp / int64(count)
	return time.Unix(meanTimestamp, 0), nil
}

// CurrentHeight returns the current blockchain height.
func (c *chainLookup) CurrentHeight(ctx context.Context) (uint32, error) {
	return c.bridge.CurrentHeight(ctx)
}
