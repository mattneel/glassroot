package report

import (
	"encoding/json"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/policy"
)

func stripApplicationBundlePaths(in policy.ApplicationDocument) policy.ApplicationDocument {
	data, _ := json.Marshal(in)
	var out policy.ApplicationDocument
	_ = json.Unmarshal(data, &out)
	for i := range out.AppliedFindings {
		stripFinding(&out.AppliedFindings[i].Original)
	}
	return out
}

func stripDeltaBundlePaths(in model.BehavioralDelta) model.BehavioralDelta {
	data, _ := json.Marshal(in)
	var out model.BehavioralDelta
	_ = json.Unmarshal(data, &out)
	for i := range out.Records {
		stripRefs(out.Records[i].Evidence)
		stripRefs(out.Records[i].BaseEvidence)
		stripRefs(out.Records[i].HeadEvidence)
		for j := range out.Records[i].BaseFacts {
			stripRefs(out.Records[i].BaseFacts[j].Evidence)
		}
		for j := range out.Records[i].HeadFacts {
			stripRefs(out.Records[i].HeadFacts[j].Evidence)
		}
	}
	return out
}

func stripFinding(f *model.Finding) {
	stripRefs(f.Evidence)
	for i := range f.Waivers {
		stripRefs(f.Waivers[i].Evidence)
	}
}

func stripRefs(refs []model.EvidenceRef) {
	for i := range refs {
		refs[i].BundlePath = nil
	}
}
