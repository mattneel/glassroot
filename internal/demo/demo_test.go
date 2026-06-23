package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/gitstore"
	"github.com/mattneel/glassroot/internal/inspect"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/report"
)

func TestCreateBehaviorChangePublishesDeterministicInspectableDemo(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("demo creation is linux-only")
	}
	out := filepath.Join(t.TempDir(), "behavior-change-demo")
	d, err := New(DefaultLimits())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := d.Create(context.Background(), Request{Fixture: FixtureBehaviorChange, OutputDir: out})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res == nil || res.Report == nil {
		t.Fatalf("Create returned nil result/report")
	}
	if res.EffectiveDisposition != model.DispositionRequiresReview || res.ExpectedExitCode != 4 {
		t.Fatalf("disposition=%s exit=%d, want requires-review/4", res.EffectiveDisposition, res.ExpectedExitCode)
	}
	assertOutputLayout(t, out)
	assertNoHostPathInPublishedFiles(t, out)
	assertReportFilesMatchResult(t, out, res.Report)
	assertMetadataMatchesResult(t, out, res, FixtureBehaviorChangeID)
	assertFixtureGitStore(t, out, res.Metadata)
	assertEvidenceVerifies(t, out, res.ManifestDigest)
	assertInspectMatchesPublishedReports(t, out, res.Metadata)

	doc := res.Report.Document()
	rules := rulesInReport(doc)
	for _, rule := range []string{"GR-OBS-001", "GR-PROC-001", "GR-NET-001", "GR-FS-001", "GR-ART-001"} {
		if !rules[rule] {
			t.Fatalf("missing expected rule %s; rules=%v", rule, sortedRuleList(rules))
		}
	}
	if len(doc.Policy.AppliedFindings) == 0 || len(doc.Behavior.Records) == 0 {
		t.Fatalf("report omitted findings or delta records")
	}
	if !bytes.Contains(mustRead(t, filepath.Join(out, "report.json")), []byte("canary.invalid")) {
		t.Fatalf("denied network destination not visible in report JSON")
	}
	for _, rec := range res.Metadata.KeyEvidence {
		if len(rec.EventIDs) == 0 || rec.EventStreamPath == "" || rec.EventStreamDigest == "" {
			t.Fatalf("key evidence record is incomplete: %+v", rec)
		}
	}
	assertKeyEvidenceEventsExist(t, out, res.Metadata)
}

func TestCreateControlPublishesSyntheticControlWithoutOrdinaryBehaviorFindings(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("demo creation is linux-only")
	}
	out := filepath.Join(t.TempDir(), "control-demo")
	d, err := New(DefaultLimits())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := d.Create(context.Background(), Request{Fixture: FixtureControl, OutputDir: out})
	if err != nil {
		t.Fatalf("Create control: %v", err)
	}
	assertOutputLayout(t, out)
	assertReportFilesMatchResult(t, out, res.Report)
	assertInspectMatchesPublishedReports(t, out, res.Metadata)
	rules := rulesInReport(res.Report.Document())
	for _, ordinary := range []string{"GR-PROC-001", "GR-NET-001", "GR-FS-001", "GR-FS-002", "GR-ART-001", "GR-DET-001", "GR-LIMIT-001"} {
		if rules[ordinary] {
			t.Fatalf("control produced ordinary behavior rule %s; rules=%v", ordinary, sortedRuleList(rules))
		}
	}
	if rules["GR-OBS-001"] {
		t.Fatalf("control should not invent an observation finding when comparison produced no delta records; rules=%v", sortedRuleList(rules))
	}
	notices := noticeCodes(res.Report.Document())
	for _, code := range []string{"fake-runner", "synthetic-evidence", "no-target-code-executed", "passed-is-not-proof-of-safety"} {
		if !notices[code] {
			t.Fatalf("control report missing notice %s; notices=%v", code, sortedRuleList(notices))
		}
	}
	if res.ExpectedExitCode != exitCodeForDisposition(res.EffectiveDisposition) {
		t.Fatalf("metadata exit/disposition mismatch: %d %s", res.ExpectedExitCode, res.EffectiveDisposition)
	}
	if res.Metadata.FixtureID != FixtureControlID {
		t.Fatalf("metadata fixture id = %q", res.Metadata.FixtureID)
	}
}

