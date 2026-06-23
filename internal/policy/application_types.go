package policy

import (
	"time"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/waiver"
)

const (
	SchemaVersionPolicyApplicationV1Alpha1 = "glassroot.dev/policy-application/v1alpha1"
	GovernanceRuleSetVersionStrictV1Alpha1 = "glassroot.dev/governance-rules/strict/v1alpha1"
	applicationJSONDomain                  = "glassroot.dev/policy-application-json/v1\x00"
)

type ApplicationRequest struct {
	Evaluation    *FrozenEvaluation
	Plan          *pipeline.FrozenPlan
	TrustedConfig config.TrustedLoadResult
	WaiverSource  config.RevisionFileSource
	EvaluatedAt   time.Time
}

type FindingOrigin string

const (
	FindingOriginBuiltinPolicy        FindingOrigin = "builtin-policy"
	FindingOriginTrustedConfiguration FindingOrigin = "trusted-configuration"
	FindingOriginWaiverGovernance     FindingOrigin = "waiver-governance"
)

type AppliedFinding struct {
	Origin               FindingOrigin        `json:"origin"`
	Original             model.Finding        `json:"original"`
	EffectiveDisposition model.Disposition    `json:"effectiveDisposition"`
	AppliedWaiver        *AppliedWaiver       `json:"appliedWaiver,omitempty"`
	GovernanceReference  *GovernanceReference `json:"governanceReference,omitempty"`
}

type AppliedWaiver struct {
	ID                      string       `json:"id"`
	TargetFindingID         string       `json:"targetFindingId"`
	RuleID                  string       `json:"ruleId"`
	Owner                   string       `json:"owner"`
	Reason                  string       `json:"reason"`
	IssuedAt                time.Time    `json:"issuedAt"`
	ExpiresAt               time.Time    `json:"expiresAt"`
	BaseRawDigest           model.Digest `json:"baseRawDigest"`
	SemanticWaiverSetDigest model.Digest `json:"semanticWaiverSetDigest"`
}

type GovernanceReference struct {
	Kind           string       `json:"kind"`
	Path           string       `json:"path,omitempty"`
	ChangeKind     string       `json:"changeKind,omitempty"`
	SecurityEffect string       `json:"securityEffect,omitempty"`
	WaiverID       string       `json:"waiverId,omitempty"`
	State          string       `json:"state,omitempty"`
	RawDigest      model.Digest `json:"rawDigest,omitempty"`
	SemanticDigest model.Digest `json:"semanticDigest,omitempty"`
}

type ConfigAuthority struct {
	Path          string                     `json:"path"`
	BaseRevision  model.CommitRef            `json:"baseRevision"`
	HeadRevision  model.CommitRef            `json:"headRevision"`
	BaseRawDigest model.Digest               `json:"baseRawDigest,omitempty"`
	BaseSizeBytes int64                      `json:"baseSizeBytes"`
	HeadState     config.HeadAssessmentState `json:"headState"`
	HeadRawDigest model.Digest               `json:"headRawDigest,omitempty"`
	Changes       []ConfigChangeReference    `json:"changes"`
}

type ConfigChangeReference struct {
	Path           string `json:"path"`
	Kind           string `json:"kind"`
	SecurityEffect string `json:"securityEffect"`
}

type WaiverAuthority struct {
	Path               string               `json:"path"`
	BaseRevision       model.CommitRef      `json:"baseRevision"`
	HeadRevision       model.CommitRef      `json:"headRevision"`
	BaseState          waiver.BaseState     `json:"baseState"`
	BaseRawDigest      model.Digest         `json:"baseRawDigest,omitempty"`
	BaseSemanticDigest model.Digest         `json:"baseSemanticDigest,omitempty"`
	BaseSizeBytes      int64                `json:"baseSizeBytes,omitempty"`
	HeadState          waiver.HeadState     `json:"headState"`
	HeadRawDigest      model.Digest         `json:"headRawDigest,omitempty"`
	HeadSemanticDigest model.Digest         `json:"headSemanticDigest,omitempty"`
	HeadSizeBytes      int64                `json:"headSizeBytes,omitempty"`
	Changes            []WaiverChangeRecord `json:"changes"`
}

