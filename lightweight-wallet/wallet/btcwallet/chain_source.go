package btcwallet

import (
	"fmt"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/chain/mempool"
)

// chainSource adapts our mempool.ChainBridge to btcwallet's chain.Interface.
type chainSource struct {
	bridge *mempool.ChainBridge
}

// newChainSource creates a new chain source adapter.
func newChainSource(bridge *mempool.ChainBridge) chain.Interface {
	return &chainSource{
		bridge: bridge,
	}
}

// Start starts the chain source.
func (c *chainSource) Start() error {
	return c.bridge.Start()
}

// Stop stops the chain source.
func (c *chainSource) Stop() {
	c.bridge.Stop()
}

// WaitForShutdown waits for the chain source to shut down.
func (c *chainSource) WaitForShutdown() {
	// mempool.ChainBridge doesn't have separate shutdown wait
}

// GetBestBlock returns the current best block hash and height.
func (c *chainSource) GetBestBlock() (*chainhash.Hash, int32, error) {
	ctx := contextWithTimeout()
	defer ctx.cancel()

	height, err := c.bridge.CurrentHeight(ctx.Context)
	if err != nil {
		return nil, 0, err
	}

	hash, err := c.bridge.GetBlockHash(ctx.Context, int64(height))
	if err != nil {
		return nil, 0, err
	}

	return &hash, int32(height), nil
}

// GetBlock returns a block given its hash.
func (c *chainSource) GetBlock(hash *chainhash.Hash) (*wire.MsgBlock, error) {
	ctx := contextWithTimeout()
	defer ctx.cancel()

	return c.bridge.GetBlock(ctx.Context, *hash)
}

// GetBlockHash returns the hash of a block at the given height.
func (c *chainSource) GetBlockHash(height int64) (*chainhash.Hash, error) {
	ctx := contextWithTimeout()
	defer ctx.cancel()

	hash, err := c.bridge.GetBlockHash(ctx.Context, height)
	if err != nil {
		return nil, err
	}

	return &hash, nil
}

// GetBlockHeader returns the header of a block given its hash.
func (c *chainSource) GetBlockHeader(hash *chainhash.Hash) (*wire.BlockHeader, error) {
	ctx := contextWithTimeout()
	defer ctx.cancel()

	// Get block and extract header
	block, err := c.bridge.GetBlock(ctx.Context, *hash)
	if err != nil {
		return nil, err
	}

	return &block.Header, nil
}

// IsCurrent returns whether the chain source is synced.
func (c *chainSource) IsCurrent() bool {
	// Always consider ourselves current since we use mempool.space
	return true
}

// FilterBlocks filters blocks for relevant transactions.
func (c *chainSource) FilterBlocks(req *chain.FilterBlocksRequest) (*chain.FilterBlocksResponse, error) {
	// Simplified implementation - would need to fetch blocks and filter
	return &chain.FilterBlocksResponse{}, fmt.Errorf("FilterBlocks not implemented")
}

// BlockStamp returns the current block stamp.
func (c *chainSource) BlockStamp() (*waddrmgr.BlockStamp, error) {
	hash, height, err := c.GetBestBlock()
	if err != nil {
		return nil, err
	}

	return &waddrmgr.BlockStamp{
		Height: height,
		Hash:   *hash,
	}, nil
}

// SendRawTransaction broadcasts a raw transaction.
func (c *chainSource) SendRawTransaction(tx *wire.MsgTx, allowHighFees bool) (*chainhash.Hash, error) {
	ctx := contextWithTimeout()
	defer ctx.cancel()

	err := c.bridge.PublishTransaction(ctx.Context, tx, "")
	if err != nil {
		return nil, err
	}

	txHash := tx.TxHash()
	return &txHash, nil
}

// Rescan initiates a blockchain rescan.
func (c *chainSource) Rescan(startHash *chainhash.Hash, addrs []btcutil.Address, outPoints map[wire.OutPoint]btcutil.Address) error {
	// Simplified implementation - mempool.space doesn't support rescan
	// Would need to implement scanning using GetBlock for each height
	return fmt.Errorf("rescan not implemented for mempool.space backend")
}

// NotifyReceived registers addresses to watch for received transactions.
func (c *chainSource) NotifyReceived(addrs []btcutil.Address) error {
	// No-op for mempool.space - we poll for all transactions
	return nil
}

// NotifyBlocks registers for block notifications.
func (c *chainSource) NotifyBlocks() error {
	// Already handled by mempool bridge
	return nil
}

// Notifications returns the notification channel.
func (c *chainSource) Notifications() <-chan interface{} {
	// Return empty channel - we handle notifications differently
	ch := make(chan interface{})
	close(ch)
	return ch
}

// BackEnd returns the name of the backend.
func (c *chainSource) BackEnd() string {
	return "mempool.space"
}

// TestMempoolAccept tests mempool acceptance.
func (c *chainSource) TestMempoolAccept(txns []*wire.MsgTx, maxFeeRate float64) ([]*btcjson.TestMempoolAcceptResult, error) {
	// Not supported by mempool.space
	return nil, fmt.Errorf("TestMempoolAccept not supported")
}

// MapRPCErr maps backend errors to standardized errors.
func (c *chainSource) MapRPCErr(err error) error {
	return err
}
