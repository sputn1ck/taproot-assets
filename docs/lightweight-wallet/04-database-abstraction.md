# Task 04: Database Abstraction

## Goal

Create an injectable database interface layer that allows mobile and WASM environments to provide their own SQL database implementation while maintaining compatibility with existing tapdb sqlc-generated code.

## Existing Code to Reuse

**Database Layer:** `tapdb/` directory
- `tapdb/interfaces.go` - Core database interfaces
- `tapdb/sqlite.go` - SQLite implementation
- `tapdb/sqlc/` - Generated query code

**Key Interfaces:**
```go
type BatchedQuerier interface {
    sqlc.Querier
    BeginTx(ctx context.Context, options TxOptions) (*sql.Tx, error)
    Backend() sqlc.BackendType
}

type TransactionExecutor[Q any] struct {
    BatchedQuerier
    createQuery QueryCreator[Q]
    opts *txExecutorOptions
}
```

**Types to Reuse:**
- All existing tapdb interfaces
- All sqlc-generated query functions
- `tapdb.BaseDB` struct

## Interface Strategy

Make the database injectable WITHOUT modifying existing tapdb code or sqlc-generated files. Create a thin bridge layer that accepts external `*sql.DB` instances.

**Key Principle:** Don't fork tapdb, extend it.

**Location:** `lightweight-wallet/db/`

## Implementation Approach

### 1. Understand Current Database Initialization

Current flow (in full tapd):
```go
// tapdb/sqlite.go
db, err := sql.Open("sqlite", dbPath)
baseDB := &BaseDB{
    DB:      db,
    Queries: sqlc.New(db),
}
```

Problem: This assumes we control database creation.

### 2. Create Injectable Database Factory

Create factory that accepts external DB:

```go
// lightweight-wallet/db/factory.go

func NewBatchedQuerierFromDB(db *sql.DB, backend sqlc.BackendType) tapdb.BatchedQuerier {
    return &tapdb.BaseDB{
        DB:      db,
        Queries: sqlc.New(db),
    }
}

func NewAssetStoreFromDB(db *sql.DB) *tapdb.AssetStore {
    baseDB := NewBatchedQuerierFromDB(db, sqlc.BackendTypeSqlite)
    return tapdb.NewAssetStore(baseDB)
}

// Similar factories for other stores...
```

### 3. Mobile Database Interface (gomobile)

gomobile supports `database/sql` but needs careful handling:

**Mobile Side (Swift/Kotlin):**
```swift
// iOS: Create SQLite database
let dbPath = /* app documents directory */ + "/tapd.db"
// Pass path to Go, which creates sql.DB internally
```

**Go Side:**
```go
// mobile/db.go

func InitDatabaseMobile(dbPath string) error {
    db, err := sql.Open("sqlite", dbPath)
    if err != nil {
        return err
    }

    // Run migrations
    if err := runMigrations(db); err != nil {
        return err
    }

    // Store globally or in config
    globalDB = db
    return nil
}
```

**Challenge:** gomobile doesn't export `*sql.DB` directly. Solution: Initialize DB on Go side with path from mobile.

### 4. WASM Database Interface

WASM can use sql.js (SQLite compiled to WASM):

**Options:**
1. **sql.js via syscall/js**: Call JS SQLite from Go
2. **SQLite WASM**: Use modernc.org/sqlite (pure Go, WASM-compatible)
3. **IndexedDB wrapper**: Create SQL-like interface over IndexedDB

**Recommended:** Use modernc.org/sqlite (pure Go, already used by tapd)

```go
// wasm/db.go

//go:build wasm

import _ "modernc.org/sqlite"

func InitDatabaseWASM() (*sql.DB, error) {
    // SQLite works in WASM with modernc.org/sqlite
    // Use in-memory or OPFS (Origin Private File System)
    db, err := sql.Open("sqlite", "file:tapd.db?mode=memory")
    return db, err
}
```

**For persistence:** Use OPFS or IndexedDB with custom VFS.

### 5. Migration Handling

Ensure migrations work with injectable DB:

**Current:** tapdb has SQL migrations in `tapdb/sqlc/migrations/`

**Strategy:**
```go
// lightweight-wallet/db/migrations.go

func RunMigrations(db *sql.DB, backend sqlc.BackendType) error {
    // Use golang-migrate or similar
    migrator := migrate.New(...)

    // Point to existing migration files
    migrator.SetFS(tapdb.MigrationsFS)

    return migrator.Up()
}
```

**Key Point:** Reuse existing migration files, don't duplicate.

### 6. Database Configuration

Make database configurable:

