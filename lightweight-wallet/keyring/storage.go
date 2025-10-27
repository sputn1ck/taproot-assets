package keyring

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/lightningnetwork/lnd/keychain"
)

// KeyStateStore is an interface for persisting key indexes.
type KeyStateStore interface {
	// GetCurrentIndex returns the current index for a key family.
	GetCurrentIndex(family keychain.KeyFamily) (uint32, error)

	// SetCurrentIndex sets the current index for a key family.
	SetCurrentIndex(family keychain.KeyFamily, index uint32) error

	// GetAllIndexes returns all key family indexes.
	GetAllIndexes() (map[keychain.KeyFamily]uint32, error)
}

// FileKeyStateStore implements KeyStateStore using a JSON file.
type FileKeyStateStore struct {
	filePath string
	indexes  map[keychain.KeyFamily]uint32
	mu       sync.RWMutex
}

// keyStateFile represents the JSON structure for key state.
type keyStateFile struct {
	KeyFamilies map[string]uint32 `json:"key_families"`
}

// NewFileKeyStateStore creates a new file-based key state store.
func NewFileKeyStateStore(filePath string) (*FileKeyStateStore, error) {
	store := &FileKeyStateStore{
		filePath: filePath,
		indexes:  make(map[keychain.KeyFamily]uint32),
	}

	// Load existing state if file exists
	if err := store.load(); err != nil {
		// If file doesn't exist, that's OK - we'll create it on first save
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load key state: %w", err)
		}
	}

	return store, nil
}

// GetCurrentIndex returns the current index for a key family.
func (s *FileKeyStateStore) GetCurrentIndex(family keychain.KeyFamily) (uint32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, exists := s.indexes[family]
	if !exists {
		return 0, nil // Start at 0 if not found
	}

	return index, nil
}

// SetCurrentIndex sets the current index for a key family.
func (s *FileKeyStateStore) SetCurrentIndex(family keychain.KeyFamily, index uint32) error {
	s.mu.Lock()
	s.indexes[family] = index
	s.mu.Unlock()

	// Persist to file
	return s.save()
}

// GetAllIndexes returns all key family indexes.
func (s *FileKeyStateStore) GetAllIndexes() (map[keychain.KeyFamily]uint32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy
	result := make(map[keychain.KeyFamily]uint32, len(s.indexes))
	for family, index := range s.indexes {
		result[family] = index
	}

	return result, nil
}

// load loads key state from file.
func (s *FileKeyStateStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	var state keyStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to unmarshal key state: %w", err)
	}

	// Convert string keys to KeyFamily
	s.indexes = make(map[keychain.KeyFamily]uint32, len(state.KeyFamilies))
	for familyStr, index := range state.KeyFamilies {
		var family uint32
		if _, err := fmt.Sscanf(familyStr, "%d", &family); err != nil {
			continue
		}
		s.indexes[keychain.KeyFamily(family)] = index
	}

	return nil
}

// save saves key state to file.
func (s *FileKeyStateStore) save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Convert KeyFamily to string for JSON
	state := keyStateFile{
		KeyFamilies: make(map[string]uint32, len(s.indexes)),
	}

	for family, index := range s.indexes {
		familyStr := fmt.Sprintf("%d", family)
		state.KeyFamilies[familyStr] = index
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal key state: %w", err)
	}

	// Write to file
	if err := os.WriteFile(s.filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write key state: %w", err)
	}

	return nil
}

// MemoryKeyStateStore implements KeyStateStore using in-memory storage.
type MemoryKeyStateStore struct {
	indexes map[keychain.KeyFamily]uint32
	mu      sync.RWMutex
}

// NewMemoryKeyStateStore creates a new in-memory key state store.
func NewMemoryKeyStateStore() *MemoryKeyStateStore {
	return &MemoryKeyStateStore{
		indexes: make(map[keychain.KeyFamily]uint32),
	}
}

// GetCurrentIndex returns the current index for a key family.
func (s *MemoryKeyStateStore) GetCurrentIndex(family keychain.KeyFamily) (uint32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, exists := s.indexes[family]
	if !exists {
		return 0, nil
	}

	return index, nil
}

// SetCurrentIndex sets the current index for a key family.
func (s *MemoryKeyStateStore) SetCurrentIndex(family keychain.KeyFamily, index uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.indexes[family] = index
	return nil
}

// GetAllIndexes returns all key family indexes.
func (s *MemoryKeyStateStore) GetAllIndexes() (map[keychain.KeyFamily]uint32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy
	result := make(map[keychain.KeyFamily]uint32, len(s.indexes))
	for family, index := range s.indexes {
		result[family] = index
	}

	return result, nil
}
