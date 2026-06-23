package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
)

type executionState struct {
	limits Limits
	sink   EventSink
	seq    uint64
	result ExecutionResult
}

func ExecutePlan(ctx context.Context, plan *pipeline.FrozenPlan, backend Runner, requirements Requirements, limits Limits, sink EventSink) (ExecutionResult, error) {
	if err := contextError(ctx, CodeContextCancelled, "execute", "", "context"); err != nil {
		return ExecutionResult{}, err
	}
	if plan == nil {
		return ExecutionResult{}, errCode(CodeInvalidPlan, "plan", "", "plan", "FrozenPlan is required", nil)
	}
	if backend == nil {
		return ExecutionResult{}, errCode(CodeBackendFailed, "backend", "", "runner", "runner backend is required", nil)
	}
	if sink == nil {
		return ExecutionResult{}, errCode(CodeSinkFailed, "sink", "", "sink", "event sink is required", nil)
	}
	limits, err := validateLimits(limits)
	if err != nil {
		return ExecutionResult{}, err
	}
	requirements, err = validateRequirements(requirements)
	if err != nil {
		return ExecutionResult{}, err
	}
	doc := plan.Document()
	if err := validatePlanDocument(doc); err != nil {
		return ExecutionResult{}, err
	}
	attempts, err := expandAttempts(doc, plan.Digest())
	if err != nil {
		return ExecutionResult{}, err
	}
	if int64(len(attempts)) > limits.MaxAttempts {
		return ExecutionResult{}, errCode(CodeInvalidPlan, "attempts", "", "count", "attempt count exceeds caller limit", nil)
	}

	caps, err := backend.Capabilities(ctx)
	if err != nil {
		return ExecutionResult{}, errCode(CodeCapabilitiesFailed, "capabilities", "", "runner", "capability query failed", err)
	}
	caps = cloneCapabilities(caps)
	if requirements.Intent == ExecutionIntentWorkload && caps.SyntheticEvidence && !caps.ExecutesTargetCode {
		return ExecutionResult{}, errCode(CodeSyntheticRunnerNotAllowed, "capabilities", "", "intent", "synthetic runner cannot satisfy workload execution", nil)
	}
	mismatches, err := MatchCapabilities(requirements, caps)
	if err != nil {
		return ExecutionResult{}, err
	}
	if int64(len(mismatches)) > limits.MaxCapabilityMismatches {
		return ExecutionResult{}, errCode(CodeCapabilityMismatch, "capabilities", "", "mismatches", "too many capability mismatches", nil)
	}
	if len(mismatches) > 0 {
		return ExecutionResult{}, errCode(CodeCapabilityMismatch, "capabilities", "", string(mismatches[0].Code), fmt.Sprintf("required %s actual %s", mismatches[0].Required, mismatches[0].Actual), nil)
	}
	if planAware, ok := backend.(PlanAwareRunner); ok {
		if err := planAware.ValidatePlan(ctx, plan.Digest(), cloneAttemptRequests(attempts), limits); err != nil {
			return ExecutionResult{}, err
		}
	}

	execCtx := ctx
	var cancel context.CancelFunc
	if doc.ResourceLimits.TimeoutMillis > 0 {
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(doc.ResourceLimits.TimeoutMillis)*time.Millisecond)
		defer cancel()
	}
	state := &executionState{limits: limits, sink: sink, result: ExecutionResult{PlanDigest: plan.Digest(), Runner: caps, Attempts: []AttemptResult{}, Limitations: syntheticLimitations(caps)}}
	for _, attempt := range attempts {
		if err := contextError(execCtx, CodeRunTimeout, "execute", "", attempt.AttemptID); err != nil {
			return cloneExecutionResult(state.result), err
		}
		attemptCtx := execCtx
		var attemptCancel context.CancelFunc
		if attempt.ScenarioTimeoutMillis > 0 {
			attemptCtx, attemptCancel = context.WithTimeout(execCtx, time.Duration(attempt.ScenarioTimeoutMillis)*time.Millisecond)
		}
		result, err := executeAttempt(attemptCtx, state, backend, attempt)
		if attemptCancel != nil {
			attemptCancel()
		}
		if err != nil {
			return cloneExecutionResult(state.result), err
		}
		state.result.Attempts = append(state.result.Attempts, result)
	}
	state.result.TotalEmittedEvents = state.seq
	state.result.Complete = true
	return cloneExecutionResult(state.result), nil
}

