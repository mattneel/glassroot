package policy

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/waiver"
)

type Applier struct{ limits ApplicationLimits }

func NewApplier(limits ApplicationLimits) (*Applier, error) {
	l, err := validateApplicationLimits(limits)
	if err != nil {
		return nil, err
	}
	return &Applier{limits: l}, nil
}

func (a *Applier) Apply(ctx context.Context, req ApplicationRequest) (*FrozenApplication, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextErr(err)
	}
	if a == nil {
		return nil, errCode(CodeInvalidLimits, "application", "", "", "applier", "applier is nil", nil)
	}
	if err := validateEvaluatedAt(req.EvaluatedAt); err != nil {
		return nil, err
	}
	if req.Evaluation == nil {
		return nil, errCode(CodeNilEvaluation, "application", "", "", "evaluation", "FrozenEvaluation is required", nil)
	}
	if req.Plan == nil {
		return nil, errCode(CodeNilPlan, "application", "", "", "plan", "FrozenPlan is required", nil)
	}
	evalJSON := req.Evaluation.JSON()
	evalDigest := req.Evaluation.Digest()
	if digestBytes(evaluationJSONDomain, evalJSON) != evalDigest {
		return nil, errCode(CodeInvalidEvaluation, "application", "", "", "digest", "policy evaluation digest mismatch", nil)
	}
	evalDoc := req.Evaluation.Document()
	planJSON := req.Plan.JSON()
	if pipeline.DigestJSON(planJSON) != req.Plan.Digest() {
		return nil, errCode(CodeInvalidPlan, "application", "", "", "digest", "run plan digest mismatch", nil)
	}
	planDoc := req.Plan.Document()
	if err := validateApplicationBinding(evalDoc, planDoc, req.Plan.Digest(), req.TrustedConfig); err != nil {
		return nil, err
	}
	if int64(len(evalDoc.Findings)) > a.limits.MaxOriginalFindings {
		return nil, errCode(CodeApplicationLimit, "application", "", "", "findings", "original finding limit exceeded", nil)
	}
	waiverLimits := applicationWaiverLimits(a.limits)
	trustedWaivers, err := waiver.LoadTrusted(ctx, req.WaiverSource, waiver.TrustedLoadRequest{Base: req.TrustedConfig.Base, Head: req.TrustedConfig.Head}, waiverLimits)
	if err != nil {
		return nil, errCode(CodeWaiverLoadFailed, "waivers", "", "", "source", "trusted waiver load failed", err)
	}
	if trustedWaivers.BaseRevision.CommitID != req.TrustedConfig.Base.CommitID || trustedWaivers.HeadRevision.CommitID != req.TrustedConfig.Head.CommitID {
		return nil, errCode(CodeInvalidWaiverState, "waivers", "", "", "revision", "trusted waiver revisions do not match request", nil)
	}
	applied, statuses, err := a.applyBaseWaivers(evalDoc.Findings, trustedWaivers, req.EvaluatedAt)
	if err != nil {
		return nil, err
	}
	gov, err := a.governanceFindings(evalDoc, req.TrustedConfig, trustedWaivers, statuses)
	if err != nil {
		return nil, err
	}
	applied = append(applied, gov...)
	if int64(len(applied)) > a.limits.MaxAppliedFindings {
		return nil, errCode(CodeApplicationLimit, "application", "", "", "appliedFindings", "applied finding limit exceeded", nil)
	}
	sortAppliedFindings(applied)
	statuses = sortWaiverStatuses(statuses)
	doc := ApplicationDocument{
		SchemaVersion:              SchemaVersionPolicyApplicationV1Alpha1,
		EvaluatedAt:                req.EvaluatedAt.UTC().Round(0),
		RunID:                      evalDoc.RunID,
		PlanDigest:                 evalDoc.PlanDigest,
		BehavioralDeltaDigest:      evalDoc.BehavioralDeltaDigest,
		BasePolicyEvaluationDigest: evalDigest,
		PolicyProfileName:          evalDoc.PolicyProfileName,
		PolicyProfileVersion:       evalDoc.PolicyProfileVersion,
		BuiltinRuleSetVersion:      evalDoc.BuiltinRuleSetVersion,
		GovernanceRuleSetVersion:   GovernanceRuleSetVersionStrictV1Alpha1,
		Base:                       planDoc.Base,
		Head:                       planDoc.Head,
		TrustedConfigAuthority:     configAuthority(req.TrustedConfig),
		TrustedWaiverAuthority:     waiverAuthority(trustedWaivers),
		AppliedFindings:            applied,
		WaiverStatuses:             statuses,
		Limitations:                sortLimitations(cloneLimitations(evalDoc.Limitations)),
	}
	doc.Summary = summarizeApplication(doc.AppliedFindings, doc.WaiverStatuses)
	doc.OverallEffectiveDisposition = overallEffectiveDisposition(doc.AppliedFindings)
	data, err := marshalApplicationDocument(doc)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > a.limits.MaxApplicationJSONBytes {
		return nil, errCode(CodeApplicationTooLarge, "application", "", "", "json", "policy application JSON exceeds limit", nil)
	}
	return &FrozenApplication{doc: cloneApplication(doc), json: append([]byte(nil), data...), digest: digestBytes(applicationJSONDomain, data)}, nil
}

