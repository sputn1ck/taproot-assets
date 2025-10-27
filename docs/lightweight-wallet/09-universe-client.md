# Task 09: Universe Client Integration

## Goal

Integrate tapd's Universe federation client for asset discovery and proof sync without modification.

## Existing Code to Reuse

**ALL Universe Code:** `universe/` directory - **REUSE COMPLETELY**

**Key Components:**
- `universe/interface.go` - Interfaces
- `universe/client.go` - RPC client
- `tapdb/universe.go` - Universe storage

**Types:**
- All universe RPC types

## Implementation Approach

### 1. Universe Client Setup

```go
import "github.com/lightninglabs/taproot-assets/universe"

universeClient := universe.NewClient(universeAddr)
```

### 2. Asset Discovery

```go
func (w *LightweightWallet) SyncUniverse(ctx context.Context) error {
    // Query universe for known assets
    roots, err := w.universeClient.AssetRoots(ctx)
    
    // Sync proofs for each asset
    for _, root := range roots {
        err := w.syncAssetProofs(ctx, root.ID)
    }
    
    return nil
}
```

### 3. Proof Sync

Use Universe as proof courier:

```go
universeCourier := proof.NewUniverseRpcCourier(&proof.UniverseRpcCourierConfig{
    UniverseAddrBook: universeClient,
})
```

## Verification

### Integration Tests

```go
func TestUniverse_AssetDiscovery(t *testing.T) {
    // Connect to testnet universe
    wallet := setupWallet(t)
    err := wallet.SyncUniverse(ctx)
    require.NoError(t, err)
    
    // Verify discovered assets
}
```

## Success Criteria

- [ ] Can connect to universe servers
- [ ] Asset discovery works
- [ ] Proof sync works
- [ ] Federation sync works

