package report

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/mattneel/glassroot/internal/compare"
	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/policy"
)

type Builder struct{ limits Limits }

type BuildRequest struct {
	Bundle      *evidence.Bundle
	Delta       *compare.FrozenDelta
	Application *policy.FrozenApplication
}

func New(limits Limits) (*Builder, error) {
	l, err := validateLimits(limits)
	if err != nil {
		return nil, err
	}
	return &Builder{limits: l}, nil
}

func validateLimits(l Limits) (Limits, error) {
	if l == (Limits{}) {
		return Limits{}, errCode(CodeInvalidLimits, "limits", "zero", "zero-value limits are invalid", nil)
	}
	checks := []struct {
		got, max int64
		name     string
	}{
		{l.MaxFindings, MaxFindingsAbsolute, "findings"}, {l.MaxDeltaRecords, MaxDeltaRecordsAbsolute, "deltaRecords"}, {l.MaxEvidenceRefsPerFinding, MaxEvidenceRefsPerFindingAbsolute, "evidenceRefsPerFinding"}, {l.MaxEvidenceRefsPerDelta, MaxEvidenceRefsPerDeltaAbsolute, "evidenceRefsPerDelta"}, {l.MaxEvidenceRefsTotal, MaxEvidenceRefsTotalAbsolute, "evidenceRefsTotal"}, {l.MaxLimitationsTotal, MaxLimitationsTotalAbsolute, "limitations"}, {l.MaxNotices, MaxNoticesAbsolute, "notices"}, {l.MaxReportJSONBytes, MaxReportJSONBytesAbsolute, "reportJSON"},
	}
	for _, c := range checks {
		if c.got <= 0 || c.got > c.max {
			return Limits{}, errCode(CodeInvalidLimits, "limits", c.name, "limit outside allowed range", nil)
		}
	}
	return l, nil
}

func validateRenderLimits(l RenderLimits) (RenderLimits, error) {
	if l == (RenderLimits{}) {
		return RenderLimits{}, errCode(CodeInvalidLimits, "render-limits", "zero", "zero-value render limits are invalid", nil)
	}
	checks := []struct {
		got, max int64
		name     string
	}{{l.MaxDisplayInputBytes, MaxDisplayInputBytesAbsolute, "displayInput"}, {l.MaxEscapedDisplayBytes, MaxEscapedDisplayBytesAbsolute, "displayOutput"}, {l.MaxMarkdownBytes, MaxMarkdownBytesAbsolute, "markdown"}, {l.MaxTerminalBytes, MaxTerminalBytesAbsolute, "terminal"}, {l.MaxRenderedLines, MaxRenderedLinesAbsolute, "lines"}, {l.MaxEvidenceRefsTotal, MaxEvidenceRefsTotalAbsolute, "evidenceRefs"}}
	for _, c := range checks {
		if c.got <= 0 || c.got > c.max {
			return RenderLimits{}, errCode(CodeInvalidLimits, "render-limits", c.name, "limit outside allowed range", nil)
		}
	}
	return l, nil
}

func (b *Builder) Build(ctx context.Context, req BuildRequest) (*FrozenReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextErr("build", err)
	}
	if b == nil {
		return nil, errCode(CodeInvalidLimits, "build", "builder", "builder is nil", nil)
	}
	if req.Bundle == nil {
		return nil, errCode(CodeNilBundle, "build", "bundle", "Bundle is required", nil)
	}
	if req.Delta == nil {
		return nil, errCode(CodeNilDelta, "build", "delta", "FrozenDelta is required", nil)
	}
	if req.Application == nil {
		return nil, errCode(CodeNilApplication, "build", "application", "FrozenApplication is required", nil)
	}
	if req.Bundle.IsClosed() {
		return nil, errCode(CodeBundleClosed, "build", "bundle", "Bundle is closed", nil)
	}

	manifest := req.Bundle.Manifest()
	bundleManifestDigest := req.Bundle.ManifestDigest()
	verification := req.Bundle.Verification()
	plan := req.Bundle.Plan()
	execDoc := req.Bundle.Execution()
	attempts := req.Bundle.Attempts()
	deltaDoc := req.Delta.Document()
	deltaJSON := req.Delta.JSON()
	deltaDigest := req.Delta.Digest()
	appDoc := req.Application.Document()
	appJSON := req.Application.JSON()
	appDigest := req.Application.Digest()

	if err := validateBinding(manifest, bundleManifestDigest, verification, plan, execDoc, attempts, deltaDoc, deltaJSON, deltaDigest, appDoc, appJSON, appDigest); err != nil {
		return nil, err
	}
	if err := validateCountsAndRefs(plan, attempts, deltaDoc, appDoc, b.limits); err != nil {
		return nil, err
	}

	doc := buildDocument(manifest, verification, plan, execDoc, attempts, deltaDoc, deltaDigest, appDoc, appDigest)
	return freezeReportDocument(doc, b.limits)
}

