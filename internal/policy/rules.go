package policy

import (
	"context"
	"sort"

	"github.com/mattneel/glassroot/internal/model"
)

type findingSpec struct {
	ruleID       string
	severity     model.Severity
	confidence   model.Confidence
	disposition  model.Disposition
	recordIDs    []string
	scenarioIDs  []string
	baseObserved bool
	headObserved bool
	evidence     []model.EvidenceRef
	limitations  []model.Limitation
	scope        string
}

func (e *Evaluator) evaluateRules(ctx context.Context, delta model.BehavioralDelta) ([]model.Finding, error) {
	findings := []model.Finding{}
	if err := ctx.Err(); err != nil {
		return nil, contextErr(err)
	}
	if !delta.ExecutionComplete || !delta.EvidenceComplete {
		f, err := e.makeFinding(findingSpec{ruleID: "GR-OBS-001", severity: model.SeverityHigh, confidence: model.ConfidenceHigh, disposition: model.DispositionFailed, scope: "global-incomplete", limitations: completenessLimitations(delta)})
		if err != nil {
			return nil, err
		}
		findings = append(findings, f)
	}
	if hasSyntheticEvidence(delta) {
		f, err := e.makeFinding(findingSpec{ruleID: "GR-OBS-001", severity: model.SeverityMedium, confidence: model.ConfidenceHigh, disposition: model.DispositionRequiresReview, scope: "global-synthetic", limitations: []model.Limitation{{ID: "synthetic-evidence", Summary: "Policy evaluation is based on synthetic evidence; it is not target workload behavior."}}})
		if err != nil {
			return nil, err
		}
		findings = append(findings, f)
	}
	perRecordRuleCount := map[string]int64{}
	for _, rec := range delta.Records {
		if err := ctx.Err(); err != nil {
			return nil, contextErr(err)
		}
		specs, err := e.specsForRecord(rec)
		if err != nil {
			return nil, err
		}
		for _, sp := range specs {
			perRecordRuleCount[rec.ID]++
			if perRecordRuleCount[rec.ID] > e.limits.MaxFindingsPerDeltaRecord {
				return nil, errCode(CodeFindingLimit, "findings", sp.ruleID, rec.ID, "count", "per-record finding limit exceeded", nil)
			}
			f, err := e.makeFinding(sp)
			if err != nil {
				return nil, err
			}
			findings = append(findings, f)
		}
	}
	return findings, nil
}

func (e *Evaluator) specsForRecord(rec model.DeltaRecord) ([]findingSpec, error) {
	out := []findingSpec{}
	add := func(rule string, sev model.Severity, disp model.Disposition) {
		out = append(out, findingSpec{ruleID: rule, severity: sev, confidence: classifyPolicyConfidence(rec.Basis, rec.Source, true), disposition: disp, recordIDs: []string{rec.ID}, scenarioIDs: rec.ScenarioIDs, baseObserved: rec.BaseObserved, headObserved: rec.HeadObserved, evidence: evidenceForRecord(rec), limitations: rec.Limitations, scope: "record"})
	}
	if rec.Kind == model.DeltaKindCoverageChanged {
		if rec.HeadObserved && len(rec.HeadFacts) > 0 {
			// Coverage-limited positive head observations are still reviewable behavior.
		} else {
			sev := model.SeverityMedium
			if rec.HeadOccurrence.Coverage != model.CoverageAssessmentComplete {
				sev = model.SeverityHigh
			}
			add("GR-OBS-001", sev, model.DispositionRequiresReview)
			return out, nil
		}
	}
	if headWarning(rec) && (rec.Kind == model.DeltaKindAdded || rec.Kind == model.DeltaKindModified || rec.Kind == model.DeltaKindCoverageChanged) {
		add("GR-OBS-001", model.SeverityMedium, model.DispositionRequiresReview)
	}
	if headPositive(rec) && isProcessNew(rec) {
		add("GR-PROC-001", model.SeverityMedium, model.DispositionRequiresReview)
	}
	if headPositive(rec) && executableHeadBehavior(rec) {
		add("GR-FS-001", model.SeverityHigh, model.DispositionRequiresReview)
	}
	if headPositive(rec) && isFilesystemFact(rec) {
		sev, ok, err := outsideRootSeverity(rec)
		if err != nil {
			return nil, err
		}
		if ok {
			add("GR-FS-002", sev, model.DispositionRequiresReview)
		}
	}
	if headPositive(rec) && isNetworkFact(rec) {
		add("GR-NET-001", model.SeverityHigh, model.DispositionRequiresReview)
	}
	if headPositive(rec) && isArtifactFact(rec) {
		add("GR-ART-001", model.SeverityMedium, model.DispositionRequiresReview)
	}
	if rec.Kind == model.DeltaKindStabilityChanged && repeatabilityWorse(rec.BaseOccurrence.Repeatability, rec.HeadOccurrence.Repeatability) {
		conf := model.ConfidenceMedium
		if rec.Basis == model.ComparisonBasisCoverageLimited || rec.HeadOccurrence.Coverage != model.CoverageAssessmentComplete {
			conf = model.ConfidenceLow
		}
		out = append(out, findingSpec{ruleID: "GR-DET-001", severity: model.SeverityMedium, confidence: conf, disposition: model.DispositionRequiresReview, recordIDs: []string{rec.ID}, scenarioIDs: rec.ScenarioIDs, baseObserved: rec.BaseObserved, headObserved: rec.HeadObserved, evidence: evidenceForRecord(rec), limitations: rec.Limitations, scope: "record"})
	}
	if headPositive(rec) && isResourceFact(rec) {
		add("GR-LIMIT-001", model.SeverityHigh, model.DispositionRequiresReview)
	}
	return out, nil
}

