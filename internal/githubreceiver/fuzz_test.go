package githubreceiver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/githubinbox"
)

func FuzzHandleGitHubWebhookRequest(f *testing.F) {
	f.Add("pull_request", validPullRequestPayload("opened"), true)
	f.Add("check_suite", `{"action":"requested"}`, true)
	f.Add("issue_comment", `{"action":"created"}`, true)
	f.Add("pull_request", `{`, false)
	f.Fuzz(func(t *testing.T, event, body string, validSig bool) {
		if len(event) > 128 || len(body) > 1<<16 {
			t.Skip()
		}
		store := &fakeStore{result: githubinbox.AcceptResult{Decision: githubinbox.AcceptNewIgnored}}
		h := newTestHandler(t, store, nil)
		req := httptest.NewRequest(http.MethodPost, "http://unix/webhooks/github", strings.NewReader(body))
		req.Header.Set("X-GitHub-Delivery", "123e4567-e89b-12d3-a456-426614174000")
		req.Header.Set("X-GitHub-Event", event)
		req.Header.Set("Content-Type", "application/json")
		if validSig {
			req.Header.Set("X-Hub-Signature-256", sign(body, testCurrentSecret))
		} else {
			req.Header.Set("X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64))
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code < 200 || rr.Code > 599 {
			t.Fatalf("invalid status %d", rr.Code)
		}
	})
}

func FuzzValidateReceiverFilesystemPaths(f *testing.F) {
	f.Add("/tmp/glassroot-receiver.sock")
	f.Add("relative")
	f.Add("/tmp/../tmp/x")
	f.Add("/tmp/with\x00nul")
	f.Fuzz(func(t *testing.T, path string) {
		if len(path) > 8192 {
			t.Skip()
		}
		_ = validatePath(path, DefaultLimits().MaxPathBytes)
	})
}
