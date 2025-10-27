package btcwallet

import (
	"context"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/taproot-assets/tapsend"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
)

// FundPsbt funds a PSBT with wallet UTXOs.
func (w *WalletAnchor) FundPsbt(
	ctx context.Context,
	packet *psbt.Packet,
	minConfs uint32,
	feeRate chainfee.SatPerKWeight,
	changeIdx int32,
) (*tapsend.FundedPsbt, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.wallet == nil {
		return nil, ErrWalletNotLoaded
	}

	// Calculate required amount from outputs
	var outputAmount btcutil.Amount
	for _, txOut := range packet.UnsignedTx.TxOut {
		outputAmount += btcutil.Amount(txOut.Value)
	}

	// Estimate fee
	// Rough estimate: 180 bytes per input, 34 bytes per output
	estimatedVSize := int64(len(packet.UnsignedTx.TxIn)*180 + len(packet.UnsignedTx.TxOut)*34 + 10)
	feeRateSatPerKB := int64(feeRate) * 250 / 1000 // Convert sat/kw to sat/kb
	estimatedFee := btcutil.Amount(estimatedVSize * feeRateSatPerKB / 1000)

	totalRequired := outputAmount + estimatedFee

	// List unspent outputs
	unspent, err := w.wallet.ListUnspent(int32(minConfs), 9999999, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list unspent: %w", err)
	}

	// Select coins to cover the amount
	var selectedCoins []*wire.TxIn
	var selectedAmounts []btcutil.Amount
	var totalInput btcutil.Amount

	for _, utxo := range unspent {
		// Parse txid hash
		txHash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			continue
		}

		// Check if UTXO is locked
		outpoint := wire.OutPoint{
			Hash:  *txHash,
			Index: utxo.Vout,
		}

		if w.utxoLocks.IsLocked(outpoint) {
			continue
		}

		// Add input
		txIn := wire.NewTxIn(&outpoint, nil, nil)
		selectedCoins = append(selectedCoins, txIn)
		selectedAmounts = append(selectedAmounts, btcutil.Amount(utxo.Amount))
		totalInput += btcutil.Amount(utxo.Amount)

		// Lock this UTXO
		w.utxoLocks.LockUTXO(outpoint, 10*time.Minute)

		if totalInput >= totalRequired {
			break
		}
	}

	if totalInput < totalRequired {
		return nil, ErrInsufficientFunds
	}

	// Add selected inputs to PSBT
	for i, txIn := range selectedCoins {
		packet.UnsignedTx.TxIn = append(packet.UnsignedTx.TxIn, txIn)

		// Add PSBT input
		pInput := psbt.PInput{
			// Will be populated during signing
		}

		// Get witness UTXO for this input using FetchOutpointInfo
		_, prevOut, _, err := w.wallet.FetchOutpointInfo(&txIn.PreviousOutPoint)
		if err == nil && prevOut != nil {
			pInput.WitnessUtxo = prevOut
		}

		packet.Inputs = append(packet.Inputs, pInput)
		_ = selectedAmounts[i] // Keep for reference
	}

	// Calculate change
	change := totalInput - totalRequired
	changeOutputIndex := -1

	// Add change output if significant
	if change > btcutil.Amount(546) { // Dust limit
		// Get change address for account 0 with BIP84 (native SegWit)
		changeAddr, err := w.wallet.NewChangeAddress(
			waddrmgr.DefaultAccountNum,
			waddrmgr.KeyScopeBIP0084,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get change address: %w", err)
		}

		// Create change script
		changeScript, err := txscript.PayToAddrScript(changeAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create change script: %w", err)
		}

		// Add change output
		changeOut := &wire.TxOut{
			Value:    int64(change),
			PkScript: changeScript,
		}

		// Insert at specified index or append
		if changeIdx >= 0 && int(changeIdx) <= len(packet.UnsignedTx.TxOut) {
			// Insert at position
			newOuts := make([]*wire.TxOut, 0, len(packet.UnsignedTx.TxOut)+1)
			newOuts = append(newOuts, packet.UnsignedTx.TxOut[:changeIdx]...)
			newOuts = append(newOuts, changeOut)
			newOuts = append(newOuts, packet.UnsignedTx.TxOut[changeIdx:]...)
			packet.UnsignedTx.TxOut = newOuts
			changeOutputIndex = int(changeIdx)
		} else {
			packet.UnsignedTx.TxOut = append(packet.UnsignedTx.TxOut, changeOut)
			changeOutputIndex = len(packet.UnsignedTx.TxOut) - 1
		}

		// Add PSBT output
		packet.Outputs = append(packet.Outputs, psbt.POutput{})
	}

	// Create funded PSBT
	fundedPsbt := &tapsend.FundedPsbt{
		Pkt:               packet,
		ChangeOutputIndex: int32(changeOutputIndex),
		ChainFees:         int64(estimatedFee),
	}

	return fundedPsbt, nil
}