func (e *Evaluator) makeFinding(sp findingSpec) (model.Finding, error) {
	rule := ruleByID(sp.ruleID)
	if rule.ID == "" || !rule.Emit {
		return model.Finding{}, errCode(CodeInvalidRuleCatalog, "findings", sp.ruleID, "", "rule", "rule is not emitted in this profile", nil)
	}
	if sp.severity == "" {
		sp.severity = rule.Severity
	}
	if sp.disposition == "" {
		sp.disposition = rule.Disposition
	}
	if sp.confidence == "" {
		sp.confidence = model.ConfidenceLow
	}
	if sp.scope == "" {
		sp.scope = "record"
	}
	ids := sortedUniqueStrings(sp.recordIDs)
	scenarios := sortedUniqueStrings(sp.scenarioIDs)
	if int64(len(ids)) > e.limits.MaxDeltaRecordIDsPerFinding || int64(len(scenarios)) > e.limits.MaxScenarioIDsPerFinding {
		return model.Finding{}, errCode(CodeFindingLimit, "findings", rule.ID, first(ids), "identity", "finding identity limit exceeded", nil)
	}
	evidence := dedupeEvidence(sp.evidence)
	if int64(len(evidence)) > e.limits.MaxEvidenceRefsPerFinding {
		return model.Finding{}, errCode(CodeEvidenceReferenceLimit, "findings", rule.ID, first(ids), "evidence", "finding evidence reference limit exceeded", nil)
	}
	limitations := sortLimitations(sp.limitations)
	if int64(len(limitations)) > e.limits.MaxLimitationsPerFinding {
		return model.Finding{}, errCode(CodeFindingLimit, "findings", rule.ID, first(ids), "limitations", "finding limitation limit exceeded", nil)
	}
	id, err := findingID(PolicyProfileVersionStrictV1Alpha1, BuiltinRuleSetVersionStrictV1Alpha1, rule.ID, rule.Version, ids, sp.scope, scenarios)
	if err != nil {
		return model.Finding{}, errCode(CodeFindingIDEncodingFailed, "findings", rule.ID, first(ids), "id", "finding id encoding failed", err)
	}
	f := model.Finding{SchemaVersion: model.SchemaVersionFindingV1Alpha1, ID: id, RuleID: rule.ID, RuleVersion: rule.Version, Title: rule.Title, Severity: sp.severity, Confidence: sp.confidence, Disposition: sp.disposition, Summary: summaryForRule(rule.ID, sp.scope), DeltaRecordIDs: ids, Evidence: evidence, ScenarioIDs: scenarios, BaseObserved: sp.baseObserved, HeadObserved: sp.headObserved, Waived: false, Limitations: limitations}
	normalizeFindingArrays(&f)
	return f, nil
}