func TestGoldenOutputsMatchBuiltInFixtures(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("demo creation is linux-only")
	}
	d, err := New(DefaultLimits())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fixtures := []Fixture{FixtureBehaviorChange, FixtureControl}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(string(fixture), func(t *testing.T) {
			out := filepath.Join(t.TempDir(), string(fixture))
			if _, err := d.Create(context.Background(), Request{Fixture: fixture, OutputDir: out}); err != nil {
				t.Fatalf("Create: %v", err)
			}
			prefix := filepath.Join("testdata", "v1alpha1", string(fixture))
			for _, item := range []struct{ suffix, rel string }{
				{".demo.json", "demo.json"},
				{".report.json", "report.json"},
				{".report.md", "report.md"},
				{".report.txt", "report.txt"},
			} {
				want := mustRead(t, prefix+item.suffix)
				got := mustRead(t, filepath.Join(out, item.rel))
				if !bytes.Equal(got, want) {
					t.Fatalf("%s golden mismatch", item.rel)
				}
			}
			wantID := mustRead(t, prefix+".identities.json")
			gotID := identityGoldenBytes(t, out)
			if !bytes.Equal(gotID, wantID) {
				t.Fatalf("identities golden mismatch\ngot  %s\nwant %s", gotID, wantID)
			}
		})
	}
}

type identityGolden struct {
	FixtureID               string            `json:"fixtureId"`
	BaseCommitID            string            `json:"baseCommitId"`
	BaseTreeID              string            `json:"baseTreeId"`
	HeadCommitID            string            `json:"headCommitId"`
	HeadTreeID              string            `json:"headTreeId"`
	PlanDigest              model.Digest      `json:"planDigest"`
	ManifestDigest          model.Digest      `json:"manifestDigest"`
	BehavioralDeltaDigest   model.Digest      `json:"behavioralDeltaDigest"`
	PolicyEvaluationDigest  model.Digest      `json:"policyEvaluationDigest"`
	PolicyApplicationDigest model.Digest      `json:"policyApplicationDigest"`
	ReportDigest            model.Digest      `json:"reportDigest"`
	MarkdownDigest          model.Digest      `json:"markdownDigest"`
	TerminalDigest          model.Digest      `json:"terminalDigest"`
	KeyEventIDs             []string          `json:"keyEventIds"`
	KeyFindingIDs           []string          `json:"keyFindingIds"`
	KeyDeltaRecordIDs       []string          `json:"keyDeltaRecordIds"`
	FindingCount            int               `json:"findingCount"`
	DeltaRecordCount        int               `json:"deltaRecordCount"`
	ExpectedDisposition     model.Disposition `json:"expectedDisposition"`
	ExpectedCLIExitCode     int               `json:"expectedCliExitCode"`
}

func identityGoldenBytes(t *testing.T, root string) []byte {
	t.Helper()
	var md Metadata
	if err := json.Unmarshal(mustRead(t, filepath.Join(root, "demo.json")), &md); err != nil {
		t.Fatalf("decode demo.json: %v", err)
	}
	var doc report.Document
	if err := json.Unmarshal(mustRead(t, filepath.Join(root, "report.json")), &doc); err != nil {
		t.Fatalf("decode report.json: %v", err)
	}
	id := identityGolden{FixtureID: md.FixtureID, BaseCommitID: md.BaseCommitID, BaseTreeID: md.BaseTreeID, HeadCommitID: md.HeadCommitID, HeadTreeID: md.HeadTreeID, PlanDigest: md.PlanDigest, ManifestDigest: md.ManifestDigest, BehavioralDeltaDigest: md.BehavioralDeltaDigest, PolicyEvaluationDigest: md.PolicyEvaluationDigest, PolicyApplicationDigest: md.PolicyApplicationDigest, ReportDigest: md.ReportDigest, MarkdownDigest: md.MarkdownDigest, TerminalDigest: md.TerminalDigest, KeyEventIDs: []string{}, KeyFindingIDs: []string{}, KeyDeltaRecordIDs: []string{}, FindingCount: len(doc.Policy.AppliedFindings), DeltaRecordCount: len(doc.Behavior.Records), ExpectedDisposition: md.EffectiveDisposition, ExpectedCLIExitCode: md.ExpectedCLIExitCode}
	for _, rec := range md.KeyEvidence {
		id.KeyEventIDs = append(id.KeyEventIDs, rec.EventIDs...)
		if rec.FindingID != "" {
			id.KeyFindingIDs = append(id.KeyFindingIDs, rec.FindingID)
		}
		id.KeyDeltaRecordIDs = append(id.KeyDeltaRecordIDs, rec.DeltaRecordIDs...)
	}
	id.KeyEventIDs = sortedUniqueStringList(id.KeyEventIDs)
	id.KeyFindingIDs = sortedUniqueStringList(id.KeyFindingIDs)
	id.KeyDeltaRecordIDs = sortedUniqueStringList(id.KeyDeltaRecordIDs)
	b, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("marshal identity golden: %v", err)
	}
	return b
}

func sortedUniqueStringList(in []string) []string {
	sort.Strings(in)
	out := in[:0]
	for _, v := range in {
		if v == "" || (len(out) > 0 && out[len(out)-1] == v) {
			continue
		}
		out = append(out, v)
	}
	if out == nil {
		return []string{}
	}
	return out
}

func TestCreateRejectsExistingOrRelativeOutput(t *testing.T) {
	d, err := New(DefaultLimits())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := d.Create(context.Background(), Request{Fixture: FixtureBehaviorChange, OutputDir: "relative-demo"}); err == nil || !IsUsageError(err) {
		t.Fatalf("relative output error = %v, want usage error", err)
	}
	existing := t.TempDir()
	if _, err := d.Create(context.Background(), Request{Fixture: FixtureBehaviorChange, OutputDir: existing}); err == nil || !IsUsageError(err) {
		t.Fatalf("existing output error = %v, want usage error", err)
	}
}

func TestCreateIsDeterministicAcrossOutputParents(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("demo creation is linux-only")
	}
	d, err := New(DefaultLimits())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	outA := filepath.Join(t.TempDir(), "demo-a")
	outB := filepath.Join(t.TempDir(), "demo-b")
	resA, err := d.Create(context.Background(), Request{Fixture: FixtureBehaviorChange, OutputDir: outA})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	resB, err := d.Create(context.Background(), Request{Fixture: FixtureBehaviorChange, OutputDir: outB})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	if resA.BaseCommitID != resB.BaseCommitID || resA.HeadCommitID != resB.HeadCommitID || resA.ManifestDigest != resB.ManifestDigest || resA.Report.Digest() != resB.Report.Digest() {
		t.Fatalf("identities differed: A=%+v B=%+v", resA.Metadata, resB.Metadata)
	}
	for _, rel := range []string{"demo.json", "report.json", "report.md", "report.txt"} {
		if !bytes.Equal(mustRead(t, filepath.Join(outA, rel)), mustRead(t, filepath.Join(outB, rel))) {
			t.Fatalf("%s differed across parents", rel)
		}
	}
}

func TestParseDemoArgumentsFuzzSeeds(t *testing.T) {
	seeds := [][]string{
		{"demo", "fake", "--help"},
		{"demo", "fake", "/tmp/out"},
		{"demo", "fake", "--fixture", "control", "--format", "json", "/tmp/out"},
		{"demo", "fake", "--fixture", "unknown", "/tmp/out"},
		{"demo", "fake", "relative"},
		{"demo", "fake", "--format", "terminal", "--format", "json", "/tmp/out"},
	}
	for _, args := range seeds {
		_, _ = ParseCLIArguments(args[2:])
	}
}

