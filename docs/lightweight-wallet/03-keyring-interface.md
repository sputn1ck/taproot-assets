# Task 03: KeyRing Interface

## Goal

Implement `tapgarden.KeyRing` interface for key derivation and management without LND dependency. Support BIP32/BIP44 HD wallet key derivation for both Bitcoin and Taproot Asset keys.

## Existing Code to Reuse

**Interface Definition:** `tapgarden/interface.go:411-436`
```go
type KeyRing interface {
    DeriveNextKey(context.Context, keychain.KeyFamily) (keychain.KeyDescriptor, error)
    IsLocalKey(context.Context, keychain.KeyDescriptor) bool
    DeriveSharedKey(context.Context, *btcec.PublicKey, *keychain.KeyLocator) ([sha256.Size]byte, error)
}
```

**Types to Reuse:**
- `keychain.KeyFamily` from `lnd`
- `keychain.KeyDescriptor` from `lnd`
- `keychain.KeyLocator` from `lnd`
- `btcec.PrivateKey`, `btcec.PublicKey` from `btcsuite/btcd`

**LND Integration:** `lndservices/signer.go` - Study for key derivation patterns

**Taproot Asset Key Families:**
From LND's keychain, TAP uses custom key families:
- `TaprootAssetsKeyFamily = keychain.KeyFamily(9)` - For asset keys
- Standard Bitcoin key families for wallet keys

## Interface Strategy

Create a simple HD wallet key ring using BIP32 that mimics LND's key derivation hierarchy without the full LND dependency.

**Key Insight:** LND's keychain uses deterministic key derivation. We can replicate this using btcwallet's hdkeychain package.

**Location:** `lightweight-wallet/keyring/`

## Implementation Approach

### 1. Seed and Master Key

Initialize from seed:

```go
import (
    "github.com/btcsuite/btcd/btcutil/hdkeychain"
    "github.com/btcsuite/btcd/chaincfg"
)
```

**Initialization:**
- Accept BIP39 mnemonic or raw seed
- Derive master key: `m/`
- Derive purpose key: `m/1017'` (TAP purpose, or use standard BIP44)
- Store extended keys for each key family

### 2. Key Family Hierarchy

Implement LND-compatible key derivation:

```
m / purpose' / coin_type' / account' / change / address_index
```

**Key Families:**
- Each KeyFamily maps to an account number
- Within each family, derive keys sequentially
- Track next index for each family

**Storage:**
```go
type KeyRing struct {
    masterKey *hdkeychain.ExtendedKey

    // Map of key family to current index
    familyIndexes map[keychain.KeyFamily]uint32

    // Map of derived keys for IsLocalKey checks
    derivedKeys map[string]keychain.KeyDescriptor

    mu sync.RWMutex
}
```

### 3. DeriveNextKey Implementation

Derive next key in key family:

**Algorithm:**
1. Get current index for key family
2. Derive key at: `m / purpose' / coin_type' / key_family' / 0 / index`
3. Increment index for next call
4. Cache derived key descriptor
5. Return KeyDescriptor with key locator and public key

**Key Descriptor:**
```go
keychain.KeyDescriptor{
    KeyLocator: keychain.KeyLocator{
        Family: keyFamily,
        Index:  index,
    },
    PubKey: derivedPubKey,
}
```

### 4. IsLocalKey Implementation

Check if we control a key:

**Algorithm:**
1. Look up key descriptor in cache
2. If not found, try deriving at the given locator
3. Compare public keys
4. Return true if match

**Use Case:**
Tapd uses this to determine if an asset belongs to our wallet.

### 5. DeriveSharedKey Implementation

ECDH key derivation:

**Algorithm:**
1. If key locator provided, derive our private key at that location
2. Otherwise, use node identity key (root key)
3. Perform ECDH: `sharedSecret = ourPrivKey * theirPubKey`
4. Hash result with SHA256
5. Return 32-byte shared secret

**Use Case:**
Used for encrypted communication, proof delivery authentication.

### 6. Taproot Asset Key Derivation

Special handling for Taproot Asset keys:

