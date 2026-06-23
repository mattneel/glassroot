package githubreceiver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
	"github.com/mattneel/glassroot/internal/githubinbox"
)

type fakeStore struct {
	mu       sync.Mutex
	calls    int
	seen     []githubinbox.VerifiedDelivery
	result   githubinbox.AcceptResult
	err      error
	entered  chan struct{}
	unblock  chan struct{}
	returned chan struct{}
}

func (s *fakeStore) Accept(ctx context.Context, d githubinbox.VerifiedDelivery) (githubinbox.AcceptResult, error) {
	s.mu.Lock()
	s.calls++
	s.seen = append(s.seen, d)
	s.mu.Unlock()
	if s.entered != nil {
		close(s.entered)
	}
	if s.unblock != nil {
		select {
		case <-s.unblock:
		case <-ctx.Done():
			if s.returned != nil {
				close(s.returned)
			}
			return githubinbox.AcceptResult{}, ctx.Err()
		}
	}
	if s.returned != nil {
		close(s.returned)
	}
	return s.result, s.err
}

type panicStore struct{}

func (panicStore) Accept(context.Context, githubinbox.VerifiedDelivery) (githubinbox.AcceptResult, error) {
	panic("raw payload should not escape")
}

func TestHandleGitHubWebhookDurabilityBeforeAccepted(t *testing.T) {
	store := &fakeStore{
		result:  githubinbox.AcceptResult{Decision: githubinbox.AcceptNewEnqueued},
		entered: make(chan struct{}),
		unblock: make(chan struct{}),
	}
	h := newTestHandler(t, store, nil)
	req := signedRequest(t, "pull_request", validPullRequestPayload("opened"), testCurrentSecret)
	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rr, req)
		close(done)
	}()
	<-store.entered
	select {
	case <-done:
		t.Fatalf("handler returned before durable store accepted")
	case <-time.After(25 * time.Millisecond):
	}
	close(store.unblock)
	<-done
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "pull_request") || strings.Contains(rr.Body.String(), "123e4567") {
		t.Fatalf("response leaked request data: %q", rr.Body.String())
	}
}

func TestHandleGitHubWebhookRecoversWithFixedResponse(t *testing.T) {
	h := newTestHandler(t, panicStore{}, nil)
	req := signedRequest(t, "pull_request", validPullRequestPayload("opened"), testCurrentSecret)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "payload") || strings.Contains(rr.Body.String(), "panic") {
		t.Fatalf("panic detail leaked: %q", rr.Body.String())
	}
}

func TestHandleGitHubWebhookRejectsInvalidSignatureBeforeStore(t *testing.T) {
	store := &fakeStore{result: githubinbox.AcceptResult{Decision: githubinbox.AcceptNewEnqueued}}
	h := newTestHandler(t, store, nil)
	req := signedRequest(t, "pull_request", validPullRequestPayload("opened"), testCurrentSecret)
	req.Header.Set("X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if store.calls != 0 {
		t.Fatalf("store called for invalid signature")
	}
}

func TestHandleGitHubWebhookPersistsIgnoredUnsupportedSignedEvent(t *testing.T) {
	store := &fakeStore{result: githubinbox.AcceptResult{Decision: githubinbox.AcceptNewIgnored}}
	h := newTestHandler(t, store, nil)
	req := signedRequest(t, "check_suite", `{"action":"requested"}`, testCurrentSecret)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if store.calls != 1 || len(store.seen) != 1 {
		t.Fatalf("store calls=%d seen=%d", store.calls, len(store.seen))
	}
	if store.seen[0].Receipt.Disposition != githubapp.DeliveryDispositionIgnored || store.seen[0].Projection.Kind != githubapp.ProjectionCheckSuite {
		t.Fatalf("unexpected delivery %#v", store.seen[0])
	}
}

func TestHandleGitHubWebhookAcceptsPreviousSecret(t *testing.T) {
	store := &fakeStore{result: githubinbox.AcceptResult{Decision: githubinbox.AcceptNewEnqueued}}
	h := newTestHandler(t, store, nil)
	req := signedRequest(t, "pull_request", validPullRequestPayload("opened"), testPreviousSecret)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if store.calls != 1 || store.seen[0].MatchedSecret != githubapp.SecretGenerationPrevious {
		t.Fatalf("previous secret generation not recorded: calls=%d seen=%#v", store.calls, store.seen)
	}
}

func TestHandleGitHubWebhookStatusMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result githubinbox.AcceptResult
		err    error
		want   int
	}{
		{"duplicate", githubinbox.AcceptResult{Decision: githubinbox.AcceptDuplicateSameDelivery}, nil, http.StatusAccepted},
		{"conflict", githubinbox.AcceptResult{Decision: githubinbox.AcceptDeliveryConflict}, nil, http.StatusConflict},
		{"store failure", githubinbox.AcceptResult{}, errors.New("database path /tmp/secret unavailable"), http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{result: tc.result, err: tc.err}
			h := newTestHandler(t, store, nil)
			req := signedRequest(t, "pull_request", validPullRequestPayload("opened"), testCurrentSecret)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%q", rr.Code, tc.want, rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "secret") || strings.Contains(rr.Body.String(), "/tmp") {
				t.Fatalf("response leaked store error: %q", rr.Body.String())
			}
		})
	}
}

