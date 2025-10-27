# Task 06: Asset Minting

## Goal

Integrate tapgarden's minting functionality (ChainPlanter, Caretaker) with lightweight wallet backends to enable asset creation without LND dependency.

## Existing Code to Reuse

**Core Minting Logic:** `tapgarden/` directory - **REUSE ALL**

**Key Components:**
- `tapgarden/planter.go` - ChainPlanter implementation
- `tapgarden/caretaker.go` - Caretaker (batch finalization)
- `tapgarden/custodian.go` - Custodian (asset receiving)
- `tapgarden/interface.go` - Core interfaces

**Types to Reuse:**
- `tapgarden.Seedling` - Asset creation request
- `tapgarden.MintingBatch` - Batch of assets to mint
- `tapgarden.GardenKit` - Configuration for minting
- All asset structs from `asset/` package

**DO NOT:**
- Modify ChainPlanter logic
- Change batch state machine
- Alter proof generation for minting

## Interface Strategy

Wire up existing `ChainPlanter` with lightweight implementations from Tasks 01-05. The minting flow remains identical, just swap out dependencies.

**Key Interfaces to Implement:**
- ChainBridge ✅ (Task 01)
- WalletAnchor ✅ (Task 02)
- KeyRing ✅ (Task 03)

**Location:** Wire in `lightweight-wallet/mint/config.go`, reuse `tapgarden/` directly

## Implementation Approach

### 1. Minting Flow Overview

```
User Request
     ↓
QueueNewSeedling (ChainPlanter)
     ↓
Batch Seedlings (Pending → Frozen)
     ↓
FundBatch (Create genesis PSBT)
     ↓
SealBatch (Derive keys, create commitments)
     ↓
FinalizeBatch (Sign PSBT, broadcast)
     ↓
Caretaker (Wait for confirmation)
     ↓
Finalized (Assets created)
```

### 2. ChainPlanter Setup

Initialize with lightweight dependencies:

```go
import "github.com/lightninglabs/taproot-assets/tapgarden"

planterCfg := &tapgarden.PlanterConfig{
    GardenKit: tapgarden.GardenKit{
        WalletAnchor:    btcWallet,      // Task 02
        ChainBridge:     mempoolBridge,  // Task 01
        KeyRing:         keyRing,        // Task 03
        GenSigner:       assetSigner,    // Local signer
        ProofFiles:      proofArchive,   // Task 05
        ProofArchive:    proofArchive,   // Task 05
        TreeStore:       treeStore,      // tapdb
    },
    MintingStore: mintingStore,  // tapdb
    BatchInterval: 10 * time.Second,
}

planter := tapgarden.NewChainPlanter(planterCfg)
```

### 3. Caretaker Setup

Initialize for batch finalization:

```go
caretakerCfg := &tapgarden.CaretakerConfig{
    ChainBridge:    mempoolBridge,
    GroupKeyIndex:  groupKeyIndex,
    ProofUpdater:   proofUpdater,
    ProofArchive:   proofArchive,
    ProofWatcher:   proofWatcher,
    ErrChan:        errChan,
}

caretaker := tapgarden.NewCaretaker(caretakerCfg)
```

### 4. Asset Minting API

Create lightweight API wrapper:

```go
type AssetMinter struct {
    planter   tapgarden.Planter
    caretaker *tapgarden.Caretaker
}

func (m *AssetMinter) MintAsset(ctx context.Context, req *MintAssetRequest) (*Asset, error) {
    // Create seedling
    seedling := &tapgarden.Seedling{
        AssetType: asset.Type(req.AssetType),
        AssetName: req.Name,
        Metadata:  req.Metadata,
        Amount:    req.Amount,
        // ...
    }

    // Queue for minting
    updatesChan, err := m.planter.QueueNewSeedling(seedling)
    if err != nil {
        return nil, err
    }

    // Wait for completion
    for update := range updatesChan {
        if update.Error != nil {
            return nil, update.Error
        }
        if update.AssetProof != nil {
            // Minting complete
            return assetFromProof(update.AssetProof)
        }
    }
}

func (m *AssetMinter) FinalizeBatch(ctx context.Context) error {
    params := tapgarden.FinalizeParams{}
    _, err := m.planter.FinalizeBatch(params)
    return err
}
```

### 5. Batch Management

Reuse existing batch management:

```go
func (m *AssetMinter) ListBatches(ctx context.Context) ([]*MintingBatch, error) {
    params := tapgarden.ListBatchesParams{}
    return m.planter.ListBatches(params)
}

func (m *AssetMinter) CancelBatch(ctx context.Context) error {
    _, err := m.planter.CancelBatch()
    return err
}
```

### 6. Genesis Transaction Funding

Handled automatically by ChainPlanter using WalletAnchor (Task 02):

- Planter creates genesis PSBT
- Calls `WalletAnchor.FundPsbt()` to add Bitcoin inputs
- Calls `WalletAnchor.SignAndFinalizePsbt()` to sign
- Calls `ChainBridge.PublishTransaction()` to broadcast

### 7. Confirmation Monitoring

Handled by Caretaker using ChainBridge (Task 01):

