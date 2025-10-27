# Lightweight Taproot Assets Wallet - Development Overview

## Project Goal

Create a lightweight, LND-independent Taproot Assets wallet daemon that can be:
- Embedded in Go applications as a library
- Used in mobile apps (iOS/Android via gomobile)
- Deployed in browsers via WASM
- Run as a standalone lightweight daemon

The wallet will use:
- **btcwallet** libraries for PSBT operations and signing
- **mempool.space API** for blockchain monitoring and transaction broadcasting
- **Existing tapd core** for asset operations, proofs, and universe sync

## Core Development Principles

### 1. Reuse Existing tapd Structs

**DO:**
- Import and use existing types from `asset`, `proof`, `commitment`, `address`, `tappsbt` packages
- Reuse database schemas and sqlc-generated code where possible
- Keep proof system, MSSMT trees, and cryptographic operations unchanged
- Use existing RPC protocol definitions

**DON'T:**
- Recreate structs that already exist in tapd
- Fork cryptographic implementations
- Duplicate proof verification logic
- Reinvent asset state machines

**Example:**
```go
// GOOD: Reuse existing types
import (
    "github.com/lightninglabs/taproot-assets/asset"
    "github.com/lightninglabs/taproot-assets/proof"
    "github.com/lightninglabs/taproot-assets/address"
)

func MintAsset(seedling *asset.Genesis) (*asset.Asset, error) {
    // Use existing asset types
}

// BAD: Don't recreate
type MyAsset struct {
    ID []byte
    // ... duplicating asset.Asset
}
```

### 2. Interface Tightly Coupled Code

When code is tightly coupled to LND, create thin abstraction layers:

**Pattern:**
1. Identify the interface (often already exists in `tapgarden` or `tapfreighter`)
2. Create lightweight implementation without changing interface
3. Keep abstractions minimal - just enough to decouple

**Example:**
```go
// Interface already exists in tapgarden/interface.go:
type ChainBridge interface {
    PublishTransaction(context.Context, *wire.MsgTx, string) error
    CurrentHeight(context.Context) (uint32, error)
    // ... other methods
}

// Create lightweight implementation:
type MempoolChainBridge struct {
    apiURL string
    client *http.Client
}

func (m *MempoolChainBridge) PublishTransaction(ctx context.Context, tx *wire.MsgTx, label string) error {
    // Implement using mempool.space API
}
```

**Don't:**
- Create complex abstraction hierarchies
- Add unnecessary middleware layers
- Change existing interfaces if possible

### 3. Test-Based Development

Write tests BEFORE implementation:

**Flow:**
1. Write unit test defining expected behavior
2. Run test (it should fail)
3. Implement minimum code to pass
4. Refactor while keeping tests green

**Test Structure:**
```go
func TestMempoolChainBridge_PublishTransaction(t *testing.T) {
    // Setup
    mockServer := httptest.NewServer(...)
    bridge := NewMempoolChainBridge(mockServer.URL)

    // Test
    tx := wire.NewMsgTx(2)
    err := bridge.PublishTransaction(ctx, tx, "test")

    // Assert
    require.NoError(t, err)
    // Verify API was called correctly
}
```

**Test Coverage Goals:**
- Unit tests: >80% coverage for new code
- Interface compliance tests: 100% (every interface method)
- Integration tests: All critical paths

### 4. Integration Test Suite from Beginning

Start integration tests from Day 1:

**Setup:**
- Bitcoin regtest node (or mock)
- Mock mempool.space API server
- Test database (in-memory SQLite)
- Sample assets and proofs

**Critical Integration Tests:**
1. Complete minting flow (create → fund → broadcast → confirm)
2. Complete send flow (select coins → build tx → broadcast → deliver proof)
3. Complete receive flow (generate address → receive → import proof)
4. Universe sync and discovery
5. Database persistence and recovery

**Directory Structure:**
```
lightweight-wallet/
├── chain/
│   ├── mempool.go
│   └── mempool_test.go          # Unit tests
├── wallet/
│   ├── btcwallet.go
│   └── btcwallet_test.go        # Unit tests
└── itest/
    ├── harness.go                # Test harness
    ├── mint_test.go              # Integration: minting
    ├── send_test.go              # Integration: sending
    ├── receive_test.go           # Integration: receiving
    └── universe_test.go          # Integration: universe sync
```

## Project Structure

