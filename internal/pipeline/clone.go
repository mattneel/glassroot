package pipeline

import "github.com/mattneel/glassroot/internal/model"

func cloneRunPlan(in model.RunPlan) model.RunPlan {
	out := in
	out.Base = cloneCommitRef(in.Base)
	out.Head = cloneCommitRef(in.Head)
	if in.Configuration != nil {
		v := *in.Configuration
		out.Configuration = &v
	}
	if in.ExecutionEnvironment != nil {
		v := *in.ExecutionEnvironment
		out.ExecutionEnvironment = &v
	}
	out.Revisions = cloneRevisionPlans(in.Revisions)
	out.Scenarios = cloneScenarioPlans(in.Scenarios)
	out.NetworkPolicy.Allowed = cloneNetworkAllowRules(in.NetworkPolicy.Allowed)
	out.Environment = cloneEnvEntries(in.Environment)
	if in.Collection != nil {
		v := *in.Collection
		v.FilesystemRoots = cloneStrings(in.Collection.FilesystemRoots)
		v.Artifacts = cloneExpectedArtifacts(in.Collection.Artifacts)
		out.Collection = &v
	}
	if in.Comparison != nil {
		v := *in.Comparison
		v.IgnoreFields = cloneStrings(in.Comparison.IgnoreFields)
		out.Comparison = &v
	}
	if in.Policy != nil {
		v := *in.Policy
		out.Policy = &v
	}
	if in.Platform != nil {
		v := *in.Platform
		out.Platform = &v
	}
	out.Limitations = cloneLimitations(in.Limitations)
	return out
}

func cloneCommitRef(in model.CommitRef) model.CommitRef { return in }

func cloneRevisionPlans(in []model.RevisionPlan) []model.RevisionPlan {
	out := make([]model.RevisionPlan, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Commit = cloneCommitRef(in[i].Commit)
		out[i].ScenarioIDs = cloneStrings(in[i].ScenarioIDs)
		if in[i].SourceSummary != nil {
			v := *in[i].SourceSummary
			out[i].SourceSummary = &v
		}
		out[i].SourceLimitations = cloneModelSourceLimitations(in[i].SourceLimitations)
	}
	return out
}

func cloneScenarioPlans(in []model.ScenarioPlan) []model.ScenarioPlan {
	out := make([]model.ScenarioPlan, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Command.Argv = cloneStrings(in[i].Command.Argv)
		out[i].Command.Environment = cloneEnvEntries(in[i].Command.Environment)
		out[i].NetworkPolicy.Allowed = cloneNetworkAllowRules(in[i].NetworkPolicy.Allowed)
		out[i].ExpectedArtifacts = cloneExpectedArtifacts(in[i].ExpectedArtifacts)
	}
	return out
}

func cloneSourceSnapshot(in SourceSnapshot) SourceSnapshot {
	out := in
	out.Limitations = cloneSourceLimitations(in.Limitations)
	return out
}

func cloneSourceLimitations(in []SourceLimitation) []SourceLimitation {
	if in == nil {
		return nil
	}
	out := make([]SourceLimitation, len(in))
	copy(out, in)
	return out
}

func cloneModelSourceLimitations(in []model.SourceLimitation) []model.SourceLimitation {
	if in == nil {
		return nil
	}
	out := make([]model.SourceLimitation, len(in))
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

func cloneEnvEntries(in []model.EnvEntry) []model.EnvEntry {
	if in == nil {
		return nil
	}
	out := make([]model.EnvEntry, len(in))
	copy(out, in)
	return out
}

func cloneExpectedArtifacts(in []model.ExpectedArtifactSpec) []model.ExpectedArtifactSpec {
	if in == nil {
		return nil
	}
	out := make([]model.ExpectedArtifactSpec, len(in))
	copy(out, in)
	return out
}

func cloneNetworkAllowRules(in []model.NetworkAllowRule) []model.NetworkAllowRule {
	if in == nil {
		return nil
	}
	out := make([]model.NetworkAllowRule, len(in))
	copy(out, in)
	return out
}

func cloneLimitations(in []model.Limitation) []model.Limitation {
	if in == nil {
		return nil
	}
	out := make([]model.Limitation, len(in))
	copy(out, in)
	return out
}