func applicationWaiverLimits(l ApplicationLimits) waiver.Limits {
	wl := waiver.DefaultLimits()
	wl.MaxWaiverFileBytes = l.MaxWaiverFileBytes
	wl.MaxWaivers = l.MaxWaivers
	wl.MaxOwnerBytes = int(l.MaxOwnerBytes)
	wl.MaxReasonBytes = int(l.MaxReasonBytes)
	wl.MaxLifetimeDays = int(l.MaxWaiverLifetimeDays)
	return wl
}

func validateEvaluatedAt(t time.Time) error {
	if t.IsZero() {
		return errCode(CodeInvalidEvaluatedAt, "application", "", "", "evaluatedAt", "evaluatedAt is required", nil)
	}
	_, offset := t.Zone()
	if offset != 0 || t.UTC().Round(0) != t {
		return errCode(CodeInvalidEvaluatedAt, "application", "", "", "evaluatedAt", "evaluatedAt must be UTC without monotonic data", nil)
	}
	if t.Before(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) || !t.Before(time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return errCode(CodeInvalidEvaluatedAt, "application", "", "", "evaluatedAt", "evaluatedAt is outside supported range", nil)
	}
	return nil
}

func validateApplicationBinding(eval EvaluationDocument, plan model.RunPlan, planDigest model.Digest, trusted config.TrustedLoadResult) error {
	if eval.SchemaVersion != SchemaVersionPolicyEvaluationV1Alpha1 || eval.PolicyProfileName != PolicyProfileNameStrict || eval.PolicyProfileVersion != PolicyProfileVersionStrictV1Alpha1 || eval.BuiltinRuleSetVersion != BuiltinRuleSetVersionStrictV1Alpha1 {
		return errCode(CodeInvalidEvaluation, "application", "", "", "profile", "unsupported policy evaluation identity", nil)
	}
	if !validDigest(eval.PlanDigest) || !validDigest(eval.BehavioralDeltaDigest) {
		return errCode(CodeInvalidEvaluation, "application", "", "", "digest", "invalid evaluation digest field", nil)
	}
	if plan.RunID == "" || plan.RunID != eval.RunID || planDigest == "" || eval.PlanDigest != planDigest {
		return errCode(CodeEvaluationPlanMismatch, "application", "", "", "planDigest", "policy evaluation and plan do not match", nil)
	}
	if plan.Runner != (model.RunnerCapabilities{}) {
		return errCode(CodeInvalidPlan, "application", "", "", "runner", "legacy runner facts are not accepted", nil)
	}
	if plan.Policy == nil || plan.Policy.Profile != PolicyProfileNameStrict {
		return errCode(CodeInvalidPlan, "application", "", "", "policy.profile", "plan policy profile is not strict", nil)
	}
	if trusted.Base.CommitID != plan.Base.CommitID || trusted.Head.CommitID != plan.Head.CommitID || trusted.Base.Kind != plan.Base.Kind || trusted.Head.Kind != plan.Head.Kind {
		return errCode(CodeTrustedConfigMismatch, "application", "", "", "revision", "trusted config revisions do not match plan", nil)
	}
	if trusted.EffectiveSource.Source != config.EffectiveSourceBase || trusted.EffectiveSource.Path != config.PipelinePath || trusted.BaseFile.Path != config.PipelinePath || trusted.EffectivePipeline.Policy.Profile != PolicyProfileNameStrict {
		return errCode(CodeTrustedConfigMismatch, "application", "", "", "source", "trusted config authority is not exact base pipeline", nil)
	}
	seen := map[string]struct{}{}
	for _, f := range eval.Findings {
		if f.ID == "" || f.RuleID == "" || f.Waived || f.Disposition == model.DispositionWaived {
			return errCode(CodeInvalidOriginalFinding, "application", f.RuleID, f.ID, "finding", "original finding is invalid for waiver application", nil)
		}
		if _, ok := seen[f.ID]; ok {
			return errCode(CodeDuplicateFindingID, "application", f.RuleID, f.ID, "finding", "duplicate original finding id", nil)
		}
		seen[f.ID] = struct{}{}
	}
	return nil
}

