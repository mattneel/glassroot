package inspect

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/mattneel/glassroot/internal/compare"
	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/gitstore"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/observe"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/policy"
	"github.com/mattneel/glassroot/internal/report"
)

type Inspector struct{ limits Limits }

type Result struct {
	Report             *report.FrozenReport
	OverallDisposition model.Disposition
	VerificationMode   VerificationMode
}

func New(limits Limits) (*Inspector, error) {
	l, err := validateLimits(limits)
	if err != nil {
		return nil, err
	}
	return &Inspector{limits: l}, nil
}

func (i *Inspector) Inspect(ctx context.Context, req Request) (res *Result, err error) {
	if i == nil {
		return nil, errCode(CodeInvalidLimits, "inspect", "inspector is nil", nil)
	}
	if err := ValidateRequest(req); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, contextErr("inspect", err)
	}
	ctx, cancel := context.WithTimeout(ctx, i.limits.MaxInspectDuration)
	defer cancel()

	bundle, err := openBundle(ctx, req, i.limits.Reader)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := bundle.Close(); closeErr != nil {
			wrapped := errCode(CodeBundleCloseFailed, "bundle", "bundle close failed", closeErr)
			if err != nil {
				err = errors.Join(err, wrapped)
			} else {
				res = nil
				err = wrapped
			}
		}
	}()

	verifiedPlan := bundle.Plan()
	manifest := bundle.Manifest()

	repo, err := gitstore.Open(ctx, req.GitDir)
	if err != nil {
		return nil, wrapStage(CodeGitOpenFailed, "git", "open trusted bare git store failed", err)
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil && err == nil {
			err = errCode(CodeGitOpenFailed, "git", "close trusted git resources failed", closeErr)
			res = nil
		}
	}()

	base, head, err := resolveAndBind(ctx, repo, req, verifiedPlan)
	if err != nil {
		return nil, err
	}
	source := gitstore.NewRevisionFileSource(repo)
	trustedReq := config.TrustedLoadRequest{Base: trustedCommitRef(verifiedPlan.Base, base, model.RevisionKindBase), Head: trustedCommitRef(verifiedPlan.Head, head, model.RevisionKindHead)}
	trusted, err := config.LoadTrusted(ctx, source, trustedReq)
	if err != nil {
		return nil, trustedConfigError(err)
	}

	rebuilt, err := rebuildPlan(ctx, verifiedPlan, trusted, manifest.PlanDigest)
	if err != nil {
		return nil, err
	}

	trace, err := normalize(ctx, bundle, i.limits.Normalize)
	if err != nil {
		return nil, err
	}
	delta, err := compareTrace(ctx, trace, i.limits.Compare)
	if err != nil {
		return nil, err
	}
	evaluation, err := evaluatePolicy(ctx, delta, i.limits.Policy)
	if err != nil {
		return nil, err
	}
	application, err := applyPolicy(ctx, evaluation, rebuilt, trusted, source, req.EvaluatedAt, i.limits.Application)
	if err != nil {
		return nil, err
	}
	frozenReport, err := buildReport(ctx, bundle, delta, application, i.limits.Report)
	if err != nil {
		return nil, err
	}
	appDoc := application.Document()
	return &Result{Report: frozenReport, OverallDisposition: appDoc.OverallEffectiveDisposition, VerificationMode: verificationMode(bundle.Verification().Mode)}, nil
}

func openBundle(ctx context.Context, req Request, limits evidence.ReaderLimits) (*evidence.Bundle, error) {
	opts := []evidence.ReaderOption{}
	if req.ManifestIntegrityMode == ManifestIntegrityExpectedDigest {
		opts = append(opts, evidence.WithExpectedManifestDigest(req.ExpectedManifestDigest))
	}
	b, err := evidence.OpenAndVerify(ctx, req.BundleDir, limits, opts...)
	if err != nil {
		return nil, wrapStage(CodeBundleOpenFailed, "bundle", "strict evidence bundle verification failed", err)
	}
	return b, nil
}

func resolveAndBind(ctx context.Context, repo *gitstore.Repository, req Request, plan model.RunPlan) (gitstore.ResolvedRevision, gitstore.ResolvedRevision, error) {
	format := repo.ObjectFormat()
	if len(req.BaseCommitID) != format.ObjectIDLength() || len(req.HeadCommitID) != format.ObjectIDLength() {
		return gitstore.ResolvedRevision{}, gitstore.ResolvedRevision{}, errCode(CodeObjectFormatMismatch, "revision", "explicit commit width does not match repository object format", nil)
	}
	base, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(req.BaseCommitID))
	if err != nil {
		return gitstore.ResolvedRevision{}, gitstore.ResolvedRevision{}, wrapStage(CodeRevisionResolveFailed, "revision", "base commit did not resolve to an exact commit", err)
	}
	if base.CommitID != req.BaseCommitID {
		return gitstore.ResolvedRevision{}, gitstore.ResolvedRevision{}, errCode(CodeRevisionMismatch, "revision", "base object did not resolve directly to itself as a commit", nil)
	}
	head, err := repo.ResolveCommit(ctx, gitstore.ObjectIDSelector(req.HeadCommitID))
	if err != nil {
		return gitstore.ResolvedRevision{}, gitstore.ResolvedRevision{}, wrapStage(CodeRevisionResolveFailed, "revision", "head commit did not resolve to an exact commit", err)
	}
	if head.CommitID != req.HeadCommitID {
		return gitstore.ResolvedRevision{}, gitstore.ResolvedRevision{}, errCode(CodeRevisionMismatch, "revision", "head object did not resolve directly to itself as a commit", nil)
	}
	if err := bindResolvedRevisions(plan, format, base, head); err != nil {
		return gitstore.ResolvedRevision{}, gitstore.ResolvedRevision{}, err
	}
	return base, head, nil
}