func validateBinding(manifest evidence.Manifest, bundleManifestDigest model.Digest, verification evidence.VerificationSummary, plan model.RunPlan, execDoc evidence.ExecutionDocument, attempts []evidence.VerifiedAttempt, delta model.BehavioralDelta, deltaJSON []byte, deltaDigest model.Digest, app policy.ApplicationDocument, appJSON []byte, appDigest model.Digest) error {
	if manifest.SchemaVersion != model.SchemaVersionEvidenceManifestV1Alpha1 || manifest.RunID == "" || !manifest.BundleTransactionValid {
		return errCode(CodeInvalidBundle, "binding", "manifest", "invalid manifest", nil)
	}
	if verification.InternallyConsistent == false || verification.ManifestDigest != bundleManifestDigest || verification.ManifestDigest != delta.ManifestDigest {
		return errCode(CodeManifestDigestMismatch, "binding", "manifestDigest", "manifest digest mismatch", nil)
	}
	if plan.SchemaVersion != model.SchemaVersionRunPlanV1Alpha1 || plan.RunID == "" {
		return errCode(CodeInvalidBundle, "binding", "plan", "invalid plan", nil)
	}
	if plan.Runner != (model.RunnerCapabilities{}) {
		return errCode(CodeInvalidBundle, "binding", "runner", "legacy runner facts are not accepted in plan", nil)
	}
	planBytes, err := json.Marshal(plan)
	if err != nil {
		return errCode(CodeSerializationFailed, "binding", "plan", "marshal plan", err)
	}
	computedPlanDigest := digestBytes(runPlanJSONDigestDomain, planBytes)
	if computedPlanDigest != manifest.PlanDigest || computedPlanDigest != execDoc.PlanDigest || computedPlanDigest != delta.PlanDigest || computedPlanDigest != app.PlanDigest {
		return errCode(CodePlanDigestMismatch, "binding", "planDigest", "plan digest mismatch", nil)
	}
	if execDoc.SchemaVersion != model.SchemaVersionExecutionResultV1Alpha1 || execDoc.RunID != plan.RunID {
		return errCode(CodeInvalidBundle, "binding", "execution", "invalid execution document", nil)
	}
	if plan.RunID != manifest.RunID || plan.RunID != delta.RunID || plan.RunID != app.RunID {
		return errCode(CodeRunIDMismatch, "binding", "runId", "run ID mismatch", nil)
	}
	if delta.SchemaVersion != model.SchemaVersionBehavioralDeltaV1Alpha1 || delta.ComparisonProfile.Version != compare.ComparisonProfileVersionV1Alpha1 || delta.NormalizationProfileVersion != "glassroot.dev/normalization-profile/v1alpha1" {
		return errCode(CodeInvalidDelta, "binding", "delta", "invalid or unsupported delta", nil)
	}
	if compare.DigestBehavioralDeltaJSON(deltaJSON) != deltaDigest || app.BehavioralDeltaDigest != deltaDigest {
		return errCode(CodeDeltaDigestMismatch, "binding", "deltaDigest", "delta digest mismatch", nil)
	}
	if app.SchemaVersion != policy.SchemaVersionPolicyApplicationV1Alpha1 || app.PolicyProfileName != policy.PolicyProfileNameStrict || app.PolicyProfileVersion != policy.PolicyProfileVersionStrictV1Alpha1 || app.BuiltinRuleSetVersion != policy.BuiltinRuleSetVersionStrictV1Alpha1 || app.GovernanceRuleSetVersion != policy.GovernanceRuleSetVersionStrictV1Alpha1 {
		return errCode(CodeInvalidApplication, "binding", "application", "invalid or unsupported application", nil)
	}
	if policy.DigestPolicyApplicationJSON(appJSON) != appDigest {
		return errCode(CodeApplicationDigestMismatch, "binding", "applicationDigest", "application digest mismatch", nil)
	}
	if !validDigest(app.BasePolicyEvaluationDigest) || !validDigest(appDigest) || !validDigest(deltaDigest) {
		return errCode(CodeInvalidApplication, "binding", "digest", "invalid application digest", nil)
	}
	if !deltaCommitCompatible(plan.Base, delta.Base) || !deltaCommitCompatible(plan.Head, delta.Head) || !sameCommit(plan.Base, app.Base) || !sameCommit(plan.Head, app.Head) {
		return errCode(CodeRevisionMismatch, "binding", "revision", "revision identity mismatch", nil)
	}
	if manifest.ExecutionComplete != execDoc.ExecutionComplete || manifest.EvidenceComplete != execDoc.EvidenceComplete || delta.ExecutionComplete != execDoc.ExecutionComplete || delta.EvidenceComplete != execDoc.EvidenceComplete {
		return errCode(CodeCompletenessMismatch, "binding", "complete", "completeness mismatch", nil)
	}
	if !validRunner(execDoc.Runner) {
		return errCode(CodeInvalidRunnerCapabilities, "binding", "runner", "invalid runner capabilities", nil)
	}
	if app.EvaluatedAt.IsZero() || app.EvaluatedAt.Location() != time.UTC {
		return errCode(CodeInvalidApplication, "binding", "evaluatedAt", "invalid evaluatedAt", nil)
	}
	if len(attempts) == 0 {
		return errCode(CodeInvalidBundle, "binding", "attempts", "missing attempt coverage", nil)
	}
	return nil
}

