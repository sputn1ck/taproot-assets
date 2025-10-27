# Task 08: Asset Receiving

## Goal

Integrate tapgarden's Custodian and address.Book functionality for receiving assets without LND dependency.

## Existing Code to Reuse

**Receiving Components:** - **REUSE ALL**

- `tapgarden/custodian.go` - Custodian (monitors incoming)
- `address/book.go` - Address book
- `address/address.go` - TAP address encoding

**Types to Reuse:**
- `address.Tap` - Taproot Asset address
- `address.AddrWithKeyInfo` - Address with key material
- All proof types from `proof/`

## Interface Strategy

Wire Custodian and AddrBook with lightweight implementations.

**Location:** Wire in `lightweight-wallet/receive/config.go`

## Implementation Approach

### 1. Receiving Flow

```
Generate Address
     ↓
Share with Sender
     ↓
Custodian Monitors Chain
     ↓
Detects Asset Transfer
     ↓
Imports Taproot Output
     ↓
Receives Proof via Courier
     ↓
Verifies Proof
     ↓
Stores Asset
     ↓
Complete
```

### 2. Address Book Setup

```go
import "github.com/lightninglabs/taproot-assets/address"

addrBook := address.NewBook(address.BookConfig{
    Store:       addrStore,    // tapdb
    Chain:       chainParams,
    KeyRing:     keyRing,      // Task 03
})
```

### 3. Custodian Setup

```go
custodianCfg := &tapgarden.CustodianConfig{
    ChainParams:    chainParams,
    WalletAnchor:   btcWallet,     // Task 02  
    ChainBridge:    mempoolBridge,  // Task 01
    AddrBook:       addrBook,
    ProofArchive:   proofArchive,   // Task 05
    ProofNotifier:  proofNotifier,
    ErrChan:        errChan,
}

custodian := tapgarden.NewCustodian(custodianCfg)
```

### 4. Address Generation API

```go
type AssetReceiver struct {
    addrBook  *address.Book
    custodian *tapgarden.Custodian
}

func (r *AssetReceiver) NewAddress(ctx context.Context, assetID asset.ID, amount uint64) (*Address, error) {
    addr, err := r.addrBook.NewAddress(ctx, assetID, amount, address.AddressOptions{})
    if err != nil {
        return nil, err
    }

    return &Address{
        Encoded:   addr.Encoded(),
        AssetID:   assetID,
        Amount:    amount,
        TapKey:    addr.TaprootOutputKey,
    }, nil
}
```

### 5. Receiving Monitoring

Custodian automatically monitors:
- Subscribes to wallet transactions (via WalletAnchor)
- Detects transfers to our addresses
- Imports taproot outputs
- Waits for proof delivery
- Verifies and stores proofs

### 6. Manual Proof Import

```go
func (r *AssetReceiver) ImportProof(ctx context.Context, proofBytes []byte) error {
    proofFile, err := proof.NewFile(...)
    
    // Verify proof
    verified, err := r.proofVerifier.Verify(ctx, proofBytes, nil)
    
    // Import to archive
    err = r.proofArchive.ImportProofs(ctx, &proof.AnnotatedProof{
        Blob: proofBytes,
    })
    
    return err
}
```

## Verification

### Integration Tests

```go
func TestReceiving_FullFlow(t *testing.T) {
    sender := setupFundedWallet(t)
    receiver := setupWallet(t)

    // Mint asset on sender
    asset, _ := sender.MintAsset(ctx, &MintRequest{Amount: 1000})

    // Receiver generates address
    addr, err := receiver.NewAddress(ctx, asset.ID(), 100)
    require.NoError(t, err)

    // Sender sends asset
    _, err = sender.SendAsset(ctx, &SendRequest{
        Address: addr.Encoded,
        Amount:  100,
    })
    require.NoError(t, err)

    // Wait for receiver to detect and import
    waitForAsset(t, receiver, asset.ID())

    // Verify asset received
    assets, _ := receiver.ListAssets(ctx)
    require.Len(t, assets, 1)
    require.Equal(t, uint64(100), assets[0].Amount)
}
```

## Integration Points

**Depends On:**
- Task 01 (Chain Backend)
- Task 02 (Wallet) - Transaction monitoring
- Task 03 (KeyRing)
- Task 04 (Database)
- Task 05 (Proof System)

**Depended On By:**
- Task 10 (Server)
- Task 11 (Go Library)

## Success Criteria

- [ ] Can generate TAP addresses
- [ ] Custodian detects incoming transfers
- [ ] Taproot outputs imported
- [ ] Proofs received and verified
- [ ] Assets stored in database
- [ ] Address tracking works
- [ ] All integration tests pass

## Configuration

```go
type ReceiverConfig struct {
    WalletAnchor  WalletAnchor
    ChainBridge   ChainBridge
    KeyRing       KeyRing
    AddrStore     *tapdb.AddrBook
    ProofArchive  proof.Archiver
    ProofCourier  proof.Courier
}
```
