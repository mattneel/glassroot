package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
)

const (
	testBaseCommit = "1111111111111111111111111111111111111111"
	testHeadCommit = "2222222222222222222222222222222222222222"
	testBaseTree   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testHeadTree   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var testCreatedAt = time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)

func TestBuildProducesDeterministicFrozenPlanFromTrustedBase(t *testing.T) {
	request := validBuildRequest(t, validPipelineYAML, validPipelineYAML)

	plan, err := Build(context.Background(), request)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if plan == nil {
		t.Fatal("Build() returned nil plan")
	}

	doc := plan.Document()
	if doc.SchemaVersion != model.SchemaVersionRunPlanV1Alpha1 {
		t.Fatalf("schemaVersion = %q", doc.SchemaVersion)
	}
	if doc.RunID != request.RunID || doc.ID != request.RunID+"-plan" {
		t.Fatalf("run identity not bound: id=%q runId=%q", doc.ID, doc.RunID)
	}
	if !doc.CreatedAt.Equal(testCreatedAt) || doc.CreatedAt.Location() != time.UTC {
		t.Fatalf("createdAt not preserved as UTC: %#v", doc.CreatedAt)
	}
	if doc.PipelineName != "default" {
		t.Fatalf("pipeline name = %q", doc.PipelineName)
	}
	if doc.Configuration == nil {
		t.Fatal("configuration provenance is nil")
	}
	if doc.Configuration.Path != config.PipelinePath || doc.Configuration.Source != model.RevisionKindBase {
		t.Fatalf("configuration provenance mismatch: %+v", doc.Configuration)
	}
	if !validDigestString(string(doc.Configuration.Digest)) || doc.Configuration.SizeBytes <= 0 {
		t.Fatalf("configuration digest/size invalid: %+v", doc.Configuration)
	}
	if doc.Configuration.ObjectID == "" {
		t.Fatalf("configuration object ID should be preserved")
	}
	if doc.ExecutionEnvironment == nil || doc.ExecutionEnvironment.Image != request.Trusted.EffectivePipeline.Image || doc.ExecutionEnvironment.ImageDigest != request.Trusted.EffectivePipeline.ImageDigest || doc.ExecutionEnvironment.Workdir != "/workspace" {
		t.Fatalf("execution environment not represented: %+v", doc.ExecutionEnvironment)
	}

	if len(doc.Revisions) != 2 || doc.Revisions[0].Kind != model.RevisionKindBase || doc.Revisions[1].Kind != model.RevisionKindHead {
		t.Fatalf("revisions not ordered base/head: %+v", doc.Revisions)
	}
	assertRevisionBinding(t, doc.Revisions[0], model.RevisionKindBase, testBaseCommit, testBaseTree, "sha256:3333333333333333333333333333333333333333333333333333333333333333", "sha256:4444444444444444444444444444444444444444444444444444444444444444")
	assertRevisionBinding(t, doc.Revisions[1], model.RevisionKindHead, testHeadCommit, testHeadTree, "sha256:5555555555555555555555555555555555555555555555555555555555555555", "sha256:6666666666666666666666666666666666666666666666666666666666666666")
	if doc.Revisions[0].SourceSummary == nil || doc.Revisions[0].SourceSummary.GitlinkCount != 1 || doc.Revisions[0].SourceSummary.LFSPointerCount != 1 {
		t.Fatalf("base source summary missing materializer facts: %+v", doc.Revisions[0].SourceSummary)
	}
	if len(doc.Revisions[0].SourceLimitations) != 1 || doc.Revisions[0].SourceLimitations[0].Code != "skipped-gitlink" {
		t.Fatalf("base source limitation missing: %+v", doc.Revisions[0].SourceLimitations)
	}

	if len(doc.Scenarios) != 2 {
		t.Fatalf("scenario count = %d", len(doc.Scenarios))
	}
	first := doc.Scenarios[0]
	if first.ID != "test" || first.Name != "Unit tests" || first.Shell != config.ShellBinSH || !strings.Contains(first.Run, "go test ./...") {
		t.Fatalf("trusted scenario fields not carried literally: %+v", first)
	}
	if first.Repetitions != 1 {
		t.Fatalf("scenario repetitions = %d", first.Repetitions)
	}
	if first.Command.WorkingDirectory != "/workspace" || first.Command.TimeoutMillis != 600000 {
		t.Fatalf("command workdir/timeout mismatch: %+v", first.Command)
	}
	if first.Command.Argv == nil || len(first.Command.Argv) != 0 {
		t.Fatalf("planner must not synthesize executable argv, got %+v", first.Command.Argv)
	}
	if first.Command.Environment == nil || len(first.Command.Environment) != 0 || doc.Environment == nil || len(doc.Environment) != 0 {
		t.Fatalf("workload environment must be explicit empty arrays: scenario=%+v plan=%+v", first.Command.Environment, doc.Environment)
	}
	if first.ResourceLimits.CPU != 2 || first.ResourceLimits.MemoryBytes != 2<<30 || first.ResourceLimits.DiskBytes != 4<<30 || first.ResourceLimits.ProcessCount != 256 || first.ResourceLimits.TimeoutMillis != 600000 {
		t.Fatalf("scenario resources mismatch: %+v", first.ResourceLimits)
	}
	if first.NetworkPolicy.Mode != model.NetworkModeDeny || first.NetworkPolicy.Allowed == nil || len(first.NetworkPolicy.Allowed) != 0 {
		t.Fatalf("scenario network policy mismatch: %+v", first.NetworkPolicy)
	}

	if doc.Collection == nil || !reflect.DeepEqual(doc.Collection.FilesystemRoots, []string{"/workspace", "/tmp"}) || len(doc.Collection.Artifacts) != 1 || doc.Collection.LogMaxBytesPerStream != 10<<20 {
		t.Fatalf("collection not represented: %+v", doc.Collection)
	}
	if doc.Comparison == nil || !reflect.DeepEqual(doc.Comparison.IgnoreFields, []string{config.CompareIgnoreEventTimestamp, config.CompareIgnoreProcessPID}) || doc.Comparison.Repetitions != 1 {
		t.Fatalf("comparison not represented: %+v", doc.Comparison)
	}
	if doc.Policy == nil || doc.Policy.Profile != config.PolicyProfileStrict {
		t.Fatalf("policy not represented: %+v", doc.Policy)
	}
	if doc.Platform == nil || doc.Platform.RequiredNetworkMode != model.NetworkModeDeny || doc.Platform.MaxCPU != 64 {
		t.Fatalf("platform constraints missing: %+v", doc.Platform)
	}

	jsonBytes := plan.JSON()
	if !json.Valid(jsonBytes) {
		t.Fatalf("plan JSON is invalid: %s", jsonBytes)
	}
	for _, forbidden := range []string{"/tmp/glassroot", "GLASSROOT_SECRET", "HOME", "PATH"} {
		if bytes.Contains(jsonBytes, []byte(forbidden)) {
			t.Fatalf("plan JSON contains forbidden host/workload environment detail %q: %s", forbidden, jsonBytes)
		}
	}
	if strings.Count(string(jsonBytes), "go test ./...") != 1 {
		t.Fatalf("run command should appear once despite repetitions, json=%s", jsonBytes)
	}
	if !validDigestString(string(plan.Digest())) {
		t.Fatalf("plan digest invalid: %q", plan.Digest())
	}

	again, err := Build(context.Background(), request)
	if err != nil {
		t.Fatalf("second Build() error = %v", err)
	}
	if !bytes.Equal(plan.JSON(), again.JSON()) || plan.Digest() != again.Digest() {
		t.Fatalf("Build not deterministic:\nfirst digest=%s\nsecond digest=%s", plan.Digest(), again.Digest())
	}
}

