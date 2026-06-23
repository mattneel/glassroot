package inspect

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/model"
)

func TestValidateRequestRequiresExplicitTrustAnchors(t *testing.T) {
	inspector, err := New(DefaultLimits())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = inspector.Inspect(context.Background(), Request{})
	assertInspectError(t, err, CodeInvalidBundlePath)

	req := validRequestForValidation(t)
	req.ManifestIntegrityMode = ""
	_, err = inspector.Inspect(context.Background(), req)
	assertInspectError(t, err, CodeIntegrityModeRequired)

	req = validRequestForValidation(t)
	req.ManifestIntegrityMode = ManifestIntegrityExpectedDigest
	req.AllowInternalConsistencyOnly = true
	_, err = inspector.Inspect(context.Background(), req)
	assertInspectError(t, err, CodeConflictingIntegrityMode)
}

func TestParseEvaluatedAtExactUTC(t *testing.T) {
	got, err := ParseEvaluatedAt("2026-06-23T00:00:00Z")
	if err != nil {
		t.Fatalf("ParseEvaluatedAt() error = %v", err)
	}
	if got.Location() != time.UTC || got.Format(TimeFormat) != "2026-06-23T00:00:00Z" {
		t.Fatalf("parsed time = %s loc=%v", got.Format(time.RFC3339Nano), got.Location())
	}
	for _, input := range []string{"", "2026-06-23T00:00:00.1Z", "2026-06-23T00:00:00+00:00", " 2026-06-23T00:00:00Z", "2026-06-23T00:00:00Z "} {
		if _, err := ParseEvaluatedAt(input); err == nil {
			t.Fatalf("ParseEvaluatedAt(%q) succeeded, want error", input)
		}
	}
}

func TestValidateRequestRejectsRefsAndRelativePaths(t *testing.T) {
	bad := []struct {
		name string
		edit func(*Request)
		code ErrorCode
	}{
		{"relative bundle", func(r *Request) { r.BundleDir = "relative" }, CodeInvalidBundlePath},
		{"relative git", func(r *Request) { r.GitDir = "relative" }, CodeInvalidGitDir},
		{"unclean bundle", func(r *Request) { r.BundleDir = t.TempDir() + "/x/.." }, CodeInvalidBundlePath},
		{"uppercase oid", func(r *Request) { r.BaseCommitID = "A" + r.BaseCommitID[1:] }, CodeInvalidBaseCommit},
		{"short oid", func(r *Request) { r.HeadCommitID = strings.Repeat("1", 39) }, CodeInvalidHeadCommit},
		{"ref", func(r *Request) { r.BaseCommitID = "refs/heads/main" }, CodeInvalidBaseCommit},
		{"malformed digest", func(r *Request) { r.ExpectedManifestDigest = model.Digest("sha256:" + strings.Repeat("A", 64)) }, CodeInvalidManifestDigest},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequestForValidation(t)
			tc.edit(&req)
			_, err := NewMust(t).Inspect(context.Background(), req)
			assertInspectError(t, err, tc.code)
		})
	}
}

func FuzzParseInspectArguments(f *testing.F) {
	f.Add("inspect", "--git-dir", "/tmp/repo.git", "--base-commit", strings.Repeat("1", 40), "--head-commit", strings.Repeat("2", 40), "--evaluated-at", "2026-06-23T00:00:00Z", "--expected-manifest-digest", "sha256:"+strings.Repeat("3", 64), "--format", "terminal", "/tmp/bundle")
	f.Add("inspect", "--allow-internal-consistency-only", "relative", "", "", "", "", "", "", "", "", "", "", "")
	f.Fuzz(func(t *testing.T, a0, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13 string) {
		_, _ = ParseCLIArguments([]string{a0, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13})
	})
}

func FuzzValidateInspectRequest(f *testing.F) {
	f.Add("/tmp/bundle", "/tmp/repo.git", strings.Repeat("1", 40), strings.Repeat("2", 40), "sha256:"+strings.Repeat("3", 64), "2026-06-23T00:00:00Z")
	f.Add("relative", "/tmp/repo.git", "HEAD", strings.Repeat("2", 64), "sha256:bad", "2026-06-23T00:00:00+00:00")
	f.Fuzz(func(t *testing.T, bundleDir, gitDir, base, head, digest, at string) {
		tm, _ := ParseEvaluatedAt(at)
		req := Request{BundleDir: bundleDir, GitDir: gitDir, BaseCommitID: base, HeadCommitID: head, EvaluatedAt: tm, ManifestIntegrityMode: ManifestIntegrityExpectedDigest, ExpectedManifestDigest: model.Digest(digest)}
		_ = ValidateRequest(req)
	})
}

func FuzzBuildPlanReconstructionInputs(f *testing.F) {
	f.Add(strings.Repeat("1", 40), strings.Repeat("a", 40), "sha1")
	f.Add(strings.Repeat("2", 64), strings.Repeat("b", 64), "sha256")
	f.Fuzz(func(t *testing.T, commit, tree, format string) {
		_, _ = sourceSnapshotFromRevisionForTest(commit, tree, format)
	})
}

func validRequestForValidation(t *testing.T) Request {
	t.Helper()
	at, err := ParseEvaluatedAt("2026-06-23T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	return Request{BundleDir: filepath.Join(t.TempDir(), "bundle"), GitDir: filepath.Join(t.TempDir(), "repo.git"), BaseCommitID: strings.Repeat("1", 40), HeadCommitID: strings.Repeat("2", 40), EvaluatedAt: at, ManifestIntegrityMode: ManifestIntegrityExpectedDigest, ExpectedManifestDigest: model.Digest("sha256:" + strings.Repeat("3", 64))}
}

func NewMust(t *testing.T) *Inspector {
	t.Helper()
	i, err := New(DefaultLimits())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return i
}

func assertInspectError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	var ie *Error
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if ie.Code != code {
		t.Fatalf("code = %s, want %s (err=%v)", ie.Code, code, err)
	}
}
