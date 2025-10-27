package btcwallet

import (
	"sync"
	"time"

	"github.com/btcsuite/btcd/wire"
)

// utxoLock represents a lock on a UTXO.
type utxoLock struct {
	expiresAt time.Time
}

// utxoLockManager manages UTXO locks to prevent double-spending.
type utxoLockManager struct {
	locks map[wire.OutPoint]utxoLock
	mu    sync.RWMutex
}

// newUTXOLockManager creates a new UTXO lock manager.
func newUTXOLockManager() *utxoLockManager {
	return &utxoLockManager{
		locks: make(map[wire.OutPoint]utxoLock),
	}
}

// LockUTXO locks a UTXO for the specified duration.
func (m *utxoLockManager) LockUTXO(outpoint wire.OutPoint, duration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already locked
	if lock, exists := m.locks[outpoint]; exists {
		if time.Now().Before(lock.expiresAt) {
			return ErrUTXOLocked
		}
	}

	// Lock the UTXO
	m.locks[outpoint] = utxoLock{
		expiresAt: time.Now().Add(duration),
	}

	return nil
}

// UnlockUTXO unlocks a UTXO.
func (m *utxoLockManager) UnlockUTXO(outpoint wire.OutPoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.locks[outpoint]; !exists {
		return ErrUTXONotLocked
	}

	delete(m.locks, outpoint)
	return nil
}

// IsLocked checks if a UTXO is currently locked.
func (m *utxoLockManager) IsLocked(outpoint wire.OutPoint) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	lock, exists := m.locks[outpoint]
	if !exists {
		return false
	}

	return time.Now().Before(lock.expiresAt)
}

// CleanupExpired removes expired locks.
func (m *utxoLockManager) CleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for outpoint, lock := range m.locks {
		if now.After(lock.expiresAt) {
			delete(m.locks, outpoint)
		}
	}
}

// GetLocked returns all currently locked outpoints.
func (m *utxoLockManager) GetLocked() []wire.OutPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	locked := make([]wire.OutPoint, 0, len(m.locks))

	for outpoint, lock := range m.locks {
		if now.Before(lock.expiresAt) {
			locked = append(locked, outpoint)
		}
	}

	return locked
}
