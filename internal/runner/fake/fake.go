package fake

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

type Program struct {
	PlanDigest model.Digest
	Attempts   []AttemptScript
}

type AttemptScript struct {
	Revision   model.RevisionKind
	ScenarioID string
	Repetition uint32
	Events     []SyntheticEvent
	Outcome    runner.AttemptOutcome
}

type SyntheticEvent struct {
	OffsetMillis int64
	Draft        runner.EventDraft
}

type Runner struct {
	program Program
	scripts map[string]AttemptScript
}

func Capabilities() model.RunnerCapabilities {
	return model.RunnerCapabilities{
		Name:                      "fake",
		Version:                   "v1",
		IsolationTier:             model.IsolationTierFake,
		FreshKernel:               false,
		BrokeredNetwork:           false,
		ExecutesTargetCode:        false,
		SyntheticEvidence:         true,
		EnforcesNetworkDeny:       false,
		ProcessEventCollection:    false,
		FilesystemEventCollection: false,
		SyscallEventCollection:    false,
		ArtifactHashing:           false,
		SnapshotSupport:           false,
	}
}

func New(program Program) (*Runner, error) {
	program = cloneProgram(program)
	if err := validateProgramShape(program); err != nil {
		return nil, err
	}
	scripts := make(map[string]AttemptScript, len(program.Attempts))
	for _, script := range program.Attempts {
		k := key(script.Revision, script.ScenarioID, script.Repetition)
		scripts[k] = cloneAttemptScript(script)
	}
	return &Runner{program: program, scripts: scripts}, nil
}

func (r *Runner) Capabilities(ctx context.Context) (model.RunnerCapabilities, error) {
	if err := ctx.Err(); err != nil {
		return model.RunnerCapabilities{}, err
	}
	return Capabilities(), nil
}

func (r *Runner) ValidatePlan(ctx context.Context, planDigest model.Digest, attempts []runner.AttemptRequest, limits runner.Limits) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil {
		return runner.ErrInvalidProgram
	}
	if r.program.PlanDigest != planDigest {
		return runnerError(runner.CodeProgramPlanMismatch, "program", "planDigest", "program is bound to a different plan", nil)
	}
	seenRequired := make(map[string]struct{}, len(attempts))
	for _, attempt := range attempts {
		seenRequired[key(attempt.Revision, attempt.ScenarioID, attempt.Repetition)] = struct{}{}
	}
	seenScripts := make(map[string]int, len(r.program.Attempts))
	for i, script := range r.program.Attempts {
		k := key(script.Revision, script.ScenarioID, script.Repetition)
		seenScripts[k]++
		if seenScripts[k] > 1 {
			return runnerError(runner.CodeDuplicateAttemptScript, "program", k, "duplicate attempt script", nil)
		}
		if _, ok := seenRequired[k]; !ok {
			return runnerError(runner.CodeExtraAttemptScript, "program", k, fmt.Sprintf("extra attempt script at index %d", i), nil)
		}
		if int64(len(script.Events)) > limits.MaxEventsPerAttempt-2 {
			return runnerError(runner.CodeInvalidProgram, "program", k, "too many synthetic events", nil)
		}
		if err := validateScript(script, limits); err != nil {
			return err
		}
	}
	missing := make([]string, 0)
	for k := range seenRequired {
		if _, ok := seenScripts[k]; !ok {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return runnerError(runner.CodeMissingAttemptScript, "program", missing[0], "missing attempt script", nil)
	}
	return nil
}