func TestHeadPipelineCannotAffectExecutionFields(t *testing.T) {
	unchanged := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
	baseline, err := Build(context.Background(), unchanged)
	if err != nil {
		t.Fatalf("Build baseline: %v", err)
	}
	baselineDoc := baseline.Document()

	cases := []struct {
		name string
		head string
	}{
		{"cpu increase", strings.Replace(validPipelineYAML, "cpu: 2", "cpu: 64", 1)},
		{"network invalid", strings.Replace(validPipelineYAML, "mode: deny", "mode: unrestricted", 1)},
		{"scenario addition", strings.Replace(validPipelineYAML, "  collect:\n", "    - id: evil\n      name: Evil\n      shell: /bin/sh\n      run: echo should-not-plan\n      timeout: 1m\n  collect:\n", 1)},
		{"run change", strings.Replace(validPipelineYAML, "go build ./cmd/glassroot", "echo head-only && go build ./cmd/glassroot", 1)},
		{"removed", ""},
		{"invalid yaml", "apiVersion: ["},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validBuildRequest(t, validPipelineYAML, tc.head)
			plan, err := Build(context.Background(), req)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			doc := plan.Document()
			if !executionFieldsEqual(baselineDoc, doc) {
				t.Fatalf("head content affected execution fields\nbaseline=%+v\nmutated=%+v", baselineDoc, doc)
			}
			if strings.Contains(string(plan.JSON()), "should-not-plan") || strings.Contains(string(plan.JSON()), "head-only") {
				t.Fatalf("head run content leaked into plan JSON: %s", plan.JSON())
			}
		})
	}
}

