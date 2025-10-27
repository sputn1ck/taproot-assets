# Task 11: Go Library API

## Goal

Create clean Go library API for embedding lightweight wallet in other Go applications.

## Implementation Approach

### 1. Client API

```go
package lwtapd

type Client struct {
    server *Server
}

func New(cfg *Config) (*Client, error) {
    server, err := NewServer(cfg)
    if err != nil {
        return nil, err
    }
    
    return &Client{server: server}, nil
}

func (c *Client) Start() error {
    return c.server.Start()
}

func (c *Client) Stop() error {
    return c.server.Stop()
}
```

### 2. Asset Operations

```go
func (c *Client) MintAsset(ctx context.Context, req *MintRequest) (*Asset, error) {
    return c.server.minter.MintAsset(ctx, req)
}

func (c *Client) SendAsset(ctx context.Context, req *SendRequest) (*SendResponse, error) {
    return c.server.sender.SendAsset(ctx, req)
}

func (c *Client) NewAddress(ctx context.Context, assetID asset.ID, amount uint64) (*Address, error) {
    return c.server.receiver.NewAddress(ctx, assetID, amount)
}

func (c *Client) ListAssets(ctx context.Context) ([]*Asset, error) {
    return c.server.db.AssetStore.FetchAllAssets(ctx)
}
```

### 3. Example Usage

```go
package main

import "github.com/lightninglabs/taproot-assets/lightweight-wallet/lwtapd"

func main() {
    client, err := lwtapd.New(&lwtapd.Config{
        Network:    "testnet",
        DBPath:     "./data/tapd.db",
        MempoolURL: "https://mempool.space/testnet/api",
        WalletSeed: seed,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    if err := client.Start(); err != nil {
        log.Fatal(err)
    }
    defer client.Stop()
    
    // Mint asset
    asset, err := client.MintAsset(ctx, &lwtapd.MintRequest{
        Name:   "TEST",
        Amount: 1000,
    })
    
    // List assets
    assets, err := client.ListAssets(ctx)
    fmt.Printf("Assets: %v\n", assets)
}
```

## Success Criteria

- [ ] Clean, idiomatic Go API
- [ ] Well-documented
- [ ] Example code provided
- [ ] Thread-safe
- [ ] Error handling clear

