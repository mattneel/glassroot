package waiver

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
)

func TestParseValidWaiverSetAndSemanticDigest(t *testing.T) {
	data := validWaiverYAML("known-network", "finding-"+strings.Repeat("1", 64), "GR-NET-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")
	set, err := Parse(data, DefaultLimits())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if set.APIVersion != APIVersionV1Alpha1 || set.Kind != KindWaiverSet || set.MetadataName != "default" {
		t.Fatalf("identity not retained: %+v", set)
	}
	if len(set.Waivers) != 1 || set.Waivers[0].ID != "known-network" || set.Waivers[0].Target.RuleID != "GR-NET-001" {
		t.Fatalf("waiver not decoded: %+v", set.Waivers)
	}
	if set.RawDigest == "" || set.SemanticDigest == "" || set.RawSizeBytes != int64(len(data)) {
		t.Fatalf("digests/size missing: %+v", set)
	}

	reordered := []byte("kind: WaiverSet\napiVersion: glassroot.dev/v1alpha1\nmetadata:\n  name: default\nspec:\n  waivers:\n    - expiresAt: \"2026-07-23T00:00:00Z\"\n      issuedAt: \"2026-06-23T00:00:00Z\"\n      reason: Known deterministic fixture behavior pending removal.\n      owner: mattneel\n      target:\n        ruleId: GR-NET-001\n        findingId: finding-" + strings.Repeat("1", 64) + "\n      id: known-network\n")
	set2, err := Parse(reordered, DefaultLimits())
	if err != nil {
		t.Fatalf("Parse(reordered) error = %v", err)
	}
	if set.SemanticDigest != set2.SemanticDigest {
		t.Fatalf("semantic digest changed for key/list order: %s vs %s", set.SemanticDigest, set2.SemanticDigest)
	}
	if set.RawDigest == set2.RawDigest {
		t.Fatalf("raw digest should capture byte identity")
	}
}

func TestParseRejectsStrictYAMLAndSemanticInvalidity(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		code ErrorCode
	}{
		{"invalid utf8", []byte{0xff}, CodeInvalidUTF8},
		{"nul", []byte("apiVersion: glassroot.dev/v1alpha1\x00\n"), CodeNULByte},
		{"multiple docs", []byte("apiVersion: glassroot.dev/v1alpha1\n---\nkind: WaiverSet\n"), CodeMultipleDocuments},
		{"alias", []byte("apiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata: &m\n  name: default\nspec:\n  waivers: []\ncopy: *m\n"), CodeUnsupportedYAMLFeature},
		{"unknown", []byte("apiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata:\n  name: default\nspec:\n  waivers: []\nextra: nope\n"), CodeUnknownField},
		{"duplicate", []byte("apiVersion: glassroot.dev/v1alpha1\napiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata:\n  name: default\nspec:\n  waivers: []\n"), CodeDuplicateKey},
		{"null required", []byte("apiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata:\n  name: default\nspec:\n  waivers: null\n"), CodeMissingRequiredField},
		{"bad id", validWaiverYAML("Bad", "finding-"+strings.Repeat("1", 64), "GR-NET-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z"), CodeInvalidWaiverID},
		{"uppercase finding", validWaiverYAML("known", "finding-"+strings.Repeat("A", 64), "GR-NET-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z"), CodeInvalidFindingID},
		{"governance rule", validWaiverYAML("known", "finding-"+strings.Repeat("1", 64), "GR-WAIVER-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z"), CodeInvalidRuleID},
		{"fractional time", validWaiverYAML("known", "finding-"+strings.Repeat("1", 64), "GR-NET-001", "2026-06-23T00:00:00.1Z", "2026-07-23T00:00:00Z"), CodeInvalidTime},
		{"too long lifetime", validWaiverYAML("known", "finding-"+strings.Repeat("1", 64), "GR-NET-001", "2026-06-23T00:00:00Z", "2026-09-22T00:00:01Z"), CodeInvalidLifetime},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.data, DefaultLimits())
			assertWaiverError(t, err, tc.code)
		})
	}
}

func TestLoadTrustedUsesFixedPathAndHeadCannotReplaceBase(t *testing.T) {
	base := commit("base")
	head := commit("head")
	source := newMemoryWaiverSource()
	active := validWaiverYAML("known-network", "finding-"+strings.Repeat("1", 64), "GR-NET-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")
	headOnly := validWaiverYAML("head-network", "finding-"+strings.Repeat("2", 64), "GR-NET-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")
	source.put(base, WaiverPath, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: active, ObjectID: "base-waivers"})
	source.put(head, WaiverPath, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: headOnly, ObjectID: "head-waivers"})
	res, err := LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head}, DefaultLimits())
	if err != nil {
		t.Fatalf("LoadTrusted() error = %v", err)
	}
	if got := source.paths(); !reflect.DeepEqual(got, []string{WaiverPath, WaiverPath}) {
		t.Fatalf("paths requested = %v", got)
	}
	if res.Base.State != BaseStateValid || len(res.Base.Waivers) != 1 || res.Base.Waivers[0].ID != "known-network" {
		t.Fatalf("base authority not retained: %+v", res.Base)
	}
	if res.Head.State != HeadStateModifiedValid || len(res.Head.Changes) == 0 {
		t.Fatalf("head assessment not reported: %+v", res.Head)
	}
	res.Base.Waivers[0].ID = "mutated"
	res2, err := LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head}, DefaultLimits())
	if err != nil {
		t.Fatalf("second LoadTrusted() error = %v", err)
	}
	if res2.Base.Waivers[0].ID != "known-network" {
		t.Fatal("returned waiver state aliases caller mutation")
	}
}

