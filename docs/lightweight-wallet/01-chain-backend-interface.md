# Task 01: Chain Backend Interface

## Goal

Implement `tapgarden.ChainBridge` interface using mempool.space API to replace LND's chain monitoring capabilities. This is the foundation for all on-chain operations.

## Existing Code to Reuse

**Interface Definition:** `tapgarden/interface.go:317-364`
```go
type ChainBridge interface {
    proof.ChainLookupGenerator
    RegisterConfirmationsNtfn(...)
    RegisterBlockEpochNtfn(...)
    GetBlock(...)
    GetBlockHash(...)
    VerifyBlock(...)
    CurrentHeight(...)
    GetBlockTimestamp(...)
    GetBlockHeaderByHeight(...)
    PublishTransaction(...)
    EstimateFee(...)
}
```

**Types to Reuse:**
- `chainhash.Hash` from `btcsuite/btcd`
- `wire.MsgBlock`, `wire.BlockHeader`, `wire.MsgTx` from `btcsuite/btcd`
- `chainntnfs.ConfirmationEvent` from `lnd`
- `chainfee.SatPerKWeight` from `lnd`

**Existing Mock:** `tapgarden/mock.go` - Study for testing patterns

## Interface Strategy

Create a thin adapter that translates mempool.space REST API calls into ChainBridge interface methods. No need to modify the interface.

**Location:** `lightweight-wallet/chain/mempool/`

**Key Components:**
1. **HTTP Client** - Rate-limited, with retry logic
2. **Polling Engine** - Simulate notifications via polling
3. **WebSocket Handler** (optional) - For real-time block notifications
4. **Cache Layer** - Reduce API calls for frequently accessed data

## Implementation Approach

### 1. HTTP Client Setup

Create robust HTTP client:
- Configurable base URL (support custom mempool.space instances)
- Rate limiting (default: 10 req/sec to avoid 429 errors)
- Exponential backoff retry logic
- Request/response logging for debugging
- Timeout configuration per endpoint

### 2. REST API Mapping

Map each ChainBridge method to mempool.space endpoint:

| Method | Endpoint | Notes |
|--------|----------|-------|
| `CurrentHeight()` | `GET /api/blocks/tip/height` | Simple integer response |
| `GetBlock(hash)` | `GET /api/block/:hash` | Returns full block JSON |
| `GetBlockHash(height)` | `GET /api/block-height/:height` | Returns block hash |
| `GetBlockTimestamp(height)` | `GET /api/block-height/:height` then parse | Two-step process |
| `GetBlockHeaderByHeight(height)` | `GET /api/block-height/:height` | Parse from block data |
| `PublishTransaction(tx)` | `POST /api/tx` | Hex-encoded raw tx |
| `EstimateFee(target)` | `GET /api/v1/fees/recommended` | Map target to fast/medium/slow |
| `VerifyBlock(header, height)` | `GET /api/block-height/:height` | Fetch and compare |

### 3. Notification Simulation

Since mempool.space doesn't have native push notifications, simulate via polling:

**Confirmation Notifications:**
```
RegisterConfirmationsNtfn(txid, numConfs) -> channel
- Start goroutine that polls every N seconds
- Check if tx is confirmed and has required confirmations
- Send event to channel when threshold met
- Support reorg detection by monitoring block hash changes
```

**Block Notifications:**
```
RegisterBlockEpochNtfn() -> channel
- Poll /api/blocks/tip/height every N seconds (configurable)
- Detect height changes
- Fetch full block when new height detected
- Send to channel
```

**Polling Configuration:**
- Default interval: 30 seconds (configurable)
- Exponential backoff on errors
- Cancel via context

### 4. Caching Strategy

Cache to minimize API calls:
- **Block headers**: LRU cache (last 100 blocks)
- **Block timestamps**: LRU cache (last 1000 heights)
- **Current height**: TTL cache (30 seconds)
- **Fee estimates**: TTL cache (60 seconds)

### 5. proof.ChainLookupGenerator Implementation

The ChainBridge also needs to implement `proof.ChainLookupGenerator`:
- `GetBlock(hash)` - Already implemented above
- Map to mempool.space appropriately

## Directory Structure

```
lightweight-wallet/chain/mempool/
├── client.go          # Main HTTP client with rate limiting
├── chain_bridge.go    # ChainBridge interface implementation
├── notifications.go   # Polling-based notification system
├── cache.go           # Caching layer
├── websocket.go       # Optional: WebSocket support
├── types.go           # API response types
├── client_test.go     # Unit tests for HTTP client
├── chain_bridge_test.go  # Interface compliance tests
└── integration_test.go   # Integration tests with mock server
```

