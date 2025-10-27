# Task 05: Proof System Integration

## Goal

Integrate existing tapd proof system (verification, storage, import/export) with the lightweight wallet without modification. The proof system is a core component that must work identically to full tapd.

## Existing Code to Reuse

**ALL OF IT** - The proof system should be reused as-is:

**Core Proof Types:** `proof/` directory
- `proof/proof.go` - Main proof struct
- `proof/mint.go` - Minting proofs
- `proof/tx.go` - Transaction proofs
- `proof/taproot.go` - Taproot proofs
- `proof/verifier.go` - Proof verification logic
- `proof/archive.go` - Proof storage

**Proof Storage:**
- `proof/file.go` - File-based proof archiver
- `tapdb/proof.go` - Database proof storage

**Proof Delivery:**
- `proof/courier.go` - Proof courier interface
- `proof/hashmail.go` - HashMail courier
- `proof/universe.go` - Universe courier

**DO NOT:**
- Modify any proof verification logic
- Change proof formats
- Alter cryptographic operations
- Fork the proof package

## Interface Strategy

Wire up existing proof components to work with lightweight implementations of ChainBridge, WalletAnchor, and database.

**Key Insight:** Proof system already has clean interfaces - just provide implementations.

**Location:** Reuse `proof/` package directly, create lightweight wiring in `lightweight-wallet/proof_config.go`

## Implementation Approach

### 1. Understand Proof Flow

**Minting Proof Flow:**
1. Create asset → Generate genesis proof
2. Broadcast transaction → Add transaction proof
3. Confirm on-chain → Add block proof
4. Store proof to archive

**Transfer Proof Flow:**
1. Receive asset → Get proof from courier
2. Verify proof chain
3. Import to local archive
4. Acknowledge receipt

**Sending Flow:**
1. Build transfer transaction
2. Generate proof suffix
3. Broadcast transaction
4. Deliver proof to recipient

### 2. Proof Verifier Setup

Reuse existing verifier:

```go
import "github.com/lightninglabs/taproot-assets/proof"

verifier := proof.NewDefaultMerkleVerifier()

// For full proof verification with chain lookups:
headerVerifier := proof.GenHeaderVerifier(ctx, chainBridge)

fullVerifier := proof.NewV0ProofVerifier(&proof.V0ProofVerifierConfig{
    MerkleVerifier: verifier,
    HeaderVerifier: headerVerifier,
    ChainLookupGen: chainBridge,  // Uses your Task 01 implementation
})
```

**Key Point:** `ChainBridge` from Task 01 implements `proof.ChainLookupGenerator`, so it just works.

### 3. Proof Archive Setup

Use multi-archive setup (database + file):

```go
import (
    "github.com/lightninglabs/taproot-assets/proof"
    "github.com/lightninglabs/taproot-assets/tapdb"
)

// File archive for large proof files
fileArchive := proof.NewFileArchiver(proofDirPath)

// Database archive for metadata
dbArchive := tapdb.NewProofArchive(assetStore)

// Multi-archive combines both
multiArchive := proof.NewMultiArchiver(
    &proof.MultiArchiverConfig{
        NotifyArchive:  dbArchive,     // Notifies on new proofs
        PrimaryArchive: fileArchive,    // Stores actual proof bytes
        SecondaryArchive: dbArchive,    // Stores metadata
    },
)
```

### 4. Proof Courier Setup

Configure proof delivery:

```go
// HashMail courier for direct peer delivery
hashmailCourier := proof.NewHashMailCourier(&proof.HashMailCourierConfig{
    ReceiverAckTimeout: 5 * time.Minute,
})

// Universe courier for federation delivery
universeClient := universe.NewClient(universeAddr)
universeCourier := proof.NewUniverseRpcCourier(&proof.UniverseRpcCourierConfig{
    UniverseAddrBook: universeClient,
})

// Dispatcher routes proofs to appropriate courier
dispatcher := proof.NewCourierDispatcher(&proof.CourierDispatcherConfig{
    HashMailCourier: hashmailCourier,
    UniverseCourier: universeCourier,
})
```

