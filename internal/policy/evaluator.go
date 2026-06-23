package policy

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/mattneel/glassroot/internal/compare"
	"github.com/mattneel/glassroot/internal/model"
)

type Evaluator struct{ limits Limits }

func New(limits Limits) (*Evaluator, error) {
	l, err := validateLimits(limits)
	if err != nil {
		return nil, err
	}
	if err := validateRuleCatalog(); err != nil {
		return nil, err
	}
	return &Evaluator{limits: l}, nil
}

func (e *Evaluator) Evaluate(ctx context.Context, req EvaluationRequest) (*FrozenEvaluation, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextErr(err)
	}
	if e == nil {
		return nil, errCode(CodeInvalidLimits, "evaluate", "", "", "evaluator", "evaluator is nil", nil)
	}
	if req.Delta == nil {
		return nil, errCode(CodeNilDelta, "input", "", "", "delta", "FrozenDelta is required", nil)
	}
	if err := validateProfile(req.Profile); err != nil {
		return nil, err
	}
	jsonBytes := req.Delta.JSON()
	digest := req.Delta.Digest()
	if compare.DigestJSON(jsonBytes) != digest {
		return nil, errCode(CodeInvalidDelta, "input", "", "", "digest", "behavioral delta digest mismatch", nil)
	}
	return e.evaluateDeltaDocument(ctx, req.Profile, req.Delta.Document(), jsonBytes, digest)
}

func (e *Evaluator) evaluateDeltaDocument(ctx context.Context, profile PolicyProfile, delta model.BehavioralDelta, deltaJSON []byte, deltaDigest model.Digest) (*FrozenEvaluation, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextErr(err)
	}
	if e == nil {
		return nil, errCode(CodeInvalidLimits, "evaluate", "", "", "evaluator", "evaluator is nil", nil)
	}
	if err := validateProfile(profile); err != nil {
		return nil, err
	}
	if err := validateDelta(delta, e.limits); err != nil {
		return nil, err
	}
	_ = deltaJSON
	findings, err := e.evaluateRules(ctx, delta)
	if err != nil {
		return nil, err
	}
	findings = sortFindings(findings, delta)
	if int64(len(findings)) > e.limits.MaxFindings {
		return nil, errCode(CodeFindingLimit, "findings", "", "", "count", "finding limit exceeded", nil)
	}
	seen := map[string]struct{}{}
	var refsTotal int64
	for _, f := range findings {
		if _, ok := seen[f.ID]; ok {
			return nil, errCode(CodeDuplicateFindingID, "findings", f.RuleID, "", f.ID, "duplicate finding id", nil)
		}
		seen[f.ID] = struct{}{}
		refsTotal += int64(len(f.Evidence))
		if refsTotal > e.limits.MaxEvidenceRefsTotal {
			return nil, errCode(CodeEvidenceReferenceLimit, "findings", "", "", "evidence", "total evidence reference limit exceeded", nil)
		}
	}
	summary := summarizeFindings(findings)
	overall := model.DispositionPassed
	if summary.Failed > 0 {
		overall = model.DispositionFailed
	} else if summary.TotalFindings > 0 {
		overall = model.DispositionRequiresReview
	}
	limits := sortLimitations(cloneLimitations(delta.Limitations))
	if delta.ManifestVerificationMode == "internal-consistency-only" {
		limits = append(limits, model.Limitation{ID: "manifest-internal-consistency-only", Summary: "Manifest digest was not matched against an independently supplied expected digest."})
		limits = sortLimitations(limits)
	}
	doc := EvaluationDocument{SchemaVersion: SchemaVersionPolicyEvaluationV1Alpha1, PolicyProfileName: profile.Name, PolicyProfileVersion: profile.Version, BuiltinRuleSetVersion: BuiltinRuleSetVersionStrictV1Alpha1, BehavioralDeltaDigest: deltaDigest, RunID: delta.RunID, PlanDigest: delta.PlanDigest, ManifestDigest: delta.ManifestDigest, ManifestVerificationMode: delta.ManifestVerificationMode, ExecutionComplete: delta.ExecutionComplete, EvidenceComplete: delta.EvidenceComplete, OverallDisposition: overall, Findings: findings, Summary: summary, Limitations: limits}
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, errCode(CodeSerializationFailed, "freeze", "", "", "json", "serialize policy evaluation", err)
	}
	if int64(len(data)) > e.limits.MaxEvaluationJSONBytes {
		return nil, errCode(CodeEvaluationTooLarge, "freeze", "", "", "json", "policy evaluation JSON exceeds limit", nil)
	}
	return &FrozenEvaluation{doc: cloneEvaluation(doc), json: append([]byte(nil), data...), digest: digestBytes(evaluationJSONDomain, data)}, nil
}

