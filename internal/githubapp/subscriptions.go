package githubapp

type WebhookActionDecision string

const (
	WebhookActionSchedule    WebhookActionDecision = "schedule"
	WebhookActionCancel      WebhookActionDecision = "cancel"
	WebhookActionNoop        WebhookActionDecision = "noop"
	WebhookActionRerequest   WebhookActionDecision = "rerequest"
	WebhookActionUnsupported WebhookActionDecision = "unsupported"
)

type WebhookProfile struct {
	SchemaVersion                   string   `json:"schemaVersion"`
	Events                          []string `json:"events"`
	PullRequestSchedulingActions    []string `json:"pullRequestSchedulingActions"`
	PullRequestCancellationActions  []string `json:"pullRequestCancellationActions"`
	PullRequestKnownNoopActions     []string `json:"pullRequestKnownNoopActions"`
	CheckRunActions                 []string `json:"checkRunActions"`
	CheckSuiteActions               []string `json:"checkSuiteActions"`
	InstallationLifecycleEvents     []string `json:"installationLifecycleEvents"`
	OperationalEvents               []string `json:"operationalEvents"`
	ExplicitlyAbsentInitialTriggers []string `json:"explicitlyAbsentInitialTriggers"`
}

func DefaultWebhookProfile() WebhookProfile {
	return WebhookProfile{
		SchemaVersion:                  SchemaGitHubAppWebhooksAdvisoryV1Alpha1,
		Events:                         []string{"pull_request", "check_run", "check_suite", "installation", "installation_repositories", "ping"},
		PullRequestSchedulingActions:   []string{"opened", "reopened", "synchronize", "ready_for_review"},
		PullRequestCancellationActions: []string{"converted_to_draft", "closed"},
		PullRequestKnownNoopActions:    []string{"assigned", "auto_merge_disabled", "auto_merge_enabled", "edited", "labeled", "locked", "ready_for_review_duplicate", "review_request_removed", "review_requested", "unassigned", "unlabeled", "unlocked"},
		CheckRunActions:                []string{"rerequested"},
		CheckSuiteActions:              []string{"noop-only"},
		InstallationLifecycleEvents:    []string{"installation", "installation_repositories"},
		OperationalEvents:              []string{"ping"},
		ExplicitlyAbsentInitialTriggers: []string{
			"push", "issue_comment", "issues", "pull_request_review", "pull_request_review_comment", "workflow_run", "workflow_job", "repository_dispatch", "merge_group", "deployment", "status",
		},
	}
}

func ClassifyWebhookAction(event, action string) (WebhookActionDecision, error) {
	switch event {
	case "pull_request":
		switch action {
		case "opened", "reopened", "synchronize", "ready_for_review":
			return WebhookActionSchedule, nil
		case "converted_to_draft", "closed":
			return WebhookActionCancel, nil
		default:
			return WebhookActionNoop, nil
		}
	case "check_run":
		if action == "rerequested" {
			return WebhookActionRerequest, nil
		}
		return WebhookActionNoop, nil
	case "check_suite", "installation", "installation_repositories", "ping":
		return WebhookActionNoop, nil
	default:
		return WebhookActionUnsupported, errCode(CodeUnsupportedEvent, "webhook", "event does not schedule work", nil)
	}
}
