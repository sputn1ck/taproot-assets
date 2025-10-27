package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClient_New tests creating a complete lightweight tapd client.
func TestClient_New(t *testing.T) {
	tmpDir := t.TempDir()

	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	cfg := &Config{
		Network:    "testnet",
		DBPath:     tmpDir + "/tapd.db",
		Seed:       seed,
		MempoolURL: "https://mempool.space/testnet/api",
		ProofDir:   tmpDir + "/proofs",
	}

	client, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify all components initialized
	require.NotNil(t, client.chainBridge)
	require.NotNil(t, client.walletAnchor)
	require.NotNil(t, client.keyRing)
	require.NotNil(t, client.stores)
	require.NotNil(t, client.proofSystem)
	require.NotNil(t, client.minter)
	require.NotNil(t, client.sender)
	require.NotNil(t, client.receiver)

	// Test start/stop
	err = client.Start()
	require.NoError(t, err)

	err = client.Stop()
	require.NoError(t, err)
}

// TestClient_ListAssets tests listing assets.
func TestClient_ListAssets(t *testing.T) {
	tmpDir := t.TempDir()

	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}

	cfg := &Config{
		Network:  "testnet",
		DBPath:   tmpDir + "/tapd.db",
		Seed:     seed,
		ProofDir: tmpDir + "/proofs",
	}

	client, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	// List assets (should be empty initially)
	ctx := context.Background()
	assets, err := client.ListAssets(ctx)
	require.NoError(t, err)
	require.Empty(t, assets)

	// Clean up
	client.Stop()
}
