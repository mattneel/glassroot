package githubapp

type PRDecisionAction string

const (
	PRDecisionSchedule    PRDecisionAction = "schedule"
	PRDecisionCancel      PRDecisionAction = "cancel"
	PRDecisionNoop        PRDecisionAction = "noop"
	PRDecisionUnavailable PRDecisionAction = "unavailable"
)

type PRGenerationState struct {
	InstallationID    int64  `json:"installationId"`
	RepositoryID      int64  `json:"repositoryId"`
	PullRequestNumber int64  `json:"pullRequestNumber"`
	Generation        int64  `json:"generation"`
	CurrentTargetID   string `json:"currentTargetId"`
	Cancelled         bool   `json:"cancelled"`
}

type CurrentPullRequestState struct {
	InstallationID    int64
	RepositoryID      int64
	PullRequestNumber int64
	BaseRepositoryID  int64
	HeadRepositoryID  int64
	BaseSHA           string
	HeadSHA           string
	Draft             bool
	Closed            bool
	HeadInaccessible  bool
}

type PRDecision struct {
	Action             PRDecisionAction `json:"action"`
	Generation         int64            `json:"generation"`
	TargetID           string           `json:"targetId,omitempty"`
	SupersededTargetID string           `json:"supersededTargetId,omitempty"`
	Reason             string           `json:"reason"`
}

func ReconcilePullRequestGeneration(existing PRGenerationState, projection PullRequestProjection, current CurrentPullRequestState, profile string) (PRDecision, PRGenerationState, error) {
	if projection.InstallationID <= 0 || current.InstallationID != projection.InstallationID {
		return PRDecision{}, existing, errCode(CodeInvalidInstallationID, "controller", "installation mismatch", nil)
	}
	if projection.RepositoryID <= 0 || current.RepositoryID != projection.RepositoryID {
		return PRDecision{}, existing, errCode(CodeInvalidRepositoryID, "controller", "repository mismatch", nil)
	}
	if projection.PullRequestNumber <= 0 || current.PullRequestNumber != projection.PullRequestNumber {
		return PRDecision{}, existing, errCode(CodeInvalidPullRequestNumber, "controller", "pull request mismatch", nil)
	}
	if existing.Generation > 0 && (existing.InstallationID != projection.InstallationID || existing.RepositoryID != projection.RepositoryID || existing.PullRequestNumber != projection.PullRequestNumber) {
		return PRDecision{}, existing, errCode(CodeInvalidTarget, "controller", "generation key mismatch", nil)
	}
	state := existing
	state.InstallationID = projection.InstallationID
	state.RepositoryID = projection.RepositoryID
	state.PullRequestNumber = projection.PullRequestNumber
	if current.Closed || current.Draft || projection.Action == "closed" || projection.Action == "converted_to_draft" {
		state.Cancelled = true
		return PRDecision{Action: PRDecisionCancel, Generation: state.Generation, TargetID: state.CurrentTargetID, Reason: "pull request closed or draft"}, state, nil
	}
	if current.HeadInaccessible {
		return PRDecision{Action: PRDecisionUnavailable, Generation: state.Generation, Reason: "head commit inaccessible"}, state, nil
	}
	target := AnalysisTarget{SchemaVersion: SchemaGitHubAnalysisTargetV1Alpha1, InstallationID: current.InstallationID, BaseRepositoryID: current.BaseRepositoryID, HeadRepositoryID: current.HeadRepositoryID, PullRequestNumber: current.PullRequestNumber, BaseCommitID: current.BaseSHA, HeadCommitID: current.HeadSHA, AnalysisProfileVersion: profile}
	targetID, err := target.ID()
	if err != nil {
		return PRDecision{}, existing, err
	}
	if state.CurrentTargetID == targetID && !state.Cancelled {
		return PRDecision{Action: PRDecisionNoop, Generation: state.Generation, TargetID: targetID, Reason: "target already current"}, state, nil
	}
	old := state.CurrentTargetID
	state.Generation++
	state.CurrentTargetID = targetID
	state.Cancelled = false
	return PRDecision{Action: PRDecisionSchedule, Generation: state.Generation, TargetID: targetID, SupersededTargetID: old, Reason: "current PR state selected immutable target"}, state, nil
}

type CheckRunMapping struct {
	CheckRunID     int64
	GitHubAppID    int64
	InstallationID int64
	RepositoryID   int64
	ExternalID     string
	TargetID       string
	Generation     int64
}

type RerequestDecision struct {
	Accepted      bool
	Historical    bool
	TargetID      string
	Generation    int64
	AttemptReason AttemptReason
}

func DecideCheckRunRerequest(projection CheckRunProjection, mapping CheckRunMapping, configuredAppID, currentGeneration int64) (RerequestDecision, error) {
	if projection.Action != "rerequested" {
		return RerequestDecision{}, errCode(CodeUnsupportedAction, "rerequest", "only check_run rerequested is supported", nil)
	}
	if mapping.CheckRunID <= 0 || mapping.CheckRunID != projection.CheckRunID || mapping.GitHubAppID != configuredAppID || projection.AppID != configuredAppID || mapping.InstallationID != projection.InstallationID || mapping.RepositoryID != projection.RepositoryID || mapping.ExternalID == "" || mapping.ExternalID != projection.ExternalID || !validateTargetID(mapping.TargetID) {
		return RerequestDecision{}, errCode(CodeForeignCheckRun, "rerequest", "check run mapping is unknown or foreign", nil)
	}
	return RerequestDecision{Accepted: true, Historical: mapping.Generation < currentGeneration, TargetID: mapping.TargetID, Generation: mapping.Generation, AttemptReason: AttemptReasonCheckRerequest}, nil
}
