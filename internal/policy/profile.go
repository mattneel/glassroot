package policy

const (
	PolicyProfileNameStrict               = "strict"
	PolicyProfileVersionStrictV1Alpha1    = "glassroot.dev/policy-profile/strict/v1alpha1"
	BuiltinRuleSetVersionStrictV1Alpha1   = "glassroot.dev/builtin-rules/strict/v1alpha1"
	SchemaVersionPolicyEvaluationV1Alpha1 = "glassroot.dev/policy-evaluation/v1alpha1"
)

type PolicyProfile struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func PolicyProfileStrict() PolicyProfile {
	return PolicyProfile{Name: PolicyProfileNameStrict, Version: PolicyProfileVersionStrictV1Alpha1}
}

func validateProfile(p PolicyProfile) error {
	if p.Name != PolicyProfileNameStrict || p.Version != PolicyProfileVersionStrictV1Alpha1 {
		return errCode(CodeInvalidPolicyProfile, "profile", "", "", "profile", "unsupported policy profile", nil)
	}
	return nil
}
