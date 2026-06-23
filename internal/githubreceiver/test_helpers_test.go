package githubreceiver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validPullRequestPayload(action string) string {
	return fmt.Sprintf(`{
  "action": %q,
  "installation": {"id": 42},
  "repository": {"id": 101, "owner": {"id": 201, "login": "route-owner"}, "name": "route-repo"},
  "pull_request": {
    "number": 7,
    "draft": false,
    "merged": false,
    "state": "open",
    "title": "Never retain me",
    "body": "do not run",
    "base": {"sha": %q, "ref": "main", "repo": {"id": 101, "owner": {"login": "route-owner"}, "name": "route-repo"}},
    "head": {"sha": %q, "ref": "feature/prose", "repo": {"id": 202, "owner": {"login": "head-route-owner"}, "name": "head-route-repo"}}
  },
  "sender": {"login": "octocat", "html_url": "https://example.invalid"}
}`, action, strings.Repeat("1", 40), strings.Repeat("2", 40))
}

func writeSecretFile(t *testing.T, dir, name, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}