func sameCommit(a, b model.CommitRef) bool {
	return a.Kind == b.Kind && a.CommitID == b.CommitID && a.TreeID == b.TreeID && a.ObjectFormat == b.ObjectFormat && a.TreeDigest == b.TreeDigest
}

func validRunner(c model.RunnerCapabilities) bool {
	if c.Name == "" || c.Version == "" {
		return false
	}
	switch c.IsolationTier {
	case model.IsolationTierFake, model.IsolationTierDevelopmentOnly, model.IsolationTierHardenedContainer, model.IsolationTierMicroVM:
	default:
		return false
	}
	if c.IsolationTier == model.IsolationTierFake && (c.ExecutesTargetCode || !c.SyntheticEvidence) {
		return false
	}
	return true
}

func validateCountsAndRefs(plan model.RunPlan, attempts []evidence.VerifiedAttempt, delta model.BehavioralDelta, app policy.ApplicationDocument, limits Limits) error {
	if int64(len(delta.Records)) > limits.MaxDeltaRecords {
		return errCode(CodeReportLimit, "validate", "deltaRecords", "too many delta records", nil)
	}
	if int64(len(app.AppliedFindings)) > limits.MaxFindings {
		return errCode(CodeReportLimit, "validate", "findings", "too many findings", nil)
	}
	if delta.Summary.TotalRecords != int64(len(delta.Records)) {
		return errCode(CodeInvalidDelta, "validate", "summary", "delta summary count mismatch", nil)
	}
	if app.Summary.TotalFindings != int64(len(app.AppliedFindings)) {
		return errCode(CodeInvalidApplication, "validate", "summary", "application summary count mismatch", nil)
	}
	deltaIDs := map[string]struct{}{}
	var refTotal int64
	for _, r := range delta.Records {
		if r.ID == "" {
			return errCode(CodeInvalidDelta, "validate", "record", "missing delta record ID", nil)
		}
		if _, ok := deltaIDs[r.ID]; ok {
			return errCode(CodeDuplicateDeltaRecordID, "validate", "record", "duplicate delta record ID", nil)
		}
		deltaIDs[r.ID] = struct{}{}
		refs := append(append([]model.EvidenceRef{}, r.Evidence...), append(r.BaseEvidence, r.HeadEvidence...)...)
		if int64(len(refs)) > limits.MaxEvidenceRefsPerDelta {
			return errCode(CodeEvidenceReferenceLimit, "validate", "deltaEvidence", "too many evidence references for delta", nil)
		}
		refTotal += int64(len(refs))
	}
	findingIDs := map[string]struct{}{}
	attemptKeys := map[string]struct{}{}
	for _, a := range attempts {
		attemptKeys[attemptKey(a.Revision, a.ScenarioID, a.Repetition)] = struct{}{}
	}
	scenarios := map[string]struct{}{}
	for _, s := range plan.Scenarios {
		scenarios[s.ID] = struct{}{}
	}
	for _, f := range app.AppliedFindings {
		id := f.Original.ID
		if id == "" {
			return errCode(CodeInvalidApplication, "validate", "finding", "missing finding ID", nil)
		}
		if _, ok := findingIDs[id]; ok {
			return errCode(CodeDuplicateFindingID, "validate", "finding", "duplicate finding ID", nil)
		}
		findingIDs[id] = struct{}{}
		for _, rid := range f.Original.DeltaRecordIDs {
			if _, ok := deltaIDs[rid]; !ok {
				return errCode(CodeMissingDeltaRecord, "validate", "deltaRecordId", "finding references unknown delta record", nil)
			}
		}
		for _, sid := range f.Original.ScenarioIDs {
			if _, ok := scenarios[sid]; !ok {
				return errCode(CodeInvalidApplication, "validate", "scenario", "finding references unknown scenario", nil)
			}
		}
		if int64(len(f.Original.Evidence)) > limits.MaxEvidenceRefsPerFinding {
			return errCode(CodeEvidenceReferenceLimit, "validate", "findingEvidence", "too many evidence references for finding", nil)
		}
		for _, ref := range f.Original.Evidence {
			if err := validateEvidenceRef(ref, attemptKeys); err != nil {
				return err
			}
		}
		refTotal += int64(len(f.Original.Evidence))
	}
	if refTotal > limits.MaxEvidenceRefsTotal {
		return errCode(CodeEvidenceReferenceLimit, "validate", "evidence", "total evidence reference limit exceeded", nil)
	}
	return nil
}

