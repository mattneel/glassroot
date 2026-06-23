package githubsource

import (
	"context"
	"encoding/base64"
	"runtime"
	"strings"
	"time"

	"github.com/mattneel/glassroot/internal/githubbroker"
	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
	"github.com/mattneel/glassroot/internal/githubsourcestore"
)

const ImportProfileSmartHTTPShallowV1Alpha1 = githubsourcestore.ImportProfileSmartHTTPShallowV1Alpha1

type Clock interface{ Now() time.Time }

type Store interface {
	ClaimSourceImports(context.Context, string, time.Time, time.Duration, int) ([]githubcontrollerstore.SourceImportRequest, error)
	ApplySourceImportResult(context.Context, githubcontrollerstore.SourceImportResult, string, int64, time.Time) error
	ReleaseSourceImport(context.Context, string, string, int64, time.Time, string) error
}

type Broker interface {
	RequestToken(context.Context, githubbroker.TokenRequest) (*githubbroker.TokenLease, error)
}

type Importer interface {
	Import(context.Context, ImportRequest) (ImportResult, error)
}

type ReuseChecker interface {
	ReuseIfAvailable(context.Context, githubcontrollerstore.SourceImportRequest) (ImportResult, bool, error)
}

type Config struct {
	SourceIngesterID string
	Store            Store
	Broker           Broker
	Importer         Importer
	Clock            Clock
	Limits           Limits
}

type Service struct {
	cfg    Config
	limits Limits
}

type ImportRequest struct {
	SourceImportRequest githubcontrollerstore.SourceImportRequest
	Token               *githubbroker.TokenLease
}

type ImportResult struct {
	SourceStoreID        string
	MetadataDigest       string
	ImportProfileVersion string
	ObjectFormat         string
	BaseCommitID         string
	HeadCommitID         string
	BaseTreeID           string
	HeadTreeID           string
	Limitations          []string
	Reused               bool
}

type Decision string

const (
	DecisionImported    Decision = "imported"
	DecisionReused      Decision = "reused"
	DecisionUnavailable Decision = "unavailable"
	DecisionStale       Decision = "stale"
)

type ProcessResult struct {
	Decision                 Decision
	RequestID, SourceStoreID string
}

func New(cfg Config) (*Service, error) {
	limits := cfg.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if !validID(cfg.SourceIngesterID) || cfg.Store == nil || cfg.Broker == nil || cfg.Importer == nil || cfg.Clock == nil {
		return nil, errCode(CodeInvalidConfig, "config", "source ingester configuration rejected", nil)
	}
	return &Service{cfg: cfg, limits: limits}, nil
}

func (s *Service) ProcessNext(ctx context.Context) (bool, ProcessResult, error) {
	if runtime.GOOS != "linux" {
		return false, ProcessResult{}, errCode(CodeUnsupportedPlatform, "platform", "source ingestion is linux-only", nil)
	}
	now := cleanTime(s.cfg.Clock.Now())
	if now.IsZero() {
		return false, ProcessResult{}, errCode(CodeInvalidConfig, "clock", "clock rejected", nil)
	}
	reqs, err := s.cfg.Store.ClaimSourceImports(ctx, s.cfg.SourceIngesterID, now, s.limits.SourceLeaseDuration, 1)
	if err != nil {
		return false, ProcessResult{}, wrap(CodeControllerResultFailed, "claim", "source claim failed", err)
	}
	if len(reqs) == 0 {
		return false, ProcessResult{}, nil
	}
	req := reqs[0]
	if err := ValidateSourceImportRequest(req, s.limits); err != nil {
		_ = s.cfg.Store.ReleaseSourceImport(ctx, req.ID, s.cfg.SourceIngesterID, req.LeaseGeneration, now, string(CodeInvalidSourceRequest))
		return true, ProcessResult{Decision: DecisionUnavailable, RequestID: req.ID}, err
	}
	if reuse, ok := s.cfg.Importer.(ReuseChecker); ok {
		res, reused, err := reuse.ReuseIfAvailable(ctx, req)
		if err != nil {
			_ = s.cfg.Store.ReleaseSourceImport(ctx, req.ID, s.cfg.SourceIngesterID, req.LeaseGeneration, now, Diagnostic(err))
			return true, ProcessResult{Decision: DecisionUnavailable, RequestID: req.ID}, err
		}
		if reused {
			if err := s.apply(ctx, req, res, now); err != nil {
				return true, ProcessResult{Decision: DecisionUnavailable, RequestID: req.ID}, err
			}
			return true, ProcessResult{Decision: DecisionReused, RequestID: req.ID, SourceStoreID: res.SourceStoreID}, nil
		}
	}
	lease, err := s.cfg.Broker.RequestToken(ctx, githubbroker.TokenRequest{SchemaVersion: githubbroker.SchemaTokenRequestV1Alpha1, Purpose: githubbroker.PurposeSourceRead, InstallationID: req.InstallationID, RepositoryID: req.Base.RepositoryID})
	if err != nil {
		_ = s.cfg.Store.ReleaseSourceImport(ctx, req.ID, s.cfg.SourceIngesterID, req.LeaseGeneration, now, string(CodeSourceTokenFailed))
		return true, ProcessResult{Decision: DecisionUnavailable, RequestID: req.ID}, wrap(CodeSourceTokenFailed, "broker", "source token request failed", err)
	}
	defer lease.Close()
	if err := validateTokenScope(lease, req); err != nil {
		_ = s.cfg.Store.ReleaseSourceImport(ctx, req.ID, s.cfg.SourceIngesterID, req.LeaseGeneration, now, string(CodeTokenScopeInvalid))
		return true, ProcessResult{Decision: DecisionUnavailable, RequestID: req.ID}, err
	}
	res, err := s.cfg.Importer.Import(ctx, ImportRequest{SourceImportRequest: req, Token: lease})
	if err != nil {
		_ = s.cfg.Store.ReleaseSourceImport(ctx, req.ID, s.cfg.SourceIngesterID, req.LeaseGeneration, now, Diagnostic(err))
		return true, ProcessResult{Decision: DecisionUnavailable, RequestID: req.ID}, err
	}
	if err := ValidateImportResult(res, req); err != nil {
		_ = s.cfg.Store.ReleaseSourceImport(ctx, req.ID, s.cfg.SourceIngesterID, req.LeaseGeneration, now, string(CodeSourceStoreVerifyFailed))
		return true, ProcessResult{Decision: DecisionUnavailable, RequestID: req.ID}, err
	}
	if err := s.apply(ctx, req, res, now); err != nil {
		return true, ProcessResult{Decision: DecisionUnavailable, RequestID: req.ID}, err
	}
	decision := DecisionImported
	if res.Reused {
		decision = DecisionReused
	}
	return true, ProcessResult{Decision: decision, RequestID: req.ID, SourceStoreID: res.SourceStoreID}, nil
}

