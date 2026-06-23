package observe

import (
	"context"
	"errors"
	"fmt"

	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

type Normalizer struct{ limits Limits }

func New(limits Limits) (*Normalizer, error) {
	l, err := validateLimits(limits)
	if err != nil {
		return nil, err
	}
	return &Normalizer{limits: l}, nil
}

func (n *Normalizer) Normalize(ctx context.Context, bundle *evidence.Bundle) (*TraceSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextErr(err)
	}
	if n == nil {
		return nil, errCode(CodeInvalidLimits, "normalize", "", "normalizer", "normalizer is nil", nil)
	}
	if bundle == nil {
		return nil, errCode(CodeNilBundle, "bundle", "", "bundle", "verified bundle is required", nil)
	}
	manifest := bundle.Manifest()
	plan := bundle.Plan()
	execution := bundle.Execution()
	verifiedAttempts := bundle.Attempts()
	verification := bundle.Verification()
	manifestDigest := bundle.ManifestDigest()
	if verification.Limitations == nil {
		verification.Limitations = []model.Limitation{}
	}
	if manifest.RunID == "" || manifest.PlanDigest == "" || plan.RunID == "" || execution.PlanDigest != manifest.PlanDigest {
		return nil, errCode(CodeInvalidPlan, "binding", "", "plan", "bundle plan/execution binding is inconsistent", nil)
	}
	if manifestDigest == "" {
		return nil, errCode(CodeBundleReadFailed, "bundle", "", "manifestDigest", "missing manifest digest", nil)
	}
	if plan.Runner != (model.RunnerCapabilities{}) {
		return nil, errCode(CodeInvalidPlan, "plan", "", "runner", "legacy runner field must be zero", nil)
	}
	profile, err := profileFromPlan(plan)
	if err != nil {
		return nil, err
	}
	attemptReqs, err := runner.ExpandPlanDocument(plan, manifest.PlanDigest)
	if err != nil {
		return nil, errCode(CodeInvalidPlan, "plan", "", "attempts", "expand verified plan attempts", err)
	}
	if len(attemptReqs) != len(verifiedAttempts) || int64(len(attemptReqs)) > n.limits.MaxAttempts {
		return nil, errCode(CodeInvalidAttemptInventory, "attempts", "", "count", "attempt inventory mismatch", nil)
	}
	attemptDocs := make([]AttemptTrace, len(attemptReqs))
	states := map[string]*attemptState{}
	refs := map[string]evidence.VerifiedEntryReference{}
	byID := map[string]int{}
	for i, req := range attemptReqs {
		va := verifiedAttempts[i]
		if req.AttemptID != va.AttemptID || req.Revision != va.Revision || req.ScenarioID != va.ScenarioID || req.Repetition != va.Repetition || req.GlobalOrdinal != va.Ordinal {
			return nil, errCode(CodeInvalidAttemptInventory, "attempts", req.AttemptID, "order", "verified attempts differ from plan expansion", nil)
		}
		key := evidence.AttemptKey{Revision: req.Revision, ScenarioID: req.ScenarioID, Repetition: req.Repetition}
		entry, err := bundle.EventStreamReference(key)
		if err != nil {
			return nil, mapBundleErr(err)
		}
		refs[req.AttemptID] = entry
		ref := RawEvidenceReference{EventStreamDigest: entry.Digest, EventStreamPath: entry.Path, Revision: req.Revision, ScenarioID: req.ScenarioID, Repetition: req.Repetition}
		states[req.AttemptID] = newAttemptState(profile, manifest.PlanDigest, req.AttemptID, ref, n.limits)
		attemptDocs[i] = AttemptTrace{AttemptID: req.AttemptID, Ordinal: req.GlobalOrdinal, Revision: req.Revision, ScenarioID: req.ScenarioID, Repetition: req.Repetition, Coverage: coverageFromAttempt(va, manifest), Result: va.Result, Events: va.Events, Stdout: va.Stdout, Stderr: va.Stderr, Artifacts: va.Artifacts, FirstEventSequence: va.FirstEventSequence, LastEventSequence: va.LastEventSequence, AcceptedEventCount: va.AcceptedEventCount, Facts: []Fact{}, Limitations: attemptLimitations(va, manifest)}
		byID[req.AttemptID] = i
	}
	var totalFacts int64
	var totalRefs int64
	var prior uint64
	err = bundle.WalkEvents(ctx, func(ev model.ObservationEvent) error {
		if err := ctx.Err(); err != nil {
			return contextErr(err)
		}
		if ev.SequenceNumber <= 0 {
			return errCode(CodeEventOrder, "events", "", "sequence", "invalid event sequence", nil)
		}
		seq := uint64(ev.SequenceNumber)
		if prior != 0 && seq != prior+1 {
			return errCode(CodeEventOrder, "events", "", "sequence", "event order gap", nil)
		}
		prior = seq
		id := fmt.Sprintf("att-%s-%s-r%d", ev.Revision, ev.ScenarioID, ev.Repetition)
		idx, ok := byID[id]
		if !ok {
			return errCode(CodeInvalidAttemptInventory, "events", id, "attempt", "event references unknown attempt", nil)
		}
		st := states[id]
		base := st.attemptKey
		base.EventID = ev.ID
		base.EventSequence = seq
		fact, err := normalizeEvent(st, ev, base)
		if err != nil {
			return err
		}
		fact.ID = factID(manifest.PlanDigest, id, fact.SemanticDigest, []string{ev.ID})
		attemptDocs[idx].Facts = append(attemptDocs[idx].Facts, fact)
		totalFacts++
		totalRefs += int64(len(fact.Evidence))
		if totalFacts > n.limits.MaxFactsPerTraceSet {
			return errCode(CodeFactLimit, "facts", id, "count", "fact limit exceeded", nil)
		}
		if totalRefs > n.limits.MaxEvidenceRefsTotal {
			return errCode(CodeEvidenceReferenceLimit, "facts", id, "evidence", "evidence reference limit exceeded", nil)
		}
		return nil
	})
	if err != nil {
		return nil, mapBundleErr(err)
	}
	global := []model.Limitation{}
	if verification.Mode == evidence.VerificationModeInternalConsistencyOnly {
		global = append(global, model.Limitation{ID: "manifest-internal-consistency-only", Summary: "No independently retained expected manifest digest was supplied; verification is internal consistency only."})
	}
	global = append(global, cloneLimitations(manifest.Limitations)...)
	global = append(global, cloneLimitations(execution.Limitations)...)
	global = sortLimitations(global)
	for i := range attemptDocs {
		if len(attemptDocs[i].Facts) == 0 {
			attemptDocs[i].Limitations = append(attemptDocs[i].Limitations, model.Limitation{ID: "empty-event-stream", Summary: "No events were present for this attempt; this is not evidence of no behavior."})
		}
		attemptDocs[i].Limitations = sortLimitations(attemptDocs[i].Limitations)
	}
	evidenceContext := model.EvidenceContext{SyntheticEvidence: execution.Runner.SyntheticEvidence, ExecutesTargetCode: execution.Runner.ExecutesTargetCode}
	doc := TraceSetDocument{SchemaVersion: TraceSetSchemaV1Alpha1, Profile: profile, PlanDigest: manifest.PlanDigest, ManifestDigest: manifestDigest, RunID: manifest.RunID, ManifestVerification: manifestVerificationFromEvidence(verification), ExecutionComplete: manifest.ExecutionComplete, EvidenceComplete: manifest.EvidenceComplete, EvidenceContext: evidenceContext, Attempts: attemptDocs, Limitations: global}
	return &TraceSet{doc: cloneTraceDocument(doc)}, nil
}

