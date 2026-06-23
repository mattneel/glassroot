package githubapp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mattneel/glassroot/internal/model"
)

type CheckStatus string

type CheckConclusion string

const (
	CheckStatusQueued     CheckStatus = "queued"
	CheckStatusInProgress CheckStatus = "in_progress"
	CheckStatusCompleted  CheckStatus = "completed"

	CheckConclusionNeutral   CheckConclusion = "neutral"
	CheckConclusionCancelled CheckConclusion = "cancelled"
)

type FindingCounts struct {
	Total          int64 `json:"total"`
	Passed         int64 `json:"passed"`
	RequiresReview int64 `json:"requiresReview"`
	Failed         int64 `json:"failed"`
}

type CheckOutput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Text    string `json:"text"`
}

type CheckProjection struct {
	SchemaVersion    string          `json:"schemaVersion"`
	Name             string          `json:"name"`
	Status           CheckStatus     `json:"status"`
	Conclusion       CheckConclusion `json:"conclusion,omitempty"`
	HeadSHA          string          `json:"headSha"`
	ExternalID       string          `json:"externalId"`
	Output           CheckOutput     `json:"output"`
	DetailsURL       string          `json:"detailsUrl,omitempty"`
	Annotations      []string        `json:"annotations"`
	RequestedActions []string        `json:"requestedActions"`
}

type CheckProjectionInput struct {
	RepositoryID          int64
	HeadSHA               string
	AttemptID             string
	TargetID              string
	Generation            int64
	PolicyDisposition     model.Disposition
	EvidenceComplete      bool
	RunnerTier            RunnerTier
	FindingCounts         FindingCounts
	Status                CheckStatus
	SupersededOrCancelled bool
	Limitations           []string
}

func ProjectAdvisoryCheck(in CheckProjectionInput) (CheckProjection, error) {
	if in.RepositoryID <= 0 || !validGitObjectID(in.HeadSHA) || !validateAttemptID(in.AttemptID) || !validateTargetID(in.TargetID) || in.Generation <= 0 {
		return CheckProjection{}, errCode(CodeInvalidCheckProjection, "check", "check projection identity invalid", nil)
	}
	if in.RunnerTier == "" || hasControl(string(in.RunnerTier)) {
		return CheckProjection{}, errCode(CodeInvalidCheckProjection, "check", "runner tier invalid", nil)
	}
	if in.Status != CheckStatusQueued && in.Status != CheckStatusInProgress && in.Status != CheckStatusCompleted {
		return CheckProjection{}, errCode(CodeInvalidCheckProjection, "check", "status invalid", nil)
	}
	conclusion := CheckConclusion("")
	if in.Status == CheckStatusCompleted {
		if in.SupersededOrCancelled {
			conclusion = CheckConclusionCancelled
		} else {
			conclusion = CheckConclusionNeutral
		}
	}
	summary := safeCheckSummary(in)
	text := "Glassroot advisory Check Runs are non-blocking. A neutral conclusion preserves the Glassroot policy disposition separately and is not a safety proof or merge approval. Public PR execution remains gated on a reviewed hardened runner."
	projection := CheckProjection{SchemaVersion: SchemaGitHubCheckProjectionV1Alpha1, Name: "Glassroot advisory", Status: in.Status, Conclusion: conclusion, HeadSHA: in.HeadSHA, ExternalID: "gr-" + domainHash(DomainCheckExternalID, fmt.Sprintf("%d", in.RepositoryID), in.AttemptID, CheckProfileAdvisoryV1Alpha1), Output: CheckOutput{Title: "Glassroot advisory result", Summary: summary, Text: text}, Annotations: []string{}, RequestedActions: []string{}}
	if len(projection.Output.Summary) > AbsoluteMaxCheckSummaryBytes || len(projection.Output.Text) > AbsoluteMaxCheckTextBytes || len(projection.Output.Title) > AbsoluteMaxCheckTitleBytes {
		return CheckProjection{}, errCode(CodeInvalidCheckProjection, "check", "check output exceeds bounds", nil)
	}
	return projection, nil
}

func safeCheckSummary(in CheckProjectionInput) string {
	limitations := append([]string(nil), in.Limitations...)
	sort.Strings(limitations)
	if limitations == nil {
		limitations = []string{}
	}
	return fmt.Sprintf("policy_disposition=%s\nevidence_complete=%t\nrunner_tier=%s\nfinding_total=%d\nfinding_failed=%d\nlimitations=%d\nneutral conclusion is advisory and not a safety proof", in.PolicyDisposition, in.EvidenceComplete, in.RunnerTier, in.FindingCounts.Total, in.FindingCounts.Failed, len(limitations))
}

func (p CheckProjection) Digest() string {
	body, err := json.Marshal(p)
	if err != nil {
		return "sha256:" + strings.Repeat("0", 64)
	}
	return DigestRawBody(body)
}

type PublishCommand struct {
	SchemaVersion string          `json:"schemaVersion"`
	RepositoryID  int64           `json:"repositoryId"`
	HeadSHA       string          `json:"headSha"`
	AttemptID     string          `json:"attemptId"`
	TargetID      string          `json:"targetId"`
	Generation    int64           `json:"generation"`
	Projection    CheckProjection `json:"projection"`
}

type PublisherReceipt struct {
	SchemaVersion    string `json:"schemaVersion"`
	RepositoryID     int64  `json:"repositoryId"`
	AttemptID        string `json:"attemptId"`
	CheckRunID       int64  `json:"checkRunId"`
	ExternalID       string `json:"externalId"`
	ProjectionDigest string `json:"projectionDigest"`
}

func ValidatePublishCommand(cmd PublishCommand, currentGeneration int64) error {
	if cmd.SchemaVersion != SchemaGitHubPublishCommandV1Alpha1 || cmd.RepositoryID <= 0 || !validGitObjectID(cmd.HeadSHA) || !validateAttemptID(cmd.AttemptID) || !validateTargetID(cmd.TargetID) || cmd.Generation <= 0 {
		return errCode(CodePublishInvariant, "publisher", "publish command identity invalid", nil)
	}
	if cmd.Generation < currentGeneration {
		return errCode(CodeStaleGeneration, "publisher", "stale generation cannot publish", nil)
	}
	if cmd.Projection.SchemaVersion != SchemaGitHubCheckProjectionV1Alpha1 || cmd.Projection.Name != "Glassroot advisory" || len(cmd.Projection.Annotations) != 0 || len(cmd.Projection.RequestedActions) != 0 || cmd.Projection.DetailsURL != "" {
		return errCode(CodeInvalidCheckProjection, "publisher", "projection features invalid", nil)
	}
	if cmd.Projection.Conclusion != "" && cmd.Projection.Conclusion != CheckConclusionNeutral && cmd.Projection.Conclusion != CheckConclusionCancelled {
		return errCode(CodeInvalidCheckConclusion, "publisher", "unsupported conclusion", nil)
	}
	return nil
}