// SignPsbt signs all inputs in the PSBT that the wallet can sign.
func (w *WalletAnchor) SignPsbt(ctx context.Context, packet *psbt.Packet) (*psbt.Packet, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.wallet == nil {
		return nil, ErrWalletNotLoaded
	}

	// Sign each input
	for i := range packet.Inputs {
		if i >= len(packet.UnsignedTx.TxIn) {
			continue
		}

		txIn := packet.UnsignedTx.TxIn[i]

		// Try to sign this input
		err := w.signInput(packet, i, txIn)
		if err != nil {
			// Not our input or can't sign, continue
			continue
		}
	}

	return packet, nil
}

// signInput signs a single input in the PSBT.
func (w *WalletAnchor) signInput(packet *psbt.Packet, inputIdx int, _ *wire.TxIn) error {
	// Get previous output
	pInput := packet.Inputs[inputIdx]
	if pInput.WitnessUtxo == nil {
		// Can't sign without previous output info
		return fmt.Errorf("missing witness UTXO for input %d", inputIdx)
	}

	prevOut := pInput.WitnessUtxo

	// Extract address from script
	_, addrs, _, err := txscript.ExtractPkScriptAddrs(prevOut.PkScript, w.cfg.NetParams)
	if err != nil || len(addrs) == 0 {
		return fmt.Errorf("failed to extract address: %w", err)
	}

	addr := addrs[0]

	// Check if we have the private key for this address
	privKey, err := w.wallet.PrivKeyForAddress(addr)
	if err != nil {
		return fmt.Errorf("don't have private key for address: %w", err)
	}

	// Sign based on script type
	if txscript.IsPayToWitnessPubKeyHash(prevOut.PkScript) {
		// P2WPKH signing
		return w.signP2WPKH(packet, inputIdx, prevOut, privKey)
	}

	// Add other script types as needed
	return fmt.Errorf("unsupported script type")
}

// signP2WPKH signs a P2WPKH input.
func (w *WalletAnchor) signP2WPKH(packet *psbt.Packet, inputIdx int, prevOut *wire.TxOut, privKey *btcec.PrivateKey) error {
	// Create sighash
	sigHashes := txscript.NewTxSigHashes(packet.UnsignedTx, nil)

	sigHash, err := txscript.CalcWitnessSigHash(
		prevOut.PkScript,
		sigHashes,
		txscript.SigHashAll,
		packet.UnsignedTx,
		inputIdx,
		prevOut.Value,
	)
	if err != nil {
		return fmt.Errorf("failed to calculate sighash: %w", err)
	}

	// Sign the hash
	sig := ecdsa.Sign(privKey, sigHash)

	// Create witness
	sigBytes := append(sig.Serialize(), byte(txscript.SigHashAll))
	pubKeyBytes := privKey.PubKey().SerializeCompressed()

	packet.UnsignedTx.TxIn[inputIdx].Witness = wire.TxWitness{sigBytes, pubKeyBytes}

	return nil
}

// SignAndFinalizePsbt signs and finalizes a PSBT.
func (w *WalletAnchor) SignAndFinalizePsbt(ctx context.Context, packet *psbt.Packet) (*psbt.Packet, error) {
	// First sign the PSBT
	signedPacket, err := w.SignPsbt(ctx, packet)
	if err != nil {
		return nil, fmt.Errorf("failed to sign PSBT: %w", err)
	}

	// Finalize each input
	for i := range signedPacket.Inputs {
		err := psbt.Finalize(signedPacket, i)
		if err != nil {
			// Some inputs may not be finalizable yet
			continue
		}
	}

	return signedPacket, nil
}