func manifestVerificationFromEvidence(in evidence.VerificationSummary) ManifestVerification {
	return ManifestVerification{Mode: in.Mode, ManifestDigest: in.ManifestDigest, ExpectedManifestDigestSupplied: in.ExpectedManifestDigestSupplied, ExpectedManifestDigestMatched: in.ExpectedManifestDigestMatched, InternallyConsistent: in.InternallyConsistent, Limitations: cloneLimitations(in.Limitations)}
}

func mapBundleErr(err error) error {
	if err == nil {
		return nil
	}
	var oe *Error
	if errors.As(err, &oe) {
		return err
	}
	var ee *evidence.Error
	if errors.As(err, &ee) {
		if ee.Code == evidence.CodeInvalidSessionState {
			return errCode(CodeBundleClosed, "bundle", "", "open", "bundle is closed", err)
		}
		return errCode(CodeBundleReadFailed, "bundle", "", string(ee.Code), "bundle read failed", err)
	}
	return err
}

func coverageFromAttempt(a evidence.VerifiedAttempt, manifest evidence.Manifest) CoverageState {
	if a.Result == evidence.CaptureStateNotProvided || a.AcceptedEventCount == 0 && !manifest.ExecutionComplete {
		return CoverageIncomplete
	}
	if a.Result == evidence.CaptureStateFailed {
		return CoverageIncomplete
	}
	return CoverageComplete
}
func attemptLimitations(a evidence.VerifiedAttempt, manifest evidence.Manifest) []model.Limitation {
	var out []model.Limitation
	if a.Stdout == evidence.CaptureStateTruncated {
		out = append(out, model.Limitation{ID: "stdout-truncated", Summary: "stdout capture was truncated."})
	}
	if a.Stderr == evidence.CaptureStateTruncated {
		out = append(out, model.Limitation{ID: "stderr-truncated", Summary: "stderr capture was truncated."})
	}
	if a.Artifacts == evidence.CaptureStateOmittedLimit {
		out = append(out, model.Limitation{ID: "artifact-omitted", Summary: "at least one artifact was omitted due to capture limits."})
	}
	for _, ar := range manifest.Artifacts {
		if ar.Attempt.Revision == a.Revision && ar.Attempt.ScenarioID == a.ScenarioID && ar.Attempt.Repetition == a.Repetition && ar.Disposition != evidence.ArtifactDispositionStored {
			out = append(out, model.Limitation{ID: "artifact-" + string(ar.Disposition), Summary: "artifact capture was not stored completely.", Details: sanitize(ar.LogicalPath, 160)})
		}
	}
	return out
}