func trustedConfigError(err error) error {
	usage := false
	if errors.Is(err, config.ErrBaseConfigMissing) || errors.Is(err, config.ErrBaseConfigInvalid) || errors.Is(err, config.ErrUnsupportedBaseEntry) {
		usage = true
	}
	ie := &Error{Code: CodeTrustedConfigFailed, Stage: "trusted-config", Message: "trusted-base pipeline configuration failed", Usage: usage, Err: err}
	return ie
}

func rebuildPlan(ctx context.Context, verified model.RunPlan, trusted config.TrustedLoadResult, planDigest model.Digest) (*pipeline.FrozenPlan, error) {
	base, err := sourceSnapshotFromRevision(verified.Revisions[0])
	if err != nil {
		return nil, err
	}
	head, err := sourceSnapshotFromRevision(verified.Revisions[1])
	if err != nil {
		return nil, err
	}
	platform, err := platformFromPlan(verified.Platform)
	if err != nil {
		return nil, err
	}
	rebuilt, err := pipeline.Build(ctx, pipeline.BuildRequest{RunID: verified.RunID, CreatedAt: verified.CreatedAt, Trusted: trusted, BaseSource: base, HeadSource: head, Platform: platform})
	if err != nil {
		return nil, wrapStage(CodePlanRebuildFailed, "rebuild", "run plan reconstruction failed", err)
	}
	if err := requirePlanEquality(verified, rebuilt, planDigest); err != nil {
		return nil, err
	}
	verifiedJSON, err := json.Marshal(verified)
	if err != nil {
		return nil, errCode(CodePlanMismatch, "rebuild", "verified plan serialization failed", err)
	}
	if pipeline.DigestRunPlanJSON(verifiedJSON) != planDigest {
		return nil, errCode(CodePlanMismatch, "rebuild", "verified plan digest does not match manifest", nil)
	}
	return rebuilt, nil
}

func normalize(ctx context.Context, bundle *evidence.Bundle, limits observe.Limits) (*observe.TraceSet, error) {
	n, err := observe.New(limits)
	if err != nil {
		return nil, errCode(CodeNormalizationFailed, "normalize", "normalizer initialization failed", err)
	}
	trace, err := n.Normalize(ctx, bundle)
	if err != nil {
		return nil, wrapStage(CodeNormalizationFailed, "normalize", "normalization failed", err)
	}
	return trace, nil
}

func compareTrace(ctx context.Context, trace *observe.TraceSet, limits compare.Limits) (*compare.FrozenDelta, error) {
	c, err := compare.New(limits)
	if err != nil {
		return nil, errCode(CodeComparisonFailed, "compare", "comparator initialization failed", err)
	}
	delta, err := c.Compare(ctx, trace)
	if err != nil {
		return nil, wrapStage(CodeComparisonFailed, "compare", "behavioral comparison failed", err)
	}
	return delta, nil
}

func evaluatePolicy(ctx context.Context, delta *compare.FrozenDelta, limits policy.Limits) (*policy.FrozenEvaluation, error) {
	e, err := policy.New(limits)
	if err != nil {
		return nil, errCode(CodePolicyEvaluationFailed, "policy", "policy evaluator initialization failed", err)
	}
	eval, err := e.Evaluate(ctx, policy.EvaluationRequest{Profile: policy.PolicyProfileStrict(), Delta: delta})
	if err != nil {
		return nil, wrapStage(CodePolicyEvaluationFailed, "policy", "built-in policy evaluation failed", err)
	}
	return eval, nil
}

func applyPolicy(ctx context.Context, eval *policy.FrozenEvaluation, plan *pipeline.FrozenPlan, trusted config.TrustedLoadResult, source config.RevisionFileSource, evaluatedAt time.Time, limits policy.ApplicationLimits) (*policy.FrozenApplication, error) {
	a, err := policy.NewApplier(limits)
	if err != nil {
		return nil, errCode(CodePolicyApplicationFailed, "policy-application", "policy applier initialization failed", err)
	}
	application, err := a.Apply(ctx, policy.ApplicationRequest{Evaluation: eval, Plan: plan, TrustedConfig: trusted, WaiverSource: source, EvaluatedAt: evaluatedAt})
	if err != nil {
		return nil, wrapStage(CodePolicyApplicationFailed, "policy-application", "policy application failed", err)
	}
	return application, nil
}

func buildReport(ctx context.Context, bundle *evidence.Bundle, delta *compare.FrozenDelta, application *policy.FrozenApplication, limits report.Limits) (*report.FrozenReport, error) {
	b, err := report.New(limits)
	if err != nil {
		return nil, errCode(CodeReportBuildFailed, "report", "report builder initialization failed", err)
	}
	frozen, err := b.Build(ctx, report.BuildRequest{Bundle: bundle, Delta: delta, Application: application})
	if err != nil {
		return nil, wrapStage(CodeReportBuildFailed, "report", "report composition failed", err)
	}
	return frozen, nil
}

func verificationMode(mode evidence.VerificationMode) VerificationMode {
	if mode == evidence.VerificationModeExpectedManifestDigest {
		return VerificationModeExpectedManifestDigest
	}
	return VerificationModeInternalConsistencyOnly
}