func validateEvidenceRef(ref model.EvidenceRef, attempts map[string]struct{}) error {
	if ref.Revision == "" && ref.ScenarioID == "" && ref.EventSequence == 0 {
		return nil
	}
	if ref.Revision != model.RevisionKindBase && ref.Revision != model.RevisionKindHead {
		return errCode(CodeInvalidEvidenceReference, "validate", "revision", "invalid evidence reference revision", nil)
	}
	if ref.ScenarioID == "" || ref.Repetition == 0 || ref.EventSequence == 0 || ref.EventStreamDigest == "" || ref.EventStreamPath == "" || len(ref.EventIDs) == 0 {
		return errCode(CodeInvalidEvidenceReference, "validate", "evidence", "incomplete evidence reference", nil)
	}
	if !validDigest(ref.EventStreamDigest) {
		return errCode(CodeInvalidEvidenceReference, "validate", "digest", "invalid evidence reference digest", nil)
	}
	if _, ok := attempts[attemptKey(ref.Revision, ref.ScenarioID, ref.Repetition)]; !ok {
		return errCode(CodeInvalidEvidenceReference, "validate", "attempt", "evidence reference does not identify a verified attempt", nil)
	}
	return nil
}

func attemptKey(rev model.RevisionKind, scenario string, rep uint32) string {
	return string(rev) + "\x00" + scenario + "\x00" + itoa(rep)
}
func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func buildDocument(manifest evidence.Manifest, verification evidence.VerificationSummary, plan model.RunPlan, execDoc evidence.ExecutionDocument, attempts []evidence.VerifiedAttempt, delta model.BehavioralDelta, deltaDigest model.Digest, app policy.ApplicationDocument, appDigest model.Digest) Document {
	doc := Document{SchemaVersion: SchemaVersionReportV1Alpha1, ReportProfileVersion: ReportProfileVersionV1Alpha1, RunID: plan.RunID, EvaluatedAt: app.EvaluatedAt, PlanDigest: delta.PlanDigest, ManifestDigest: verification.ManifestDigest, BehavioralDeltaDigest: deltaDigest, BuiltinPolicyEvaluationDigest: app.BasePolicyEvaluationDigest, PolicyApplicationDigest: appDigest, ManifestVerificationMode: string(verification.Mode), Source: sourceIdentity(plan), Runner: runnerSection(execDoc.Runner), Completeness: completenessSection(manifest, verification, execDoc, attempts), Policy: policySection(app), Behavior: behaviorSection(delta), Notices: notices(verification, execDoc, app), Limitations: collectLimitations(manifest, verification, execDoc, delta, app)}
	sort.Slice(doc.Notices, func(i, j int) bool { return doc.Notices[i].Code < doc.Notices[j].Code })
	return doc
}