func (a *Applier) applyBaseWaivers(findings []model.Finding, trusted waiver.TrustedLoadResult, at time.Time) ([]AppliedFinding, []WaiverStatusRecord, error) {
	applied := make([]AppliedFinding, 0, len(findings))
	byID := map[string]int{}
	for _, f := range findings {
		idx := len(applied)
		byID[f.ID] = idx
		applied = append(applied, AppliedFinding{Origin: FindingOriginBuiltinPolicy, Original: cloneFinding(f), EffectiveDisposition: f.Disposition})
	}
	statuses := []WaiverStatusRecord{}
	if trusted.Base.State != waiver.BaseStateValid {
		return applied, statuses, nil
	}
	for _, w := range trusted.Base.Waivers {
		status := WaiverStatusRecord{WaiverID: w.ID, FindingID: w.Target.FindingID, RuleID: w.Target.RuleID}
		switch {
		case at.Before(w.IssuedAt):
			status.Status = WaiverStatusNotYetValid
		case !at.Before(w.ExpiresAt):
			status.Status = WaiverStatusExpired
		default:
			idx, ok := byID[w.Target.FindingID]
			if !ok {
				status.Status = WaiverStatusUnused
				break
			}
			f := applied[idx].Original
			if f.RuleID != w.Target.RuleID {
				status.Status = WaiverStatusTargetRuleMismatch
				break
			}
			if !waiverEligibleFinding(f) {
				status.Status = WaiverStatusTargetIneligible
				break
			}
			status.Status = WaiverStatusApplied
			applied[idx].EffectiveDisposition = model.DispositionWaived
			applied[idx].AppliedWaiver = &AppliedWaiver{ID: w.ID, TargetFindingID: w.Target.FindingID, RuleID: w.Target.RuleID, Owner: w.Owner, Reason: w.Reason, IssuedAt: w.IssuedAt, ExpiresAt: w.ExpiresAt, BaseRawDigest: trusted.Base.File.Digest, SemanticWaiverSetDigest: trusted.Base.SemanticDigest}
		}
		statuses = append(statuses, status)
	}
	return applied, statuses, nil
}

func waiverEligibleFinding(f model.Finding) bool {
	if f.Disposition != model.DispositionRequiresReview {
		return false
	}
	switch f.RuleID {
	case "GR-PROC-001", "GR-FS-001", "GR-FS-002", "GR-NET-001", "GR-ART-001", "GR-DET-001", "GR-LIMIT-001":
		return true
	default:
		return false
	}
}

