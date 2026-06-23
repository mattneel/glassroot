package artifactcollect

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/mattneel/glassroot/internal/model"
)

const testDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

type recordingSink struct {
	calls       []sinkCall
	err         error
	short       bool
	wrongDigest bool
	wrongSize   bool
}

type sinkCall struct {
	attempt  AttemptIdentity
	logical  string
	declared int64
	max      int64
	exec     bool
	bytes    []byte
}

func (s *recordingSink) StoreArtifact(ctx context.Context, in ArtifactInput) (StoredArtifact, error) {
	if s.err != nil {
		return StoredArtifact{}, s.err
	}
	limit := in.DeclaredSize
	if s.short && limit > 0 {
		limit--
	}
	var buf bytes.Buffer
	if _, err := io.CopyN(&buf, in.Reader, limit); err != nil && !errors.Is(err, io.EOF) {
		return StoredArtifact{}, err
	}
	s.calls = append(s.calls, sinkCall{attempt: in.Attempt, logical: in.LogicalPath, declared: in.DeclaredSize, max: in.MaxBytes, exec: in.Executable, bytes: append([]byte(nil), buf.Bytes()...)})
	d := digestBytes(buf.Bytes())
	if s.wrongDigest {
		d = model.Digest("sha256:2222222222222222222222222222222222222222222222222222222222222222")
	}
	size := int64(buf.Len())
	if s.wrongSize {
		size++
	}
	return StoredArtifact{ContentDigest: d, SizeBytes: size}, nil
}

func testPlan(rules ...ArtifactRule) CollectionPlan {
	return CollectionPlan{
		PlanDigest: model.Digest(testDigest),
		Attempt:    AttemptIdentity{AttemptID: "att-head-build-r1", Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1},
		Workdir:    "/workspace",
		Rules:      rules,
	}
}

func bindTestWorkspace(t *testing.T) (string, *BoundWorkspace) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	collector, err := New(DefaultLimits())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	bound, err := collector.BindWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("BindWorkspace() error = %v", err)
	}
	t.Cleanup(func() { _ = bound.Close() })
	return root, bound
}

func writeFile(t *testing.T, root, rel string, mode os.FileMode, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, rel), mode); err != nil {
		t.Fatal(err)
	}
}

func TestCollectStoresStableRegularFileThroughSink(t *testing.T) {
	root, bound := bindTestWorkspace(t)
	payload := []byte{'h', 'e', 'l', 'l', 'o', 0, 0xff, '\n', 0x1b, '[', '3', '1', 'm'}
	writeFile(t, root, "out.bin", 0o755, payload)

	sink := &recordingSink{}
	result, err := bound.Collect(context.Background(), testPlan(ArtifactRule{ID: "artifact-0", Pattern: "/workspace/out.bin", MaxBytes: 1024}), sink)
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if !result.CollectionComplete {
		t.Fatalf("CollectionComplete = false, limitations=%v", result.Limitations)
	}
	if len(sink.calls) != 1 || !bytes.Equal(sink.calls[0].bytes, payload) {
		t.Fatalf("sink calls = %#v", sink.calls)
	}
	if sink.calls[0].logical != "/workspace/out.bin" || sink.calls[0].declared != int64(len(payload)) || !sink.calls[0].exec {
		t.Fatalf("sink metadata = %#v", sink.calls[0])
	}
	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v", result.Artifacts)
	}
	artifact := result.Artifacts[0]
	if artifact.Disposition != ArtifactDispositionStored || artifact.ContentDigest != digestBytes(payload) || artifact.SizeBytes != int64(len(payload)) || !artifact.Executable {
		t.Fatalf("artifact = %#v", artifact)
	}
	if strings.Contains(fmt.Sprintf("%#v", result), root) {
		t.Fatalf("result exposed host path %q: %#v", root, result)
	}
	if strings.Contains(fmt.Sprintf("%#v", result), string(payload)) {
		t.Fatalf("result exposed raw artifact bytes")
	}
}

