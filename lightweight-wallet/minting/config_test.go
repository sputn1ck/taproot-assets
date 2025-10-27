package minting

import (
	"testing"

	"github.com/lightninglabs/taproot-assets/lightweight-wallet/chain/mempool"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/db"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/keyring"
	"github.com/lightninglabs/taproot-assets/lightweight-wallet/wallet/btcwallet"
	"github.com/lightninglabs/taproot-assets/tapdb"
	"github.com/stretchr/testify/require"
	"github.com/btcsuite/btcd/chaincfg"
)

// TestMinter_New tests minter creation.
func TestMinter_New(t *testing.T) {
	t.Parallel()

	// Create mock components
	chainBridge := &mempool.ChainBridge{}

	seed := make([]byte, 32)
	kr, err := keyring.New(keyring.DefaultConfig(seed, &chaincfg.TestNet3Params))
	require.NoError(t, err)

	walletCfg := btcwallet.DefaultConfig(chainBridge)
	walletCfg.DBPath = t.TempDir() + "/wallet.db"
	walletCfg.Seed = seed
	wallet, err := btcwallet.New(walletCfg)
	require.NoError(t, err)

	// Create database and stores
	dbStore, err := db.InitMemoryDatabase()
	require.NoError(t, err)
	sqliteStore, ok := dbStore.(*tapdb.SqliteStore)
	require.True(t, ok)
	defer sqliteStore.DB.Close()

	stores, err := db.InitAllStores(sqliteStore)
	require.NoError(t, err)

	// Create minter config
	cfg := &Config{
		ChainBridge:  chainBridge,
		WalletAnchor: wallet,
		KeyRing:      kr,
		MintingStore: stores.MintingStore,
		TreeStore:    stores.TreeStore,
		ProofFileDir: t.TempDir(),
	}

	// Create minter
	minter, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, minter)
}

// TestMinter_InvalidConfig tests configuration validation.
func TestMinter_InvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "missing chain bridge",
			cfg: &Config{
				WalletAnchor: &btcwallet.WalletAnchor{},
				KeyRing:      &keyring.KeyRing{},
				MintingStore: &tapdb.AssetMintingStore{},
				TreeStore:    &tapdb.TaprootAssetTreeStore{},
			},
		},
		{
			name: "missing wallet",
			cfg: &Config{
				ChainBridge:  &mempool.ChainBridge{},
				KeyRing:      &keyring.KeyRing{},
				MintingStore: &tapdb.AssetMintingStore{},
				TreeStore:    &tapdb.TaprootAssetTreeStore{},
			},
		},
		{
			name: "missing keyring",
			cfg: &Config{
				ChainBridge:  &mempool.ChainBridge{},
				WalletAnchor: &btcwallet.WalletAnchor{},
				MintingStore: &tapdb.AssetMintingStore{},
				TreeStore:    &tapdb.TaprootAssetTreeStore{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minter, err := New(tt.cfg)
			require.Error(t, err)
			require.Nil(t, minter)
		})
	}
}
