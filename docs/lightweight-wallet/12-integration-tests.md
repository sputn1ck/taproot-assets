# Task 14: Integration Test Suite

## Goal

Comprehensive integration tests covering all workflows from day one.

## Test Structure

```
lightweight-wallet/itest/
├── harness.go           # Test harness setup
├── harness_test.go      # Harness tests
├── mint_test.go         # Minting tests
├── send_test.go         # Sending tests
├── receive_test.go      # Receiving tests
├── universe_test.go     # Universe sync tests
├── proof_test.go        # Proof tests
└── e2e_test.go          # End-to-end scenarios
```

## Test Harness

```go
type Harness struct {
    t           *testing.T
    bitcoind    *BitcoindHarness
    mempoolMock *MempoolMock
    wallets     []*lwtapd.Client
}

func NewHarness(t *testing.T) *Harness {
    // Start Bitcoin regtest
    bitcoind := startBitcoind(t)
    
    // Start mock mempool.space API
    mempoolMock := startMempoolMock(t, bitcoind)
    
    return &Harness{
        t:           t,
        bitcoind:    bitcoind,
        mempoolMock: mempoolMock,
        wallets:     make([]*lwtapd.Client, 0),
    }
}

func (h *Harness) NewWallet() *lwtapd.Client {
    wallet, err := lwtapd.New(&lwtapd.Config{
        Network:    "regtest",
        DBPath:     h.t.TempDir() + "/tapd.db",
        MempoolURL: h.mempoolMock.URL(),
        WalletSeed: randomSeed(),
    })
    require.NoError(h.t, err)
    
    h.wallets = append(h.wallets, wallet)
    return wallet
}
```

## Test Categories

### 1. Minting Tests

```go
func TestMint_SingleAsset(t *testing.T)
func TestMint_BatchMultipleAssets(t *testing.T)
func TestMint_WithMetadata(t *testing.T)
func TestMint_Collectible(t *testing.T)
```

### 2. Transfer Tests

```go
func TestSend_SimpleTransfer(t *testing.T)
func TestSend_WithSplit(t *testing.T)
func TestSend_MultipleInputs(t *testing.T)
func TestSend_ToMultipleRecipients(t *testing.T)
```

### 3. Receiving Tests

```go
func TestReceive_DirectTransfer(t *testing.T)
func TestReceive_WithProofDelivery(t *testing.T)
func TestReceive_ManualProofImport(t *testing.T)
```

### 4. End-to-End Tests

```go
func TestE2E_MintSendReceive(t *testing.T) {
    h := NewHarness(t)
    defer h.Cleanup()
    
    // Create two wallets
    alice := h.NewWallet()
    bob := h.NewWallet()
    
    // Fund Alice's wallet
    h.FundWallet(alice, 1*btcutil.SatoshiPerBitcoin)
    
    // Alice mints asset
    asset, err := alice.MintAsset(ctx, &lwtapd.MintRequest{
        Name:   "ALICE_COIN",
        Amount: 1000,
    })
    require.NoError(t, err)
    
    // Mine blocks
    h.MineBlocks(6)
    
    // Bob generates address
    addr, err := bob.NewAddress(ctx, asset.ID(), 100)
    require.NoError(t, err)
    
    // Alice sends to Bob
    _, err = alice.SendAsset(ctx, &lwtapd.SendRequest{
        Address: addr.Encoded,
        Amount:  100,
    })
    require.NoError(t, err)
    
    // Mine blocks
    h.MineBlocks(6)
    
    // Wait for Bob to receive
    h.WaitForAsset(bob, asset.ID())
    
    // Verify Bob has asset
    bobAssets, err := bob.ListAssets(ctx)
    require.NoError(t, err)
    require.Len(t, bobAssets, 1)
    require.Equal(t, uint64(100), bobAssets[0].Amount)
    
    // Verify Alice has change
    aliceAssets, err := alice.ListAssets(ctx)
    require.NoError(t, err)
    totalAmount := sumAssetAmounts(aliceAssets, asset.ID())
    require.Equal(t, uint64(900), totalAmount)
}
```

## Running Tests

```bash
# Run all integration tests
go test -v ./lightweight-wallet/itest/...

# Run specific test
go test -v ./lightweight-wallet/itest/ -run TestE2E_MintSendReceive

# Skip slow tests
go test -v -short ./lightweight-wallet/itest/...
```

## Success Criteria

- [ ] All integration tests pass
- [ ] Coverage >80% for integration paths
- [ ] Tests run in CI/CD
- [ ] Tests are deterministic
- [ ] Fast execution (<5 min total)