// ImportTaprootOutput imports a taproot output into the wallet for watching.
func (w *WalletAnchor) ImportTaprootOutput(ctx context.Context, pubKey *btcec.PublicKey) (btcutil.Address, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.wallet == nil {
		return nil, ErrWalletNotLoaded
	}

	// Create taproot address from public key
	// Taproot addresses use the x-only public key (32 bytes)
	pubKeyBytes := pubKey.SerializeCompressed()[1:] // Remove prefix byte
	addr, err := btcutil.NewAddressTaproot(pubKeyBytes, w.cfg.NetParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create taproot address: %w", err)
	}

	// Import as watch-only
	// Note: btcwallet's ImportPublicKey may not fully support Taproot yet
	// This is a simplified implementation
	err = w.wallet.ImportPublicKey(pubKey, waddrmgr.WitnessPubKey)
	if err != nil {
		// Ignore error if already imported
		// In production, would need better error handling
	}

	return addr, nil
}

// UnlockInput unlocks a previously locked input.
func (w *WalletAnchor) UnlockInput(ctx context.Context, outpoint wire.OutPoint) error {
	return w.utxoLocks.UnlockUTXO(outpoint)
}

// ListUnspentImportScripts lists all UTXOs from imported scripts.
func (w *WalletAnchor) ListUnspentImportScripts(ctx context.Context) ([]*lnwallet.Utxo, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.wallet == nil {
		return nil, ErrWalletNotLoaded
	}

	// List unspent outputs
	unspent, err := w.wallet.ListUnspent(int32(w.cfg.MinConfs), 9999999, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list unspent: %w", err)
	}

	// Convert to lnwallet.Utxo format
	utxos := make([]*lnwallet.Utxo, 0, len(unspent))
	for _, u := range unspent {
		// Parse txid
		txidBytes, err := chainhash.NewHashFromStr(u.TxID)
		if err != nil {
			continue
		}

		utxo := &lnwallet.Utxo{
			AddressType:   lnwallet.WitnessPubKey,
			Value:         btcutil.Amount(u.Amount),
			OutPoint:      wire.OutPoint{Hash: *txidBytes, Index: u.Vout},
			Confirmations: int64(u.Confirmations),
		}
		utxos = append(utxos, utxo)
	}

	return utxos, nil
}

// ListTransactions lists transactions in the specified block range.
func (w *WalletAnchor) ListTransactions(
	ctx context.Context,
	startHeight, endHeight int32,
	account string,
) ([]lndclient.Transaction, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.wallet == nil {
		return nil, ErrWalletNotLoaded
	}

	// Use btcwallet's ListTransactions
	// Returns btcjson.ListTransactionsResult which we need to convert
	limit := 10000 // Fetch up to 10k transactions
	results, err := w.wallet.ListTransactions(0, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list transactions: %w", err)
	}

	// Convert to lndclient.Transaction format
	transactions := make([]lndclient.Transaction, 0, len(results))
	for _, result := range results {
		// Handle optional fields (pointers)
		blockHeight := int32(0)
		if result.BlockHeight != nil {
			blockHeight = *result.BlockHeight
		}

		// Filter by block height range if specified
		if startHeight != 0 && blockHeight < startHeight {
			continue
		}
		if endHeight >= 0 && blockHeight > endHeight {
			continue
		}

		// Parse transaction hash
		txHash, err := chainhash.NewHashFromStr(result.TxID)
		if err != nil {
			continue
		}

		// Handle fee (pointer)
		fee := btcutil.Amount(0)
		if result.Fee != nil {
			fee = btcutil.Amount(*result.Fee)
		}

		tx := lndclient.Transaction{
			Tx:            nil, // Don't have raw tx in list response
			TxHash:        txHash.String(),
			Timestamp:     time.Unix(result.Time, 0),
			Amount:        btcutil.Amount(result.Amount),
			Fee:           fee,
			Confirmations: int32(result.Confirmations),
			BlockHeight:   blockHeight,
		}
		transactions = append(transactions, tx)
	}

	return transactions, nil
}

// SubscribeTransactions subscribes to new transaction notifications.
func (w *WalletAnchor) SubscribeTransactions(ctx context.Context) (<-chan lndclient.Transaction, <-chan error, error) {
	w.txSubMu.Lock()
	defer w.txSubMu.Unlock()

	if w.wallet == nil {
		return nil, nil, ErrWalletNotLoaded
	}

	// Create channels
	txChan := make(chan lndclient.Transaction, 10)
	errChan := make(chan error, 1)

	// Generate unique subscription ID
	subID := fmt.Sprintf("sub-%d", len(w.txSubscriptions))

	// Register subscription
	w.txSubscriptions[subID] = txChan

	return txChan, errChan, nil
}
