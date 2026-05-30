//go:build js && wasm

package tapdb

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/lightninglabs/taproot-assets/tapdb/sqlc"
	"github.com/sputn1ck/go-wasmsqlite"
	_ "github.com/sputn1ck/go-wasmsqlite"
	"github.com/stretchr/testify/require"
)

const (
	defaultMaxConns        = 25
	defaultWasmSQLiteVFS   = "auto"
	defaultConnMaxLifetime = 10 * time.Minute
)

var (
	sqliteSchemaReplacements = sqlc.SQLiteSchemaReplacements
)

// SqliteConfig holds all the config arguments needed to interact with our
// sqlite DB.
//
// nolint: lll
type SqliteConfig struct {
	// SkipMigrations if true, then all the tables will be created on start
	// up if they don't already exist.
	SkipMigrations bool `long:"skipmigrations" description:"Skip applying migrations on startup."`

	// SkipMigrationDbBackup if true, then a backup of the database will not
	// be created before applying migrations.
	SkipMigrationDbBackup bool `long:"skipmigrationdbbackup" description:"Skip creating a backup of the database before applying migrations."`

	// SkipTmpDirCheck disables the writable temp directory check performed
	// before opening the database.
	SkipTmpDirCheck bool `long:"skiptmpdircheck" description:"Skip checking that the temp directory is writable before opening the database."`

	// DatabaseFileName is the full file path where the database file can be
	// found.
	DatabaseFileName string `long:"dbfile" description:"The full path to the database."`
}

// SqliteStore is a sqlite3 based database for the Taproot Asset daemon.
type SqliteStore struct {
	cfg *SqliteConfig

	*BaseDB
}

// NewSqliteStore attempts to open a new sqlite database based on the passed
// config.
func NewSqliteStore(cfg *SqliteConfig) (*SqliteStore, error) {
	pragmaOptions := []string{
		"foreign_keys=on",
		"synchronous=full",
	}

	wasmOptions := make(url.Values)
	setWasmSQLiteStorage(wasmOptions, cfg.DatabaseFileName)
	wasmOptions.Set("busy_timeout", "5000")
	wasmOptions.Set("parse_time", "true")
	wasmOptions.Set("pragma", strings.Join(pragmaOptions, ";"))

	db, err := sql.Open("wasmsqlite", wasmOptions.Encode())
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(defaultConnMaxLifetime)

	queries := sqlc.NewSqlite(db)
	s := &SqliteStore{
		cfg: cfg,
		BaseDB: &BaseDB{
			DB:      db,
			Queries: queries,
		},
	}

	if !cfg.SkipMigrations {
		err := s.ExecuteMigrations(
			s.backupAndMigrate, WithProgrammaticMigrations(
				makeProgrammaticMigrations(
					s, programmaticMigrations, true,
				),
			),
		)
		if err != nil {
			return nil, fmt.Errorf("error executing migrations: "+
				"%w", err)
		}
	}

	return s, nil
}

func (s *SqliteStore) backupAndMigrate(mig *migrate.Migrate,
	currentDbVersion int, maxMigrationVersion uint) error {

	if currentDbVersion >= int(maxMigrationVersion) {
		log.Infof("Current database version is up-to-date, skipping "+
			"migration attempt (current_db_version=%v, "+
			"max_migration_version=%v)", currentDbVersion,
			maxMigrationVersion)

		return nil
	}

	log.Infof("Applying migrations to browser sqlite database")
	return mig.Up()
}

// ExecuteMigrations runs migrations for the sqlite database, depending on the
// target given, either all migrations or up to a given version.
func (s *SqliteStore) ExecuteMigrations(target MigrationTarget,
	optFuncs ...MigrateOpt) error {

	opts := defaultMigrateOptions()
	for _, optFunc := range optFuncs {
		optFunc(opts)
	}

	driver, err := wasmsqlite.NewMigrateDriver(s.DB)
	if err != nil {
		return fmt.Errorf("error creating sqlite migration: %w", err)
	}

	sqliteFS := newReplacerFS(sqlSchemas, sqliteSchemaReplacements)
	return applyMigrations(
		sqliteFS, driver, "sqlc/migrations", "sqlite", target, opts,
	)
}

// NewTestSqliteDB is a helper function that creates a new SQLite database
// instance for testing.
func NewTestSqliteDB(t testing.TB) *SqliteStore {
	t.Helper()

	dbFileName := fmt.Sprintf("/%s.db", t.Name())
	return NewTestSqliteDbHandleFromPath(t, dbFileName)
}

// NewTestSqliteDbHandleFromPath is a helper function that creates a SQLite
// database handle given a database file path.
func NewTestSqliteDbHandleFromPath(t testing.TB, dbPath string) *SqliteStore {
	t.Helper()

	sqlDB, err := NewSqliteStore(&SqliteConfig{
		DatabaseFileName: dbPath,
		SkipMigrations:   false,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, sqlDB.DB.Close())
	})

	return sqlDB
}

// NewTestSqliteDBWithVersion is a helper function that creates a SQLite
// database for testing and migrates it to the given version.
func NewTestSqliteDBWithVersion(t testing.TB, version uint) *SqliteStore {
	t.Helper()

	dbFileName := fmt.Sprintf("/%s-%d.db", t.Name(), version)
	sqlDB, err := NewSqliteStore(&SqliteConfig{
		DatabaseFileName: dbFileName,
		SkipMigrations:   true,
	})
	require.NoError(t, err)

	require.NoError(t, sqlDB.ExecuteMigrations(TargetVersion(version)))

	t.Cleanup(func() {
		require.NoError(t, sqlDB.DB.Close())
	})

	return sqlDB
}

func setWasmSQLiteStorage(values url.Values, fileName string) {
	vfs := defaultWasmSQLiteVFS
	globalVFS := js.Global().Get("tapdWasmSQLiteVFS")
	if globalVFS.Type() == js.TypeString && globalVFS.String() != "" {
		vfs = globalVFS.String()
	} else {
		globalVFS = js.Global().Get("lndWasmSQLiteVFS")
		if globalVFS.Type() == js.TypeString && globalVFS.String() != "" {
			vfs = globalVFS.String()
		}
	}

	if vfs == "memory" {
		values.Set("file", ":memory:")
		values.Set("vfs", "memory")
		return
	}

	values.Set("file", fileName)
	values.Set("vfs", vfs)
	values.Set("journal_mode", "WAL")
}
