# Task 10: Server Initialization

## Goal

Create lightweight server that initializes all components without LND dependency.

## Implementation Approach

### 1. Configuration

```go
type Config struct {
    Network      string
    DBPath       string
    
    // Chain
    ChainBackend ChainBackendType
    MempoolURL   string
    
    // Wallet
    WalletSeed   []byte
    
    // RPC
    RPCListen    string
    RESTListen   string
}
```

### 2. Server Struct

```go
type Server struct {
    cfg *Config
    
    // Core components
    chainBridge  ChainBridge
    wallet       WalletAnchor
    keyRing      KeyRing
    db           *Stores
    
    // Operations
    minter       *AssetMinter
    sender       *AssetSender
    receiver     *AssetReceiver
    
    // RPC
    rpcServer    *rpcServer
}
```

### 3. Initialization Flow

```go
func NewServer(cfg *Config) (*Server, error) {
    // 1. Initialize database
    stores, err := db.InitAllStores(cfg.DBPath)
    
    // 2. Initialize chain backend
    chainBridge := chain.NewMempoolBridge(cfg.MempoolURL)
    
    // 3. Initialize wallet
    wallet := wallet.NewBTCWallet(cfg.WalletSeed, chainBridge)
    
    // 4. Initialize keyring
    keyRing := keyring.NewKeyRing(cfg.WalletSeed)
    
    // 5. Initialize proof system
    proofSystem := proof.SetupProofSystem(...)
    
    // 6. Initialize operations
    minter := mint.NewAssetMinter(...)
    sender := send.NewAssetSender(...)
    receiver := receive.NewAssetReceiver(...)
    
    // 7. Initialize RPC
    rpcServer := newRPCServer(...)
    
    return &Server{
        cfg:         cfg,
        chainBridge: chainBridge,
        wallet:      wallet,
        keyRing:     keyRing,
        db:          stores,
        minter:      minter,
        sender:      sender,
        receiver:    receiver,
        rpcServer:   rpcServer,
    }, nil
}
```

### 4. Start/Stop

```go
func (s *Server) Start() error {
    // Start chain monitoring
    s.chainBridge.Start()
    
    // Start custodian
    s.receiver.Start()
    
    // Start RPC server
    s.rpcServer.Start()
    
    return nil
}

func (s *Server) Stop() error {
    s.rpcServer.Stop()
    s.receiver.Stop()
    s.chainBridge.Stop()
    return nil
}
```

## Success Criteria

- [ ] Server initializes all components
- [ ] Can start/stop cleanly
- [ ] RPC server works
- [ ] All operations functional

