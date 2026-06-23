package evidence

import "time"

const (
	MaxBundlePathBytes                  = 4096
	MaxPhysicalPathBytes                = 4096
	MaxPhysicalPathComponentBytes       = 255
	MaxPhysicalDepth                    = 32
	MaxDirectories                      = 100000
	MaxFiles                            = 250001
	MaxInventoryEntries                 = 350000
	MaxJSONDepth                        = 64
	MaxJSONTokens                       = 5000000
	MaxJSONMembersPerObject             = 100000
	MaxJSONArrayElements                = 250000
	MaxJSONStringBytes            int64 = 1 << 20
	MaxJSONNumberBytes                  = 128
	MaxDiagnostics                      = 100
)

const MaxReadDuration = 10 * time.Minute

type ReaderLimits struct {
	MaxAttempts                   int
	MaxBundlePathBytes            int
	MaxPhysicalPathBytes          int
	MaxPhysicalPathComponentBytes int
	MaxPhysicalDepth              int
	MaxDirectories                int
	MaxFiles                      int
	MaxInventoryEntries           int
	MaxBundleBytes                int64
	MaxManifestBytes              int64
	MaxPlanBytes                  int64
	MaxExecutionBytes             int64
	MaxAttemptResultBytes         int64
	MaxArtifactIndexBytes         int64
	MaxEventLineBytes             int64
	MaxEventStreamBytesPerAttempt int64
	MaxEventsPerAttempt           int
	MaxEventsPerBundle            int
	MaxLogBytesPerStream          int64
	MaxTotalLogBytes              int64
	MaxSingleArtifactBytes        int64
	MaxTotalArtifactBytes         int64
	MaxArtifactsPerAttempt        int
	MaxArtifactsPerBundle         int
	MaxManifestEntries            int
	MaxJSONDepth                  int
	MaxJSONTokens                 int
	MaxJSONMembersPerObject       int
	MaxJSONArrayElements          int
	MaxJSONStringBytes            int64
	MaxJSONNumberBytes            int
	MaxDiagnostics                int
	MaxReadDuration               time.Duration
}

func DefaultReaderLimits() ReaderLimits {
	return ReaderLimits{MaxAttempts: MaxAttempts, MaxBundlePathBytes: MaxBundlePathBytes, MaxPhysicalPathBytes: MaxPhysicalPathBytes, MaxPhysicalPathComponentBytes: MaxPhysicalPathComponentBytes, MaxPhysicalDepth: MaxPhysicalDepth, MaxDirectories: MaxDirectories, MaxFiles: MaxFiles, MaxInventoryEntries: MaxInventoryEntries, MaxBundleBytes: MaxBundleBytes, MaxManifestBytes: MaxManifestBytes, MaxPlanBytes: 16 << 20, MaxExecutionBytes: 64 << 20, MaxAttemptResultBytes: 1 << 20, MaxArtifactIndexBytes: MaxArtifactIndexBytes, MaxEventLineBytes: MaxEventJSONBytes, MaxEventStreamBytesPerAttempt: MaxEventStreamBytesPerAttempt, MaxEventsPerAttempt: MaxEventsPerAttempt, MaxEventsPerBundle: MaxEventsPerBundle, MaxLogBytesPerStream: MaxLogBytesPerStream, MaxTotalLogBytes: MaxTotalLogBytes, MaxSingleArtifactBytes: MaxSingleArtifactBytes, MaxTotalArtifactBytes: MaxTotalArtifactBytes, MaxArtifactsPerAttempt: MaxArtifactsPerAttempt, MaxArtifactsPerBundle: MaxArtifactsPerBundle, MaxManifestEntries: MaxManifestEntries, MaxJSONDepth: MaxJSONDepth, MaxJSONTokens: MaxJSONTokens, MaxJSONMembersPerObject: MaxJSONMembersPerObject, MaxJSONArrayElements: MaxJSONArrayElements, MaxJSONStringBytes: MaxJSONStringBytes, MaxJSONNumberBytes: MaxJSONNumberBytes, MaxDiagnostics: MaxDiagnostics, MaxReadDuration: MaxReadDuration}
}

