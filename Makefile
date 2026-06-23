GO ?= go
GOFMT ?= gofmt

.PHONY: fmt fmt-check vet lint test test-race test-integration schema-check test-fuzz-seeds test-gitstore test-gitstore-fuzz-seeds test-materialize test-materialize-fuzz-seeds test-pipeline test-pipeline-fuzz-seeds test-runner test-runner-fuzz-seeds test-evidence test-evidence-fuzz-seeds test-evidence-reader test-evidence-reader-fuzz-seeds test-observe test-observe-fuzz-seeds test-compare test-compare-fuzz-seeds test-policy test-policy-fuzz-seeds test-waiver test-waiver-fuzz-seeds test-policy-application test-report test-report-fuzz-seeds test-inspect test-inspect-fuzz-seeds test-demo test-demo-fuzz-seeds demo-golden-check test-dockerengine test-dockerdev test-dockerdev-fuzz-seeds test-dockerdev-integration test-artifactcollect test-artifactcollect-fuzz-seeds test-localrun test-localrun-fuzz-seeds test-localrun-integration test-gvisor-monitor test-gvisor-spike test-gvisor-spike-fuzz-seeds test-gvisor-spike-integration test-githubapp test-githubapp-fuzz-seeds test-githubreceiver test-githubinbox test-githubreceiver-fuzz-seeds build build-receiver generate verify

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
	$(GO) test ./internal/compare -run 'FuzzBuildOccurrenceProfiles|FuzzBuildTypedComparisonAnchor|FuzzEncodeDeltaRecord' -count=1
	$(GO) test ./internal/policy -run 'FuzzClassifyPolicyConfidence|FuzzEncodeFindingID|FuzzEvaluateDeltaRecord' -count=1
	$(GO) test ./internal/waiver -run 'FuzzParseWaiverSet' -count=1
	$(GO) test ./internal/policy -run 'FuzzHeadWaiverCannotAffectEffectiveApplication|FuzzApplyWaiverTargets|FuzzEncodePolicyApplication' -count=1
	$(GO) test ./internal/report -run 'FuzzVisibleDisplayText|FuzzMarkdownCodeSpan|FuzzRenderReportValue|FuzzBuildReportBindings' -count=1
	$(GO) test ./internal/inspect -run 'FuzzParseInspectArguments|FuzzValidateInspectRequest|FuzzBuildPlanReconstructionInputs' -count=1
	$(GO) test ./internal/demo -run 'FuzzParseDemoArguments|FuzzValidateDemoOutputPath|FuzzBuildFakeProgramCoverage|FuzzEncodeDemoMetadata' -count=1
	$(GO) test ./internal/artifactcollect -run 'FuzzValidateArtifactCollectionPath|FuzzMatchArtifactPattern|FuzzReconcileWorkspaceInventories|FuzzValidateArtifactSinkResult|FuzzCollectPlanValidationNoFilesystem' -count=1
	$(GO) test ./internal/localrun -run 'FuzzParseLocalRunArguments|FuzzValidateLocalRunRequest|FuzzBuildAttemptWorkspaceBindings|FuzzTranslateArtifactCollectionResult' -count=1
	$(GO) test ./internal/githubapp -run 'FuzzParseGitHubSignatureHeader|FuzzPreflightGitHubWebhookJSON|FuzzProjectGitHubWebhook|FuzzDecideWebhookReplay|FuzzEncodeGitHubAnalysisTarget|FuzzProjectAdvisoryCheck' -count=1
	$(GO) test ./internal/githubreceiver -run 'FuzzHandleGitHubWebhookRequest|FuzzValidateReceiverFilesystemPaths' -count=1
	$(GO) test ./internal/githubinbox -run 'FuzzDecodeInboxRecord|FuzzDecideInboxAcceptance|FuzzTransitionOutboxLease' -count=1

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

build-receiver:
	@tmp="$$(mktemp -t glassroot-receiver.XXXXXX)"; \
	$(GO) build -o "$$tmp" ./cmd/glassroot-receiver; \
	rm -f "$$tmp"; \
	tmp="$$(mktemp -t glassroot-receiver-linux-amd64.XXXXXX)"; \
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -o "$$tmp" ./cmd/glassroot-receiver; \
	rm -f "$$tmp"; \
	tmp="$$(mktemp -t glassroot-receiver-linux-arm64.XXXXXX)"; \
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -o "$$tmp" ./cmd/glassroot-receiver; \
	rm -f "$$tmp"

generate:
	$(GO) generate ./...

verify: fmt-check vet test schema-check test-gvisor-monitor test-githubreceiver test-githubinbox build build-receiver


test-evidence-reader:
	$(GO) test ./internal/evidence -run 'TestOpenAndVerify|TestWalkEvents|TestReader' -count=1

test-evidence-reader-fuzz-seeds:
	$(GO) test ./internal/evidence -run 'FuzzStrictJSONPreflight|FuzzParseEventJSONLine|FuzzReconcileBundleInventory|FuzzValidateArtifactReferences' -count=1


test-observe:
	$(GO) test ./internal/observe -count=1

test-observe-fuzz-seeds:
	$(GO) test ./internal/observe -run 'FuzzNormalizeProcessTrace|FuzzNormalizeObservedPath|FuzzEncodeNormalizedFact' -count=1

test-compare:
	$(GO) test ./internal/compare -count=1

test-compare-fuzz-seeds:
	$(GO) test ./internal/compare -run 'FuzzBuildOccurrenceProfiles|FuzzBuildTypedComparisonAnchor|FuzzEncodeDeltaRecord' -count=1

test-policy:
	$(GO) test ./internal/policy -count=1

test-policy-fuzz-seeds:
	$(GO) test ./internal/policy -run 'FuzzClassifyPolicyConfidence|FuzzEncodeFindingID|FuzzEvaluateDeltaRecord' -count=1