func (r *Runner) RunAttempt(ctx context.Context, request runner.AttemptRequest, sink runner.DraftSink) (runner.AttemptOutcome, error) {
	if err := ctx.Err(); err != nil {
		return runner.AttemptOutcome{}, err
	}
	if r == nil || sink == nil {
		return runner.AttemptOutcome{}, runnerError(runner.CodeInvalidProgram, "fake", request.AttemptID, "fake runner is not initialized", nil)
	}
	script, ok := r.scripts[key(request.Revision, request.ScenarioID, request.Repetition)]
	if !ok {
		return runner.AttemptOutcome{}, runnerError(runner.CodeMissingAttemptScript, "fake", request.AttemptID, "missing attempt script", nil)
	}
	script = cloneAttemptScript(script)
	if err := validateScript(script, runner.DefaultLimits()); err != nil {
		return runner.AttemptOutcome{}, err
	}
	if err := validateOutcomeForRequest(script.Outcome, request); err != nil {
		return runner.AttemptOutcome{}, err
	}
	startAt := runnerSyntheticTime(request, 0)
	if err := sink.Emit(ctx, runner.EventDraft{ObservedAt: startAt, Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindScenarioStarted, Scenario: &model.ScenarioObservation{Status: model.ScenarioStatusRunning, StartedAt: &startAt, DurationMillis: 0}}); err != nil {
		return runner.AttemptOutcome{}, err
	}
	for _, event := range script.Events {
		if err := ctx.Err(); err != nil {
			return runner.AttemptOutcome{}, err
		}
		draft := cloneDraft(event.Draft)
		draft.Source = model.ObservationSourceSyntheticTestGenerated
		draft.ObservedAt = runnerSyntheticTime(request, event.OffsetMillis)
		if err := sink.Emit(ctx, draft); err != nil {
			return runner.AttemptOutcome{}, err
		}
	}
	completedAt := runnerSyntheticTime(request, script.Outcome.DurationMillis)
	status := scenarioStatus(script.Outcome.Status)
	if err := sink.Emit(ctx, runner.EventDraft{ObservedAt: completedAt, Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindScenarioCompleted, Scenario: &model.ScenarioObservation{Status: status, CompletedAt: &completedAt, DurationMillis: script.Outcome.DurationMillis}}); err != nil {
		return runner.AttemptOutcome{}, err
	}
	return cloneOutcome(script.Outcome), nil
}

func (r *Runner) ProgramForTest() Program {
	if r == nil {
		return Program{}
	}
	return cloneProgram(r.program)
}

func validateProgramShape(program Program) error {
	if !validDigest(program.PlanDigest) {
		return runnerError(runner.CodeInvalidProgram, "program", "planDigest", "program digest must be sha256 lowercase hex", nil)
	}
	if len(program.Attempts) == 0 || len(program.Attempts) > runner.MaxAttempts {
		return runnerError(runner.CodeInvalidProgram, "program", "attempts", "program must contain a bounded attempt script set", nil)
	}
	seen := make(map[string]struct{}, len(program.Attempts))
	for _, script := range program.Attempts {
		k := key(script.Revision, script.ScenarioID, script.Repetition)
		if _, ok := seen[k]; ok {
			return runnerError(runner.CodeDuplicateAttemptScript, "program", k, "duplicate attempt script", nil)
		}
		seen[k] = struct{}{}
		if script.Revision != model.RevisionKindBase && script.Revision != model.RevisionKindHead {
			return runnerError(runner.CodeInvalidProgram, "program", k, "invalid revision kind", nil)
		}
		if invalidScenarioID(script.ScenarioID) || script.Repetition == 0 {
			return runnerError(runner.CodeInvalidProgram, "program", k, "invalid attempt key", nil)
		}
		if len(script.Events) > runner.MaxEventsPerAttempt-2 {
			return runnerError(runner.CodeInvalidProgram, "program", k, "too many synthetic events", nil)
		}
	}
	return nil
}

