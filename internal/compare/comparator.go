package compare

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/observe"
)

type Comparator struct{ limits Limits }

type FrozenDelta struct {
	doc    model.BehavioralDelta
	json   []byte
	digest model.Digest
}

func New(limits Limits) (*Comparator, error) {
	l, err := validateLimits(limits)
	if err != nil {
		return nil, err
	}
	return &Comparator{limits: l}, nil
}

func (c *Comparator) Compare(ctx context.Context, traces *observe.TraceSet) (*FrozenDelta, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextErr(err)
	}
	if c == nil {
		return nil, errCode(CodeInvalidLimits, "compare", "", "comparator", "comparator is nil", nil)
	}
	if traces == nil {
		return nil, errCode(CodeNilTraceSet, "input", "", "traceSet", "TraceSet is required", nil)
	}
	return c.compareDocument(ctx, traces.Document())
}

func (c *Comparator) compareDocument(ctx context.Context, doc observe.TraceSetDocument) (*FrozenDelta, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextErr(err)
	}
	if err := validateTraceDocument(doc, c.limits); err != nil {
		return nil, err
	}
	profile := comparisonProfile()
	groups := groupScenarios(doc.Attempts)
	records := []model.DeltaRecord{}
	scenarios := []model.ScenarioComparison{}
	var totalRefs int64
	for _, sg := range groups {
		if err := ctx.Err(); err != nil {
			return nil, contextErr(err)
		}
		sc, lims := scenarioComparison(sg)
		scenarios = append(scenarios, sc)
		for _, lim := range lims {
			records = append(records, coverageRecord(doc.Profile.Version, sg, lim))
		}
		recs, refs, err := c.compareScenario(doc.Profile.Version, sg)
		if err != nil {
			return nil, err
		}
		records = append(records, recs...)
		totalRefs += refs
		if totalRefs > c.limits.MaxEvidenceRefsTotal {
			return nil, errCode(CodeEvidenceReferenceLimit, "records", sg.id, "evidence", "evidence reference limit exceeded", nil)
		}
		if int64(len(records)) > c.limits.MaxDeltaRecords {
			return nil, errCode(CodeDeltaLimit, "records", sg.id, "count", "delta record limit exceeded", nil)
		}
	}
	records = sortRecords(records)
	for i := range records {
		if records[i].ID == "" {
			id, err := deltaRecordID(profile.Version, doc.Profile.Version, firstScenario(records[i].ScenarioIDs), records[i])
			if err != nil {
				return nil, err
			}
			records[i].ID = id
		}
	}
	ids := scenarioIDs(groups)
	delta := model.BehavioralDelta{SchemaVersion: model.SchemaVersionBehavioralDeltaV1Alpha1, ID: "delta-" + string(doc.PlanDigest)[7:23], RunID: doc.RunID, PlanDigest: doc.PlanDigest, ManifestDigest: doc.ManifestDigest, ManifestVerificationMode: string(doc.ManifestVerification.Mode), ExecutionComplete: doc.ExecutionComplete, EvidenceComplete: doc.EvidenceComplete, EvidenceContext: doc.EvidenceContext, ComparisonProfile: profile, NormalizationProfileVersion: doc.Profile.Version, ScenarioIDs: ids, ScenarioComparisons: scenarios, Records: records, Summary: summarize(records), Limitations: sortLimitations(doc.Limitations)}
	data, err := json.Marshal(delta)
	if err != nil {
		return nil, errCode(CodeSerializationFailed, "freeze", "", "json", "serialize delta", err)
	}
	if int64(len(data)) > c.limits.MaxDeltaJSONBytes {
		return nil, errCode(CodeDeltaTooLarge, "freeze", "", "json", "delta JSON exceeds limit", nil)
	}
	return &FrozenDelta{doc: cloneDelta(delta), json: append([]byte(nil), data...), digest: digestBytes(deltaJSONDomain, data)}, nil
}

func (d *FrozenDelta) Document() model.BehavioralDelta {
	if d == nil {
		return model.BehavioralDelta{}
	}
	return cloneDelta(d.doc)
}
func (d *FrozenDelta) JSON() []byte {
	if d == nil {
		return nil
	}
	return append([]byte(nil), d.json...)
}
func (d *FrozenDelta) Digest() model.Digest {
	if d == nil {
		return ""
	}
	return d.digest
}

