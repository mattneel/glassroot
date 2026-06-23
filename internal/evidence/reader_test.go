package evidence

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

func TestOpenAndVerifyValidBundleAndPathSafeAccessors(t *testing.T) {
	plan := mustPlan(t)
	parent := t.TempDir()
	writer := mustWriter(t, parent, DefaultLimits())
	session, err := writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	logKey := AttemptKey{Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1}
	stdout, err := session.OpenLog(context.Background(), logKey, LogStreamStdout)
	if err != nil {
		t.Fatalf("OpenLog() error = %v", err)
	}
	if _, err := stdout.Write([]byte("hello\x00\xff\r\n")); err != nil {
		t.Fatalf("stdout write: %v", err)
	}
	if err := stdout.Close(); err != nil {
		t.Fatalf("stdout close: %v", err)
	}
	artifactBytes := []byte("artifact\x00bytes")
	artifactKey := AttemptKey{Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1}
	artifact, err := session.AddArtifact(context.Background(), ArtifactInput{Attempt: artifactKey, LogicalPath: "/workspace/out.bin", Reader: bytes.NewReader(artifactBytes)})
	if err != nil {
		t.Fatalf("AddArtifact() error = %v", err)
	}
	result, err := runner.ExecutePlan(context.Background(), plan, mustFake(t, fakeProgramForPlan(plan)), runner.SyntheticTestRequirements(), runner.DefaultLimits(), session)
	if err != nil {
		t.Fatalf("ExecutePlan() error = %v", err)
	}
	bundleResult, err := session.Commit(context.Background(), Complete(result))
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	bundle, err := OpenAndVerify(context.Background(), bundleResult.Path, DefaultReaderLimits(), WithExpectedManifestDigest(bundleResult.ManifestDigest))
	if err != nil {
		t.Fatalf("OpenAndVerify() error = %v", err)
	}
	defer bundle.Close()
	if bundle.ManifestDigest() != bundleResult.ManifestDigest {
		t.Fatalf("manifest digest = %s, want %s", bundle.ManifestDigest(), bundleResult.ManifestDigest)
	}
	if got := bundle.Verification(); !got.ExpectedManifestDigestSupplied || !got.ExpectedManifestDigestMatched || got.Mode != VerificationModeExpectedManifestDigest {
		t.Fatalf("verification summary mismatch: %+v", got)
	}
	if bundle.Plan().RunID != "run-0001" || bundle.Execution().Runner.Name != "fake" {
		t.Fatalf("plan/execution metadata mismatch")
	}
	if len(bundle.Attempts()) != 4 {
		t.Fatalf("attempts = %d", len(bundle.Attempts()))
	}

	var eventCount int
	if err := bundle.WalkEvents(context.Background(), func(event model.ObservationEvent) error {
		eventCount++
		if event.ID == "" || event.RunID != "run-0001" || event.SequenceNumber != int64(eventCount) {
			t.Fatalf("event ordering/identity mismatch: %+v", event)
		}
		// Mutating the callback value must not affect later events or bundle state.
		event.ID = "mutated"
		return nil
	}); err != nil {
		t.Fatalf("WalkEvents() error = %v", err)
	}
	if eventCount != int(result.TotalEmittedEvents) {
		t.Fatalf("event count = %d, want %d", eventCount, result.TotalEmittedEvents)
	}

	var logOut bytes.Buffer
	copyLog, err := bundle.CopyLog(context.Background(), logKey, LogStreamStdout, &logOut)
	if err != nil {
		t.Fatalf("CopyLog() error = %v", err)
	}
	if !bytes.Equal(logOut.Bytes(), []byte("hello\x00\xff\r\n")) || copyLog.Bytes != int64(logOut.Len()) {
		t.Fatalf("log copy mismatch: result=%+v bytes=%q", copyLog, logOut.Bytes())
	}
	var artifactOut bytes.Buffer
	copyArtifact, err := bundle.CopyArtifact(context.Background(), artifactKey, artifact.LogicalPath, &artifactOut)
	if err != nil {
		t.Fatalf("CopyArtifact() error = %v", err)
	}
	if !bytes.Equal(artifactOut.Bytes(), artifactBytes) || copyArtifact.Digest != artifact.Digest {
		t.Fatalf("artifact copy mismatch: result=%+v bytes=%q", copyArtifact, artifactOut.Bytes())
	}

	manifest := bundle.Manifest()
	manifest.RunID = "mutated"
	if bundle.Manifest().RunID != "run-0001" {
		t.Fatal("Manifest accessor returned aliased data")
	}
	if err := bundle.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := bundle.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if err := bundle.WalkEvents(context.Background(), func(model.ObservationEvent) error { return nil }); err == nil {
		t.Fatal("WalkEvents after Close should fail")
	}
}

