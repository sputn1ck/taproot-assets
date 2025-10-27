package btcwallet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/taproot-assets/tapgarden"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"

	_ "github.com/btcsuite/btcwallet/walletdb/bdb" // Import bdb driver
)

// WalletAnchor implements the tapgarden.WalletAnchor interface using btcwallet.
type WalletAnchor struct {
	cfg *Config

	// btcwallet instance
	wallet *wallet.Wallet
	db     walletdb.DB
	loader *wallet.Loader

	// Chain source for wallet
	chainSource chain.Interface

	// UTXO lock manager
	utxoLocks *utxoLockManager

	// Transaction monitoring
	txSubscriptions map[string]chan lndclient.Transaction
	txSubMu         sync.RWMutex

	started bool
	quit    chan struct{}
	wg      sync.WaitGroup
	mu      sync.RWMutex
}

// New creates a new WalletAnchor.
func New(cfg *Config) (*WalletAnchor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	wa := &WalletAnchor{
		cfg:             cfg,
		utxoLocks:       newUTXOLockManager(),
		txSubscriptions: make(map[string]chan lndclient.Transaction),
		quit:            make(chan struct{}),
	}

	return wa, nil
}

// Start starts the wallet anchor.
func (w *WalletAnchor) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return nil
	}

	// Initialize or load wallet
	if err := w.initWallet(); err != nil {
		return fmt.Errorf("failed to initialize wallet: %w", err)
	}

	// Start wallet
	w.wallet.Start()

	// Start transaction monitor
	w.wg.Add(1)
	go w.txMonitor()

	w.started = true

	return nil
}

// Stop stops the wallet anchor.
func (w *WalletAnchor) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return nil
	}

	close(w.quit)
	w.wg.Wait()

	// Stop wallet
	w.wallet.Stop()
	w.wallet.WaitForShutdown()

	// Close database
	if w.db != nil {
		w.db.Close()
	}

	w.started = false

	return nil
}

// initWallet initializes or loads the wallet.
func (w *WalletAnchor) initWallet() error {
	var err error

	// Setup database path
	dbDir := filepath.Dir(w.cfg.DBPath)
	if dbDir != "" && dbDir != "." {
		if err := os.MkdirAll(dbDir, 0700); err != nil {
			return fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	// Create loader
	w.loader = wallet.NewLoader(
		w.cfg.NetParams,
		dbDir,
		true,                      // noFreelistSync
		250,                       // dbTimeout (blocks)
		w.cfg.RecoveryWindow,
	)

	// Check if wallet exists
	walletExists, err := w.loader.WalletExists()
	if err != nil {
		return fmt.Errorf("failed to check if wallet exists: %w", err)
	}

	if !walletExists {
		// Create new wallet
		if len(w.cfg.Seed) == 0 {
			return fmt.Errorf("seed required for new wallet")
		}

		// Create extended master key from seed
		masterKey, err := hdkeychain.NewMaster(w.cfg.Seed, w.cfg.NetParams)
		if err != nil {
			return fmt.Errorf("failed to create master key: %w", err)
		}

		w.wallet, err = w.loader.CreateNewWallet(
			w.cfg.PublicPass,
			w.cfg.PrivatePass,
			w.cfg.Seed,
			w.cfg.Birthday,
		)
		if err != nil {
			return fmt.Errorf("failed to create wallet: %w", err)
		}

		_ = masterKey // Created for validation
	} else {
		// Load existing wallet
		w.wallet, err = w.loader.OpenExistingWallet(w.cfg.PublicPass, false)
		if err != nil {
			return fmt.Errorf("failed to open wallet: %w", err)
		}
	}

	// Unlock wallet with private passphrase
	err = w.wallet.Unlock(w.cfg.PrivatePass, nil)
	if err != nil {
		return fmt.Errorf("failed to unlock wallet: %w", err)
	}

	// Set up chain source - using our mempool bridge as the chain backend
	w.chainSource = newChainSource(w.cfg.ChainBridge)
	w.wallet.SetChainSynced(true) // Mark as synced since we use mempool.space

	return nil
}

// txMonitor monitors wallet transactions.
func (w *WalletAnchor) txMonitor() {
	defer w.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var lastHeight int32

	for {
		select {
		case <-w.quit:
			return
		case <-ticker.C:
			// Poll for new transactions
			// Get current height
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			height, err := w.cfg.ChainBridge.CurrentHeight(ctx)
			cancel()

			if err != nil {
				continue
			}

			currentHeight := int32(height)
			if currentHeight > lastHeight {
				// New blocks, check for new transactions
				w.checkNewTransactions(lastHeight, currentHeight)
				lastHeight = currentHeight
			}
		}
	}
}

// checkNewTransactions checks for new transactions in the given block range.
func (w *WalletAnchor) checkNewTransactions(startHeight, endHeight int32) {
	// This is a simplified implementation
	// In a full implementation, we'd query the wallet for new transactions
	// and notify subscribers
}

// MinRelayFee returns the minimum relay fee.
func (w *WalletAnchor) MinRelayFee(ctx context.Context) (chainfee.SatPerKWeight, error) {
	// Query from chain bridge
	return w.cfg.ChainBridge.EstimateFee(ctx, 1000)
}

// Verify interface compliance at compile time.
var _ tapgarden.WalletAnchor = (*WalletAnchor)(nil)
