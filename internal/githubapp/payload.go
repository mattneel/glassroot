package githubapp

import (
	"bytes"
	"encoding/json"
)

type ProjectionKind string

const (
	ProjectionPullRequest              ProjectionKind = "pull_request"
	ProjectionCheckRun                 ProjectionKind = "check_run"
	ProjectionCheckSuite               ProjectionKind = "check_suite"
	ProjectionInstallation             ProjectionKind = "installation"
	ProjectionInstallationRepositories ProjectionKind = "installation_repositories"
	ProjectionPing                     ProjectionKind = "ping"
	ProjectionIgnored                  ProjectionKind = "ignored"
)

type WebhookProjection struct {
	Kind         ProjectionKind          `json:"kind"`
	PullRequest  *PullRequestProjection  `json:"pullRequest,omitempty"`
	CheckRun     *CheckRunProjection     `json:"checkRun,omitempty"`
	Installation *InstallationProjection `json:"installation,omitempty"`
}

type PullRequestProjection struct {
	Action            string `json:"action"`
	InstallationID    int64  `json:"installationId"`
	RepositoryID      int64  `json:"repositoryId"`
	RepositoryOwnerID int64  `json:"repositoryOwnerId,omitempty"`
	PullRequestNumber int64  `json:"pullRequestNumber"`
	BaseSHA           string `json:"baseSha"`
	HeadSHA           string `json:"headSha"`
	HeadRepositoryID  int64  `json:"headRepositoryId,omitempty"`
	Draft             bool   `json:"draft"`
	Closed            bool   `json:"closed"`
	Merged            bool   `json:"merged"`
}

type CheckRunProjection struct {
	Action         string `json:"action"`
	InstallationID int64  `json:"installationId"`
	RepositoryID   int64  `json:"repositoryId"`
	CheckRunID     int64  `json:"checkRunId"`
	HeadSHA        string `json:"headSha"`
	AppID          int64  `json:"appId"`
	ExternalID     string `json:"externalId"`
}

type InstallationProjection struct {
	Action         string  `json:"action"`
	InstallationID int64   `json:"installationId"`
	RepositoryIDs  []int64 `json:"repositoryIds"`
}

func ProjectWebhook(event string, body []byte, limits Limits) (WebhookProjection, error) {
	var out WebhookProjection
	if err := PreflightGitHubWebhookJSON(body, limits); err != nil {
		return out, err
	}
	decision, err := ClassifyWebhookAction(event, extractAction(body))
	if err != nil && event != "issue_comment" {
		return out, err
	}
	_ = decision
	dec := json.NewDecoder(bytes.NewReader(body))
	switch event {
	case "pull_request":
		var p pullRequestPayload
		if err := dec.Decode(&p); err != nil {
			return out, errCode(CodeProjectionInvalid, "projection", "pull request payload invalid", err)
		}
		pr, err := projectPullRequest(p, limits)
		if err != nil {
			return out, err
		}
		return WebhookProjection{Kind: ProjectionPullRequest, PullRequest: &pr}, nil
	case "check_run":
		var p checkRunPayload
		if err := dec.Decode(&p); err != nil {
			return out, errCode(CodeProjectionInvalid, "projection", "check run payload invalid", err)
		}
		cr, err := projectCheckRun(p, limits)
		if err != nil {
			return out, err
		}
		if cr.Action != "rerequested" {
			return WebhookProjection{Kind: ProjectionIgnored}, nil
		}
		return WebhookProjection{Kind: ProjectionCheckRun, CheckRun: &cr}, nil
	case "check_suite":
		return WebhookProjection{Kind: ProjectionCheckSuite}, nil
	case "installation", "installation_repositories":
		var p installationPayload
		if err := dec.Decode(&p); err != nil {
			return out, errCode(CodeProjectionInvalid, "projection", "installation payload invalid", err)
		}
		inst, err := projectInstallation(p)
		if err != nil {
			return out, err
		}
		return WebhookProjection{Kind: ProjectionInstallation, Installation: &inst}, nil
	case "ping":
		return WebhookProjection{Kind: ProjectionPing}, nil
	default:
		return out, errCode(CodeUnsupportedEvent, "projection", "event is not supported by advisory profile", nil)
	}
}

func extractAction(body []byte) string {
	var p struct {
		Action string `json:"action"`
	}
	_ = json.Unmarshal(body, &p)
	return p.Action
}

type pullRequestPayload struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository struct {
		ID    int64 `json:"id"`
		Owner struct {
			ID int64 `json:"id"`
		} `json:"owner"`
	} `json:"repository"`
	PullRequest struct {
		Number int64  `json:"number"`
		Draft  bool   `json:"draft"`
		Merged bool   `json:"merged"`
		State  string `json:"state"`
		Base   struct {
			SHA  string `json:"sha"`
			Repo struct {
				ID int64 `json:"id"`
			} `json:"repo"`
		} `json:"base"`
		Head struct {
			SHA  string `json:"sha"`
			Repo struct {
				ID int64 `json:"id"`
			} `json:"repo"`
		} `json:"head"`
	} `json:"pull_request"`
}