func TestBuildRejectsInconsistentInputsAndPlatformViolations(t *testing.T) {
	t.Run("invalid run id", func(t *testing.T) {
		req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
		req.RunID = "Bad Run"
		_, err := Build(context.Background(), req)
		assertPlannerError(t, err, CodeInvalidRunID)
	})
	t.Run("invalid created at", func(t *testing.T) {
		req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
		req.CreatedAt = time.Date(2026, 2, 3, 4, 5, 6, 0, time.FixedZone("offset", 3600))
		_, err := Build(context.Background(), req)
		assertPlannerError(t, err, CodeInvalidCreatedAt)
	})
	t.Run("base commit mismatch", func(t *testing.T) {
		req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
		req.BaseSource.CommitID = "9999999999999999999999999999999999999999"
		_, err := Build(context.Background(), req)
		assertPlannerError(t, err, CodeTrustedConfigMismatch)
	})
	t.Run("reversed revision kind", func(t *testing.T) {
		req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
		req.BaseSource.RevisionKind = model.RevisionKindHead
		_, err := Build(context.Background(), req)
		assertPlannerError(t, err, CodeRevisionMismatch)
	})
	t.Run("malformed tree id", func(t *testing.T) {
		req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
		req.HeadSource.TreeID = strings.ToUpper(testHeadTree)
		_, err := Build(context.Background(), req)
		assertPlannerError(t, err, CodeInvalidObjectID)
	})
	t.Run("malformed digest", func(t *testing.T) {
		req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
		req.HeadSource.MaterializedTreeDigest = "sha256:not-hex"
		_, err := Build(context.Background(), req)
		assertPlannerError(t, err, CodeInvalidSourceDigest)
	})
	t.Run("platform ceiling exceeded", func(t *testing.T) {
		req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
		req.Platform.MaxCPU = 1
		_, err := Build(context.Background(), req)
		assertPlannerError(t, err, CodePlatformLimitExceeded)
		if err != nil && !strings.Contains(err.Error(), "spec.resources.cpu") {
			t.Fatalf("platform error should identify field, got %v", err)
		}
	})
	t.Run("every platform admission ceiling", func(t *testing.T) {
		cases := []struct {
			name string
			edit func(*BuildRequest)
			path string
		}{
			{"memory", func(r *BuildRequest) { r.Platform.MaxMemoryBytes = (2 << 30) - 1 }, "spec.resources.memoryBytes"},
			{"disk", func(r *BuildRequest) { r.Platform.MaxDiskBytes = (4 << 30) - 1 }, "spec.resources.diskBytes"},
			{"processes", func(r *BuildRequest) { r.Platform.MaxProcessCount = 255 }, "spec.resources.processCount"},
			{"global timeout", func(r *BuildRequest) { r.Platform.MaxGlobalTimeoutMillis = 899999 }, "spec.resources.timeoutMillis"},
			{"scenario timeout", func(r *BuildRequest) { r.Platform.MaxScenarioTimeoutMillis = 599999 }, "spec.scenarios[0].timeoutMillis"},
			{"scenario count", func(r *BuildRequest) { r.Platform.MaxScenarioCount = 1 }, "spec.scenarios"},
			{"repetitions", func(r *BuildRequest) { r.Platform.MaxRepetitions = 0 }, "maxRepetitions"},
			{"filesystem roots", func(r *BuildRequest) { r.Platform.MaxFilesystemRootCount = 1 }, "spec.collect.filesystem.roots"},
			{"artifact count", func(r *BuildRequest) { r.Platform.MaxArtifactCount = 0 }, "maxArtifactCount"},
			{"artifact bytes", func(r *BuildRequest) { r.Platform.MaxArtifactBytes = (50 << 20) - 1 }, "spec.collect.artifacts[0].maxBytes"},
			{"log bytes", func(r *BuildRequest) { r.Platform.MaxLogBytesPerStream = (10 << 20) - 1 }, "spec.collect.logs.maxBytesPerStream"},
			{"plan bytes", func(r *BuildRequest) { r.Platform.MaxPlanJSONBytes = 1024 }, "json"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
				tc.edit(&req)
				_, err := Build(context.Background(), req)
				if tc.name == "repetitions" || tc.name == "artifact count" {
					assertPlannerError(t, err, CodeInvalidPlatformConstraints)
				} else if tc.name == "plan bytes" {
					assertPlannerError(t, err, CodePlanTooLarge)
				} else {
					assertPlannerError(t, err, CodePlatformLimitExceeded)
				}
				if err != nil && !strings.Contains(err.Error(), tc.path) {
					t.Fatalf("error should identify %s, got %v", tc.path, err)
				}
			})
		}
	})
	t.Run("exact platform boundaries succeed", func(t *testing.T) {
		req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
		req.Platform.MaxCPU = 2
		req.Platform.MaxMemoryBytes = 2 << 30
		req.Platform.MaxDiskBytes = 4 << 30
		req.Platform.MaxProcessCount = 256
		req.Platform.MaxGlobalTimeoutMillis = 900000
		req.Platform.MaxScenarioTimeoutMillis = 600000
		req.Platform.MaxScenarioCount = 2
		req.Platform.MaxRepetitions = 1
		req.Platform.MaxFilesystemRootCount = 2
		req.Platform.MaxArtifactCount = 1
		req.Platform.MaxArtifactBytes = 50 << 20
		req.Platform.MaxLogBytesPerStream = 10 << 20
		if _, err := Build(context.Background(), req); err != nil {
			t.Fatalf("exact platform boundaries should succeed: %v", err)
		}
	})
	t.Run("unsupported network policy", func(t *testing.T) {
		req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
		req.Platform.RequiredNetworkMode = model.NetworkModeAllowlist
		_, err := Build(context.Background(), req)
		assertPlannerError(t, err, CodeUnsupportedNetworkPolicy)
	})
	t.Run("cancelled context", func(t *testing.T) {
		req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := Build(ctx, req)
		assertPlannerError(t, err, CodeContextCancelled)
	})
}