func (a *Applier) governanceFindings(eval EvaluationDocument, trusted config.TrustedLoadResult, waivers waiver.TrustedLoadResult, statuses []WaiverStatusRecord) ([]AppliedFinding, error) {
	out := []AppliedFinding{}
	add := func(f model.Finding, origin FindingOrigin, ref GovernanceReference) error {
		if int64(len(out)+1) > a.limits.MaxGovernanceFindings {
			return errCode(CodeGovernanceFindingLimit, "governance", f.RuleID, f.ID, "count", "governance finding limit exceeded", nil)
		}
		out = append(out, AppliedFinding{Origin: origin, Original: f, EffectiveDisposition: f.Disposition, GovernanceReference: &ref})
		return nil
	}
	for _, spec := range configGovernanceSpecs(trusted) {
		f, err := makeGovernanceFinding("GR-CONFIG-001", spec.severity, model.ConfidenceHigh, model.DispositionRequiresReview, spec.scope, spec.ref)
		if err != nil {
			return nil, err
		}
		if err := add(f, FindingOriginTrustedConfiguration, spec.ref); err != nil {
			return nil, err
		}
	}
	for _, spec := range waiverGovernanceSpecs(waivers, statuses) {
		f, err := makeGovernanceFinding("GR-WAIVER-001", spec.severity, spec.confidence, spec.disposition, spec.scope, spec.ref)
		if err != nil {
			return nil, err
		}
		if err := add(f, FindingOriginWaiverGovernance, spec.ref); err != nil {
			return nil, err
		}
	}
	_ = eval
	return out, nil
}

type governanceSpec struct {
	severity    model.Severity
	confidence  model.Confidence
	disposition model.Disposition
	scope       string
	ref         GovernanceReference
}

func configGovernanceSpecs(trusted config.TrustedLoadResult) []governanceSpec {
	a := trusted.HeadAssessment
	specs := []governanceSpec{}
	switch a.State {
	case config.HeadStateUnchanged, config.HeadStateContentChangedSemanticallyEquivalent, "":
		return specs
	case config.HeadStateModifiedValid:
		for _, ch := range a.Changes {
			ref := GovernanceReference{Kind: "config-change", Path: ch.Path, ChangeKind: string(ch.Kind), SecurityEffect: string(ch.Effect), RawDigest: a.HeadFile.Digest}
			specs = append(specs, governanceSpec{severity: severityForConfigEffect(ch.Effect), confidence: model.ConfidenceHigh, disposition: model.DispositionRequiresReview, scope: "config:" + ch.Path + ":" + string(ch.Kind) + ":" + string(ch.Effect), ref: ref})
		}
	default:
		ref := GovernanceReference{Kind: "config-file-state", State: string(a.State), RawDigest: a.HeadFile.Digest}
		specs = append(specs, governanceSpec{severity: model.SeverityHigh, confidence: model.ConfidenceHigh, disposition: model.DispositionRequiresReview, scope: "config-state:" + string(a.State), ref: ref})
	}
	return specs
}
func severityForConfigEffect(e config.SecurityEffect) model.Severity {
	switch e {
	case config.SecurityEffectPrivilegeDecrease, config.SecurityEffectObservationStrengthened:
		return model.SeverityMedium
	case config.SecurityEffectInformational:
		return model.SeverityLow
	default:
		return model.SeverityHigh
	}
}