func summaryForRule(ruleID, scope string) string {
	switch ruleID {
	case "GR-OBS-001":
		if scope == "global-incomplete" {
			return "Observation coverage or execution completeness is insufficient for an adequate strict-profile evaluation."
		}
		if scope == "global-synthetic" {
			return "The policy evaluation is based on synthetic evidence; no target workload behavior is established by that evidence."
		}
		return "Observation coverage or observer-reported state requires review."
	case "GR-PROC-001":
		return "Typed comparison data shows new or increased head process behavior requiring review."
	case "GR-FS-001":
		return "Typed comparison data shows new or changed executable file or artifact behavior requiring review."
	case "GR-FS-002":
		return "Typed comparison data shows head filesystem access outside configured roots requiring review."
	case "GR-NET-001":
		return "Typed comparison data shows new or changed head network behavior requiring review."
	case "GR-ART-001":
		return "Typed comparison data shows new or changed head artifact behavior requiring review."
	case "GR-DET-001":
		return "Typed comparison data shows degraded behavioral repeatability requiring review."
	case "GR-LIMIT-001":
		return "Typed comparison data shows new or changed head resource-limit behavior requiring review."
	default:
		return "Typed comparison data matched a built-in rule requiring review."
	}
}

func completenessLimitations(delta model.BehavioralDelta) []model.Limitation {
	out := []model.Limitation{}
	if !delta.ExecutionComplete {
		out = append(out, model.Limitation{ID: "execution-incomplete", Summary: "Execution did not complete; missing behavior must not be treated as absent."})
	}
	if !delta.EvidenceComplete {
		out = append(out, model.Limitation{ID: "evidence-incomplete", Summary: "Evidence collection did not complete; missing behavior must not be treated as absent."})
	}
	return out
}

func hasSyntheticEvidence(delta model.BehavioralDelta) bool {
	for _, r := range delta.Records {
		if r.Source == model.ObservationSourceSyntheticTestGenerated {
			return true
		}
		for _, f := range append(append([]model.DeltaFactSnapshot{}, r.BaseFacts...), r.HeadFacts...) {
			if f.Source == model.ObservationSourceSyntheticTestGenerated {
				return true
			}
		}
	}
	return false
}

func headPositive(rec model.DeltaRecord) bool {
	if !rec.HeadObserved || len(rec.HeadFacts) == 0 {
		return false
	}
	switch rec.Kind {
	case model.DeltaKindAdded, model.DeltaKindModified, model.DeltaKindCoverageChanged:
		return true
	case model.DeltaKindCountChanged:
		return headCountIncrease(rec)
	default:
		return false
	}
}
func headCountIncrease(rec model.DeltaRecord) bool {
	b, h := rec.BaseOccurrence, rec.HeadOccurrence
	if b.Coverage == model.CoverageAssessmentNone || h.Coverage == model.CoverageAssessmentNone {
		return false
	}
	if h.TotalKnownCount > b.TotalKnownCount {
		return true
	}
	if h.MaximumKnownCount > b.MaximumKnownCount {
		return true
	}
	return false
}
func headWarning(rec model.DeltaRecord) bool {
	return rec.FactKind == "observer-warning" || rec.FactKind == "unsupported-observation"
}
func isProcessNew(rec model.DeltaRecord) bool {
	return rec.FactKind == "process-start" && (rec.Kind != model.DeltaKindModified || !onlyChangedExitOrDuration(rec.ChangedFields))
}
func onlyChangedExitOrDuration(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if f != "process.exitCode" && f != "process.durationMillis" {
			return false
		}
	}
	return true
}
func isFilesystemFact(rec model.DeltaRecord) bool {
	switch rec.FactKind {
	case "filesystem-create", "filesystem-read", "filesystem-write", "filesystem-delete", "filesystem-rename", "filesystem-chmod":
		return true
	default:
		return false
	}
}
func isNetworkFact(rec model.DeltaRecord) bool {
	return rec.FactKind == "network-connection" || rec.FactKind == "dns-query"
}
func isArtifactFact(rec model.DeltaRecord) bool { return rec.FactKind == "artifact-activity" }
func isResourceFact(rec model.DeltaRecord) bool { return rec.FactKind == "resource-limit" }