- Registers for confirmation notifications
- Waits for required confirmations
- Updates batch state
- Generates final proofs

## Directory Structure

```
lightweight-wallet/mint/
├── config.go          # Minter configuration
├── minter.go          # AssetMinter wrapper
├── api.go             # Public API
└── minter_test.go     # Tests

# Reuse directly (no copies):
../../tapgarden/       # All minting logic
```

## Verification

### Unit Tests

Test minter setup:

```go
func TestAssetMinter_Setup(t *testing.T) {
    wallet := setupTestWallet(t)
    chainBridge := setupTestChainBridge(t)
    keyRing := setupTestKeyRing(t)

    minter := NewAssetMinter(&MinterConfig{
        WalletAnchor: wallet,
        ChainBridge:  chainBridge,
        KeyRing:      keyRing,
        // ...
    })

    require.NotNil(t, minter)
}
```

### Integration Tests

Test full minting flow:

```go
func TestAssetMinter_MintAsset(t *testing.T) {
    // Setup lightweight wallet with funded BTC wallet
    wallet := setupFundedLightweightWallet(t)

    // Create mint request
    req := &MintAssetRequest{
        AssetType: asset.Normal,
        Name:      "TEST_ASSET",
        Amount:    1000,
        Metadata:  []byte("test metadata"),
    }

    // Mint asset
    mintedAsset, err := wallet.MintAsset(ctx, req)
    require.NoError(t, err)
    require.NotNil(t, mintedAsset)

    // Verify asset exists in database
    stored, err := wallet.FetchAsset(ctx, mintedAsset.ID())
    require.NoError(t, err)
    require.Equal(t, mintedAsset.ID(), stored.ID())

    // Verify proof exists
    proofLocator := proof.Locator{AssetID: &mintedAsset.ID()}
    proofBlob, err := wallet.proofArchive.FetchProof(ctx, proofLocator)
    require.NoError(t, err)
    require.NotEmpty(t, proofBlob)
}

func TestAssetMinter_BatchMinting(t *testing.T) {
    wallet := setupFundedLightweightWallet(t)

    // Queue multiple assets
    assets := []string{"ASSET1", "ASSET2", "ASSET3"}
    for _, name := range assets {
        _, err := wallet.QueueAssetForMinting(ctx, &MintAssetRequest{
            Name: name,
            Amount: 100,
        })
        require.NoError(t, err)
    }

    // Finalize batch
    err := wallet.FinalizeBatch(ctx)
    require.NoError(t, err)

    // Wait for confirmation
    // Verify all assets minted
}
```

### State Machine Tests

Test batch state transitions:

```go
func TestBatchStateMachine(t *testing.T) {
    // Test: Pending → Frozen → Committed → Broadcast → Confirmed → Finalized
    // Verify state transitions work correctly
    // Test cancellation at various states
}
```

## Integration Points

**Depends On:**
- Task 01 (Chain Backend) - Transaction broadcast, confirmations
- Task 02 (Wallet) - PSBT funding and signing
- Task 03 (KeyRing) - Key derivation for assets
- Task 04 (Database) - MintingStore
- Task 05 (Proof System) - Proof generation and storage

**Depended On By:**
- Task 10 (Server) - Exposes minting via RPC
- Task 11 (Go Library) - Embeddable minting API

## Success Criteria

- [ ] Can mint individual assets
- [ ] Can batch multiple assets in single transaction
- [ ] Genesis transaction broadcasts successfully
- [ ] Confirmation monitoring works
- [ ] Proofs generated correctly
- [ ] Assets stored in database
- [ ] Batch state machine works correctly
- [ ] Can cancel pending batches
- [ ] Error handling comprehensive
- [ ] All integration tests pass
- [ ] Compatible with proofs from full tapd

## Configuration

```go
type MinterConfig struct {
    // Dependencies
    WalletAnchor  WalletAnchor
    ChainBridge   ChainBridge
    KeyRing       KeyRing
    ProofArchive  proof.Archiver
    MintingStore  tapgarden.MintingStore

    // Batch settings
    BatchInterval time.Duration  // Default: 10s

    // Confirmation settings
    NumConfs      uint32          // Default: 6

    // Fee settings
    FeeRate       chainfee.SatPerKWeight
}
```

## Error Handling

Handle these error cases:
- Insufficient funds for genesis transaction
- Transaction broadcast failure
- Confirmation timeout
- Reorg detection
- Batch cancellation
- Invalid seedling parameters
- Duplicate asset names
- Proof generation failures

## Performance Considerations

- Batching reduces on-chain footprint
- Larger batches = better efficiency
- Consider batch size limits (avoid huge transactions)
- Balance batch interval vs user experience

## Asset Types

Support all asset types:
- **Normal:** Fixed supply
- **Collectible:** Unique items (amount=1)

Future: Asset groups for reissuance.

## Metadata Handling

Support asset metadata:
- Arbitrary bytes (images, JSON, etc.)
- Metadata hash committed in genesis
- Metadata stored separately (off-chain or Universe)

## Future Enhancements

- Asset groups (reissuance)
- Group key management
- Batch size optimization
- Fee estimation improvements
- Multi-sig genesis transactions
