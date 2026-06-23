package compare

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/observe"
)

type scenarioGroup struct {
	id   string
	base []observe.AttemptTrace
	head []observe.AttemptTrace
}
type variant struct {
	key        string
	kind       observe.FactKind
	source     model.ObservationSource
	semantic   model.Digest
	factsByRep map[uint32][]observe.Fact
	facts      []observe.Fact
	profile    model.OccurrenceProfile
	anchor     model.Digest
	consumed   bool
	ambiguous  bool
}
type repetitionFacts struct {
	repetition uint32
	coverage   observe.CoverageState
	facts      int
}

func groupScenarios(attempts []observe.AttemptTrace) []scenarioGroup {
	idx := map[string]int{}
	groups := []scenarioGroup{}
	for _, a := range attempts {
		i, ok := idx[a.ScenarioID]
		if !ok {
			idx[a.ScenarioID] = len(groups)
			groups = append(groups, scenarioGroup{id: a.ScenarioID})
			i = len(groups) - 1
		}
		if a.Revision == model.RevisionKindBase {
			groups[i].base = append(groups[i].base, a)
		} else {
			groups[i].head = append(groups[i].head, a)
		}
	}
	for i := range groups {
		sort.Slice(groups[i].base, func(a, b int) bool { return groups[i].base[a].Repetition < groups[i].base[b].Repetition })
		sort.Slice(groups[i].head, func(a, b int) bool { return groups[i].head[a].Repetition < groups[i].head[b].Repetition })
	}
	return groups
}

func (c *Comparator) compareScenario(normProfile string, sg scenarioGroup) ([]model.DeltaRecord, int64, error) {
	base := buildVariants(sg.base)
	head := buildVariants(sg.head)
	for _, v := range base {
		v.profile, _ = profileForVariant(sg.base, v)
		a, err := typedAnchorDigest(v.facts[0])
		if err != nil {
			return nil, 0, err
		}
		v.anchor = a
	}
	for _, v := range head {
		v.profile, _ = profileForVariant(sg.head, v)
		a, err := typedAnchorDigest(v.facts[0])
		if err != nil {
			return nil, 0, err
		}
		v.anchor = a
	}
	records := []model.DeltaRecord{}
	keys := unionKeys(base, head)
	for _, k := range keys {
		b, h := base[k], head[k]
		if b != nil && h != nil {
			if occurrenceEqual(b.profile, h.profile) {
				b.consumed = true
				h.consumed = true
				continue
			}
			kind := model.DeltaKindCountChanged
			if b.profile.Repeatability != h.profile.Repeatability {
				kind = model.DeltaKindStabilityChanged
			}
			rec := c.makeRecord(normProfile, sg.id, kind, b, h, nil, nil)
			records = append(records, rec)
			b.consumed = true
			h.consumed = true
		}
	}
	anchorBase := map[model.Digest][]*variant{}
	anchorHead := map[model.Digest][]*variant{}
	for _, v := range base {
		if !v.consumed && v.anchor != "" {
			anchorBase[v.anchor] = append(anchorBase[v.anchor], v)
		}
	}
	for _, v := range head {
		if !v.consumed && v.anchor != "" {
			anchorHead[v.anchor] = append(anchorHead[v.anchor], v)
		}
	}
	anchors := unionAnchorKeys(anchorBase, anchorHead)
	for _, a := range anchors {
		bs, hs := anchorBase[a], anchorHead[a]
		if len(bs) == 1 && len(hs) == 1 && bs[0].kind == hs[0].kind && bs[0].source == hs[0].source {
			changed := changedFields(bs[0].facts[0], hs[0].facts[0])
			if len(changed) > 0 {
				rec := c.makeRecord(normProfile, sg.id, model.DeltaKindModified, bs[0], hs[0], changed, nil)
				records = append(records, rec)
				bs[0].consumed = true
				hs[0].consumed = true
			}
		} else if len(bs) > 0 && len(hs) > 0 {
			for _, v := range bs {
				v.ambiguous = true
			}
			for _, v := range hs {
				v.ambiguous = true
			}
		}
	}
	baseComplete := attemptsComplete(sg.base)
	headComplete := attemptsComplete(sg.head)
	for _, v := range sortedVariants(base) {
		if v.consumed {
			continue
		}
		kind := model.DeltaKindRemoved
		basis := basisForAbsence(headComplete, v.profile)
		if !headComplete {
			kind = model.DeltaKindCoverageChanged
		}
		rec := c.makeRecord(normProfile, sg.id, kind, v, nil, nil, ambiguityLimit(v))
		rec.Basis = basis
		records = append(records, rec)
	}
	for _, v := range sortedVariants(head) {
		if v.consumed {
			continue
		}
		kind := model.DeltaKindAdded
		basis := basisForAbsence(baseComplete, v.profile)
		if !baseComplete {
			kind = model.DeltaKindCoverageChanged
		}
		rec := c.makeRecord(normProfile, sg.id, kind, nil, v, nil, ambiguityLimit(v))
		rec.Basis = basis
		records = append(records, rec)
	}
	if od := orderRecord(normProfile, sg); od.ID != "" {
		records = append(records, od)
	}
	var refs int64
	for _, r := range records {
		refs += int64(len(r.BaseEvidence) + len(r.HeadEvidence))
	}
	return records, refs, nil
}

