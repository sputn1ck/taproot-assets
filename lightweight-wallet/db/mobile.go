package db

import (
	"fmt"

	"github.com/lightninglabs/taproot-assets/tapdb"
)

// MobileConfig holds configuration for mobile database initialization.
type MobileConfig struct {
	// DBPath is the path to the database file provided by the mobile app.
	// On iOS: Usually in app's Documents directory
	// On Android: Usually in app's files directory
	DBPath string

	// SkipMigrations can be set if the database is pre-bundled with migrations.
	SkipMigrations bool
}

// InitMobileDatabase initializes a database for mobile environments.
//
// Mobile apps should:
// 1. Determine the appropriate storage location for their platform
// 2. Pass the full path to this function
// 3. Handle the returned SqliteStore
//
// Example (iOS/Swift):
//   let dbPath = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
//       .appendingPathComponent("tapd.db").path
//   // Pass dbPath to Go initialization
//
// Example (Android/Kotlin):
//   val dbPath = context.filesDir.path + "/tapd.db"
//   // Pass dbPath to Go initialization
func InitMobileDatabase(cfg *MobileConfig) (*tapdb.SqliteStore, error) {
	if cfg == nil || cfg.DBPath == "" {
		return nil, fmt.Errorf("mobile config with DBPath required")
	}

	sqliteCfg := &tapdb.SqliteConfig{
		SkipMigrations:   cfg.SkipMigrations,
		DatabaseFileName: cfg.DBPath,
	}

	store, err := tapdb.NewSqliteStore(sqliteCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create mobile database: %w", err)
	}

	return store, nil
}

// MobileDatabasePath returns the recommended database path for mobile platforms.
// This is a helper for gomobile bindings.
func MobileDatabasePath(platform, appDir string) string {
	// gomobile doesn't support enums well, so we use strings
	switch platform {
	case "ios":
		// iOS: Use Documents directory
		return appDir + "/tapd.db"
	case "android":
		// Android: Use app files directory
		return appDir + "/tapd.db"
	default:
		// Fallback
		return "./tapd.db"
	}
}
