package mempool

import (
	"time"
)

// API response types for mempool.space REST API

// BlockResponse represents a block from the mempool.space API.
type BlockResponse struct {
	ID                string  `json:"id"`
	Height            int64   `json:"height"`
	Version           int32   `json:"version"`
	Timestamp         int64   `json:"timestamp"`
	TxCount           int     `json:"tx_count"`
	Size              int     `json:"size"`
	Weight            int     `json:"weight"`
	MerkleRoot        string  `json:"merkle_root"`
	PreviousBlockHash string  `json:"previousblockhash"`
	Nonce             uint32  `json:"nonce"`
	Bits              uint32  `json:"bits"`
	Difficulty        float64 `json:"difficulty"`
}

// TransactionResponse represents a transaction from the mempool.space API.
type TransactionResponse struct {
	TxID     string                 `json:"txid"`
	Version  int32                  `json:"version"`
	Locktime uint32                 `json:"locktime"`
	Size     int                    `json:"size"`
	Weight   int                    `json:"weight"`
	Fee      int64                  `json:"fee"`
	Vin      []TransactionInput     `json:"vin"`
	Vout     []TransactionOutput    `json:"vout"`
	Status   TransactionStatus      `json:"status"`
}

// TransactionInput represents a transaction input.
type TransactionInput struct {
	TxID         string   `json:"txid"`
	Vout         uint32   `json:"vout"`
	Prevout      *Output  `json:"prevout,omitempty"`
	ScriptSig    string   `json:"scriptsig"`
	ScriptSigAsm string   `json:"scriptsig_asm"`
	Witness      []string `json:"witness,omitempty"`
	Sequence     uint32   `json:"sequence"`
	IsCoinbase   bool     `json:"is_coinbase"`
}

// TransactionOutput represents a transaction output.
type TransactionOutput struct {
	ScriptPubKey    string `json:"scriptpubkey"`
	ScriptPubKeyAsm string `json:"scriptpubkey_asm"`
	ScriptPubKeyType string `json:"scriptpubkey_type"`
	ScriptPubKeyAddr string `json:"scriptpubkey_address,omitempty"`
	Value           int64  `json:"value"`
}

// Output represents an output with additional info.
type Output struct {
	ScriptPubKey    string `json:"scriptpubkey"`
	ScriptPubKeyAsm string `json:"scriptpubkey_asm"`
	ScriptPubKeyType string `json:"scriptpubkey_type"`
	ScriptPubKeyAddr string `json:"scriptpubkey_address,omitempty"`
	Value           int64  `json:"value"`
}

// TransactionStatus represents the confirmation status of a transaction.
type TransactionStatus struct {
	Confirmed   bool   `json:"confirmed"`
	BlockHeight int64  `json:"block_height,omitempty"`
	BlockHash   string `json:"block_hash,omitempty"`
	BlockTime   int64  `json:"block_time,omitempty"`
}

// FeeEstimates represents fee estimates for different confirmation targets.
type FeeEstimates struct {
	FastestFee  int64 `json:"fastestFee"`  // Next block
	HalfHourFee int64 `json:"halfHourFee"` // ~3 blocks
	HourFee     int64 `json:"hourFee"`     // ~6 blocks
	EconomyFee  int64 `json:"economyFee"`  // ~12 blocks
	MinimumFee  int64 `json:"minimumFee"`  // Minimum relay fee
}

// cacheEntry is a generic cache entry with TTL.
type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}
