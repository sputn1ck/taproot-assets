# Task 12: Mobile Bindings

## Goal

Create gomobile-compatible bindings for iOS and Android.

## Implementation Approach

### 1. Mobile API

```go
//go:build mobile
package mobile

type MobileTapd struct {
    client *lwtapd.Client
}

func NewMobileTapd(dbPath, network, seed string) (*MobileTapd, error) {
    seedBytes, err := hex.DecodeString(seed)
    if err != nil {
        return nil, err
    }
    
    client, err := lwtapd.New(&lwtapd.Config{
        Network:    network,
        DBPath:     dbPath,
        MempoolURL: getMempoolURL(network),
        WalletSeed: seedBytes,
    })
    
    return &MobileTapd{client: client}, nil
}

func (m *MobileTapd) Start() error {
    return m.client.Start()
}

func (m *MobileTapd) MintAsset(name string, amount int64) (string, error) {
    asset, err := m.client.MintAsset(context.Background(), &lwtapd.MintRequest{
        Name:   name,
        Amount: uint64(amount),
    })
    if err != nil {
        return "", err
    }
    
    // Return JSON
    return assetToJSON(asset)
}

// More methods...
```

### 2. Build Commands

```bash
# iOS
gomobile bind -target=ios github.com/lightninglabs/taproot-assets/lightweight-wallet/mobile

# Android  
gomobile bind -target=android github.com/lightninglabs/taproot-assets/lightweight-wallet/mobile
```

### 3. iOS Usage

```swift
import Lwtapd

let dbPath = /* documents directory */ + "/tapd.db"
let tapd = LwtapdNewMobileTapd(dbPath, "testnet", seedHex)
try tapd.start()

let assetJSON = try tapd.mintAsset("TEST", 1000)
```

### 4. Android Usage

```kotlin
import lwtapd.Mobile

val dbPath = context.filesDir.path + "/tapd.db"
val tapd = Mobile.newMobileTapd(dbPath, "testnet", seedHex)
tapd.start()

val assetJSON = tapd.mintAsset("TEST", 1000)
```

## Success Criteria

- [ ] gomobile bindings build successfully
- [ ] iOS framework works
- [ ] Android AAR works
- [ ] Example apps provided
- [ ] Documentation complete

