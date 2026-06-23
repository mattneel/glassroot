package artifactcollect

import (
	"context"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/model"
)

func FuzzValidateArtifactCollectionPath(f *testing.F) {
	for _, seed := range []string{"out.bin", "dir/file", "../escape", "a\\b", ".git/config", "unicodé", "bad\x00name", strings.Repeat("a", 300)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, p string) {
		_ = validateInventoryRelativePath(p, DefaultLimits())
	})
}

func FuzzMatchArtifactPattern(f *testing.F) {
	seeds := [][2]string{{"*.txt", "a.txt"}, {"**", "a/b"}, {"bin/**", "bin/app"}, {"[ab]", "a"}, {"a**b", "ab"}}
	for _, seed := range seeds {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, pattern, rel string) {
		if len(pattern) > 4096 || len(rel) > 4096 {
			t.Skip()
		}
		_, _ = matchRelativePattern(pattern, rel, DefaultLimits())
	})
}

func FuzzReconcileWorkspaceInventories(f *testing.F) {
	f.Add("a", int64(1), int64(1))
	f.Add("a/b", int64(2), int64(2))
	f.Fuzz(func(t *testing.T, rel string, sizeA int64, sizeB int64) {
		if len(rel) > 128 {
			t.Skip()
		}
		a := inventory{entries: []entry{{rel: rel, kind: entryRegular, size: sizeA}}}
		b := inventory{entries: []entry{{rel: rel, kind: entryRegular, size: sizeB}}}
		_ = reconcileInventories(a, b)
	})
}

func FuzzValidateArtifactSinkResult(f *testing.F) {
	f.Add("sha256:"+strings.Repeat("a", 64), int64(0), int64(0))
	f.Add("bad", int64(1), int64(1))
	f.Fuzz(func(t *testing.T, digest string, gotSize int64, wantSize int64) {
		_ = validateStoredArtifact(StoredArtifact{ContentDigest: model.Digest(digest), SizeBytes: gotSize}, model.Digest("sha256:"+strings.Repeat("a", 64)), wantSize)
	})
}

func FuzzCollectPlanValidationNoFilesystem(f *testing.F) {
	f.Add("/workspace", "/workspace/**", int64(1))
	f.Add("/workspace", "/workspace2/**", int64(1))
	f.Fuzz(func(t *testing.T, workdir, pattern string, max int64) {
		if len(workdir) > 4096 || len(pattern) > 4096 {
			t.Skip()
		}
		_, _ = validatePlan(context.Background(), CollectionPlan{PlanDigest: model.Digest(testDigest), Attempt: AttemptIdentity{AttemptID: "att", Revision: model.RevisionKindHead, ScenarioID: "s", Repetition: 1}, Workdir: workdir, Rules: []ArtifactRule{{ID: "r", Pattern: pattern, MaxBytes: max}}}, DefaultLimits())
	})
}
