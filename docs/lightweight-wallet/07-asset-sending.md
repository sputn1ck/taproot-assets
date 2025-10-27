# Task 07: Asset Sending

## Goal

Integrate tapfreighter's transfer functionality (ChainPorter, AssetWallet) with lightweight wallet to enable sending assets without LND dependency.

## Existing Code to Reuse

**ALL Transfer Logic:** `tapfreighter/` directory - **REUSE COMPLETELY**

**Key Components:**
- `tapfreighter/chain_porter.go` - ChainPorter (coordinates transfers)
- `tapfreighter/wallet.go` - AssetWallet (coin selection)
- `tapfreighter/parcel.go` - Parcel (transfer requests)
- `tapfreighter/interface.go` - Core interfaces

**Types to Reuse:**
- `tapfreighter.Parcel` - Transfer request
- `tapfreighter.OutboundParcel` - Transfer result
- `tapfreighter.CommitmentConstraints` - Coin selection
- All types from `tappsbt/` package

## Interface Strategy

Wire existing ChainPorter with lightweight implementations. Transfer logic is complex - don't modify it.

**Key Interfaces:**
- ChainBridge ✅ (Task 01)
- WalletAnchor ✅ (Task 02) - Extended with SignPsbt
- KeyRing ✅ (Task 03)
- CoinLister - From tapdb.AssetStore
- ExportLog - From tapdb

**Location:** Wire in `lightweight-wallet/send/config.go`, reuse `tapfreighter/`

## Implementation Approach

### 1. Transfer Flow Overview

```
Send Request
     ↓
Select Coins (AssetWallet)
     ↓
Build Virtual PSBTs (tappsbt)
     ↓
Fund Anchor Transaction (WalletAnchor)
     ↓
Sign PSBTs
     ↓
Broadcast Transaction (ChainBridge)
     ↓
Wait for Confirmation
     ↓
Deliver Proof (Courier)
     ↓
Complete
```

### 2. ChainPorter Setup

```go
import "github.com/lightninglabs/taproot-assets/tapfreighter"

porterCfg := &tapfreighter.ChainPorterConfig{
    Signer:          assetSigner,      // Local signer
    TxValidator:     txValidator,      
    ExportLog:       exportLog,        // tapdb
    ChainBridge:     mempoolBridge,    // Task 01
    Wallet:          btcWallet,        // Task 02
    KeyRing:         keyRing,          // Task 03
    AssetWallet:     assetWallet,      // From tapdb
    CoinSelector:    coinSelector,     // From tapdb
    ProofArchive:    proofArchive,     // Task 05
    ProofCourier:    proofCourier,     // Task 05
}

porter := tapfreighter.NewChainPorter(porterCfg)
```

### 3. Asset Wallet (Coin Selection)

Reuse existing:

```go
assetWallet := tapfreighter.NewAssetWallet(&tapfreighter.WalletConfig{
    CoinSelector: tapdb.NewCoinSelect(assetStore),
})
```

### 4. Send API

Create wrapper:

```go
type AssetSender struct {
    porter tapfreighter.Porter
}

func (s *AssetSender) SendAsset(ctx context.Context, req *SendRequest) (*SendResponse, error) {
    // Build parcel
    parcel := &tapfreighter.Parcel{
        Outputs: []*tapfreighter.ParcelOutput{
            {
                Address:  req.Address,  // TAP address
                Amount:   req.Amount,
                // ...
            },
        },
    }

    // Request shipment
    outbound, err := s.porter.RequestShipment(parcel)
    if err != nil {
        return nil, err
    }

    return &SendResponse{
        TxID:      outbound.AnchorTx.TxHash(),
        Transfers: outbound.Outputs,
    }, nil
}
```

### 5. Address Parsing

Reuse address package:

```go
import "github.com/lightninglabs/taproot-assets/address"

tapAddr, err := address.DecodeAddress(addrString, &chainParams)
if err != nil {
    return nil, err
}

// Use in parcel
parcel := &tapfreighter.Parcel{
    Outputs: []*tapfreighter.ParcelOutput{
        {
            Address: tapAddr,
            Amount:  amount,
        },
    },
}
```

### 6. Custom Transaction Building

For advanced use cases (custom PSBTs):