func validateTraceDocument(doc observe.TraceSetDocument, limits Limits) error {
	if doc.SchemaVersion != observe.TraceSetSchemaV1Alpha1 || doc.Profile.Version != observe.ProfileVersionV1Alpha1 {
		return errCode(CodeUnsupportedNormalizationProfile, "input", "", "profile", "unsupported normalization profile", nil)
	}
	if !validDigest(doc.PlanDigest) || !validDigest(doc.ManifestDigest) {
		return errCode(CodeInvalidTraceSet, "input", "", "digest", "invalid plan or manifest digest", nil)
	}
	if doc.RunID == "" {
		return errCode(CodeInvalidTraceSet, "input", "", "runId", "missing run id", nil)
	}
	if len(doc.Attempts) == 0 || int64(len(doc.Attempts)) > limits.MaxAttempts {
		return errCode(CodeInvalidAttemptInventory, "attempts", "", "count", "invalid attempt count", nil)
	}
	seenAttempts := map[string]struct{}{}
	seenCoordinates := map[string]struct{}{}
	seenFacts := map[observe.FactID]struct{}{}
	seenScenario := map[string]struct{}{}
	var facts int64
	var sawHead bool
	for i, a := range doc.Attempts {
		if a.AttemptID == "" || a.ScenarioID == "" || a.Repetition == 0 {
			return errCode(CodeInvalidAttemptInventory, "attempts", a.ScenarioID, "identity", "attempt identity incomplete", nil)
		}
		if _, ok := seenAttempts[a.AttemptID]; ok {
			return errCode(CodeDuplicateAttempt, "attempts", a.ScenarioID, a.AttemptID, "duplicate attempt", nil)
		}
		seenAttempts[a.AttemptID] = struct{}{}
		coord := string(a.Revision) + "\x00" + a.ScenarioID + "\x00" + itoa(a.Repetition)
		if _, ok := seenCoordinates[coord]; ok {
			return errCode(CodeDuplicateAttempt, "attempts", a.ScenarioID, "coordinate", "duplicate attempt coordinate", nil)
		}
		seenCoordinates[coord] = struct{}{}
		if a.Revision == model.RevisionKindHead {
			sawHead = true
		} else if a.Revision == model.RevisionKindBase && sawHead {
			return errCode(CodeInvalidAttemptInventory, "attempts", a.ScenarioID, "order", "base attempt appeared after head", nil)
		} else if a.Revision != model.RevisionKindBase {
			return errCode(CodeInvalidAttemptInventory, "attempts", a.ScenarioID, "revision", "invalid revision", nil)
		}
		if _, ok := seenScenario[a.ScenarioID]; !ok && a.Revision == model.RevisionKindBase {
			seenScenario[a.ScenarioID] = struct{}{}
		}
		if int64(len(a.Facts)) > limits.MaxFactsPerAttempt {
			return errCode(CodeComparisonLimit, "facts", a.ScenarioID, "count", "attempt fact limit exceeded", nil)
		}
		facts += int64(len(a.Facts))
		if facts > limits.MaxFactsTotal {
			return errCode(CodeComparisonLimit, "facts", "", "count", "total fact limit exceeded", nil)
		}
		if uint64(i+1) != a.Ordinal {
			return errCode(CodeInvalidAttemptInventory, "attempts", a.ScenarioID, "ordinal", "attempt order is not deterministic", nil)
		}
		for _, f := range a.Facts {
			if err := validateFact(a, f, seenFacts); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateFact(a observe.AttemptTrace, f observe.Fact, seen map[observe.FactID]struct{}) error {
	if f.ID == "" {
		return errCode(CodeInvalidTraceSet, "facts", a.ScenarioID, "id", "fact id missing", nil)
	}
	if _, ok := seen[f.ID]; ok {
		return errCode(CodeDuplicateFactID, "facts", a.ScenarioID, string(f.ID), "duplicate fact id", nil)
	}
	seen[f.ID] = struct{}{}
	if !validDigest(f.SemanticDigest) {
		return errCode(CodeInvalidSemanticDigest, "facts", a.ScenarioID, string(f.SemanticDigest), "invalid semantic digest", nil)
	}
	if !isSupportedFactKind(f.Kind) {
		return errCode(CodeUnsupportedFactKind, "facts", a.ScenarioID, string(f.Kind), "unsupported fact kind", nil)
	}
	if !isSupportedSource(f.Source) {
		return errCode(CodeInvalidObservationSource, "facts", a.ScenarioID, string(f.Source), "invalid observation source", nil)
	}
	if err := validateFactPayload(f); err != nil {
		return err
	}
	for _, ref := range f.Evidence {
		if ref.Revision != a.Revision || ref.ScenarioID != a.ScenarioID || ref.Repetition != a.Repetition || ref.EventID == "" || ref.EventSequence == 0 || !validDigest(ref.EventStreamDigest) || ref.EventStreamPath == "" {
			return errCode(CodeEvidenceReferenceInvalid, "facts", a.ScenarioID, "evidence", "invalid evidence reference", nil)
		}
	}
	return nil
}

func validateFactPayload(f observe.Fact) error {
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
		return errCode(CodeInvalidFactPayload, "facts", "", string(f.Kind), "fact must contain exactly one typed payload", nil)
	}
	return nil
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
