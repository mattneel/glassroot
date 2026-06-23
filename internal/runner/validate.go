package runner

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/model"
)

func ValidateRunnerCapabilities(caps model.RunnerCapabilities) error {
	if invalidBoundedText(caps.Name, MaxRunnerNameBytes, false) {
		return errCode(CodeInvalidCapabilities, "capabilities", "", "name", "runner name is required and bounded", nil)
	}
	if invalidBoundedText(caps.Version, MaxRunnerVersionBytes, false) {
		return errCode(CodeInvalidCapabilities, "capabilities", "", "version", "runner version is required and bounded", nil)
	}
	if !knownIsolationTier(caps.IsolationTier) {
		return errCode(CodeInvalidCapabilities, "capabilities", "", "isolationTier", "unknown isolation tier", nil)
	}
	if caps.IsolationTier == model.IsolationTierFake && caps.ExecutesTargetCode {
		return errCode(CodeInvalidCapabilities, "capabilities", "", "executesTargetCode", "fake isolation tier cannot execute target code", nil)
	}
	if caps.IsolationTier == model.IsolationTierFake && !caps.SyntheticEvidence {
		return errCode(CodeInvalidCapabilities, "capabilities", "", "syntheticEvidence", "fake isolation tier must identify synthetic evidence", nil)
	}
	return nil
}

func validateRequirements(req Requirements) (Requirements, error) {
	out := cloneRequirements(req)
	if out.Intent != ExecutionIntentSyntheticTest && out.Intent != ExecutionIntentWorkload {
		return Requirements{}, errCode(CodeInvalidRequirements, "requirements", "", "intent", "execution intent is required", nil)
	}
	if len(out.AllowedIsolationTiers) == 0 {
		return Requirements{}, errCode(CodeInvalidRequirements, "requirements", "", "allowedIsolationTiers", "allowed isolation tiers must be explicit", nil)
	}
	for i, tier := range out.AllowedIsolationTiers {
		if !knownIsolationTier(tier) {
			return Requirements{}, errCode(CodeInvalidRequirements, "requirements", "", fmt.Sprintf("allowedIsolationTiers[%d]", i), "unknown isolation tier", nil)
		}
	}
	if out.Intent == ExecutionIntentSyntheticTest {
		if out.TargetExecutionRequired || !out.SyntheticEvidenceAllowed {
			return Requirements{}, errCode(CodeInvalidRequirements, "requirements", "", "synthetic-test", "synthetic test intent must not require target execution and must explicitly allow synthetic evidence", nil)
		}
	}
	if out.Intent == ExecutionIntentWorkload && !out.TargetExecutionRequired {
		return Requirements{}, errCode(CodeInvalidRequirements, "requirements", "", "targetExecutionRequired", "workload intent requires target execution", nil)
	}
	return out, nil
}

func validateLimits(limits Limits) (Limits, error) {
	checks := []struct {
		name string
		got  int64
		max  int64
	}{
		{"maxAttempts", limits.MaxAttempts, MaxAttempts},
		{"maxEventsPerAttempt", limits.MaxEventsPerAttempt, MaxEventsPerAttempt},
		{"maxEventsPerExecution", limits.MaxEventsPerExecution, MaxEventsPerExecution},
		{"maxEventJsonBytes", limits.MaxEventJSONBytes, MaxEventJSONBytes},
		{"maxCapabilityMismatches", limits.MaxCapabilityMismatches, MaxCapabilityMismatches},
		{"maxResultLimitations", limits.MaxResultLimitations, MaxResultLimitations},
	}
	for _, check := range checks {
		if check.got <= 0 || check.got > check.max {
			return Limits{}, errCode(CodeInvalidLimits, "limits", "", check.name, fmt.Sprintf("limit must be positive and no greater than %d", check.max), nil)
		}
	}
	return limits, nil
}