```
lightweight-wallet/
├── chain/                        # Chain backend implementations
│   ├── interface.go              # ChainBridge interface
│   ├── mempool/                  # Mempool.space client
│   │   ├── client.go
│   │   ├── notifications.go
│   │   └── websocket.go
│   └── mock.go                   # Mock for testing
├── wallet/                       # Wallet implementations
│   ├── interface.go              # WalletAnchor interface
│   ├── btcwallet/
│   │   ├── wallet.go
│   │   ├── psbt.go
│   │   └── utxo.go
│   └── mock.go                   # Mock for testing
├── keyring/                      # Key management
│   ├── interface.go              # KeyRing interface
│   ├── keyring.go                # BIP32/44 implementation
│   └── mock.go                   # Mock for testing
├── db/                           # Database abstraction
│   ├── interface.go              # Injectable DB interface
│   └── bridge.go                 # Bridge to tapdb
├── server/                       # Lightweight server
│   ├── server.go                 # Main server
│   ├── config.go                 # Configuration
│   └── rpc.go                    # RPC handlers (reuse existing)
├── client/                       # Go library API
│   └── client.go                 # Embeddable client
├── mobile/                       # Mobile bindings
│   ├── tapd.go                   # gomobile exports
│   └── db_mobile.go              # Mobile DB interface
├── wasm/                         # WASM support
│   ├── tapd_js.go                # WASM exports
│   └── db_wasm.go                # IndexedDB integration
├── itest/                        # Integration tests
│   ├── harness.go
│   ├── mint_test.go
│   ├── send_test.go
│   ├── receive_test.go
│   └── universe_test.go
└── docs/                         # This documentation
    └── lightweight-wallet/
        ├── 00-overview.md
        └── ...
```

## Development Workflow

### Phase 1: Foundation (Weeks 1-3)
1. Chain backend interface (Task 01)
2. Wallet interface (Task 02)
3. KeyRing interface (Task 03)
4. Basic integration test harness (Task 14)

### Phase 2: Core Operations (Weeks 4-7)
5. Database abstraction (Task 04)
6. Proof system integration (Task 05)
7. Asset minting (Task 06)
8. Asset sending (Task 07)
9. Asset receiving (Task 08)

### Phase 3: Federation & Server (Weeks 8-10)
10. Universe client integration (Task 09)
11. Server initialization (Task 10)
12. Go library API (Task 11)

### Phase 4: Platform Support (Weeks 11-14)
13. Mobile bindings (Task 12)
14. WASM support (Task 13)
15. Full integration test suite (Task 14)

## Code Review Checklist

Before marking any task complete, verify:

- [ ] All unit tests pass
- [ ] Integration tests updated and passing
- [ ] Existing tapd structs reused where possible
- [ ] Interfaces match existing tapd interfaces
- [ ] No tight coupling to implementation details
- [ ] Documentation updated
- [ ] Mock implementations provided for testing
- [ ] Error handling is comprehensive
- [ ] No hardcoded values (use config)
- [ ] Logging added for debugging

## Testing Strategy

### Unit Tests
- Test each component in isolation
- Use mocks for dependencies
- Focus on business logic
- Fast execution (<1s per test file)

### Integration Tests
- Test complete workflows
- Use real database (in-memory SQLite)
- Mock external services (mempool.space)
- Slower but comprehensive

### Manual Testing
- Run against Bitcoin regtest
- Test mobile builds on actual devices
- Test WASM in real browsers
- Performance testing with large proof sets

## Success Metrics

- **Code Coverage**: >80% for new code
- **Test Execution**: All tests pass on every commit
- **Integration**: Can mint, send, receive assets without LND
- **Mobile**: Successfully builds and runs on iOS/Android
- **WASM**: Successfully runs in Chrome/Firefox/Safari
- **Performance**: Acceptable latency with mempool.space API
- **Compatibility**: Can interact with full tapd nodes

## Getting Started

1. Read this overview document thoroughly
2. Review existing tapd codebase to understand:
   - `tapgarden/interface.go` - Core interfaces to implement
   - `tapfreighter/interface.go` - Transfer interfaces
   - `proof/` - Proof system (reuse as-is)
   - `asset/` - Asset types (reuse as-is)
   - `tapdb/` - Database layer (understand schema)
3. Start with Task 01 (Chain Backend Interface)
4. Write tests first, then implement
5. Run integration tests frequently
6. Keep commits atomic and well-documented

## Questions?

When unsure about implementation:
1. Check if tapd already has the code
2. Look for existing interfaces
3. Prefer composition over inheritance
4. Keep it simple - MVP first
5. Ask for code review early and often

## References

- [TAP Protocol Spec](https://github.com/lightninglabs/taproot-assets/blob/main/docs/bip-tap.mediawiki)
- [btcwallet Documentation](https://pkg.go.dev/github.com/btcsuite/btcwallet)
- [mempool.space API](https://mempool.space/docs/api/rest)
- [gomobile](https://pkg.go.dev/golang.org/x/mobile/cmd/gomobile)
- Main tapd codebase: `../` (parent directory)