func buildVariants(attempts []observe.AttemptTrace) map[string]*variant {
	out := map[string]*variant{}
	for _, a := range attempts {
		for _, f := range a.Facts {
			key := string(f.SemanticDigest) + "\x00" + string(f.Kind) + "\x00" + string(f.Source)
			v := out[key]
			if v == nil {
				v = &variant{key: key, kind: f.Kind, source: f.Source, semantic: f.SemanticDigest, factsByRep: map[uint32][]observe.Fact{}}
				out[key] = v
			}
			v.factsByRep[a.Repetition] = append(v.factsByRep[a.Repetition], f)
			v.facts = append(v.facts, f)
		}
	}
	return out
}

func profileForVariant(attempts []observe.AttemptTrace, v *variant) (model.OccurrenceProfile, error) {
	reps := make([]repetitionFacts, 0, len(attempts))
	for _, a := range attempts {
		reps = append(reps, repetitionFacts{repetition: a.Repetition, coverage: a.Coverage, facts: len(v.factsByRep[a.Repetition])})
	}
	return buildOccurrenceProfile(len(attempts), reps)
}

func buildOccurrenceProfile(planned int, reps []repetitionFacts) (model.OccurrenceProfile, error) {
	out := model.OccurrenceProfile{PlannedRepetitionCount: int64(planned), Repetitions: []model.RepetitionOccurrence{}}
	minSet := false
	for _, r := range reps {
		ro := model.RepetitionOccurrence{Repetition: r.repetition}
		if r.coverage == observe.CoverageComplete {
			ro.Coverage = model.CoverageAssessmentComplete
			ro.CountKnown = true
			ro.Count = int64(r.facts)
			out.CompleteRepetitionCount++
			out.TotalKnownCount += ro.Count
			if !minSet || ro.Count < out.MinimumKnownCount {
				out.MinimumKnownCount = ro.Count
				minSet = true
			}
			if ro.Count > out.MaximumKnownCount {
				out.MaximumKnownCount = ro.Count
			}
		} else {
			ro.Coverage = model.CoverageAssessmentPartial
			out.IncompleteRepetitionCount++
		}
		out.Repetitions = append(out.Repetitions, ro)
	}
	if out.CompleteRepetitionCount == int64(planned) {
		out.Coverage = model.CoverageAssessmentComplete
	} else if out.CompleteRepetitionCount > 0 {
		out.Coverage = model.CoverageAssessmentPartial
	} else {
		out.Coverage = model.CoverageAssessmentNone
	}
	if out.Coverage != model.CoverageAssessmentComplete || out.CompleteRepetitionCount == 0 {
		out.Repeatability = model.RepeatabilityNotAssessable
	} else if out.CompleteRepetitionCount == 1 {
		out.Repeatability = model.RepeatabilitySingleSample
	} else if out.MinimumKnownCount == out.MaximumKnownCount {
		out.Repeatability = model.RepeatabilityStable
	} else {
		out.Repeatability = model.RepeatabilityVariable
	}
	return out, nil
}

