package githubapp

import (
	"strings"
	"testing"
)

func TestPullRequestProjectionIncludesBoundedBaseRouteHints(t *testing.T) {
	body := `{"action":"opened","installation":{"id":42},"repository":{"id":101,"name":"repo","owner":{"id":201,"login":"owner"}},"pull_request":{"number":7,"draft":false,"merged":false,"state":"open","base":{"sha":"` + strings.Repeat("1", 40) + `","repo":{"id":101,"name":"repo","owner":{"login":"owner"}}},"head":{"sha":"` + strings.Repeat("2", 40) + `","repo":{"id":202,"name":"headrepo","owner":{"login":"headowner"}}}}}`
	projection, err := ProjectWebhook("pull_request", []byte(body), DefaultLimits())
	if err != nil {
		t.Fatalf("ProjectWebhook: %v", err)
	}
	if projection.PullRequest.BaseRepositoryOwner != "owner" || projection.PullRequest.BaseRepositoryName != "repo" {
		t.Fatalf("missing route hints: %#v", projection.PullRequest)
	}
}
