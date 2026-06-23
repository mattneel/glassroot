package githubreceiver

import (
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
)

const (
	WebhookPath = "/webhooks/github"
)

type Limits struct {
	GitHub                  githubapp.Limits
	MaxHeaderBytes          int
	MaxActiveRequests       int
	MaxActiveConnections    int
	ReadHeaderTimeout       time.Duration
	ReadTimeout             time.Duration
	PerRequestIntakeTimeout time.Duration
	WriteTimeout            time.Duration
	IdleTimeout             time.Duration
	ShutdownTimeout         time.Duration
	MaxPathBytes            int
}

func DefaultLimits() Limits {
	return Limits{GitHub: githubapp.DefaultLimits(), MaxHeaderBytes: 32 << 10, MaxActiveRequests: 64, MaxActiveConnections: 128, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 8 * time.Second, PerRequestIntakeTimeout: 8 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: 30 * time.Second, ShutdownTimeout: 9 * time.Second, MaxPathBytes: 4096}
}

func validateLimits(l Limits) error {
	if l.GitHub == (githubapp.Limits{}) {
		l.GitHub = githubapp.DefaultLimits()
	}
	if l.MaxHeaderBytes <= 0 || l.MaxHeaderBytes > 32<<10 || l.MaxActiveRequests <= 0 || l.MaxActiveRequests > 1024 || l.MaxActiveConnections <= 0 || l.MaxActiveConnections > 4096 || l.ReadHeaderTimeout <= 0 || l.ReadTimeout <= 0 || l.PerRequestIntakeTimeout <= 0 || l.WriteTimeout <= 0 || l.IdleTimeout <= 0 || l.ShutdownTimeout <= 0 || l.MaxPathBytes <= 0 || l.MaxPathBytes > 4096 {
		return errCode(CodeInvalidConfig, "limits", "limits rejected", nil)
	}
	return nil
}