func (c *Comparator) makeRecord(normProfile, scenario string, kind model.DeltaKind, b, h *variant, changed []string, lim []model.Limitation) model.DeltaRecord {
	rec := model.DeltaRecord{Kind: kind, ScenarioIDs: []string{scenario}, Limitations: sortLimitations(lim), Evidence: []model.EvidenceRef{}, ChangedFields: cloneStrings(changed)}
	if b != nil {
		rec.FactKind = string(b.kind)
		rec.Source = b.source
		rec.AnchorDigest = b.anchor
		rec.BaseObserved = true
		rec.BaseOccurrence = b.profile
		rec.BaseFacts = snapshots(b.facts)
		rec.BaseEvidence = evidenceRefs(b.facts)
		rec.BaseSemanticDigests = []model.Digest{b.semantic}
		rec.Evidence = append(rec.Evidence, rec.BaseEvidence...)
	}
	if h != nil {
		rec.FactKind = string(h.kind)
		rec.Source = h.source
		rec.AnchorDigest = h.anchor
		rec.HeadObserved = true
		rec.HeadOccurrence = h.profile
		rec.HeadFacts = snapshots(h.facts)
		rec.HeadEvidence = evidenceRefs(h.facts)
		rec.HeadSemanticDigests = []model.Digest{h.semantic}
		rec.Evidence = append(rec.Evidence, rec.HeadEvidence...)
	}
	if b == nil {
		rec.BaseOccurrence = emptyOccurrence()
	}
	if h == nil {
		rec.HeadOccurrence = emptyOccurrence()
	}
	if rec.Basis == "" {
		if b != nil && b.profile.Repeatability == model.RepeatabilityVariable || h != nil && h.profile.Repeatability == model.RepeatabilityVariable {
			rec.Basis = model.ComparisonBasisRepetitionVariable
		} else if b != nil && b.profile.Coverage != model.CoverageAssessmentComplete || h != nil && h.profile.Coverage != model.CoverageAssessmentComplete {
			rec.Basis = model.ComparisonBasisCoverageLimited
		} else if b != nil && b.profile.Repeatability == model.RepeatabilitySingleSample || h != nil && h.profile.Repeatability == model.RepeatabilitySingleSample {
			rec.Basis = model.ComparisonBasisSingleSample
		} else {
			rec.Basis = model.ComparisonBasisCompleteObservation
		}
	}
	rec.Evidence = dedupeEvidence(rec.Evidence)
	rec.BaseEvidence = dedupeEvidence(rec.BaseEvidence)
	rec.HeadEvidence = dedupeEvidence(rec.HeadEvidence)
	rec.Summary = summaryFor(rec)
	rec.BaseSemanticDigests = sortDigests(rec.BaseSemanticDigests)
	rec.HeadSemanticDigests = sortDigests(rec.HeadSemanticDigests)
	id, _ := deltaRecordID(ComparisonProfileVersionV1Alpha1, normProfile, scenario, rec)
	rec.ID = id
	return rec
}

