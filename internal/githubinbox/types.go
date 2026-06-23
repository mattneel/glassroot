package githubinbox

import (
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
)

const (
	SchemaInboxStoreV1Alpha1         = "glassroot.dev/github-inbox-store/v1alpha1"
	SchemaInboxRecordV1Alpha1        = "glassroot.dev/github-inbox-record/v1alpha1"
	SchemaControllerEnvelopeV1Alpha1 = "glassroot.dev/github-controller-envelope/v1alpha1"
)

type Config struct {
	StateDir   string
	ReceiverID string
	Limits     Limits
}

type VerifiedDelivery struct {
	ReceiverID           string
	DeliveryID           string
	Event                string
	Action               string
	BodyDigest           string
	MatchedSecret        githubapp.SecretGeneration
	ReceivedAt           time.Time
	Projection           githubapp.WebhookProjection
	Receipt              githubapp.DeliveryReceipt
	Disposition          githubapp.DeliveryDisposition
	IntakeFingerprint    string
	RawBodyCanaryForTest string `json:"-"`
}

type AcceptDecision string

const (
	AcceptNewEnqueued           AcceptDecision = "new-enqueued"
	AcceptNewIgnored            AcceptDecision = "new-ignored"
	AcceptDuplicateSameDelivery AcceptDecision = "duplicate-same-delivery"
	AcceptDeliveryConflict      AcceptDecision = "delivery-conflict"
)

type AcceptResult struct {
	Decision AcceptDecision
	OutboxID string
}

type LeaseOwner string

type OutboxState string

const (
	OutboxStatePending      OutboxState = "pending"
	OutboxStateLeased       OutboxState = "leased"
	OutboxStateAcknowledged OutboxState = "acknowledged"
)

type LeasedRecord struct {
	ID               string
	ReceiverID       string
	DeliveryID       string
	Sequence         int64
	ProjectionKind   githubapp.ProjectionKind
	Projection       githubapp.WebhookProjection
	ProjectionDigest string
	Receipt          githubapp.DeliveryReceipt
	LeaseOwner       LeaseOwner
	LeaseGeneration  uint64
	AttemptCount     uint64
	CreatedAt        time.Time
	LeaseExpiresAt   time.Time
}