func validateScript(script AttemptScript, limits runner.Limits) error {
	if err := validateOutcomeForTimeout(script.Outcome, 0); err != nil {
		return err
	}
	for _, event := range script.Events {
		if event.OffsetMillis < 0 {
			return runnerError(runner.CodeInvalidProgram, "program", key(script.Revision, script.ScenarioID, script.Repetition), "negative synthetic offset", nil)
		}
		if event.Draft.Source != model.ObservationSourceSyntheticTestGenerated {
			return runnerError(runner.CodeInvalidProgram, "program", key(script.Revision, script.ScenarioID, script.Repetition), "fake events must use synthetic provenance", nil)
		}
		draft := cloneDraft(event.Draft)
		if err := runner.ValidateEventDraft(draft, limits); err != nil {
			return err
		}
		if draft.Kind == model.ObservationKindScenarioStarted || draft.Kind == model.ObservationKindScenarioCompleted {
			return runnerError(runner.CodeInvalidProgram, "program", key(script.Revision, script.ScenarioID, script.Repetition), "program cannot supply scenario lifecycle events", nil)
		}
	}
	return nil
}

func validateOutcomeForRequest(outcome runner.AttemptOutcome, request runner.AttemptRequest) error {
	return validateOutcomeForTimeout(outcome, request.ScenarioTimeoutMillis)
}

func validateOutcomeForTimeout(outcome runner.AttemptOutcome, timeoutMillis int64) error {
	if outcome.DurationMillis < 0 || (timeoutMillis > 0 && outcome.DurationMillis > timeoutMillis) {
		return runnerError(runner.CodeInvalidOutcome, "fake", "durationMillis", "invalid fake outcome duration", nil)
	}
	switch outcome.Status {
	case runner.AttemptStatusSucceeded:
		if outcome.ExitCode == nil || *outcome.ExitCode != 0 {
			return runnerError(runner.CodeInvalidOutcome, "fake", "exitCode", "succeeded outcome requires exit code 0", nil)
		}
	case runner.AttemptStatusFailed:
		if outcome.ExitCode == nil || *outcome.ExitCode == 0 {
			return runnerError(runner.CodeInvalidOutcome, "fake", "exitCode", "failed outcome requires nonzero exit code", nil)
		}
	case runner.AttemptStatusTimedOut, runner.AttemptStatusResourceLimited:
		if outcome.ExitCode != nil && *outcome.ExitCode == 0 {
			return runnerError(runner.CodeInvalidOutcome, "fake", "exitCode", "non-success outcome cannot use exit code 0", nil)
		}
	default:
		return runnerError(runner.CodeInvalidOutcome, "fake", "status", "unknown fake outcome status", nil)
	}
	return nil
}

func scenarioStatus(status runner.AttemptStatus) model.ScenarioStatus {
	switch status {
	case runner.AttemptStatusSucceeded:
		return model.ScenarioStatusPassed
	case runner.AttemptStatusFailed:
		return model.ScenarioStatusFailed
	case runner.AttemptStatusTimedOut:
		return model.ScenarioStatusTimedOut
	case runner.AttemptStatusResourceLimited:
		return model.ScenarioStatusIncomplete
	default:
		return model.ScenarioStatusError
	}
}

func key(revision model.RevisionKind, scenarioID string, repetition uint32) string {
	return string(revision) + "\x00" + scenarioID + "\x00" + fmt.Sprintf("%d", repetition)
}

func invalidScenarioID(id string) bool {
	if id == "" || len(id) > 64 || !utf8.ValidString(id) || strings.ContainsAny(id, "\x00\r\n\t ") {
		return true
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return true
	}
	return false
}

func validDigest(d model.Digest) bool {
	s := string(d)
	if !strings.HasPrefix(s, "sha256:") || len(s) != len("sha256:")+64 || !utf8.ValidString(s) {
		return false
	}
	for _, r := range strings.TrimPrefix(s, "sha256:") {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func runnerError(code runner.ErrorCode, stage, path, msg string, err error) error {
	return &runner.Error{Code: code, Stage: stage, Path: path, Err: wrapMessage(msg, err)}
}

func wrapMessage(msg string, err error) error {
	if err == nil {
		return fmt.Errorf("%s", msg)
	}
	return fmt.Errorf("%s: %w", msg, err)
}