type checkRunPayload struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	CheckRun struct {
		ID         int64  `json:"id"`
		HeadSHA    string `json:"head_sha"`
		ExternalID string `json:"external_id"`
		App        struct {
			ID int64 `json:"id"`
		} `json:"app"`
	} `json:"check_run"`
}

type installationPayload struct {
	Action       string `json:"action"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repositories []struct {
		ID int64 `json:"id"`
	} `json:"repositories"`
}

func projectPullRequest(p pullRequestPayload, limits Limits) (PullRequestProjection, error) {
	if p.Installation.ID <= 0 {
		return PullRequestProjection{}, errCode(CodeInvalidInstallationID, "projection", "installation id must be positive", nil)
	}
	if p.Repository.ID <= 0 {
		return PullRequestProjection{}, errCode(CodeInvalidRepositoryID, "projection", "repository id must be positive", nil)
	}
	if p.PullRequest.Number <= 0 {
		return PullRequestProjection{}, errCode(CodeInvalidPullRequestNumber, "projection", "pull request number must be positive", nil)
	}
	if !validGitObjectID(p.PullRequest.Base.SHA) || !validGitObjectID(p.PullRequest.Head.SHA) {
		return PullRequestProjection{}, errCode(CodeInvalidObjectID, "projection", "base/head SHA must be full lowercase object id", nil)
	}
	if _, err := ClassifyWebhookAction("pull_request", p.Action); err != nil {
		return PullRequestProjection{}, err
	}
	if len(p.Action) > limits.MaxProjectionStringBytes {
		return PullRequestProjection{}, errCode(CodeProjectionInvalid, "projection", "action too large", nil)
	}
	return PullRequestProjection{Action: p.Action, InstallationID: p.Installation.ID, RepositoryID: p.Repository.ID, RepositoryOwnerID: p.Repository.Owner.ID, PullRequestNumber: p.PullRequest.Number, BaseSHA: p.PullRequest.Base.SHA, HeadSHA: p.PullRequest.Head.SHA, HeadRepositoryID: p.PullRequest.Head.Repo.ID, Draft: p.PullRequest.Draft, Closed: p.PullRequest.State == "closed", Merged: p.PullRequest.Merged}, nil
}

func projectCheckRun(p checkRunPayload, limits Limits) (CheckRunProjection, error) {
	if p.Installation.ID <= 0 {
		return CheckRunProjection{}, errCode(CodeInvalidInstallationID, "projection", "installation id must be positive", nil)
	}
	if p.Repository.ID <= 0 {
		return CheckRunProjection{}, errCode(CodeInvalidRepositoryID, "projection", "repository id must be positive", nil)
	}
	if p.CheckRun.ID <= 0 || p.CheckRun.App.ID <= 0 {
		return CheckRunProjection{}, errCode(CodeProjectionInvalid, "projection", "check run/app id must be positive", nil)
	}
	if !validGitObjectID(p.CheckRun.HeadSHA) {
		return CheckRunProjection{}, errCode(CodeInvalidObjectID, "projection", "head SHA must be full lowercase object id", nil)
	}
	if len(p.CheckRun.ExternalID) > limits.MaxExternalIDBytes || hasControl(p.CheckRun.ExternalID) {
		return CheckRunProjection{}, errCode(CodeProjectionInvalid, "projection", "external id rejected", nil)
	}
	return CheckRunProjection{Action: p.Action, InstallationID: p.Installation.ID, RepositoryID: p.Repository.ID, CheckRunID: p.CheckRun.ID, HeadSHA: p.CheckRun.HeadSHA, AppID: p.CheckRun.App.ID, ExternalID: p.CheckRun.ExternalID}, nil
}

func projectInstallation(p installationPayload) (InstallationProjection, error) {
	if p.Installation.ID <= 0 {
		return InstallationProjection{}, errCode(CodeInvalidInstallationID, "projection", "installation id must be positive", nil)
	}
	ids := make([]int64, 0, len(p.Repositories))
	for _, r := range p.Repositories {
		if r.ID <= 0 {
			return InstallationProjection{}, errCode(CodeInvalidRepositoryID, "projection", "repository id must be positive", nil)
		}
		ids = append(ids, r.ID)
	}
	if ids == nil {
		ids = []int64{}
	}
	return InstallationProjection{Action: p.Action, InstallationID: p.Installation.ID, RepositoryIDs: ids}, nil
}

func validGitObjectID(s string) bool { return isLowerHex(s, 40) || isLowerHex(s, 64) }