func TestFrozenPlanOwnershipAndDeepCopies(t *testing.T) {
	req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
	plan, err := Build(context.Background(), req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	originalDigest := plan.Digest()
	originalJSON := append([]byte(nil), plan.JSON()...)

	req.BaseSource.Limitations[0].Code = "mutated-input"
	req.Trusted.EffectivePipeline.Scenarios[0].Run = "echo mutated"
	if !bytes.Equal(plan.JSON(), originalJSON) || plan.Digest() != originalDigest {
		t.Fatal("FrozenPlan changed after caller-owned input mutation")
	}

	doc := plan.Document()
	doc.Revisions[0].SourceLimitations[0].Code = "mutated-doc"
	doc.Scenarios[0].Run = "echo mutated doc"
	doc.Scenarios[0].Command.Environment = append(doc.Scenarios[0].Command.Environment, model.EnvEntry{Name: "PATH", Value: "/bin"})
	if got := plan.Document(); got.Scenarios[0].Run == "echo mutated doc" || got.Revisions[0].SourceLimitations[0].Code == "mutated-doc" || len(got.Scenarios[0].Command.Environment) != 0 {
		t.Fatalf("Document() did not return a deep copy: %+v", got)
	}

	jsonCopy := plan.JSON()
	jsonCopy[0] = '!'
	if bytes.Equal(plan.JSON(), jsonCopy) || !bytes.Equal(plan.JSON(), originalJSON) {
		t.Fatal("JSON() did not return a copy")
	}
}

func TestPlanDigestSensitivityAndGoldenFixture(t *testing.T) {
	req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
	plan, err := Build(context.Background(), req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	goldenJSON := readTestdata(t, "run-plan.json")
	goldenDigest := strings.TrimSpace(string(readTestdata(t, "run-plan.digest")))
	if !bytes.Equal(plan.JSON(), goldenJSON) {
		t.Fatalf("plan JSON golden mismatch\nwant: %s\n got: %s", goldenJSON, plan.JSON())
	}
	if string(plan.Digest()) != goldenDigest {
		t.Fatalf("digest = %s, want %s", plan.Digest(), goldenDigest)
	}

	mutations := []struct {
		name string
		edit func(BuildRequest) BuildRequest
	}{
		{"image", func(r BuildRequest) BuildRequest {
			r.Trusted.EffectivePipeline.Image = strings.Replace(r.Trusted.EffectivePipeline.Image, "012345", "112345", 1)
			return r
		}},
		{"workdir", func(r BuildRequest) BuildRequest { r.Trusted.EffectivePipeline.Workdir = "/src"; return r }},
		{"resource", func(r BuildRequest) BuildRequest {
			r.Trusted.EffectivePipeline.Resources.MemoryBytes = 3 << 30
			return r
		}},
		{"shell", func(r BuildRequest) BuildRequest {
			r.Trusted.EffectivePipeline.Scenarios[0].Shell = config.ShellBinBash
			return r
		}},
		{"run", func(r BuildRequest) BuildRequest {
			r.Trusted.EffectivePipeline.Scenarios[0].Run += "\necho changed"
			return r
		}},
		{"timeout", func(r BuildRequest) BuildRequest {
			r.Trusted.EffectivePipeline.Scenarios[0].TimeoutMillis = 300000
			return r
		}},
		{"collection", func(r BuildRequest) BuildRequest {
			r.Trusted.EffectivePipeline.Collect.FilesystemRoots = []string{"/workspace"}
			return r
		}},
		{"compare", func(r BuildRequest) BuildRequest {
			r.Trusted.EffectivePipeline.Compare.IgnoreFields = []string{config.CompareIgnoreEventTimestamp}
			return r
		}},
		{"policy", func(r BuildRequest) BuildRequest { r.Trusted.EffectivePipeline.Policy.Profile = "strict-v2"; return r }},
		{"commit", func(r BuildRequest) BuildRequest {
			r.HeadSource.CommitID = "3333333333333333333333333333333333333333"
			r.Trusted.Head.CommitID = r.HeadSource.CommitID
			return r
		}},
		{"tree", func(r BuildRequest) BuildRequest {
			r.HeadSource.TreeID = "cccccccccccccccccccccccccccccccccccccccc"
			return r
		}},
		{"materialized digest", func(r BuildRequest) BuildRequest {
			r.BaseSource.MaterializedTreeDigest = "sha256:7777777777777777777777777777777777777777777777777777777777777777"
			return r
		}},
		{"manifest digest", func(r BuildRequest) BuildRequest {
			r.HeadSource.MaterializationManifestDigest = "sha256:8888888888888888888888888888888888888888888888888888888888888888"
			return r
		}},
		{"limitation", func(r BuildRequest) BuildRequest { r.BaseSource.Limitations[0].Code = "detected-lfs-pointer"; return r }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated, err := Build(context.Background(), mutation.edit(validBuildRequest(t, validPipelineYAML, validPipelineYAML)))
			if err != nil {
				t.Fatalf("Build(mutated) error = %v", err)
			}
			if mutated.Digest() == plan.Digest() || bytes.Equal(mutated.JSON(), plan.JSON()) {
				t.Fatalf("mutation %q did not affect frozen JSON/digest", mutation.name)
			}
		})
	}
}

func TestDigestDomainEncodingIsBoundarySafe(t *testing.T) {
	a := planJSONDigestForTest([]byte("ab"))
	b := planJSONDigestForTest([]byte("a" + "b"))
	if a != b {
		t.Fatalf("identical bytes should digest identically: %s != %s", a, b)
	}
	if planJSONDigestForTest([]byte("a\x00b")) == planJSONDigestForTest([]byte("a")) {
		t.Fatal("digest encoding failed to separate JSON byte lengths")
	}
}

func FuzzValidateSourceSnapshot(f *testing.F) {
	f.Add(string(ObjectFormatSHA1), testBaseCommit, testBaseTree, "sha256:3333333333333333333333333333333333333333333333333333333333333333", int64(1), "skipped-gitlink", "vendor/submodule")
	f.Add(string(ObjectFormatSHA256), strings.Repeat("1", 64), strings.Repeat("2", 64), "sha256:"+strings.Repeat("3", 64), int64(0), "", "")
	f.Add("sha1", "", "", "sha256:nothex", int64(-1), "\x1b[31m", "../escape")
	f.Fuzz(func(t *testing.T, format, commit, tree, digest string, totalBytes int64, code, path string) {
		snapshot := validSourceSnapshot(model.RevisionKindBase, testBaseCommit, testBaseTree, "sha256:3333333333333333333333333333333333333333333333333333333333333333", "sha256:4444444444444444444444444444444444444444444444444444444444444444")
		snapshot.ObjectFormat = ObjectFormat(format)
		snapshot.CommitID = commit
		snapshot.TreeID = tree
		snapshot.MaterializedTreeDigest = model.Digest(digest)
		snapshot.Summary.TotalMaterializedFileBytes = totalBytes
		snapshot.Limitations = []SourceLimitation{{Code: code, Path: path, Summary: "bounded"}}
		_, _ = validateSourceSnapshot(snapshot, model.RevisionKindBase)
	})
}

func FuzzBuildFrozenPlan(f *testing.F) {
	f.Add("run-0001", "echo fuzz", int64(2), "sha256:3333333333333333333333333333333333333333333333333333333333333333")
	f.Add("bad run", "\x1b[31m", int64(0), "bad")
	f.Fuzz(func(t *testing.T, runID, run string, cpu int64, digest string) {
		req := validBuildRequest(t, validPipelineYAML, validPipelineYAML)
		req.RunID = runID
		req.Trusted.EffectivePipeline.Scenarios[0].Run = run
		req.Trusted.EffectivePipeline.Resources.CPU = cpu
		req.BaseSource.MaterializedTreeDigest = model.Digest(digest)
		_, _ = Build(context.Background(), req)
	})
}

func FuzzPlannerIdentifiersAndDigests(f *testing.F) {
	f.Add("run-0001", "sha256:3333333333333333333333333333333333333333333333333333333333333333")
	f.Add("RUN", "SHA256:"+strings.Repeat("a", 64))
	f.Add("\xff", "sha256:"+strings.Repeat("g", 64))
	f.Fuzz(func(t *testing.T, runID, digest string) {
		_ = validateRunID(runID)
		_ = validateDigest(model.Digest(digest))
	})
}

func assertRevisionBinding(t *testing.T, rev model.RevisionPlan, kind model.RevisionKind, commit, tree, materialized, manifest string) {
	t.Helper()
	if rev.Kind != kind || rev.Commit.CommitID != commit || rev.Commit.TreeID != tree || rev.Commit.ObjectFormat != model.GitObjectFormatSHA1 {
		t.Fatalf("revision identity mismatch: %+v", rev)
	}
	if rev.ObjectFormat != model.GitObjectFormatSHA1 || rev.TreeID != tree {
		t.Fatalf("revision exact object fields missing: %+v", rev)
	}
	if string(rev.MaterializedTreeDigest) != materialized || string(rev.MaterializationManifestDigest) != manifest {
		t.Fatalf("revision materialization digests mismatch: %+v", rev)
	}
	if !reflect.DeepEqual(rev.ScenarioIDs, []string{"test", "build"}) {
		t.Fatalf("revision scenario ids = %+v", rev.ScenarioIDs)
	}
}

func executionFieldsEqual(a, b model.RunPlan) bool {
	return reflect.DeepEqual(a.Scenarios, b.Scenarios) &&
		reflect.DeepEqual(a.ResourceLimits, b.ResourceLimits) &&
		reflect.DeepEqual(a.NetworkPolicy, b.NetworkPolicy) &&
		reflect.DeepEqual(a.Environment, b.Environment) &&
		reflect.DeepEqual(a.ExecutionEnvironment, b.ExecutionEnvironment) &&
		reflect.DeepEqual(a.Collection, b.Collection) &&
		reflect.DeepEqual(a.Comparison, b.Comparison) &&
		reflect.DeepEqual(a.Policy, b.Policy) &&
		a.PipelineName == b.PipelineName
}

func assertPlannerError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected planner error %s, got nil", code)
	}
	var perr *Error
	if !errors.As(err, &perr) {
		t.Fatalf("error %T is not *pipeline.Error: %v", err, err)
	}
	if perr.Code != code {
		t.Fatalf("error code = %s, want %s; err=%v", perr.Code, code, err)
	}
	if strings.ContainsAny(err.Error(), "\x1b\r\n") {
		t.Fatalf("error contains raw control characters: %q", err.Error())
	}
}