func normalizeEventForTest(state *attemptState, event model.ObservationEvent, ref RawEvidenceReference) (Fact, error) {
	return normalizeEvent(state, event, ref)
}
func normalizeEvent(state *attemptState, event model.ObservationEvent, ref RawEvidenceReference) (Fact, error) {
	if err := validateSource(event.Source); err != nil {
		return Fact{}, err
	}
	if err := validateKindPayload(event); err != nil {
		return Fact{}, err
	}
	timing, tlim := state.normalizeTiming(event)
	fact := Fact{Kind: FactKind(event.Kind), Source: event.Source, Timing: timing, Evidence: []RawEvidenceReference{ref}, Limitations: tlim}
	switch event.Kind {
	case model.ObservationKindProcessStart:
		p, lim, err := state.processStart(event.Source, event.Process)
		if err != nil {
			return Fact{}, err
		}
		fact.Process = p
		fact.Limitations = append(fact.Limitations, lim...)
	case model.ObservationKindProcessExit:
		p, lim, err := state.processExit(event.Source, event.Process)
		if err != nil {
			return Fact{}, err
		}
		fact.Process = p
		fact.Limitations = append(fact.Limitations, lim...)
	case model.ObservationKindFilesystemCreate, model.ObservationKindFilesystemRead, model.ObservationKindFilesystemWrite, model.ObservationKindFilesystemDelete, model.ObservationKindFilesystemRename, model.ObservationKindFilesystemChmod:
		fs := event.Filesystem
		old := (*NormalizedPath)(nil)
		if fs.OldPath != "" {
			op := normalizeObservedPath(fs.OldPath, state.profile.RootAliases)
			old = &op
		}
		fact.Filesystem = &FilesystemFact{Operation: fs.Operation, Path: normalizeObservedPath(fs.Path, state.profile.RootAliases), OldPath: old, Mode: fs.Mode, Digest: fs.Digest, SizeBytes: fs.SizeBytes, Executable: fs.Executable, Truncated: fs.Truncated}
	case model.ObservationKindDNSQuery, model.ObservationKindNetworkConnection:
		n := event.Network
		fact.Network = &NetworkFact{Operation: n.Operation, Protocol: n.Protocol, QueryName: n.QueryName, DestinationHost: n.DestinationHost, DestinationPort: n.DestinationPort, ResolvedAddresses: cloneStringsBounded(n.ResolvedAddresses), Result: n.Result, DurationMillis: n.DurationMillis}
	case model.ObservationKindArtifactActivity:
		a := event.Artifact
		fact.Artifact = &ArtifactFact{Operation: a.Operation, ArtifactID: a.ArtifactID, Path: normalizeObservedPath(a.Path, state.profile.RootAliases), Digest: a.Digest, SizeBytes: a.SizeBytes, Executable: a.Executable, SourceEventIDs: cloneStringsBounded(a.SourceEventIDs)}
	case model.ObservationKindScenarioStarted, model.ObservationKindScenarioCompleted:
		s := event.Scenario
		fact.Scenario = &ScenarioFact{Status: s.Status, Message: s.Message, DurationMillis: s.DurationMillis}
	case model.ObservationKindObserverWarning, model.ObservationKindUnsupportedObservation:
		w := event.ObserverWarning
		fact.Warning = &WarningFact{Code: w.Code, Message: w.Message, Unsupported: w.Unsupported, Limitations: cloneLimitations(w.Limitations)}
		if w.Unsupported {
			fact.Limitations = append(fact.Limitations, model.Limitation{ID: "unsupported-observation", Summary: "An observer reported unsupported observation data."})
		}
	case model.ObservationKindResourceLimit:
		r := event.ResourceLimit
		fact.Resource = &ResourceFact{LimitKind: r.LimitKind, LimitValue: r.LimitValue, Unit: r.Unit, ObservedValue: r.ObservedValue, Exceeded: r.Exceeded}
	default:
		return Fact{}, errCode(CodeUnsupportedObservationKind, "event", state.attemptID, string(event.Kind), "unsupported observation kind", nil)
	}
	fact.Limitations = sortLimitations(fact.Limitations)
	d, err := semanticDigest(state.profile, fact)
	if err != nil {
		return Fact{}, err
	}
	fact.SemanticDigest = d
	if int64(len(fact.Evidence)) > state.limits.MaxEvidenceRefsPerFact {
		return Fact{}, errCode(CodeEvidenceReferenceLimit, "event", state.attemptID, "evidence", "too many evidence references", nil)
	}
	return fact, nil
}