func (s *Service) apply(ctx context.Context, req githubcontrollerstore.SourceImportRequest, res ImportResult, now time.Time) error {
	apply := githubcontrollerstore.SourceImportResult{RequestID: req.ID, TargetID: req.TargetID, JobID: req.JobID, Generation: req.Generation, BaseRepositoryID: req.Base.RepositoryID, HeadRepositoryID: req.Head.RepositoryID, BaseCommitID: req.Base.CommitID, HeadCommitID: req.Head.CommitID, SourceStoreID: res.SourceStoreID, MetadataDigest: res.MetadataDigest, ImportProfileVersion: res.ImportProfileVersion, ObjectFormat: res.ObjectFormat, BaseTreeID: res.BaseTreeID, HeadTreeID: res.HeadTreeID, Limitations: append([]string(nil), res.Limitations...)}
	if err := s.cfg.Store.ApplySourceImportResult(ctx, apply, s.cfg.SourceIngesterID, req.LeaseGeneration, now); err != nil {
		return wrap(CodeControllerResultFailed, "apply", "controller source result failed", err)
	}
	return nil
}

func ValidateSourceImportRequest(req githubcontrollerstore.SourceImportRequest, limits Limits) error {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return err
	}
	if req.SchemaVersion != githubcontrollerstore.SchemaSourceImportRequestV1Alpha1 || req.ID == "" || req.TargetID == "" || req.JobID == "" || req.Generation <= 0 || req.InstallationID <= 0 || req.PullRequestNumber <= 0 || req.ControllerProfileVersion != githubcontrollerstore.ControllerProfileAdvisoryV1Alpha1 {
		return errCode(CodeInvalidSourceRequest, "request", "source request rejected", nil)
	}
	if !validObjectID(req.Base.CommitID) || !validObjectID(req.Head.CommitID) || len(req.Base.CommitID) != len(req.Head.CommitID) {
		return errCode(CodeInvalidObjectID, "request", "commit identity rejected", nil)
	}
	if req.Base.RepositoryID <= 0 || req.Head.RepositoryID <= 0 {
		return errCode(CodeInvalidSourceRequest, "request", "repository identity rejected", nil)
	}
	if !validRoute(req.Base.Owner, limits.MaxRouteSegmentBytes) || !validRoute(req.Base.Name, limits.MaxRouteSegmentBytes) {
		return errCode(CodeInvalidRoute, "request", "base route rejected", nil)
	}
	return nil
}

func ValidateImportResult(res ImportResult, req githubcontrollerstore.SourceImportRequest) error {
	if !strings.HasPrefix(res.SourceStoreID, "source-") || len(res.SourceStoreID) != len("source-")+64 || res.MetadataDigest == "" || res.ImportProfileVersion != ImportProfileSmartHTTPShallowV1Alpha1 || res.BaseCommitID != req.Base.CommitID || res.HeadCommitID != req.Head.CommitID || res.ObjectFormat == "" || !validObjectID(res.BaseTreeID) || !validObjectID(res.HeadTreeID) || len(res.BaseTreeID) != len(req.Base.CommitID) || len(res.HeadTreeID) != len(req.Head.CommitID) || len(res.Limitations) == 0 {
		return errCode(CodeSourceStoreVerifyFailed, "result", "source result rejected", nil)
	}
	return nil
}

func validateTokenScope(lease *githubbroker.TokenLease, req githubcontrollerstore.SourceImportRequest) error {
	meta := lease.Metadata()
	if meta.Purpose != githubbroker.PurposeSourceRead || meta.InstallationID != req.InstallationID || meta.RepositoryID != req.Base.RepositoryID {
		return errCode(CodeTokenScopeInvalid, "token", "source token scope rejected", nil)
	}
	return nil
}

func BasicAuthHeader(token []byte) []byte {
	plain := append([]byte("x-access-token:"), token...)
	enc := make([]byte, base64.StdEncoding.EncodedLen(len(plain)))
	base64.StdEncoding.Encode(enc, plain)
	out := append([]byte("Authorization: Basic "), enc...)
	zero(plain)
	zero(enc)
	return out
}

func cleanTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC().Round(0)
}
func validID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i, r := range s {
		if i == 0 && (r < 'a' || r > 'z') {
			return false
		}
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func validObjectID(s string) bool { return isLowerHex(s, 40) || isLowerHex(s, 64) }
func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if r < '0' || (r > '9' && r < 'a') || r > 'f' {
			return false
		}
	}
	return true
}
func validRoute(s string, max int) bool {
	if s == "" || len(s) > max || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' || r == '?' || r == '#' {
			return false
		}
	}
	return true
}
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