### 5. Proof Import/Export

Reuse existing functions:

```go
// Import proof file
proofFile, err := proof.NewFile(...)
annotatedProof := &proof.AnnotatedProof{
    Locator: proof.Locator{...},
    Blob:    proofFile.RawProofBytes(),
}

err = multiArchive.ImportProofs(ctx, annotatedProof)

// Export proof
proofBlob, err := multiArchive.FetchProof(ctx, locator)

// Verify proof
_, err = fullVerifier.Verify(ctx, proofBlob.Blob, nil)
```

### 6. Proof Generation

Reuse existing proof building:

```go
import "github.com/lightninglabs/taproot-assets/proof"

// Build genesis proof (during minting)
genesisProof, err := proof.CreateMintingProof(
    &proof.BaseProofParams{
        Block:   block,
        Tx:      genesisTx,
        TxIndex: txIndex,
        // ...
    },
    asset,
)

// Build transfer proof (during sending)
transferProof, err := proof.CreateTransitionProof(
    prevProof,
    &proof.BaseProofParams{...},
    newAsset,
    inclusionProof,
)
```

## Directory Structure

```
lightweight-wallet/
├── proof_config.go      # Proof system configuration
└── proof_test.go        # Integration tests

# Reuse these directly (no copies):
../../proof/              # All proof logic
../../tapdb/proof.go      # Database proof storage
```

## Verification

### Unit Tests

Test proof system wiring:

```go
func TestProofSystem_VerifierSetup(t *testing.T) {
    chainBridge := setupTestChainBridge(t)

    verifier := setupProofVerifier(chainBridge)
    require.NotNil(t, verifier)

    // Verify can verify a known-good proof
    testProof := loadTestProof(t)
    _, err := verifier.Verify(ctx, testProof, nil)
    require.NoError(t, err)
}
```

### Integration Tests

Test full proof workflows:

```go
func TestProofSystem_MintAndVerify(t *testing.T) {
    // Setup lightweight wallet
    wallet := setupLightweightWallet(t)

    // Mint asset
    asset, err := wallet.MintAsset(ctx, mintParams)
    require.NoError(t, err)

    // Retrieve proof
    proofLocator := proof.Locator{
        AssetID:   &asset.ID,
        ScriptKey: *asset.ScriptKey.PubKey,
    }

    proofBlob, err := wallet.proofArchive.FetchProof(ctx, proofLocator)
    require.NoError(t, err)

    // Verify proof
    _, err = wallet.proofVerifier.Verify(ctx, proofBlob, nil)
    require.NoError(t, err)
}

func TestProofSystem_ImportExport(t *testing.T) {
    wallet := setupLightweightWallet(t)

    // Export proof
    exported, err := wallet.ExportProof(ctx, locator)
    require.NoError(t, err)

    // Create second wallet
    wallet2 := setupLightweightWallet(t)

    // Import proof
    err = wallet2.ImportProof(ctx, exported)
    require.NoError(t, err)

    // Verify imported proof exists
    _, err = wallet2.proofArchive.FetchProof(ctx, locator)
    require.NoError(t, err)
}
```

### Proof Verification Tests

```go
func TestProofSystem_VerifyChain(t *testing.T) {
    // Create chain of proofs: genesis → transfer1 → transfer2
    // Verify each proof in chain
    // Verify full chain validation
}

func TestProofSystem_DetectInvalidProof(t *testing.T) {
    // Corrupt proof
    // Verify verification fails
    // Test various corruption types
}
```

### Proof Delivery Tests

```go
func TestProofSystem_CourierDelivery(t *testing.T) {
    // Setup sender and receiver
    // Send asset with proof delivery
    // Verify receiver gets proof via courier
    // Verify acknowledgment
}
```

## Integration Points

**Depends On:**
- Task 01 (Chain Backend) - proof.ChainLookupGenerator interface
- Task 04 (Database) - Proof storage in tapdb

