.PHONY: run-api build-api run-ctl build-ctl serve fmt-go test-go harness-smoke harness-regression harness-refresh-baselines ci

GO ?= go
GOCACHE ?= /tmp/go-build

run-api:
	$(GO) run ./cmd/mnemosyne-api

build-api:
	$(GO) build ./cmd/mnemosyne-api

run-ctl:
	$(GO) run ./cmd/mnemosynectl

build-ctl:
	$(GO) build ./cmd/mnemosynectl

serve:
	$(GO) run ./cmd/mnemosynectl serve

fmt-go:
	gofmt -w ./cmd ./internal

test-go:
	GOCACHE=$(GOCACHE) $(GO) test ./...

harness-smoke:
	GOCACHE=$(GOCACHE) ./scripts/ci-harness.sh smoke

harness-regression:
	GOCACHE=$(GOCACHE) ./scripts/ci-harness.sh regression

harness-refresh-baselines:
	GOCACHE=$(GOCACHE) ./scripts/refresh-harness-baselines.sh

ci: test-go harness-smoke harness-regression

.PHONY: service-install-macos service-uninstall-macos
service-install-macos:
	chmod +x ./scripts/install-macos-service.sh
	./scripts/install-macos-service.sh

service-uninstall-macos:
	chmod +x ./scripts/uninstall-macos-service.sh
	./scripts/uninstall-macos-service.sh
