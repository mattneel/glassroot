package runner_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

func TestExecutePlanWithHooksPlacesPostAttemptEventsBeforeCompletion(t *testing.T) {
	plan := mustPlan(t, validPipelineYAML, validPipelineYAML)
	backend := &hookBackend{}
	hooks := &recordingHooks{}
	sink := &collectingSink{limit: 100}
	result, err := runner.ExecutePlanWithHooks(context.Background(), plan, backend, runner.WorkloadRequirements([]model.IsolationTier{model.IsolationTierDevelopmentOnly}), runner.DefaultLimits(), sink, hooks)
	if err != nil {
		t.Fatalf("ExecutePlanWithHooks() error = %v", err)
	}
	if !result.Complete || len(result.Attempts) != 4 {
		t.Fatalf("result mismatch: %+v", result)
	}
	var kinds []model.ObservationKind
	for _, ev := range sink.events[:4] {
		kinds = append(kinds, ev.Kind)
	}
	want := []model.ObservationKind{model.ObservationKindScenarioStarted, model.ObservationKindProcessStart, model.ObservationKindArtifactActivity, model.ObservationKindScenarioCompleted}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("first attempt event kinds = %v, want %v", kinds, want)
	}
	if hooks.before != 4 || hooks.after != 4 || hooks.abort != 0 || backend.calls != 4 {
		t.Fatalf("hook/backend counts before=%d after=%d abort=%d backend=%d", hooks.before, hooks.after, hooks.abort, backend.calls)
	}
}

func TestExecutePlanWithHooksAbortStopsAfterBackendError(t *testing.T) {
	plan := mustPlan(t, validPipelineYAML, validPipelineYAML)
	boom := errors.New("backend boom")
	backend := &hookBackend{fail: boom}
	hooks := &recordingHooks{}
	result, err := runner.ExecutePlanWithHooks(context.Background(), plan, backend, runner.WorkloadRequirements([]model.IsolationTier{model.IsolationTierDevelopmentOnly}), runner.DefaultLimits(), &collectingSink{limit: 100}, hooks)
	assertRunnerError(t, err, runner.CodeBackendFailed)
	if !errors.Is(err, boom) {
		t.Fatalf("backend error not preserved: %v", err)
	}
	if result.Complete || hooks.before != 1 || hooks.after != 0 || hooks.abort != 1 || backend.calls != 1 {
		t.Fatalf("abort counts/result mismatch: result=%+v hooks=%+v backend=%+v", result, hooks, backend)
	}
}

type hookBackend struct {
	calls int
	fail  error
}

func (b *hookBackend) Capabilities(context.Context) (model.RunnerCapabilities, error) {
	return model.RunnerCapabilities{Name: "hook", Version: "v1", IsolationTier: model.IsolationTierDevelopmentOnly, ExecutesTargetCode: true, SyntheticEvidence: false, EnforcesNetworkDeny: true}, nil
}

func (b *hookBackend) ValidatePlan(context.Context, model.Digest, []runner.AttemptRequest, runner.Limits) error {
	return nil
}

func (b *hookBackend) RunAttemptWithOutput(ctx context.Context, attempt runner.AttemptRequest, sink runner.DraftSink, logs runner.AttemptOutputSink) (runner.AttemptOutcome, error) {
	b.calls++
	if b.fail != nil {
		return runner.AttemptOutcome{}, b.fail
	}
	if err := sink.Emit(ctx, runner.EventDraft{Source: model.ObservationSourceSandboxRuntimeObserved, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: int64(b.calls), ExecutablePath: attempt.Shell, Arguments: []string{}, Environment: []model.EnvEntry{}}}); err != nil {
		return runner.AttemptOutcome{}, err
	}
	if err := logs.WriteLog(ctx, runner.LogStreamStdout, []byte("ok")); err != nil {
		return runner.AttemptOutcome{}, err
	}
	return runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 12}, nil
}

type recordingHooks struct{ before, after, abort int }

func (h *recordingHooks) BeforeAttempt(ctx context.Context, attempt runner.AttemptRequest) (runner.AttemptOutputSink, error) {
	h.before++
	return runner.DiscardOutputSink(), nil
}

func (h *recordingHooks) AfterAttempt(ctx context.Context, attempt runner.AttemptRequest, outcome runner.AttemptOutcome, sink runner.DraftSink) (runner.AttemptOutcome, error) {
	h.after++
	if err := sink.Emit(ctx, runner.EventDraft{Source: model.ObservationSourceHostObserved, Kind: model.ObservationKindArtifactActivity, Artifact: &model.ArtifactObservation{Operation: "post-run-collect", ArtifactID: "artifact-" + attempt.AttemptID, Path: "/workspace/out.txt", Digest: model.Digest("sha256:" + strings.Repeat("a", 64)), SizeBytes: 2, Executable: false, SourceEventIDs: []string{}}}); err != nil {
		return runner.AttemptOutcome{}, err
	}
	return outcome, nil
}

func (h *recordingHooks) AbortAttempt(context.Context, runner.AttemptRequest, error) error {
	h.abort++
	return nil
}