```go
type DatabaseConfig struct {
    // For standard Go apps
    DBPath string

    // For mobile (Go creates DB)
    MobileDBPath string

    // For WASM
    UseMemory bool

    // For custom DB
    ExternalDB *sql.DB

    // Backend type
    Backend sqlc.BackendType  // sqlite or postgres
}

func InitDatabase(cfg DatabaseConfig) (tapdb.BatchedQuerier, error) {
    var db *sql.DB
    var err error

    if cfg.ExternalDB != nil {
        db = cfg.ExternalDB
    } else if cfg.MobileDBPath != "" {
        db, err = sql.Open("sqlite", cfg.MobileDBPath)
    } else if cfg.UseMemory {
        db, err = sql.Open("sqlite", ":memory:")
    } else {
        db, err = sql.Open("sqlite", cfg.DBPath)
    }

    if err != nil {
        return nil, err
    }

    // Run migrations
    if err := RunMigrations(db, cfg.Backend); err != nil {
        return nil, err
    }

    return NewBatchedQuerierFromDB(db, cfg.Backend), nil
}
```

### 7. Store Initialization

Create factory functions for all tapdb stores:

```go
type Stores struct {
    AssetStore        *tapdb.AssetStore
    MintingStore      *tapdb.AssetMintingStore
    UniverseStore     *tapdb.UniverseStore
    // ... all other stores
}

func InitAllStores(db tapdb.BatchedQuerier) (*Stores, error) {
    return &Stores{
        AssetStore:    tapdb.NewAssetStore(db),
        MintingStore:  tapdb.NewAssetMintingStore(db),
        UniverseStore: tapdb.NewUniverseStore(db),
        // ...
    }, nil
}
```

## Directory Structure

```
lightweight-wallet/db/
├── factory.go         # Database factory functions
├── migrations.go      # Migration runner
├── config.go          # Database configuration
├── stores.go          # Store initialization
├── mobile.go          # Mobile-specific DB init
├── wasm.go            # WASM-specific DB init (//go:build wasm)
├── factory_test.go    # Tests
└── integration_test.go # Integration tests
```

## Verification

### Unit Tests

Test database initialization:

```go
func TestDatabaseFactory_FromPath(t *testing.T) {
    tmpDir := t.TempDir()
    dbPath := filepath.Join(tmpDir, "test.db")

    cfg := DatabaseConfig{
        DBPath:  dbPath,
        Backend: sqlc.BackendTypeSqlite,
    }

    db, err := InitDatabase(cfg)
    require.NoError(t, err)
    require.NotNil(t, db)

    // Verify database works
    stores, err := InitAllStores(db)
    require.NoError(t, err)
    require.NotNil(t, stores.AssetStore)
}

func TestDatabaseFactory_ExternalDB(t *testing.T) {
    externalDB, _ := sql.Open("sqlite", ":memory:")

    cfg := DatabaseConfig{
        ExternalDB: externalDB,
        Backend:    sqlc.BackendTypeSqlite,
    }

    db, err := InitDatabase(cfg)
    require.NoError(t, err)

    // Should use provided DB
    // Verify migrations ran
}
```

### Migration Tests

```go
func TestMigrations_RunSuccessfully(t *testing.T) {
    db, _ := sql.Open("sqlite", ":memory:")

    err := RunMigrations(db, sqlc.BackendTypeSqlite)
    require.NoError(t, err)

    // Verify tables exist
    tables := []string{"assets", "genesis_info_view", "mint_batches", ...}
    for _, table := range tables {
        var exists bool
        err := db.QueryRow(
            "SELECT 1 FROM sqlite_master WHERE type='table' AND name=?",
            table,
        ).Scan(&exists)
        require.NoError(t, err)
    }
}
```

### Store Initialization Tests

```go
func TestStores_InitializeCorrectly(t *testing.T) {
    db := setupTestDB(t)
    stores, err := InitAllStores(db)
    require.NoError(t, err)

    // Test each store works
    ctx := context.Background()

    // Test AssetStore
    assets, err := stores.AssetStore.FetchAllAssets(ctx, ...)
    require.NoError(t, err)
}
```

### Mobile Simulation Test

```go
func TestMobile_DBInitialization(t *testing.T) {
    // Simulate mobile environment
    tmpDir := t.TempDir()
    mobilePath := filepath.Join(tmpDir, "mobile.db")

    err := InitDatabaseMobile(mobilePath)
    require.NoError(t, err)

    // Verify DB accessible
    // Verify stores work
}
```

### WASM Test

```go
//go:build wasm

func TestWASM_DBInitialization(t *testing.T) {
    db, err := InitDatabaseWASM()
    require.NoError(t, err)

    // Verify in-memory DB works
    stores, err := InitAllStores(db)
    require.NoError(t, err)
}
```

## Integration Points

**Depends On:**
- None (foundation layer)

