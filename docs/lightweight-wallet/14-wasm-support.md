# Task 13: WASM Support

## Goal

Enable running lightweight wallet in browser via WebAssembly.

## Implementation Approach

### 1. WASM Exports

```go
//go:build wasm
package main

import (
    "syscall/js"
    "github.com/lightninglabs/taproot-assets/lightweight-wallet/lwtapd"
)

var client *lwtapd.Client

func initialize(this js.Value, args []js.Value) interface{} {
    // args[0] = config object
    config := parseConfig(args[0])
    
    var err error
    client, err = lwtapd.New(config)
    if err != nil {
        return js.ValueOf(map[string]interface{}{
            "error": err.Error(),
        })
    }
    
    return js.ValueOf(map[string]interface{}{
        "success": true,
    })
}

func mintAsset(this js.Value, args []js.Value) interface{} {
    name := args[0].String()
    amount := args[1].Int()
    
    asset, err := client.MintAsset(context.Background(), &lwtapd.MintRequest{
        Name:   name,
        Amount: uint64(amount),
    })
    
    // Return promise
    return promisify(func() (interface{}, error) {
        if err != nil {
            return nil, err
        }
        return assetToJSValue(asset), nil
    })
}

func main() {
    js.Global().Set("TapdClient", js.ValueOf(map[string]interface{}{
        "initialize": js.FuncOf(initialize),
        "mintAsset":  js.FuncOf(mintAsset),
    }))
    
    select {} // Keep alive
}
```

### 2. JavaScript Usage

```javascript
// Load WASM
const go = new Go();
WebAssembly.instantiateStreaming(fetch("lwtapd.wasm"), go.importObject)
    .then(result => go.run(result.instance));

// Initialize
await TapdClient.initialize({
    network: "testnet",
    mempoolURL: "https://mempool.space/testnet/api",
    seed: seedHex,
});

// Mint asset
const asset = await TapdClient.mintAsset("TEST", 1000);
console.log(asset);
```

### 3. Build

```bash
GOOS=js GOARCH=wasm go build -o lwtapd.wasm ./lightweight-wallet/wasm
```

## Success Criteria

- [ ] WASM builds successfully
- [ ] Runs in browser
- [ ] IndexedDB storage works
- [ ] Example web app provided

