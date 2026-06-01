WASM_EXEC_JS := $(shell go env GOROOT)/misc/wasm/wasm_exec.js
FW_PAYLOAD ?= bin/fw_payload.bin

.PHONY: all wasm serve test rvsmoke fw-payload clean
all: wasm

wasm:
	mkdir -p web
	GOOS=js GOARCH=wasm go build -trimpath -buildvcs=false -ldflags='-s -w' -o web/riscv.wasm ./cmd/rvwasm
	cp $(WASM_EXEC_JS) web/wasm_exec.js

test:
	go test ./...

rvsmoke:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o bin/rvsmoke ./cmd/rvsmoke

serve: wasm
	node scripts/serve-web.mjs

fw-payload: wasm
	node scripts/run-fw-payload.mjs web/riscv.wasm "$(FW_PAYLOAD)"

clean:
	rm -f web/riscv.wasm web/wasm_exec.js