func TestWorkspaceBindingValidationAndLifecycle(t *testing.T) {
	collector, err := New(DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collector.BindWorkspace(context.Background(), "relative"); !hasCode(err, CodeInvalidWorkspacePath) {
		t.Fatalf("relative path error = %v", err)
	}

	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := collector.BindWorkspace(context.Background(), root); !hasCode(err, CodeWorkspaceModeInvalid) {
		t.Fatalf("wrong mode error = %v", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := collector.BindWorkspace(context.Background(), link); !hasCode(err, CodeWorkspaceSymlink) {
		t.Fatalf("symlink error = %v", err)
	}

	bound, err := collector.BindWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("bind valid = %v", err)
	}
	if err := bound.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := bound.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	_, err = bound.Collect(context.Background(), testPlan(), &recordingSink{})
	if !hasCode(err, CodeWorkspaceOpenFailed) {
		t.Fatalf("Collect after close error = %v", err)
	}
}

func TestCollectRejectsSecondCollectAndPlanMutationDoesNotAffectCollection(t *testing.T) {
	root, bound := bindTestWorkspace(t)
	writeFile(t, root, "out.bin", 0o600, []byte("one"))
	plan := testPlan(ArtifactRule{ID: "artifact-0", Pattern: "/workspace/out.bin", MaxBytes: 10})
	result, err := bound.Collect(context.Background(), plan, &recordingSink{})
	if err != nil || !result.CollectionComplete {
		t.Fatalf("first Collect result=%#v err=%v", result, err)
	}
	plan.Rules[0].Pattern = "/workspace/other"
	_, err = bound.Collect(context.Background(), plan, &recordingSink{})
	if !hasCode(err, CodeInvalidCollectionPlan) {
		t.Fatalf("second Collect error = %v", err)
	}
}

func TestSymlinkSpecialLimitAndNoMatchResultsAreExplicit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fifo setup is linux-specific")
	}
	root, bound := bindTestWorkspace(t)
	writeFile(t, root, "out.bin", 0o600, []byte("12345"))
	if err := os.Symlink("out.bin", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := bound.Collect(context.Background(), testPlan(
		ArtifactRule{ID: "r-limit", Pattern: "/workspace/out.bin", MaxBytes: 4},
		ArtifactRule{ID: "r-symlink", Pattern: "/workspace/link", MaxBytes: 10},
		ArtifactRule{ID: "r-special", Pattern: "/workspace/pipe", MaxBytes: 10},
		ArtifactRule{ID: "r-miss", Pattern: "/workspace/missing", MaxBytes: 10},
	), &recordingSink{})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if result.CollectionComplete {
		t.Fatalf("CollectionComplete = true for omitted artifacts: %#v", result)
	}
	got := map[ArtifactDisposition]int{}
	for _, artifact := range result.Artifacts {
		got[artifact.Disposition]++
	}
	if got[ArtifactDispositionOmittedLimit] != 1 || got[ArtifactDispositionOmittedSymlink] != 1 || got[ArtifactDispositionOmittedSpecial] != 1 {
		t.Fatalf("artifact dispositions = %#v artifacts=%#v", got, result.Artifacts)
	}
	if len(result.Patterns) != 4 || result.Patterns[3].Disposition != PatternDispositionNoMatch {
		t.Fatalf("pattern results = %#v", result.Patterns)
	}
}

func TestHardlinksAndInvalidInventoryFailClosed(t *testing.T) {
	root, bound := bindTestWorkspace(t)
	writeFile(t, root, "original", 0o600, []byte("x"))
	if err := os.Link(filepath.Join(root, "original"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	_, err := bound.Collect(context.Background(), testPlan(ArtifactRule{ID: "r", Pattern: "/workspace/original", MaxBytes: 10}), &recordingSink{})
	if !hasCode(err, CodeHardlinkEntry) {
		t.Fatalf("hardlink error = %v", err)
	}

	root2, bound2 := bindTestWorkspace(t)
	writeFile(t, root2, ".Git/config", 0o600, []byte("x"))
	_, err = bound2.Collect(context.Background(), testPlan(ArtifactRule{ID: "r", Pattern: "/workspace/**", MaxBytes: 10}), &recordingSink{})
	if !hasCode(err, CodeInvalidEntryPath) {
		t.Fatalf(".git error = %v", err)
	}
}

func TestSinkAndMutationFailuresReturnNoResult(t *testing.T) {
	root, bound := bindTestWorkspace(t)
	writeFile(t, root, "out.bin", 0o600, []byte("stable"))
	sinkErr := errors.New("sink failed")
	_, err := bound.Collect(context.Background(), testPlan(ArtifactRule{ID: "r", Pattern: "/workspace/out.bin", MaxBytes: 10}), &recordingSink{err: sinkErr})
	if !hasCode(err, CodeSinkFailed) || !errors.Is(err, sinkErr) {
		t.Fatalf("sink error = %v", err)
	}

	root2, bound2 := bindTestWorkspace(t)
	writeFile(t, root2, "out.bin", 0o600, []byte("stable"))
	bound2.hooks.AfterPreflight = func() error {
		return os.WriteFile(filepath.Join(root2, "new.bin"), []byte("new"), 0o600)
	}
	_, err = bound2.Collect(context.Background(), testPlan(ArtifactRule{ID: "r", Pattern: "/workspace/out.bin", MaxBytes: 10}), &recordingSink{})
	if !hasCode(err, CodeWorkspaceChanged) {
		t.Fatalf("mutation error = %v", err)
	}

	root3, bound3 := bindTestWorkspace(t)
	writeFile(t, root3, "out.bin", 0o600, []byte("stable"))
	outside := filepath.Join(t.TempDir(), "canary")
	if err := os.WriteFile(outside, []byte("do-not-read"), 0o600); err != nil {
		t.Fatal(err)
	}
	bound3.hooks.BeforeOpenFile = func(logical string) error {
		if logical == "/workspace/out.bin" {
			if err := os.Remove(filepath.Join(root3, "out.bin")); err != nil {
				return err
			}
			return os.Symlink(outside, filepath.Join(root3, "out.bin"))
		}
		return nil
	}
	_, err = bound3.Collect(context.Background(), testPlan(ArtifactRule{ID: "r", Pattern: "/workspace/out.bin", MaxBytes: 20}), &recordingSink{})
	if !hasCode(err, CodeFileChanged) {
		t.Fatalf("replace-with-symlink error = %v", err)
	}
	canary, err := os.ReadFile(outside)
	if err != nil || string(canary) != "do-not-read" {
		t.Fatalf("outside canary changed/read unexpectedly: %q err=%v", canary, err)
	}
}

func TestSinkResultValidation(t *testing.T) {
	cases := []struct {
		name string
		sink *recordingSink
		code ErrorCode
	}{
		{"short", &recordingSink{short: true}, CodeSinkShortRead},
		{"wrong-size", &recordingSink{wrongSize: true}, CodeSinkResultMismatch},
		{"wrong-digest", &recordingSink{wrongDigest: true}, CodeSinkResultMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, bound := bindTestWorkspace(t)
			writeFile(t, root, "out.bin", 0o600, []byte("abc"))
			_, err := bound.Collect(context.Background(), testPlan(ArtifactRule{ID: "r", Pattern: "/workspace/out.bin", MaxBytes: 10}), tc.sink)
			if !hasCode(err, tc.code) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestFinalInventoryReconciliationDetectsModeAndContentMutation(t *testing.T) {
	root, bound := bindTestWorkspace(t)
	writeFile(t, root, "out.bin", 0o600, []byte("abc"))
	bound.hooks.AfterStoreFile = func(logical string) error {
		if logical == "/workspace/out.bin" {
			return os.WriteFile(filepath.Join(root, "out.bin"), []byte("xyz"), 0o600)
		}
		return nil
	}
	_, err := bound.Collect(context.Background(), testPlan(ArtifactRule{ID: "r", Pattern: "/workspace/out.bin", MaxBytes: 10}), &recordingSink{})
	if !hasCode(err, CodeFileChanged) && !hasCode(err, CodeWorkspaceChanged) {
		t.Fatalf("content mutation error = %v", err)
	}
}

func digestBytes(data []byte) model.Digest {
	sum := sha256.Sum256(data)
	return model.Digest("sha256:" + fmt.Sprintf("%x", sum[:]))
}

func hasCode(err error, code ErrorCode) bool {
	var ce *Error
	return errors.As(err, &ce) && ce.Code == code
}
