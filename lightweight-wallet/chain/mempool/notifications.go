package mempool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/chainntnfs"
)

// confirmationRequest represents a pending confirmation notification request.
type confirmationRequest struct {
	txid         *chainhash.Hash
	pkScript     []byte
	numConfs     uint32
	heightHint   uint32
	includeBlock bool
	reOrgChan    chan struct{}

	confChan chan *chainntnfs.TxConfirmation
	errChan  chan error

	cancel context.CancelFunc
}

// confirmationNotifier manages confirmation notifications via polling.
type confirmationNotifier struct {
	client       *Client
	pollInterval time.Duration

	requests map[chainhash.Hash]*confirmationRequest
	mu       sync.RWMutex

	quit chan struct{}
	wg   sync.WaitGroup
}

// newConfirmationNotifier creates a new confirmation notifier.
func newConfirmationNotifier(client *Client, pollInterval time.Duration) *confirmationNotifier {
	return &confirmationNotifier{
		client:       client,
		pollInterval: pollInterval,
		requests:     make(map[chainhash.Hash]*confirmationRequest),
		quit:         make(chan struct{}),
	}
}

// Start starts the confirmation notifier.
func (n *confirmationNotifier) Start() {
	n.wg.Add(1)
	go n.pollLoop()
}

// Stop stops the confirmation notifier.
func (n *confirmationNotifier) Stop() {
	close(n.quit)
	n.wg.Wait()

	// Cancel all pending requests
	n.mu.Lock()
	for _, req := range n.requests {
		req.cancel()
	}
	n.requests = make(map[chainhash.Hash]*confirmationRequest)
	n.mu.Unlock()
}

// RegisterConfirmation registers a confirmation notification.
func (n *confirmationNotifier) RegisterConfirmation(
	ctx context.Context,
	txid *chainhash.Hash,
	pkScript []byte,
	numConfs, heightHint uint32,
	includeBlock bool,
	reOrgChan chan struct{},
) (*chainntnfs.ConfirmationEvent, chan error, error) {
	// Create channels
	confChan := make(chan *chainntnfs.TxConfirmation, 1)
	errChan := make(chan error, 1)

	// Create cancellable context
	reqCtx, cancel := context.WithCancel(ctx)

	// Create request
	req := &confirmationRequest{
		txid:         txid,
		pkScript:     pkScript,
		numConfs:     numConfs,
		heightHint:   heightHint,
		includeBlock: includeBlock,
		reOrgChan:    reOrgChan,
		confChan:     confChan,
		errChan:      errChan,
		cancel:       cancel,
	}

	// Register request
	n.mu.Lock()
	n.requests[*txid] = req
	n.mu.Unlock()

	// Start monitoring in goroutine
	n.wg.Add(1)
	go n.monitorConfirmation(reqCtx, req)

	// Create confirmation event
	confEvent := &chainntnfs.ConfirmationEvent{
		Confirmed: confChan,
		// Note: LND's chainntnfs also has Updates and NegativeConf channels
		// We're simplifying here
	}

	return confEvent, errChan, nil
}

