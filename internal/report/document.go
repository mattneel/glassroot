package report

import (
	"encoding/json"
	"time"

	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/policy"
)

const (
	SchemaVersionReportV1Alpha1     = "glassroot.dev/report/v1alpha1"
	ReportProfileVersionV1Alpha1    = "glassroot.dev/report-profile/v1alpha1"
	MarkdownRendererVersionV1Alpha1 = "glassroot.dev/markdown-renderer/v1alpha1"
	TerminalRendererVersionV1Alpha1 = "glassroot.dev/terminal-renderer/v1alpha1"
)

type NoticeCode string

const (
	NoticeEvidenceIncomplete                          NoticeCode = "evidence-incomplete"
	NoticeExecutionIncomplete                         NoticeCode = "execution-incomplete"
	NoticeSyntheticEvidence                           NoticeCode = "synthetic-evidence"
	NoticeNoTargetCodeExecuted                        NoticeCode = "no-target-code-executed"
	NoticeFakeRunner                                  NoticeCode = "fake-runner"
	NoticeDevelopmentOnlyRunner                       NoticeCode = "development-only-runner"
	NoticeNetworkDenyNotEnforced                      NoticeCode = "network-deny-not-enforced"
	NoticeInternalConsistencyOnlyManifestVerification NoticeCode = "internal-consistency-only-manifest-verification"
	NoticeWaiversApplied                              NoticeCode = "waivers-applied"
	NoticeGovernanceFindingsPresent                   NoticeCode = "governance-findings-present"
	NoticeObserverLimitationsPresent                  NoticeCode = "observer-limitations-present"
	NoticePassedNotProofOfSafety                      NoticeCode = "passed-is-not-proof-of-safety"
)

type Notice struct {
	Code NoticeCode `json:"code"`
	Text string     `json:"text"`
}

type Document struct {
	SchemaVersion                 string              `json:"schemaVersion"`
	ReportProfileVersion          string              `json:"reportProfileVersion"`
	RunID                         string              `json:"runId"`
	EvaluatedAt                   time.Time           `json:"evaluatedAt"`
	PlanDigest                    model.Digest        `json:"planDigest"`
	ManifestDigest                model.Digest        `json:"manifestDigest"`
	BehavioralDeltaDigest         model.Digest        `json:"behavioralDeltaDigest"`
	BuiltinPolicyEvaluationDigest model.Digest        `json:"builtinPolicyEvaluationDigest"`
	PolicyApplicationDigest       model.Digest        `json:"policyApplicationDigest"`
	ManifestVerificationMode      string              `json:"manifestVerificationMode"`
	Source                        SourceIdentity      `json:"source"`
	Runner                        RunnerSection       `json:"runner"`
	Completeness                  CompletenessSection `json:"completeness"`
	Policy                        PolicySection       `json:"policy"`
	Behavior                      BehaviorSection     `json:"behavior"`
	Notices                       []Notice            `json:"notices"`
	Limitations                   []LimitationReport  `json:"limitations"`
}

type SourceIdentity struct {
	Base                              model.CommitRef `json:"base"`
	Head                              model.CommitRef `json:"head"`
	BaseMaterializedTreeDigest        model.Digest    `json:"baseMaterializedTreeDigest,omitempty"`
	BaseMaterializationManifestDigest model.Digest    `json:"baseMaterializationManifestDigest,omitempty"`
	HeadMaterializedTreeDigest        model.Digest    `json:"headMaterializedTreeDigest,omitempty"`
	HeadMaterializationManifestDigest model.Digest    `json:"headMaterializationManifestDigest,omitempty"`
}

type RunnerSection struct {
	Name                      string              `json:"name"`
	Version                   string              `json:"version"`
	IsolationTier             model.IsolationTier `json:"isolationTier"`
	FreshKernel               bool                `json:"freshKernel"`
	BrokeredNetwork           bool                `json:"brokeredNetwork"`
	ExecutesTargetCode        bool                `json:"executesTargetCode"`
	SyntheticEvidence         bool                `json:"syntheticEvidence"`
	EnforcesNetworkDeny       bool                `json:"enforcesNetworkDeny"`
	ProcessEventCollection    bool                `json:"processEventCollection"`
	FilesystemEventCollection bool                `json:"filesystemEventCollection"`
	SyscallEventCollection    bool                `json:"syscallEventCollection"`
	ArtifactHashing           bool                `json:"artifactHashing"`
	SnapshotSupport           bool                `json:"snapshotSupport"`
}

type CompletenessSection struct {
	BundleTransactionValid         bool                    `json:"bundleTransactionValid"`
	ExecutionComplete              bool                    `json:"executionComplete"`
	EvidenceComplete               bool                    `json:"evidenceComplete"`
	SyntheticEvidence              bool                    `json:"syntheticEvidence"`
	ExpectedManifestDigestSupplied bool                    `json:"expectedManifestDigestSupplied"`
	ExpectedManifestDigestMatched  bool                    `json:"expectedManifestDigestMatched"`
	AttemptCoverage                []AttemptCoverageReport `json:"attemptCoverage"`
}

type AttemptCoverageReport struct {
	AttemptID          string                `json:"attemptId"`
	Ordinal            uint64                `json:"ordinal"`
	Revision           model.RevisionKind    `json:"revision"`
	ScenarioID         string                `json:"scenarioId"`
	Repetition         uint32                `json:"repetition"`
	Events             evidence.CaptureState `json:"events"`
	Stdout             evidence.CaptureState `json:"stdout"`
	Stderr             evidence.CaptureState `json:"stderr"`
	Artifacts          evidence.CaptureState `json:"artifacts"`
	Result             evidence.CaptureState `json:"result"`
	AcceptedEventCount uint64                `json:"acceptedEventCount"`
}

