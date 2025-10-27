package keyring

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/taproot-assets/tapgarden"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/stretchr/testify/require"
)

// TestKeyRing_Interface verifies interface compliance.
func TestKeyRing_Interface(t *testing.T) {
	t.Parallel()

	var _ tapgarden.KeyRing = (*KeyRing)(nil)
}

// TestKeyRing_DeriveNextKey tests sequential key derivation.
func TestKeyRing_DeriveNextKey(t *testing.T) {
	t.Parallel()

	// Create test seed
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	cfg := DefaultConfig(seed, &chaincfg.TestNet3Params)
	kr, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, kr)

	ctx := context.Background()
	keyFamily := keychain.KeyFamily(9) // Taproot Assets key family

	// Derive first key
	key1, err := kr.DeriveNextKey(ctx, keyFamily)
	require.NoError(t, err)
	require.Equal(t, keyFamily, key1.Family)
	require.Equal(t, uint32(0), key1.Index)
	require.NotNil(t, key1.PubKey)

	// Derive second key
	key2, err := kr.DeriveNextKey(ctx, keyFamily)
	require.NoError(t, err)
	require.Equal(t, keyFamily, key2.Family)
	require.Equal(t, uint32(1), key2.Index)
	require.NotNil(t, key2.PubKey)

	// Keys should be different
	require.NotEqual(t,
		key1.PubKey.SerializeCompressed(),
		key2.PubKey.SerializeCompressed(),
	)
}

// TestKeyRing_DeriveNextKey_MultipleFamilies tests derivation across multiple families.
func TestKeyRing_DeriveNextKey_MultipleFamilies(t *testing.T) {
	t.Parallel()

	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}

	cfg := DefaultConfig(seed, &chaincfg.TestNet3Params)
	kr, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Derive keys from different families
	families := []keychain.KeyFamily{0, 1, 9, 100}
	derivedKeys := make(map[keychain.KeyFamily]keychain.KeyDescriptor)

	for _, family := range families {
		key, err := kr.DeriveNextKey(ctx, family)
		require.NoError(t, err)
		require.Equal(t, uint32(0), key.Index, "first key in family should have index 0")
		derivedKeys[family] = key
	}

	// All keys should be different
	pubKeys := make(map[string]bool)
	for _, key := range derivedKeys {
		pubKeyStr := string(key.PubKey.SerializeCompressed())
		require.False(t, pubKeys[pubKeyStr], "duplicate public key found")
		pubKeys[pubKeyStr] = true
	}
}

// TestKeyRing_IsLocalKey tests local key identification.
func TestKeyRing_IsLocalKey(t *testing.T) {
	t.Parallel()

	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 2)
	}

	cfg := DefaultConfig(seed, &chaincfg.TestNet3Params)
	kr, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	keyFamily := keychain.KeyFamily(9)

	// Derive a key
	key1, err := kr.DeriveNextKey(ctx, keyFamily)
	require.NoError(t, err)

	// Should recognize our own key
	require.True(t, kr.IsLocalKey(ctx, key1))

	// Create a random key that we don't control
	privKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	randomKey := keychain.KeyDescriptor{
		KeyLocator: keychain.KeyLocator{
			Family: keychain.KeyFamily(99),
			Index:  0,
		},
		PubKey: privKey.PubKey(),
	}

	// Should not recognize random key
	require.False(t, kr.IsLocalKey(ctx, randomKey))
}

// TestKeyRing_DeriveSharedKey tests ECDH shared key derivation.
func TestKeyRing_DeriveSharedKey(t *testing.T) {
	t.Parallel()

	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 3)
	}

	cfg := DefaultConfig(seed, &chaincfg.TestNet3Params)
	kr, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// Create ephemeral key pair
	ephemeralPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	ephemeralPub := ephemeralPriv.PubKey()

	// Derive shared key without key locator (uses master key)
	sharedKey1, err := kr.DeriveSharedKey(ctx, ephemeralPub, nil)
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, sharedKey1)

	// Derive with specific key locator
	keyLoc := &keychain.KeyLocator{
		Family: keychain.KeyFamily(9),
		Index:  0,
	}
	sharedKey2, err := kr.DeriveSharedKey(ctx, ephemeralPub, keyLoc)
	require.NoError(t, err)
	require.NotEqual(t, [32]byte{}, sharedKey2)

	// Different keys should produce different shared secrets
	require.NotEqual(t, sharedKey1, sharedKey2)

	// Same inputs should produce same output (deterministic)
	sharedKey3, err := kr.DeriveSharedKey(ctx, ephemeralPub, nil)
	require.NoError(t, err)
	require.Equal(t, sharedKey1, sharedKey3)
}