func TestOpenAndVerifyRejectsUnsafeFilesystemEntries(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, root string)
		code   ErrorCode
	}{
		{name: "symlink", mutate: func(t *testing.T, root string) {
			if err := os.Symlink("plan.json", filepath.Join(root, "evil-link")); err != nil {
				t.Fatal(err)
			}
		}, code: CodeSymlinkEntry},
		{name: "undeclared file", mutate: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, "extra.json"), []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, code: CodeUndeclaredEntry},
		{name: "hardlink", mutate: func(t *testing.T, root string) {
			if err := os.Link(filepath.Join(root, "plan.json"), filepath.Join(root, "hard-plan")); err != nil {
				t.Skipf("hard link unavailable: %v", err)
			}
		}, code: CodeHardlinkEntry},
		{name: "fifo", mutate: func(t *testing.T, root string) {
			mkfifoForTest(t, filepath.Join(root, "pipe"))
		}, code: CodeSpecialEntry},
		{name: "executable mode", mutate: func(t *testing.T, root string) {
			if err := os.Chmod(filepath.Join(root, "plan.json"), 0o700); err != nil {
				t.Fatal(err)
			}
		}, code: CodeUnexpectedEntryMode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, digest := makeVerifiedBundleForReaderTest(t)
			tc.mutate(t, root)
			_, err := OpenAndVerify(context.Background(), root, DefaultReaderLimits(), WithExpectedManifestDigest(digest))
			assertEvidenceError(t, err, tc.code)
		})
	}
}

