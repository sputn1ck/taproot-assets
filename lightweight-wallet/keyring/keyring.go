package keyring

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightninglabs/taproot-assets/tapgarden"
	"github.com/lightningnetwork/lnd/keychain"
)

const (
	// DefaultGapLimit is the default gap limit for key derivation.
	DefaultGapLimit = 20

	// TaprootAssetsPurpose is the BIP43 purpose for Taproot Assets.
	// Using 1017 (TAP = 20-01-16 = 1017)
	TaprootAssetsPurpose = 1017

	// DefaultCoinType is Bitcoin (0).
	DefaultCoinType = 0
)

// Config holds the configuration for the KeyRing.
type Config struct {
	// NetParams is the network parameters.
	NetParams *chaincfg.Params

	// Seed is the wallet seed for key derivation.
	Seed []byte

	// Purpose is the BIP43 purpose field.
	// Default: 1017 (Taproot Assets)
	Purpose uint32

	// CoinType is the BIP44 coin type.
	// Default: 0 (Bitcoin)
	CoinType uint32

	// KeyStateStore is optional storage for key indexes.
	// If nil, indexes are kept in memory only.
	KeyStateStore KeyStateStore
}

// DefaultConfig returns a default KeyRing configuration.
func DefaultConfig(seed []byte, params *chaincfg.Params) *Config {
	return &Config{
		NetParams: params,
		Seed:      seed,
		Purpose:   TaprootAssetsPurpose,
		CoinType:  DefaultCoinType,
	}
}

// KeyRing implements the tapgarden.KeyRing interface using BIP32 HD wallet derivation.
type KeyRing struct {
	cfg *Config

	// Master extended key
	masterKey *hdkeychain.ExtendedKey

	// Current index for each key family
	familyIndexes map[keychain.KeyFamily]uint32

	// Cache of derived keys for IsLocalKey checks
	derivedKeys map[keychain.KeyDescriptor]*btcec.PrivateKey

	mu sync.RWMutex
}

// New creates a new KeyRing.
func New(cfg *Config) (*KeyRing, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	if len(cfg.Seed) == 0 {
		return nil, fmt.Errorf("seed is required")
	}

	if cfg.NetParams == nil {
		return nil, fmt.Errorf("network params required")
	}

	// Create master key from seed
	masterKey, err := hdkeychain.NewMaster(cfg.Seed, cfg.NetParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create master key: %w", err)
	}

	kr := &KeyRing{
		cfg:           cfg,
		masterKey:     masterKey,
		familyIndexes: make(map[keychain.KeyFamily]uint32),
		derivedKeys:   make(map[keychain.KeyDescriptor]*btcec.PrivateKey),
	}

	// Load key indexes from store if available
	if cfg.KeyStateStore != nil {
		if err := kr.loadKeyIndexes(); err != nil {
			return nil, fmt.Errorf("failed to load key indexes: %w", err)
		}
	}

	return kr, nil
}

// DeriveNextKey derives the next key in the specified key family.
//
// Derivation path: m / purpose' / coin_type' / key_family' / 0 / index
func (kr *KeyRing) DeriveNextKey(ctx context.Context, keyFamily keychain.KeyFamily) (keychain.KeyDescriptor, error) {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	// Get current index for this key family
	index := kr.familyIndexes[keyFamily]

	// Derive key at path: m / purpose' / coin_type' / key_family' / 0 / index
	key, err := kr.deriveKeyAtPath(kr.cfg.Purpose, kr.cfg.CoinType, uint32(keyFamily), 0, index)
	if err != nil {
		return keychain.KeyDescriptor{}, fmt.Errorf("failed to derive key: %w", err)
	}

	// Get private key
	privKey, err := key.ECPrivKey()
	if err != nil {
		return keychain.KeyDescriptor{}, fmt.Errorf("failed to get private key: %w", err)
	}

	// Get public key
	pubKey, err := key.ECPubKey()
	if err != nil {
		return keychain.KeyDescriptor{}, fmt.Errorf("failed to get public key: %w", err)
	}

	// Create key descriptor
	keyDesc := keychain.KeyDescriptor{
		KeyLocator: keychain.KeyLocator{
			Family: keyFamily,
			Index:  index,
		},
		PubKey: pubKey,
	}

	// Cache the derived key
	kr.derivedKeys[keyDesc] = privKey

	// Increment index for next call
	kr.familyIndexes[keyFamily] = index + 1

	// Persist new index if store available
	if kr.cfg.KeyStateStore != nil {
		if err := kr.cfg.KeyStateStore.SetCurrentIndex(keyFamily, index+1); err != nil {
			// Log error but don't fail - we have the key
			fmt.Printf("Warning: failed to persist key index: %v\n", err)
		}
	}

	return keyDesc, nil
}