func validBuildRequest(t *testing.T, baseYAML, headYAML string) BuildRequest {
	t.Helper()
	trusted := loadTrustedForTest(t, baseYAML, headYAML)
	return BuildRequest{
		RunID:     "run-0001",
		CreatedAt: testCreatedAt,
		Trusted:   trusted,
		BaseSource: validSourceSnapshot(model.RevisionKindBase, testBaseCommit, testBaseTree,
			"sha256:3333333333333333333333333333333333333333333333333333333333333333",
			"sha256:4444444444444444444444444444444444444444444444444444444444444444"),
		HeadSource: validSourceSnapshot(model.RevisionKindHead, testHeadCommit, testHeadTree,
			"sha256:5555555555555555555555555555555555555555555555555555555555555555",
			"sha256:6666666666666666666666666666666666666666666666666666666666666666"),
		Platform: defaultPlatformConstraintsForTest(),
	}
}

func loadTrustedForTest(t *testing.T, baseYAML, headYAML string) config.TrustedLoadResult {
	t.Helper()
	base := model.CommitRef{Kind: model.RevisionKindBase, Repository: "https://example.invalid/org/repo.git", Ref: "refs/heads/master", CommitID: testBaseCommit, TreeDigest: model.Digest(testBaseTree)}
	head := model.CommitRef{Kind: model.RevisionKindHead, Repository: "https://example.invalid/org/repo.git", Ref: "refs/pull/7/head", CommitID: testHeadCommit, TreeDigest: model.Digest(testHeadTree)}
	source := &memoryRevisionSource{files: map[string]config.RevisionFile{}}
	source.files[key(base, config.PipelinePath)] = config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(baseYAML), ObjectID: strings.Repeat("a", 40)}
	if headYAML != "" {
		source.files[key(head, config.PipelinePath)] = config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(headYAML), ObjectID: strings.Repeat("b", 40)}
	}
	trusted, err := config.LoadTrusted(context.Background(), source, config.TrustedLoadRequest{Base: base, Head: head})
	if err != nil {
		t.Fatalf("LoadTrusted() error = %v", err)
	}
	return trusted
}

