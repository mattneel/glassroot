package githubbroker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
)

type Issuer interface {
	IssueInstallationToken(context.Context, githubapi.TokenRequest) (*githubapi.TokenLease, error)
}
type ServerConfig struct {
	ListenUnix string
	Issuer     Issuer
	Logger     *slog.Logger
	Limits     Limits
}
type Server struct {
	ln      *UnixListener
	issuer  Issuer
	logger  *slog.Logger
	limits  Limits
	sem     chan struct{}
	connSem chan struct{}
	done    chan struct{}
	once    sync.Once
}

func NewServer(cfg ServerConfig) (*Server, error) {
	limits := cfg.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if cfg.Issuer == nil {
		return nil, errCode(CodeInvalidConfig, "server", "issuer required", nil)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	ln, err := ListenUnix(cfg.ListenUnix, limits)
	if err != nil {
		return nil, err
	}
	return &Server{ln: ln, issuer: cfg.Issuer, logger: logger, limits: limits, sem: make(chan struct{}, limits.MaxRequestsInFlight), connSem: make(chan struct{}, limits.MaxConcurrentConnections), done: make(chan struct{})}, nil
}
func (s *Server) SocketPath() string {
	if s == nil || s.ln == nil {
		return ""
	}
	return s.ln.path
}
func (s *Server) Serve() error {
	defer close(s.done)
	for {
		conn, err := s.ln.AcceptUnix()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return wrap(CodeListenerChanged, "serve", "accept failed", err)
		}
		select {
		case s.connSem <- struct{}{}:
			go func() { defer func() { <-s.connSem }(); s.handle(conn) }()
		default:
			s.writeError(conn, CodeConnectionLimit)
			_ = conn.Close()
		}
	}
}
func (s *Server) handle(conn *net.UnixConn) {
	start := time.Now()
	decision := "rejected"
	code := ErrorCode("")
	purpose := TokenPurpose("")
	defer func() { s.logDecision(purpose, decision, code, time.Since(start)) }()
	defer conn.Close()
	if err := checkPeer(conn); err != nil {
		code = codeFrom(err)
		s.writeError(conn, code)
		return
	}
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	default:
		code = CodeConnectionLimit
		s.writeError(conn, code)
		return
	}
	_ = conn.SetDeadline(time.Now().Add(s.limits.PerConnectionTimeout))
	payload, err := readFrame(conn, s.limits.MaxRequestFrameBytes)
	if err != nil {
		code = codeFrom(err)
		s.writeError(conn, code)
		return
	}
	req, err := decodeRequestPayload(payload, s.limits)
	if err != nil {
		code = codeFrom(err)
		s.writeError(conn, code)
		return
	}
	purpose = req.Purpose
	ctx, cancel := context.WithTimeout(context.Background(), s.limits.PerConnectionTimeout)
	defer cancel()
	lease, err := s.issuer.IssueInstallationToken(ctx, toAPIRequest(req))
	if err != nil {
		code = CodeTokenIssueFailed
		s.writeError(conn, code)
		return
	}
	defer lease.Close()
	meta := fromAPIMetadata(lease.Metadata())
	var tok []byte
	if err := lease.Use(func(b []byte) error { tok = append([]byte(nil), b...); return nil }); err != nil {
		code = CodeTokenIssueFailed
		s.writeError(conn, code)
		return
	}
	defer zero(tok)
	resp := TokenResponse{SchemaVersion: SchemaTokenResponseV1Alpha1, Purpose: meta.Purpose, InstallationID: meta.InstallationID, RepositoryID: meta.RepositoryID, ExpiresAt: &meta.ExpiresAt, GrantedPermissions: meta.GrantedPermissions, Token: string(tok)}
	if err := writeFrame(conn, resp, s.limits.MaxResponseFrameBytes); err != nil {
		code = CodeResponseWriteFailed
		return
	}
	decision = "issued"
}
func (s *Server) writeError(w io.Writer, code ErrorCode) {
	resp := TokenResponse{SchemaVersion: SchemaTokenResponseV1Alpha1, ErrorCode: code, Message: "request rejected"}
	_ = writeFrame(w, resp, s.limits.MaxResponseFrameBytes)
}
func writeFrame(w io.Writer, v any, max int) error {
	frame, err := encodeJSONFrame(v, max)
	if err != nil {
		return err
	}
	if _, err := w.Write(frame); err != nil {
		return wrap(CodeResponseWriteFailed, "response", "response write failed", err)
	}
	return nil
}
func (s *Server) Shutdown(ctx context.Context) error {
	var err error
	s.once.Do(func() {
		err = s.ln.CloseAndRemove()
		select {
		case <-s.done:
		case <-ctx.Done():
			if err == nil {
				err = wrap(CodeShutdownFailed, "shutdown", "shutdown timed out", ctx.Err())
			}
		}
	})
	return err
}
func codeFrom(err error) ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeRequestFrameInvalid
}

func (s *Server) logDecision(purpose TokenPurpose, decision string, code ErrorCode, d time.Duration) {
	if s == nil || s.logger == nil {
		return
	}
	attrs := []any{"component", "github-credential-broker", "operation", "token-request", "decision", decision, "durationMs", d.Milliseconds()}
	if purpose != "" {
		attrs = append(attrs, "purpose", string(purpose))
	}
	if code != "" {
		attrs = append(attrs, "errorCode", string(code))
	}
	s.logger.Info("github credential broker request", attrs...)
}
