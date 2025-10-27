package db

import (
	"testing"

	"github.com/lightninglabs/taproot-assets/tapdb"
	"github.com/lightninglabs/taproot-assets/tapdb/sqlc"
	"github.com/stretchr/testify/require"
)

// TestInitDatabase_MemoryDB tests creating an in-memory database.
func TestInitDatabase_MemoryDB(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Backend:   sqlc.BackendTypeSqlite,
		UseMemory: true,
	}

	db, err := InitDatabase(cfg)
	require.NoError(t, err)
	require.NotNil(t, db)

	// Should be a SqliteStore
	sqliteStore, ok := db.(*tapdb.SqliteStore)
	require.True(t, ok, "should be SqliteStore")
	require.NotNil(t, sqliteStore)

	// Should have correct backend
	require.Equal(t, sqlc.BackendTypeSqlite, db.Backend())
}

// TestInitDatabase_FileDB tests creating a file-based database.
func TestInitDatabase_FileDB(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	cfg := &Config{
		Backend: sqlc.BackendTypeSqlite,
		DBPath:  dbPath,
	}

	db, err := InitDatabase(cfg)
	require.NoError(t, err)
	require.NotNil(t, db)

	// Clean up
	if sqliteStore, ok := db.(*tapdb.SqliteStore); ok {
		sqliteStore.DB.Close()
	}
}

// TestInitAllStores tests store initialization.
func TestInitAllStores(t *testing.T) {
	t.Parallel()

	// Create in-memory database
	cfg := &Config{
		Backend:   sqlc.BackendTypeSqlite,
		UseMemory: true,
	}

	db, err := InitDatabase(cfg)
	require.NoError(t, err)

	sqliteStore, ok := db.(*tapdb.SqliteStore)
	require.True(t, ok)

	// Initialize all stores
	stores, err := InitAllStores(sqliteStore)
	require.NoError(t, err)
	require.NotNil(t, stores)

	// Verify stores are created
	require.NotNil(t, stores.AssetStore)
	require.NotNil(t, stores.MintingStore)
	require.NotNil(t, stores.AddrBook)
	require.NotNil(t, stores.TreeStore)
	require.NotNil(t, stores.RootKeyStore)
	require.NotNil(t, stores.BaseDB)

	// Clean up
	sqliteStore.DB.Close()
}

// TestInitDatabaseFromPath tests convenience function.
func TestInitDatabaseFromPath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := InitDatabaseFromPath(dbPath, false)
	require.NoError(t, err)
	require.NotNil(t, db)

	// Clean up
	if sqliteStore, ok := db.(*tapdb.SqliteStore); ok {
		sqliteStore.DB.Close()
	}
}

// TestInitMemoryDatabase tests memory database helper.
func TestInitMemoryDatabase(t *testing.T) {
	// Don't run in parallel since in-memory DBs can have issues with migrations

	db, err := InitMemoryDatabase()
	require.NoError(t, err)
	require.NotNil(t, db)

	// Should work as BatchedQuerier
	require.Equal(t, sqlc.BackendTypeSqlite, db.Backend())

	// Clean up
	if sqliteStore, ok := db.(*tapdb.SqliteStore); ok {
		sqliteStore.DB.Close()
	}
}