**Taproot Asset Key Family:** Usually `KeyFamily(9)` or configurable

**Script Keys vs. Internal Keys:**
- Script keys: Used for asset ownership (P2TR script keys)
- Internal keys: Used for anchor outputs (Bitcoin P2TR)

Both can be derived from same KeyRing using different families.

### 7. Key Persistence

Persist key state to prevent reuse:

**What to Store:**
- Current index for each key family
- Optionally: derived public keys for quick lookup

**Storage Options:**
- SQLite database (reuse tapdb)
- Separate key state file
- Encrypted with wallet password

### 8. Thread Safety

All methods must be thread-safe:
- Use `sync.RWMutex` for state access
- Atomic index increments
- Safe concurrent DeriveNextKey calls

## Directory Structure

```
lightweight-wallet/keyring/
├── keyring.go         # Main KeyRing implementation
├── derivation.go      # BIP32 derivation logic
├── shared_key.go      # ECDH implementation
├── storage.go         # Key index persistence
├── types.go           # Type adapters if needed
├── keyring_test.go    # Unit tests
└── integration_test.go # Integration tests
```

## Verification

### Unit Tests

Test key derivation:

```go
func TestKeyRing_DeriveNextKey(t *testing.T) {
    seed := [32]byte{0x01, 0x02, ...} // Test seed
    kr := NewKeyRing(seed, chaincfg.TestNet3Params)

    // Derive first key in family
    keyFamily := keychain.KeyFamily(9)
    key1, err := kr.DeriveNextKey(ctx, keyFamily)
    require.NoError(t, err)
    require.Equal(t, uint32(0), key1.Index)

    // Derive second key
    key2, err := kr.DeriveNextKey(ctx, keyFamily)
    require.NoError(t, err)
    require.Equal(t, uint32(1), key2.Index)

    // Keys should be different
    require.NotEqual(t, key1.PubKey.SerializeCompressed(),
                      key2.PubKey.SerializeCompressed())
}
```

Test all methods:
- ✅ DeriveNextKey (sequential derivation)
- ✅ DeriveNextKey (multiple families)
- ✅ IsLocalKey (known keys return true)
- ✅ IsLocalKey (unknown keys return false)
- ✅ DeriveSharedKey (ECDH correctness)
- ✅ Thread safety (concurrent derivations)
- ✅ Key persistence (restart preserves state)

### Deterministic Tests

Verify derivation is deterministic:

```go
func TestKeyRing_Deterministic(t *testing.T) {
    seed := [32]byte{0x01, 0x02, ...}

    // Create two keyrings with same seed
    kr1 := NewKeyRing(seed, chaincfg.TestNet3Params)
    kr2 := NewKeyRing(seed, chaincfg.TestNet3Params)

    // Derive keys
    key1, _ := kr1.DeriveNextKey(ctx, keychain.KeyFamily(9))
    key2, _ := kr2.DeriveNextKey(ctx, keychain.KeyFamily(9))

    // Should be identical
    require.Equal(t, key1.PubKey.SerializeCompressed(),
                   key2.PubKey.SerializeCompressed())
}
```

### Integration Tests

Test with real Taproot Asset operations:

```go
func TestKeyRing_WithAssetMinting(t *testing.T) {
    kr := setupTestKeyRing(t)

    // Derive script key for asset
    scriptKey, err := kr.DeriveNextKey(ctx, TaprootAssetsKeyFamily)
    require.NoError(t, err)

    // Create asset with this script key
    asset := createTestAsset(t, scriptKey.PubKey)

    // Verify we recognize the key
    require.True(t, kr.IsLocalKey(ctx, scriptKey))
}
```

### Interface Compliance Test

```go
func TestKeyRing_ImplementsInterface(t *testing.T) {
    var _ tapgarden.KeyRing = (*KeyRing)(nil)
}
```

## Integration Points

**Depends On:**
- None (foundation layer, but may use Task 04 for key state persistence)

**Depended On By:**
- Task 02 (Wallet) - May use for deriving wallet keys
- Task 06 (Asset Minting) - Uses for deriving script keys
- Task 07 (Asset Sending) - Uses for deriving keys
- Task 08 (Asset Receiving) - Uses for key checks

