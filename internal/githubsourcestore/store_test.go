package githubsourcestore_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/githubsourcestore"
)

func TestSourceStoreIDAndMetadataAreDeterministicOpaqueAndCredentialFree(t *testing.T) {
	id := githubsourcestore.Identity{
		ImportProfileVersion: githubsourcestore.ImportProfileSmartHTTPShallowV1Alpha1,
		TargetID:             "target-" + strings.Repeat("a", 64),
		BaseRepositoryID:     101,
		HeadRepositoryID:     202,
		PullRequestNumber:    7,
		BaseCommitID:         strings.Repeat("1", 40),
		HeadCommitID:         strings.Repeat("2", 40),
	}
	storeID, err := githubsourcestore.ComputeSourceStoreID(id)
	if err != nil {
		t.Fatalf("ComputeSourceStoreID: %v", err)
	}
	again, err := githubsourcestore.ComputeSourceStoreID(id)
	if err != nil || again != storeID {
		t.Fatalf("source store ID not deterministic: %q %q err=%v", storeID, again, err)
	}
	if !strings.HasPrefix(storeID, "source-") || len(storeID) != len("source-")+64 {
		t.Fatalf("source store ID shape = %q", storeID)
	}
	meta, err := githubsourcestore.NewMetadata(id, "sha1", strings.Repeat("3", 40), strings.Repeat("4", 40))
	if err != nil {
		t.Fatalf("NewMetadata: %v", err)
	}
	encoded, digest, err := githubsourcestore.EncodeMetadata(meta)
	if err != nil {
		t.Fatalf("EncodeMetadata: %v", err)
	}
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("metadata digest shape = %q", digest)
	}
	for _, forbidden := range []string{"token", "github.com", "https://", "/tmp", "owner", "repo", "branch"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("metadata leaked %q: %s", forbidden, encoded)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("metadata JSON invalid: %v", err)
	}
	if decoded["schemaVersion"] != githubsourcestore.SchemaSourceStoreV1Alpha1 || decoded["sourceStoreId"] != storeID || decoded["shallow"] != true {
		t.Fatalf("metadata core fields wrong: %v", decoded)
	}
	root := t.TempDir()
	path, err := githubsourcestore.LayoutPath(root, storeID)
	if err != nil {
		t.Fatalf("LayoutPath: %v", err)
	}
	wantPrefix := filepath.Join(root, "stores", "sha256", storeID[len("source-"):len("source-")+2])
	if !strings.HasPrefix(path, wantPrefix) || strings.Contains(path, id.BaseCommitID) || strings.Contains(path, "owner") {
		t.Fatalf("unexpected layout path %q", path)
	}
}
