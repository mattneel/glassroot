package githubapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubauth"
)

func TestClientVerifiesAppIdentityAndPermissionProfile(t *testing.T) {
	rt := &recordingRoundTripper{responses: []fakeResponse{{status: 200, body: `{"id":123,"client_id":"Iv1.client","permissions":{"checks":"write","contents":"read","pull_requests":"read","metadata":"read"},"events":["pull_request","check_run","check_suite","installation","installation_repositories","ping"]}`}}}
	client := newTestClient(t, rt)
	if err := client.VerifyApp(context.Background()); err != nil {
		t.Fatalf("VerifyApp: %v", err)
	}
	req := rt.requests[0]
	if req.Method != http.MethodGet || req.URL.String() != "https://api.github.com/app" {
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
	}
	assertCommonHeaders(t, req)
	if req.Body != nil && req.ContentLength > 0 {
		t.Fatalf("GET /app had body")
	}
}

func TestClientRejectsUnexpectedAppWritePermission(t *testing.T) {
	rt := &recordingRoundTripper{responses: []fakeResponse{{status: 200, body: `{"id":123,"client_id":"Iv1.client","permissions":{"checks":"write","contents":"read","pull_requests":"read","metadata":"read","issues":"write"},"events":["pull_request"]}`}}}
	client := newTestClient(t, rt)
	if err := client.VerifyApp(context.Background()); err == nil {
		t.Fatalf("unexpected write permission accepted")
	}
}

func TestIssueInstallationTokenUsesExactEndpointRepositoryAndPurposeScope(t *testing.T) {
	expires := time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC)
	rt := &recordingRoundTripper{responses: []fakeResponse{
		{status: 200, body: `{"id":777,"app_id":123,"target_id":999,"repository_selection":"selected","permissions":{"pull_requests":"read","contents":"read","metadata":"read"},"suspended_at":null}`},
		{status: 201, body: `{"token":"opaque-variable-length-token-ghs_1234567890abcdef","expires_at":"` + expires.Format(time.RFC3339) + `","permissions":{"pull_requests":"read","metadata":"read"},"repositories":[{"id":456}]}`},
	}}
	client := newTestClient(t, rt)
	lease, err := client.IssueInstallationToken(context.Background(), githubapi.TokenRequest{Purpose: githubapi.PurposePullRequestRead, InstallationID: 777, RepositoryID: 456})
	if err != nil {
		t.Fatalf("IssueInstallationToken: %v", err)
	}
	defer lease.Close()
	if lease.Metadata().Purpose != githubapi.PurposePullRequestRead || !lease.Metadata().ExpiresAt.Equal(expires) {
		t.Fatalf("bad metadata: %#v", lease.Metadata())
	}
	var gotToken []byte
	if err := lease.Use(func(token []byte) error { gotToken = append([]byte(nil), token...); return nil }); err != nil {
		t.Fatalf("Use token: %v", err)
	}
	if string(gotToken) != "opaque-variable-length-token-ghs_1234567890abcdef" {
		t.Fatalf("token not preserved as opaque bytes")
	}
	if len(rt.requests) != 2 {
		t.Fatalf("request count = %d", len(rt.requests))
	}
	instReq := rt.requests[0]
	if instReq.Method != http.MethodGet || instReq.URL.String() != "https://api.github.com/app/installations/777" {
		t.Fatalf("unexpected installation request: %s %s", instReq.Method, instReq.URL.String())
	}
	postReq := rt.requests[1]
	if postReq.Method != http.MethodPost || postReq.URL.String() != "https://api.github.com/app/installations/777/access_tokens" {
		t.Fatalf("unexpected token request: %s %s", postReq.Method, postReq.URL.String())
	}
	assertCommonHeaders(t, postReq)
	var body map[string]any
	if err := json.Unmarshal(rt.bodies[1], &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["repositories"]; ok {
		t.Fatalf("repository names field used: %s", rt.bodies[1])
	}
	ids := body["repository_ids"].([]any)
	if len(ids) != 1 || int64(ids[0].(float64)) != 456 {
		t.Fatalf("bad repository_ids: %s", rt.bodies[1])
	}
	perms := body["permissions"].(map[string]any)
	if len(perms) != 1 || perms["pull_requests"] != "read" {
		t.Fatalf("bad permissions: %s", rt.bodies[1])
	}
}