func validateReaderLimits(l ReaderLimits) error {
	d := DefaultReaderLimits()
	checks := []struct {
		name     string
		got, max int64
	}{
		{"maxAttempts", int64(l.MaxAttempts), int64(d.MaxAttempts)}, {"maxBundlePathBytes", int64(l.MaxBundlePathBytes), int64(d.MaxBundlePathBytes)}, {"maxPhysicalPathBytes", int64(l.MaxPhysicalPathBytes), int64(d.MaxPhysicalPathBytes)}, {"maxPhysicalPathComponentBytes", int64(l.MaxPhysicalPathComponentBytes), int64(d.MaxPhysicalPathComponentBytes)}, {"maxPhysicalDepth", int64(l.MaxPhysicalDepth), int64(d.MaxPhysicalDepth)}, {"maxDirectories", int64(l.MaxDirectories), int64(d.MaxDirectories)}, {"maxFiles", int64(l.MaxFiles), int64(d.MaxFiles)}, {"maxInventoryEntries", int64(l.MaxInventoryEntries), int64(d.MaxInventoryEntries)}, {"maxBundleBytes", l.MaxBundleBytes, d.MaxBundleBytes}, {"maxManifestBytes", l.MaxManifestBytes, d.MaxManifestBytes}, {"maxPlanBytes", l.MaxPlanBytes, d.MaxPlanBytes}, {"maxExecutionBytes", l.MaxExecutionBytes, d.MaxExecutionBytes}, {"maxAttemptResultBytes", l.MaxAttemptResultBytes, d.MaxAttemptResultBytes}, {"maxArtifactIndexBytes", l.MaxArtifactIndexBytes, d.MaxArtifactIndexBytes}, {"maxEventLineBytes", l.MaxEventLineBytes, d.MaxEventLineBytes}, {"maxEventStreamBytesPerAttempt", l.MaxEventStreamBytesPerAttempt, d.MaxEventStreamBytesPerAttempt}, {"maxEventsPerAttempt", int64(l.MaxEventsPerAttempt), int64(d.MaxEventsPerAttempt)}, {"maxEventsPerBundle", int64(l.MaxEventsPerBundle), int64(d.MaxEventsPerBundle)}, {"maxLogBytesPerStream", l.MaxLogBytesPerStream, d.MaxLogBytesPerStream}, {"maxTotalLogBytes", l.MaxTotalLogBytes, d.MaxTotalLogBytes}, {"maxSingleArtifactBytes", l.MaxSingleArtifactBytes, d.MaxSingleArtifactBytes}, {"maxTotalArtifactBytes", l.MaxTotalArtifactBytes, d.MaxTotalArtifactBytes}, {"maxArtifactsPerAttempt", int64(l.MaxArtifactsPerAttempt), int64(d.MaxArtifactsPerAttempt)}, {"maxArtifactsPerBundle", int64(l.MaxArtifactsPerBundle), int64(d.MaxArtifactsPerBundle)}, {"maxManifestEntries", int64(l.MaxManifestEntries), int64(d.MaxManifestEntries)}, {"maxJSONDepth", int64(l.MaxJSONDepth), int64(d.MaxJSONDepth)}, {"maxJSONTokens", int64(l.MaxJSONTokens), int64(d.MaxJSONTokens)}, {"maxJSONMembersPerObject", int64(l.MaxJSONMembersPerObject), int64(d.MaxJSONMembersPerObject)}, {"maxJSONArrayElements", int64(l.MaxJSONArrayElements), int64(d.MaxJSONArrayElements)}, {"maxJSONStringBytes", l.MaxJSONStringBytes, d.MaxJSONStringBytes}, {"maxJSONNumberBytes", int64(l.MaxJSONNumberBytes), int64(d.MaxJSONNumberBytes)}, {"maxDiagnostics", int64(l.MaxDiagnostics), int64(d.MaxDiagnostics)},
	}
	for _, c := range checks {
		if c.got <= 0 || c.got > c.max {
			return errCode(CodeInvalidLimits, "reader-limits", c.name, "reader limit must be positive and no greater than absolute ceiling", nil)
		}
	}
	if l.MaxReadDuration <= 0 || l.MaxReadDuration > d.MaxReadDuration {
		return errCode(CodeInvalidLimits, "reader-limits", "maxReadDuration", "reader duration limit invalid", nil)
	}
	return nil
}