type memoryRevisionSource struct {
	files map[string]config.RevisionFile
}

func (s *memoryRevisionSource) ReadFile(ctx context.Context, revision model.CommitRef, path string, maxBytes int64) (config.RevisionFile, error) {
	if err := ctx.Err(); err != nil {
		return config.RevisionFile{}, err
	}
	file, ok := s.files[key(revision, path)]
	if !ok {
		return config.RevisionFile{}, fs.ErrNotExist
	}
	if int64(len(file.Data)) > maxBytes {
		return config.RevisionFile{}, config.ErrRevisionFileTooLarge
	}
	file.Data = append([]byte(nil), file.Data...)
	return file, nil
}

func key(ref model.CommitRef, path string) string {
	return string(ref.Kind) + ":" + ref.CommitID + ":" + path
}

func validSourceSnapshot(kind model.RevisionKind, commit, tree, treeDigest, manifestDigest string) SourceSnapshot {
	return SourceSnapshot{
		RevisionKind:                  kind,
		CommitID:                      commit,
		TreeID:                        tree,
		ObjectFormat:                  ObjectFormatSHA1,
		MaterializedTreeDigest:        model.Digest(treeDigest),
		MaterializationManifestDigest: model.Digest(manifestDigest),
		Summary:                       SourceSummary{DirectoryCount: 3, RegularFileCount: 4, ExecutableFileCount: 1, SymlinkCount: 1, GitlinkCount: 1, LFSPointerCount: 1, TotalMaterializedFileBytes: 1234, SkippedEntryCount: 1},
		Limitations:                   []SourceLimitation{{Code: "skipped-gitlink", Path: "vendor/submodule", Summary: "gitlink was reported but not traversed or materialized"}},
	}
}

