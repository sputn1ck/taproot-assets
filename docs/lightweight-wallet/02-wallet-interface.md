# Task 02: Wallet Interface

## Goal

Implement `tapgarden.WalletAnchor` and `tapfreighter.WalletAnchor` interfaces using btcwallet libraries to replace LND's wallet functionality for PSBT operations, signing, and UTXO management.

## Existing Code to Reuse

**Interface Definition:** `tapgarden/interface.go:366-409`
```go
type WalletAnchor interface {
    FundPsbt(ctx context.Context, packet *psbt.Packet, minConfs uint32,
        feeRate chainfee.SatPerKWeight, changeIdx int32) (*tapsend.FundedPsbt, error)
    SignAndFinalizePsbt(context.Context, *psbt.Packet) (*psbt.Packet, error)
    ImportTaprootOutput(context.Context, *btcec.PublicKey) (btcutil.Address, error)
    UnlockInput(context.Context, wire.OutPoint) error
    ListUnspentImportScripts(ctx context.Context) ([]*lnwallet.Utxo, error)
    ListTransactions(ctx context.Context, startHeight, endHeight int32,
        account string) ([]lndclient.Transaction, error)
    SubscribeTransactions(context.Context) (<-chan lndclient.Transaction,
        <-chan error, error)
    MinRelayFee(ctx context.Context) (chainfee.SatPerKWeight, error)
}
```

**Extended Interface:** `tapfreighter/interface.go:620-627`
```go
type WalletAnchor interface {
    tapgarden.WalletAnchor
    SignPsbt(ctx context.Context, packet *psbt.Packet) (*psbt.Packet, error)
}
```

**Types to Reuse:**
- `psbt.Packet` from `btcsuite/btcd/btcutil/psbt`
- `tapsend.FundedPsbt` from `taproot-assets`
- `wire.OutPoint`, `wire.MsgTx` from `btcsuite/btcd`
- `chainfee.SatPerKWeight` from `lnd`
- `lnwallet.Utxo` from `lnd`

**Existing Mock:** `tapgarden/mock.go:312-437` (MockWalletAnchor)

## Interface Strategy

Use `btcwallet` as the underlying wallet engine, creating an adapter layer that implements the WalletAnchor interface.

