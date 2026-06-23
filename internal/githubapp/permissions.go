package githubapp

const (
	SchemaGitHubAppPermissionsAdvisoryV1Alpha1 = "glassroot.dev/github-app-permissions/advisory/v1alpha1"
	SchemaGitHubAppWebhooksAdvisoryV1Alpha1    = "glassroot.dev/github-app-webhooks/advisory/v1alpha1"
	SchemaGitHubWebhookReceiptV1Alpha1         = "glassroot.dev/github-webhook-receipt/v1alpha1"
	SchemaGitHubAnalysisTargetV1Alpha1         = "glassroot.dev/github-analysis-target/v1alpha1"
	SchemaGitHubAnalysisJobV1Alpha1            = "glassroot.dev/github-analysis-job/v1alpha1"
	SchemaGitHubAnalysisAttemptV1Alpha1        = "glassroot.dev/github-analysis-attempt/v1alpha1"
	SchemaGitHubWorkerAssignmentV1Alpha1       = "glassroot.dev/github-worker-assignment/v1alpha1"
	SchemaGitHubWorkerResultV1Alpha1           = "glassroot.dev/github-worker-result/v1alpha1"
	SchemaGitHubPublishCommandV1Alpha1         = "glassroot.dev/github-publish-command/v1alpha1"
	SchemaGitHubCheckProjectionV1Alpha1        = "glassroot.dev/github-check-projection/v1alpha1"
	CheckProfileAdvisoryV1Alpha1               = "glassroot.dev/github-check-profile/advisory/v1alpha1"
)

type Permission struct {
	Name   string `json:"name"`
	Access string `json:"access"`
}

type PermissionProfile struct {
	SchemaVersion                string       `json:"schemaVersion"`
	RepositoryPermissions        []Permission `json:"repositoryPermissions"`
	AbsentRepositoryPermissions  []string     `json:"absentRepositoryPermissions"`
	OrganizationPermissions      []Permission `json:"organizationPermissions"`
	UserAuthorization            string       `json:"userAuthorization"`
	ComponentDownscopePrinciples []string     `json:"componentDownscopePrinciples"`
}

func DefaultPermissionProfile() PermissionProfile {
	return PermissionProfile{
		SchemaVersion: SchemaGitHubAppPermissionsAdvisoryV1Alpha1,
		RepositoryPermissions: []Permission{
			{Name: "checks", Access: "write"},
			{Name: "contents", Access: "read"},
			{Name: "pull_requests", Access: "read"},
			{Name: "metadata", Access: "read"},
		},
		AbsentRepositoryPermissions: []string{
			"actions", "administration", "codespaces", "commit_statuses", "deployments", "environments", "issues", "members", "packages", "pages", "repository_hooks", "repository_projects", "secret_scanning", "secrets", "security_events", "workflows",
		},
		OrganizationPermissions: []Permission{},
		UserAuthorization:       "disabled",
		ComponentDownscopePrinciples: []string{
			"receiver has webhook secret only",
			"source ingestion uses repository-scoped read token only",
			"publisher uses repository-scoped checks-write token only",
			"worker receives no GitHub credential",
		},
	}
}