func MatchCapabilities(req Requirements, caps model.RunnerCapabilities) ([]CapabilityMismatch, error) {
	req, err := validateRequirements(req)
	if err != nil {
		return nil, err
	}
	if err := ValidateRunnerCapabilities(caps); err != nil {
		return nil, err
	}
	mismatches := make([]CapabilityMismatch, 0)
	add := func(code CapabilityMismatchCode, required, actual string) {
		if len(mismatches) < MaxCapabilityMismatches {
			mismatches = append(mismatches, CapabilityMismatch{Code: code, Required: required, Actual: actual})
		}
	}
	if req.Intent == ExecutionIntentWorkload {
		if !caps.ExecutesTargetCode || caps.SyntheticEvidence {
			add(MismatchExecutionIntent, "workload target execution", capabilityBool(caps.ExecutesTargetCode)+"/synthetic="+capabilityBool(caps.SyntheticEvidence))
		}
	}
	if req.Intent == ExecutionIntentSyntheticTest {
		if caps.ExecutesTargetCode || !caps.SyntheticEvidence || !req.SyntheticEvidenceAllowed {
			add(MismatchExecutionIntent, "synthetic-test without target execution", capabilityBool(caps.ExecutesTargetCode)+"/synthetic="+capabilityBool(caps.SyntheticEvidence))
		}
	}
	if !tierAllowed(caps.IsolationTier, req.AllowedIsolationTiers) {
		add(MismatchIsolationTier, tiersString(req.AllowedIsolationTiers), string(caps.IsolationTier))
	}
	if req.TargetExecutionRequired && !caps.ExecutesTargetCode {
		add(MismatchTargetExecution, "true", "false")
	}
	if req.SyntheticEvidenceAllowed && !caps.SyntheticEvidence && req.Intent == ExecutionIntentSyntheticTest {
		add(MismatchSyntheticEvidence, "true", "false")
	}
	if req.NetworkDenyEnforcementRequired && !caps.EnforcesNetworkDeny {
		add(MismatchNetworkDenyEnforcement, "true", "false")
	}
	if req.BrokeredNetworkRequired && !caps.BrokeredNetwork {
		add(MismatchBrokeredNetwork, "true", "false")
	}
	if req.ProcessEventsRequired && !caps.ProcessEventCollection {
		add(MismatchProcessEvents, "true", "false")
	}
	if req.FilesystemEventsRequired && !caps.FilesystemEventCollection {
		add(MismatchFilesystemEvents, "true", "false")
	}
	if req.SyscallEventsRequired && !caps.SyscallEventCollection {
		add(MismatchSyscallEvents, "true", "false")
	}
	if req.ArtifactHashingRequired && !caps.ArtifactHashing {
		add(MismatchArtifactHashing, "true", "false")
	}
	if req.SnapshotSupportRequired && !caps.SnapshotSupport {
		add(MismatchSnapshotSupport, "true", "false")
	}
	if req.FreshKernelRequired && !caps.FreshKernel {
		add(MismatchFreshKernel, "true", "false")
	}
	return mismatches, nil
}

func ValidateEventDraft(draft EventDraft, limits Limits) error {
	limits, err := validateLimits(limits)
	if err != nil {
		return err
	}
	if draft.ObservedAt.IsZero() {
		// Backends may leave the time zero only before a test double assigns it.
	}
	if !knownObservationSource(draft.Source) {
		return errCode(CodeInvalidEventDraft, "event", "", "source", "unknown observation source", nil)
	}
	payloads := 0
	if draft.Process != nil {
		payloads++
	}
	if draft.Filesystem != nil {
		payloads++
	}
	if draft.Network != nil {
		payloads++
	}
	if draft.Artifact != nil {
		payloads++
	}
	if draft.Scenario != nil {
		payloads++
	}
	if draft.ObserverWarning != nil {
		payloads++
	}
	if draft.ResourceLimit != nil {
		payloads++
	}
	if payloads != 1 {
		return errCode(CodeInvalidEventDraft, "event", "", "payload", "exactly one typed payload is required", nil)
	}
	if !payloadMatchesKind(draft) {
		return errCode(CodeInvalidEventDraft, "event", "", "kind", "observation kind does not match payload", nil)
	}
	if estimateDraftSize(draft) > limits.MaxEventJSONBytes {
		return errCode(CodeEventTooLarge, "event", "", "payload", "event draft exceeds size limit", nil)
	}
	return nil
}

func validateOutcome(outcome AttemptOutcome, timeoutMillis int64) error {
	if outcome.DurationMillis < 0 || (timeoutMillis > 0 && outcome.DurationMillis > timeoutMillis) {
		return errCode(CodeInvalidOutcome, "attempt", "", "durationMillis", "outcome duration is invalid", nil)
	}
	if len(outcome.Limitations) > MaxResultLimitations {
		return errCode(CodeInvalidOutcome, "attempt", "", "limitations", "too many outcome limitations", nil)
	}
	for _, lim := range outcome.Limitations {
		if invalidBoundedText(lim.ID, MaxLimitationCodeBytes, false) || invalidBoundedText(lim.Summary, MaxLimitationMessageBytes, false) || containsControl(lim.Details) {
			return errCode(CodeInvalidOutcome, "attempt", "", "limitations", "outcome limitation is invalid", nil)
		}
	}
	switch outcome.Status {
	case AttemptStatusSucceeded:
		if outcome.ExitCode == nil || *outcome.ExitCode != 0 {
			return errCode(CodeInvalidOutcome, "attempt", "", "exitCode", "succeeded outcome requires exit code 0", nil)
		}
	case AttemptStatusFailed:
		if outcome.ExitCode == nil || *outcome.ExitCode == 0 {
			return errCode(CodeInvalidOutcome, "attempt", "", "exitCode", "failed outcome requires nonzero exit code", nil)
		}
	case AttemptStatusTimedOut, AttemptStatusResourceLimited:
		if outcome.ExitCode != nil && *outcome.ExitCode == 0 {
			return errCode(CodeInvalidOutcome, "attempt", "", "exitCode", "non-success outcome cannot report exit code 0", nil)
		}
	default:
		return errCode(CodeInvalidOutcome, "attempt", "", "status", "unknown target outcome status", nil)
	}
	return nil
}