func executableHeadBehavior(rec model.DeltaRecord) bool {
	if isFilesystemFact(rec) {
		for _, f := range rec.HeadFacts {
			if f.Filesystem != nil && f.Filesystem.Executable {
				if rec.Kind == model.DeltaKindModified && !containsString(rec.ChangedFields, "filesystem.executable") && !headCountIncrease(rec) {
					continue
				}
				return true
			}
		}
	}
	if isArtifactFact(rec) {
		for _, f := range rec.HeadFacts {
			if f.Artifact != nil && f.Artifact.Executable {
				if rec.Kind == model.DeltaKindModified && !containsString(rec.ChangedFields, "artifact.executable") && !headCountIncrease(rec) {
					continue
				}
				return true
			}
		}
	}
	return false
}

func outsideRootSeverity(rec model.DeltaRecord) (model.Severity, bool, error) {
	mut := map[string]bool{"create": true, "write": true, "delete": true, "rename": true, "chmod": true, "permission-change": true}
	read := map[string]bool{"read": true, "stat": true, "metadata": true, "metadata-read": true}
	for _, f := range rec.HeadFacts {
		if f.Filesystem == nil {
			continue
		}
		if f.Filesystem.Path.Namespace != "absolute-unmapped" {
			continue
		}
		op := f.Filesystem.Operation
		if mut[op] {
			return model.SeverityHigh, true, nil
		}
		if read[op] {
			return model.SeverityMedium, true, nil
		}
		return "", false, errCode(CodeInvalidDeltaRecord, "rules", "GR-FS-002", rec.ID, "filesystem.operation", "unknown filesystem operation for outside-root rule", nil)
	}
	return "", false, nil
}

func repeatabilityWorse(base, head model.RepeatabilityAssessment) bool {
	return repeatabilityRank(head) > repeatabilityRank(base)
}
func repeatabilityRank(r model.RepeatabilityAssessment) int {
	switch r {
	case model.RepeatabilityStable:
		return 0
	case model.RepeatabilitySingleSample:
		return 1
	case model.RepeatabilityVariable:
		return 2
	case model.RepeatabilityNotAssessable:
		return 3
	default:
		return 4
	}
}

func classifyPolicyConfidence(basis model.ComparisonBasis, source model.ObservationSource, targetBehavior bool) model.Confidence {
	conf := model.ConfidenceLow
	switch basis {
	case model.ComparisonBasisCompleteObservation:
		conf = model.ConfidenceHigh
	case model.ComparisonBasisSingleSample, model.ComparisonBasisRepetitionVariable:
		conf = model.ConfidenceMedium
	case model.ComparisonBasisCoverageLimited, model.ComparisonBasisAmbiguousCorrelation:
		conf = model.ConfidenceLow
	}
	cap := model.ConfidenceHigh
	switch source {
	case model.ObservationSourceGuestAgentReported, model.ObservationSourceWorkloadReported, model.ObservationSourceStaticAnalysisDerived:
		cap = model.ConfidenceMedium
	case model.ObservationSourceModelInferred:
		cap = model.ConfidenceLow
	case model.ObservationSourceSyntheticTestGenerated:
		if targetBehavior {
			cap = model.ConfidenceLow
		}
	}
	if confidenceRank(conf) > confidenceRank(cap) {
		return cap
	}
	return conf
}
func classifyEvidenceStateConfidence() model.Confidence { return model.ConfidenceHigh }
func confidenceRank(c model.Confidence) int {
	switch c {
	case model.ConfidenceLow:
		return 0
	case model.ConfidenceMedium:
		return 1
	case model.ConfidenceHigh:
		return 2
	default:
		return -1
	}
}

