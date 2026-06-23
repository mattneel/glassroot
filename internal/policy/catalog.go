package policy

import "github.com/mattneel/glassroot/internal/model"

type ruleMeta struct {
	ID          string
	Version     string
	Title       string
	Description string
	Severity    model.Severity
	Disposition model.Disposition
	Emit        bool
}

const ruleVersionV1Alpha1 = "v1alpha1"

func ruleCatalog() []ruleMeta {
	return []ruleMeta{
		{ID: "GR-OBS-001", Version: ruleVersionV1Alpha1, Title: "Observation coverage incomplete or weakened", Description: "Observation coverage, warnings, unsupported observations, or synthetic evidence require review.", Severity: model.SeverityMedium, Disposition: model.DispositionRequiresReview, Emit: true},
		{ID: "GR-PROC-001", Version: ruleVersionV1Alpha1, Title: "New process or executable", Description: "Head introduced or increased typed process behavior.", Severity: model.SeverityMedium, Disposition: model.DispositionRequiresReview, Emit: true},
		{ID: "GR-FS-001", Version: ruleVersionV1Alpha1, Title: "New executable file or artifact", Description: "Head introduced or changed typed executable file or artifact behavior.", Severity: model.SeverityHigh, Disposition: model.DispositionRequiresReview, Emit: true},
		{ID: "GR-FS-002", Version: ruleVersionV1Alpha1, Title: "New filesystem access outside configured roots", Description: "Head introduced or increased typed absolute-unmapped filesystem access.", Severity: model.SeverityMedium, Disposition: model.DispositionRequiresReview, Emit: true},
		{ID: "GR-NET-001", Version: ruleVersionV1Alpha1, Title: "New or changed network behavior", Description: "Head introduced or changed typed DNS or network behavior.", Severity: model.SeverityHigh, Disposition: model.DispositionRequiresReview, Emit: true},
		{ID: "GR-ART-001", Version: ruleVersionV1Alpha1, Title: "New or changed artifact", Description: "Head introduced or changed typed artifact behavior.", Severity: model.SeverityMedium, Disposition: model.DispositionRequiresReview, Emit: true},
		{ID: "GR-DET-001", Version: ruleVersionV1Alpha1, Title: "Behavioral repeatability degraded", Description: "Head repeatability became less assessable or more variable.", Severity: model.SeverityMedium, Disposition: model.DispositionRequiresReview, Emit: true},
		{ID: "GR-LIMIT-001", Version: ruleVersionV1Alpha1, Title: "Resource limit behavior introduced", Description: "Head introduced or changed typed resource limit behavior.", Severity: model.SeverityHigh, Disposition: model.DispositionRequiresReview, Emit: true},
		{ID: "GR-CONFIG-001", Version: ruleVersionV1Alpha1, Title: "Trusted security configuration changed in head", Description: "Reserved for GR-10B trusted-base configuration policy.", Emit: false},
		{ID: "GR-WAIVER-001", Version: ruleVersionV1Alpha1, Title: "Waiver added, changed, invalid, or expired", Description: "Reserved for GR-10B trusted-base waiver policy.", Emit: false},
	}
}

func ruleByID(id string) ruleMeta {
	for _, r := range ruleCatalog() {
		if r.ID == id {
			return r
		}
	}
	return ruleMeta{}
}

func emittedRuleIDs() []string {
	ids := []string{}
	for _, r := range ruleCatalog() {
		if r.Emit {
			ids = append(ids, r.ID)
		}
	}
	return sortedStrings(ids)
}

func validateRuleCatalog() error {
	seen := map[string]struct{}{}
	for _, r := range ruleCatalog() {
		if r.ID == "" || r.Version == "" || r.Title == "" || len(r.ID) > MaxRuleIDBytes || len(r.Version) > MaxRuleVersionBytes || len(r.Title) > MaxTitleBytes {
			return errCode(CodeInvalidRuleCatalog, "catalog", r.ID, "", "metadata", "invalid rule metadata", nil)
		}
		if _, ok := seen[r.ID]; ok {
			return errCode(CodeInvalidRuleCatalog, "catalog", r.ID, "", "id", "duplicate rule id", nil)
		}
		seen[r.ID] = struct{}{}
	}
	return nil
}
