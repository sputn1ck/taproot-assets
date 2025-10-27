package db

import (
	"database/sql"
	"fmt"

	"github.com/lightninglabs/taproot-assets/tapdb"
	"github.com/lightninglabs/taproot-assets/tapdb/sqlc"

	_ "modernc.org/sqlite" // SQLite driver
)

// Config holds configuration for database initialization.
type Config struct {
	// Backend type (sqlite or postgres)
	Backend sqlc.BackendType

	// For standard apps: path to database file
	DBPath string

	// For mobile: path provided by mobile app
	MobileDBPath string

	// For WASM: use in-memory database
	UseMemory bool

	// For custom initialization: external DB handle
	// Note: If provided, migrations must be run separately
	ExternalDB *sql.DB

	// Skip running migrations (useful if migrations already applied)
	SkipMigrations bool
}

// DefaultConfig returns a default database configuration.
func DefaultConfig(dbPath string) *Config {
	return &Config{
		Backend: sqlc.BackendTypeSqlite,
		DBPath:  dbPath,
	}
}

// InitDatabase initializes a database and returns a store implementation.
//
// This uses tapdb's existing store implementations (SqliteStore or PostgresStore)
// which handle migrations and provide all necessary interfaces.
func InitDatabase(cfg *Config) (tapdb.BatchedQuerier, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	// If external DB provided, wrap it
	if cfg.ExternalDB != nil {
		return wrapExternalDB(cfg.ExternalDB)
	}

	// Determine database path
	dbPath := cfg.DBPath
	if cfg.MobileDBPath != "" {
		dbPath = cfg.MobileDBPath
	}
	if cfg.UseMemory {
		dbPath = ":memory:"
	}

	if dbPath == "" && !cfg.UseMemory {
		return nil, fmt.Errorf("database path required")
	}

	// Use tapdb's native store constructors which handle migrations
	switch cfg.Backend {
	case sqlc.BackendTypeSqlite:
		sqliteCfg := &tapdb.SqliteConfig{
			SkipMigrations:   cfg.SkipMigrations,
			DatabaseFileName: dbPath,
		}
		return tapdb.NewSqliteStore(sqliteCfg)

	case sqlc.BackendTypePostgres:
		// TODO: Add postgres support
		return nil, fmt.Errorf("postgres not yet supported in lightweight wallet")

	default:
		return nil, fmt.Errorf("unsupported backend: %v", cfg.Backend)
	}
}

// wrapExternalDB wraps an external *sql.DB for use with tapdb.
// Note: Migrations must be run on the external DB before calling this.
func wrapExternalDB(db *sql.DB) (tapdb.BatchedQuerier, error) {
	// Create BaseDB wrapper
	baseDB := &tapdb.BaseDB{
		DB:      db,
		Queries: sqlc.New(db),
	}

	return baseDB, nil
}

// InitDatabaseFromPath is a convenience function that creates a database from a file path.
func InitDatabaseFromPath(dbPath string, skipMigrations bool) (tapdb.BatchedQuerier, error) {
	cfg := &Config{
		Backend:        sqlc.BackendTypeSqlite,
		DBPath:         dbPath,
		SkipMigrations: skipMigrations,
	}

	return InitDatabase(cfg)
}

// InitMemoryDatabase creates an in-memory database (useful for testing).
func InitMemoryDatabase() (tapdb.BatchedQuerier, error) {
	cfg := &Config{
		Backend:   sqlc.BackendTypeSqlite,
		UseMemory: true,
	}

	return InitDatabase(cfg)
}