func waiverGovernanceSpecs(result waiver.TrustedLoadResult, statuses []WaiverStatusRecord) []governanceSpec {
	specs := []governanceSpec{}
	if result.Base.State == waiver.BaseStateInvalid || result.Base.State == waiver.BaseStateUnsupportedEntryKind {
		specs = append(specs, governanceSpec{severity: model.SeverityHigh, confidence: model.ConfidenceHigh, disposition: model.DispositionFailed, scope: "base-state:" + string(result.Base.State), ref: GovernanceReference{Kind: "waiver-base-state", State: string(result.Base.State), RawDigest: result.Base.File.Digest}})
	}
	for _, st := range statuses {
		switch st.Status {
		case WaiverStatusExpired, WaiverStatusNotYetValid, WaiverStatusUnused, WaiverStatusTargetRuleMismatch, WaiverStatusTargetIneligible:
			specs = append(specs, governanceSpec{severity: model.SeverityMedium, confidence: model.ConfidenceHigh, disposition: model.DispositionRequiresReview, scope: "waiver-status:" + st.WaiverID + ":" + string(st.Status), ref: GovernanceReference{Kind: "waiver-status", WaiverID: st.WaiverID, State: string(st.Status), SemanticDigest: result.Base.SemanticDigest}})
		}
	}
	switch result.Head.State {
	case waiver.HeadStateAbsentOnBoth, waiver.HeadStateUnchanged, waiver.HeadStateContentChangedSemanticallyEquivalent, "":
		return specs
	case waiver.HeadStateAdded, waiver.HeadStateRemoved, waiver.HeadStateModifiedInvalid, waiver.HeadStateUnsupportedEntryKind:
		sev := model.SeverityHigh
		specs = append(specs, governanceSpec{severity: sev, confidence: model.ConfidenceHigh, disposition: model.DispositionRequiresReview, scope: "head-state:" + string(result.Head.State), ref: GovernanceReference{Kind: "waiver-head-state", State: string(result.Head.State), RawDigest: result.Head.File.Digest, SemanticDigest: result.Head.SemanticDigest}})
	case waiver.HeadStateModifiedValid:
		for _, ch := range result.Head.Changes {
			sev := model.SeverityMedium
			if ch.Kind == waiver.ChangeWaiverTargetChanged || ch.Kind == waiver.ChangeWaiverExpiryChanged || ch.Kind == waiver.ChangeWaiverAdded || ch.Kind == waiver.ChangeWaiverRemoved {
				sev = model.SeverityHigh
			}
			specs = append(specs, governanceSpec{severity: sev, confidence: model.ConfidenceHigh, disposition: model.DispositionRequiresReview, scope: "head-change:" + ch.WaiverID + ":" + string(ch.Kind), ref: GovernanceReference{Kind: "waiver-head-change", WaiverID: ch.WaiverID, State: string(ch.Kind), RawDigest: result.Head.File.Digest, SemanticDigest: result.Head.SemanticDigest}})
		}
	}
	return specs
}

func makeGovernanceFinding(ruleID string, severity model.Severity, confidence model.Confidence, disposition model.Disposition, scope string, ref GovernanceReference) (model.Finding, error) {
	meta := governanceRuleByID(ruleID)
	if meta.ID == "" {
		return model.Finding{}, errCode(CodeInvalidRuleCatalog, "governance", ruleID, "", "rule", "unknown governance rule", nil)
	}
	id, err := findingID(PolicyProfileVersionStrictV1Alpha1, GovernanceRuleSetVersionStrictV1Alpha1, meta.ID, meta.Version, nil, scope, nil)
	if err != nil {
		return model.Finding{}, errCode(CodeFindingIDEncodingFailed, "governance", ruleID, "", "id", "governance finding id encoding failed", err)
	}
	f := model.Finding{SchemaVersion: model.SchemaVersionFindingV1Alpha1, ID: id, RuleID: meta.ID, RuleVersion: meta.Version, Title: meta.Title, Severity: severity, Confidence: confidence, Disposition: disposition, Summary: governanceSummary(meta.ID, ref.Kind), DeltaRecordIDs: []string{}, Evidence: []model.EvidenceRef{}, ScenarioIDs: []string{}, BaseObserved: false, HeadObserved: true, Waived: false, Limitations: []model.Limitation{}}
	normalizeFindingArrays(&f)
	return f, nil
}

func governanceRuleByID(id string) ruleMeta {
	for _, r := range governanceRuleCatalog() {
		if r.ID == id {
			return r
		}
	}
	return ruleMeta{}
}
func governanceRuleCatalog() []ruleMeta {
	return []ruleMeta{{ID: "GR-CONFIG-001", Version: "v1", Title: "Trusted security configuration changed in head", Description: "Head changed trusted security configuration; effective configuration remains trusted base."}, {ID: "GR-WAIVER-001", Version: "v1", Title: "Waiver governance issue", Description: "Waiver authority, lifecycle, or head proposal requires governance review."}}
}
func governanceSummary(ruleID, kind string) string {
	switch ruleID {
	case "GR-CONFIG-001":
		return "Trusted head configuration differs from the effective base configuration; review is required before adopting it."
	case "GR-WAIVER-001":
		return "Trusted waiver metadata or head waiver proposal requires governance review."
	default:
		_ = kind
		return "Governance metadata requires review."
	}
}

