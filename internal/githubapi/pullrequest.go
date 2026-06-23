package githubapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

const PullRequestSnapshotSchemaV1Alpha1 = "glassroot.dev/github-pull-request-snapshot/v1alpha1"

type TokenUser interface {
	Use(func([]byte) error) error
}

type RepositoryRoute struct {
	Owner        string
	Repo         string
	RepositoryID int64
}

type PullRequestState string

const (
	PullRequestStateOpen   PullRequestState = "open"
	PullRequestStateClosed PullRequestState = "closed"
)

type PullRequestEndpoint struct {
	RepositoryID int64
	Owner        string
	Name         string
	CommitID     string
	Available    bool
}

type PullRequestSnapshot struct {
	SchemaVersion string
	Number        int64
	State         PullRequestState
	Draft         bool
	Merged        bool
	Base          PullRequestEndpoint
	Head          PullRequestEndpoint
}

type pullRequestResponse struct {
	Number int64  `json:"number"`
	State  string `json:"state"`
	Draft  bool   `json:"draft"`
	Merged bool   `json:"merged"`
	Base   struct {
		SHA  string `json:"sha"`
		Repo *struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repo"`
	} `json:"base"`
	Head struct {
		SHA  string `json:"sha"`
		Repo *struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repo"`
	} `json:"head"`
}

func (c *Client) GetPullRequest(ctx context.Context, token TokenUser, route RepositoryRoute, number int64) (PullRequestSnapshot, error) {
	if err := validateRepositoryRoute(route); err != nil {
		return PullRequestSnapshot{}, err
	}
	if number <= 0 {
		return PullRequestSnapshot{}, errCode(CodeResponseInvalid, "pull-request", "pull request number rejected", nil)
	}
	path := "/repos/" + url.PathEscape(route.Owner) + "/" + url.PathEscape(route.Repo) + "/pulls/" + strconv.FormatInt(number, 10)
	var out pullRequestResponse
	if err := c.getInstallationJSON(ctx, token, path, &out); err != nil {
		return PullRequestSnapshot{}, err
	}
	return decodePullRequestSnapshot(out, route, number)
}

func validateRepositoryRoute(route RepositoryRoute) error {
	if route.RepositoryID <= 0 || !validRouteSegment(route.Owner) || !validRouteSegment(route.Repo) {
		return errCode(CodeResponseInvalid, "route", "repository route rejected", nil)
	}
	return nil
}

func validRouteSegment(s string) bool {
	if s == "" || len(s) > 256 || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '/' || r == '?' || r == '#' || r == '\\' {
			return false
		}
	}
	return true
}

func decodePullRequestSnapshot(r pullRequestResponse, route RepositoryRoute, requestedNumber int64) (PullRequestSnapshot, error) {
	if r.Number != requestedNumber || r.Number <= 0 {
		return PullRequestSnapshot{}, errCode(CodeResponseInvalid, "pull-request", "pull request number mismatch", nil)
	}
	state := PullRequestState(r.State)
	if state != PullRequestStateOpen && state != PullRequestStateClosed {
		return PullRequestSnapshot{}, errCode(CodeResponseInvalid, "pull-request", "pull request state rejected", nil)
	}
	if r.Merged && state == PullRequestStateOpen {
		return PullRequestSnapshot{}, errCode(CodeResponseInvalid, "pull-request", "merged open pull request rejected", nil)
	}
	if r.Base.Repo == nil || r.Base.Repo.ID != route.RepositoryID || !validRouteSegment(r.Base.Repo.Owner.Login) || !validRouteSegment(r.Base.Repo.Name) {
		return PullRequestSnapshot{}, errCode(CodeResponseInvalid, "pull-request", "base repository mismatch", nil)
	}
	if !validGitObjectID(r.Base.SHA) {
		return PullRequestSnapshot{}, errCode(CodeResponseInvalid, "pull-request", "base commit rejected", nil)
	}
	snap := PullRequestSnapshot{SchemaVersion: PullRequestSnapshotSchemaV1Alpha1, Number: r.Number, State: state, Draft: r.Draft, Merged: r.Merged, Base: PullRequestEndpoint{RepositoryID: r.Base.Repo.ID, Owner: r.Base.Repo.Owner.Login, Name: r.Base.Repo.Name, CommitID: r.Base.SHA, Available: true}}
	if r.Head.Repo == nil {
		snap.Head.Available = false
		return snap, nil
	}
	if r.Head.Repo.ID <= 0 || !validRouteSegment(r.Head.Repo.Owner.Login) || !validRouteSegment(r.Head.Repo.Name) || !validGitObjectID(r.Head.SHA) {
		return PullRequestSnapshot{}, errCode(CodeResponseInvalid, "pull-request", "head repository rejected", nil)
	}
	snap.Head = PullRequestEndpoint{RepositoryID: r.Head.Repo.ID, Owner: r.Head.Repo.Owner.Login, Name: r.Head.Repo.Name, CommitID: r.Head.SHA, Available: true}
	return snap, nil
}

func (c *Client) getInstallationJSON(ctx context.Context, token TokenUser, path string, out any) error {
	return c.doTokenJSON(ctx, http.MethodGet, path, nil, token, out)
}

func validGitObjectID(s string) bool { return isLowerHex(s, 40) || isLowerHex(s, 64) }
func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if r < '0' || (r > '9' && r < 'a') || r > 'f' {
			return false
		}
	}
	return true
}
