package localrun

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/artifactcollect"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/dockerdev"
)

func FuzzParseLocalRunArguments(f *testing.F) {
	valid := []string{"--git-dir", "/tmp/repo.git", "--base-commit", strings.Repeat("1", 40), "--head-commit", strings.Repeat("2", 40), "--docker-socket", "/tmp/docker.sock", "--run-id", "run-13c", "--created-at", "2026-06-23T12:00:00Z", "--evaluated-at", "2026-06-23T12:30:00Z", "--acknowledge-unsafe-development-runner", dockerdev.UnsafeDevelopmentAcknowledgementText, "/tmp/out"}
	f.Add(strings.Join(valid, "\x00"))
	f.Add("")
	f.Add("--help")
	f.Add("--git-dir\x00relative\x00--base-commit\x00" + strings.Repeat("A", 40))
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 8192 {
			raw = raw[:8192]
		}
		args := []string{}
		if raw != "" {
			args = strings.Split(raw, "\x00")
		}
		_, _ = ParseCLIArgumentsSyntaxOnly(args)
	})
}

func FuzzValidateLocalRunRequest(f *testing.F) {
	f.Add("run-13c", strings.Repeat("1", 40), strings.Repeat("2", 40), "2026-06-23T12:00:00Z", "2026-06-23T12:30:00Z")
	f.Add("Run 13C", strings.Repeat("A", 40), "HEAD", "bad", "2026-06-23T12:30:00-04:00")
	f.Fuzz(func(t *testing.T, runID, base, head, created, evaluated string) {
		if len(runID) > 256 || len(base) > 256 || len(head) > 256 || len(created) > 128 || len(evaluated) > 128 {
			return
		}
		root := t.TempDir()
		out := filepath.Join(root, "out")
		ct, _ := ParseTime(created, CodeInvalidCreatedAt)
		et, _ := ParseTime(evaluated, CodeInvalidEvaluatedAt)
		ack, _ := dockerdev.AcknowledgeUnsafeDevelopmentRunner(dockerdev.UnsafeDevelopmentAcknowledgementText)
		_ = ValidateRequest(Request{OutputDir: out, GitDir: filepath.Join(root, "repo.git"), DockerSocket: filepath.Join(root, "docker.sock"), BaseCommitID: base, HeadCommitID: head, RunID: runID, CreatedAt: ct, EvaluatedAt: et, Acknowledgement: ack})
	})
}

func FuzzBuildAttemptWorkspaceBindings(f *testing.F) {
	f.Add("attempt-1", "/tmp/glassroot/work-1", strings.Repeat("a", 64), uint32(1))
	f.Add("", "relative", "bad", uint32(0))
	f.Fuzz(func(t *testing.T, attemptID, hostPath, digestHex string, repetition uint32) {
		if len(attemptID) > 256 || len(hostPath) > 4096 || len(digestHex) > 256 {
			return
		}
		digest := model.Digest("sha256:" + digestHex)
		_ = dockerBindingForAttempt(runner.AttemptRequest{AttemptID: attemptID, Revision: model.RevisionKindBase, ScenarioID: "scenario", Repetition: repetition, MaterializedTreeDigest: digest}, hostPath)
	})
}

func FuzzTranslateArtifactCollectionResult(f *testing.F) {
	f.Add("/workspace/out.bin", strings.Repeat("b", 64), int64(7), true)
	f.Add("bad\x00path", "digest", int64(-1), false)
	f.Fuzz(func(t *testing.T, logical, digestHex string, size int64, executable bool) {
		if len(logical) > 4096 || len(digestHex) > 256 || size < -1 || size > 1<<30 {
			return
		}
		req := runner.AttemptRequest{AttemptID: "attempt", Revision: model.RevisionKindHead, ScenarioID: "scenario", Repetition: 1}
		art := artifactcollect.ArtifactResult{LogicalPath: logical, Disposition: artifactcollect.ArtifactDispositionStored, ContentDigest: model.Digest("sha256:" + digestHex), SizeBytes: size, Executable: executable, MatchingRuleIDs: []string{"artifact-001"}}
		_ = artifactEvent(req, art)
		art.Disposition = artifactcollect.ArtifactDispositionOmittedLimit
		_ = omissionWarning(art)
		_, _ = collectionPlanForAttempt(model.Digest("sha256:"+strings.Repeat("0", 64)), runner.AttemptRequest{AttemptID: "attempt", Revision: model.RevisionKindBase, ScenarioID: "scenario", Repetition: 1, Workdir: "/workspace", Collection: runner.CollectionSettings{Artifacts: []model.ExpectedArtifactSpec{{LogicalPath: logical, MaxSizeBytes: size}}}})
		_ = context.Background()
	})
}
