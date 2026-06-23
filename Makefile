GO ?= go
GOFMT ?= gofmt

.PHONY: fmt fmt-check vet lint test test-race test-integration schema-check test-fuzz-seeds build generate verify

fmt:
	$(GOFMT) -w .

fmt-check:
	@test -z "$$($(GOFMT) -l .)" || { echo "gofmt needed:"; $(GOFMT) -l .; exit 1; }

vet:
	$(GO) vet ./...

lint: vet

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-integration:
	@echo "no integration tests in GR-4"

schema-check:
	$(GO) test ./internal/config -run 'TestPipelineSchema|TestSchemaDocuments' -count=1

test-fuzz-seeds:
	$(GO) test ./internal/config -run FuzzParseAndValidate -count=1

build:
	@tmp="$$(mktemp -t glassroot.XXXXXX)"; \
	$(GO) build -o "$$tmp" ./cmd/glassroot; \
	rm -f "$$tmp"

generate:
	$(GO) generate ./...

verify: fmt-check vet test schema-check build