**Depended On By:**
- Task 05 (Proof System) - Uses AssetStore, UniverseStore
- Task 06 (Asset Minting) - Uses MintingStore
- Task 07 (Asset Sending) - Uses AssetStore, TransferLog
- Task 08 (Asset Receiving) - Uses AssetStore, AddrBook
- Task 09 (Universe) - Uses UniverseStore
- Task 10 (Server) - Initializes all stores

## Success Criteria

- [ ] Can initialize database from file path
- [ ] Can initialize database from external `*sql.DB`
- [ ] All migrations run successfully
- [ ] All tapdb stores initialize correctly
- [ ] Mobile-compatible API (no unsupported types)
- [ ] WASM-compatible (pure Go SQLite)
- [ ] No modifications to existing tapdb code
- [ ] All unit tests pass
- [ ] Integration tests verify store operations
- [ ] Thread-safe initialization
- [ ] Proper error handling

## Configuration

```go
type DatabaseConfig struct {
    // Standard desktop/server
    DBPath string

    // Mobile
    MobileDBPath string

    // WASM
    UseMemory      bool
    UseOPFS        bool  // Origin Private File System

    // Custom
    ExternalDB     *sql.DB
    SkipMigrations bool

    // Common
    Backend        sqlc.BackendType

    // Performance
    MaxOpenConns   int
    MaxIdleConns   int
    ConnMaxLife    time.Duration
}
```

## Mobile-Specific Considerations

### gomobile Limitations

- Can't export `*sql.DB` directly
- Can pass primitive types (string, int, []byte)
- Can pass interfaces but with limitations

**Solution:** Initialize DB in Go code, expose operations:

```go
// mobile/tapd.go

type MobileTapd struct {
    db     *sql.DB
    stores *Stores
}

func NewMobileTapd(dbPath string, network string) (*MobileTapd, error) {
    cfg := DatabaseConfig{
        MobileDBPath: dbPath,
        Backend:      sqlc.BackendTypeSqlite,
    }

    db, err := InitDatabase(cfg)
    if err != nil {
        return nil, err
    }

    stores, err := InitAllStores(db)
    if err != nil {
        return nil, err
    }

    return &MobileTapd{
        db:     db,
        stores: stores,
    }, nil
}
```

### Database File Location

**iOS:**
- Use app's Documents directory
- `FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]`

**Android:**
- Use `Context.getFilesDir()`
- `/data/data/com.yourapp/files/`

Pass path to Go initialization function.

## WASM-Specific Considerations

### Storage Options

1. **In-Memory:** Fast but data lost on page reload
2. **OPFS:** Persistent but requires WASM build flags
3. **IndexedDB:** Persistent, accessible from JS

**Recommendation:** Start with in-memory, add OPFS for persistence.

### OPFS Integration

```go
//go:build wasm

import (
    "syscall/js"
)

func createOPFSDatabase() (*sql.DB, error) {
    // Use OPFS via syscall/js
    // Call JavaScript OPFS APIs
    // Mount as VFS for SQLite
}
```

### Build Tags

```go
// db_wasm.go
//go:build wasm
// WASM-specific implementation

// db_native.go
//go:build !wasm
// Native implementation
```

## Migration Strategy

### Existing Migrations

tapd has 40+ migration files in `tapdb/sqlc/migrations/`.

**Strategy:** Reuse as-is, don't copy.

```go
import (
    "embed"
    "github.com/golang-migrate/migrate/v4"
    _ "github.com/golang-migrate/migrate/v4/database/sqlite"
    "github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed ../../tapdb/sqlc/migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(db *sql.DB, backend sqlc.BackendType) error {
    sourceDriver, err := iofs.New(migrationsFS, ".")
    if err != nil {
        return err
    }

    migrator, err := migrate.NewWithSourceInstance(
        "iofs", sourceDriver,
        fmt.Sprintf("sqlite://%s", dbPath),
    )
    if err != nil {
        return err
    }

    return migrator.Up()
}
```

## Error Handling

Handle these error cases:
- Database file not found
- Permission denied
- Corruption
- Migration failures
- Schema version mismatch
- Concurrent access

## Performance Considerations

### Connection Pooling

```go
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(25)
db.SetConnMaxLifetime(5 * time.Minute)
```

### WAL Mode (SQLite)

```go
db.Exec("PRAGMA journal_mode=WAL")
db.Exec("PRAGMA synchronous=NORMAL")
```

### Indexes

Verify existing tapdb indexes are sufficient for lightweight use cases.

## Security Considerations

- Database encryption (SQLCipher for mobile)
- Secure file permissions
- SQL injection prevention (use parameterized queries - sqlc handles this)
- Backup encryption

## Future Enhancements

- PostgreSQL support (tapdb already has postgres support)
- Database encryption
- Automatic backups
- Database compaction
- Query performance monitoring
- Connection pooling tuning