test-waiver:
	$(GO) test ./internal/waiver -count=1

test-waiver-fuzz-seeds:
	$(GO) test ./internal/waiver -run 'FuzzParseWaiverSet' -count=1
	$(GO) test ./internal/policy -run 'FuzzHeadWaiverCannotAffectEffectiveApplication|FuzzApplyWaiverTargets|FuzzEncodePolicyApplication' -count=1

test-policy-application:
	$(GO) test ./internal/policy -run 'TestApply|TestHeadWaiver|TestInvalidBaseWaiver|TestApplication|TestConfigChange|TestFrozenApplication' -count=1

test-report:
	$(GO) test ./internal/report -count=1

test-report-fuzz-seeds:
	$(GO) test ./internal/report -run 'FuzzVisibleDisplayText|FuzzMarkdownCodeSpan|FuzzRenderReportValue|FuzzBuildReportBindings' -count=1


test-inspect:
	$(GO) test ./internal/inspect ./cmd/glassroot -count=1

test-inspect-fuzz-seeds:
	$(GO) test ./internal/inspect -run 'FuzzParseInspectArguments|FuzzValidateInspectRequest|FuzzBuildPlanReconstructionInputs' -count=1


test-demo:
	$(GO) test ./internal/demo ./cmd/glassroot -count=1

test-demo-fuzz-seeds:
	$(GO) test ./internal/demo -run 'FuzzParseDemoArguments|FuzzValidateDemoOutputPath|FuzzBuildFakeProgramCoverage|FuzzEncodeDemoMetadata' -count=1

demo-golden-check:
	$(GO) test ./internal/demo -run TestGoldenOutputsMatchBuiltInFixtures -count=1


test-dockerengine:
	$(GO) test ./internal/dockerengine -count=1

test-dockerdev:
	$(GO) test ./internal/runner/dockerdev -count=1

test-dockerdev-fuzz-seeds:
	$(GO) test ./internal/dockerengine -run 'FuzzValidateDockerSocketPath|FuzzValidateImmutableLocalImage|FuzzDecodeDockerAttachFrames' -count=1
	$(GO) test ./internal/runner/dockerdev -run 'FuzzValidateDockerDevWorkspace|FuzzBuildContainerConfiguration' -count=1

test-dockerdev-integration:
	GLASSROOT_DOCKERDEV_INTEGRATION=1 $(GO) test ./internal/runner/dockerdev -run TestDockerDevIntegration -count=1


test-artifactcollect:
	$(GO) test ./internal/artifactcollect -count=1

test-artifactcollect-fuzz-seeds:
	$(GO) test ./internal/artifactcollect -run 'FuzzValidateArtifactCollectionPath|FuzzMatchArtifactPattern|FuzzReconcileWorkspaceInventories|FuzzValidateArtifactSinkResult|FuzzCollectPlanValidationNoFilesystem' -count=1


test-localrun:
	$(GO) test ./internal/localrun ./cmd/glassroot -count=1

test-localrun-fuzz-seeds:
	$(GO) test ./internal/localrun -run 'FuzzParseLocalRunArguments|FuzzValidateLocalRunRequest|FuzzBuildAttemptWorkspaceBindings|FuzzTranslateArtifactCollectionResult' -count=1
	$(GO) test ./internal/githubapp -run 'FuzzParseGitHubSignatureHeader|FuzzPreflightGitHubWebhookJSON|FuzzProjectGitHubWebhook|FuzzDecideWebhookReplay|FuzzEncodeGitHubAnalysisTarget|FuzzProjectAdvisoryCheck' -count=1

test-localrun-integration:
	GLASSROOT_LOCALRUN_INTEGRATION=1 $(GO) test ./internal/localrun -run TestLocalRunIntegration -count=1


test-gvisor-monitor:
	cd tools/gvisormonitor && $(GO) test ./... -count=1

test-gvisor-spike:
	$(GO) test ./internal/gvisorspike -count=1

test-gvisor-spike-fuzz-seeds:
	$(GO) test ./internal/gvisorspike -run 'FuzzBuildPodInitConfiguration|FuzzApplyProcessLifecycleEvent' -count=1
	cd tools/gvisormonitor && $(GO) test ./internal/protocol -run FuzzDecodeRemoteEnvelope -count=1
	cd tools/gvisormonitor && $(GO) test ./internal/convert -run FuzzConvertGVisorTracePoint -count=1
	cd tools/gvisormonitor && $(GO) test ./internal/monitor -run FuzzApplyProcessLifecycleEvent -count=1

test-gvisor-spike-integration:
	$(GO) test -v ./internal/gvisorspike -run TestGVisorSpikeIntegration -count=1


test-githubapp:
	$(GO) test ./internal/githubapp -count=1

test-githubapp-fuzz-seeds:
	$(GO) test ./internal/githubapp -run 'FuzzParseGitHubSignatureHeader|FuzzPreflightGitHubWebhookJSON|FuzzProjectGitHubWebhook|FuzzDecideWebhookReplay|FuzzEncodeGitHubAnalysisTarget|FuzzProjectAdvisoryCheck' -count=1


test-githubreceiver:
	$(GO) test ./internal/githubreceiver ./cmd/glassroot-receiver -count=1

test-githubinbox:
	$(GO) test ./internal/githubinbox -count=1

test-githubreceiver-fuzz-seeds:
	$(GO) test ./internal/githubreceiver -run 'FuzzHandleGitHubWebhookRequest|FuzzValidateReceiverFilesystemPaths' -count=1
	$(GO) test ./internal/githubinbox -run 'FuzzDecodeInboxRecord|FuzzDecideInboxAcceptance|FuzzTransitionOutboxLease' -count=1
