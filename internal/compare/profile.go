package compare

import (
	"sort"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/observe"
)

const (
	ComparisonProfileVersionV1Alpha1 = "glassroot.dev/comparison-profile/v1alpha1"
	ExactSemanticMatchAlgorithmV1    = "glassroot.dev/exact-semantic-multiset/v1"
	TypedAnchorAlgorithmV1           = "glassroot.dev/comparison-anchor/v1"
	RepetitionAssessmentAlgorithmV1  = "glassroot.dev/repetition-occurrence-profile/v1"
	OrderAssessmentAlgorithmV1       = "glassroot.dev/strict-semantic-sequence/v1"
	AbsencePolicyV1                  = "glassroot.dev/unknown-is-not-absence/v1"
)

func comparisonProfile() model.ComparisonProfile {
	kinds := make([]string, 0)
	for _, k := range SupportedFactKinds() {
		kinds = append(kinds, string(k))
	}
	return model.ComparisonProfile{Version: ComparisonProfileVersionV1Alpha1, RequiredNormalizationProfile: observe.ProfileVersionV1Alpha1, ExactSemanticMatchAlgorithm: ExactSemanticMatchAlgorithmV1, TypedAnchorAlgorithm: TypedAnchorAlgorithmV1, RepetitionAssessmentAlgorithm: RepetitionAssessmentAlgorithmV1, OrderAssessmentAlgorithm: OrderAssessmentAlgorithmV1, AbsencePolicy: AbsencePolicyV1, IncludedFactKinds: kinds}
}

func SupportedFactKinds() []observe.FactKind {
	out := make([]observe.FactKind, 0)
	for _, kind := range observe.SupportedObservationKinds() {
		out = append(out, observe.FactKind(kind))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func isSupportedFactKind(k observe.FactKind) bool {
	for _, x := range SupportedFactKinds() {
		if x == k {
			return true
		}
	}
	return false
}
func isSupportedSource(s model.ObservationSource) bool {
	switch s {
	case model.ObservationSourceHostObserved, model.ObservationSourceNetworkBrokerObserved, model.ObservationSourceSandboxRuntimeObserved, model.ObservationSourceGuestAgentReported, model.ObservationSourceWorkloadReported, model.ObservationSourceStaticAnalysisDerived, model.ObservationSourceModelInferred, model.ObservationSourceSyntheticTestGenerated:
		return true
	default:
		return false
	}
}
