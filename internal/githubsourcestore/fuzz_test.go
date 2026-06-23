package githubsourcestore_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/githubsourcestore"
)

func FuzzValidateSourceStoreMetadata(f *testing.F) {
	id := githubsourcestore.Identity{ImportProfileVersion: githubsourcestore.ImportProfileSmartHTTPShallowV1Alpha1, TargetID: "target-" + strings.Repeat("a", 64), BaseRepositoryID: 1, HeadRepositoryID: 2, PullRequestNumber: 7, BaseCommitID: strings.Repeat("1", 40), HeadCommitID: strings.Repeat("2", 40)}
	meta, _ := githubsourcestore.NewMetadata(id, "sha1", strings.Repeat("3", 40), strings.Repeat("4", 40))
	seed, _, _ := githubsourcestore.EncodeMetadata(meta)
	f.Add(seed)
	f.Add([]byte(`{"schemaVersion":"bad"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var m githubsourcestore.Metadata
		_ = json.Unmarshal(data, &m)
		_ = githubsourcestore.ValidateMetadata(m)
	})
}

func FuzzValidateShallowMetadata(f *testing.F) {
	f.Add([]byte(strings.Repeat("1", 40)+"\n"), "sha1")
	f.Add([]byte("bad\n"), "sha1")
	f.Fuzz(func(t *testing.T, data []byte, format string) {
		_ = githubsourcestore.ValidateShallowMetadata(data, format, 2)
	})
}

func FuzzReconcileSourceStorePublication(f *testing.F) {
	f.Add("target-"+strings.Repeat("a", 64), int64(1), int64(2), int64(7), strings.Repeat("1", 40), strings.Repeat("2", 40))
	f.Fuzz(func(t *testing.T, target string, baseRepo, headRepo, pr int64, base, head string) {
		id := githubsourcestore.Identity{ImportProfileVersion: githubsourcestore.ImportProfileSmartHTTPShallowV1Alpha1, TargetID: target, BaseRepositoryID: baseRepo, HeadRepositoryID: headRepo, PullRequestNumber: pr, BaseCommitID: base, HeadCommitID: head}
		sid, err := githubsourcestore.ComputeSourceStoreID(id)
		if err == nil {
			_, _ = githubsourcestore.LayoutPath("/tmp/source-root", sid)
		}
	})
}
