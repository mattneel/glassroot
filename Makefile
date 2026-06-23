GO ?= go
GOFMT ?= gofmt

.PHONY: fmt fmt-check vet lint test test-race test-integration schema-check test-fuzz-seeds test-gitstore test-gitstore-fuzz-seeds test-materialize test-materialize-fuzz-seeds test-pipeline test-pipeline-fuzz-seeds test-runner test-runner-fuzz-seeds test-evidence test-evidence-fuzz-seeds test-evidence-reader test-evidence-reader-fuzz-seeds test-observe test-observe-fuzz-seeds build generate verify

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
	$(GO) test ./internal/gitstore -run 'FuzzParseLsTree|FuzzParseCatFileHeader|FuzzValidateGitTreePath' -count=1
	$(GO) test ./internal/materialize -run 'FuzzValidateMaterializationInventory|FuzzValidateSymlinkTarget|FuzzParseLFSPointer|FuzzMaterializationDigestEncoding' -count=1
	$(GO) test ./internal/pipeline -run 'FuzzValidateSourceSnapshot|FuzzBuildFrozenPlan|FuzzPlannerIdentifiersAndDigests' -count=1
	$(GO) test ./internal/runner/... -run 'FuzzValidateRunnerCapabilities|FuzzValidateEventDraft|FuzzFakeProgramAttemptKeys' -count=1
	$(GO) test ./internal/evidence -run 'FuzzValidateEvidenceEntryPath|FuzzEncodeEventLine|FuzzValidateLogicalArtifactPath|FuzzNormalizeManifest' -count=1
	$(GO) test ./internal/evidence -run 'FuzzStrictJSONPreflight|FuzzParseEventJSONLine|FuzzReconcileBundleInventory|FuzzValidateArtifactReferences' -count=1
	$(GO) test ./internal/observe -run 'FuzzNormalizeProcessTrace|FuzzNormalizeObservedPath|FuzzEncodeNormalizedFact' -count=1

test-gitstore:
	$(GO) test ./internal/gitstore -count=1

test-gitstore-fuzz-seeds:
	$(GO) test ./internal/gitstore -run 'FuzzParseLsTree|FuzzParseCatFileHeader|FuzzValidateGitTreePath' -count=1

test-materialize:
	$(GO) test ./internal/materialize -count=1

test-materialize-fuzz-seeds:
	$(GO) test ./internal/materialize -run 'FuzzValidateMaterializationInventory|FuzzValidateSymlinkTarget|FuzzParseLFSPointer|FuzzMaterializationDigestEncoding' -count=1

test-pipeline:
	$(GO) test ./internal/pipeline -count=1

test-pipeline-fuzz-seeds:
	$(GO) test ./internal/pipeline -run 'FuzzValidateSourceSnapshot|FuzzBuildFrozenPlan|FuzzPlannerIdentifiersAndDigests' -count=1

test-runner:
	$(GO) test ./internal/runner/... -count=1

test-runner-fuzz-seeds:
	$(GO) test ./internal/runner/... -run 'FuzzValidateRunnerCapabilities|FuzzValidateEventDraft|FuzzFakeProgramAttemptKeys' -count=1

test-evidence:
	$(GO) test ./internal/evidence -count=1

test-evidence-fuzz-seeds:
	$(GO) test ./internal/evidence -run 'FuzzValidateEvidenceEntryPath|FuzzEncodeEventLine|FuzzValidateLogicalArtifactPath|FuzzNormalizeManifest' -count=1

build:
	@tmp="$$(mktemp -t glassroot.XXXXXX)"; \
	$(GO) build -o "$$tmp" ./cmd/glassroot; \
	rm -f "$$tmp"

generate:
	$(GO) generate ./...

verify: fmt-check vet test schema-check build


test-evidence-reader:
	$(GO) test ./internal/evidence -run 'TestOpenAndVerify|TestWalkEvents|TestReader' -count=1

test-evidence-reader-fuzz-seeds:
	$(GO) test ./internal/evidence -run 'FuzzStrictJSONPreflight|FuzzParseEventJSONLine|FuzzReconcileBundleInventory|FuzzValidateArtifactReferences' -count=1


test-observe:
	$(GO) test ./internal/observe -count=1

test-observe-fuzz-seeds:
	$(GO) test ./internal/observe -run 'FuzzNormalizeProcessTrace|FuzzNormalizeObservedPath|FuzzEncodeNormalizedFact' -count=1