func validateSource(s model.ObservationSource) error {
	switch s {
	case model.ObservationSourceHostObserved, model.ObservationSourceNetworkBrokerObserved, model.ObservationSourceSandboxRuntimeObserved, model.ObservationSourceGuestAgentReported, model.ObservationSourceWorkloadReported, model.ObservationSourceStaticAnalysisDerived, model.ObservationSourceModelInferred, model.ObservationSourceSyntheticTestGenerated:
		return nil
	default:
		return errCode(CodeInvalidObservationSource, "event", "", string(s), "unknown observation source", nil)
	}
}
func validateKindPayload(e model.ObservationEvent) error {
	switch e.Kind {
	case model.ObservationKindProcessStart, model.ObservationKindProcessExit, model.ObservationKindFilesystemCreate, model.ObservationKindFilesystemRead, model.ObservationKindFilesystemWrite, model.ObservationKindFilesystemDelete, model.ObservationKindFilesystemRename, model.ObservationKindFilesystemChmod, model.ObservationKindDNSQuery, model.ObservationKindNetworkConnection, model.ObservationKindArtifactActivity, model.ObservationKindScenarioStarted, model.ObservationKindScenarioCompleted, model.ObservationKindObserverWarning, model.ObservationKindUnsupportedObservation, model.ObservationKindResourceLimit:
	default:
		return errCode(CodeUnsupportedObservationKind, "event", "", string(e.Kind), "unsupported observation kind", nil)
	}
	count := 0
	if e.Process != nil {
		count++
	}
	if e.Filesystem != nil {
		count++
	}
	if e.Network != nil {
		count++
	}
	if e.Artifact != nil {
		count++
	}
	if e.Scenario != nil {
		count++
	}
	if e.ObserverWarning != nil {
		count++
	}
	if e.ResourceLimit != nil {
		count++
	}
	if count != 1 {
		return errCode(CodeInvalidObservationPayload, "event", "", string(e.Kind), "exactly one payload is required", nil)
	}
	switch e.Kind {
	case model.ObservationKindProcessStart, model.ObservationKindProcessExit:
		if e.Process == nil {
			return errCode(CodeInvalidObservationPayload, "event", "", "process", "missing process payload", nil)
		}
	case model.ObservationKindFilesystemCreate, model.ObservationKindFilesystemRead, model.ObservationKindFilesystemWrite, model.ObservationKindFilesystemDelete, model.ObservationKindFilesystemRename, model.ObservationKindFilesystemChmod:
		if e.Filesystem == nil {
			return errCode(CodeInvalidObservationPayload, "event", "", "filesystem", "missing filesystem payload", nil)
		}
	case model.ObservationKindDNSQuery, model.ObservationKindNetworkConnection:
		if e.Network == nil {
			return errCode(CodeInvalidObservationPayload, "event", "", "network", "missing network payload", nil)
		}
	case model.ObservationKindArtifactActivity:
		if e.Artifact == nil {
			return errCode(CodeInvalidObservationPayload, "event", "", "artifact", "missing artifact payload", nil)
		}
	case model.ObservationKindScenarioStarted, model.ObservationKindScenarioCompleted:
		if e.Scenario == nil {
			return errCode(CodeInvalidObservationPayload, "event", "", "scenario", "missing scenario payload", nil)
		}
	case model.ObservationKindObserverWarning, model.ObservationKindUnsupportedObservation:
		if e.ObserverWarning == nil {
			return errCode(CodeInvalidObservationPayload, "event", "", "warning", "missing warning payload", nil)
		}
	case model.ObservationKindResourceLimit:
		if e.ResourceLimit == nil {
			return errCode(CodeInvalidObservationPayload, "event", "", "resource", "missing resource payload", nil)
		}
	}
	return nil
}

func cloneLimitations(in []model.Limitation) []model.Limitation {
	if in == nil {
		return []model.Limitation{}
	}
	out := make([]model.Limitation, len(in))
	copy(out, in)
	return out
}
