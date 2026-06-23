package runner

import (
	"fmt"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
)

func expandAttempts(doc model.RunPlan, planDigest model.Digest) ([]AttemptRequest, error) {
	if err := validatePlanDocument(doc); err != nil {
		return nil, err
	}
	attempts := make([]AttemptRequest, 0)
	var ordinal uint64
	for _, revision := range doc.Revisions {
		for _, scenario := range doc.Scenarios {
			for rep := int64(1); rep <= scenario.Repetitions; rep++ {
				ordinal++
				if ordinal > MaxAttempts {
					return nil, errCode(CodeInvalidPlan, "attempts", "", "count", "attempt count exceeds absolute limit", nil)
				}
				id := fmt.Sprintf("att-%s-%s-r%d", revision.Kind, scenario.ID, rep)
				if len(id) > MaxAttemptIDBytes {
					return nil, errCode(CodeInvalidAttempt, "attempts", id, "attemptId", "attempt id exceeds limit", nil)
				}
				attempts = append(attempts, AttemptRequest{
					PlanDigest:                    planDigest,
					RunID:                         doc.RunID,
					PlanCreatedAt:                 doc.CreatedAt,
					AttemptID:                     id,
					GlobalOrdinal:                 ordinal,
					Revision:                      revision.Kind,
					CommitID:                      revision.Commit.CommitID,
					TreeID:                        revision.TreeID,
					ObjectFormat:                  revision.ObjectFormat,
					MaterializedTreeDigest:        revision.MaterializedTreeDigest,
					MaterializationManifestDigest: revision.MaterializationManifestDigest,
					Image:                         doc.ExecutionEnvironment.Image,
					Workdir:                       doc.ExecutionEnvironment.Workdir,
					Environment:                   cloneEnv(doc.Environment),
					ResourceLimits:                scenario.ResourceLimits,
					NetworkPolicy:                 model.NetworkPolicy{Mode: scenario.NetworkPolicy.Mode, Allowed: cloneNetworkAllowed(scenario.NetworkPolicy.Allowed)},
					ScenarioID:                    scenario.ID,
					ScenarioName:                  scenario.Name,
					Shell:                         scenario.Shell,
					Run:                           scenario.Run,
					ScenarioTimeoutMillis:         scenario.ResourceLimits.TimeoutMillis,
					Repetition:                    uint32(rep),
					Collection:                    collectionFromPlan(doc.Collection),
				})
			}
		}
	}
	return attempts, nil
}

func collectionFromPlan(in *model.CollectionPlan) CollectionSettings {
	if in == nil {
		return CollectionSettings{FilesystemRoots: []string{}, Artifacts: []model.ExpectedArtifactSpec{}}
	}
	return CollectionSettings{
		FilesystemRoots:      cloneStrings(in.FilesystemRoots),
		FilesystemContents:   in.FilesystemContents,
		Artifacts:            cloneArtifacts(in.Artifacts),
		LogMaxBytesPerStream: in.LogMaxBytesPerStream,
	}
}

func validatePlanDocument(doc model.RunPlan) error {
	if doc.SchemaVersion != model.SchemaVersionRunPlanV1Alpha1 || doc.RunID == "" || doc.CreatedAt.IsZero() {
		return errCode(CodeInvalidPlan, "plan", "", "identity", "run plan identity is invalid", nil)
	}
	if !isZeroRunnerCapabilities(doc.Runner) {
		return errCode(CodeInvalidPlanRunnerField, "plan", "", "runner", "legacy run-plan runner field must remain non-authoritative and zero", nil)
	}
	if doc.ExecutionEnvironment == nil || doc.ExecutionEnvironment.Image == "" || doc.ExecutionEnvironment.Workdir == "" {
		return errCode(CodeInvalidPlan, "plan", "", "executionEnvironment", "execution environment is incomplete", nil)
	}
	if len(doc.Revisions) != 2 || doc.Revisions[0].Kind != model.RevisionKindBase || doc.Revisions[1].Kind != model.RevisionKindHead {
		return errCode(CodeInvalidPlan, "plan", "", "revisions", "revisions must be base then head", nil)
	}
	if len(doc.Scenarios) == 0 {
		return errCode(CodeInvalidPlan, "plan", "", "scenarios", "at least one scenario is required", nil)
	}
	var attempts int64
	for i, rev := range doc.Revisions {
		if rev.Commit.CommitID == "" || rev.TreeID == "" || rev.ObjectFormat == "" || rev.MaterializedTreeDigest == "" || rev.MaterializationManifestDigest == "" {
			return errCode(CodeInvalidPlan, "plan", "", fmt.Sprintf("revisions[%d]", i), "revision provenance is incomplete", nil)
		}
	}
	for i, scenario := range doc.Scenarios {
		if scenario.ID == "" || scenario.Shell == "" || scenario.Run == "" || scenario.Repetitions <= 0 {
			return errCode(CodeInvalidPlan, "plan", "", fmt.Sprintf("scenarios[%d]", i), "scenario execution fields are incomplete", nil)
		}
		attempts += scenario.Repetitions * int64(len(doc.Revisions))
		if attempts > MaxAttempts {
			return errCode(CodeInvalidPlan, "plan", "", "scenarios", "attempt count exceeds limit", nil)
		}
	}
	return nil
}

func ValidatePlanDocumentForTest(doc model.RunPlan) error { return validatePlanDocument(doc) }

func isZeroRunnerCapabilities(c model.RunnerCapabilities) bool {
	return c == (model.RunnerCapabilities{})
}

// ExpandPlanAttempts returns the deterministic attempt inventory for a frozen
// plan without executing it. The returned requests are owned copies. Evidence
// writers use this to route events without reconstructing execution semantics.

// ExpandPlanDocument returns the deterministic attempt inventory for an already
// verified run-plan document. The caller supplies the independently recomputed
// plan digest; this helper does not construct or trust a FrozenPlan. Returned
// requests are owned copies.
func ExpandPlanDocument(doc model.RunPlan, planDigest model.Digest) ([]AttemptRequest, error) {
	if err := validatePlanDocument(doc); err != nil {
		return nil, err
	}
	attempts, err := expandAttempts(doc, planDigest)
	if err != nil {
		return nil, err
	}
	return cloneAttemptRequests(attempts), nil
}

func ExpandPlanAttempts(plan *pipeline.FrozenPlan) ([]AttemptRequest, error) {
	if plan == nil {
		return nil, errCode(CodeInvalidPlan, "plan", "", "plan", "FrozenPlan is required", nil)
	}
	doc := plan.Document()
	if err := validatePlanDocument(doc); err != nil {
		return nil, err
	}
	attempts, err := expandAttempts(doc, plan.Digest())
	if err != nil {
		return nil, err
	}
	return cloneAttemptRequests(attempts), nil
}
