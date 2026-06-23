package githubcontrollerstore_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
)

func TestSourceImportRequestCarriesPullRequestNumber(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	created := scheduleEligible(t, store, "outbox-pr-number", "5", "6")
	req, err := store.GetSourceImportRequest(ctx, created.SourceRequestID)
	if err != nil {
		t.Fatalf("GetSourceImportRequest: %v", err)
	}
	if req.PullRequestNumber != 7 {
		t.Fatalf("PullRequestNumber = %d, want durable PR number 7", req.PullRequestNumber)
	}
}

func TestSourceImportRequestTypeHasNoCredentialField(t *testing.T) {
	typ := reflect.TypeOf(SourceImportRequestForTest{})
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		if strings.Contains(name, "token") || strings.Contains(name, "secret") || strings.Contains(name, "credential") {
			t.Fatalf("SourceImportRequest contains credential-like field %q", typ.Field(i).Name)
		}
	}
}

// SourceImportRequestForTest aliases the production type so this test fails if a
// credential-like field is added to the durable source-import contract.
type SourceImportRequestForTest = githubcontrollerstore.SourceImportRequest
