package db

import (
	"database/sql"
	"fmt"

	"github.com/lightninglabs/taproot-assets/tapdb"
	"github.com/lightningnetwork/lnd/clock"
)

// Stores holds references to commonly used tapdb stores.
type Stores struct {
	// Core asset storage
	AssetStore *tapdb.AssetStore

	// Asset minting
	MintingStore *tapdb.AssetMintingStore

	// Address book
	AddrBook *tapdb.TapAddressBook

	// Tree stores
	TreeStore *tapdb.TaprootAssetTreeStore

	// Macaroons
	RootKeyStore *tapdb.RootKeyStore

	// Base DB (SqliteStore or PostgresStore)
	BaseDB tapdb.BatchedQuerier
}

// InitAllStores initializes commonly used tapdb stores from a SqliteStore.
//
// Note: This function expects a *tapdb.SqliteStore (or *tapdb.PostgresStore)
// which provides the WithTx method needed for transaction executors.
//
// For mobile/external DB, use tapdb.NewSqliteStore() to create the store first.
func InitAllStores(sqliteStore *tapdb.SqliteStore) (*Stores, error) {
	if sqliteStore == nil {
		return nil, fmt.Errorf("database is required")
	}

	// Get backend type
	dbType := sqliteStore.Backend()

	// Create clock
	clk := clock.NewDefaultClock()

	// Initialize asset store
	assetDB := tapdb.NewTransactionExecutor(
		sqliteStore, func(tx *sql.Tx) tapdb.ActiveAssetsStore {
			return sqliteStore.WithTx(tx)
		},
	)

	metaDB := tapdb.NewTransactionExecutor(
		sqliteStore, func(tx *sql.Tx) tapdb.MetaStore {
			return sqliteStore.WithTx(tx)
		},
	)

	assetStore := tapdb.NewAssetStore(assetDB, metaDB, clk, dbType)

	// Initialize minting store
	mintingDB := tapdb.NewTransactionExecutor(
		sqliteStore, func(tx *sql.Tx) tapdb.PendingAssetStore {
			return sqliteStore.WithTx(tx)
		},
	)
	mintingStore := tapdb.NewAssetMintingStore(mintingDB)

	// Initialize address book
	addrDB := tapdb.NewTransactionExecutor(
		sqliteStore, func(tx *sql.Tx) tapdb.AddrBook {
			return sqliteStore.WithTx(tx)
		},
	)
	addrBook := tapdb.NewTapAddressBook(addrDB, nil, clk)

	// Initialize tree store
	treeDB := tapdb.NewTransactionExecutor(
		sqliteStore, func(tx *sql.Tx) tapdb.TreeStore {
			return sqliteStore.WithTx(tx)
		},
	)
	// Namespace for the tree (empty string for default)
	treeStore := tapdb.NewTaprootAssetTreeStore(treeDB, "")

	// Initialize root key store
	keyDB := tapdb.NewTransactionExecutor(
		sqliteStore, func(tx *sql.Tx) tapdb.KeyStore {
			return sqliteStore.WithTx(tx)
		},
	)
	rootKeyStore := tapdb.NewRootKeyStore(keyDB)

	return &Stores{
		AssetStore:   assetStore,
		MintingStore: mintingStore,
		AddrBook:     addrBook,
		TreeStore:    treeStore,
		RootKeyStore: rootKeyStore,
		BaseDB:       sqliteStore,
	}, nil
}