```go
import "github.com/lightninglabs/taproot-assets/tappsbt"

// Build custom virtual PSBT
vPkt := &tappsbt.VPacket{
    Inputs: []*tappsbt.VInput{...},
    Outputs: []*tappsbt.VOutput{...},
}

// Fund and sign
fundedPkt, err := s.fundVPacket(vPkt)
signedPkt, err := s.signVPacket(fundedPkt)

// Broadcast
err = s.broadcastPacket(signedPkt)
```

### 7. Proof Delivery

Handled automatically by ChainPorter:
- Generates proof suffix
- Delivers via configured courier
- Tracks delivery status
- Retries on failure

## Directory Structure

```
lightweight-wallet/send/
├── config.go          # Sender configuration  
├── sender.go          # AssetSender wrapper
├── api.go             # Public API
└── sender_test.go     # Tests

# Reuse directly:
../../tapfreighter/    # All transfer logic
```

## Verification

### Integration Tests

```go
func TestAssetSender_SendAsset(t *testing.T) {
    // Setup two wallets
    sender := setupFundedWallet(t)
    receiver := setupWallet(t)

    // Sender mints asset
    asset, err := sender.MintAsset(ctx, &MintRequest{...})
    require.NoError(t, err)

    // Receiver generates address
    addr, err := receiver.NewAddress(ctx, asset.ID())
    require.NoError(t, err)

    // Send asset
    sendResp, err := sender.SendAsset(ctx, &SendRequest{
        Address: addr.Encoded(),
        Amount:  100,
    })
    require.NoError(t, err)

    // Wait for confirmation
    waitForConfirmation(t, sendResp.TxID)

    // Verify receiver got asset
    receivedAssets, err := receiver.ListAssets(ctx)
    require.NoError(t, err)
    require.Len(t, receivedAssets, 1)
}

func TestAssetSender_SplitTransfer(t *testing.T) {
    wallet := setupFundedWallet(t)

    // Mint 1000 units
    asset, _ := wallet.MintAsset(ctx, &MintRequest{Amount: 1000})

    // Send 100 to addr1, keep 900 as change
    addr1, _ := generateAddress(t)
    _, err := wallet.SendAsset(ctx, &SendRequest{
        Address: addr1,
        Amount:  100,
    })
    require.NoError(t, err)

    // Verify change output exists
    ownedAssets, _ := wallet.ListAssets(ctx)
    totalAmount := sumAssetAmounts(ownedAssets, asset.ID())
    require.Equal(t, uint64(900), totalAmount)
}
```

## Integration Points

**Depends On:**
- Task 01 (Chain Backend) - Transaction broadcast
- Task 02 (Wallet) - PSBT funding, signing
- Task 03 (KeyRing) - Key derivation
- Task 04 (Database) - Asset storage, export log
- Task 05 (Proof System) - Proof generation, delivery

**Depended On By:**
- Task 10 (Server) - Exposes sending via RPC
- Task 11 (Go Library) - Embeddable send API

## Success Criteria

- [ ] Can send assets to TAP addresses
- [ ] Coin selection works correctly
- [ ] Split transactions work (send + change)
- [ ] Multiple inputs supported
- [ ] Transaction broadcasts successfully
- [ ] Proofs delivered to recipient
- [ ] Transfer recorded in export log
- [ ] Error handling comprehensive
- [ ] All integration tests pass

## Configuration

```go
type SenderConfig struct {
    // Dependencies
    WalletAnchor  WalletAnchor
    ChainBridge   ChainBridge
    KeyRing       KeyRing
    AssetStore    *tapdb.AssetStore
    ProofArchive  proof.Archiver
    ProofCourier  proof.Courier

    // Transfer settings
    NumConfs      uint32  // Default: 6
    FeeRate       chainfee.SatPerKWeight

    // Proof delivery
    ProofDeliveryTimeout time.Duration  // Default: 5min
}
```

## Error Handling

- Insufficient asset balance
- Invalid TAP address
- Transaction broadcast failure
- Proof delivery failure
- Confirmation timeout
- Reorg handling

## Future Enhancements

- Batch sends (multiple recipients in one tx)
- Fee bumping (RBF)
- Interactive transfers (for LN)
- Atomic swaps