// IsLocalKey checks if a key is controlled by this wallet.
func (kr *KeyRing) IsLocalKey(ctx context.Context, keyDesc keychain.KeyDescriptor) bool {
	kr.mu.RLock()
	defer kr.mu.RUnlock()

	// Check cache first
	if _, exists := kr.derivedKeys[keyDesc]; exists {
		return true
	}

	// Try to derive at the specified locator
	key, err := kr.deriveKeyAtPath(
		kr.cfg.Purpose,
		kr.cfg.CoinType,
		uint32(keyDesc.Family),
		0,
		keyDesc.Index,
	)
	if err != nil {
		return false
	}

	// Get public key
	pubKey, err := key.ECPubKey()
	if err != nil {
		return false
	}

	// Compare public keys
	if keyDesc.PubKey == nil {
		return false
	}

	return pubKey.IsEqual(keyDesc.PubKey)
}

// DeriveSharedKey performs ECDH to derive a shared secret.
func (kr *KeyRing) DeriveSharedKey(
	ctx context.Context,
	ephemeralPubKey *btcec.PublicKey,
	keyLoc *keychain.KeyLocator,
) ([sha256.Size]byte, error) {
	kr.mu.RLock()
	defer kr.mu.RUnlock()

	var privKey *btcec.PrivateKey

	if keyLoc != nil {
		// Derive private key at specified location
		key, err := kr.deriveKeyAtPath(
			kr.cfg.Purpose,
			kr.cfg.CoinType,
			uint32(keyLoc.Family),
			0,
			keyLoc.Index,
		)
		if err != nil {
			return [32]byte{}, fmt.Errorf("failed to derive key: %w", err)
		}

		privKey, err = key.ECPrivKey()
		if err != nil {
			return [32]byte{}, fmt.Errorf("failed to get private key: %w", err)
		}
	} else {
		// Use master key
		var err error
		privKey, err = kr.masterKey.ECPrivKey()
		if err != nil {
			return [32]byte{}, fmt.Errorf("failed to get master private key: %w", err)
		}
	}

	// Perform ECDH: sharedSecret = privKey * ephemeralPubKey
	sharedSecret := btcec.GenerateSharedSecret(privKey, ephemeralPubKey)

	// Hash the shared secret
	return sha256.Sum256(sharedSecret), nil
}

// deriveKeyAtPath derives a key at the specified BIP32 path.
// Path: m / purpose' / coin_type' / account' / change / index
func (kr *KeyRing) deriveKeyAtPath(purpose, coinType, account, change, index uint32) (*hdkeychain.ExtendedKey, error) {
	// Start with master key
	key := kr.masterKey

	// Derive purpose (hardened)
	key, err := key.Derive(hdkeychain.HardenedKeyStart + purpose)
	if err != nil {
		return nil, fmt.Errorf("failed to derive purpose: %w", err)
	}

	// Derive coin type (hardened)
	key, err = key.Derive(hdkeychain.HardenedKeyStart + coinType)
	if err != nil {
		return nil, fmt.Errorf("failed to derive coin type: %w", err)
	}

	// Derive account (hardened)
	key, err = key.Derive(hdkeychain.HardenedKeyStart + account)
	if err != nil {
		return nil, fmt.Errorf("failed to derive account: %w", err)
	}

	// Derive change (not hardened)
	key, err = key.Derive(change)
	if err != nil {
		return nil, fmt.Errorf("failed to derive change: %w", err)
	}

	// Derive index (not hardened)
	key, err = key.Derive(index)
	if err != nil {
		return nil, fmt.Errorf("failed to derive index: %w", err)
	}

	return key, nil
}

// loadKeyIndexes loads key indexes from the store.
func (kr *KeyRing) loadKeyIndexes() error {
	allIndexes, err := kr.cfg.KeyStateStore.GetAllIndexes()
	if err != nil {
		return fmt.Errorf("failed to get all indexes: %w", err)
	}

	// Load into our map
	for family, index := range allIndexes {
		kr.familyIndexes[family] = index
	}

	return nil
}

// Verify interface compliance at compile time.
var _ tapgarden.KeyRing = (*KeyRing)(nil)
