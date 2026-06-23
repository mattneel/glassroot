package githubapp

import "fmt"

type RunnerTier string

const (
	RunnerTierHardenedContainer RunnerTier = "hardened-container"
	RunnerTierDevelopmentOnly   RunnerTier = "development-only"
	RunnerTierDockerDev         RunnerTier = "docker-dev"
	RunnerTierFake              RunnerTier = "fake"
)

type AttemptReason string

const (
	AttemptReasonInitial             AttemptReason = "initial"
	AttemptReasonInfrastructureRetry AttemptReason = "infrastructure-retry"
	AttemptReasonCheckRerequest      AttemptReason = "check-rerequest"
)

type AnalysisJob struct {
	SchemaVersion          string         `json:"schemaVersion"`
	ID                     string         `json:"id"`
	TargetID               string         `json:"targetId"`
	Target                 AnalysisTarget `json:"target"`
	Generation             int64          `json:"generation"`
	AnalysisProfileVersion string         `json:"analysisProfileVersion"`
	RequiredRunnerTier     RunnerTier     `json:"requiredRunnerTier"`
}

type AnalysisAttempt struct {
	SchemaVersion string        `json:"schemaVersion"`
	ID            string        `json:"id"`
	JobID         string        `json:"jobId"`
	TargetID      string        `json:"targetId"`
	Generation    int64         `json:"generation"`
	AttemptNumber int64         `json:"attemptNumber"`
	Reason        AttemptReason `json:"reason"`
}

func NewAnalysisJob(target AnalysisTarget, generation int64, profile string, tier RunnerTier) (AnalysisJob, error) {
	if tier != RunnerTierHardenedContainer {
		return AnalysisJob{}, errCode(CodeInvalidJob, "job", "public PR jobs require hardened-container or stronger", nil)
	}
	if generation <= 0 || generation > AbsoluteMaxControllerGeneration || profile == "" || hasControl(profile) {
		return AnalysisJob{}, errCode(CodeInvalidJob, "job", "generation or profile invalid", nil)
	}
	targetID, err := target.ID()
	if err != nil {
		return AnalysisJob{}, err
	}
	id := prefixedID("job", DomainAnalysisJobID, targetID, fmt.Sprintf("%d", generation), profile, string(tier))
	return AnalysisJob{SchemaVersion: SchemaGitHubAnalysisJobV1Alpha1, ID: id, TargetID: targetID, Target: target, Generation: generation, AnalysisProfileVersion: profile, RequiredRunnerTier: tier}, nil
}

func NewAnalysisAttempt(job AnalysisJob, number int64, reason AttemptReason) (AnalysisAttempt, error) {
	if !validateJobID(job.ID) || !validateTargetID(job.TargetID) || job.Generation <= 0 || number <= 0 || number > AbsoluteMaxAttemptsPerTarget {
		return AnalysisAttempt{}, errCode(CodeInvalidAttempt, "attempt", "job or attempt identity invalid", nil)
	}
	if reason != AttemptReasonInitial && reason != AttemptReasonInfrastructureRetry && reason != AttemptReasonCheckRerequest {
		return AnalysisAttempt{}, errCode(CodeInvalidAttempt, "attempt", "attempt reason invalid", nil)
	}
	id := prefixedID("attempt", DomainAnalysisAttemptID, job.ID, job.TargetID, fmt.Sprintf("%d", job.Generation), fmt.Sprintf("%d", number), string(reason))
	return AnalysisAttempt{SchemaVersion: SchemaGitHubAnalysisAttemptV1Alpha1, ID: id, JobID: job.ID, TargetID: job.TargetID, Generation: job.Generation, AttemptNumber: number, Reason: reason}, nil
}

func validateJobID(id string) bool {
	return len(id) == len("job-")+64 && id[:len("job-")] == "job-" && isLowerHex(id[len("job-"):], 64)
}
func validateAttemptID(id string) bool {
	return len(id) == len("attempt-")+64 && id[:len("attempt-")] == "attempt-" && isLowerHex(id[len("attempt-"):], 64)
}

func CheckExternalID(appIdentity string, repositoryID int64, attemptID, profile string) (string, error) {
	if appIdentity == "" || hasControl(appIdentity) || repositoryID <= 0 || !validateAttemptID(attemptID) || profile == "" || hasControl(profile) {
		return "", errCode(CodePublishInvariant, "publisher", "external id inputs invalid", nil)
	}
	return "gr-" + domainHash(DomainCheckExternalID, appIdentity, fmt.Sprintf("%d", repositoryID), attemptID, profile), nil
}