func marshalApplicationDocument(doc ApplicationDocument) ([]byte, error) {
	normalizeApplication(&doc)
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, errCode(CodeSerializationFailed, "application", "", "", "json", "marshal policy application", err)
	}
	return data, nil
}

func normalizeApplication(doc *ApplicationDocument) {
	if doc.AppliedFindings == nil {
		doc.AppliedFindings = []AppliedFinding{}
	}
	if doc.WaiverStatuses == nil {
		doc.WaiverStatuses = []WaiverStatusRecord{}
	}
	if doc.Limitations == nil {
		doc.Limitations = []model.Limitation{}
	}
	if doc.TrustedConfigAuthority.Changes == nil {
		doc.TrustedConfigAuthority.Changes = []ConfigChangeReference{}
	}
	if doc.TrustedWaiverAuthority.Changes == nil {
		doc.TrustedWaiverAuthority.Changes = []WaiverChangeRecord{}
	}
	for i := range doc.AppliedFindings {
		normalizeFindingArrays(&doc.AppliedFindings[i].Original)
	}
}

func configAuthority(t config.TrustedLoadResult) ConfigAuthority {
	changes := make([]ConfigChangeReference, len(t.HeadAssessment.Changes))
	for i, ch := range t.HeadAssessment.Changes {
		changes[i] = ConfigChangeReference{Path: ch.Path, Kind: string(ch.Kind), SecurityEffect: string(ch.Effect)}
	}
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path+changes[i].Kind+changes[i].SecurityEffect < changes[j].Path+changes[j].Kind+changes[j].SecurityEffect
	})
	return ConfigAuthority{Path: config.PipelinePath, BaseRevision: t.Base, HeadRevision: t.Head, BaseRawDigest: t.BaseFile.Digest, BaseSizeBytes: t.BaseFile.SizeBytes, HeadState: t.HeadAssessment.State, HeadRawDigest: t.HeadAssessment.HeadFile.Digest, Changes: changes}
}
func waiverAuthority(t waiver.TrustedLoadResult) WaiverAuthority {
	changes := make([]WaiverChangeRecord, len(t.Head.Changes))
	for i, ch := range t.Head.Changes {
		changes[i] = WaiverChangeRecord{WaiverID: ch.WaiverID, Kind: string(ch.Kind)}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].WaiverID+changes[i].Kind < changes[j].WaiverID+changes[j].Kind })
	return WaiverAuthority{Path: waiver.WaiverPath, BaseRevision: t.BaseRevision, HeadRevision: t.HeadRevision, BaseState: t.Base.State, BaseRawDigest: t.Base.File.Digest, BaseSemanticDigest: t.Base.SemanticDigest, BaseSizeBytes: t.Base.File.SizeBytes, HeadState: t.Head.State, HeadRawDigest: t.Head.File.Digest, HeadSemanticDigest: t.Head.SemanticDigest, HeadSizeBytes: t.Head.File.SizeBytes, Changes: changes}
}

