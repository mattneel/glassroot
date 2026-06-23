package fake

import (
	"testing"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

func TestProgramConstructorDeepCopiesInput(t *testing.T) {
	program := Program{PlanDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111", Attempts: []AttemptScript{{Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1, Events: []SyntheticEvent{{Draft: runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindObserverWarning, ObserverWarning: &model.ObserverWarningObservation{Code: "synthetic", Message: "bounded", Limitations: []model.Limitation{}}}}}, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0)}}}}
	backend, err := New(program)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	program.Attempts[0].ScenarioID = "mutated"
	program.Attempts[0].Events[0].Draft.ObserverWarning.Message = "mutated"
	got := backend.ProgramForTest()
	if got.Attempts[0].ScenarioID != "test" || got.Attempts[0].Events[0].Draft.ObserverWarning.Message != "bounded" {
		t.Fatalf("program was not deep-copied: %+v", got)
	}
}

func FuzzFakeProgramAttemptKeys(f *testing.F) {
	f.Add("sha256:1111111111111111111111111111111111111111111111111111111111111111", string(model.RevisionKindBase), "test", uint32(1))
	f.Add("bad", string(model.RevisionKind("evil")), "\x1b", uint32(0))
	f.Fuzz(func(t *testing.T, digest, revision, scenario string, repetition uint32) {
		_, _ = New(Program{PlanDigest: model.Digest(digest), Attempts: []AttemptScript{{Revision: model.RevisionKind(revision), ScenarioID: scenario, Repetition: repetition, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0)}}}})
	})
}

func intPtr(v int) *int { return &v }