func knownIsolationTier(tier model.IsolationTier) bool {
	switch tier {
	case model.IsolationTierFake, model.IsolationTierDevelopmentOnly, model.IsolationTierHardenedContainer, model.IsolationTierMicroVM:
		return true
	default:
		return false
	}
}

func knownObservationSource(source model.ObservationSource) bool {
	switch source {
	case model.ObservationSourceHostObserved, model.ObservationSourceNetworkBrokerObserved, model.ObservationSourceSandboxRuntimeObserved, model.ObservationSourceGuestAgentReported, model.ObservationSourceWorkloadReported, model.ObservationSourceStaticAnalysisDerived, model.ObservationSourceModelInferred, model.ObservationSourceSyntheticTestGenerated:
		return true
	default:
		return false
	}
}

func payloadMatchesKind(draft EventDraft) bool {
	switch draft.Kind {
	case model.ObservationKindProcessStart, model.ObservationKindProcessExit:
		return draft.Process != nil
	case model.ObservationKindFilesystemCreate, model.ObservationKindFilesystemRead, model.ObservationKindFilesystemWrite, model.ObservationKindFilesystemDelete, model.ObservationKindFilesystemRename, model.ObservationKindFilesystemChmod:
		return draft.Filesystem != nil
	case model.ObservationKindDNSQuery, model.ObservationKindNetworkConnection:
		return draft.Network != nil
	case model.ObservationKindArtifactActivity:
		return draft.Artifact != nil
	case model.ObservationKindScenarioStarted, model.ObservationKindScenarioCompleted:
		return draft.Scenario != nil
	case model.ObservationKindObserverWarning, model.ObservationKindUnsupportedObservation:
		return draft.ObserverWarning != nil
	case model.ObservationKindResourceLimit:
		return draft.ResourceLimit != nil
	default:
		return false
	}
}

func tierAllowed(tier model.IsolationTier, allowed []model.IsolationTier) bool {
	for _, item := range allowed {
		if item == tier {
			return true
		}
	}
	return false
}

func tiersString(tiers []model.IsolationTier) string {
	parts := make([]string, len(tiers))
	for i, tier := range tiers {
		parts[i] = string(tier)
	}
	return strings.Join(parts, ",")
}

func capabilityBool(v bool) string { return strconv.FormatBool(v) }

func invalidBoundedText(s string, maxBytes int, allowEmpty bool) bool {
	if (!allowEmpty && s == "") || len(s) > maxBytes || !utf8.ValidString(s) || containsControl(s) {
		return true
	}
	return false
}

func containsControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func estimateDraftSize(draft EventDraft) int64 {
	size := len(string(draft.Source)) + len(string(draft.Kind)) + 64
	if draft.Process != nil {
		size += len(draft.Process.Operation) + len(draft.Process.ExecutablePath)
		for _, arg := range draft.Process.Arguments {
			size += len(arg)
		}
		for _, env := range draft.Process.Environment {
			size += len(env.Name) + len(env.Value)
		}
	}
	if draft.Filesystem != nil {
		size += len(draft.Filesystem.Operation) + len(draft.Filesystem.Path) + len(draft.Filesystem.OldPath) + len(draft.Filesystem.Mode) + len(draft.Filesystem.Digest)
	}
	if draft.Network != nil {
		size += len(draft.Network.Operation) + len(draft.Network.Protocol) + len(draft.Network.QueryName) + len(draft.Network.DestinationHost) + len(draft.Network.Result)
		for _, a := range draft.Network.ResolvedAddresses {
			size += len(a)
		}
	}
	if draft.Artifact != nil {
		size += len(draft.Artifact.Operation) + len(draft.Artifact.ArtifactID) + len(draft.Artifact.Path) + len(draft.Artifact.Digest)
		for _, id := range draft.Artifact.SourceEventIDs {
			size += len(id)
		}
	}
	if draft.Scenario != nil {
		size += len(draft.Scenario.Status) + len(draft.Scenario.Message)
	}
	if draft.ObserverWarning != nil {
		size += len(draft.ObserverWarning.Code) + len(draft.ObserverWarning.Message)
		for _, l := range draft.ObserverWarning.Limitations {
			size += len(l.ID) + len(l.Summary) + len(l.Details)
		}
	}
	if draft.ResourceLimit != nil {
		size += len(draft.ResourceLimit.LimitKind) + len(draft.ResourceLimit.Unit)
	}
	return int64(size)
}