## Success Criteria

- [ ] All KeyRing interface methods implemented
- [ ] All unit tests pass with >90% coverage
- [ ] Key derivation is deterministic (same seed → same keys)
- [ ] Multiple key families supported
- [ ] IsLocalKey correctly identifies our keys
- [ ] DeriveSharedKey produces correct ECDH result
- [ ] Thread-safe concurrent key derivation
- [ ] Key indexes persist across restarts
- [ ] Compatible with LND key derivation (same seed → same keys)
- [ ] Supports both testnet and mainnet

## Configuration

Add to lightweight wallet config:

```go
type KeyRingConfig struct {
    // Seed for HD wallet (BIP39 mnemonic or raw bytes)
    Seed []byte

    // Network parameters
    NetParams *chaincfg.Params

    // Derivation path customization
    Purpose   uint32  // Default: 1017 (TAP) or 86 (BIP86)
    CoinType  uint32  // Default: 0 (Bitcoin)

    // Key state storage path
    KeyDBPath string
}
```

## BIP32 Derivation Path

Use TAP-specific or standard BIP86 derivation:

**Option 1: Custom TAP path**
```
m / 1017' / 0' / key_family' / 0 / index
```

**Option 2: BIP86 taproot path**
```
m / 86' / 0' / key_family' / 0 / index
```

**Recommendation:** Use custom TAP path (1017) to avoid conflicts with standard wallet keys.

## Key Index Management

Track indexes per key family:

```go
type KeyIndexStorage interface {
    GetCurrentIndex(family keychain.KeyFamily) (uint32, error)
    SetCurrentIndex(family keychain.KeyFamily, index uint32) error
}
```

Persist to database or file:
```json
{
  "key_families": {
    "9": 42,    // Taproot Asset keys
    "0": 10,    // Wallet keys
    "6": 5      // Other family
  }
}
```

## Shared Key Derivation (ECDH)

Implement Diffie-Hellman:

```go
func (kr *KeyRing) DeriveSharedKey(
    ctx context.Context,
    ephemeralPubKey *btcec.PublicKey,
    keyLoc *keychain.KeyLocator,
) ([sha256.Size]byte, error) {
    // Get our private key
    var privKey *btcec.PrivateKey
    if keyLoc != nil {
        privKey = kr.derivePrivateKey(*keyLoc)
    } else {
        privKey = kr.nodePrivateKey
    }

    // ECDH: sharedPoint = privKey * ephemeralPubKey
    sharedPoint := btcec.GenerateSharedSecret(privKey, ephemeralPubKey)

    // Hash to get 32-byte key
    return sha256.Sum256(sharedPoint), nil
}
```

## Security Considerations

- Master seed must be stored securely (encrypted)
- Private keys should never be exported
- Implement secure key deletion
- Consider hardware wallet support
- Use constant-time comparisons for key matching

## Backup and Recovery

Support wallet recovery from seed:

```go
func NewKeyRingFromMnemonic(mnemonic string, params *chaincfg.Params) (*KeyRing, error)
func NewKeyRingFromSeed(seed []byte, params *chaincfg.Params) (*KeyRing, error)
```

Recovery process:
1. Restore from mnemonic/seed
2. Scan blockchain for used keys (gap limit)
3. Restore key indexes
4. Sync with database

## Gap Limit

Implement BIP44 gap limit for key discovery:

- Default gap limit: 20
- Scan ahead to find all used keys
- Stop when gap limit consecutive unused keys found

## Error Handling

Handle these error cases:
- Invalid seed
- Derivation path too deep
- Key family not supported
- Index overflow (uint32 max)
- Key not found (IsLocalKey)

## Performance Considerations

- Cache derived keys to avoid re-derivation
- Lazy key derivation (only derive when requested)
- Batch key derivation if needed
- Index lookup should be O(1)

## Future Enhancements

- Hardware wallet integration (sign with Ledger/Trezor)
- Multi-signature key management
- Key rotation policies
- Hierarchical deterministic groups
- BIP39 passphrase support
- Key backup encryption
