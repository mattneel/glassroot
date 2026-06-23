package observe

import (
	"fmt"

	"github.com/mattneel/glassroot/internal/model"
)

type attemptState struct {
	profile    NormalizationProfile
	planDigest model.Digest
	attemptID  string
	attemptKey RawEvidenceReference
	limits     Limits
	process    map[model.ObservationSource]*processNamespace
	time       map[model.ObservationSource]*timeNamespace
	facts      uint64
	refs       uint64
}

type processNamespace struct {
	active      map[int64]processGeneration
	occurrence  map[string]uint64
	generations uint64
}
type processGeneration struct {
	pid            int64
	stable         ProcessID
	parent         ProcessID
	parentRelation string
	seed           string
	exited         bool
}
type timeNamespace struct {
	originSet bool
	origin    timeComparable
	last      timeComparable
}
type timeComparable struct{ nsec int64 }

func newAttemptState(profile NormalizationProfile, planDigest model.Digest, attemptID string, ref RawEvidenceReference, limits Limits) *attemptState {
	return &attemptState{profile: profile, planDigest: planDigest, attemptID: attemptID, attemptKey: ref, limits: limits, process: map[model.ObservationSource]*processNamespace{}, time: map[model.ObservationSource]*timeNamespace{}}
}
func newAttemptStateForTest(profile NormalizationProfile) *attemptState {
	l := DefaultLimits()
	return newAttemptState(profile, model.Digest("sha256:"+repeatChar('0', 64)), "att-test", RawEvidenceReference{}, l)
}
func repeatChar(c byte, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return string(b)
}

func (s *attemptState) ns(src model.ObservationSource) *processNamespace {
	n := s.process[src]
	if n == nil {
		n = &processNamespace{active: map[int64]processGeneration{}, occurrence: map[string]uint64{}}
		s.process[src] = n
	}
	return n
}
func (s *attemptState) normalizeTiming(event model.ObservationEvent) (NormalizedTiming, []model.Limitation) {
	tn := event.ObservedAt.UTC().Round(0).UnixNano()
	ns := s.time[event.Source]
	if ns == nil {
		ns = &timeNamespace{}
		s.time[event.Source] = ns
	}
	lim := []model.Limitation{}
	if !ns.originSet {
		ns.originSet = true
		ns.origin = timeComparable{tn}
		ns.last = timeComparable{tn}
	}
	rel := tn - ns.origin.nsec
	if tn < ns.last.nsec {
		lim = append(lim, model.Limitation{ID: "clock-regression", Summary: "Observation timestamp moved backward within one source clock."})
	}
	ns.last = timeComparable{tn}
	return NormalizedTiming{SourceRelativeNanos: rel, IncludedInSemanticDigest: includeTiming(s.profile), ClockRegression: len(lim) > 0}, lim
}

func (s *attemptState) processStart(src model.ObservationSource, p *model.ProcessObservation) (*ProcessFact, []model.Limitation, error) {
	if p == nil {
		return nil, nil, errCode(CodeInvalidObservationPayload, "process", s.attemptID, "process", "missing process payload", nil)
	}
	ns := s.ns(src)
	lim := []model.Limitation{}
	if _, active := ns.active[p.ProcessID]; active {
		lim = append(lim, model.Limitation{ID: "active-pid-reuse", Summary: "A process start reused an active PID within one observer source."})
	}
	var parentID ProcessID
	relation := "root"
	if p.ParentProcessID != nil && *p.ParentProcessID != 0 {
		if parent, ok := ns.active[*p.ParentProcessID]; ok {
			parentID = parent.stable
			relation = "known-parent"
		} else {
			parentID = ProcessID("proc-unresolved-parent")
			relation = "unresolved-parent"
			lim = append(lim, model.Limitation{ID: "unresolved-parent", Summary: "A process start referenced a parent PID without an active generation."})
		}
	}
	exe := normalizeObservedPath(p.ExecutablePath, s.profile.RootAliases)
	seed := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%v\x00%v", src, parentID, relation, exe.Display, p.Arguments, p.Environment)
	occ := ns.occurrence[seed] + 1
	ns.occurrence[seed] = occ
	ns.generations++
	if int64(ns.generations) > s.limits.MaxProcessGenerationsPerAttempt || int64(len(ns.active)+1) > s.limits.MaxActiveProcessesPerAttempt {
		return nil, nil, errCode(CodeProcessLimit, "process", s.attemptID, "count", "process generation limit exceeded", nil)
	}
	stable := processID(string(src), string(parentID), relation, exe.Display, fmt.Sprint(p.Arguments), fmt.Sprint(p.Environment), fmt.Sprintf("%d", occ))
	ns.active[p.ProcessID] = processGeneration{pid: p.ProcessID, stable: stable, parent: parentID, parentRelation: relation, seed: seed}
	return &ProcessFact{Operation: p.Operation, StableID: stable, ParentStableID: parentID, ParentRelation: relation, Executable: exe, Arguments: cloneStringsBounded(p.Arguments), Environment: cloneEnv(p.Environment), ExitCode: cloneInt(p.ExitCode), DurationMillis: p.DurationMillis}, lim, nil
}
func (s *attemptState) processExit(src model.ObservationSource, p *model.ProcessObservation) (*ProcessFact, []model.Limitation, error) {
	if p == nil {
		return nil, nil, errCode(CodeInvalidObservationPayload, "process", s.attemptID, "process", "missing process payload", nil)
	}
	ns := s.ns(src)
	gen, ok := ns.active[p.ProcessID]
	lim := []model.Limitation{}
	if !ok {
		gen = processGeneration{stable: ProcessID("proc-unresolved-actor"), parentRelation: "unresolved-actor"}
		lim = append(lim, model.Limitation{ID: "unresolved-actor", Summary: "A process exit referenced a PID without an active generation."})
	} else {
		delete(ns.active, p.ProcessID)
	}
	return &ProcessFact{Operation: p.Operation, StableID: gen.stable, ParentStableID: gen.parent, ParentRelation: gen.parentRelation, Executable: normalizeObservedPath(p.ExecutablePath, s.profile.RootAliases), Arguments: cloneStringsBounded(p.Arguments), Environment: cloneEnv(p.Environment), ExitCode: cloneInt(p.ExitCode), DurationMillis: p.DurationMillis}, lim, nil
}

func cloneStringsBounded(in []string) []string {
	if in == nil {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
func cloneEnv(in []model.EnvEntry) []model.EnvEntry {
	if in == nil {
		return []model.EnvEntry{}
	}
	out := make([]model.EnvEntry, len(in))
	copy(out, in)
	return out
}
func cloneInt(in *int) *int {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}
