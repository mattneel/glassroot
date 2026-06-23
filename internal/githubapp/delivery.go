package githubapp

import "time"

type DeliveryDisposition string

const (
	DeliveryDispositionEnqueued  DeliveryDisposition = "enqueued"
	DeliveryDispositionDuplicate DeliveryDisposition = "duplicate"
	DeliveryDispositionIgnored   DeliveryDisposition = "ignored"
	DeliveryDispositionRejected  DeliveryDisposition = "rejected"
)

type DeliveryReceipt struct {
	SchemaVersion              string              `json:"schemaVersion"`
	ReceiverID                 string              `json:"receiverId"`
	DeliveryID                 string              `json:"deliveryId"`
	Event                      string              `json:"event"`
	BodyDigest                 string              `json:"bodyDigest"`
	MatchedSecret              SecretGeneration    `json:"matchedSecret"`
	ReceivedAt                 time.Time           `json:"receivedAt"`
	InstallationTargetIdentity string              `json:"installationTargetIdentity,omitempty"`
	ProjectionKind             ProjectionKind      `json:"projectionKind"`
	Disposition                DeliveryDisposition `json:"disposition"`
}

type DeliveryInboxRecord struct {
	ReceiverID string        `json:"receiverId"`
	DeliveryID string        `json:"deliveryId"`
	BodyDigest string        `json:"bodyDigest"`
	State      DeliveryState `json:"state"`
}

type ReplayDecision string

const (
	ReplayNew               ReplayDecision = "new"
	ReplayDuplicateSameBody ReplayDecision = "duplicate-same-body"
	ReplayDeliveryConflict  ReplayDecision = "delivery-conflict"
)

func DecideWebhookReplay(existing *DeliveryInboxRecord, incoming DeliveryReceipt) (ReplayDecision, *DeliveryInboxRecord, error) {
	if incoming.ReceiverID == "" || incoming.DeliveryID == "" || !validateDigest(incoming.BodyDigest) {
		return "", nil, errCode(CodeProjectionInvalid, "replay", "receipt identity invalid", nil)
	}
	if existing == nil {
		return ReplayNew, &DeliveryInboxRecord{ReceiverID: incoming.ReceiverID, DeliveryID: incoming.DeliveryID, BodyDigest: incoming.BodyDigest, State: DeliveryStatePersisted}, nil
	}
	if existing.ReceiverID != incoming.ReceiverID || existing.DeliveryID != incoming.DeliveryID {
		return "", nil, errCode(CodeDeliveryConflict, "replay", "replay key mismatch", nil)
	}
	if existing.BodyDigest == incoming.BodyDigest {
		copy := *existing
		return ReplayDuplicateSameBody, &copy, nil
	}
	copy := *existing
	return ReplayDeliveryConflict, &copy, errCode(CodeDeliveryConflict, "replay", "same delivery id had different body digest", nil)
}