## Verification

### Unit Tests

Test each method with mock HTTP server:

```go
func TestMempoolBridge_CurrentHeight(t *testing.T) {
    // Setup mock HTTP server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/api/blocks/tip/height" {
            w.Write([]byte("850000"))
            return
        }
    }))
    defer server.Close()

    bridge := NewMempoolChainBridge(server.URL)
    height, err := bridge.CurrentHeight(context.Background())

    require.NoError(t, err)
    require.Equal(t, uint32(850000), height)
}
```

Test all methods:
- ✅ CurrentHeight
- ✅ GetBlock
- ✅ GetBlockHash
- ✅ GetBlockTimestamp
- ✅ GetBlockHeaderByHeight
- ✅ PublishTransaction
- ✅ EstimateFee
- ✅ VerifyBlock
- ✅ RegisterConfirmationsNtfn (with polling)
- ✅ RegisterBlockEpochNtfn (with polling)

### Integration Tests

Test with real mempool.space API (testnet):

```go
func TestMempoolBridge_RealAPI_Testnet(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    bridge := NewMempoolChainBridge("https://mempool.space/testnet/api")

    // Test fetching current height
    height, err := bridge.CurrentHeight(context.Background())
    require.NoError(t, err)
    require.Greater(t, height, uint32(0))

    // Test fetching block
    hash, err := bridge.GetBlockHash(context.Background(), int64(height-1))
    require.NoError(t, err)

    block, err := bridge.GetBlock(context.Background(), hash)
    require.NoError(t, err)
    require.NotNil(t, block)
}
```

### Interface Compliance Test

Ensure all interface methods are implemented:

```go
func TestMempoolBridge_ImplementsChainBridge(t *testing.T) {
    var _ tapgarden.ChainBridge = (*MempoolChainBridge)(nil)
}
```

### Performance Tests

Test rate limiting and caching:

```go
func TestMempoolBridge_RateLimiting(t *testing.T) {
    // Make 100 requests rapidly
    // Verify rate limiting works (no 429 errors)
    // Verify requests are spread over time
}

func TestMempoolBridge_Caching(t *testing.T) {
    // Make same request multiple times
    // Verify only one API call made
    // Verify cache TTL works
}
```

## Integration Points

**Depends On:**
- None (foundation layer)

**Depended On By:**
- Task 06 (Asset Minting) - Uses ChainBridge for tx broadcast and confirmations
- Task 07 (Asset Sending) - Uses ChainBridge for tx broadcast
- Task 08 (Asset Receiving) - Uses ChainBridge for monitoring incoming txs
- Task 05 (Proof System) - Uses proof.ChainLookupGenerator for proof verification

## Success Criteria

- [ ] All ChainBridge interface methods implemented
- [ ] All unit tests pass with >90% coverage
- [ ] Integration tests work with testnet mempool.space
- [ ] Rate limiting prevents 429 errors
- [ ] Caching reduces redundant API calls
- [ ] Confirmation notifications work correctly
- [ ] Block epoch notifications work correctly
- [ ] Reorg detection works (test by comparing block hashes)
- [ ] Error handling is comprehensive (network errors, API errors, timeouts)
- [ ] Configurable polling intervals
- [ ] Can publish transactions successfully

## Configuration

Add to lightweight wallet config:

```go
type ChainConfig struct {
    // Backend type (mempool, bitcoind, etc.)
    Backend string

    // MempoolSpace specific
    MempoolAPIURL string  // e.g., "https://mempool.space/api"
    PollInterval  time.Duration  // Default: 30s
    RateLimit     int      // Requests per second, default: 10

    // Caching
    CacheSize     int      // Number of items to cache, default: 100
    CacheTTL      time.Duration  // Default: 60s
}
```

## Error Handling

Handle these error cases:
- Network timeouts
- API rate limiting (429)
- API downtime (503)
- Invalid responses
- Transaction broadcast failures
- Block not found
- Reorg detection

Implement retry logic with exponential backoff:
- Initial delay: 1s
- Max delay: 60s
- Max retries: 5

## Performance Considerations

- Poll interval affects confirmation latency (lower = faster but more API calls)
- Cache size affects memory usage
- Rate limiting affects throughput
- Consider WebSocket support for lower latency (optional enhancement)

## Future Enhancements

- WebSocket support for real-time notifications
- Multiple backend support (Electrum, Bitcoin Core RPC)
- Automatic failover between backends
- Transaction fee bumping (RBF)
- CPFP for stuck transactions