func defaultPlatformConstraintsForTest() PlatformConstraints {
	return PlatformConstraints{
		MaxCPU:                   config.MaxCPU,
		MaxMemoryBytes:           config.MaxMemoryBytes,
		MaxDiskBytes:             config.MaxDiskBytes,
		MaxProcessCount:          config.MaxProcessCount,
		MaxGlobalTimeoutMillis:   config.MaxTimeoutMillis,
		MaxScenarioTimeoutMillis: config.MaxTimeoutMillis,
		MaxScenarioCount:         config.MaxScenarioCount,
		MaxRepetitions:           config.MaxRepetitions,
		MaxFilesystemRootCount:   config.MaxFilesystemRootCount,
		MaxArtifactCount:         config.MaxArtifactCount,
		MaxArtifactBytes:         config.MaxArtifactBytes,
		MaxLogBytesPerStream:     config.MaxLogBytesPerStream,
		MaxPlanJSONBytes:         MaxPlanJSONBytes,
		RequiredNetworkMode:      model.NetworkModeDeny,
	}
}

func validDigestString(s string) bool {
	return strings.HasPrefix(s, "sha256:") && len(s) == len("sha256:")+64
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/v1alpha1/" + name)
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return data
}

const validPipelineYAML = `apiVersion: glassroot.dev/v1alpha1
kind: Pipeline
metadata:
  name: default
spec:
  environment:
    image: docker.io/library/golang:1.26@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    workdir: /workspace
  resources:
    cpu: 2
    memory: 2GiB
    disk: 4GiB
    processes: 256
    timeout: 15m
  network:
    mode: deny
    allow: []
  scenarios:
    - id: test
      name: Unit tests
      shell: /bin/sh
      run: |
        echo "literal shell text stays data"
        go test ./...
      timeout: 10m
    - id: build
      name: Build
      shell: /bin/bash
      run: go build ./cmd/glassroot
      timeout: 5m
  collect:
    filesystem:
      roots:
        - /workspace
        - /tmp
      contents: metadata-and-digests
    artifacts:
      - path: /workspace/bin/**
        maxBytes: 50MiB
    logs:
      maxBytesPerStream: 10MiB
  compare:
    ignore:
      - field: event.timestamp
      - field: process.pid
    repetitions: 1
  policy:
    profile: strict
`
