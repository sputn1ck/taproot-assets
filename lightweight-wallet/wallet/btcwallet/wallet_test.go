package btcwallet

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/chain/mempool"
	"github.com/lightninglabs/taproot-assets/tapfreighter"
	"github.com/lightninglabs/taproot-assets/tapgarden"
	"github.com/stretchr/testify/require"
)

// TestWalletAnchor_InterfaceCompliance verifies interface compliance.
func TestWalletAnchor_InterfaceCompliance(t *testing.T) {
	t.Parallel()

	// Verify WalletAnchor implements tapgarden.WalletAnchor
	var _ tapgarden.WalletAnchor = (*WalletAnchor)(nil)

	// Verify WalletAnchor implements tapfreighter.WalletAnchor
	var _ tapfreighter.WalletAnchor = (*WalletAnchor)(nil)
}

// TestUTXOLockManager tests the UTXO lock manager.
func TestUTXOLockManager(t *testing.T) {
	t.Parallel()

	lockMgr := newUTXOLockManager()
	require.NotNil(t, lockMgr)

	// Create test outpoint
	outpoint := wire.OutPoint{
		Hash:  chainhash.Hash{0x01},
		Index: 0,
	}

	// Should not be locked initially
	require.False(t, lockMgr.IsLocked(outpoint))

	// Lock UTXO
	err := lockMgr.LockUTXO(outpoint, 1*time.Minute)
	require.NoError(t, err)

	// Should be locked now
	require.True(t, lockMgr.IsLocked(outpoint))

	// Try to lock again - should fail
	err = lockMgr.LockUTXO(outpoint, 1*time.Minute)
	require.ErrorIs(t, err, ErrUTXOLocked)

	// Unlock UTXO
	err = lockMgr.UnlockUTXO(outpoint)
	require.NoError(t, err)

	// Should not be locked anymore
	require.False(t, lockMgr.IsLocked(outpoint))

	// Unlock again - should fail
	err = lockMgr.UnlockUTXO(outpoint)
	require.ErrorIs(t, err, ErrUTXONotLocked)
}

// TestUTXOLockManager_Expiry tests UTXO lock expiration.
func TestUTXOLockManager_Expiry(t *testing.T) {
	t.Parallel()

	lockMgr := newUTXOLockManager()

	outpoint := wire.OutPoint{
		Hash:  chainhash.Hash{0x02},
		Index: 0,
	}

	// Lock for very short duration
	err := lockMgr.LockUTXO(outpoint, 100*time.Millisecond)
	require.NoError(t, err)
	require.True(t, lockMgr.IsLocked(outpoint))

	// Wait for lock to expire
	time.Sleep(200 * time.Millisecond)

	// Should not be locked anymore
	require.False(t, lockMgr.IsLocked(outpoint))

	// Should be able to lock again
	err = lockMgr.LockUTXO(outpoint, 1*time.Minute)
	require.NoError(t, err)
	require.True(t, lockMgr.IsLocked(outpoint))
}

// TestConfig_Validation tests configuration validation.
func TestConfig_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *Config
		wantErr error
	}{
		{
			name: "valid config",
			cfg: &Config{
				NetParams:    &chaincfg.TestNet3Params,
				ChainBridge:  &mempool.ChainBridge{},
				PrivatePass:  []byte("password"),
				PublicPass:   []byte("public"),
			},
			wantErr: nil,
		},
		{
			name: "missing net params",
			cfg: &Config{
				ChainBridge: &mempool.ChainBridge{},
				PrivatePass: []byte("password"),
			},
			wantErr: ErrInvalidNetParams,
		},
		{
			name: "missing chain bridge",
			cfg: &Config{
				NetParams:   &chaincfg.TestNet3Params,
				PrivatePass: []byte("password"),
			},
			wantErr: ErrChainBridgeRequired,
		},
		{
			name: "missing private pass",
			cfg: &Config{
				NetParams:   &chaincfg.TestNet3Params,
				ChainBridge: &mempool.ChainBridge{},
			},
			wantErr: ErrPrivatePassRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
