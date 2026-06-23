package githubreceiver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/mattneel/glassroot/internal/githubapp"
	"github.com/mattneel/glassroot/internal/githubinbox"
)

type Server struct {
	HTTP                 *http.Server
	Listener             *UnixListener
	Store                *githubinbox.Store
	Secrets              githubapp.WebhookSecrets
	handler              *Handler
	maxActiveConnections int
}

type ServeConfig struct {
	ListenUnix         string
	StateDir           string
	ReceiverID         string
	CurrentSecretFile  string
	PreviousSecretFile string
	Limits             Limits
	InboxLimits        githubinbox.Limits
	Clock              Clock
	Logger             *slog.Logger
}

func NewServer(ctx context.Context, cfg ServeConfig) (*Server, error) {
	limits := cfg.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if cfg.Clock == nil {
		return nil, errCode(CodeInvalidConfig, "server", "clock required", nil)
	}
	secrets, err := LoadWebhookSecrets(cfg.CurrentSecretFile, cfg.PreviousSecretFile, limits)
	if err != nil {
		return nil, err
	}
	store, err := githubinbox.Open(ctx, githubinbox.Config{StateDir: cfg.StateDir, ReceiverID: cfg.ReceiverID, Limits: cfg.InboxLimits})
	if err != nil {
		ZeroSecrets(secrets)
		return nil, errCode(CodeInvalidStateDir, "store", "store open failed", err)
	}
	ln, err := ListenUnix(cfg.ListenUnix, cfg.StateDir, limits)
	if err != nil {
		_ = store.Close()
		ZeroSecrets(secrets)
		return nil, err
	}
	h, err := NewHandler(HandlerConfig{ReceiverID: cfg.ReceiverID, Store: store, Secrets: secrets, Limits: limits, Clock: cfg.Clock, Logger: cfg.Logger})
	if err != nil {
		_ = ln.CloseAndRemove()
		_ = store.Close()
		ZeroSecrets(secrets)
		return nil, err
	}
	server := &http.Server{Handler: h, MaxHeaderBytes: limits.MaxHeaderBytes, ReadHeaderTimeout: limits.ReadHeaderTimeout, ReadTimeout: limits.ReadTimeout, WriteTimeout: limits.WriteTimeout, IdleTimeout: limits.IdleTimeout, ErrorLog: slog.NewLogLogger(slog.NewTextHandler(ioDiscard{}, nil), slog.LevelError)}
	return &Server{HTTP: server, Listener: ln, Store: store, Secrets: secrets, handler: h, maxActiveConnections: limits.MaxActiveConnections}, nil
}

func (s *Server) Serve() error {
	if s == nil || s.HTTP == nil || s.Listener == nil {
		return errCode(CodeInvalidConfig, "serve", "server not initialized", nil)
	}
	limit := s.maxActiveConnections
	if limit <= 0 {
		limit = DefaultLimits().MaxActiveConnections
	}
	ll := &limitedListener{Listener: s.Listener, sem: make(chan struct{}, limit)}
	err := s.HTTP.Serve(ll)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	var first error
	if s == nil {
		return nil
	}
	if s.HTTP != nil {
		if err := s.HTTP.Shutdown(ctx); err != nil && first == nil {
			first = wrap(CodeShutdownFailed, "shutdown", "http shutdown failed", err)
		}
	}
	if s.Listener != nil {
		if err := s.Listener.CloseAndRemove(); err != nil && first == nil {
			first = err
		}
	}
	if s.Store != nil {
		if err := s.Store.Close(); err != nil && first == nil {
			first = err
		}
	}
	if s.handler != nil {
		s.handler.ZeroSecrets()
	}
	ZeroSecrets(s.Secrets)
	return first
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
