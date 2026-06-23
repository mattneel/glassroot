package fake

import (
	"time"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

func cloneProgram(in Program) Program {
	out := in
	out.Attempts = make([]AttemptScript, len(in.Attempts))
	for i := range in.Attempts {
		out.Attempts[i] = cloneAttemptScript(in.Attempts[i])
	}
	return out
}

func cloneAttemptScript(in AttemptScript) AttemptScript {
	out := in
	out.Events = make([]SyntheticEvent, len(in.Events))
	for i := range in.Events {
		out.Events[i] = SyntheticEvent{OffsetMillis: in.Events[i].OffsetMillis, Draft: cloneDraft(in.Events[i].Draft)}
	}
	out.Outcome = cloneOutcome(in.Outcome)
	return out
}

func cloneOutcome(in runner.AttemptOutcome) runner.AttemptOutcome {
	out := in
	if in.ExitCode != nil {
		v := *in.ExitCode
		out.ExitCode = &v
	}
	out.Limitations = cloneLimitations(in.Limitations)
	return out
}

func cloneDraft(in runner.EventDraft) runner.EventDraft {
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
	out.StartedAt = cloneTime(in.StartedAt)
	out.ExitedAt = cloneTime(in.ExitedAt)
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
	out.StartedAt = cloneTime(in.StartedAt)
	out.CompletedAt = cloneTime(in.CompletedAt)
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

func cloneTime(in *time.Time) *time.Time {
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
