package githubreceiver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
	"github.com/mattneel/glassroot/internal/githubinbox"
)

type Handler struct {
	receiverID string
	store      Store
	secrets    githubapp.WebhookSecrets
	limits     Limits
	clock      Clock
	logger     *slog.Logger
	active     chan struct{}
}

func NewHandler(cfg HandlerConfig) (*Handler, error) {
	limits := cfg.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if limits.GitHub == (githubapp.Limits{}) {
		limits.GitHub = githubapp.DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if err := validateReceiverID(cfg.ReceiverID); err != nil {
		return nil, err
	}
	if cfg.Store == nil {
		return nil, errCode(CodeInvalidConfig, "handler", "store required", nil)
	}
	if err := validateSecretSet(cfg.Secrets, limits.GitHub); err != nil {
		return nil, err
	}
	clock := cfg.Clock
	if clock == nil {
		return nil, errCode(CodeInvalidConfig, "handler", "clock required", nil)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	}
	return &Handler{receiverID: cfg.ReceiverID, store: cfg.Store, secrets: cloneSecrets(cfg.Secrets), limits: limits, clock: clock, logger: logger, active: make(chan struct{}, limits.MaxActiveRequests)}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := h.clock.Now().UTC().Round(0)
	status, code, decision := http.StatusServiceUnavailable, CodeStoreUnavailable, "rejected"
	defer func() {
		if recovered := recover(); recovered != nil {
			setFixedHeaders(w)
			fixedResponse(w, http.StatusServiceUnavailable, "unavailable\n")
			status, code, decision = http.StatusServiceUnavailable, CodeStoreUnavailable, "rejected"
		}
		h.log(r.Context(), status, code, decision, h.clock.Now().UTC().Round(0).Sub(start))
	}()
	status, code, decision = h.handle(w, r)
}

func (h *Handler) handle(w http.ResponseWriter, r *http.Request) (int, ErrorCode, string) {
	setFixedHeaders(w)
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		fixedResponse(w, http.StatusMethodNotAllowed, "method not allowed\n")
		return http.StatusMethodNotAllowed, CodeInvalidMethod, "rejected"
	}
	if r.URL == nil || r.URL.Path != WebhookPath {
		fixedResponse(w, http.StatusNotFound, "not found\n")
		return http.StatusNotFound, CodeInvalidPath, "rejected"
	}
	if r.URL.RawQuery != "" {
		fixedResponse(w, http.StatusNotFound, "not found\n")
		return http.StatusNotFound, CodeQueryNotAllowed, "rejected"
	}
	select {
	case h.active <- struct{}{}:
		defer func() { <-h.active }()
	default:
		fixedResponse(w, http.StatusServiceUnavailable, "unavailable\n")
		return http.StatusServiceUnavailable, CodeRequestCapacity, "rejected"
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.limits.PerRequestIntakeTimeout)
	defer cancel()
	headers, err := parseRequestHeaders(r, h.limits)
	if err != nil {
		status := statusForHeaderError(err)
		fixedResponse(w, status, responseFor(status))
		return status, codeForError(err), "rejected"
	}
	if len(r.Trailer) > 0 || r.Header.Get("Trailer") != "" {
		fixedResponse(w, http.StatusBadRequest, "bad request\n")
		return http.StatusBadRequest, CodeProjectionInvalid, "rejected"
	}
	body, err := readBoundedBody(w, r, h.limits.GitHub.MaxWebhookBodyBytes)
	if err != nil {
		status := http.StatusBadRequest
		code := CodeBodyReadFailed
		if errors.Is(err, http.ErrBodyReadAfterClose) {
			status = http.StatusBadRequest
		}
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			status = http.StatusRequestEntityTooLarge
			code = CodeBodyTooLarge
		}
		fixedResponse(w, status, responseFor(status))
		return status, code, "rejected"
	}
	defer zero(body)
	secretGen, err := githubapp.VerifyWebhookSignature(body, headers.Signature256, h.secrets, h.limits.GitHub)
	if err != nil {
		fixedResponse(w, http.StatusUnauthorized, "unauthorized\n")
		return http.StatusUnauthorized, CodeSignatureInvalid, "rejected"
	}
	projection, projectErr := githubapp.ProjectWebhook(headers.Event, body, h.limits.GitHub)
	if projectErr != nil {
		if errors.Is(projectErr, githubapp.ErrCode(githubapp.CodeUnsupportedEvent)) {
			projection = githubapp.WebhookProjection{Kind: githubapp.ProjectionIgnored}
		} else {
			fixedResponse(w, http.StatusBadRequest, "bad request\n")
			return http.StatusBadRequest, CodeProjectionInvalid, "rejected"
		}
	}
	action := projectionAction(projection)
	if err := crossCheck(headers.Event, projection); err != nil {
		fixedResponse(w, http.StatusBadRequest, "bad request\n")
		return http.StatusBadRequest, CodeProjectionInvalid, "rejected"
	}
	disposition := dispositionFor(headers.Event, action, projection.Kind)
	now := h.clock.Now().UTC().Round(0)
	bodyDigest := githubapp.DigestRawBody(body)
	fingerprint := githubinbox.ComputeIntakeFingerprint(h.receiverID, headers.DeliveryID, headers.Event, bodyDigest, string(projection.Kind))
	receipt := githubapp.DeliveryReceipt{SchemaVersion: githubapp.SchemaGitHubWebhookReceiptV1Alpha1, ReceiverID: h.receiverID, DeliveryID: headers.DeliveryID, Event: headers.Event, BodyDigest: bodyDigest, MatchedSecret: secretGen, ReceivedAt: now, ProjectionKind: projection.Kind, Disposition: disposition}
	delivery := githubinbox.VerifiedDelivery{ReceiverID: h.receiverID, DeliveryID: headers.DeliveryID, Event: headers.Event, Action: action, BodyDigest: bodyDigest, MatchedSecret: secretGen, ReceivedAt: now, Projection: projection, Receipt: receipt, Disposition: disposition, IntakeFingerprint: fingerprint}
	res, err := h.store.Accept(ctx, delivery)
	if err != nil && !errors.Is(err, githubinbox.ErrCode(githubinbox.CodeDeliveryConflict)) {
		fixedResponse(w, http.StatusServiceUnavailable, "unavailable\n")
		return http.StatusServiceUnavailable, CodeStoreUnavailable, "rejected"
	}
	switch res.Decision {
	case githubinbox.AcceptNewEnqueued:
		fixedResponse(w, http.StatusAccepted, "accepted\n")
		return http.StatusAccepted, "", "accepted"
	case githubinbox.AcceptNewIgnored:
		fixedResponse(w, http.StatusAccepted, "accepted\n")
		return http.StatusAccepted, "", "ignored"
	case githubinbox.AcceptDuplicateSameDelivery:
		fixedResponse(w, http.StatusAccepted, "accepted\n")
		return http.StatusAccepted, "", "duplicate"
	case githubinbox.AcceptDeliveryConflict:
		fixedResponse(w, http.StatusConflict, "conflict\n")
		return http.StatusConflict, CodeStoreUnavailable, "conflict"
	default:
		fixedResponse(w, http.StatusServiceUnavailable, "unavailable\n")
		return http.StatusServiceUnavailable, CodeStoreUnavailable, "rejected"
	}
}

