package githubinbox

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
)

func FuzzDecodeInboxRecord(f *testing.F) {
	f.Add(`{"receiverId":"receiver-1","deliveryId":"123","bodyDigest":"sha256:` + strings.Repeat("0", 64) + `"}`)
	f.Add(`{}`)
	f.Fuzz(func(t *testing.T, data string) {
		if len(data) > 1<<16 {
			t.Skip()
		}
		var v map[string]string
		_ = json.Unmarshal([]byte(data), &v)
	})
}

func FuzzDecideInboxAcceptance(f *testing.F) {
	f.Add("receiver-1", "123e4567-e89b-12d3-a456-426614174000", "pull_request", githubapp.DigestRawBody([]byte("{}")))
	f.Add("bad", "delivery", "issue_comment", "sha256:"+strings.Repeat("0", 64))
	f.Fuzz(func(t *testing.T, receiver, delivery, event, digest string) {
		if len(receiver)+len(delivery)+len(event)+len(digest) > 4096 {
			t.Skip()
		}
		fp := ComputeIntakeFingerprint(receiver, delivery, event, digest, string(githubapp.ProjectionIgnored))
		_ = VerifiedDelivery{ReceiverID: receiver, DeliveryID: delivery, Event: event, BodyDigest: digest, Projection: githubapp.WebhookProjection{Kind: githubapp.ProjectionIgnored}, Receipt: githubapp.DeliveryReceipt{SchemaVersion: githubapp.SchemaGitHubWebhookReceiptV1Alpha1, ReceiverID: receiver, DeliveryID: delivery, Event: event, BodyDigest: digest, ProjectionKind: githubapp.ProjectionIgnored, ReceivedAt: time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC), MatchedSecret: githubapp.SecretGenerationCurrent, Disposition: githubapp.DeliveryDispositionIgnored}, MatchedSecret: githubapp.SecretGenerationCurrent, ReceivedAt: time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC), Disposition: githubapp.DeliveryDispositionIgnored, IntakeFingerprint: fp}
	})
}

func FuzzTransitionOutboxLease(f *testing.F) {
	f.Add("controller-1", int64(60), 1)
	f.Add("", int64(-1), 0)
	f.Fuzz(func(t *testing.T, owner string, seconds int64, limit int) {
		if len(owner) > 1024 {
			t.Skip()
		}
		s := &Store{limits: DefaultLimits()}
		_ = s.validateLeaseInputs(LeaseOwner(owner), time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC), time.Duration(seconds)*time.Second, limit)
	})
}
