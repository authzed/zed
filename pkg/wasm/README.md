# WebAssembly zed Package

This package provides zed's functionality via a WebAssembly interface, for use with browser-based tooling.

> **Warning**
> The WebAssembly development interface is **not stable** and subject to change between versions of zed.

## Generating WebAssembly

```sh
GOOS=js GOARCH=wasm go build -o main.wasm
```

## Integrating with the browser

To see an example of invoking the WebAssembly based interface:

1. Build `main.wasm` and copy into the [example](example) directory.
2. Copy [https://github.com/golang/go/blob/master/misc/wasm/wasm_exec.js](https://github.com/golang/go/blob/master/misc/wasm/wasm_exec.js) into the [example](example) directory
3. Run an HTTP server over the example directory and visit wasm.html:

```sh
python3 -m http.server
```
