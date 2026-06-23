package githubapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
)

func TestGetPullRequestUsesInstallationTokenAndKeepsMinimalSnapshot(t *testing.T) {
	base := strings.Repeat("1", 40)
	head := strings.Repeat("2", 40)
	rt := &recordingRoundTripper{responses: []fakeResponse{{status: 200, body: `{"number":7,"state":"open","draft":false,"merged":false,"base":{"sha":"` + base + `","repo":{"id":101,"name":"REPO","owner":{"login":"OWNER"}}},"head":{"sha":"` + head + `","repo":{"id":202,"name":"HEADREPO","owner":{"login":"HEADOWNER"}}},"title":"secret title","body":"secret body","html_url":"https://example.invalid"}`}}}
	client := newTestClient(t, rt)
	lease := githubapi.NewTokenLeaseForTest(githubapi.TokenMetadata{Purpose: githubapi.PurposePullRequestRead, InstallationID: 42, RepositoryID: 101, ExpiresAt: time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC)}, []byte("opaque-token"))
	defer lease.Close()

	snap, err := client.GetPullRequest(context.Background(), lease, githubapi.RepositoryRoute{Owner: "OWNER", Repo: "REPO", RepositoryID: 101}, 7)
	if err != nil {
		t.Fatalf("GetPullRequest failed: %v", err)
	}
	if len(rt.requests) != 1 {
		t.Fatalf("request count = %d", len(rt.requests))
	}
	req := rt.requests[0]
	if req.Method != http.MethodGet || req.URL.String() != "https://api.github.com/repos/OWNER/REPO/pulls/7" {
		t.Fatalf("unexpected request %s %s", req.Method, req.URL.String())
	}
	assertCommonHeaders(t, req)
	if got := req.Header.Get("Authorization"); got != "Bearer opaque-token" {
		t.Fatalf("authorization = %q", got)
	}
	if snap.Number != 7 || snap.State != githubapi.PullRequestStateOpen || snap.Draft || snap.Merged {
		t.Fatalf("unexpected state: %#v", snap)
	}
	if snap.Base.RepositoryID != 101 || snap.Head.RepositoryID != 202 || snap.Base.CommitID != base || snap.Head.CommitID != head || !snap.Head.Available {
		t.Fatalf("unexpected identities: %#v", snap)
	}
	if text := fmt.Sprintf("%#v", snap); strings.Contains(text, "secret title") || strings.Contains(text, "secret body") || strings.Contains(text, "example.invalid") {
		t.Fatalf("prohibited response fields retained: %s", text)
	}
}