func TestIssueSourceReadTokenUsesContentsOnly(t *testing.T) {
	expires := time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC)
	rt := &recordingRoundTripper{responses: []fakeResponse{
		{status: 200, body: `{"id":888,"app_id":123,"target_id":999,"repository_selection":"all","permissions":{"contents":"read","metadata":"read"},"suspended_at":null}`},
		{status: 201, body: `{"token":"opaque-source-token","expires_at":"` + expires.Format(time.RFC3339) + `","permissions":{"contents":"read","metadata":"read"},"repositories":[{"id":654}]}`},
	}}
	client := newTestClient(t, rt)
	lease, err := client.IssueInstallationToken(context.Background(), githubapi.TokenRequest{Purpose: githubapi.PurposeSourceRead, InstallationID: 888, RepositoryID: 654})
	if err != nil {
		t.Fatalf("IssueInstallationToken: %v", err)
	}
	defer lease.Close()
	var body map[string]any
	if err := json.Unmarshal(rt.bodies[1], &body); err != nil {
		t.Fatal(err)
	}
	perms := body["permissions"].(map[string]any)
	if len(perms) != 1 || perms["contents"] != "read" {
		t.Fatalf("bad source-read body: %s", rt.bodies[1])
	}
}

func TestIssueInstallationTokenRejectsBroaderResponseScope(t *testing.T) {
	rt := &recordingRoundTripper{responses: []fakeResponse{
		{status: 200, body: `{"id":777,"app_id":123,"target_id":999,"repository_selection":"selected","permissions":{"pull_requests":"read","metadata":"read"},"suspended_at":null}`},
		{status: 201, body: `{"token":"secret-canary-token","expires_at":"2026-06-23T13:00:00Z","permissions":{"pull_requests":"read","checks":"write","metadata":"read"},"repositories":[{"id":456}]}`},
	}}
	client := newTestClient(t, rt)
	_, err := client.IssueInstallationToken(context.Background(), githubapi.TokenRequest{Purpose: githubapi.PurposePullRequestRead, InstallationID: 777, RepositoryID: 456})
	if err == nil {
		t.Fatalf("broader token response accepted")
	}
	if strings.Contains(err.Error(), "secret-canary-token") {
		t.Fatalf("token leaked in error: %v", err)
	}
}

func TestClientRejectsRedirectAndOversizedResponse(t *testing.T) {
	redirectRT := &recordingRoundTripper{responses: []fakeResponse{{status: 302, body: `{}`}}}
	if err := newTestClient(t, redirectRT).VerifyApp(context.Background()); err == nil {
		t.Fatalf("redirect accepted")
	}
	oversized := strings.Repeat("a", githubapi.DefaultLimits().MaxAPIResponseBytes+1)
	bigRT := &recordingRoundTripper{responses: []fakeResponse{{status: 200, body: oversized}}}
	if err := newTestClient(t, bigRT).VerifyApp(context.Background()); err == nil {
		t.Fatalf("oversized response accepted")
	}
}

func FuzzDecodeGitHubTokenResponse(f *testing.F) {
	f.Add([]byte(`{"token":"abc","expires_at":"2026-06-23T13:00:00Z","permissions":{"contents":"read"},"repositories":[{"id":1}]}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = githubapi.DecodeTokenResponseForTest(b, githubapi.TokenRequest{Purpose: githubapi.PurposeSourceRead, InstallationID: 1, RepositoryID: 1}, time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC), githubapi.DefaultLimits())
	})
}

type fakeJWTSigner struct{}

func (fakeJWTSigner) SignJWT(githubauth.AppIdentity, time.Time, githubauth.Limits) (*githubauth.AppJWT, error) {
	return githubauth.NewAppJWTForTest([]byte("jwt.canary.value")), nil
}

func newTestClient(t *testing.T, rt *recordingRoundTripper) *githubapi.Client {
	t.Helper()
	client, err := githubapi.NewClient(githubapi.Config{Identity: githubauth.AppIdentity{AppID: 123, ClientID: "Iv1.client"}, Signer: fakeJWTSigner{}, Clock: fixedClock{time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)}, Transport: rt, Limits: githubapi.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

type fakeResponse struct {
	status  int
	body    string
	headers map[string]string
}
type recordingRoundTripper struct {
	mu        sync.Mutex
	responses []fakeResponse
	requests  []*http.Request
	bodies    [][]byte
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
	}
	r.requests = append(r.requests, req.Clone(req.Context()))
	r.bodies = append(r.bodies, body)
	if len(r.responses) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	resp := r.responses[0]
	r.responses = r.responses[1:]
	headers := make(http.Header)
	for k, v := range resp.headers {
		headers.Set(k, v)
	}
	return &http.Response{StatusCode: resp.status, Header: headers, Body: io.NopCloser(bytes.NewReader([]byte(resp.body))), Request: req}, nil
}
func assertCommonHeaders(t *testing.T, req *http.Request) {
	t.Helper()
	if req.URL.Scheme != "https" || req.URL.Host != "api.github.com" {
		t.Fatalf("bad origin: %s", req.URL.String())
	}
	if req.Header.Get("Accept") != "application/vnd.github+json" {
		t.Fatalf("bad accept")
	}
	if req.Header.Get("X-GitHub-Api-Version") != "2026-03-10" {
		t.Fatalf("bad api version")
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
		t.Fatalf("bad authorization header")
	}
	if req.Header.Get("User-Agent") == "" {
		t.Fatalf("missing user-agent")
	}
}