func basisForAbsence(otherComplete bool, p model.OccurrenceProfile) model.ComparisonBasis {
	if !otherComplete || p.Coverage != model.CoverageAssessmentComplete {
		return model.ComparisonBasisCoverageLimited
	}
	if p.Repeatability == model.RepeatabilitySingleSample {
		return model.ComparisonBasisSingleSample
	}
	if p.Repeatability == model.RepeatabilityVariable {
		return model.ComparisonBasisRepetitionVariable
	}
	return model.ComparisonBasisCompleteObservation
}
func attemptsComplete(at []observe.AttemptTrace) bool {
	if len(at) == 0 {
		return false
	}
	for _, a := range at {
		if a.Coverage != observe.CoverageComplete {
			return false
		}
	}
	return true
}
func ambiguityLimit(v *variant) []model.Limitation {
	if v != nil && v.ambiguous {
		return []model.Limitation{{ID: "ambiguous-correlation", Summary: "Typed anchor matched multiple unmatched variants; no one-to-one modification was inferred."}}
	}
	return nil
}

func unionKeys(a, b map[string]*variant) []string {
	m := map[string]struct{}{}
	for k := range a {
		m[k] = struct{}{}
	}
	for k := range b {
		m[k] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func unionAnchorKeys(a, b map[model.Digest][]*variant) []model.Digest {
	m := map[model.Digest]struct{}{}
	for k := range a {
		m[k] = struct{}{}
	}
	for k := range b {
		m[k] = struct{}{}
	}
	out := make([]model.Digest, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
func sortedVariants(m map[string]*variant) []*variant {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*variant, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

func occurrenceEqual(a, b model.OccurrenceProfile) bool {
	da, _ := json.Marshal(a)
	db, _ := json.Marshal(b)
	return string(da) == string(db)
}

func changedFields(a, b observe.Fact) []string {
	fields := []string{}
	switch a.Kind {
	case observe.FactKindFilesystemCreate, observe.FactKindFilesystemRead, observe.FactKindFilesystemWrite, observe.FactKindFilesystemDelete, observe.FactKindFilesystemRename, observe.FactKindFilesystemChmod:
		if a.Filesystem == nil || b.Filesystem == nil {
			return nil
		}
		if a.Filesystem.Digest != b.Filesystem.Digest {
			fields = append(fields, "filesystem.digest")
		}
		if a.Filesystem.Mode != b.Filesystem.Mode {
			fields = append(fields, "filesystem.mode")
		}
		if a.Filesystem.SizeBytes != b.Filesystem.SizeBytes {
			fields = append(fields, "filesystem.sizeBytes")
		}
		if a.Filesystem.Executable != b.Filesystem.Executable {
			fields = append(fields, "filesystem.executable")
		}
		if a.Filesystem.Truncated != b.Filesystem.Truncated {
			fields = append(fields, "filesystem.truncated")
		}
	case observe.FactKindArtifactActivity:
		if a.Artifact == nil || b.Artifact == nil {
			return nil
		}
		if a.Artifact.Digest != b.Artifact.Digest {
			fields = append(fields, "artifact.digest")
		}
		if a.Artifact.SizeBytes != b.Artifact.SizeBytes {
			fields = append(fields, "artifact.sizeBytes")
		}
		if a.Artifact.Executable != b.Artifact.Executable {
			fields = append(fields, "artifact.executable")
		}
		if a.Artifact.Operation != b.Artifact.Operation {
			fields = append(fields, "artifact.operation")
		}
	case observe.FactKindNetworkConnection, observe.FactKindDNSQuery:
		if a.Network == nil || b.Network == nil {
			return nil
		}
		if a.Network.Result != b.Network.Result {
			fields = append(fields, "network.result")
		}
		if a.Network.DurationMillis != b.Network.DurationMillis {
			fields = append(fields, "network.durationMillis")
		}
		if strings.Join(a.Network.ResolvedAddresses, "\x00") != strings.Join(b.Network.ResolvedAddresses, "\x00") {
			fields = append(fields, "network.resolvedAddresses")
		}
	case observe.FactKindScenarioStarted, observe.FactKindScenarioCompleted:
		if a.Scenario == nil || b.Scenario == nil {
			return nil
		}
		if a.Scenario.Status != b.Scenario.Status {
			fields = append(fields, "scenario.status")
		}
		if a.Scenario.DurationMillis != b.Scenario.DurationMillis {
			fields = append(fields, "scenario.durationMillis")
		}
		if a.Scenario.Message != b.Scenario.Message {
			fields = append(fields, "scenario.message")
		}
	case observe.FactKindObserverWarning, observe.FactKindUnsupportedObservation:
		if a.Warning == nil || b.Warning == nil {
			return nil
		}
		if a.Warning.Message != b.Warning.Message {
			fields = append(fields, "warning.message")
		}
		if a.Warning.Unsupported != b.Warning.Unsupported {
			fields = append(fields, "warning.unsupported")
		}
	case observe.FactKindResourceLimit:
		if a.Resource == nil || b.Resource == nil {
			return nil
		}
		if a.Resource.LimitValue != b.Resource.LimitValue {
			fields = append(fields, "resource.limitValue")
		}
		if a.Resource.ObservedValue != b.Resource.ObservedValue {
			fields = append(fields, "resource.observedValue")
		}
		if a.Resource.Exceeded != b.Resource.Exceeded {
			fields = append(fields, "resource.exceeded")
		}
	case observe.FactKindProcessStart, observe.FactKindProcessExit:
		if a.Process == nil || b.Process == nil {
			return nil
		}
		if !intPtrEqual(a.Process.ExitCode, b.Process.ExitCode) {
			fields = append(fields, "process.exitCode")
		}
		if a.Process.DurationMillis != b.Process.DurationMillis {
			fields = append(fields, "process.durationMillis")
		}
	}
	sort.Strings(fields)
	return fields
}
func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func orderRecord(normProfile string, sg scenarioGroup) model.DeltaRecord {
	baseSeqs, okB := stableSequences(sg.base)
	headSeqs, okH := stableSequences(sg.head)
	if !okB || !okH || len(baseSeqs) == 0 || len(headSeqs) == 0 {
		return model.DeltaRecord{}
	}
	if !sameMultiset(baseSeqs, headSeqs) || equalDigestSlices(baseSeqs, headSeqs) {
		return model.DeltaRecord{}
	}
	rec := model.DeltaRecord{Kind: model.DeltaKindOrderChanged, ScenarioIDs: []string{sg.id}, Basis: model.ComparisonBasisCompleteObservation, FactKind: "sequence", ChangedFields: []string{"sequence.order"}, BaseOccurrence: emptyOccurrence(), HeadOccurrence: emptyOccurrence(), BaseSemanticDigests: baseSeqs, HeadSemanticDigests: headSeqs, Evidence: []model.EvidenceRef{}, BaseEvidence: []model.EvidenceRef{}, HeadEvidence: []model.EvidenceRef{}, BaseFacts: []model.DeltaFactSnapshot{}, HeadFacts: []model.DeltaFactSnapshot{}, Limitations: []model.Limitation{}, Summary: "Complete repetitions contained the same semantic facts in a different stable order."}
	id, _ := deltaRecordID(ComparisonProfileVersionV1Alpha1, normProfile, sg.id, rec)
	rec.ID = id
	return rec
}
func stableSequences(at []observe.AttemptTrace) ([]model.Digest, bool) {
	if len(at) < 2 {
		return nil, false
	}
	var first []model.Digest
	for i, a := range at {
		if a.Coverage != observe.CoverageComplete {
			return nil, false
		}
		seq := make([]model.Digest, 0, len(a.Facts))
		for _, f := range a.Facts {
			seq = append(seq, f.SemanticDigest)
		}
		if i == 0 {
			first = seq
		} else if !equalDigestSlices(first, seq) {
			return nil, false
		}
	}
	return first, true
}
func sameMultiset(a, b []model.Digest) bool {
	aa := sortDigests(a)
	bb := sortDigests(b)
	return equalDigestSlices(aa, bb)
}
func equalDigestSlices(a, b []model.Digest) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func scenarioComparison(sg scenarioGroup) (model.ScenarioComparison, []model.Limitation) {
	sc := model.ScenarioComparison{ScenarioID: sg.id, BaseRepetitions: attemptCoverage(sg.base), HeadRepetitions: attemptCoverage(sg.head), RepeatabilityNotes: []model.Limitation{}, Limitations: []model.Limitation{}}
	if attemptsComplete(sg.base) && attemptsComplete(sg.head) {
		sc.Coverage = model.CoverageAssessmentComplete
	} else if len(sg.base)+len(sg.head) > 0 {
		sc.Coverage = model.CoverageAssessmentPartial
	} else {
		sc.Coverage = model.CoverageAssessmentNone
	}
	lims := []model.Limitation{}
	if !attemptsComplete(sg.base) || !attemptsComplete(sg.head) {
		lims = append(lims, model.Limitation{ID: "scenario-coverage-limited", Summary: "At least one repetition has incomplete observation coverage."})
	}
	return sc, lims
}
func attemptCoverage(at []observe.AttemptTrace) []model.AttemptCoverage {
	out := make([]model.AttemptCoverage, 0, len(at))
	for _, a := range at {
		cov := model.CoverageAssessmentPartial
		if a.Coverage == observe.CoverageComplete {
			cov = model.CoverageAssessmentComplete
		} else if a.Coverage == observe.CoverageNotStarted {
			cov = model.CoverageAssessmentNone
		}
		out = append(out, model.AttemptCoverage{AttemptID: a.AttemptID, Revision: a.Revision, Repetition: a.Repetition, Coverage: cov})
	}
	return out
}
func coverageRecord(normProfile string, sg scenarioGroup, lim model.Limitation) model.DeltaRecord {
	rec := model.DeltaRecord{Kind: model.DeltaKindCoverageChanged, Summary: "Scenario coverage limitation is explicit comparison data.", Basis: model.ComparisonBasisCoverageLimited, ScenarioIDs: []string{sg.id}, Limitations: []model.Limitation{lim}, Evidence: []model.EvidenceRef{}, ChangedFields: []string{}, BaseOccurrence: emptyOccurrence(), HeadOccurrence: emptyOccurrence(), BaseEvidence: []model.EvidenceRef{}, HeadEvidence: []model.EvidenceRef{}, BaseFacts: []model.DeltaFactSnapshot{}, HeadFacts: []model.DeltaFactSnapshot{}}
	id, _ := deltaRecordID(ComparisonProfileVersionV1Alpha1, normProfile, sg.id, rec)
	rec.ID = id
	return rec
}

func firstScenario(ids []string) string {
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}
func scenarioIDs(groups []scenarioGroup) []string {
	out := make([]string, len(groups))
	for i, g := range groups {
		out[i] = g.id
	}
	return out
}
func summarize(records []model.DeltaRecord) model.DeltaSummary {
	s := model.DeltaSummary{TotalRecords: int64(len(records))}
	for _, r := range records {
		switch r.Kind {
		case model.DeltaKindAdded:
			s.Added++
		case model.DeltaKindRemoved:
			s.Removed++
		case model.DeltaKindModified:
			s.Modified++
		case model.DeltaKindCountChanged:
			s.CountChanged++
		case model.DeltaKindOrderChanged:
			s.OrderChanged++
		case model.DeltaKindStabilityChanged:
			s.StabilityChanged++
		case model.DeltaKindCoverageChanged:
			s.CoverageChanged++
		}
	}
	return s
}
func summaryFor(r model.DeltaRecord) string {
	return fmt.Sprintf("%s %s comparison for scenario %s", r.Kind, r.FactKind, firstScenario(r.ScenarioIDs))
}

func emptyOccurrence() model.OccurrenceProfile {
	return model.OccurrenceProfile{Repetitions: []model.RepetitionOccurrence{}, Coverage: model.CoverageAssessmentNone, Repeatability: model.RepeatabilityNotAssessable}
}