func executeAttempt(ctx context.Context, state *executionState, backend Runner, attempt AttemptRequest) (AttemptResult, error) {
	if err := contextError(ctx, CodeAttemptTimeout, "attempt", attempt.AttemptID, "context"); err != nil {
		return AttemptResult{}, err
	}
	attempt = cloneAttemptRequest(attempt)
	sink := &attemptDraftSink{state: state, attempt: attempt}
	outcome, err := backend.RunAttempt(ctx, attempt, sink)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return AttemptResult{}, errCode(CodeContextCancelled, "attempt", attempt.AttemptID, "context", "context cancelled", err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return AttemptResult{}, errCode(CodeAttemptTimeout, "attempt", attempt.AttemptID, "context", "attempt timeout", err)
		}
		var rerr *Error
		if errors.As(err, &rerr) {
			return AttemptResult{}, err
		}
		return AttemptResult{}, errCode(CodeBackendFailed, "attempt", attempt.AttemptID, "runner", "backend failed", err)
	}
	outcome = cloneOutcome(outcome)
	if err := validateOutcome(outcome, attempt.ScenarioTimeoutMillis); err != nil {
		return AttemptResult{}, err
	}
	return AttemptResult{
		AttemptID:             attempt.AttemptID,
		Revision:              attempt.Revision,
		ScenarioID:            attempt.ScenarioID,
		Repetition:            attempt.Repetition,
		Outcome:               outcome,
		FirstAcceptedSequence: sink.firstSeq,
		LastAcceptedSequence:  sink.lastSeq,
		AcceptedEventCount:    sink.count,
		Limitations:           cloneLimitations(outcome.Limitations),
	}, nil
}

type attemptDraftSink struct {
	state    *executionState
	attempt  AttemptRequest
	count    uint64
	firstSeq uint64
	lastSeq  uint64
}

func (s *attemptDraftSink) Emit(ctx context.Context, draft EventDraft) error {
	if err := contextError(ctx, CodeAttemptTimeout, "event", s.attempt.AttemptID, "context"); err != nil {
		return err
	}
	if s.count >= uint64(s.state.limits.MaxEventsPerAttempt) || s.state.seq >= uint64(s.state.limits.MaxEventsPerExecution) {
		return errCode(CodeEventLimit, "event", s.attempt.AttemptID, "count", "event limit exceeded", nil)
	}
	if err := ValidateEventDraft(draft, s.state.limits); err != nil {
		return err
	}
	nextSeq := s.state.seq + 1
	event := envelopeEvent(s.attempt, nextSeq, draft)
	size, err := eventJSONSize(event)
	if err != nil {
		return errCode(CodeSerializationFailed, "event", s.attempt.AttemptID, "json", "marshal observation event", err)
	}
	if int64(size) > s.state.limits.MaxEventJSONBytes {
		return errCode(CodeEventTooLarge, "event", s.attempt.AttemptID, "json", "observation event exceeds size limit", nil)
	}
	if err := s.state.sink.Emit(ctx, cloneObservationEvent(event)); err != nil {
		return errCode(CodeSinkFailed, "sink", s.attempt.AttemptID, "emit", "event sink failed", err)
	}
	s.state.seq = nextSeq
	s.state.result.TotalEmittedEvents = s.state.seq
	s.count++
	if s.firstSeq == 0 {
		s.firstSeq = nextSeq
	}
	s.lastSeq = nextSeq
	return nil
}

func syntheticLimitations(caps model.RunnerCapabilities) []model.Limitation {
	if !caps.SyntheticEvidence {
		return []model.Limitation{}
	}
	return []model.Limitation{
		{ID: "synthetic-no-target-execution", Summary: "No target code was executed by this runner."},
		{ID: "synthetic-observations", Summary: "All observations are synthetic test data, not observed repository behavior."},
	}
}

func contextError(ctx context.Context, deadlineCode ErrorCode, stage, attempt, path string) error {
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return errCode(deadlineCode, stage, attempt, path, "context deadline exceeded", err)
		}
		return errCode(CodeContextCancelled, stage, attempt, path, "context cancelled", err)
	}
	return nil
}