func TestOpenAndVerifyRejectsStrictJSONCorruption(t *testing.T) {
	cases := []struct {
		name   string
		mutate func([]byte) []byte
		code   ErrorCode
	}{
		{name: "duplicate", mutate: func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"runId":"run-0001"`), []byte(`"runId":"run-0001","runId":"run-0001"`), 1)
		}, code: CodeDuplicateJSONMember},
		{name: "case variant", mutate: func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"runId"`), []byte(`"RunId"`), 1)
		}, code: CodeInvalidJSONFieldCase},
		{name: "unknown", mutate: func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"runId":"run-0001"`), []byte(`"runId":"run-0001","surprise":true`), 1)
		}, code: CodeUnknownJSONField},
		{name: "noncanonical whitespace", mutate: func(data []byte) []byte {
			return append([]byte("\n"), data...)
		}, code: CodeNoncanonicalJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, digest := makeVerifiedBundleForReaderTest(t)
			p := filepath.Join(root, "manifest.json")
			data := readHostFile(t, p)
			if err := os.WriteFile(p, tc.mutate(data), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := OpenAndVerify(context.Background(), root, DefaultReaderLimits(), WithExpectedManifestDigest(digest))
			assertEvidenceError(t, err, tc.code)
		})
	}
}

func TestOpenAndVerifyRejectsDigestSequenceAndReferenceCorruption(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, root string)
		code   ErrorCode
	}{
		{name: "payload digest", mutate: func(t *testing.T, root string) {
			appendPayloadByte(t, filepath.Join(root, "plan.json"))
		}, code: CodePayloadSizeMismatch},
		{name: "expected manifest", mutate: func(t *testing.T, root string) {}, code: CodeExpectedManifestDigestMismatch},
		{name: "event id", mutate: func(t *testing.T, root string) {
			p := filepath.Join(root, "attempts", "base", "test", "repetition-0001", "events.jsonl")
			data := readHostFile(t, p)
			data = bytes.Replace(data, []byte(`"id":"evt-`), []byte(`"id":"evt-ffffffff`), 1)
			if err := os.WriteFile(p, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}, code: CodePayloadSizeMismatch},
		{name: "orphan object", mutate: func(t *testing.T, root string) {
			dir := filepath.Join(root, "objects", "sha256", "aa")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, strings.Repeat("a", 64)), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, code: CodeUndeclaredEntry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, digest := makeVerifiedBundleForReaderTest(t)
			tc.mutate(t, root)
			opts := []ReaderOption{WithExpectedManifestDigest(digest)}
			if tc.name == "expected manifest" {
				opts = []ReaderOption{WithExpectedManifestDigest(model.Digest("sha256:" + strings.Repeat("0", 64)))}
			}
			_, err := OpenAndVerify(context.Background(), root, DefaultReaderLimits(), opts...)
			assertEvidenceError(t, err, tc.code)
		})
	}
}

func TestOpenAndVerifyWithoutExpectedDigestReportsInternalConsistencyOnly(t *testing.T) {
	root, _ := makeVerifiedBundleForReaderTest(t)
	bundle, err := OpenAndVerify(context.Background(), root, DefaultReaderLimits())
	if err != nil {
		t.Fatalf("OpenAndVerify() error = %v", err)
	}
	defer bundle.Close()
	if got := bundle.Verification(); got.Mode != VerificationModeInternalConsistencyOnly || got.ExpectedManifestDigestSupplied || got.ExpectedManifestDigestMatched {
		t.Fatalf("verification summary = %+v", got)
	}
}

func TestWalkEventsAndCopyOperationsFailClosed(t *testing.T) {
	root, digest := makeVerifiedBundleForReaderTest(t)
	bundle, err := OpenAndVerify(context.Background(), root, DefaultReaderLimits(), WithExpectedManifestDigest(digest))
	if err != nil {
		t.Fatalf("OpenAndVerify() error = %v", err)
	}
	defer bundle.Close()
	callbackErr := errors.New("stop")
	err = bundle.WalkEvents(context.Background(), func(model.ObservationEvent) error { return callbackErr })
	if !errors.Is(err, callbackErr) {
		t.Fatalf("WalkEvents callback error = %v", err)
	}
	if _, err := bundle.CopyLog(context.Background(), AttemptKey{Revision: model.RevisionKindBase, ScenarioID: "missing", Repetition: 1}, LogStreamStdout, io.Discard); err == nil {
		t.Fatal("unknown log attempt accepted")
	}
	if _, err := bundle.CopyArtifact(context.Background(), AttemptKey{Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1}, "../escape", io.Discard); err == nil {
		t.Fatal("invalid logical artifact path accepted")
	}
}

func TestVerifiedBundleStreamingRejectsPostOpenMutation(t *testing.T) {
	root, digest := makeVerifiedBundleForReaderTest(t)
	bundle, err := OpenAndVerify(context.Background(), root, DefaultReaderLimits(), WithExpectedManifestDigest(digest))
	if err != nil {
		t.Fatalf("OpenAndVerify() error = %v", err)
	}
	defer bundle.Close()
	// The reader must not trust bytes merely because OpenAndVerify succeeded; accessors recheck digest and identity.
	eventPath := filepath.Join(root, "attempts", "base", "test", "repetition-0001", "events.jsonl")
	appendPayloadByte(t, eventPath)
	if err := bundle.WalkEvents(context.Background(), func(model.ObservationEvent) error { return nil }); err == nil {
		t.Fatal("WalkEvents accepted mutated event stream")
	}

	root, digest = makeVerifiedBundleForReaderTest(t)
	bundle, err = OpenAndVerify(context.Background(), root, DefaultReaderLimits(), WithExpectedManifestDigest(digest))
	if err != nil {
		t.Fatalf("OpenAndVerify2() error = %v", err)
	}
	defer bundle.Close()
	objectPath := filepath.Join(root, "objects", "sha256")
	var objectFile string
	if err := filepath.WalkDir(objectPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && objectFile == "" {
			objectFile = p
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if objectFile == "" {
		t.Fatal("artifact object not found")
	}
	appendPayloadByte(t, objectFile)
	var out bytes.Buffer
	_, err = bundle.CopyArtifact(context.Background(), AttemptKey{Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1}, "/workspace/out.bin", &out)
	if err == nil {
		t.Fatal("CopyArtifact accepted mutated object")
	}
}

func makeVerifiedBundleForReaderTest(t *testing.T) (string, model.Digest) {
	t.Helper()
	plan := mustPlan(t)
	writer := mustWriter(t, t.TempDir(), DefaultLimits())
	session, err := writer.Begin(context.Background(), plan)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := session.AddArtifact(context.Background(), ArtifactInput{Attempt: AttemptKey{Revision: model.RevisionKindHead, ScenarioID: "build", Repetition: 1}, LogicalPath: "/workspace/out.bin", Reader: strings.NewReader("artifact")}); err != nil {
		t.Fatalf("AddArtifact() error = %v", err)
	}
	result, err := runner.ExecutePlan(context.Background(), plan, mustFake(t, fakeProgramForPlan(plan)), runner.SyntheticTestRequirements(), runner.DefaultLimits(), session)
	if err != nil {
		t.Fatalf("ExecutePlan() error = %v", err)
	}
	bundle, err := session.Commit(context.Background(), Complete(result))
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	return bundle.Path, bundle.ManifestDigest
}

func appendPayloadByte(t *testing.T, p string) {
	t.Helper()
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
}

func mkfifoForTest(t *testing.T, p string) {
	t.Helper()
	if err := makeFIFO(p); err != nil {
		t.Skipf("fifo unavailable: %v", err)
	}
}

var _ = fs.ErrNotExist
