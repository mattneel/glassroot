GO ?= go

.PHONY: fmt vet lint test test-race test-integration build generate verify

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint: vet

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-integration:
	@echo "no integration tests in GR-1"

build:
	mkdir -p bin
	$(GO) build -o bin/glassroot ./cmd/glassroot

generate:
	$(GO) generate ./...

verify: fmt vet test build
