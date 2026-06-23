package githubapp

import "fmt"

const (
	DomainAnalysisTargetID  = "glassroot.dev/github-analysis-target-id/v1\x00"
	DomainAnalysisJobID     = "glassroot.dev/github-analysis-job-id/v1\x00"
	DomainAnalysisAttemptID = "glassroot.dev/github-analysis-attempt-id/v1\x00"
	DomainCheckExternalID   = "glassroot.dev/github-check-external-id/v1\x00"
)

type AnalysisTarget struct {
	SchemaVersion          string `json:"schemaVersion"`
	InstallationID         int64  `json:"installationId"`
	BaseRepositoryID       int64  `json:"baseRepositoryId"`
	HeadRepositoryID       int64  `json:"headRepositoryId"`
	PullRequestNumber      int64  `json:"pullRequestNumber"`
	BaseCommitID           string `json:"baseCommitId"`
	HeadCommitID           string `json:"headCommitId"`
	AnalysisProfileVersion string `json:"analysisProfileVersion"`
}

func (t AnalysisTarget) Validate() error {
	if t.SchemaVersion != SchemaGitHubAnalysisTargetV1Alpha1 {
		return errCode(CodeInvalidTarget, "target", "schema version invalid", nil)
	}
	if t.InstallationID <= 0 || t.BaseRepositoryID <= 0 || t.HeadRepositoryID <= 0 || t.PullRequestNumber <= 0 {
		return errCode(CodeInvalidTarget, "target", "numeric identities must be positive", nil)
	}
	if !validGitObjectID(t.BaseCommitID) || !validGitObjectID(t.HeadCommitID) {
		return errCode(CodeInvalidObjectID, "target", "commit ids must be full lowercase object ids", nil)
	}
	if t.AnalysisProfileVersion == "" || hasControl(t.AnalysisProfileVersion) {
		return errCode(CodeInvalidTarget, "target", "analysis profile version invalid", nil)
	}
	return nil
}

func (t AnalysisTarget) ID() (string, error) {
	if err := t.Validate(); err != nil {
		return "", err
	}
	return prefixedID("target", DomainAnalysisTargetID,
		fmt.Sprintf("%d", t.InstallationID),
		fmt.Sprintf("%d", t.BaseRepositoryID),
		fmt.Sprintf("%d", t.HeadRepositoryID),
		fmt.Sprintf("%d", t.PullRequestNumber),
		t.BaseCommitID,
		t.HeadCommitID,
		t.AnalysisProfileVersion,
	), nil
}

func validateTargetID(id string) bool {
	return len(id) == len("target-")+64 && id[:len("target-")] == "target-" && isLowerHex(id[len("target-"):], 64)
}
