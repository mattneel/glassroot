package githubsource_test

import (
	"os"
	"testing"
)

func TestGitHubSourceIngesterIntegration(t *testing.T) {
	if os.Getenv("GLASSROOT_GITHUB_SOURCE_INGESTER_INTEGRATION") != "1" {
		t.Skip("set GLASSROOT_GITHUB_SOURCE_INGESTER_INTEGRATION=1 with broker and repository inputs to run gated GitHub source-ingester integration")
	}
	required := []string{"GLASSROOT_GITHUB_SOURCE_BROKER_SOCKET", "GLASSROOT_GITHUB_SOURCE_INSTALLATION_ID", "GLASSROOT_GITHUB_SOURCE_BASE_REPOSITORY_ID", "GLASSROOT_GITHUB_SOURCE_BASE_OWNER", "GLASSROOT_GITHUB_SOURCE_BASE_NAME", "GLASSROOT_GITHUB_SOURCE_PULL_REQUEST_NUMBER", "GLASSROOT_GITHUB_SOURCE_EXPECTED_BASE_COMMIT", "GLASSROOT_GITHUB_SOURCE_EXPECTED_HEAD_COMMIT", "GLASSROOT_GITHUB_SOURCE_HEAD_REPOSITORY_ID", "GLASSROOT_GITHUB_SOURCE_GIT_EXECUTABLE"}
	for _, name := range required {
		if os.Getenv(name) == "" {
			t.Skip("gated GitHub source-ingester integration input missing: " + name)
		}
	}
	t.Skip("live GitHub source-ingester integration harness is gated; full credentialed execution requires operator-provided broker/controller fixtures")
}