func validateDelta(delta model.BehavioralDelta, limits Limits) error {
	if delta.SchemaVersion != model.SchemaVersionBehavioralDeltaV1Alpha1 {
		return errCode(CodeInvalidDelta, "delta", "", "", "schemaVersion", "unsupported behavioral delta schema", nil)
	}
	if delta.ComparisonProfile.Version != compare.ComparisonProfileVersionV1Alpha1 {
		return errCode(CodeUnsupportedComparisonProfile, "delta", "", "", "comparisonProfile", "unsupported comparison profile", nil)
	}
	if delta.NormalizationProfileVersion != "glassroot.dev/normalization-profile/v1alpha1" {
		return errCode(CodeUnsupportedNormalizationProfile, "delta", "", "", "normalizationProfile", "unsupported normalization profile", nil)
	}
	if !validDigest(delta.PlanDigest) || !validDigest(delta.ManifestDigest) || delta.RunID == "" {
		return errCode(CodeInvalidDelta, "delta", "", "", "identity", "invalid delta identity", nil)
	}
	if int64(len(delta.Records)) > limits.MaxDeltaRecords {
		return errCode(CodeInvalidDelta, "delta", "", "", "records", "delta record limit exceeded", nil)
	}
	if err := validateSummary(delta.Summary, delta.Records); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, rec := range delta.Records {
		if rec.ID == "" {
			return errCode(CodeInvalidDeltaRecord, "records", "", "", "id", "delta record id missing", nil)
		}
		if _, ok := seen[rec.ID]; ok {
			return errCode(CodeInvalidDelta, "records", "", rec.ID, "id", "duplicate delta record id", nil)
		}
		seen[rec.ID] = struct{}{}
		if err := validateDeltaRecord(rec, limits); err != nil {
			return err
		}
	}
	return nil
}

func validateSummary(s model.DeltaSummary, records []model.DeltaRecord) error {
	want := model.DeltaSummary{TotalRecords: int64(len(records))}
	for _, r := range records {
		switch r.Kind {
		case model.DeltaKindAdded:
			want.Added++
		case model.DeltaKindRemoved:
			want.Removed++
		case model.DeltaKindModified:
			want.Modified++
		case model.DeltaKindCountChanged:
			want.CountChanged++
		case model.DeltaKindOrderChanged:
			want.OrderChanged++
		case model.DeltaKindStabilityChanged:
			want.StabilityChanged++
		case model.DeltaKindCoverageChanged:
			want.CoverageChanged++
		}
	}
	if s != want {
		return errCode(CodeInvalidDelta, "delta", "", "", "summary", "delta summary does not match records", nil)
	}
	return nil
}