func sourceIdentity(plan model.RunPlan) SourceIdentity {
	s := SourceIdentity{Base: plan.Base, Head: plan.Head}
	for _, r := range plan.Revisions {
		if r.Kind == model.RevisionKindBase {
			s.BaseMaterializedTreeDigest = r.MaterializedTreeDigest
			s.BaseMaterializationManifestDigest = r.MaterializationManifestDigest
		}
		if r.Kind == model.RevisionKindHead {
			s.HeadMaterializedTreeDigest = r.MaterializedTreeDigest
			s.HeadMaterializationManifestDigest = r.MaterializationManifestDigest
		}
	}
	return s
}
func runnerSection(c model.RunnerCapabilities) RunnerSection {
	return RunnerSection{Name: c.Name, Version: c.Version, IsolationTier: c.IsolationTier, FreshKernel: c.FreshKernel, BrokeredNetwork: c.BrokeredNetwork, ExecutesTargetCode: c.ExecutesTargetCode, SyntheticEvidence: c.SyntheticEvidence, EnforcesNetworkDeny: c.EnforcesNetworkDeny, ProcessEventCollection: c.ProcessEventCollection, FilesystemEventCollection: c.FilesystemEventCollection, SyscallEventCollection: c.SyscallEventCollection, ArtifactHashing: c.ArtifactHashing, SnapshotSupport: c.SnapshotSupport}
}
func completenessSection(m evidence.Manifest, v evidence.VerificationSummary, e evidence.ExecutionDocument, attempts []evidence.VerifiedAttempt) CompletenessSection {
	out := CompletenessSection{BundleTransactionValid: m.BundleTransactionValid && e.BundleTransactionValid, ExecutionComplete: e.ExecutionComplete, EvidenceComplete: e.EvidenceComplete, SyntheticEvidence: e.Runner.SyntheticEvidence, ExpectedManifestDigestSupplied: v.ExpectedManifestDigestSupplied, ExpectedManifestDigestMatched: v.ExpectedManifestDigestMatched, AttemptCoverage: []AttemptCoverageReport{}}
	for _, a := range attempts {
		out.AttemptCoverage = append(out.AttemptCoverage, AttemptCoverageReport{AttemptID: a.AttemptID, Ordinal: a.Ordinal, Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition, Events: a.Events, Stdout: a.Stdout, Stderr: a.Stderr, Artifacts: a.Artifacts, Result: a.Result, AcceptedEventCount: a.AcceptedEventCount})
	}
	return out
}
func policySection(app policy.ApplicationDocument) PolicySection {
	app = stripApplicationBundlePaths(app)
	return PolicySection{ProfileName: app.PolicyProfileName, ProfileVersion: app.PolicyProfileVersion, BuiltinRuleSetVersion: app.BuiltinRuleSetVersion, GovernanceRuleSetVersion: app.GovernanceRuleSetVersion, OverallEffectiveDisposition: app.OverallEffectiveDisposition, Summary: app.Summary, AppliedFindings: app.AppliedFindings, WaiverStatuses: app.WaiverStatuses, TrustedConfigAuthority: app.TrustedConfigAuthority, TrustedWaiverAuthority: app.TrustedWaiverAuthority}
}
func behaviorSection(delta model.BehavioralDelta) BehaviorSection {
	delta = stripDeltaBundlePaths(delta)
	return BehaviorSection{ComparisonProfile: delta.ComparisonProfile, NormalizationProfileVersion: delta.NormalizationProfileVersion, ScenarioIDs: append([]string(nil), delta.ScenarioIDs...), ScenarioComparisons: delta.ScenarioComparisons, Summary: delta.Summary, Records: delta.Records}
}