func summarizeApplication(findings []AppliedFinding, statuses []WaiverStatusRecord) ApplicationSummary {
	s := ApplicationSummary{TotalFindings: int64(len(findings))}
	for _, f := range findings {
		switch f.Origin {
		case FindingOriginBuiltinPolicy:
			s.BuiltinFindings++
		case FindingOriginTrustedConfiguration:
			s.ConfigurationFindings++
		case FindingOriginWaiverGovernance:
			s.WaiverGovernanceFindings++
		}
		switch f.EffectiveDisposition {
		case model.DispositionPassed:
			s.EffectivePassed++
		case model.DispositionRequiresReview:
			s.EffectiveRequiresReview++
		case model.DispositionFailed:
			s.EffectiveFailed++
		case model.DispositionWaived:
			s.EffectiveWaived++
		}
		switch f.Original.Severity {
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
	}
	for _, st := range statuses {
		switch st.Status {
		case WaiverStatusApplied:
			s.AppliedWaivers++
		case WaiverStatusExpired:
			s.ExpiredWaivers++
		case WaiverStatusUnused, WaiverStatusTargetRuleMismatch, WaiverStatusTargetIneligible, WaiverStatusNotYetValid:
			s.UnusedWaivers++
		case WaiverStatusInvalid:
			s.InvalidWaivers++
		}
	}
	for _, f := range findings {
		if f.Origin == FindingOriginWaiverGovernance && f.EffectiveDisposition == model.DispositionFailed {
			s.InvalidWaivers++
		}
	}
	return s
}
func overallEffectiveDisposition(findings []AppliedFinding) model.Disposition {
	for _, f := range findings {
		if f.EffectiveDisposition == model.DispositionFailed {
			return model.DispositionFailed
		}
	}
	for _, f := range findings {
		if f.EffectiveDisposition == model.DispositionRequiresReview {
			return model.DispositionRequiresReview
		}
	}
	return model.DispositionPassed
}

func sortAppliedFindings(in []AppliedFinding) {
	sort.SliceStable(in, func(i, j int) bool {
		a, b := in[i], in[j]
		ka := []string{rankDisposition(a.EffectiveDisposition), rankSeverity(a.Original.Severity), string(a.Origin), a.Original.RuleID, first(a.Original.ScenarioIDs), first(a.Original.DeltaRecordIDs), a.Original.ID}
		kb := []string{rankDisposition(b.EffectiveDisposition), rankSeverity(b.Original.Severity), string(b.Origin), b.Original.RuleID, first(b.Original.ScenarioIDs), first(b.Original.DeltaRecordIDs), b.Original.ID}
		return strings.Join(ka, "\x00") < strings.Join(kb, "\x00")
	})
}
func sortWaiverStatuses(in []WaiverStatusRecord) []WaiverStatusRecord {
	out := append([]WaiverStatusRecord(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].WaiverID+string(out[i].Status) < out[j].WaiverID+string(out[j].Status)
	})
	return out
}
func rankDisposition(d model.Disposition) string {
	switch d {
	case model.DispositionFailed:
		return "0"
	case model.DispositionRequiresReview:
		return "1"
	case model.DispositionWaived:
		return "2"
	case model.DispositionPassed:
		return "3"
	default:
		return "9"
	}
}
func rankSeverity(s model.Severity) string {
	switch s {
	case model.SeverityCritical:
		return "0"
	case model.SeverityHigh:
		return "1"
	case model.SeverityMedium:
		return "2"
	case model.SeverityLow:
		return "3"
	case model.SeverityInfo:
		return "4"
	default:
		return "9"
	}
}

func cloneApplication(in ApplicationDocument) ApplicationDocument {
	out := in
	out.AppliedFindings = make([]AppliedFinding, len(in.AppliedFindings))
	for i, f := range in.AppliedFindings {
		out.AppliedFindings[i] = cloneAppliedFinding(f)
	}
	out.WaiverStatuses = append([]WaiverStatusRecord(nil), in.WaiverStatuses...)
	out.Limitations = cloneLimitations(in.Limitations)
	out.TrustedConfigAuthority.Changes = append([]ConfigChangeReference(nil), in.TrustedConfigAuthority.Changes...)
	out.TrustedWaiverAuthority.Changes = append([]WaiverChangeRecord(nil), in.TrustedWaiverAuthority.Changes...)
	return out
}
func cloneAppliedFinding(in AppliedFinding) AppliedFinding {
	out := in
	out.Original = cloneFinding(in.Original)
	if in.AppliedWaiver != nil {
		w := *in.AppliedWaiver
		out.AppliedWaiver = &w
	}
	if in.GovernanceReference != nil {
		r := *in.GovernanceReference
		out.GovernanceReference = &r
	}
	return out
}
func cloneFinding(in model.Finding) model.Finding {
	out := in
	out.DeltaRecordIDs = append([]string(nil), in.DeltaRecordIDs...)
	out.Evidence = append([]model.EvidenceRef(nil), in.Evidence...)
	out.ScenarioIDs = append([]string(nil), in.ScenarioIDs...)
	out.Waivers = append([]model.Waiver(nil), in.Waivers...)
	out.Limitations = cloneLimitations(in.Limitations)
	normalizeFindingArrays(&out)
	return out
}
