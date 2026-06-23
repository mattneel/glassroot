package githubapp

import (
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/model"
)

func FuzzParseGitHubSignatureHeader(f *testing.F) {
	f.Add("sha256=" + strings.Repeat("a", 64))
	f.Add("")
	f.Add("sha1=" + strings.Repeat("a", 40))
	f.Add("sha256=" + strings.Repeat("A", 64))
	f.Fuzz(func(t *testing.T, header string) {
		if len(header) > 1024 {
			header = header[:1024]
		}
		_, _ = ParseGitHubSignatureHeader(header, DefaultLimits())
	})
}

func FuzzPreflightGitHubWebhookJSON(f *testing.F) {
	f.Add([]byte(validPullRequestPayload("opened")))
	f.Add([]byte(`{"a":1,"a":2}`))
	f.Add([]byte("\xef\xbb\xbf{}"))
	f.Add([]byte("{\"nul\":\"\u0000\"}"))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1<<20 {
			body = body[:1<<20]
		}
		_ = PreflightGitHubWebhookJSON(body, DefaultLimits())
	})
}

func FuzzProjectGitHubWebhook(f *testing.F) {
	f.Add("pull_request", []byte(validPullRequestPayload("opened")))
	f.Add("check_run", []byte(validCheckRunPayload()))
	f.Add("issue_comment", []byte(`{"action":"created"}`))
	f.Fuzz(func(t *testing.T, event string, body []byte) {
		if len(event) > 128 || len(body) > 1<<20 {
			return
		}
		_, _ = ProjectWebhook(event, body, DefaultLimits())
	})
}

func FuzzDecideWebhookReplay(f *testing.F) {
	f.Add("delivery-123", "{}", "{}")
	f.Add("123e4567-e89b-12d3-a456-426614174000", "{}", `{"changed":true}`)
	f.Fuzz(func(t *testing.T, delivery, first, second string) {
		if len(delivery) > 256 || len(first) > 4096 || len(second) > 4096 {
			return
		}
		receipt := DeliveryReceipt{SchemaVersion: SchemaGitHubWebhookReceiptV1Alpha1, ReceiverID: "receiver", DeliveryID: delivery, Event: "pull_request", BodyDigest: DigestRawBody([]byte(first)), MatchedSecret: SecretGenerationCurrent, ProjectionKind: ProjectionPullRequest, Disposition: DeliveryDispositionEnqueued}
		_, record, _ := DecideWebhookReplay(nil, receipt)
		if record != nil {
			receipt.BodyDigest = DigestRawBody([]byte(second))
			_, _, _ = DecideWebhookReplay(record, receipt)
		}
	})
}

func FuzzEncodeGitHubAnalysisTarget(f *testing.F) {
	f.Add(int64(42), int64(101), int64(202), int64(7), strings.Repeat("1", 40), strings.Repeat("2", 40))
	f.Add(int64(0), int64(-1), int64(1<<62), int64(-3), "HEAD", strings.Repeat("A", 40))
	f.Fuzz(func(t *testing.T, installation, baseRepo, headRepo, pr int64, base, head string) {
		if len(base) > 128 || len(head) > 128 {
			return
		}
		target := AnalysisTarget{SchemaVersion: SchemaGitHubAnalysisTargetV1Alpha1, InstallationID: installation, BaseRepositoryID: baseRepo, HeadRepositoryID: headRepo, PullRequestNumber: pr, BaseCommitID: base, HeadCommitID: head, AnalysisProfileVersion: "glassroot.dev/analysis-profile/public-pr/v1alpha1"}
		_, _ = target.ID()
	})
}

func FuzzProjectAdvisoryCheck(f *testing.F) {
	f.Add(strings.Repeat("2", 40), "attempt-"+strings.Repeat("a", 64), "target-"+strings.Repeat("b", 64), int64(1), string(model.DispositionPassed), true)
	f.Add("HEAD", "bad", "bad", int64(-1), "weird", false)
	f.Fuzz(func(t *testing.T, head, attempt, target string, generation int64, disposition string, complete bool) {
		if len(head) > 128 || len(attempt) > 256 || len(target) > 256 || len(disposition) > 128 {
			return
		}
		_, _ = ProjectAdvisoryCheck(CheckProjectionInput{RepositoryID: 101, HeadSHA: head, AttemptID: attempt, TargetID: target, Generation: generation, PolicyDisposition: model.Disposition(disposition), EvidenceComplete: complete, RunnerTier: RunnerTierHardenedContainer, Status: CheckStatusCompleted})
	})
}
