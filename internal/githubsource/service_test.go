package githubsource_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubbroker"
	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
	"github.com/mattneel/glassroot/internal/githubsource"
)

func TestProcessNextRequestsOnlyBaseRepositorySourceReadTokenAndAppliesOpaqueStoreResult(t *testing.T) {
	ctx := context.Background()
	req := sourceRequest()
	store := &fakeControllerStore{requests: []githubcontrollerstore.SourceImportRequest{req}}
	broker := &fakeBroker{lease: githubbroker.NewTokenLeaseForTest(githubbroker.TokenMetadata{Purpose: githubbroker.PurposeSourceRead, InstallationID: req.InstallationID, RepositoryID: req.Base.RepositoryID, ExpiresAt: time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC)}, []byte("source-token-canary"))}
	importer := &fakeImporter{result: githubsource.ImportResult{SourceStoreID: "source-" + strings.Repeat("a", 64), MetadataDigest: "sha256:" + strings.Repeat("b", 64), ImportProfileVersion: githubsource.ImportProfileSmartHTTPShallowV1Alpha1, ObjectFormat: "sha1", BaseCommitID: req.Base.CommitID, HeadCommitID: req.Head.CommitID, BaseTreeID: strings.Repeat("3", 40), HeadTreeID: strings.Repeat("4", 40), Limitations: []string{"history outside selected shallow commits not imported"}}}
	svc, err := githubsource.New(githubsource.Config{SourceIngesterID: "source-1", Store: store, Broker: broker, Importer: importer, Clock: fixedClock{time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	processed, res, err := svc.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("ProcessNext: %v", err)
	}
	if !processed || res.Decision != githubsource.DecisionImported {
		t.Fatalf("bad process result: processed=%v res=%#v", processed, res)
	}
	if broker.request.Purpose != githubbroker.PurposeSourceRead || broker.request.RepositoryID != req.Base.RepositoryID || broker.request.InstallationID != req.InstallationID {
		t.Fatalf("bad token request: %#v", broker.request)
	}
	if broker.request.RepositoryID == req.Head.RepositoryID {
		t.Fatalf("requested head repository token: %#v", broker.request)
	}
	if !importer.usedToken {
		t.Fatalf("importer did not receive token through TokenLease.Use")
	}
	if store.result.SourceStoreID != importer.result.SourceStoreID || store.result.BaseRepositoryID != req.Base.RepositoryID || store.result.HeadRepositoryID != req.Head.RepositoryID {
		t.Fatalf("bad applied result: %#v", store.result)
	}
	if strings.Contains(store.result.SourceStoreID, "token") || strings.Contains(store.result.SourceStoreID, "/") {
		t.Fatalf("result leaked credential/path: %#v", store.result)
	}
}

func TestProcessNextReusesExistingStoreWithoutRequestingToken(t *testing.T) {
	ctx := context.Background()
	req := sourceRequest()
	store := &fakeControllerStore{requests: []githubcontrollerstore.SourceImportRequest{req}}
	broker := &fakeBroker{lease: githubbroker.NewTokenLeaseForTest(githubbroker.TokenMetadata{Purpose: githubbroker.PurposeSourceRead, InstallationID: req.InstallationID, RepositoryID: req.Base.RepositoryID, ExpiresAt: time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC)}, []byte("source-token-canary"))}
	importer := &fakeReuseImporter{result: githubsource.ImportResult{SourceStoreID: "source-" + strings.Repeat("f", 64), MetadataDigest: "sha256:" + strings.Repeat("a", 64), ImportProfileVersion: githubsource.ImportProfileSmartHTTPShallowV1Alpha1, ObjectFormat: "sha1", BaseCommitID: req.Base.CommitID, HeadCommitID: req.Head.CommitID, BaseTreeID: strings.Repeat("5", 40), HeadTreeID: strings.Repeat("6", 40), Limitations: []string{"history outside selected shallow commits not imported"}, Reused: true}}
	svc, err := githubsource.New(githubsource.Config{SourceIngesterID: "source-1", Store: store, Broker: broker, Importer: importer, Clock: fixedClock{time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	processed, res, err := svc.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("ProcessNext: %v", err)
	}
	if !processed || res.Decision != githubsource.DecisionReused {
		t.Fatalf("bad reuse result: processed=%v res=%#v", processed, res)
	}
	if broker.calls != 0 {
		t.Fatalf("broker called despite reusable existing store: %#v", broker.request)
	}
	if importer.importCalls != 0 {
		t.Fatalf("fresh import called despite reusable existing store")
	}
	if store.result.SourceStoreID != importer.result.SourceStoreID || strings.Contains(store.result.SourceStoreID, "token") {
		t.Fatalf("bad reused result: %#v", store.result)
	}
}

func sourceRequest() githubcontrollerstore.SourceImportRequest {
	return githubcontrollerstore.SourceImportRequest{SchemaVersion: githubcontrollerstore.SchemaSourceImportRequestV1Alpha1, ID: "source-" + strings.Repeat("c", 64), TargetID: "target-" + strings.Repeat("d", 64), JobID: "job-" + strings.Repeat("e", 64), Generation: 1, InstallationID: 42, PullRequestNumber: 7, Base: githubcontrollerstore.RouteHint{RepositoryID: 101, Owner: "owner", Name: "repo", CommitID: strings.Repeat("1", 40)}, Head: githubcontrollerstore.RouteHint{RepositoryID: 202, Owner: "head", Name: "headrepo", CommitID: strings.Repeat("2", 40)}, ControllerProfileVersion: githubcontrollerstore.ControllerProfileAdvisoryV1Alpha1, State: githubcontrollerstore.SourceStatePending, CreatedAt: time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)}
}

type fakeControllerStore struct {
	requests []githubcontrollerstore.SourceImportRequest
	result   githubcontrollerstore.SourceImportResult
}

func (f *fakeControllerStore) ClaimSourceImports(ctx context.Context, owner string, now time.Time, duration time.Duration, limit int) ([]githubcontrollerstore.SourceImportRequest, error) {
	return f.requests, nil
}
func (f *fakeControllerStore) ApplySourceImportResult(ctx context.Context, r githubcontrollerstore.SourceImportResult, owner string, leaseGeneration int64, when time.Time) error {
	f.result = r
	return nil
}
func (f *fakeControllerStore) ReleaseSourceImport(ctx context.Context, id, owner string, generation int64, when time.Time, failureCode string) error {
	return nil
}

type fakeBroker struct {
	lease   *githubbroker.TokenLease
	request githubbroker.TokenRequest
	calls   int
}

func (f *fakeBroker) RequestToken(ctx context.Context, req githubbroker.TokenRequest) (*githubbroker.TokenLease, error) {
	f.calls++
	f.request = req
	return f.lease, nil
}

type fakeImporter struct {
	result    githubsource.ImportResult
	usedToken bool
}

func (f *fakeImporter) Import(ctx context.Context, req githubsource.ImportRequest) (githubsource.ImportResult, error) {
	_ = req.Token.Use(func(token []byte) error { f.usedToken = string(token) == "source-token-canary"; return nil })
	return f.result, nil
}

type fakeReuseImporter struct {
	result      githubsource.ImportResult
	importCalls int
}

func (f *fakeReuseImporter) ReuseIfAvailable(ctx context.Context, req githubcontrollerstore.SourceImportRequest) (githubsource.ImportResult, bool, error) {
	return f.result, true, nil
}

func (f *fakeReuseImporter) Import(ctx context.Context, req githubsource.ImportRequest) (githubsource.ImportResult, error) {
	f.importCalls++
	return githubsource.ImportResult{}, nil
}

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }
