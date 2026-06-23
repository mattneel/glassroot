package githubapi_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubauth"
)

func TestGitHubBrokerIntegration(t *testing.T) {
	if os.Getenv("GLASSROOT_GITHUB_BROKER_INTEGRATION") != "1" {
		t.Skip("set GLASSROOT_GITHUB_BROKER_INTEGRATION=1 with explicit app credentials to run")
	}
	appID, err := strconv.ParseInt(os.Getenv("GLASSROOT_GITHUB_APP_ID"), 10, 64)
	if err != nil || appID <= 0 {
		t.Skip("missing valid GLASSROOT_GITHUB_APP_ID")
	}
	clientID := os.Getenv("GLASSROOT_GITHUB_APP_CLIENT_ID")
	keyPath := os.Getenv("GLASSROOT_GITHUB_APP_PRIVATE_KEY")
	installationID, err := strconv.ParseInt(os.Getenv("GLASSROOT_GITHUB_INSTALLATION_ID"), 10, 64)
	if err != nil || installationID <= 0 {
		t.Skip("missing valid GLASSROOT_GITHUB_INSTALLATION_ID")
	}
	repositoryID, err := strconv.ParseInt(os.Getenv("GLASSROOT_GITHUB_REPOSITORY_ID"), 10, 64)
	if err != nil || repositoryID <= 0 {
		t.Skip("missing valid GLASSROOT_GITHUB_REPOSITORY_ID")
	}
	if clientID == "" || keyPath == "" {
		t.Skip("missing GitHub App client ID or private-key path")
	}
	key, err := githubauth.LoadPrivateKey(keyPath, githubauth.DefaultLimits())
	if err != nil {
		t.Fatalf("load private key: %v", githubauth.Diagnostic(err))
	}
	defer key.Close()
	client, err := githubapi.NewClient(githubapi.Config{Identity: githubauth.AppIdentity{AppID: appID, ClientID: clientID}, Signer: key, Clock: realIntegrationClock{}, Limits: githubapi.DefaultLimits(), AuthLimits: githubauth.DefaultLimits()})
	if err != nil {
		t.Fatalf("client: %v", githubapi.Diagnostic(err))
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.VerifyApp(ctx); err != nil {
		t.Fatalf("verify app: %v", githubapi.Diagnostic(err))
	}
	lease, err := client.IssueInstallationToken(ctx, githubapi.TokenRequest{Purpose: githubapi.PurposePullRequestRead, InstallationID: installationID, RepositoryID: repositoryID})
	if err != nil {
		t.Fatalf("issue token: %v", githubapi.Diagnostic(err))
	}
	defer lease.Close()
	if lease.Metadata().RepositoryID != repositoryID || lease.Metadata().Purpose != githubapi.PurposePullRequestRead {
		t.Fatalf("unexpected token metadata")
	}
}

type realIntegrationClock struct{}

func (realIntegrationClock) Now() time.Time { return time.Now().UTC().Round(0) }