func readBoundedBody(w http.ResponseWriter, r *http.Request, max int) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(max))
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}
func setFixedHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func fixedResponse(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
func responseFor(status int) string {
	switch status {
	case http.StatusUnsupportedMediaType:
		return "unsupported media type\n"
	case http.StatusRequestEntityTooLarge:
		return "request too large\n"
	case http.StatusUnauthorized:
		return "unauthorized\n"
	case http.StatusConflict:
		return "conflict\n"
	case http.StatusServiceUnavailable:
		return "unavailable\n"
	default:
		return "bad request\n"
	}
}

func (h *Handler) log(ctx context.Context, status int, code ErrorCode, decision string, dur time.Duration) {
	if h.logger == nil {
		return
	}
	attrs := []any{"component", "githubreceiver", "operation", "webhook", "status", status, "decision", decision, "duration_ms", dur.Milliseconds()}
	if code != "" {
		attrs = append(attrs, "error_code", string(code))
	}
	h.logger.InfoContext(ctx, "github webhook intake", attrs...)
}

func dispositionFor(event, action string, kind githubapp.ProjectionKind) githubapp.DeliveryDisposition {
	if kind == githubapp.ProjectionIgnored || kind == githubapp.ProjectionPing || kind == githubapp.ProjectionCheckSuite {
		return githubapp.DeliveryDispositionIgnored
	}
	decision, err := githubapp.ClassifyWebhookAction(event, action)
	if err == nil {
		switch decision {
		case githubapp.WebhookActionSchedule, githubapp.WebhookActionCancel, githubapp.WebhookActionRerequest:
			return githubapp.DeliveryDispositionEnqueued
		}
	}
	if kind == githubapp.ProjectionInstallation {
		return githubapp.DeliveryDispositionEnqueued
	}
	return githubapp.DeliveryDispositionIgnored
}
func projectionAction(p githubapp.WebhookProjection) string {
	if p.PullRequest != nil {
		return p.PullRequest.Action
	}
	if p.CheckRun != nil {
		return p.CheckRun.Action
	}
	if p.Installation != nil {
		return p.Installation.Action
	}
	return ""
}
func crossCheck(event string, p githubapp.WebhookProjection) error {
	switch event {
	case "pull_request":
		if p.Kind != githubapp.ProjectionPullRequest || p.PullRequest == nil {
			return errCode(CodeProjectionInvalid, "projection", "event projection mismatch", nil)
		}
	case "check_run":
		if p.Kind != githubapp.ProjectionCheckRun && p.Kind != githubapp.ProjectionIgnored {
			return errCode(CodeProjectionInvalid, "projection", "event projection mismatch", nil)
		}
	case "check_suite":
		if p.Kind != githubapp.ProjectionCheckSuite {
			return errCode(CodeProjectionInvalid, "projection", "event projection mismatch", nil)
		}
	case "installation", "installation_repositories":
		if p.Kind != githubapp.ProjectionInstallation {
			return errCode(CodeProjectionInvalid, "projection", "event projection mismatch", nil)
		}
	case "ping":
		if p.Kind != githubapp.ProjectionPing {
			return errCode(CodeProjectionInvalid, "projection", "event projection mismatch", nil)
		}
	}
	return nil
}
func cloneSecrets(s githubapp.WebhookSecrets) githubapp.WebhookSecrets {
	return githubapp.WebhookSecrets{Current: append([]byte(nil), s.Current...), Previous: append([]byte(nil), s.Previous...)}
}

func (h *Handler) ZeroSecrets() {
	if h == nil {
		return
	}
	ZeroSecrets(h.secrets)
}

func validateSecretSet(s githubapp.WebhookSecrets, limits githubapp.Limits) error {
	if len(s.Current) < limits.MinWebhookSecretBytes || len(s.Current) > limits.MaxWebhookSecretBytes {
		return errCode(CodeInvalidConfig, "handler", "current secret rejected", nil)
	}
	if len(s.Previous) > 0 {
		if len(s.Previous) < limits.MinWebhookSecretBytes || len(s.Previous) > limits.MaxWebhookSecretBytes {
			return errCode(CodeInvalidConfig, "handler", "previous secret rejected", nil)
		}
		if bytes.Equal(s.Current, s.Previous) {
			return errCode(CodeInvalidConfig, "handler", "secret generations must differ", nil)
		}
	}
	return nil
}
func stringsRepeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