func FuzzParseDemoArguments(f *testing.F) {
	for _, s := range []string{"--help", "--fixture control /tmp/out", "--format json /tmp/out", "--fixture bad relative", "--format terminal --format json /tmp/out"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		parts := strings.Fields(s)
		if len(parts) > 32 {
			parts = parts[:32]
		}
		_, _ = ParseCLIArguments(parts)
	})
}

func FuzzValidateDemoOutputPath(f *testing.F) {
	for _, s := range []string{"/tmp/demo", "relative", "/tmp/a\x00b", strings.Repeat("a", 5000)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { _ = ValidateOutputPathSyntaxForTest(s) })
}

func FuzzBuildFakeProgramCoverage(f *testing.F) {
	f.Add(uint8(1), uint8(1), uint8(0))
	f.Add(uint8(2), uint8(2), uint8(0))
	f.Add(uint8(2), uint8(1), uint8(1))
	f.Fuzz(func(t *testing.T, reps, scripts, mode uint8) {
		r := int(reps%4) + 1
		s := int(scripts % 8)
		_ = ValidateFakeProgramCoverageForTest(r, s, int(mode))
	})
}

func FuzzEncodeDemoMetadata(f *testing.F) {
	f.Add("behavior-change", "sha256:"+strings.Repeat("1", 64))
	f.Add("control", "not-a-digest")
	f.Fuzz(func(t *testing.T, fixture, digest string) {
		md := Metadata{SchemaVersion: SchemaVersionDemoV1Alpha1, FixtureID: fixture, ManifestDigest: model.Digest(digest), RelativePaths: MetadataPaths{FixtureGit: "fixture.git", Evidence: "evidence", ReportJSON: "report.json", ReportMarkdown: "report.md", ReportTerminal: "report.txt"}, KeyEvidence: []KeyEvidenceRecord{}}
		_, _ = EncodeMetadataForTest(md)
	})
}

func assertOutputLayout(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir output: %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Name())
		info, err := e.Info()
		if err != nil {
			t.Fatalf("Info(%s): %v", e.Name(), err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("symlink in output: %s", e.Name())
		}
		if !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			t.Fatalf("executable published file: %s %s", e.Name(), info.Mode())
		}
	}
	want := []string{"demo.json", "evidence", "fixture.git", "report.json", "report.md", "report.txt"}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("layout = %v, want %v", got, want)
	}
	for _, rel := range []string{"demo.json", "report.json", "report.md", "report.txt"} {
		st, err := os.Stat(filepath.Join(root, rel))
		if err != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode/stat = %v %v", rel, st, err)
		}
	}
}

func assertReportFilesMatchResult(t *testing.T, root string, fr *report.FrozenReport) {
	t.Helper()
	if !bytes.Equal(mustRead(t, filepath.Join(root, "report.json")), fr.JSON()) {
		t.Fatalf("report.json differs from FrozenReport.JSON")
	}
	mdOut, err := report.RenderMarkdown(context.Background(), fr, report.DefaultRenderLimits())
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !bytes.Equal(mustRead(t, filepath.Join(root, "report.md")), mdOut.Bytes) {
		t.Fatalf("report.md differs from renderer")
	}
	txt, err := report.RenderTerminal(context.Background(), fr, report.DefaultRenderLimits())
	if err != nil {
		t.Fatalf("RenderTerminal: %v", err)
	}
	if !bytes.Equal(mustRead(t, filepath.Join(root, "report.txt")), txt.Bytes) {
		t.Fatalf("report.txt differs from renderer")
	}
}

func assertMetadataMatchesResult(t *testing.T, root string, res *Result, fixtureID string) {
	t.Helper()
	var md Metadata
	if err := json.Unmarshal(mustRead(t, filepath.Join(root, "demo.json")), &md); err != nil {
		t.Fatalf("decode demo.json: %v", err)
	}
	if md.SchemaVersion != SchemaVersionDemoV1Alpha1 || md.FixtureID != fixtureID || md.RunID == "" || md.BaseCommitID != res.BaseCommitID || md.HeadCommitID != res.HeadCommitID || md.ManifestDigest != res.ManifestDigest || md.ReportDigest != res.Report.Digest() || md.EffectiveDisposition != res.EffectiveDisposition {
		t.Fatalf("metadata mismatch: file=%+v result=%+v", md, res.Metadata)
	}
	if strings.Contains(string(mustRead(t, filepath.Join(root, "demo.json"))), root) {
		t.Fatalf("demo.json contains absolute output path")
	}
}