// TestKeyRing_Deterministic tests that key derivation is deterministic.
func TestKeyRing_Deterministic(t *testing.T) {
	t.Parallel()

	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 4)
	}

	// Create two keyrings with same seed
	cfg1 := DefaultConfig(seed, &chaincfg.TestNet3Params)
	kr1, err := New(cfg1)
	require.NoError(t, err)

	cfg2 := DefaultConfig(seed, &chaincfg.TestNet3Params)
	kr2, err := New(cfg2)
	require.NoError(t, err)

	ctx := context.Background()
	keyFamily := keychain.KeyFamily(9)

	// Derive same key from both
	key1, err := kr1.DeriveNextKey(ctx, keyFamily)
	require.NoError(t, err)

	key2, err := kr2.DeriveNextKey(ctx, keyFamily)
	require.NoError(t, err)

	// Should be identical
	require.Equal(t,
		key1.PubKey.SerializeCompressed(),
		key2.PubKey.SerializeCompressed(),
		"same seed should produce same keys",
	)
	require.Equal(t, key1.Index, key2.Index)
	require.Equal(t, key1.Family, key2.Family)
}

// TestKeyRing_Persistence tests key index persistence.
func TestKeyRing_Persistence(t *testing.T) {
	t.Parallel()

	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 5)
	}

	// Use in-memory store for testing
	store := NewMemoryKeyStateStore()

	cfg := DefaultConfig(seed, &chaincfg.TestNet3Params)
	cfg.KeyStateStore = store

	kr, err := New(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	keyFamily := keychain.KeyFamily(9)

	// Derive several keys
	for i := 0; i < 5; i++ {
		_, err := kr.DeriveNextKey(ctx, keyFamily)
		require.NoError(t, err)
	}

	// Check stored index
	index, err := store.GetCurrentIndex(keyFamily)
	require.NoError(t, err)
	require.Equal(t, uint32(5), index, "index should be persisted")

	// Create new keyring with same store
	kr2, err := New(cfg)
	require.NoError(t, err)

	// Next key should continue from index 5
	key, err := kr2.DeriveNextKey(ctx, keyFamily)
	require.NoError(t, err)
	require.Equal(t, uint32(5), key.Index)
}

// TestECDH_Correctness tests ECDH correctness.
func TestECDH_Correctness(t *testing.T) {
	t.Parallel()

	// Alice's keyring
	aliceSeed := make([]byte, 32)
	for i := range aliceSeed {
		aliceSeed[i] = byte(i)
	}
	aliceCfg := DefaultConfig(aliceSeed, &chaincfg.TestNet3Params)
	aliceKR, err := New(aliceCfg)
	require.NoError(t, err)

	// Bob's keypair
	bobPriv, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	bobPub := bobPriv.PubKey()

	ctx := context.Background()

	// Alice derives shared secret using Bob's public key
	aliceShared, err := aliceKR.DeriveSharedKey(ctx, bobPub, nil)
	require.NoError(t, err)

	// Bob derives shared secret using Alice's public key
	aliceMasterKey, err := aliceKR.masterKey.ECPubKey()
	require.NoError(t, err)

	bobShared := btcec.GenerateSharedSecret(bobPriv, aliceMasterKey)
	bobSharedHash := sha256.Sum256(bobShared)

	// Shared secrets should match
	require.Equal(t, aliceShared, bobSharedHash, "ECDH shared secrets should match")
}

// TestMemoryKeyStateStore tests in-memory key state store.
func TestMemoryKeyStateStore(t *testing.T) {
	t.Parallel()

	store := NewMemoryKeyStateStore()
	require.NotNil(t, store)

	family := keychain.KeyFamily(9)

	// Initial index should be 0
	index, err := store.GetCurrentIndex(family)
	require.NoError(t, err)
	require.Equal(t, uint32(0), index)

	// Set index
	err = store.SetCurrentIndex(family, 42)
	require.NoError(t, err)

	// Get index
	index, err = store.GetCurrentIndex(family)
	require.NoError(t, err)
	require.Equal(t, uint32(42), index)

	// Get all indexes
	allIndexes, err := store.GetAllIndexes()
	require.NoError(t, err)
	require.Equal(t, uint32(42), allIndexes[family])
}

// TestFileKeyStateStore tests file-based key state store.
func TestFileKeyStateStore(t *testing.T) {
	t.Parallel()

	tmpFile := t.TempDir() + "/keystate.json"

	store, err := NewFileKeyStateStore(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, store)

	family := keychain.KeyFamily(9)

	// Set index
	err = store.SetCurrentIndex(family, 100)
	require.NoError(t, err)

	// Create new store from same file
	store2, err := NewFileKeyStateStore(tmpFile)
	require.NoError(t, err)

	// Should load persisted index
	index, err := store2.GetCurrentIndex(family)
	require.NoError(t, err)
	require.Equal(t, uint32(100), index)
}
