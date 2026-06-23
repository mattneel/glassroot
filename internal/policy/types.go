package policy

import (
	"github.com/mattneel/glassroot/internal/compare"
	"github.com/mattneel/glassroot/internal/model"
)

type EvaluationRequest struct {
	Profile PolicyProfile
	Delta   *compare.FrozenDelta
}

type EvaluationDocument struct {
	SchemaVersion            string             `json:"schemaVersion"`
	PolicyProfileName        string             `json:"policyProfileName"`
	PolicyProfileVersion     string             `json:"policyProfileVersion"`
	BuiltinRuleSetVersion    string             `json:"builtinRuleSetVersion"`
	BehavioralDeltaDigest    model.Digest       `json:"behavioralDeltaDigest"`
	RunID                    string             `json:"runId"`
	PlanDigest               model.Digest       `json:"planDigest"`
	ManifestDigest           model.Digest       `json:"manifestDigest"`
	ManifestVerificationMode string             `json:"manifestVerificationMode"`
	ExecutionComplete        bool               `json:"executionComplete"`
	EvidenceComplete         bool               `json:"evidenceComplete"`
	OverallDisposition       model.Disposition  `json:"overallDisposition"`
	Findings                 []model.Finding    `json:"findings"`
	Summary                  EvaluationSummary  `json:"summary"`
	Limitations              []model.Limitation `json:"limitations"`
}

type EvaluationSummary struct {
	TotalFindings  int64 `json:"totalFindings"`
	Info           int64 `json:"info"`
	Low            int64 `json:"low"`
	Medium         int64 `json:"medium"`
	High           int64 `json:"high"`
	Critical       int64 `json:"critical"`
	RequiresReview int64 `json:"requiresReview"`
	Failed         int64 `json:"failed"`
	Passed         int64 `json:"passed"`
	Waived         int64 `json:"waived"`
}

type FrozenEvaluation struct {
	doc    EvaluationDocument
	json   []byte
	digest model.Digest
}

func (f *FrozenEvaluation) Document() EvaluationDocument {
	if f == nil {
		return EvaluationDocument{}
	}
	return cloneEvaluation(f.doc)
}
func (f *FrozenEvaluation) JSON() []byte {
	if f == nil {
		return nil
	}
	return append([]byte(nil), f.json...)
}
func (f *FrozenEvaluation) Digest() model.Digest {
	if f == nil {
		return ""
	}
	return f.digest
}
