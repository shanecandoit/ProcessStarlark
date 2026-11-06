#!/bin/sh
# Build the browser playground into web/dist. Run from anywhere.
# Serve it with any static server, e.g.: python3 -m http.server -d web/dist 8000
set -e
cd "$(dirname "$0")/.."
mkdir -p web/dist
GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o web/dist/park.wasm ./cmd/park-wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/dist/wasm_exec.js
go run ./cmd/playground-build
ls -lh web/dist/park.wasm | awk '{print "park.wasm: " $5}'
gzip -9 -c web/dist/park.wasm | wc -c | awk '{printf "park.wasm gzipped: %.1fM\n", $1/1048576}'
