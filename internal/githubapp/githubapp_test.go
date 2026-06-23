package githubapp

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/model"
)

func TestPermissionProfileExactInventory(t *testing.T) {
	profile := DefaultPermissionProfile()
	if profile.SchemaVersion != SchemaGitHubAppPermissionsAdvisoryV1Alpha1 {
		t.Fatalf("schema = %q", profile.SchemaVersion)
	}
	want := map[string]string{
		"checks":        "write",
		"contents":      "read",
		"pull_requests": "read",
		"metadata":      "read",
	}
	if len(profile.RepositoryPermissions) != len(want) {
		t.Fatalf("requested permissions = %#v", profile.RepositoryPermissions)
	}
	for _, p := range profile.RepositoryPermissions {
		if want[p.Name] != p.Access {
			t.Fatalf("unexpected permission %#v", p)
		}
		delete(want, p.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing permissions: %#v", want)
	}
	absent := map[string]bool{}
	for _, p := range profile.AbsentRepositoryPermissions {
		absent[p] = true
	}
	for _, name := range []string{"actions", "administration", "commit_statuses", "issues", "workflows", "secret_scanning", "secrets"} {
		if !absent[name] {
			t.Fatalf("absent permission %q not listed", name)
		}
	}
	if len(profile.OrganizationPermissions) != 0 || profile.UserAuthorization != "disabled" {
		t.Fatalf("unexpected non-repository access: %#v", profile)
	}
}

func TestWebhookSubscriptionActionMatrix(t *testing.T) {
	profile := DefaultWebhookProfile()
	if profile.SchemaVersion != SchemaGitHubAppWebhooksAdvisoryV1Alpha1 {
		t.Fatalf("schema = %q", profile.SchemaVersion)
	}
	cases := []struct {
		event, action string
		want          WebhookActionDecision
	}{
		{"pull_request", "opened", WebhookActionSchedule},
		{"pull_request", "reopened", WebhookActionSchedule},
		{"pull_request", "synchronize", WebhookActionSchedule},
		{"pull_request", "ready_for_review", WebhookActionSchedule},
		{"pull_request", "converted_to_draft", WebhookActionCancel},
		{"pull_request", "closed", WebhookActionCancel},
		{"pull_request", "edited", WebhookActionNoop},
		{"check_run", "rerequested", WebhookActionRerequest},
		{"check_run", "created", WebhookActionNoop},
		{"check_suite", "requested", WebhookActionNoop},
		{"ping", "", WebhookActionNoop},
		{"issue_comment", "created", WebhookActionUnsupported},
		{"pull_request", "unknown_new_action", WebhookActionNoop},
	}
	for _, tc := range cases {
		got, err := ClassifyWebhookAction(tc.event, tc.action)
		if tc.want == WebhookActionUnsupported {
			if err == nil || got != WebhookActionUnsupported {
				t.Fatalf("%s/%s got %q err %v", tc.event, tc.action, got, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%s/%s got %q err %v", tc.event, tc.action, got, err)
		}
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	limits := DefaultLimits()
	limits.MinWebhookSecretBytes = 1
	body := []byte("Hello, World!")
	secrets := WebhookSecrets{Current: []byte("It's a Secret to Everybody"), Previous: []byte(strings.Repeat("p", 32))}
	gen, err := VerifyWebhookSignature(body, "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17", secrets, limits)
	if err != nil || gen != SecretGenerationCurrent {
		t.Fatalf("official vector got gen=%q err=%v", gen, err)
	}
	prevSig := signWebhookForTest(body, secrets.Previous)
	gen, err = VerifyWebhookSignature(body, prevSig, secrets, limits)
	if err != nil || gen != SecretGenerationPrevious {
		t.Fatalf("previous got gen=%q err=%v", gen, err)
	}
	mutated := append([]byte(nil), body...)
	mutated[0] = 'h'
	if _, err := VerifyWebhookSignature(mutated, "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17", secrets, limits); !errors.Is(err, ErrCode(CodeSignatureMismatch)) {
		t.Fatalf("mutated body err=%v", err)
	}
	badHeaders := []string{
		"",
		"sha1=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17",
		"sha256:757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17",
		"sha256=757107EA0EB2509FC211221CCE984B8A37570B6D7586C22C46F4379C8B043E17",
		"sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e1",
		" sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17",
		"sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17 extra",
	}
	for _, h := range badHeaders {
		if _, err := VerifyWebhookSignature(body, h, secrets, limits); err == nil {
			t.Fatalf("bad header %q accepted", h)
		}
	}
	if _, err := VerifyWebhookSignature(bytesOf(limits.MaxWebhookBodyBytes+1), signWebhookForTest([]byte("x"), secrets.Current), secrets, limits); !errors.Is(err, ErrCode(CodeBodyTooLarge)) {
		t.Fatalf("large body err=%v", err)
	}
	canary := "super-secret-canary-super-secret-canary"
	_, err = VerifyWebhookSignature(body, "sha256="+strings.Repeat("0", 64), WebhookSecrets{Current: []byte(canary)}, limits)
	if err == nil || strings.Contains(err.Error(), canary) || strings.Contains(err.Error(), string(body)) {
		t.Fatalf("error leaked secret/body or missed mismatch: %v", err)
	}
}

func TestWebhookHeaders(t *testing.T) {
	limits := DefaultLimits()
	headers := []HeaderValue{
		{Name: "X-GitHub-Delivery", Value: "123e4567-e89b-12d3-a456-426614174000"},
		{Name: "X-GitHub-Event", Value: "pull_request"},
		{Name: "X-Hub-Signature-256", Value: "sha256=" + strings.Repeat("a", 64)},
		{Name: "Content-Type", Value: "application/json; charset=utf-8"},
		{Name: "Content-Encoding", Value: "identity"},
	}
	got, err := ParseWebhookHeaders(headers, limits)
	if err != nil {
		t.Fatalf("valid headers: %v", err)
	}
	if got.DeliveryID != "123e4567-e89b-12d3-a456-426614174000" || got.Event != "pull_request" || got.ContentType != "application/json" || got.Charset != "utf-8" {
		t.Fatalf("unexpected headers %#v", got)
	}
	dup := append(append([]HeaderValue(nil), headers...), HeaderValue{Name: "x-github-delivery", Value: "123e4567-e89b-12d3-a456-426614174001"})
	if _, err := ParseWebhookHeaders(dup, limits); !errors.Is(err, ErrCode(CodeDuplicateRequiredHeader)) {
		t.Fatalf("duplicate err=%v", err)
	}
	bad := append([]HeaderValue(nil), headers...)
	bad[3].Value = "text/plain"
	if _, err := ParseWebhookHeaders(bad, limits); !errors.Is(err, ErrCode(CodeInvalidContentType)) {
		t.Fatalf("content-type err=%v", err)
	}
	bad = append([]HeaderValue(nil), headers...)
	bad = append(bad, HeaderValue{Name: "Content-Encoding", Value: "gzip"})
	if _, err := ParseWebhookHeaders(bad, limits); !errors.Is(err, ErrCode(CodeUnsupportedContentEncoding)) {
		t.Fatalf("encoding err=%v", err)
	}
}

func TestJSONPreflightAndProjection(t *testing.T) {
	limits := DefaultLimits()
	if err := PreflightGitHubWebhookJSON([]byte(validPullRequestPayload("opened")), limits); err != nil {
		t.Fatalf("valid json: %v", err)
	}
	badJSON := [][]byte{
		[]byte("\xef\xbb\xbf{}"),
		[]byte("{\"a\":1}\n{\"b\":2}"),
		[]byte("{\"a\":1,\"a\":2}"),
		[]byte("{\"a\":{\"b\":1,\"b\":2}}"),
		[]byte("{\"a\":1,\"\\u0061\":2}"),
		[]byte("{\"nul\":\"\u0000\"}"),
		[]byte("[]"),
	}
	for _, js := range badJSON {
		if err := PreflightGitHubWebhookJSON(js, limits); err == nil {
			t.Fatalf("bad json accepted: %q", js)
		}
	}
	projection, err := ProjectWebhook("pull_request", []byte(validPullRequestPayload("synchronize")), limits)
	if err != nil {
		t.Fatalf("project pull_request: %v", err)
	}
	if projection.Kind != ProjectionPullRequest || projection.PullRequest == nil {
		t.Fatalf("bad projection %#v", projection)
	}
	pr := projection.PullRequest
	if pr.Action != "synchronize" || pr.InstallationID != 42 || pr.RepositoryID != 101 || pr.RepositoryOwnerID != 201 || pr.PullRequestNumber != 7 || pr.HeadSHA != strings.Repeat("2", 40) || pr.BaseSHA != strings.Repeat("1", 40) || pr.HeadRepositoryID != 202 {
		t.Fatalf("unexpected pr projection %#v", pr)
	}
	encoded, _ := json.Marshal(projection)
	for _, forbidden := range []string{"Never retain me", "feature/prose", "https://", "do not run"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("projection retained forbidden field %q in %s", forbidden, encoded)
		}
	}
	if pr.BaseRepositoryOwner != "octocat" || pr.BaseRepositoryName != "repo" {
		t.Fatalf("missing bounded route hints %#v", pr)
	}
	projection, err = ProjectWebhook("check_run", []byte(validCheckRunPayload()), limits)
	if err != nil || projection.CheckRun == nil || projection.CheckRun.Action != "rerequested" || projection.CheckRun.CheckRunID != 555 || projection.CheckRun.AppID != 999 || projection.CheckRun.ExternalID != "gr-"+strings.Repeat("a", 64) {
		t.Fatalf("check_run projection=%#v err=%v", projection, err)
	}
	projection, err = ProjectWebhook("ping", []byte(`{"zen":"Approachable is better than simple."}`), limits)
	if err != nil || projection.Kind != ProjectionPing {
		t.Fatalf("ping projection=%#v err=%v", projection, err)
	}
	if _, err := ProjectWebhook("issue_comment", []byte(`{"action":"created"}`), limits); !errors.Is(err, ErrCode(CodeUnsupportedEvent)) {
		t.Fatalf("unsupported event err=%v", err)
	}
}

func TestDeliveryReplayDecisions(t *testing.T) {
	receivedAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	receipt := DeliveryReceipt{SchemaVersion: SchemaGitHubWebhookReceiptV1Alpha1, ReceiverID: "receiver-1", DeliveryID: "123e4567-e89b-12d3-a456-426614174000", Event: "pull_request", BodyDigest: DigestRawBody([]byte("{}")), MatchedSecret: SecretGenerationCurrent, ReceivedAt: receivedAt, ProjectionKind: ProjectionPullRequest, Disposition: DeliveryDispositionEnqueued}
	decision, record, err := DecideWebhookReplay(nil, receipt)
	if err != nil || decision != ReplayNew || record == nil {
		t.Fatalf("new decision=%q record=%#v err=%v", decision, record, err)
	}
	decision, _, err = DecideWebhookReplay(record, receipt)
	if err != nil || decision != ReplayDuplicateSameBody {
		t.Fatalf("duplicate decision=%q err=%v", decision, err)
	}
	conflict := receipt
	conflict.BodyDigest = DigestRawBody([]byte("{\"changed\":true}"))
	decision, _, err = DecideWebhookReplay(record, conflict)
	if decision != ReplayDeliveryConflict || !errors.Is(err, ErrCode(CodeDeliveryConflict)) {
		t.Fatalf("conflict decision=%q err=%v", decision, err)
	}
	if receipt.ReceivedAt.IsZero() || strings.Contains(record.BodyDigest, "changed") {
		t.Fatalf("bad receipt/record %#v %#v", receipt, record)
	}
}

func TestAnalysisTargetJobAttemptAndExternalIDDeterminism(t *testing.T) {
	target := sampleTarget()
	id, err := target.ID()
	if err != nil {
		t.Fatalf("target id: %v", err)
	}
	if !strings.HasPrefix(id, "target-") || len(id) != len("target-")+64 {
		t.Fatalf("bad target id %q", id)
	}
	if id != "target-7faa664e986a8a2668778636e7c87e0f667a9e4118349baaed433f308dabe41d" {
		t.Fatalf("target id changed: %s", id)
	}
	job, err := NewAnalysisJob(target, 3, "glassroot.dev/analysis-profile/public-pr/v1alpha1", RunnerTierHardenedContainer)
	if err != nil {
		t.Fatalf("job: %v", err)
	}
	if job.ID != "job-f931946228486539359d4f078a2483801acc67e66e54239f29f97d533fd83ecd" {
		t.Fatalf("job id changed: %s", job.ID)
	}
	attempt, err := NewAnalysisAttempt(job, 2, AttemptReasonCheckRerequest)
	if err != nil {
		t.Fatalf("attempt: %v", err)
	}
	if attempt.ID != "attempt-de0a0d9f8c3d9417baa570f0761fb08b0f484e922d7aabed4959bfbd0d7166c6" {
		t.Fatalf("attempt id changed: %s", attempt.ID)
	}
	external, err := CheckExternalID("github-app-123", 101, attempt.ID, CheckProfileAdvisoryV1Alpha1)
	if err != nil {
		t.Fatalf("external id: %v", err)
	}
	if external != "gr-fc9b408892cf99c5bd1bbd76af2dc91d3ceb5b413a2aadd38259c28690091abd" {
		t.Fatalf("external id changed: %s", external)
	}
	changed := target
	changed.HeadCommitID = strings.Repeat("3", 40)
	changedID, _ := changed.ID()
	if changedID == id {
		t.Fatalf("head change did not affect target id")
	}
}

func TestStateMachineTransitions(t *testing.T) {
	job := JobStateQueued
	for _, next := range []JobState{JobStateImportingSource, JobStatePlanning, JobStateAwaitingRunner, JobStateRunning, JobStateValidatingReport, JobStateReadyToPublish, JobStateCompleted} {
		var err error
		job, err = TransitionJob(job, next)
		if err != nil {
			t.Fatalf("job transition to %s: %v", next, err)
		}
	}
	if _, err := TransitionJob(job, JobStateRunning); !errors.Is(err, ErrCode(CodeInvalidStateTransition)) {
		t.Fatalf("terminal job transition err=%v", err)
	}
	if _, err := TransitionPublication(CheckPublicationCompleted, CheckPublicationInProgress); !errors.Is(err, ErrCode(CodeInvalidStateTransition)) {
		t.Fatalf("publication backward err=%v", err)
	}
	if _, err := TransitionAttempt(AttemptStateLeaseExpired, AttemptStateQueued); err != nil {
		t.Fatalf("lease-expired retry queue should be controller-policy representable: %v", err)
	}
}

func TestWorkerCredentialBoundariesAndEligibility(t *testing.T) {
	target := sampleTarget()
	job, err := NewAnalysisJob(target, 1, "glassroot.dev/analysis-profile/public-pr/v1alpha1", RunnerTierDockerDev)
	if err == nil || job.ID != "" {
		t.Fatalf("docker-dev public job accepted: job=%#v err=%v", job, err)
	}
	job, err = NewAnalysisJob(target, 1, "glassroot.dev/analysis-profile/public-pr/v1alpha1", RunnerTierHardenedContainer)
	if err != nil {
		t.Fatalf("hardened job: %v", err)
	}
	attempt, _ := NewAnalysisAttempt(job, 1, AttemptReasonInitial)
	assignment := WorkerAssignment{SchemaVersion: SchemaGitHubWorkerAssignmentV1Alpha1, AttemptID: attempt.ID, TargetID: job.TargetID, BaseCommitID: target.BaseCommitID, HeadCommitID: target.HeadCommitID, SourceStoreID: "store-" + strings.Repeat("a", 64), PlanDigest: "sha256:" + strings.Repeat("b", 64), RequiredRunnerTier: RunnerTierHardenedContainer, EvidenceOutputCapabilityID: "evidence-capability-1", ControllerGeneration: job.Generation, Limitations: []string{}}
	if err := ValidateWorkerAssignment(assignment); err != nil {
		t.Fatalf("assignment: %v", err)
	}
	encoded, _ := json.Marshal(assignment)
	for _, forbidden := range []string{"ghs_", "token", "secret", "private", "github.com", "https://"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("credential/url field leaked in assignment: %s", encoded)
		}
	}
	assignment.RequiredRunnerTier = RunnerTierDevelopmentOnly
	if err := ValidateWorkerAssignment(assignment); !errors.Is(err, ErrCode(CodeInvalidWorkerAssignment)) {
		t.Fatalf("development-only assignment err=%v", err)
	}
}

func TestAdvisoryCheckProjection(t *testing.T) {
	target := sampleTarget()
	job, _ := NewAnalysisJob(target, 1, "glassroot.dev/analysis-profile/public-pr/v1alpha1", RunnerTierHardenedContainer)
	attempt, _ := NewAnalysisAttempt(job, 1, AttemptReasonInitial)
	for _, disp := range []model.Disposition{model.DispositionPassed, model.DispositionRequiresReview, model.DispositionFailed} {
		projection, err := ProjectAdvisoryCheck(CheckProjectionInput{RepositoryID: target.BaseRepositoryID, HeadSHA: target.HeadCommitID, AttemptID: attempt.ID, TargetID: job.TargetID, Generation: job.Generation, PolicyDisposition: disp, EvidenceComplete: disp == model.DispositionPassed, RunnerTier: RunnerTierHardenedContainer, FindingCounts: FindingCounts{Total: 3, Failed: 1}, Status: CheckStatusCompleted})
		if err != nil {
			t.Fatalf("projection %s: %v", disp, err)
		}
		if projection.Name != "Glassroot advisory" || projection.Conclusion != CheckConclusionNeutral || projection.Status != CheckStatusCompleted {
			t.Fatalf("bad projection %#v", projection)
		}
		if projection.DetailsURL != "" || len(projection.Annotations) != 0 || len(projection.RequestedActions) != 0 {
			t.Fatalf("unsupported check features present %#v", projection)
		}
		body, _ := json.Marshal(projection)
		for _, forbidden := range []string{"success", "failure", "action_required", "feature/prose", "Never retain me", "logs", "artifact"} {
			if strings.Contains(string(body), forbidden) {
				t.Fatalf("projection contains forbidden %q: %s", forbidden, body)
			}
		}
		if !strings.Contains(projection.Output.Summary, string(disp)) || !strings.Contains(projection.Output.Summary, "not a safety proof") {
			t.Fatalf("summary missing disposition/not-proof text: %q", projection.Output.Summary)
		}
		again, _ := ProjectAdvisoryCheck(CheckProjectionInput{RepositoryID: target.BaseRepositoryID, HeadSHA: target.HeadCommitID, AttemptID: attempt.ID, TargetID: job.TargetID, Generation: job.Generation, PolicyDisposition: disp, EvidenceComplete: disp == model.DispositionPassed, RunnerTier: RunnerTierHardenedContainer, FindingCounts: FindingCounts{Total: 3, Failed: 1}, Status: CheckStatusCompleted})
		if projection.Digest() != again.Digest() {
			t.Fatalf("digest nondeterministic")
		}
	}
	cancelled, err := ProjectAdvisoryCheck(CheckProjectionInput{RepositoryID: target.BaseRepositoryID, HeadSHA: target.HeadCommitID, AttemptID: attempt.ID, TargetID: job.TargetID, Generation: job.Generation, PolicyDisposition: model.DispositionFailed, EvidenceComplete: false, RunnerTier: RunnerTierHardenedContainer, Status: CheckStatusCompleted, SupersededOrCancelled: true})
	if err != nil || cancelled.Conclusion != CheckConclusionCancelled {
		t.Fatalf("cancelled projection=%#v err=%v", cancelled, err)
	}
	if err := ValidatePublishCommand(PublishCommand{SchemaVersion: SchemaGitHubPublishCommandV1Alpha1, RepositoryID: target.BaseRepositoryID, HeadSHA: target.HeadCommitID, AttemptID: attempt.ID, TargetID: job.TargetID, Generation: job.Generation, Projection: cancelled}, job.Generation+1); !errors.Is(err, ErrCode(CodeStaleGeneration)) {
		t.Fatalf("stale publish err=%v", err)
	}
}

func sampleTarget() AnalysisTarget {
	return AnalysisTarget{SchemaVersion: SchemaGitHubAnalysisTargetV1Alpha1, InstallationID: 42, BaseRepositoryID: 101, HeadRepositoryID: 202, PullRequestNumber: 7, BaseCommitID: strings.Repeat("1", 40), HeadCommitID: strings.Repeat("2", 40), AnalysisProfileVersion: "glassroot.dev/analysis-profile/public-pr/v1alpha1"}
}

func validPullRequestPayload(action string) string {
	return `{
		"action": "` + action + `",
		"installation": {"id": 42},
		"repository": {"id": 101, "name": "repo", "full_name": "owner/repo", "owner": {"id": 201, "login": "octocat"}},
		"pull_request": {
			"number": 7,
			"title": "Never retain me",
			"body": "do not run",
			"draft": false,
			"merged": false,
			"state": "open",
			"base": {"sha": "` + strings.Repeat("1", 40) + `", "ref": "main", "repo": {"id": 101}},
			"head": {"sha": "` + strings.Repeat("2", 40) + `", "ref": "feature/prose", "repo": {"id": 202, "clone_url": "https://example.invalid/repo.git"}}
		},
		"sender": {"login":"octocat"}
	}`
}

func validCheckRunPayload() string {
	return `{
		"action": "rerequested",
		"installation": {"id": 42},
		"repository": {"id": 101},
		"check_run": {"id": 555, "head_sha": "` + strings.Repeat("2", 40) + `", "external_id": "gr-` + strings.Repeat("a", 64) + `", "app": {"id": 999}, "name": "Glassroot advisory"},
		"requested_action": {"identifier":"ignored"}
	}`
}

func bytesOf(n int) []byte {
	if n < 0 || n > 8<<20 {
		return nil
	}
	return []byte(strings.Repeat("x", n))
}

func signWebhookForTest(body, secret []byte) string {
	sum := hmacSHA256(body, secret)
	return "sha256=" + hex.EncodeToString(sum[:])
}