**Depended On By:**
- Task 06 (Asset Minting) - Generates minting proofs
- Task 07 (Asset Sending) - Generates transfer proofs, delivers proofs
- Task 08 (Asset Receiving) - Imports and verifies proofs
- Task 09 (Universe) - Proof sync with federation

## Success Criteria

- [ ] Proof verifier works with lightweight ChainBridge
- [ ] Can generate minting proofs
- [ ] Can generate transfer proofs
- [ ] Can verify proof chains
- [ ] Proof import/export works
- [ ] File archive stores proofs correctly
- [ ] Database archive indexes proofs
- [ ] Multi-archive coordinates both stores
- [ ] Proof courier can deliver proofs
- [ ] All integration tests pass
- [ ] No modifications to proof/ package
- [ ] Compatible with proofs from full tapd

## Configuration

```go
type ProofConfig struct {
    // File storage
    ProofDir string  // Directory for proof files

    // Courier
    HashMailAddr      string
    UniverseAddrs     []string
    CourierTimeout    time.Duration

    // Verification
    SkipChainLookup   bool  // For testing

    // Archive
    EnableFileArchive bool  // Default: true
    EnableDBArchive   bool  // Default: true
}

func SetupProofSystem(
    cfg ProofConfig,
    chainBridge ChainBridge,
    assetStore *tapdb.AssetStore,
) (*ProofSystem, error) {
    // Setup verifier
    verifier := setupProofVerifier(chainBridge, cfg)

    // Setup archive
    archive := setupProofArchive(cfg, assetStore)

    // Setup courier
    courier := setupProofCourier(cfg)

    return &ProofSystem{
        Verifier: verifier,
        Archive:  archive,
        Courier:  courier,
    }, nil
}
```

## Proof File Management

**File Naming:** Reuse existing convention
```
proofs/
├── <asset_id>_<script_key>.proof
└── <asset_id>_<script_key>_<anchor_hash>.proof
```

**File Size Considerations:**
- Proof chains grow with each transfer
- Implement pruning for very long chains
- Compress proof files (optional)

## Proof Verification Optimization

**Caching:**
```go
// Cache verified proofs to avoid re-verification
type VerifiedProofCache struct {
    cache *lru.Cache // proof hash → verification result
}
```

**Batch Verification:**
```go
// Verify multiple proofs in parallel
func (v *Verifier) VerifyBatch(ctx context.Context, proofs [][]byte) ([]error, error)
```

## Proof Courier Strategies

**1. HashMail (Direct P2P):**
- For known recipients
- Requires recipient to be online or have mailbox server
- Fast delivery

**2. Universe (Federation):**
- Upload to universe server
- Recipient fetches when ready
- Works offline
- Public proofs

**3. Custom:**
- Implement custom proof.Courier interface
- Could use email, messaging apps, QR codes, etc.

## Error Handling

Handle these error cases:
- Proof verification failure (invalid proof)
- Chain lookup failures (chain backend down)
- Missing previous proofs in chain
- Corrupted proof files
- Proof delivery timeout
- Archive storage failures

## Security Considerations

- Proof verification must be cryptographically sound
- Don't trust unverified proofs
- Validate all proof signatures
- Verify merkle proofs
- Check block headers against chain

## Performance Considerations

- Large proof files (can be MBs for long chains)
- Verification can be CPU-intensive
- Parallel proof verification
- Cache verified proofs
- Incremental verification (verify new parts only)

## Proof Compression

Optional optimization:

```go
import "compress/gzip"

func CompressProof(proof []byte) ([]byte, error) {
    // gzip compression
}

func DecompressProof(compressed []byte) ([]byte, error) {
    // gzip decompression
}
```

Can reduce proof size by 60-80%.

## Future Enhancements

- Proof pruning (remove old intermediate proofs)
- Proof compression
- Proof aggregation (combine multiple proofs)
- SPV-style proof verification (don't need full blocks)
- Distributed proof storage (IPFS, etc.)
- Proof streaming (for very large proofs)