func TestLoadTrustedInvalidBaseAppliesNoWaiversButStillInspectsHead(t *testing.T) {
	base := commit("base")
	head := commit("head")
	source := newMemoryWaiverSource()
	source.put(base, WaiverPath, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte("apiVersion: nope\n"), ObjectID: "bad"})
	source.put(head, WaiverPath, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: validWaiverYAML("head", "finding-"+strings.Repeat("1", 64), "GR-NET-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")})
	res, err := LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head}, DefaultLimits())
	if err != nil {
		t.Fatalf("invalid base content should be reportable, not operational error: %v", err)
	}
	if res.Base.State != BaseStateInvalid || len(res.Base.Waivers) != 0 || res.Head.State != HeadStateAdded {
		t.Fatalf("invalid base/head assessment mismatch: %+v", res)
	}
}

func TestLoadTrustedOperationalBaseFailureStopsBeforeHead(t *testing.T) {
	base := commit("base")
	head := commit("head")
	source := newMemoryWaiverSource()
	source.fail(base, WaiverPath, errors.New("storage unavailable"))
	source.put(head, WaiverPath, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: validWaiverYAML("head", "finding-"+strings.Repeat("1", 64), "GR-NET-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")})
	_, err := LoadTrusted(context.Background(), source, TrustedLoadRequest{Base: base, Head: head}, DefaultLimits())
	assertWaiverError(t, err, CodeBaseReadFailed)
	if len(source.calls) != 1 || source.calls[0].revision.CommitID != base.CommitID {
		t.Fatalf("head read occurred after operational base failure: %+v", source.calls)
	}
}

func FuzzParseWaiverSet(f *testing.F) {
	f.Add(validWaiverYAML("known", "finding-"+strings.Repeat("1", 64), "GR-NET-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z"))
	f.Add([]byte("apiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata:\n  name: default\nspec:\n  waivers: []\n"))
	f.Add([]byte("a: &x 1\nb: *x\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = Parse(data, DefaultLimits())
	})
}

func validWaiverYAML(id, findingID, ruleID, issuedAt, expiresAt string) []byte {
	return []byte("apiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata:\n  name: default\nspec:\n  waivers:\n    - id: " + id + "\n      target:\n        findingId: " + findingID + "\n        ruleId: " + ruleID + "\n      owner: mattneel\n      reason: Known deterministic fixture behavior pending removal.\n      issuedAt: \"" + issuedAt + "\"\n      expiresAt: \"" + expiresAt + "\"\n")
}

func commit(name string) model.CommitRef {
	return model.CommitRef{Kind: model.RevisionKindBase, Repository: "repo", Ref: "refs/heads/" + name, CommitID: strings.Repeat("a", 39) + name[:1], ObjectFormat: model.GitObjectFormatSHA1, TreeID: strings.Repeat("b", 40)}
}

type memoryWaiverSource struct {
	files map[string]config.RevisionFile
	errs  map[string]error
	calls []sourceCall
}

type sourceCall struct {
	revision model.CommitRef
	path     string
	maxBytes int64
}

func newMemoryWaiverSource() *memoryWaiverSource {
	return &memoryWaiverSource{files: map[string]config.RevisionFile{}, errs: map[string]error{}}
}
func sourceKey(rev model.CommitRef, path string) string { return rev.CommitID + "\x00" + path }
func (s *memoryWaiverSource) put(rev model.CommitRef, path string, f config.RevisionFile) {
	s.files[sourceKey(rev, path)] = f
}
func (s *memoryWaiverSource) fail(rev model.CommitRef, path string, err error) {
	s.errs[sourceKey(rev, path)] = err
}
func (s *memoryWaiverSource) paths() []string {
	out := make([]string, len(s.calls))
	for i, c := range s.calls {
		out[i] = c.path
	}
	return out
}
func (s *memoryWaiverSource) ReadFile(ctx context.Context, rev model.CommitRef, path string, maxBytes int64) (config.RevisionFile, error) {
	s.calls = append(s.calls, sourceCall{rev, path, maxBytes})
	if err := ctx.Err(); err != nil {
		return config.RevisionFile{}, err
	}
	if err, ok := s.errs[sourceKey(rev, path)]; ok {
		return config.RevisionFile{}, err
	}
	f, ok := s.files[sourceKey(rev, path)]
	if !ok {
		return config.RevisionFile{}, fs.ErrNotExist
	}
	if int64(len(f.Data)) > maxBytes {
		return config.RevisionFile{}, config.ErrRevisionFileTooLarge
	}
	f.Data = append([]byte(nil), f.Data...)
	return f, nil
}

func assertWaiverError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", code)
	}
	var we *Error
	if !errors.As(err, &we) {
		t.Fatalf("error %T is not *waiver.Error: %v", err, err)
	}
	if we.Code != code {
		t.Fatalf("code=%s want=%s err=%v", we.Code, code, err)
	}
	if strings.ContainsAny(err.Error(), "\x1b\r\n") {
		t.Fatalf("raw controls in error: %q", err.Error())
	}
}