func notices(v evidence.VerificationSummary, e evidence.ExecutionDocument, app policy.ApplicationDocument) []Notice {
	codes := map[NoticeCode]struct{}{NoticePassedNotProofOfSafety: {}}
	if !e.ExecutionComplete {
		codes[NoticeExecutionIncomplete] = struct{}{}
	}
	if !e.EvidenceComplete {
		codes[NoticeEvidenceIncomplete] = struct{}{}
	}
	if e.Runner.SyntheticEvidence {
		codes[NoticeSyntheticEvidence] = struct{}{}
	}
	if !e.Runner.ExecutesTargetCode {
		codes[NoticeNoTargetCodeExecuted] = struct{}{}
	}
	if e.Runner.IsolationTier == model.IsolationTierFake {
		codes[NoticeFakeRunner] = struct{}{}
	}
	if e.Runner.IsolationTier == model.IsolationTierDevelopmentOnly {
		codes[NoticeDevelopmentOnlyRunner] = struct{}{}
	}
	if !e.Runner.EnforcesNetworkDeny {
		codes[NoticeNetworkDenyNotEnforced] = struct{}{}
	}
	if v.Mode == evidence.VerificationModeInternalConsistencyOnly {
		codes[NoticeInternalConsistencyOnlyManifestVerification] = struct{}{}
	}
	if app.Summary.EffectiveWaived > 0 || app.Summary.AppliedWaivers > 0 {
		codes[NoticeWaiversApplied] = struct{}{}
	}
	if app.Summary.ConfigurationFindings > 0 || app.Summary.WaiverGovernanceFindings > 0 {
		codes[NoticeGovernanceFindingsPresent] = struct{}{}
	}
	if len(e.Limitations) > 0 || len(app.Limitations) > 0 {
		codes[NoticeObserverLimitationsPresent] = struct{}{}
	}
	out := make([]Notice, 0, len(codes))
	for c := range codes {
		out = append(out, Notice{Code: c, Text: noticeText(c)})
	}
	return out
}

func noticeText(c NoticeCode) string {
	switch c {
	case NoticeEvidenceIncomplete:
		return "Evidence is incomplete; missing evidence is not treated as clean behavior."
	case NoticeExecutionIncomplete:
		return "Execution did not complete; behavior absence is not established."
	case NoticeSyntheticEvidence:
		return "Evidence is synthetic test data, not observed target behavior."
	case NoticeNoTargetCodeExecuted:
		return "The runner reported that no target code was executed."
	case NoticeFakeRunner:
		return "The fake runner is for tests and is not a security boundary."
	case NoticeDevelopmentOnlyRunner:
		return "The development-only runner is not a hardened security boundary."
	case NoticeNetworkDenyNotEnforced:
		return "The runner did not report enforced network deny."
	case NoticeInternalConsistencyOnlyManifestVerification:
		return "The manifest was verified for internal consistency only, not authentication."
	case NoticeWaiversApplied:
		return "One or more findings are effectively waived but remain visible."
	case NoticeGovernanceFindingsPresent:
		return "Configuration or waiver governance findings are present."
	case NoticeObserverLimitationsPresent:
		return "Observer or capture limitations are present."
	case NoticePassedNotProofOfSafety:
		return "A passed disposition does not prove the code is safe."
	default:
		return "Unknown report notice."
	}
}

func collectLimitations(m evidence.Manifest, v evidence.VerificationSummary, e evidence.ExecutionDocument, d model.BehavioralDelta, a policy.ApplicationDocument) []LimitationReport {
	out := []LimitationReport{}
	out = append(out, m.Limitations...)
	out = append(out, v.Limitations...)
	out = append(out, e.Limitations...)
	out = append(out, d.Limitations...)
	out = append(out, a.Limitations...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Summary < out[j].Summary
	})
	return out
}

func deltaCommitCompatible(plan, delta model.CommitRef) bool {
	if delta == (model.CommitRef{}) {
		return true
	}
	return sameCommit(plan, delta)
}
