package observe

import (
	"sort"

	"github.com/mattneel/glassroot/internal/model"
)

func SupportedObservationKinds() []model.ObservationKind {
	return []model.ObservationKind{model.ObservationKindProcessStart, model.ObservationKindProcessExit, model.ObservationKindFilesystemCreate, model.ObservationKindFilesystemRead, model.ObservationKindFilesystemWrite, model.ObservationKindFilesystemDelete, model.ObservationKindFilesystemRename, model.ObservationKindFilesystemChmod, model.ObservationKindDNSQuery, model.ObservationKindNetworkConnection, model.ObservationKindArtifactActivity, model.ObservationKindScenarioStarted, model.ObservationKindScenarioCompleted, model.ObservationKindObserverWarning, model.ObservationKindUnsupportedObservation, model.ObservationKindResourceLimit}
}

func validateProfile(p NormalizationProfile) error {
	if p.Version != ProfileVersionV1Alpha1 || p.ProcessIdentityAlgorithm != ProcessIdentityAlgorithmV1 || p.TimestampAlgorithm != TimestampAlgorithmV1 || p.PathRootAlgorithm != PathRootAlgorithmV1 {
		return errCode(CodeInvalidNormalizationProfile, "profile", "", "version", "unsupported normalization profile", nil)
	}
	for _, f := range p.IgnoreFields {
		switch f {
		case IgnoreFieldEventTimestamp, IgnoreFieldProcessPID:
		default:
			return errCode(CodeUnsupportedIgnoreField, "profile", "", f, "unsupported trusted compare-ignore field", nil)
		}
	}
	if len(p.RootAliases) > MaxPathRoots {
		return errCode(CodeInvalidNormalizationProfile, "profile", "", "rootAliases", "too many path roots", nil)
	}
	return nil
}

func profileFromPlan(plan model.RunPlan) (NormalizationProfile, error) {
	p := NormalizationProfile{Version: ProfileVersionV1Alpha1, ProcessIdentityAlgorithm: ProcessIdentityAlgorithmV1, TimestampAlgorithm: TimestampAlgorithmV1, PathRootAlgorithm: PathRootAlgorithmV1, IgnoreFields: []string{}, RootAliases: []PathRootAlias{}}
	if plan.Comparison != nil {
		p.IgnoreFields = append([]string(nil), plan.Comparison.IgnoreFields...)
	}
	if !containsString(p.IgnoreFields, IgnoreFieldEventTimestamp) || !containsString(p.IgnoreFields, IgnoreFieldProcessPID) {
		// Do not invent hidden defaults. A profile can omit these if trusted config omits them.
	}
	if plan.ExecutionEnvironment != nil && plan.ExecutionEnvironment.Workdir != "" {
		p.RootAliases = append(p.RootAliases, PathRootAlias{Namespace: PathNamespaceWorkdirRoot, RootIndex: 0, Root: plan.ExecutionEnvironment.Workdir, Alias: "@workdir"})
	}
	seen := map[string]struct{}{}
	if len(p.RootAliases) > 0 {
		seen[p.RootAliases[0].Root] = struct{}{}
	}
	idx := uint32(1)
	if plan.Collection != nil {
		for _, root := range plan.Collection.FilesystemRoots {
			if _, ok := seen[root]; ok {
				continue
			}
			seen[root] = struct{}{}
			p.RootAliases = append(p.RootAliases, PathRootAlias{Namespace: PathNamespaceCollectionRoot, RootIndex: idx, Root: root, Alias: "@root" + itoa(idx)})
			idx++
		}
	}
	if err := validateProfile(p); err != nil {
		return NormalizationProfile{}, err
	}
	return p, nil
}
func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
func includeTiming(p NormalizationProfile) bool {
	return !containsString(p.IgnoreFields, IgnoreFieldEventTimestamp)
}
func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	b := []byte{}
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
func sortLimitations(l []model.Limitation) []model.Limitation {
	if len(l) == 0 {
		return []model.Limitation{}
	}
	out := append([]model.Limitation(nil), l...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		if out[i].Summary != out[j].Summary {
			return out[i].Summary < out[j].Summary
		}
		return out[i].Details < out[j].Details
	})
	return out
}