func assertFixtureGitStore(t *testing.T, root string, md Metadata) {
	t.Helper()
	gitDir := filepath.Join(root, "fixture.git")
	for _, rel := range []string{"hooks", "objects/info/alternates", "info/grafts", "modules"} {
		if _, err := os.Lstat(filepath.Join(gitDir, rel)); err == nil {
			t.Fatalf("unsafe fixture git entry exists: %s", rel)
		}
	}
	cfg := mustRead(t, filepath.Join(gitDir, "config"))
	for _, forbidden := range []string{"[remote", "include", "worktree", "promisor"} {
		if bytes.Contains(cfg, []byte(forbidden)) {
			t.Fatalf("fixture git config contains forbidden token %q", forbidden)
		}
	}
	repo, err := gitstore.Open(context.Background(), gitDir)
	if err != nil {
		t.Fatalf("gitstore.Open fixture: %v", err)
	}
	defer repo.Close()
	base, err := repo.ResolveCommit(context.Background(), gitstore.ObjectIDSelector(md.BaseCommitID))
	if err != nil {
		t.Fatalf("resolve base: %v", err)
	}
	head, err := repo.ResolveCommit(context.Background(), gitstore.ObjectIDSelector(md.HeadCommitID))
	if err != nil {
		t.Fatalf("resolve head: %v", err)
	}
	if base.TreeID != md.BaseTreeID || head.TreeID != md.HeadTreeID {
		t.Fatalf("tree IDs mismatch: base=%s/%s head=%s/%s", base.TreeID, md.BaseTreeID, head.TreeID, md.HeadTreeID)
	}
	source := gitstore.NewRevisionFileSource(repo)
	baseRef := model.CommitRef{Kind: model.RevisionKindBase, Repository: "glassroot.dev/fake-demo", Ref: "refs/heads/main", CommitID: md.BaseCommitID, ObjectFormat: model.GitObjectFormatSHA1, TreeID: md.BaseTreeID, TreeDigest: model.Digest(md.BaseTreeID)}
	headRef := model.CommitRef{Kind: model.RevisionKindHead, Repository: "glassroot.dev/fake-demo", Ref: "refs/heads/main", CommitID: md.HeadCommitID, ObjectFormat: model.GitObjectFormatSHA1, TreeID: md.HeadTreeID, TreeDigest: model.Digest(md.HeadTreeID)}
	baseFile, err := source.ReadFile(context.Background(), baseRef, ".glassroot/pipeline.yaml", int64(len(demoPipelineYAML)+1))
	if err != nil {
		t.Fatalf("read base pipeline: %v", err)
	}
	headFile, err := source.ReadFile(context.Background(), headRef, ".glassroot/pipeline.yaml", int64(len(demoPipelineYAML)+1))
	if err != nil {
		t.Fatalf("read head pipeline: %v", err)
	}
	if !bytes.Equal(baseFile.Data, []byte(demoPipelineYAML)) || !bytes.Equal(headFile.Data, []byte(demoPipelineYAML)) || baseFile.Executable || headFile.Executable {
		t.Fatalf("pipeline bytes or modes were not deterministic inert data")
	}
	if _, err := source.ReadFile(context.Background(), baseRef, ".glassroot/waivers.yaml", 4096); err == nil {
		t.Fatalf("base waiver file unexpectedly exists")
	}
	if _, err := source.ReadFile(context.Background(), headRef, ".glassroot/waivers.yaml", 4096); err == nil {
		t.Fatalf("head waiver file unexpectedly exists")
	}
}

