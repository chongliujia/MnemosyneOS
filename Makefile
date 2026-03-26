.PHONY: run-api build-api run-ctl build-ctl fmt-go

run-api:
	go run ./cmd/mnemosyne-api

build-api:
	go build ./cmd/mnemosyne-api

run-ctl:
	go run ./cmd/mnemosynectl

build-ctl:
	go build ./cmd/mnemosynectl

fmt-go:
	gofmt -w ./cmd ./internal
