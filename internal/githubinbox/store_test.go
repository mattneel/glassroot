package githubinbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapp"
)

func TestStoreAcceptReplayAndOutbox(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, "receiver-1")
	d := testDelivery("123e4567-e89b-12d3-a456-426614174000", "pull_request", githubapp.ProjectionPullRequest, githubapp.DeliveryDispositionEnqueued)
	res, err := store.Accept(ctx, d)
	if err != nil || res.Decision != AcceptNewEnqueued {
		t.Fatalf("first accept decision=%#v err=%v", res, err)
	}
	if got := countRows(t, store, "github_inbox"); got != 1 {
		t.Fatalf("inbox rows=%d", got)
	}
	if got := countRows(t, store, "github_outbox"); got != 1 {
		t.Fatalf("outbox rows=%d", got)
	}
	var createdAt string
	if err := store.db.QueryRow(`SELECT created_at FROM github_inbox WHERE receiver_id=? AND delivery_id=?`, d.ReceiverID, d.DeliveryID).Scan(&createdAt); err != nil {
		t.Fatalf("created_at: %v", err)
	}
	if createdAt != formatTime(d.ReceivedAt) {
		t.Fatalf("created_at=%q want explicit received_at %q", createdAt, formatTime(d.ReceivedAt))
	}
	res, err = store.Accept(ctx, d)
	if err != nil || res.Decision != AcceptDuplicateSameDelivery {
		t.Fatalf("duplicate decision=%#v err=%v", res, err)
	}
	if got := countRows(t, store, "github_outbox"); got != 1 {
		t.Fatalf("duplicate inserted outbox rows=%d", got)
	}
	conflict := d
	conflict.Event = "check_run"
	conflict.Receipt.Event = "check_run"
	conflict.IntakeFingerprint = ComputeIntakeFingerprint(conflict.ReceiverID, conflict.DeliveryID, conflict.Event, conflict.BodyDigest, string(conflict.Projection.Kind))
	res, err = store.Accept(ctx, conflict)
	if err == nil || !errors.Is(err, ErrCode(CodeDeliveryConflict)) || res.Decision != AcceptDeliveryConflict {
		t.Fatalf("conflict decision=%#v err=%v", res, err)
	}
	if got := countRows(t, store, "github_outbox"); got != 1 {
		t.Fatalf("conflict inserted outbox rows=%d", got)
	}
}

func TestStorePersistsIgnoredWithoutOutboxAndNoRawProse(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, "receiver-1")
	d := testDelivery("223e4567-e89b-12d3-a456-426614174000", "check_suite", githubapp.ProjectionCheckSuite, githubapp.DeliveryDispositionIgnored)
	d.RawBodyCanaryForTest = "Never retain me feature/branch https://example.invalid"
	res, err := store.Accept(ctx, d)
	if err != nil || res.Decision != AcceptNewIgnored {
		t.Fatalf("ignored decision=%#v err=%v", res, err)
	}
	if got := countRows(t, store, "github_inbox"); got != 1 {
		t.Fatalf("inbox rows=%d", got)
	}
	if got := countRows(t, store, "github_outbox"); got != 0 {
		t.Fatalf("outbox rows=%d", got)
	}
	blob := dumpDatabaseText(t, store)
	for _, forbidden := range []string{"Never retain me", "feature/branch", "https://", "sha256="} {
		if strings.Contains(blob, forbidden) {
			t.Fatalf("database retained forbidden %q in %s", forbidden, blob)
		}
	}
}

func TestStoreOutboxLeaseStateMachine(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, "receiver-1")
	for i := 0; i < 3; i++ {
		d := testDelivery(string(rune('a'+i))+"23e4567-e89b-12d3-a456-426614174000", "pull_request", githubapp.ProjectionPullRequest, githubapp.DeliveryDispositionEnqueued)
		if _, err := store.Accept(ctx, d); err != nil {
			t.Fatalf("accept %d: %v", i, err)
		}
	}
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	leased, err := store.ClaimOutbox(ctx, LeaseOwner("controller-1"), now, time.Minute, 2)
	if err != nil || len(leased) != 2 {
		t.Fatalf("claim len=%d err=%v", len(leased), err)
	}
	if leased[0].Sequence >= leased[1].Sequence {
		t.Fatalf("claim order not durable: %#v", leased)
	}
	if err := store.AcknowledgeOutbox(ctx, leased[0].ID, LeaseOwner("wrong"), leased[0].LeaseGeneration, now); !errors.Is(err, ErrCode(CodeStaleLease)) {
		t.Fatalf("wrong owner ack err=%v", err)
	}
	if err := store.AcknowledgeOutbox(ctx, leased[0].ID, LeaseOwner("controller-1"), leased[0].LeaseGeneration, now); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if err := store.AcknowledgeOutbox(ctx, leased[0].ID, LeaseOwner("controller-1"), leased[0].LeaseGeneration, now); err != nil {
		t.Fatalf("idempotent ack: %v", err)
	}
	if err := store.ReleaseOutbox(ctx, leased[1].ID, LeaseOwner("controller-1"), leased[1].LeaseGeneration, now, "retry"); err != nil {
		t.Fatalf("release: %v", err)
	}
	leased2, err := store.ClaimOutbox(ctx, LeaseOwner("controller-2"), now.Add(2*time.Minute), time.Minute, 10)
	if err != nil {
		t.Fatalf("claim2: %v", err)
	}
	if len(leased2) != 2 {
		t.Fatalf("claim2 len=%d want 2", len(leased2))
	}
	for _, rec := range leased2 {
		if rec.ID == leased[0].ID {
			t.Fatalf("acknowledged record reclaimed")
		}
	}
}