func assertEvidenceVerifies(t *testing.T, root string, digest model.Digest) {
	t.Helper()
	b, err := evidence.OpenAndVerify(context.Background(), filepath.Join(root, "evidence"), evidence.DefaultReaderLimits(), evidence.WithExpectedManifestDigest(digest))
	if err != nil {
		t.Fatalf("OpenAndVerify published evidence: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("close bundle: %v", err)
	}
}

func assertInspectMatchesPublishedReports(t *testing.T, root string, md Metadata) {
	t.Helper()
	i, err := inspect.New(inspect.DefaultLimits())
	if err != nil {
		t.Fatalf("inspect.New: %v", err)
	}
	res, err := i.Inspect(context.Background(), inspect.Request{BundleDir: filepath.Join(root, "evidence"), GitDir: filepath.Join(root, "fixture.git"), BaseCommitID: md.BaseCommitID, HeadCommitID: md.HeadCommitID, EvaluatedAt: md.PolicyEvaluatedAtTime(), ManifestIntegrityMode: inspect.ManifestIntegrityExpectedDigest, ExpectedManifestDigest: md.ManifestDigest})
	if err != nil {
		t.Fatalf("Inspect published output: %v", err)
	}
	if !bytes.Equal(res.Report.JSON(), mustRead(t, filepath.Join(root, "report.json"))) {
		t.Fatalf("inspect report JSON differs from published report.json")
	}
	mdOut, err := report.RenderMarkdown(context.Background(), res.Report, report.DefaultRenderLimits())
	if err != nil {
		t.Fatalf("inspect RenderMarkdown: %v", err)
	}
	if !bytes.Equal(mdOut.Bytes, mustRead(t, filepath.Join(root, "report.md"))) {
		t.Fatalf("inspect markdown differs from published report.md")
	}
	txt, err := report.RenderTerminal(context.Background(), res.Report, report.DefaultRenderLimits())
	if err != nil {
		t.Fatalf("inspect RenderTerminal: %v", err)
	}
	if !bytes.Equal(txt.Bytes, mustRead(t, filepath.Join(root, "report.txt"))) {
		t.Fatalf("inspect terminal differs from published report.txt")
	}
}

func assertKeyEvidenceEventsExist(t *testing.T, root string, md Metadata) {
	t.Helper()
	b, err := evidence.OpenAndVerify(context.Background(), filepath.Join(root, "evidence"), evidence.DefaultReaderLimits(), evidence.WithExpectedManifestDigest(md.ManifestDigest))
	if err != nil {
		t.Fatalf("OpenAndVerify for key evidence: %v", err)
	}
	defer b.Close()
	events := map[string]model.ObservationEvent{}
	if err := b.WalkEvents(context.Background(), func(ev model.ObservationEvent) error {
		events[ev.ID] = ev
		return nil
	}); err != nil {
		t.Fatalf("WalkEvents: %v", err)
	}
	for _, rec := range md.KeyEvidence {
		for _, id := range rec.EventIDs {
			ev, ok := events[id]
			if !ok {
				t.Fatalf("key evidence event %s not found", id)
			}
			if ev.Revision != rec.Revision || ev.ScenarioID != rec.ScenarioID || ev.Repetition != rec.Repetition {
				t.Fatalf("key evidence event identity mismatch: record=%+v event=%+v", rec, ev)
			}
		}
	}
}

func assertNoHostPathInPublishedFiles(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{"demo.json", "report.json", "report.md", "report.txt"} {
		if bytes.Contains(mustRead(t, filepath.Join(root, rel)), []byte(root)) {
			t.Fatalf("%s contains output path", rel)
		}
	}
}

func rulesInReport(doc report.Document) map[string]bool {
	out := map[string]bool{}
	for _, f := range doc.Policy.AppliedFindings {
		out[f.Original.RuleID] = true
	}
	return out
}

func noticeCodes(doc report.Document) map[string]bool {
	out := map[string]bool{}
	for _, n := range doc.Notices {
		out[string(n.Code)] = true
	}
	return out
}

func sortedRuleList(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", filepath.Base(path), err)
	}
	return b
}