type PolicySection struct {
	ProfileName                 string                      `json:"profileName"`
	ProfileVersion              string                      `json:"profileVersion"`
	BuiltinRuleSetVersion       string                      `json:"builtinRuleSetVersion"`
	GovernanceRuleSetVersion    string                      `json:"governanceRuleSetVersion"`
	OverallEffectiveDisposition model.Disposition           `json:"overallEffectiveDisposition"`
	Summary                     policy.ApplicationSummary   `json:"summary"`
	AppliedFindings             []AppliedFindingReport      `json:"appliedFindings"`
	WaiverStatuses              []policy.WaiverStatusRecord `json:"waiverStatuses"`
	TrustedConfigAuthority      policy.ConfigAuthority      `json:"trustedConfigAuthority"`
	TrustedWaiverAuthority      policy.WaiverAuthority      `json:"trustedWaiverAuthority"`
}

type BehaviorSection struct {
	ComparisonProfile           model.ComparisonProfile    `json:"comparisonProfile"`
	NormalizationProfileVersion string                     `json:"normalizationProfileVersion"`
	ScenarioIDs                 []string                   `json:"scenarioIds"`
	ScenarioComparisons         []model.ScenarioComparison `json:"scenarioComparisons"`
	Summary                     model.DeltaSummary         `json:"summary"`
	Records                     []DeltaRecordReport        `json:"records"`
}

type AppliedFindingReport = policy.AppliedFinding
type DeltaRecordReport = model.DeltaRecord
type LimitationReport = model.Limitation

type FrozenReport struct {
	doc    Document
	json   []byte
	digest model.Digest
}

type RenderedOutput struct {
	RendererVersion string
	Bytes           []byte
	Digest          model.Digest
}

func (r *FrozenReport) Document() Document {
	if r == nil {
		return Document{}
	}
	return cloneDocument(r.doc)
}
func (r *FrozenReport) JSON() []byte {
	if r == nil {
		return nil
	}
	return append([]byte(nil), r.json...)
}
func (r *FrozenReport) Digest() model.Digest {
	if r == nil {
		return ""
	}
	return r.digest
}

func freezeReportDocument(doc Document, limits Limits) (*FrozenReport, error) {
	if doc.SchemaVersion != SchemaVersionReportV1Alpha1 || doc.ReportProfileVersion != ReportProfileVersionV1Alpha1 {
		return nil, errCode(CodeUnsupportedReportProfile, "freeze", "schema", "unsupported report schema or profile", nil)
	}
	ensureDocumentSlices(&doc)
	if int64(len(doc.Notices)) > limits.MaxNotices {
		return nil, errCode(CodeReportLimit, "freeze", "notices", "notice limit exceeded", nil)
	}
	if int64(len(doc.Limitations)) > limits.MaxLimitationsTotal {
		return nil, errCode(CodeReportLimit, "freeze", "limitations", "limitation limit exceeded", nil)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, errCode(CodeSerializationFailed, "freeze", "json", "serialize report", err)
	}
	if limits.MaxReportJSONBytes <= 0 || limits.MaxReportJSONBytes > MaxReportJSONBytesAbsolute {
		return nil, errCode(CodeInvalidLimits, "freeze", "limits", "invalid report JSON limit", nil)
	}
	if int64(len(data)) > limits.MaxReportJSONBytes {
		return nil, errCode(CodeReportTooLarge, "freeze", "json", "report JSON exceeds limit", nil)
	}
	return &FrozenReport{doc: cloneDocument(doc), json: append([]byte(nil), data...), digest: digestBytes(reportJSONDomain, data)}, nil
}

func cloneDocument(in Document) Document {
	data, _ := json.Marshal(in)
	var out Document
	_ = json.Unmarshal(data, &out)
	ensureDocumentSlices(&out)
	return out
}

func ensureDocumentSlices(doc *Document) {
	if doc.Notices == nil {
		doc.Notices = []Notice{}
	}
	if doc.Limitations == nil {
		doc.Limitations = []LimitationReport{}
	}
	if doc.Policy.AppliedFindings == nil {
		doc.Policy.AppliedFindings = []AppliedFindingReport{}
	}
	if doc.Policy.WaiverStatuses == nil {
		doc.Policy.WaiverStatuses = []policy.WaiverStatusRecord{}
	}
	if doc.Policy.TrustedConfigAuthority.Changes == nil {
		doc.Policy.TrustedConfigAuthority.Changes = []policy.ConfigChangeReference{}
	}
	if doc.Policy.TrustedWaiverAuthority.Changes == nil {
		doc.Policy.TrustedWaiverAuthority.Changes = []policy.WaiverChangeRecord{}
	}
	if doc.Behavior.ScenarioIDs == nil {
		doc.Behavior.ScenarioIDs = []string{}
	}
	if doc.Behavior.ScenarioComparisons == nil {
		doc.Behavior.ScenarioComparisons = []model.ScenarioComparison{}
	}
	if doc.Behavior.Records == nil {
		doc.Behavior.Records = []DeltaRecordReport{}
	}
	if doc.Completeness.AttemptCoverage == nil {
		doc.Completeness.AttemptCoverage = []AttemptCoverageReport{}
	}
}
