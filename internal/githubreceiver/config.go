package githubreceiver

import (
	"context"
	"log/slog"
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
	"github.com/mattneel/glassroot/internal/githubinbox"
)

type Clock interface{ Now() time.Time }

type Store interface {
	Accept(context.Context, githubinbox.VerifiedDelivery) (githubinbox.AcceptResult, error)
}

type HandlerConfig struct {
	ReceiverID string
	Store      Store
	Secrets    githubapp.WebhookSecrets
	Limits     Limits
	Clock      Clock
	Logger     *slog.Logger
}

type ServerConfig struct {
	ListenUnix string
	StateDir   string
	ReceiverID string
	Secrets    githubapp.WebhookSecrets
	Store      Store
	Limits     Limits
	Clock      Clock
	Logger     *slog.Logger
}
