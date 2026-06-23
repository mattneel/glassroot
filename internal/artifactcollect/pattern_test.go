package artifactcollect

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestPatternSemanticsAndRuleOrdering(t *testing.T) {
	root, bound := bindTestWorkspace(t)
	writeFile(t, root, "bin/app", 0o600, []byte("app"))
	writeFile(t, root, "bin/deep/log.txt", 0o600, []byte("log"))
	writeFile(t, root, ".hidden", 0o600, []byte("hidden"))

	result, err := bound.Collect(context.Background(), testPlan(
		ArtifactRule{ID: "deep", Pattern: "/workspace/bin/**", MaxBytes: 100},
		ArtifactRule{ID: "question", Pattern: "/workspace/bin/a??", MaxBytes: 100},
		ArtifactRule{ID: "class", Pattern: "/workspace/bin/[ad]pp", MaxBytes: 100},
		ArtifactRule{ID: "dot", Pattern: "/workspace/.*", MaxBytes: 100},
	), &recordingSink{})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if !result.CollectionComplete {
		t.Fatalf("incomplete result: %#v", result)
	}
	got := make([]string, 0, len(result.Artifacts))
	for _, a := range result.Artifacts {
		got = append(got, a.LogicalPath+":"+joinRuleIDs(a.MatchingRuleIDs))
	}
	want := []string{
		"/workspace/.hidden:dot",
		"/workspace/bin/app:class,deep,question",
		"/workspace/bin/deep/log.txt:deep",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifacts = %#v, want %#v", got, want)
	}
}

func TestPlanValidationRejectsUnsafePatterns(t *testing.T) {
	root, bound := bindTestWorkspace(t)
	writeFile(t, root, "out", 0o600, []byte("x"))
	cases := []ArtifactRule{
		{ID: "outside", Pattern: "/workspace2/out", MaxBytes: 10},
		{ID: "relative", Pattern: "workspace/out", MaxBytes: 10},
		{ID: "traversal", Pattern: "/workspace/../out", MaxBytes: 10},
		{ID: "backslash", Pattern: "/workspace\\out", MaxBytes: 10},
		{ID: "bad-glob", Pattern: "/workspace/[", MaxBytes: 10},
		{ID: "embedded-doublestar", Pattern: "/workspace/a**b", MaxBytes: 10},
		{ID: "zero-limit", Pattern: "/workspace/out", MaxBytes: 0},
	}
	for _, rule := range cases {
		t.Run(rule.ID, func(t *testing.T) {
			_, err := bound.Collect(context.Background(), testPlan(rule), &recordingSink{})
			if err == nil {
				t.Fatalf("Collect accepted rule %#v", rule)
			}
		})
		root, bound = bindTestWorkspace(t)
		writeFile(t, root, "out", 0o600, []byte("x"))
	}
}

func TestDuplicatePatternRejectedBeforeSinkWrite(t *testing.T) {
	root, bound := bindTestWorkspace(t)
	writeFile(t, root, "out", 0o600, []byte("x"))
	sink := &recordingSink{}
	_, err := bound.Collect(context.Background(), testPlan(
		ArtifactRule{ID: "a", Pattern: "/workspace/out", MaxBytes: 10},
		ArtifactRule{ID: "b", Pattern: "/workspace/out", MaxBytes: 10},
	), sink)
	if !hasCode(err, CodeDuplicateArtifactPattern) {
		t.Fatalf("duplicate error = %v", err)
	}
	if len(sink.calls) != 0 {
		t.Fatalf("sink called before plan rejection: %#v", sink.calls)
	}
}

func TestBlockedSymlinkDescendantMarksPatternIncomplete(t *testing.T) {
	root, bound := bindTestWorkspace(t)
	if err := os.Mkdir(filepath.Join(root, "real"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "real/out", 0o600, []byte("x"))
	if err := os.Symlink("real", filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	result, err := bound.Collect(context.Background(), testPlan(ArtifactRule{ID: "r", Pattern: "/workspace/linkdir/**", MaxBytes: 10}), &recordingSink{})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if result.CollectionComplete || len(result.Patterns) != 1 || result.Patterns[0].Disposition != PatternDispositionBlockedSymlink {
		t.Fatalf("result = %#v", result)
	}
}

func TestNoMatchIsComplete(t *testing.T) {
	_, bound := bindTestWorkspace(t)
	result, err := bound.Collect(context.Background(), testPlan(ArtifactRule{ID: "missing", Pattern: "/workspace/missing/**", MaxBytes: 10}), &recordingSink{})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if !result.CollectionComplete || len(result.Artifacts) != 0 || len(result.Patterns) != 1 || result.Patterns[0].Disposition != PatternDispositionNoMatch {
		t.Fatalf("result = %#v", result)
	}
}

func joinRuleIDs(ids []string) string {
	ids = append([]string(nil), ids...)
	sort.Strings(ids)
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += id
	}
	return out
}
