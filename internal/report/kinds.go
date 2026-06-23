package report

import "github.com/mattneel/glassroot/internal/model"

func validateDeltaKind(k model.DeltaKind) error {
	switch k {
	case model.DeltaKindAdded, model.DeltaKindRemoved, model.DeltaKindModified, model.DeltaKindCountChanged, model.DeltaKindOrderChanged, model.DeltaKindStabilityChanged, model.DeltaKindCoverageChanged,
		model.DeltaKindAddedProcess, model.DeltaKindAddedFilesystemActivity, model.DeltaKindAddedNetworkConnection, model.DeltaKindArtifactChanged, model.DeltaKindObservationIncomplete:
		return nil
	default:
		return errCode(CodeUnsupportedDeltaKind, "render", "deltaKind", "unsupported delta kind", nil)
	}
}