func evidenceForRecord(rec model.DeltaRecord) []model.EvidenceRef {
	if rec.HeadObserved && len(rec.HeadEvidence) > 0 {
		return rec.HeadEvidence
	}
	if rec.BaseObserved && len(rec.BaseEvidence) > 0 {
		return rec.BaseEvidence
	}
	return rec.Evidence
}
func dedupeEvidence(in []model.EvidenceRef) []model.EvidenceRef {
	seen := map[string]model.EvidenceRef{}
	for _, r := range in {
		key := evidenceKey(r)
		seen[key] = cloneEvidenceRef(r)
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]model.EvidenceRef, 0, len(keys))
	for _, k := range keys {
		out = append(out, seen[k])
	}
	return out
}
func evidenceKey(r model.EvidenceRef) string {
	id := ""
	if len(r.EventIDs) > 0 {
		id = r.EventIDs[0]
	}
	return string(r.Revision) + "\x00" + r.ScenarioID + "\x00" + itoa32(r.Repetition) + "\x00" + itoa64(r.EventSequence) + "\x00" + id + "\x00" + string(r.EventStreamDigest) + "\x00" + r.EventStreamPath
}
func cloneEvidenceRef(r model.EvidenceRef) model.EvidenceRef {
	out := r
	out.EventIDs = append([]string(nil), r.EventIDs...)
	if r.BundlePath != nil {
		p := *r.BundlePath
		out.BundlePath = &p
	}
	return out
}

func sortFindings(in []model.Finding, delta model.BehavioralDelta) []model.Finding {
	out := append([]model.Finding(nil), in...)
	scenarioRank := map[string]int{}
	for i, id := range delta.ScenarioIDs {
		scenarioRank[id] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if dispositionRank(out[i].Disposition) != dispositionRank(out[j].Disposition) {
			return dispositionRank(out[i].Disposition) < dispositionRank(out[j].Disposition)
		}
		if severityRank(out[i].Severity) != severityRank(out[j].Severity) {
			return severityRank(out[i].Severity) < severityRank(out[j].Severity)
		}
		if out[i].RuleID != out[j].RuleID {
			return out[i].RuleID < out[j].RuleID
		}
		if minScenarioRank(out[i].ScenarioIDs, scenarioRank) != minScenarioRank(out[j].ScenarioIDs, scenarioRank) {
			return minScenarioRank(out[i].ScenarioIDs, scenarioRank) < minScenarioRank(out[j].ScenarioIDs, scenarioRank)
		}
		if first(out[i].DeltaRecordIDs) != first(out[j].DeltaRecordIDs) {
			return first(out[i].DeltaRecordIDs) < first(out[j].DeltaRecordIDs)
		}
		return out[i].ID < out[j].ID
	})
	return out
}
func dispositionRank(d model.Disposition) int {
	switch d {
	case model.DispositionFailed:
		return 0
	case model.DispositionRequiresReview:
		return 1
	case model.DispositionPassed:
		return 2
	case model.DispositionWaived:
		return 3
	default:
		return 9
	}
}
func severityRank(s model.Severity) int {
	switch s {
	case model.SeverityCritical:
		return 0
	case model.SeverityHigh:
		return 1
	case model.SeverityMedium:
		return 2
	case model.SeverityLow:
		return 3
	case model.SeverityInfo:
		return 4
	default:
		return 9
	}
}
func minScenarioRank(ids []string, rank map[string]int) int {
	best := 1 << 30
	for _, id := range ids {
		if r, ok := rank[id]; ok && r < best {
			best = r
		}
	}
	return best
}

func summarizeFindings(findings []model.Finding) EvaluationSummary {
	s := EvaluationSummary{TotalFindings: int64(len(findings))}
	for _, f := range findings {
		switch f.Severity {
		case model.SeverityInfo:
			s.Info++
		case model.SeverityLow:
			s.Low++
		case model.SeverityMedium:
			s.Medium++
		case model.SeverityHigh:
			s.High++
		case model.SeverityCritical:
			s.Critical++
		}
		switch f.Disposition {
		case model.DispositionRequiresReview:
			s.RequiresReview++
		case model.DispositionFailed:
			s.Failed++
		case model.DispositionPassed:
			s.Passed++
		case model.DispositionWaived:
			s.Waived++
		}
	}
	return s
}

func sortedUniqueStrings(in []string) []string {
	m := map[string]struct{}{}
	for _, v := range in {
		if v != "" {
			m[v] = struct{}{}
		}
	}
	out := make([]string, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
func containsString(in []string, v string) bool {
	for _, x := range in {
		if x == v {
			return true
		}
	}
	return false
}
func first(in []string) string {
	if len(in) == 0 {
		return ""
	}
	return in[0]
}
func itoa32(v uint32) string {
	if v == 0 {
		return "0"
	}
	b := []byte{}
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
func itoa64(v uint64) string {
	if v == 0 {
		return "0"
	}
	b := []byte{}
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