**Key Decision:** This wallet manages ONLY the Bitcoin UTXOs used for anchoring Taproot Asset commitments. It does NOT manage Taproot Assets themselves (that's handled by tapdb).

**Location:** `lightweight-wallet/wallet/btcwallet/`

## Implementation Approach

### 1. Wallet Initialization

Use btcwallet's native capabilities:

```go
import (
    "github.com/btcsuite/btcwallet/wallet"
    "github.com/btcsuite/btcwallet/walletdb"
    "github.com/btcsuite/btcwallet/chain"
)
```

**Initialization Steps:**
1. Create or open wallet database
2. Load wallet with seed/password
3. Connect to chain backend (from Task 01)
4. Sync wallet to current height
5. Start transaction notification system

### 2. PSBT Funding

Implement `FundPsbt()`:

**Requirements:**
- Select UTXOs to fund the PSBT
- Add inputs to PSBT packet
- Add change output if necessary
- Respect `minConfs` parameter
- Target specified `feeRate`
- Use `changeIdx` if provided, or append change output

**btcwallet Methods to Use:**
- `wallet.FundPsbt()` - btcwallet has native PSBT funding
- `wallet.SelectInputs()` - Manual coin selection
- Calculate fees using `txrules` package

**Edge Cases:**
- Insufficient funds
- Dust outputs
- Fee rate too low (below min relay fee)
- Change output creation

### 3. PSBT Signing

Implement `SignAndFinalizePsbt()` and `SignPsbt()`:

**Requirements:**
- Sign all inputs the wallet controls
- Add witness data
- Finalize PSBT (combine signatures, create final witness)
- Extract final transaction

**btcwallet Methods:**
- `wallet.SignTransaction()` - Sign a transaction
- `psbt.Finalize()` - Finalize PSBT
- Handle taproot (P2TR) signing

**Difference:**
- `SignAndFinalizePsbt()`: Sign + Finalize + Extract final tx
- `SignPsbt()`: Sign only (for multi-sig workflows)

### 4. Taproot Output Import

Implement `ImportTaprootOutput()`:

**Requirements:**
- Import a P2TR output so wallet watches for it
- Return the taproot address
- Enable spending from this output later

**btcwallet Methods:**
- `wallet.ImportTaprootScript()` or similar
- May need to implement custom watch-only import

**Use Case:**
When minting or receiving assets, we need the wallet to watch the anchor output so we can spend it later.

### 5. UTXO Management

Implement `ListUnspentImportScripts()`:

**Requirements:**
- List all UTXOs from imported taproot scripts
- Return as `lnwallet.Utxo` structs
- Filter for confirmed UTXOs

**btcwallet Methods:**
- `wallet.ListUnspent()` - List all unspent outputs
- Filter for imported scripts

Implement `UnlockInput()`:

**Requirements:**
- Unlock a previously locked UTXO
- Used when abandoning a transaction

**Implementation:**
- Maintain internal lock map
- Prevent double-spending during transaction construction

### 6. Transaction Monitoring

Implement `ListTransactions()`:

**Requirements:**
- Query transactions within block height range
- Include unconfirmed if `endHeight = -1`
- Return as `lndclient.Transaction` (may need adapter)

**btcwallet Methods:**
- `wallet.ListTransactionDetails()`
- Convert to lndclient.Transaction format

Implement `SubscribeTransactions()`:

**Requirements:**
- Stream new transactions as they arrive
- Return channel of transactions

**Implementation:**
- Hook into btcwallet notification system
- Or poll ListTransactions periodically
- Send new transactions to channel

### 7. Fee Estimation

Implement `MinRelayFee()`:

**Requirements:**
- Return network's minimum relay fee

**Implementation:**
- Query from chain backend (Task 01)
- Or use btcwallet's internal fee estimator
- Default to 1 sat/vbyte if unavailable

## Directory Structure

```
lightweight-wallet/wallet/btcwallet/
├── wallet.go          # Main wallet struct and initialization
├── psbt.go            # PSBT funding and signing
├── utxo.go            # UTXO management and locking
├── transactions.go    # Transaction monitoring
├── import.go          # Taproot output import
├── types.go           # Type adapters (lndclient.Transaction, etc.)
├── wallet_test.go     # Unit tests
└── integration_test.go # Integration tests
```

## Verification

### Unit Tests

Test each method with mocked chain backend:

```go
func TestBTCWallet_FundPsbt(t *testing.T) {
    // Create test wallet with known UTXOs
    w := setupTestWallet(t)

    // Create empty PSBT
    packet, err := psbt.New(...)
    require.NoError(t, err)

    // Fund PSBT
    funded, err := w.FundPsbt(ctx, packet, 1, 1000, -1)
    require.NoError(t, err)

    // Verify inputs added
    require.Greater(t, len(funded.Pkt.Inputs), 0)

    // Verify fees are reasonable
    // ...
}
```

Test all methods:
- ✅ FundPsbt (various fee rates, change scenarios)
- ✅ SignAndFinalizePsbt (P2TR inputs)
- ✅ SignPsbt (partial signing)
- ✅ ImportTaprootOutput
- ✅ UnlockInput
- ✅ ListUnspentImportScripts
- ✅ ListTransactions
- ✅ SubscribeTransactions
- ✅ MinRelayFee

### Integration Tests

Test with real Bitcoin regtest:

```go
func TestBTCWallet_CompleteFlow(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Setup regtest Bitcoin node
    btcNode := setupRegtest(t)

    // Create wallet
    w := NewBTCWallet(...)

    // Fund wallet with regtest coins
    fundWallet(t, btcNode, w, 10*btcutil.SatoshiPerBitcoin)

    // Create and fund PSBT
    psbtPacket := createTestPsbt(t)
    funded, err := w.FundPsbt(ctx, psbtPacket, ...)
    require.NoError(t, err)

    // Sign and finalize
    signed, err := w.SignAndFinalizePsbt(ctx, funded.Pkt)
    require.NoError(t, err)

    // Extract and broadcast transaction
    finalTx, err := psbt.Extract(signed)
    require.NoError(t, err)

    // Verify transaction is valid
    // ...
}
```

### Interface Compliance Test

```go
func TestBTCWallet_ImplementsWalletAnchor(t *testing.T) {
    var _ tapgarden.WalletAnchor = (*BTCWallet)(nil)
    var _ tapfreighter.WalletAnchor = (*BTCWallet)(nil)
}
```

## Integration Points

**Depends On:**
- Task 01 (Chain Backend) - Uses for transaction broadcasting and monitoring
- Task 03 (KeyRing) - Uses for key derivation

**Depended On By:**
- Task 06 (Asset Minting) - Uses WalletAnchor for funding genesis tx
- Task 07 (Asset Sending) - Uses WalletAnchor for funding transfer tx
- Task 08 (Asset Receiving) - Uses for importing taproot outputs

## Success Criteria

- [ ] All WalletAnchor interface methods implemented
- [ ] All unit tests pass with >85% coverage
- [ ] Integration tests work with regtest Bitcoin node
- [ ] Can fund PSBTs with various fee rates
- [ ] Can sign taproot (P2TR) inputs correctly
- [ ] Change handling works correctly
- [ ] UTXO locking prevents double-spending
- [ ] Transaction monitoring delivers new transactions
- [ ] Imported taproot outputs are tracked correctly
- [ ] Fee estimation is accurate
- [ ] Error handling is comprehensive

## Configuration

Add to lightweight wallet config:

```go
type WalletConfig struct {
    // Wallet database path
    DBPath string

    // Network (mainnet, testnet, regtest)
    Network string

    // Wallet password (for encryption)
    Password string

    // Seed (for wallet recovery)
    Seed []byte

    // Min confirmations for coin selection
    MinConfs uint32  // Default: 1

    // Change address type
    ChangeAddressType string  // "p2tr", "p2wpkh", etc.
}
```

## UTXO Lock Management

Implement UTXO locking to prevent double-spending during concurrent operations:

```go
type LockManager struct {
    locks map[wire.OutPoint]time.Time
    mu    sync.RWMutex
}

func (lm *LockManager) LockUTXO(op wire.OutPoint, duration time.Duration) error
func (lm *LockManager) UnlockUTXO(op wire.OutPoint) error
func (lm *LockManager) IsLocked(op wire.OutPoint) bool
func (lm *LockManager) CleanupExpired()
```

Locks expire automatically after duration (default: 10 minutes).

## Type Adapters

btcwallet and lndclient use slightly different types. Create adapters:

```go
// Convert btcwallet UTXO to lnwallet.Utxo
func adaptUTXO(btcwalletUTXO wallet.Utxo) *lnwallet.Utxo

// Convert btcwallet transaction to lndclient.Transaction
func adaptTransaction(btcwalletTx wallet.TransactionDetails) lndclient.Transaction
```

## Error Handling

Handle these error cases:
- Insufficient funds
- Wallet locked (password needed)
- Invalid PSBT format
- Signing failures (unknown keys)
- Fee rate too low
- UTXO already locked
- Transaction broadcast failures

## Security Considerations

- Wallet database should be encrypted
- Private keys never leave wallet
- Implement password retry limits
- Auto-lock wallet after inactivity
- Seed backup and recovery

## Performance Considerations

- UTXO selection can be slow with many UTXOs (implement efficient coin selection)
- Transaction monitoring should not poll too frequently
- Cache wallet balance
- Batch UTXO updates

## Future Enhancements

- Hardware wallet support (Ledger, Trezor)
- Multi-signature wallets
- HD wallet full BIP44 support
- Watch-only wallets
- Custom coin selection strategies
- Fee bumping (RBF) support
