package githubsource_test

import (
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
	"github.com/mattneel/glassroot/internal/githubsource"
	"github.com/mattneel/glassroot/internal/gitstore"
)

func FuzzValidateGitHubSourceRoute(f *testing.F) {
	f.Add("owner", "repo")
	f.Add("..", "repo")
	f.Fuzz(func(t *testing.T, owner, repo string) { _, _ = githubsource.BuildRemoteURL(owner, repo) })
}

func FuzzBuildGitHubFetchCommand(f *testing.F) {
	f.Add("owner", "repo", int64(7))
	f.Fuzz(func(t *testing.T, owner, repo string, pr int64) {
		remote, err := githubsource.BuildRemoteURL(owner, repo)
		if err != nil {
			return
		}
		_, _ = gitstore.BuildGitHubSourceImportPlan(gitstore.GitHubSourceImportConfig{GitPath: "/usr/bin/git", RepositoryDir: "/tmp/repository.git", TemplateDir: "/tmp/template", ObjectFormat: gitstore.ObjectFormatSHA1, RemoteURL: remote, BaseCommitID: strings.Repeat("1", 40), ExpectedHeadID: strings.Repeat("2", 40), PullRequestNumber: pr, AuthHeaderEnvName: gitstore.DefaultGitAuthHeaderEnvName, AuthHeaderValue: []byte("Authorization: Basic abc")})
	})
}

func FuzzValidateSourceImportResult(f *testing.F) {
	req := sourceRequest()
	f.Add("source-"+strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64), strings.Repeat("3", 40), strings.Repeat("4", 40))
	f.Fuzz(func(t *testing.T, sid, digest, baseTree, headTree string) {
		_ = githubsource.ValidateImportResult(githubsource.ImportResult{SourceStoreID: sid, MetadataDigest: digest, ImportProfileVersion: githubsource.ImportProfileSmartHTTPShallowV1Alpha1, ObjectFormat: "sha1", BaseCommitID: req.Base.CommitID, HeadCommitID: req.Head.CommitID, BaseTreeID: baseTree, HeadTreeID: headTree, Limitations: []string{"history outside selected shallow commits not imported"}}, req)
	})
}

var _ githubcontrollerstore.SourceImportRequest