func validateDeltaRecord(rec model.DeltaRecord, limits Limits) error {
	if !supportedDeltaKind(rec.Kind) {
		return errCode(CodeUnsupportedDeltaKind, "records", "", rec.ID, "kind", "unsupported delta kind", nil)
	}
	if !supportedBasis(rec.Basis) {
		return errCode(CodeUnsupportedComparisonBasis, "records", "", rec.ID, "basis", "unsupported comparison basis", nil)
	}
	if rec.FactKind != "" && !supportedFactKind(rec.FactKind) {
		return errCode(CodeUnsupportedFactKind, "records", "", rec.ID, "factKind", "unsupported fact kind", nil)
	}
	if rec.Source != "" && !supportedSource(rec.Source) {
		return errCode(CodeInvalidObservationSource, "records", "", rec.ID, "source", "invalid observation source", nil)
	}
	if rec.AnchorDigest != "" && !validDigest(rec.AnchorDigest) {
		return errCode(CodeInvalidDeltaRecord, "records", "", rec.ID, "anchorDigest", "invalid anchor digest", nil)
	}
	if len(rec.ScenarioIDs) == 0 || int64(len(rec.ScenarioIDs)) > limits.MaxScenarioIDsPerFinding {
		return errCode(CodeInvalidDeltaRecord, "records", "", rec.ID, "scenarioIds", "invalid scenario ids", nil)
	}
	if int64(len(rec.Evidence)) > limits.MaxEvidenceRefsPerFinding || int64(len(rec.BaseEvidence)) > limits.MaxEvidenceRefsPerFinding || int64(len(rec.HeadEvidence)) > limits.MaxEvidenceRefsPerFinding {
		return errCode(CodeEvidenceReferenceLimit, "records", "", rec.ID, "evidence", "evidence reference limit exceeded", nil)
	}
	for _, ref := range append(append([]model.EvidenceRef{}, rec.BaseEvidence...), rec.HeadEvidence...) {
		if err := validateEvidenceRef(ref, rec.ID); err != nil {
			return err
		}
	}
	if err := validateOccurrence(rec.BaseOccurrence); err != nil {
		return errCode(CodeInvalidOccurrenceProfile, "records", "", rec.ID, "baseOccurrence", "invalid base occurrence profile", err)
	}
	if err := validateOccurrence(rec.HeadOccurrence); err != nil {
		return errCode(CodeInvalidOccurrenceProfile, "records", "", rec.ID, "headOccurrence", "invalid head occurrence profile", err)
	}
	for _, f := range append(append([]model.DeltaFactSnapshot{}, rec.BaseFacts...), rec.HeadFacts...) {
		if err := validateSnapshot(f, rec); err != nil {
			return err
		}
	}
	return nil
}

func validateSnapshot(f model.DeltaFactSnapshot, rec model.DeltaRecord) error {
	if f.ID == "" || !validDigest(f.SemanticDigest) || f.Kind == "" || f.Source == "" {
		return errCode(CodeInvalidDeltaRecord, "facts", "", rec.ID, "fact", "invalid fact snapshot identity", nil)
	}
	if !supportedFactKind(f.Kind) {
		return errCode(CodeUnsupportedFactKind, "facts", "", rec.ID, "kind", "unsupported fact kind", nil)
	}
	if !supportedSource(f.Source) {
		return errCode(CodeInvalidObservationSource, "facts", "", rec.ID, "source", "invalid observation source", nil)
	}
	n := 0
	if f.Process != nil {
		n++
	}
	if f.Filesystem != nil {
		n++
	}
	if f.Network != nil {
		n++
	}
	if f.Artifact != nil {
		n++
	}
	if f.Scenario != nil {
		n++
	}
	if f.Warning != nil {
		n++
	}
	if f.Resource != nil {
		n++
	}
	if n != 1 {
		return errCode(CodeInvalidDeltaRecord, "facts", "", rec.ID, "payload", "fact snapshot must contain exactly one typed payload", nil)
	}
	return nil
}

func validateOccurrence(p model.OccurrenceProfile) error {
	if p.PlannedRepetitionCount < 0 || p.CompleteRepetitionCount < 0 || p.IncompleteRepetitionCount < 0 || p.MinimumKnownCount < 0 || p.MaximumKnownCount < 0 || p.TotalKnownCount < 0 {
		return errCode(CodeInvalidOccurrenceProfile, "occurrence", "", "", "count", "negative occurrence count", nil)
	}
	if p.Coverage != "" && !supportedCoverage(p.Coverage) {
		return errCode(CodeInvalidOccurrenceProfile, "occurrence", "", "", "coverage", "invalid coverage", nil)
	}
	if p.Repeatability != "" && !supportedRepeatability(p.Repeatability) {
		return errCode(CodeInvalidOccurrenceProfile, "occurrence", "", "", "repeatability", "invalid repeatability", nil)
	}
	return nil
}

