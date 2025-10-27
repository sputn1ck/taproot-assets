//go:build !wasm

package db

import (
	"fmt"

	"github.com/lightninglabs/taproot-assets/tapdb"
)

// InitWASMDatabase is not available on non-WASM platforms.
func InitWASMDatabase() (*tapdb.SqliteStore, error) {
	return nil, fmt.Errorf("WASM database only available on wasm platform")
}

// ExportDatabase is not available on non-WASM platforms.
func ExportDatabase(store *tapdb.SqliteStore) ([]byte, error) {
	return nil, fmt.Errorf("database export only available on wasm platform")
}

// ImportDatabase is not available on non-WASM platforms.
func ImportDatabase(data []byte) (*tapdb.SqliteStore, error) {
	return nil, fmt.Errorf("database import only available on wasm platform")
}