func TestHandleGitHubWebhookRequestValidation(t *testing.T) {
	store := &fakeStore{result: githubinbox.AcceptResult{Decision: githubinbox.AcceptNewEnqueued}}
	h := newTestHandler(t, store, nil)
	cases := []struct {
		name string
		mut  func(*http.Request)
		want int
	}{
		{"wrong method", func(r *http.Request) { r.Method = http.MethodGet }, http.StatusMethodNotAllowed},
		{"wrong path", func(r *http.Request) { r.URL.Path = "/webhooks/github/" }, http.StatusNotFound},
		{"query", func(r *http.Request) { r.URL.RawQuery = "x=1" }, http.StatusNotFound},
		{"duplicate delivery", func(r *http.Request) { r.Header.Add("X-GitHub-Delivery", "other") }, http.StatusBadRequest},
		{"comma signature", func(r *http.Request) {
			r.Header.Set("X-Hub-Signature-256", r.Header.Get("X-Hub-Signature-256")+",sha256="+strings.Repeat("1", 64))
		}, http.StatusUnauthorized},
		{"bad type", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
		{"gzip", func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }, http.StatusUnsupportedMediaType},
		{"trailer", func(r *http.Request) { r.Trailer = http.Header{"X-Trailer": []string{"x"}} }, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := signedRequest(t, "pull_request", validPullRequestPayload("opened"), testCurrentSecret)
			tc.mut(req)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%q", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

func TestLoadWebhookSecretsRejectsUnsafeFiles(t *testing.T) {
	dir := t.TempDir()
	current := writeSecretFile(t, dir, "current", testCurrentSecret, 0o600)
	previous := writeSecretFile(t, dir, "previous", testPreviousSecret, 0o400)
	secrets, err := LoadWebhookSecrets(current, previous, DefaultLimits())
	if err != nil {
		t.Fatalf("load valid secrets: %v", err)
	}
	if !bytes.Equal(secrets.Current, []byte(testCurrentSecret)) || !bytes.Equal(secrets.Previous, []byte(testPreviousSecret)) {
		t.Fatalf("secret bytes not preserved")
	}
	badMode := writeSecretFile(t, dir, "badmode", strings.Repeat("b", 32), 0o644)
	if _, err := LoadWebhookSecrets(badMode, "", DefaultLimits()); !errors.Is(err, ErrCode(CodeSecretModeInvalid)) {
		t.Fatalf("bad mode err=%v", err)
	}
	short := writeSecretFile(t, dir, "short", "short", 0o600)
	if _, err := LoadWebhookSecrets(short, "", DefaultLimits()); !errors.Is(err, ErrCode(CodeSecretSizeInvalid)) {
		t.Fatalf("short err=%v", err)
	}
	if _, err := LoadWebhookSecrets(current, current, DefaultLimits()); !errors.Is(err, ErrCode(CodeDuplicateSecret)) {
		t.Fatalf("same path err=%v", err)
	}
	same := writeSecretFile(t, dir, "same", testCurrentSecret, 0o600)
	if _, err := LoadWebhookSecrets(current, same, DefaultLimits()); !errors.Is(err, ErrCode(CodeDuplicateSecret)) {
		t.Fatalf("same bytes err=%v", err)
	}
}

func TestNewHandlerRequiresExplicitClock(t *testing.T) {
	_, err := NewHandler(HandlerConfig{
		ReceiverID: "receiver-1",
		Store:      &fakeStore{result: githubinbox.AcceptResult{Decision: githubinbox.AcceptNewIgnored}},
		Secrets:    githubapp.WebhookSecrets{Current: []byte(testCurrentSecret)},
		Limits:     newTestLimits(),
	})
	if !errors.Is(err, ErrCode(CodeInvalidConfig)) {
		t.Fatalf("err=%v", err)
	}
}

func newTestHandler(t *testing.T, store Store, limits *Limits) *Handler {
	t.Helper()
	l := newTestLimits()
	if limits != nil {
		l = *limits
	}
	h, err := NewHandler(HandlerConfig{ReceiverID: "receiver-1", Store: store, Secrets: githubapp.WebhookSecrets{Current: []byte(testCurrentSecret), Previous: []byte(testPreviousSecret)}, Limits: l, Clock: fixedClock{t: time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func newTestLimits() Limits {
	l := DefaultLimits()
	l.GitHub.MinWebhookSecretBytes = 1
	l.GitHub.MaxWebhookBodyBytes = 1024 * 1024
	return l
}

const (
	testCurrentSecret  = "current secret material has enough bytes"
	testPreviousSecret = "previous secret material has exactly enough bytes"
)

func signedRequest(t *testing.T, event, body, secret string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://unix/webhooks/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Delivery", "123e4567-e89b-12d3-a456-426614174000")
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-Hub-Signature-256", sign(body, secret))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func sign(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }
