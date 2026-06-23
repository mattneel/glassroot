package githubapi

import (
	"strings"
	"testing"
)

func FuzzDecodePullRequestSnapshot(f *testing.F) {
	base := strings.Repeat("1", 40)
	head := strings.Repeat("2", 40)
	f.Add([]byte(`{"number":7,"state":"open","draft":false,"merged":false,"base":{"sha":"` + base + `","repo":{"id":101,"name":"repo","owner":{"login":"owner"}}},"head":{"sha":"` + head + `","repo":{"id":202,"name":"head","owner":{"login":"headowner"}}}}`))
	f.Add([]byte(`{"number":7,"state":"closed","draft":false,"merged":true,"base":{"sha":"` + base + `","repo":{"id":101,"name":"repo","owner":{"login":"owner"}}},"head":{"sha":"` + head + `","repo":null}}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 2<<20 {
			body = body[:2<<20]
		}
		var out pullRequestResponse
		if err := decodeStrict(body, DefaultLimits(), &out); err != nil {
			return
		}
		_, _ = decodePullRequestSnapshot(out, RepositoryRoute{Owner: "owner", Repo: "repo", RepositoryID: 101}, 7)
	})
}

func FuzzValidateRepositoryRoute(f *testing.F) {
	f.Add("owner", "repo", int64(101))
	f.Add("../owner", "repo/name", int64(-1))
	f.Fuzz(func(t *testing.T, owner, repo string, id int64) {
		if len(owner) > 1024 || len(repo) > 1024 {
			return
		}
		_ = validateRepositoryRoute(RepositoryRoute{Owner: owner, Repo: repo, RepositoryID: id})
	})
}