// monitorConfirmation monitors a transaction for confirmations.
func (n *confirmationNotifier) monitorConfirmation(ctx context.Context, req *confirmationRequest) {
	defer n.wg.Done()

	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	var lastBlockHeight int64
	var txBlockHeight int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-n.quit:
			return
		case <-ticker.C:
			// Fetch transaction status
			tx, err := n.client.GetTransaction(ctx, req.txid.String())
			if err != nil {
				// Transaction not found yet, keep polling
				continue
			}

			// Check if confirmed
			if !tx.Status.Confirmed {
				continue
			}

			// Store block height on first confirmation
			if txBlockHeight == 0 {
				txBlockHeight = tx.Status.BlockHeight
			}

			// Get current height
			currentHeight, err := n.client.GetCurrentHeight(ctx)
			if err != nil {
				continue
			}

			// Calculate confirmations
			confs := uint32(int64(currentHeight) - txBlockHeight + 1)

			// Check for reorg
			if lastBlockHeight > 0 && txBlockHeight != lastBlockHeight {
				// Potential reorg detected
				if req.reOrgChan != nil {
					select {
					case req.reOrgChan <- struct{}{}:
					default:
					}
				}
			}
			lastBlockHeight = txBlockHeight

			// Check if we have enough confirmations
			if confs >= req.numConfs {
				// TODO: Fetch full block if requested (req.includeBlock)
				// For now, we don't include the full block

				// Send confirmation
				confirmation := &chainntnfs.TxConfirmation{
					BlockHeight: uint32(txBlockHeight),
					BlockHash:   nil, // Would need to parse
					TxIndex:     0,   // Would need to get from block
					Tx:          nil, // Would need to reconstruct
					Block:       nil, // Would need to fetch full block
				}

				select {
				case req.confChan <- confirmation:
				case <-ctx.Done():
					return
				case <-n.quit:
					return
				}

				// Cleanup request
				n.mu.Lock()
				delete(n.requests, *req.txid)
				n.mu.Unlock()

				return
			}
		}
	}
}

// pollLoop polls for updates to all registered confirmations.
func (n *confirmationNotifier) pollLoop() {
	defer n.wg.Done()

	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.quit:
			return
		case <-ticker.C:
			// Polling is handled per-request in monitorConfirmation
			// This loop could be used for cleanup or optimization
		}
	}
}

// epochNotifier manages block epoch notifications via polling.
type epochNotifier struct {
	client       *Client
	pollInterval time.Duration

	subscribers []epochSubscriber
	mu          sync.RWMutex

	lastHeight uint32

	quit chan struct{}
	wg   sync.WaitGroup
}

// epochSubscriber represents a block epoch subscriber.
type epochSubscriber struct {
	blockChan chan int32
	errChan   chan error
	cancel    context.CancelFunc
}

// newEpochNotifier creates a new epoch notifier.
func newEpochNotifier(client *Client, pollInterval time.Duration) *epochNotifier {
	return &epochNotifier{
		client:       client,
		pollInterval: pollInterval,
		subscribers:  make([]epochSubscriber, 0),
		quit:         make(chan struct{}),
	}
}

// Start starts the epoch notifier.
func (n *epochNotifier) Start() {
	n.wg.Add(1)
	go n.pollLoop()
}

// Stop stops the epoch notifier.
func (n *epochNotifier) Stop() {
	close(n.quit)
	n.wg.Wait()

	// Cancel all subscribers
	n.mu.Lock()
	for _, sub := range n.subscribers {
		sub.cancel()
	}
	n.subscribers = nil
	n.mu.Unlock()
}

// RegisterEpoch registers for block epoch notifications.
func (n *epochNotifier) RegisterEpoch(ctx context.Context) (chan int32, chan error, error) {
	blockChan := make(chan int32, 10)
	errChan := make(chan error, 1)

	_, cancel := context.WithCancel(ctx)

	subscriber := epochSubscriber{
		blockChan: blockChan,
		errChan:   errChan,
		cancel:    cancel,
	}

	n.mu.Lock()
	n.subscribers = append(n.subscribers, subscriber)
	n.mu.Unlock()

	return blockChan, errChan, nil
}

// pollLoop polls for new blocks.
func (n *epochNotifier) pollLoop() {
	defer n.wg.Done()

	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.quit:
			return
		case <-ticker.C:
			// Fetch current height
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			height, err := n.client.GetCurrentHeight(ctx)
			cancel()

			if err != nil {
				// Send error to all subscribers
				n.mu.RLock()
				for _, sub := range n.subscribers {
					select {
					case sub.errChan <- fmt.Errorf("failed to get height: %w", err):
					default:
					}
				}
				n.mu.RUnlock()
				continue
			}

			// Check if height changed
			if height > n.lastHeight {
				// Notify all subscribers of new height
				n.mu.RLock()
				for _, sub := range n.subscribers {
					select {
					case sub.blockChan <- int32(height):
					default:
						// Channel full, skip
					}
				}
				n.mu.RUnlock()

				n.lastHeight = height
			}
		}
	}
}
