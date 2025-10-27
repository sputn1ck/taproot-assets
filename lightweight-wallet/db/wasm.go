//go:build wasm

package db

import (
	"fmt"

	"github.com/lightninglabs/taproot-assets/tapdb"
	"github.com/lightninglabs/taproot-assets/tapdb/sqlc"
)

// InitWASMDatabase initializes a database for WASM environments.
//
// WASM databases use in-memory SQLite since filesystem access is limited.
// For persistence, use browser storage APIs (IndexedDB, localStorage) to
// export/import the database.
//
// Usage:
//   db, err := db.InitWASMDatabase()
//   // Use database
//   // Before page unload, export database to IndexedDB
func InitWASMDatabase() (*tapdb.SqliteStore, error) {
	cfg := &Config{
		Backend:   sqlc.BackendTypeSqlite,
		UseMemory: true,
	}

	store, err := InitDatabase(cfg)
	if err != nil {
		return nil, err
	}

	sqliteStore, ok := store.(*tapdb.SqliteStore)
	if !ok {
		return nil, fmt.Errorf("expected SqliteStore")
	}

	return sqliteStore, nil
}

// ExportDatabase exports the in-memory database to bytes.
// This can be saved to IndexedDB or localStorage for persistence.
func ExportDatabase(store *tapdb.SqliteStore) ([]byte, error) {
	// TODO: Implement database export
	// Would use SQLite backup API to export to bytes
	return nil, fmt.Errorf("database export not yet implemented")
}

// ImportDatabase imports a database from bytes.
// Used to restore a previously exported database.
func ImportDatabase(data []byte) (*tapdb.SqliteStore, error) {
	// TODO: Implement database import
	// Would create in-memory DB and restore from bytes
	return nil, fmt.Errorf("database import not yet implemented")
}