func TestStoreConcurrentClaimersReceiveDisjointRecords(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, "receiver-1")
	for i := 0; i < 8; i++ {
		d := testDelivery(string(rune('a'+i))+"23e4567-e89b-12d3-a456-426614174000", "pull_request", githubapp.ProjectionPullRequest, githubapp.DeliveryDispositionEnqueued)
		if _, err := store.Accept(ctx, d); err != nil {
			t.Fatalf("accept %d: %v", i, err)
		}
	}
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	start := make(chan struct{})
	results := make(chan []LeasedRecord, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			leased, err := store.ClaimOutbox(ctx, LeaseOwner("controller-"+string(rune('a'+i))), now, time.Minute, 3)
			if err != nil {
				t.Errorf("claim %d: %v", i, err)
				return
			}
			results <- leased
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for leased := range results {
		for _, rec := range leased {
			if seen[rec.ID] {
				t.Fatalf("record claimed by multiple claimers: %s", rec.ID)
			}
			seen[rec.ID] = true
		}
	}
}

func TestStoreConcurrentIdenticalAcceptsCreateOneOutbox(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t, "receiver-1")
	d := testDelivery("323e4567-e89b-12d3-a456-426614174000", "pull_request", githubapp.ProjectionPullRequest, githubapp.DeliveryDispositionEnqueued)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = store.Accept(ctx, d) }()
	}
	wg.Wait()
	if got := countRows(t, store, "github_inbox"); got != 1 {
		t.Fatalf("inbox rows=%d", got)
	}
	if got := countRows(t, store, "github_outbox"); got != 1 {
		t.Fatalf("outbox rows=%d", got)
	}
}

func TestOpenValidatesReceiverAndDatabaseIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := Open(ctx, Config{StateDir: dir, ReceiverID: "receiver-1", Limits: DefaultLimits()})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := Open(ctx, Config{StateDir: dir, ReceiverID: "other", Limits: DefaultLimits()}); !errors.Is(err, ErrCode(CodeDatabaseReceiverMismatch)) {
		t.Fatalf("mismatch err=%v", err)
	}
	info, err := filepath.Glob(filepath.Join(dir, "github-webhook.sqlite*"))
	if err != nil || len(info) == 0 {
		t.Fatalf("db files not created: %v %v", info, err)
	}
}

func TestStoreReopenPreservesPendingOutbox(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := Open(ctx, Config{StateDir: dir, ReceiverID: "receiver-1", Limits: DefaultLimits()})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	d := testDelivery("423e4567-e89b-12d3-a456-426614174000", "pull_request", githubapp.ProjectionPullRequest, githubapp.DeliveryDispositionEnqueued)
	if _, err := store.Accept(ctx, d); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	reopened, err := Open(ctx, Config{StateDir: dir, ReceiverID: "receiver-1", Limits: DefaultLimits()})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	leased, err := reopened.ClaimOutbox(ctx, LeaseOwner("controller-1"), time.Date(2026, 6, 23, 12, 1, 0, 0, time.UTC), time.Minute, 10)
	if err != nil {
		t.Fatalf("claim after reopen: %v", err)
	}
	if len(leased) != 1 || leased[0].DeliveryID != d.DeliveryID {
		t.Fatalf("leased after reopen=%#v", leased)
	}
}

func openTestStore(t *testing.T, receiverID string) *Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(context.Background(), Config{StateDir: dir, ReceiverID: receiverID, Limits: DefaultLimits()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testDelivery(deliveryID, event string, kind githubapp.ProjectionKind, disposition githubapp.DeliveryDisposition) VerifiedDelivery {
	bodyDigest := githubapp.DigestRawBody([]byte(`{"minimal":true}`))
	projection := githubapp.WebhookProjection{Kind: kind}
	if kind == githubapp.ProjectionPullRequest {
		projection.PullRequest = &githubapp.PullRequestProjection{Action: "opened", InstallationID: 42, RepositoryID: 101, RepositoryOwnerID: 201, PullRequestNumber: 7, BaseSHA: strings.Repeat("1", 40), HeadSHA: strings.Repeat("2", 40), HeadRepositoryID: 202}
	}
	return VerifiedDelivery{ReceiverID: "receiver-1", DeliveryID: deliveryID, Event: event, Action: "opened", BodyDigest: bodyDigest, MatchedSecret: githubapp.SecretGenerationCurrent, ReceivedAt: time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC), Projection: projection, Receipt: githubapp.DeliveryReceipt{SchemaVersion: githubapp.SchemaGitHubWebhookReceiptV1Alpha1, ReceiverID: "receiver-1", DeliveryID: deliveryID, Event: event, BodyDigest: bodyDigest, MatchedSecret: githubapp.SecretGenerationCurrent, ReceivedAt: time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC), ProjectionKind: kind, Disposition: disposition}, Disposition: disposition, IntakeFingerprint: ComputeIntakeFingerprint("receiver-1", deliveryID, event, bodyDigest, string(kind))}
}

func countRows(t *testing.T, store *Store, table string) int {
	t.Helper()
	var n int
	if err := store.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func dumpDatabaseText(t *testing.T, store *Store) string {
	t.Helper()
	rows, err := store.db.Query(`SELECT receipt_json || ' ' || projection_json || ' ' || COALESCE(action,'') || ' ' || event_name FROM github_inbox`)
	if err != nil {
		t.Fatalf("dump: %v", err)
	}
	defer rows.Close()
	var out strings.Builder
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out.WriteString(s)
	}
	return out.String()
}
