package githubapp

import "testing"

func TestInstallationRepositoriesProjectionKeepsDistinctKind(t *testing.T) {
	body := []byte(`{"action":"removed","installation":{"id":42},"repositories":[{"id":101}]}`)
	projection, err := ProjectWebhook("installation_repositories", body, DefaultLimits())
	if err != nil {
		t.Fatalf("ProjectWebhook: %v", err)
	}
	if projection.Kind != ProjectionInstallationRepositories {
		t.Fatalf("kind = %q", projection.Kind)
	}
	if projection.Installation == nil || projection.Installation.Action != "removed" || len(projection.Installation.RepositoryIDs) != 1 || projection.Installation.RepositoryIDs[0] != 101 {
		t.Fatalf("bad projection %#v", projection.Installation)
	}
}
