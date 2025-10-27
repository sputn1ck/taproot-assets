package btcwallet

import "errors"

var (
	// ErrInvalidNetParams is returned when network parameters are invalid.
	ErrInvalidNetParams = errors.New("invalid network parameters")

	// ErrChainBridgeRequired is returned when chain bridge is not provided.
	ErrChainBridgeRequired = errors.New("chain bridge is required")

	// ErrPrivatePassRequired is returned when private passphrase is not provided.
	ErrPrivatePassRequired = errors.New("private passphrase is required")

	// ErrWalletNotLoaded is returned when wallet is not loaded.
	ErrWalletNotLoaded = errors.New("wallet not loaded")

	// ErrWalletLocked is returned when wallet is locked.
	ErrWalletLocked = errors.New("wallet is locked")

	// ErrInsufficientFunds is returned when wallet has insufficient funds.
	ErrInsufficientFunds = errors.New("insufficient funds")

	// ErrInvalidPsbt is returned when PSBT is invalid.
	ErrInvalidPsbt = errors.New("invalid PSBT")

	// ErrKeyNotFound is returned when key is not found in wallet.
	ErrKeyNotFound = errors.New("key not found")

	// ErrUTXOLocked is returned when UTXO is already locked.
	ErrUTXOLocked = errors.New("UTXO is locked")

	// ErrUTXONotLocked is returned when trying to unlock a non-locked UTXO.
	ErrUTXONotLocked = errors.New("UTXO is not locked")
)
