package githubapp

import "strings"

type WorkerAssignment struct {
	SchemaVersion              string     `json:"schemaVersion"`
	AttemptID                  string     `json:"attemptId"`
	TargetID                   string     `json:"targetId"`
	SourceStoreID              string     `json:"sourceStoreId"`
	BaseCommitID               string     `json:"baseCommitId"`
	HeadCommitID               string     `json:"headCommitId"`
	PlanDigest                 string     `json:"planDigest"`
	RequiredRunnerTier         RunnerTier `json:"requiredRunnerTier"`
	EvidenceOutputCapabilityID string     `json:"evidenceOutputCapabilityId"`
	ControllerGeneration       int64      `json:"controllerGeneration"`
	Limitations                []string   `json:"limitations"`
}

type WorkerResult struct {
	SchemaVersion           string     `json:"schemaVersion"`
	AttemptID               string     `json:"attemptId"`
	JobID                   string     `json:"jobId"`
	TargetID                string     `json:"targetId"`
	Generation              int64      `json:"generation"`
	CompletionState         string     `json:"completionState"`
	ReportDigest            string     `json:"reportDigest"`
	ManifestDigest          string     `json:"manifestDigest"`
	PolicyApplicationDigest string     `json:"policyApplicationDigest"`
	EffectiveDisposition    string     `json:"effectiveDisposition"`
	RunnerTier              RunnerTier `json:"runnerTier"`
	Limitations             []string   `json:"limitations"`
}

func ValidateWorkerAssignment(a WorkerAssignment) error {
	if a.SchemaVersion != SchemaGitHubWorkerAssignmentV1Alpha1 || !validateAttemptID(a.AttemptID) || !validateTargetID(a.TargetID) || a.ControllerGeneration <= 0 {
		return errCode(CodeInvalidWorkerAssignment, "worker", "assignment identity invalid", nil)
	}
	if a.RequiredRunnerTier != RunnerTierHardenedContainer && a.RequiredRunnerTier != RunnerTierMicroVM {
		return errCode(CodeInvalidWorkerAssignment, "worker", "public PR worker assignment requires hardened-container or microvm", nil)
	}
	if !validGitObjectID(a.BaseCommitID) || !validGitObjectID(a.HeadCommitID) || !validateDigest(a.PlanDigest) {
		return errCode(CodeInvalidWorkerAssignment, "worker", "assignment source or plan invalid", nil)
	}
	if invalidCredentialLike(a.SourceStoreID) || invalidCredentialLike(a.EvidenceOutputCapabilityID) {
		return errCode(CodeCredentialBoundaryViolation, "worker", "credential-like field rejected", nil)
	}
	if a.Limitations == nil {
		return errCode(CodeInvalidWorkerAssignment, "worker", "limitations must be non-null", nil)
	}
	return nil
}

func invalidCredentialLike(s string) bool {
	if s == "" || hasControl(s) {
		return true
	}
	lower := strings.ToLower(s)
	for _, marker := range []string{"token", "secret", "private", "ghs_", "github.com", "https://", "http://"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
