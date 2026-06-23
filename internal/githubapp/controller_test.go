package githubapp

import (
	"errors"
	"strings"
	"testing"
)

func TestPullRequestReconciliationAndSupersession(t *testing.T) {
	base := strings.Repeat("1", 40)
	head1 := strings.Repeat("2", 40)
	head2 := strings.Repeat("3", 40)
	projection := PullRequestProjection{Action: "opened", InstallationID: 42, RepositoryID: 101, PullRequestNumber: 7, BaseSHA: base, HeadSHA: head1, HeadRepositoryID: 202}
	current := CurrentPullRequestState{InstallationID: 42, RepositoryID: 101, PullRequestNumber: 7, BaseRepositoryID: 101, HeadRepositoryID: 202, BaseSHA: base, HeadSHA: head1}
	decision, state, err := ReconcilePullRequestGeneration(PRGenerationState{}, projection, current, "glassroot.dev/analysis-profile/public-pr/v1alpha1")
	if err != nil || decision.Action != PRDecisionSchedule || state.Generation != 1 || !validateTargetID(decision.TargetID) {
		t.Fatalf("opened decision=%#v state=%#v err=%v", decision, state, err)
	}
	decision, same, err := ReconcilePullRequestGeneration(state, projection, current, "glassroot.dev/analysis-profile/public-pr/v1alpha1")
	if err != nil || decision.Action != PRDecisionNoop || same.Generation != state.Generation || same.CurrentTargetID != state.CurrentTargetID {
		t.Fatalf("duplicate decision=%#v state=%#v err=%v", decision, same, err)
	}
	oldProjection := projection
	oldProjection.Action = "synchronize"
	newCurrent := current
	newCurrent.HeadSHA = head2
	decision, updated, err := ReconcilePullRequestGeneration(state, oldProjection, newCurrent, "glassroot.dev/analysis-profile/public-pr/v1alpha1")
	if err != nil || decision.Action != PRDecisionSchedule || updated.Generation != 2 || updated.CurrentTargetID == state.CurrentTargetID || decision.SupersededTargetID != state.CurrentTargetID {
		t.Fatalf("new head decision=%#v state=%#v err=%v", decision, updated, err)
	}
	closed := newCurrent
	closed.Closed = true
	decision, cancelled, err := ReconcilePullRequestGeneration(updated, oldProjection, closed, "glassroot.dev/analysis-profile/public-pr/v1alpha1")
	if err != nil || decision.Action != PRDecisionCancel || !cancelled.Cancelled {
		t.Fatalf("closed decision=%#v state=%#v err=%v", decision, cancelled, err)
	}
	inaccessible := newCurrent
	inaccessible.HeadInaccessible = true
	decision, _, err = ReconcilePullRequestGeneration(updated, oldProjection, inaccessible, "glassroot.dev/analysis-profile/public-pr/v1alpha1")
	if err != nil || decision.Action != PRDecisionUnavailable {
		t.Fatalf("inaccessible decision=%#v err=%v", decision, err)
	}
	bad := projection
	bad.RepositoryID = 999
	if _, _, err := ReconcilePullRequestGeneration(state, bad, current, "glassroot.dev/analysis-profile/public-pr/v1alpha1"); !errors.Is(err, ErrCode(CodeInvalidRepositoryID)) {
		t.Fatalf("mismatch err=%v", err)
	}
}

func TestCheckRunRerequestDecision(t *testing.T) {
	projection := CheckRunProjection{Action: "rerequested", InstallationID: 42, RepositoryID: 101, CheckRunID: 555, HeadSHA: strings.Repeat("2", 40), AppID: 999, ExternalID: "gr-" + strings.Repeat("a", 64)}
	mapping := CheckRunMapping{CheckRunID: 555, GitHubAppID: 999, InstallationID: 42, RepositoryID: 101, ExternalID: projection.ExternalID, TargetID: "target-" + strings.Repeat("b", 64), Generation: 3}
	decision, err := DecideCheckRunRerequest(projection, mapping, 999, 4)
	if err != nil || !decision.Accepted || !decision.Historical || decision.TargetID != mapping.TargetID || decision.Generation != mapping.Generation || decision.AttemptReason != AttemptReasonCheckRerequest {
		t.Fatalf("historical rerequest decision=%#v err=%v", decision, err)
	}
	if _, err := DecideCheckRunRerequest(projection, CheckRunMapping{}, 999, 4); !errors.Is(err, ErrCode(CodeForeignCheckRun)) {
		t.Fatalf("unknown mapping err=%v", err)
	}
	foreign := mapping
	foreign.GitHubAppID = 1000
	if _, err := DecideCheckRunRerequest(projection, foreign, 999, 4); !errors.Is(err, ErrCode(CodeForeignCheckRun)) {
		t.Fatalf("foreign app err=%v", err)
	}
	foreign = mapping
	foreign.ExternalID = "gr-" + strings.Repeat("c", 64)
	if _, err := DecideCheckRunRerequest(projection, foreign, 999, 4); !errors.Is(err, ErrCode(CodeForeignCheckRun)) {
		t.Fatalf("external id err=%v", err)
	}
	projection.Action = "requested_action"
	if _, err := DecideCheckRunRerequest(projection, mapping, 999, 4); !errors.Is(err, ErrCode(CodeUnsupportedAction)) {
		t.Fatalf("requested_action err=%v", err)
	}
}

func TestDeliveryStateTransitions(t *testing.T) {
	state := DeliveryStateReceived
	for _, next := range []DeliveryState{DeliveryStateVerified, DeliveryStatePersisted, DeliveryStateEnqueued} {
		var err error
		state, err = TransitionDelivery(state, next)
		if err != nil {
			t.Fatalf("delivery transition to %s: %v", next, err)
		}
	}
	if _, err := TransitionDelivery(state, DeliveryStateVerified); !errors.Is(err, ErrCode(CodeInvalidStateTransition)) {
		t.Fatalf("delivery backward err=%v", err)
	}
	if _, err := TransitionDelivery(DeliveryStateVerified, DeliveryStateRejected); err != nil {
		t.Fatalf("verified rejected should be explicit terminal transition: %v", err)
	}
}