type WaiverChangeRecord struct {
	WaiverID string `json:"waiverId,omitempty"`
	Kind     string `json:"kind"`
}

type WaiverStatus string

const (
	WaiverStatusApplied            WaiverStatus = "applied"
	WaiverStatusExpired            WaiverStatus = "expired"
	WaiverStatusNotYetValid        WaiverStatus = "not-yet-valid"
	WaiverStatusUnused             WaiverStatus = "unused"
	WaiverStatusTargetRuleMismatch WaiverStatus = "target-rule-mismatch"
	WaiverStatusTargetIneligible   WaiverStatus = "target-ineligible"
	WaiverStatusInvalid            WaiverStatus = "invalid"
)

type WaiverStatusRecord struct {
	WaiverID  string       `json:"waiverId"`
	FindingID string       `json:"findingId,omitempty"`
	RuleID    string       `json:"ruleId,omitempty"`
	Status    WaiverStatus `json:"status"`
}

type ApplicationDocument struct {
	SchemaVersion               string               `json:"schemaVersion"`
	EvaluatedAt                 time.Time            `json:"evaluatedAt"`
	RunID                       string               `json:"runId"`
	PlanDigest                  model.Digest         `json:"planDigest"`
	BehavioralDeltaDigest       model.Digest         `json:"behavioralDeltaDigest"`
	BasePolicyEvaluationDigest  model.Digest         `json:"basePolicyEvaluationDigest"`
	PolicyProfileName           string               `json:"policyProfileName"`
	PolicyProfileVersion        string               `json:"policyProfileVersion"`
	BuiltinRuleSetVersion       string               `json:"builtinRuleSetVersion"`
	GovernanceRuleSetVersion    string               `json:"governanceRuleSetVersion"`
	Base                        model.CommitRef      `json:"base"`
	Head                        model.CommitRef      `json:"head"`
	TrustedConfigAuthority      ConfigAuthority      `json:"trustedConfigAuthority"`
	TrustedWaiverAuthority      WaiverAuthority      `json:"trustedWaiverAuthority"`
	OverallEffectiveDisposition model.Disposition    `json:"overallEffectiveDisposition"`
	AppliedFindings             []AppliedFinding     `json:"appliedFindings"`
	WaiverStatuses              []WaiverStatusRecord `json:"waiverStatuses"`
	Summary                     ApplicationSummary   `json:"summary"`
	Limitations                 []model.Limitation   `json:"limitations"`
}

type ApplicationSummary struct {
	TotalFindings            int64 `json:"totalFindings"`
	BuiltinFindings          int64 `json:"builtinFindings"`
	ConfigurationFindings    int64 `json:"configurationFindings"`
	WaiverGovernanceFindings int64 `json:"waiverGovernanceFindings"`
	EffectivePassed          int64 `json:"effectivePassed"`
	EffectiveRequiresReview  int64 `json:"effectiveRequiresReview"`
	EffectiveFailed          int64 `json:"effectiveFailed"`
	EffectiveWaived          int64 `json:"effectiveWaived"`
	Info                     int64 `json:"info"`
	Low                      int64 `json:"low"`
	Medium                   int64 `json:"medium"`
	High                     int64 `json:"high"`
	Critical                 int64 `json:"critical"`
	AppliedWaivers           int64 `json:"appliedWaivers"`
	ExpiredWaivers           int64 `json:"expiredWaivers"`
	UnusedWaivers            int64 `json:"unusedWaivers"`
	InvalidWaivers           int64 `json:"invalidWaivers"`
}

type FrozenApplication struct {
	doc    ApplicationDocument
	json   []byte
	digest model.Digest
}

func (f *FrozenApplication) Document() ApplicationDocument {
	if f == nil {
		return ApplicationDocument{}
	}
	return cloneApplication(f.doc)
}
func (f *FrozenApplication) JSON() []byte {
	if f == nil {
		return nil
	}
	return append([]byte(nil), f.json...)
}
func (f *FrozenApplication) Digest() model.Digest {
	if f == nil {
		return ""
	}
	return f.digest
}
