package proofconfig

import (
	"context"
	"testing"

	"github.com/lightninglabs/taproot-assets/lightweight-wallet/chain/mempool"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/db"
	"github.com/lightninglabs/taproot-assets/proof"
	"github.com/lightninglabs/taproot-assets/tapdb"
	"github.com/stretchr/testify/require"
)

// TestProofSystem_New tests creating a proof system.
func TestProofSystem_New(t *testing.T) {
	t.Parallel()

	// Create mock chain bridge
	chainBridge := &mempool.ChainBridge{}

	// Create test database and stores
	dbStore, err := db.InitMemoryDatabase()
	require.NoError(t, err)
	sqliteStore, ok := dbStore.(*tapdb.SqliteStore)
	require.True(t, ok)
	defer sqliteStore.DB.Close()

	stores, err := db.InitAllStores(sqliteStore)
	require.NoError(t, err)

	// Create proof system
	cfg := &Config{
		ProofFileDir: t.TempDir(),
		ChainBridge:  chainBridge,
		AssetStore:   stores.AssetStore,
	}

	ps, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, ps)
	require.NotNil(t, ps.Verifier)
	require.NotNil(t, ps.ChainBridge)
	require.NotNil(t, ps.AssetStore)
}

// TestProofSystem_InvalidConfig tests configuration validation.
func TestProofSystem_InvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "nil config",
			cfg:  nil,
		},
		{
			name: "missing chain bridge",
			cfg: &Config{
				AssetStore: &tapdb.AssetStore{},
			},
		},
		{
			name: "missing asset store",
			cfg: &Config{
				ChainBridge: &mempool.ChainBridge{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps, err := New(tt.cfg)
			require.Error(t, err)
			require.Nil(t, ps)
			require.ErrorIs(t, err, ErrInvalidConfig)
		})
	}
}

// TestProofSystem_VerifyProof tests proof verification setup.
func TestProofSystem_VerifyProof(t *testing.T) {
	// This is a minimal test to verify the API works
	// Full proof verification would require valid proof data

	// Create mock chain bridge
	chainBridge := &mempool.ChainBridge{}

	// Create test database
	dbStore, err := db.InitMemoryDatabase()
	require.NoError(t, err)
	sqliteStore, ok := dbStore.(*tapdb.SqliteStore)
	require.True(t, ok)
	defer sqliteStore.DB.Close()

	stores, err := db.InitAllStores(sqliteStore)
	require.NoError(t, err)

	// Create proof system
	cfg := &Config{
		ProofFileDir: t.TempDir(),
		ChainBridge:  chainBridge,
		AssetStore:   stores.AssetStore,
	}

	ps, err := New(cfg)
	require.NoError(t, err)

	// Try to verify empty proof (will fail, but verifies API works)
	ctx := context.Background()
	emptyProof := proof.Blob{}

	_, err = ps.VerifyProof(ctx, emptyProof)
	require.Error(t, err) // Empty proof should fail validation

	// The important thing is the API exists and compiles
}
