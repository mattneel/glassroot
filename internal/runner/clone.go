package runner

import "github.com/mattneel/glassroot/internal/model"

func cloneAttemptRequest(in AttemptRequest) AttemptRequest {
	out := in
	out.Environment = cloneEnv(in.Environment)
	out.NetworkPolicy.Allowed = cloneNetworkAllowed(in.NetworkPolicy.Allowed)
	out.Collection.FilesystemRoots = cloneStrings(in.Collection.FilesystemRoots)
	out.Collection.Artifacts = cloneArtifacts(in.Collection.Artifacts)
	return out
}

func cloneAttemptRequests(in []AttemptRequest) []AttemptRequest {
	out := make([]AttemptRequest, len(in))
	for i := range in {
		out[i] = cloneAttemptRequest(in[i])
	}
	return out
}

func cloneCapabilities(in model.RunnerCapabilities) model.RunnerCapabilities { return in }

func cloneRequirements(in Requirements) Requirements {
	out := in
	out.AllowedIsolationTiers = append([]model.IsolationTier(nil), in.AllowedIsolationTiers...)
	return out
}

func cloneOutcome(in AttemptOutcome) AttemptOutcome {
	out := in
	if in.ExitCode != nil {
		v := *in.ExitCode
		out.ExitCode = &v
	}
	out.Limitations = cloneLimitations(in.Limitations)
	return out
}

func cloneExecutionResult(in ExecutionResult) ExecutionResult {
	out := in
	out.Runner = cloneCapabilities(in.Runner)
	out.Attempts = make([]AttemptResult, len(in.Attempts))
	for i := range in.Attempts {
		out.Attempts[i] = in.Attempts[i]
		out.Attempts[i].Outcome = cloneOutcome(in.Attempts[i].Outcome)
		out.Attempts[i].Limitations = cloneLimitations(in.Attempts[i].Limitations)
	}
	out.Limitations = cloneLimitations(in.Limitations)
	return out
}

func cloneEventDraft(in EventDraft) EventDraft {
	out := in
	out.Process = cloneProcess(in.Process)
	out.Filesystem = cloneFilesystem(in.Filesystem)
	out.Network = cloneNetwork(in.Network)
	out.Artifact = cloneArtifact(in.Artifact)
	out.Scenario = cloneScenario(in.Scenario)
	out.ObserverWarning = cloneObserverWarning(in.ObserverWarning)
	out.ResourceLimit = cloneResourceLimit(in.ResourceLimit)
	return out
}

func cloneObservationEvent(in model.ObservationEvent) model.ObservationEvent {
	out := in
	out.Process = cloneProcess(in.Process)
	out.Filesystem = cloneFilesystem(in.Filesystem)
	out.Network = cloneNetwork(in.Network)
	out.Artifact = cloneArtifact(in.Artifact)
	out.Scenario = cloneScenario(in.Scenario)
	out.ObserverWarning = cloneObserverWarning(in.ObserverWarning)
	out.ResourceLimit = cloneResourceLimit(in.ResourceLimit)
	return out
}

func cloneProcess(in *model.ProcessObservation) *model.ProcessObservation {
	if in == nil {
		return nil
	}
	out := *in
	out.Arguments = cloneStrings(in.Arguments)
	out.Environment = cloneEnv(in.Environment)
	if in.ParentProcessID != nil {
		v := *in.ParentProcessID
		out.ParentProcessID = &v
	}
	if in.ExitCode != nil {
		v := *in.ExitCode
		out.ExitCode = &v
	}
	if in.StartedAt != nil {
		v := *in.StartedAt
		out.StartedAt = &v
	}
	if in.ExitedAt != nil {
		v := *in.ExitedAt
		out.ExitedAt = &v
	}
	return &out
}

func cloneFilesystem(in *model.FilesystemObservation) *model.FilesystemObservation {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneNetwork(in *model.NetworkObservation) *model.NetworkObservation {
	if in == nil {
		return nil
	}
	out := *in
	out.ResolvedAddresses = cloneStrings(in.ResolvedAddresses)
	return &out
}

func cloneArtifact(in *model.ArtifactObservation) *model.ArtifactObservation {
	if in == nil {
		return nil
	}
	out := *in
	out.SourceEventIDs = cloneStrings(in.SourceEventIDs)
	return &out
}

func cloneScenario(in *model.ScenarioObservation) *model.ScenarioObservation {
	if in == nil {
		return nil
	}
	out := *in
	if in.StartedAt != nil {
		v := *in.StartedAt
		out.StartedAt = &v
	}
	if in.CompletedAt != nil {
		v := *in.CompletedAt
		out.CompletedAt = &v
	}
	return &out
}

func cloneObserverWarning(in *model.ObserverWarningObservation) *model.ObserverWarningObservation {
	if in == nil {
		return nil
	}
	out := *in
	out.Limitations = cloneLimitations(in.Limitations)
	return &out
}

func cloneResourceLimit(in *model.ResourceLimitObservation) *model.ResourceLimitObservation {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneLimitations(in []model.Limitation) []model.Limitation {
	if in == nil {
		return nil
	}
	out := make([]model.Limitation, len(in))
	copy(out, in)
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneEnv(in []model.EnvEntry) []model.EnvEntry {
	if in == nil {
		return nil
	}
	out := make([]model.EnvEntry, len(in))
	copy(out, in)
	return out
}

func cloneArtifacts(in []model.ExpectedArtifactSpec) []model.ExpectedArtifactSpec {
	if in == nil {
		return nil
	}
	out := make([]model.ExpectedArtifactSpec, len(in))
	copy(out, in)
	return out
}

func cloneNetworkAllowed(in []model.NetworkAllowRule) []model.NetworkAllowRule {
	if in == nil {
		return nil
	}
	out := make([]model.NetworkAllowRule, len(in))
	copy(out, in)
	return out
}