func validateEvidenceRef(ref model.EvidenceRef, record string) error {
	if ref.EventStreamDigest != "" && !validDigest(ref.EventStreamDigest) {
		return errCode(CodeEvidenceReferenceInvalid, "evidence", "", record, "eventStreamDigest", "invalid event stream digest", nil)
	}
	if ref.EventStreamPath == "" && ref.BundlePath == nil {
		return errCode(CodeEvidenceReferenceInvalid, "evidence", "", record, "path", "evidence reference missing logical path", nil)
	}
	if len(ref.EventIDs) == 0 && ref.EventSequence == 0 {
		return errCode(CodeEvidenceReferenceInvalid, "evidence", "", record, "event", "evidence reference missing event identity", nil)
	}
	if ref.Revision != "" && ref.Revision != model.RevisionKindBase && ref.Revision != model.RevisionKindHead {
		return errCode(CodeEvidenceReferenceInvalid, "evidence", "", record, "revision", "invalid evidence revision", nil)
	}
	return nil
}

func supportedDeltaKind(k model.DeltaKind) bool {
	switch k {
	case model.DeltaKindAdded, model.DeltaKindRemoved, model.DeltaKindModified, model.DeltaKindCountChanged, model.DeltaKindOrderChanged, model.DeltaKindStabilityChanged, model.DeltaKindCoverageChanged:
		return true
	default:
		return false
	}
}
func supportedBasis(b model.ComparisonBasis) bool {
	switch b {
	case model.ComparisonBasisCompleteObservation, model.ComparisonBasisSingleSample, model.ComparisonBasisRepetitionVariable, model.ComparisonBasisCoverageLimited, model.ComparisonBasisAmbiguousCorrelation:
		return true
	default:
		return false
	}
}
func supportedFactKind(k string) bool {
	switch k {
	case "process-start", "process-exit", "filesystem-create", "filesystem-read", "filesystem-write", "filesystem-delete", "filesystem-rename", "filesystem-chmod", "dns-query", "network-connection", "artifact-activity", "scenario-started", "scenario-completed", "observer-warning", "unsupported-observation", "resource-limit", "sequence":
		return true
	default:
		return false
	}
}
func supportedSource(s model.ObservationSource) bool {
	switch s {
	case model.ObservationSourceHostObserved, model.ObservationSourceNetworkBrokerObserved, model.ObservationSourceSandboxRuntimeObserved, model.ObservationSourceGuestAgentReported, model.ObservationSourceWorkloadReported, model.ObservationSourceStaticAnalysisDerived, model.ObservationSourceModelInferred, model.ObservationSourceSyntheticTestGenerated:
		return true
	default:
		return false
	}
}
func supportedCoverage(c model.CoverageAssessment) bool {
	switch c {
	case model.CoverageAssessmentComplete, model.CoverageAssessmentPartial, model.CoverageAssessmentNone:
		return true
	default:
		return false
	}
}
func supportedRepeatability(r model.RepeatabilityAssessment) bool {
	switch r {
	case model.RepeatabilityStable, model.RepeatabilityVariable, model.RepeatabilitySingleSample, model.RepeatabilityNotAssessable:
		return true
	default:
		return false
	}
}

func validDigest(d model.Digest) bool {
	s := string(d)
	if len(s) != 71 || !strings.HasPrefix(s, "sha256:") {
		return false
	}
	for _, c := range s[7:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func cloneEvaluation(in EvaluationDocument) EvaluationDocument {
	var out EvaluationDocument
	b, _ := json.Marshal(in)
	_ = json.Unmarshal(b, &out)
	if out.Findings == nil {
		out.Findings = []model.Finding{}
	}
	if out.Limitations == nil {
		out.Limitations = []model.Limitation{}
	}
	for i := range out.Findings {
		normalizeFindingArrays(&out.Findings[i])
	}
	return out
}

func normalizeFindingArrays(f *model.Finding) {
	if f.Evidence == nil {
		f.Evidence = []model.EvidenceRef{}
	}
	if f.ScenarioIDs == nil {
		f.ScenarioIDs = []string{}
	}
	if f.DeltaRecordIDs == nil {
		f.DeltaRecordIDs = []string{}
	}
	if f.Limitations == nil {
		f.Limitations = []model.Limitation{}
	}
}

func sortLimitations(in []model.Limitation) []model.Limitation {
	out := cloneLimitations(in)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		if out[i].Summary != out[j].Summary {
			return out[i].Summary < out[j].Summary
		}
		return out[i].Details < out[j].Details
	})
	return out
}
func cloneLimitations(in []model.Limitation) []model.Limitation {
	if len(in) == 0 {
		return []model.Limitation{}
	}
	out := make([]model.Limitation, len(in))
	copy(out, in)
	return out
}
