package policy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/waiver"
)

func TestApplyActiveTrustedBaseWaiverPreservesOriginalFinding(t *testing.T) {
	fx := newApplicationFixture(t, headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied")))
	target := findingForRule(fx.evaluation.Document(), "GR-NET-001")
	fx.source.putWaiver(fx.base, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: waiverYAML("known-network", target.ID, target.RuleID, "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")})
	fx.source.putWaiver(fx.head, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: waiverYAML("head-proposal", target.ID, target.RuleID, "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")})

	app := mustApplier(t)
	frozen, err := app.Apply(context.Background(), fx.request(time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	doc := frozen.Document()
	applied := appliedFindingForRule(doc, "GR-NET-001")
	if applied == nil {
		t.Fatalf("missing original GR-NET-001 finding: %+v", doc.AppliedFindings)
	}
	if applied.Origin != FindingOriginBuiltinPolicy || applied.Original.ID != target.ID || applied.Original.Disposition != model.DispositionRequiresReview || applied.Original.Waived {
		t.Fatalf("original finding not preserved: %+v", *applied)
	}
	if applied.EffectiveDisposition != model.DispositionWaived || applied.AppliedWaiver == nil || applied.AppliedWaiver.ID != "known-network" {
		t.Fatalf("active base waiver was not applied exactly: %+v", *applied)
	}
	if applied.Original.Severity != target.Severity || applied.Original.Confidence != target.Confidence || !sameEvidence(applied.Original.Evidence, target.Evidence) {
		t.Fatalf("waiver changed original severity/confidence/evidence: before=%+v after=%+v", target, applied.Original)
	}
	if doc.Summary.EffectiveWaived != 1 || doc.Summary.AppliedWaivers != 1 {
		t.Fatalf("summary waived counts wrong: %+v", doc.Summary)
	}
	if doc.TrustedWaiverAuthority.HeadState != waiver.HeadStateModifiedValid {
		t.Fatalf("head waiver proposal should be inspected as modified-valid, got %s", doc.TrustedWaiverAuthority.HeadState)
	}
	if hasAppliedRule(doc, "GR-WAIVER-001") == false {
		t.Fatalf("head waiver proposal did not emit governance finding: %+v", doc.AppliedFindings)
	}
	if len(frozen.JSON()) == 0 || !strings.HasPrefix(string(frozen.Digest()), "sha256:") {
		t.Fatalf("frozen application missing JSON/digest")
	}
}

func TestHeadWaiverCannotAffectEffectiveApplication(t *testing.T) {
	fx := newApplicationFixture(t, headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied")))
	target := findingForRule(fx.evaluation.Document(), "GR-NET-001")
	fx.source.putWaiver(fx.head, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: waiverYAML("head-only", target.ID, target.RuleID, "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")})

	frozen, err := mustApplier(t).Apply(context.Background(), fx.request(time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	doc := frozen.Document()
	applied := appliedFindingForRule(doc, "GR-NET-001")
	if applied == nil || applied.EffectiveDisposition != model.DispositionRequiresReview || applied.AppliedWaiver != nil {
		t.Fatalf("head-only waiver affected effective result: %+v", applied)
	}
	if doc.TrustedWaiverAuthority.BaseState != waiver.BaseStateAbsent || doc.TrustedWaiverAuthority.HeadState != waiver.HeadStateAdded {
		t.Fatalf("unexpected waiver authority states: %+v", doc.TrustedWaiverAuthority)
	}
	if !hasAppliedRule(doc, "GR-WAIVER-001") {
		t.Fatalf("head-only waiver proposal not reported: %+v", doc.AppliedFindings)
	}
}

func TestSyntheticObservationFindingCannotBeWaived(t *testing.T) {
	fx := newApplicationFixture(t)
	target := findingForRule(fx.evaluation.Document(), "GR-OBS-001")
	fx.source.putWaiver(fx.base, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: waiverYAML("synthetic-observation", target.ID, target.RuleID, "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")})

	frozen, err := mustApplier(t).Apply(context.Background(), fx.request(time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	doc := frozen.Document()
	applied := appliedFindingForRule(doc, "GR-OBS-001")
	if applied == nil || applied.EffectiveDisposition != model.DispositionRequiresReview || applied.AppliedWaiver != nil {
		t.Fatalf("synthetic GR-OBS finding should remain unwaived: %+v", applied)
	}
	gov := appliedFindingForRule(doc, "GR-WAIVER-001")
	if gov == nil || gov.EffectiveDisposition != model.DispositionFailed {
		t.Fatalf("invalid GR-OBS waiver target should be a failed governance issue: %+v", doc.AppliedFindings)
	}
}

func TestInvalidBaseWaiverAppliesNoneAndEmitsFailedGovernance(t *testing.T) {
	fx := newApplicationFixture(t, headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied")))
	fx.source.putWaiver(fx.base, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte("apiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata:\n  name: default\nspec:\n  waivers:\n    - id: bad\n")})

	frozen, err := mustApplier(t).Apply(context.Background(), fx.request(time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	doc := frozen.Document()
	if appliedFindingForRule(doc, "GR-NET-001").EffectiveDisposition != model.DispositionRequiresReview {
		t.Fatalf("invalid base waiver unexpectedly waived finding: %+v", doc.AppliedFindings)
	}
	gov := appliedFindingForRule(doc, "GR-WAIVER-001")
	if gov == nil || gov.EffectiveDisposition != model.DispositionFailed || gov.Original.Severity != model.SeverityHigh {
		t.Fatalf("invalid base waiver did not emit failed governance finding: %+v", doc.AppliedFindings)
	}
	if doc.Summary.EffectiveFailed == 0 || doc.OverallEffectiveDisposition != model.DispositionFailed {
		t.Fatalf("failed governance did not affect overall disposition: %+v overall=%s", doc.Summary, doc.OverallEffectiveDisposition)
	}
}

func TestApplicationBindingRejectsMismatchedInputsAndInvalidTime(t *testing.T) {
	fx := newApplicationFixture(t, headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied")))
	app := mustApplier(t)
	_, err := app.Apply(context.Background(), fx.request(time.Time{}))
	assertApplicationError(t, err, CodeInvalidEvaluatedAt)

	badTrusted := fx.trusted
	badTrusted.Base.CommitID = strings.Repeat("f", 40)
	req := fx.request(time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	req.TrustedConfig = badTrusted
	_, err = app.Apply(context.Background(), req)
	assertApplicationError(t, err, CodeTrustedConfigMismatch)
}

func TestConfigChangeEmitsGovernanceFinding(t *testing.T) {
	fx := newApplicationFixture(t, headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied")))
	trusted := fx.trusted
	trusted.HeadAssessment = config.HeadAssessment{State: config.HeadStateModifiedValid, BaseFile: trusted.BaseFile, HeadFile: trusted.BaseFile, Changes: []config.ConfigChange{{Path: "spec.resources.cpu", Kind: config.ChangeKindModified, Effect: config.SecurityEffectPrivilegeIncrease}}}
	req := fx.request(time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	req.TrustedConfig = trusted
	frozen, err := mustApplier(t).Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	doc := frozen.Document()
	gov := appliedFindingForRule(doc, "GR-CONFIG-001")
	if gov == nil || gov.Origin != FindingOriginTrustedConfiguration || gov.Original.Severity != model.SeverityHigh || gov.EffectiveDisposition != model.DispositionRequiresReview {
		t.Fatalf("config privilege increase did not emit high review governance finding: %+v", doc.AppliedFindings)
	}
	if gov.AppliedWaiver != nil || len(gov.Original.Evidence) != 0 {
		t.Fatalf("config governance finding should not be waived or invent event evidence: %+v", *gov)
	}
}

func TestFrozenApplicationOwnershipAndDeterminism(t *testing.T) {
	fx := newApplicationFixture(t, headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied")))
	frozen, err := mustApplier(t).Apply(context.Background(), fx.request(time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	doc := frozen.Document()
	jsonBytes := frozen.JSON()
	doc.AppliedFindings = nil
	if len(jsonBytes) > 0 {
		jsonBytes[0] = '{'
	}
	if len(frozen.Document().AppliedFindings) == 0 || !bytes.Equal(frozen.JSON(), frozen.JSON()) {
		t.Fatal("FrozenApplication exposed mutable internals")
	}
	again, err := mustApplier(t).Apply(context.Background(), fx.request(time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}
	if !bytes.Equal(frozen.JSON(), again.JSON()) || frozen.Digest() != again.Digest() {
		t.Fatalf("application is not deterministic")
	}
}

func TestGoldenPolicyApplication(t *testing.T) {
	fx := newApplicationFixture(t,
		headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied")),
		headRecord("delta-art", model.DeltaKindAdded, "artifact-activity", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, artifactSnapshot("fact-art", true)),
	)
	netFinding := findingForRule(fx.evaluation.Document(), "GR-NET-001")
	expiredTarget := "finding-" + strings.Repeat("3", 64)
	fx.source.putWaiver(fx.base, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte("apiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata:\n  name: default\nspec:\n  waivers:\n    - id: expired-artifact\n      target:\n        findingId: " + expiredTarget + "\n        ruleId: GR-ART-001\n      owner: mattneel\n      reason: Expired deterministic fixture waiver.\n      issuedAt: \"2026-01-01T00:00:00Z\"\n      expiresAt: \"2026-02-01T00:00:00Z\"\n    - id: known-network\n      target:\n        findingId: " + netFinding.ID + "\n        ruleId: GR-NET-001\n      owner: mattneel\n      reason: Known deterministic fixture behavior pending removal.\n      issuedAt: \"2026-06-23T00:00:00Z\"\n      expiresAt: \"2026-07-23T00:00:00Z\"\n")})
	fx.source.putWaiver(fx.head, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: waiverYAML("known-network", netFinding.ID, "GR-NET-001", "2026-06-23T00:00:00Z", "2026-08-01T00:00:00Z")})
	trusted := fx.trusted
	trusted.HeadAssessment = config.HeadAssessment{State: config.HeadStateModifiedValid, BaseFile: trusted.BaseFile, HeadFile: trusted.BaseFile, Changes: []config.ConfigChange{{Path: "spec.resources.cpu", Kind: config.ChangeKindModified, Effect: config.SecurityEffectPrivilegeIncrease}}}
	req := fx.request(time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC))
	req.TrustedConfig = trusted
	frozen, err := mustApplier(t).Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	got := frozen.JSON()
	if os.Getenv("UPDATE_POLICY_APPLICATION_GOLDEN") == "1" {
		writePolicyTestFile(t, got, "testdata", "v1alpha1", "policy-application.json")
		writePolicyTestFile(t, []byte(frozen.Digest()), "testdata", "v1alpha1", "policy-application.digest")
	}
	want := readPolicyTestFile(t, "testdata", "v1alpha1", "policy-application.json")
	if !bytes.Equal(got, want) {
		t.Fatalf("policy application golden mismatch\nwant=%s\n got=%s", want, got)
	}
	wantDigest := strings.TrimSpace(string(readPolicyTestFile(t, "testdata", "v1alpha1", "policy-application.digest")))
	if string(frozen.Digest()) != wantDigest {
		t.Fatalf("application digest = %s, want %s", frozen.Digest(), wantDigest)
	}
}

func FuzzHeadWaiverCannotAffectEffectiveApplication(f *testing.F) {
	f.Add("head-only", "GR-NET-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")
	f.Fuzz(func(t *testing.T, waiverID, ruleID, issuedAt, expiresAt string) {
		fx := newApplicationFixture(t, headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied")))
		target := findingForRule(fx.evaluation.Document(), "GR-NET-001")
		fx.source.putWaiver(fx.head, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: waiverYAML(waiverID, target.ID, ruleID, issuedAt, expiresAt)})
		frozen, err := mustApplier(t).Apply(context.Background(), fx.request(time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)))
		if err == nil {
			applied := appliedFindingForRule(frozen.Document(), "GR-NET-001")
			if applied != nil && applied.EffectiveDisposition == model.DispositionWaived {
				t.Fatalf("head waiver affected effective application")
			}
		}
	})
}

func FuzzApplyWaiverTargets(f *testing.F) {
	f.Add("known-network", "GR-NET-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")
	f.Add("bad", "GR-OBS-001", "2026-06-23T00:00:00Z", "2026-07-23T00:00:00Z")
	f.Fuzz(func(t *testing.T, waiverID, ruleID, issuedAt, expiresAt string) {
		fx := newApplicationFixture(t, headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied")))
		target := findingForRule(fx.evaluation.Document(), "GR-NET-001")
		fx.source.putWaiver(fx.base, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: waiverYAML(waiverID, target.ID, ruleID, issuedAt, expiresAt)})
		_, _ = mustApplier(t).Apply(context.Background(), fx.request(time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)))
	})
}

func FuzzEncodePolicyApplication(f *testing.F) {
	f.Add("delta-net", "GR-NET-001")
	f.Add("\x1b", "bad")
	f.Fuzz(func(t *testing.T, recordID, ruleID string) {
		fx := newApplicationFixture(t, headRecord("delta-net", model.DeltaKindAdded, "network-connection", model.ObservationSourceHostObserved, model.ComparisonBasisCompleteObservation, networkSnapshot("fact-net", "denied")))
		doc := fx.evaluation.Document()
		if len(doc.Findings) > 0 {
			doc.Findings[0].DeltaRecordIDs = []string{recordID}
			doc.Findings[0].RuleID = ruleID
		}
		js, err := marshalApplicationDocument(ApplicationDocument{SchemaVersion: SchemaVersionPolicyApplicationV1Alpha1, AppliedFindings: []AppliedFinding{{Origin: FindingOriginBuiltinPolicy, Original: doc.Findings[0], EffectiveDisposition: doc.Findings[0].Disposition}}})
		if err == nil && len(js) > int(DefaultApplicationLimits().MaxApplicationJSONBytes) {
			t.Fatalf("encoded application exceeded default limit")
		}
	})
}

type applicationFixture struct {
	base       model.CommitRef
	head       model.CommitRef
	source     *applicationMemorySource
	trusted    config.TrustedLoadResult
	plan       *pipeline.FrozenPlan
	evaluation *FrozenEvaluation
}

func newApplicationFixture(t *testing.T, records ...model.DeltaRecord) applicationFixture {
	t.Helper()
	base := appCommit("base", strings.Repeat("1", 40))
	head := appCommit("head", strings.Repeat("2", 40))
	source := newApplicationMemorySource(base, head)
	trusted, err := config.LoadTrusted(context.Background(), source, config.TrustedLoadRequest{Base: base, Head: head})
	if err != nil {
		t.Fatalf("config.LoadTrusted() error = %v", err)
	}
	plan, err := pipeline.Build(context.Background(), pipeline.BuildRequest{RunID: "run-0001", CreatedAt: time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC), Trusted: trusted, BaseSource: appSourceSnapshot(model.RevisionKindBase, base.CommitID), HeadSource: appSourceSnapshot(model.RevisionKindHead, head.CommitID), Platform: appPlatform()})
	if err != nil {
		t.Fatalf("pipeline.Build() error = %v", err)
	}
	delta := policyDeltaDoc(records...)
	delta.PlanDigest = plan.Digest()
	eval, err := mustEvaluator(t).evaluateDeltaDocument(context.Background(), PolicyProfileStrict(), delta, []byte(`{"delta":"owned"}`), model.Digest("sha256:"+strings.Repeat("9", 64)))
	if err != nil {
		t.Fatalf("evaluateDeltaDocument() error = %v", err)
	}
	return applicationFixture{base: base, head: head, source: source, trusted: trusted, plan: plan, evaluation: eval}
}

func (f applicationFixture) request(at time.Time) ApplicationRequest {
	return ApplicationRequest{Evaluation: f.evaluation, Plan: f.plan, TrustedConfig: f.trusted, WaiverSource: f.source, EvaluatedAt: at}
}

type applicationMemorySource struct {
	files map[string]map[string]config.RevisionFile
	base  model.CommitRef
	head  model.CommitRef
}

func newApplicationMemorySource(base, head model.CommitRef) *applicationMemorySource {
	s := &applicationMemorySource{files: map[string]map[string]config.RevisionFile{}, base: base, head: head}
	s.put(base, config.PipelinePath, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(applicationPipelineYAML), ObjectID: "base-pipeline"})
	s.put(head, config.PipelinePath, config.RevisionFile{Kind: config.EntryKindRegularFile, Data: []byte(applicationPipelineYAML), ObjectID: "head-pipeline"})
	return s
}

func (s *applicationMemorySource) putWaiver(rev model.CommitRef, file config.RevisionFile) {
	s.put(rev, waiver.WaiverPath, file)
}
func (s *applicationMemorySource) put(rev model.CommitRef, path string, file config.RevisionFile) {
	key := rev.CommitID
	if s.files[key] == nil {
		s.files[key] = map[string]config.RevisionFile{}
	}
	file.Data = append([]byte(nil), file.Data...)
	s.files[key][path] = file
}
func (s *applicationMemorySource) ReadFile(ctx context.Context, rev model.CommitRef, path string, maxBytes int64) (config.RevisionFile, error) {
	if err := ctx.Err(); err != nil {
		return config.RevisionFile{}, err
	}
	file, ok := s.files[rev.CommitID][path]
	if !ok {
		return config.RevisionFile{}, fs.ErrNotExist
	}
	if int64(len(file.Data)) > maxBytes {
		return config.RevisionFile{}, config.ErrRevisionFileTooLarge
	}
	file.Data = append([]byte(nil), file.Data...)
	return file, nil
}

func mustApplier(t *testing.T) *Applier {
	t.Helper()
	a, err := NewApplier(DefaultApplicationLimits())
	if err != nil {
		t.Fatalf("NewApplier() error = %v", err)
	}
	return a
}

func appCommit(kind, id string) model.CommitRef {
	return model.CommitRef{Kind: model.RevisionKind(kind), CommitID: id, TreeID: strings.Repeat(id[:1], 40), ObjectFormat: "sha1"}
}
func appSourceSnapshot(kind model.RevisionKind, commit string) pipeline.SourceSnapshot {
	return pipeline.SourceSnapshot{RevisionKind: kind, CommitID: commit, TreeID: strings.Repeat(commit[:1], 40), ObjectFormat: pipeline.ObjectFormatSHA1, MaterializedTreeDigest: model.Digest("sha256:" + digestHex(kind+"mat")), MaterializationManifestDigest: model.Digest("sha256:" + digestHex(kind+"manifest")), Summary: pipeline.SourceSummary{DirectoryCount: 1, RegularFileCount: 1}}
}
func appPlatform() pipeline.PlatformConstraints {
	return pipeline.PlatformConstraints{MaxCPU: 8, MaxMemoryBytes: 8 << 30, MaxDiskBytes: 16 << 30, MaxProcessCount: 1024, MaxGlobalTimeoutMillis: 60 * 60 * 1000, MaxScenarioTimeoutMillis: 60 * 60 * 1000, MaxScenarioCount: 64, MaxRepetitions: 10, MaxFilesystemRootCount: 16, MaxArtifactCount: 64, MaxArtifactBytes: 1 << 30, MaxLogBytesPerStream: 100 << 20, MaxPlanJSONBytes: pipeline.MaxPlanJSONBytes, RequiredNetworkMode: model.NetworkModeDeny}
}
func digestHex(seed any) string {
	var s string
	switch v := seed.(type) {
	case model.RevisionKind:
		s = string(v)
	case string:
		s = v
	default:
		s = "seed"
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func waiverYAML(id, findingID, ruleID, issuedAt, expiresAt string) []byte {
	return []byte("apiVersion: glassroot.dev/v1alpha1\nkind: WaiverSet\nmetadata:\n  name: default\nspec:\n  waivers:\n    - id: " + id + "\n      target:\n        findingId: " + findingID + "\n        ruleId: " + ruleID + "\n      owner: mattneel\n      reason: Known deterministic fixture behavior pending removal.\n      issuedAt: \"" + issuedAt + "\"\n      expiresAt: \"" + expiresAt + "\"\n")
}

func appliedFindingForRule(doc ApplicationDocument, ruleID string) *AppliedFinding {
	for i := range doc.AppliedFindings {
		if doc.AppliedFindings[i].Original.RuleID == ruleID {
			return &doc.AppliedFindings[i]
		}
	}
	return nil
}
func hasAppliedRule(doc ApplicationDocument, ruleID string) bool {
	return appliedFindingForRule(doc, ruleID) != nil
}
func sameEvidence(a, b []model.EvidenceRef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].EventStreamPath != b[i].EventStreamPath || a[i].EventSequence != b[i].EventSequence || a[i].Revision != b[i].Revision {
			return false
		}
	}
	return true
}

func assertApplicationError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected application error %s, got nil", code)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error %T is not *policy.Error: %v", err, err)
	}
	if e.Code != code {
		t.Fatalf("error code=%s want=%s err=%v", e.Code, code, err)
	}
	if strings.ContainsAny(err.Error(), "\x1b\r\n") {
		t.Fatalf("error contains raw controls: %q", err.Error())
	}
}

const applicationPipelineYAML = `apiVersion: glassroot.dev/v1alpha1
kind: Pipeline
metadata:
  name: default
spec:
  environment:
    image: docker.io/library/golang:1.26@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    workdir: /workspace
  resources:
    cpu: 2
    memory: 2GiB
    disk: 4GiB
    processes: 256
    timeout: 15m
  network:
    mode: deny
    allow: []
  scenarios:
    - id: test
      name: Unit tests
      shell: /bin/sh
      run: echo test
      timeout: 10m
  collect:
    filesystem:
      roots:
        - /workspace
      contents: metadata-and-digests
    artifacts: []
    logs:
      maxBytesPerStream: 10MiB
  compare:
    ignore:
      - field: event.timestamp
      - field: process.pid
    repetitions: 1
  policy:
    profile: strict
`
