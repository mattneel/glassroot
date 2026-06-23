package config

import (
	"bytes"
	"context"
	"reflect"
	"testing"
)

func FuzzHeadCannotAffectEffectiveConfiguration(f *testing.F) {
	base := readFixture(f, "valid/pipeline.yaml")
	seeds := [][]byte{
		base,
		append([]byte("# comment\n"), base...),
		mutatePipeline(f, base, "cpu: 2", "cpu: 64"),
		mutatePipeline(f, base, "mode: deny", "mode: unrestricted"),
		mutatePipeline(f, base, "allow: []", "allow:\n      - example.invalid"),
		addScenario(base),
		mutatePipeline(f, base, "go build ./cmd/glassroot", "echo changed"),
		[]byte("apiVersion: ["),
		readFixture(f, "invalid/duplicate-key.yaml"),
		readFixture(f, "invalid/alias.yaml"),
		bytes.Repeat([]byte("a"), MaxPipelineBytes+1),
		[]byte("__MISSING__"),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	expected, err := ParseAndValidate(base)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, head []byte) {
		source := newMemoryRevisionSource()
		baseRef := commitRef("base-sha")
		headRef := commitRef("head-sha")
		source.put(baseRef, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: base})
		if bytes.Equal(head, []byte("__MISSING__")) {
			source.fail(headRef, PipelinePath, ErrRevisionFileMissing)
		} else {
			source.put(headRef, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: head})
		}
		result, err := LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: baseRef, Head: headRef})
		assertBoundedSanitizedError(t, err)
		if err != nil {
			return
		}
		if !reflect.DeepEqual(result.EffectivePipeline, expected) {
			t.Fatalf("head affected effective pipeline")
		}
		assertBoundedSanitizedError(t, result.HeadAssessment.Diagnostics)
	})
}
