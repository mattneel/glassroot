package evidence

import "time"

const (
	MaxAttempts                         = 1280
	MaxBundleEntries                    = 250000
	MaxBundleBytes                int64 = 16 << 30
	MaxManifestBytes              int64 = 64 << 20
	MaxManifestEntries                  = 250000
	MaxEventJSONBytes             int64 = 256 << 10
	MaxEventsPerAttempt                 = 10000
	MaxEventsPerBundle                  = 100000
	MaxEventStreamBytesPerAttempt int64 = 2 << 30
	MaxLogBytesPerStream          int64 = 100 << 20
	MaxTotalLogBytes              int64 = 4 << 30
	MaxArtifactsPerAttempt              = 1000
	MaxArtifactsPerBundle               = 10000
	MaxSingleArtifactBytes        int64 = 1 << 30
	MaxTotalArtifactBytes         int64 = 8 << 30
	MaxArtifactIndexBytes         int64 = 64 << 20
	MaxLimitations                      = 1000
	MaxLimitationCodeBytes              = 128
	MaxLimitationMessageBytes           = 1 << 10
	MaxLogicalPathBytes                 = 4096
	MaxPhysicalEntryPathBytes           = 4096
)

const MaxBundleWriteDuration = 10 * time.Minute

type Limits struct {
	MaxAttempts                   int
	MaxBundleEntries              int
	MaxBundleBytes                int64
	MaxManifestBytes              int64
	MaxManifestEntries            int
	MaxEventJSONBytes             int64
	MaxEventsPerAttempt           int
	MaxEventsPerBundle            int
	MaxEventStreamBytesPerAttempt int64
	MaxLogBytesPerStream          int64
	MaxTotalLogBytes              int64
	MaxArtifactsPerAttempt        int
	MaxArtifactsPerBundle         int
	MaxSingleArtifactBytes        int64
	MaxTotalArtifactBytes         int64
	MaxArtifactIndexBytes         int64
	MaxLimitations                int
	MaxLimitationCodeBytes        int
	MaxLimitationMessageBytes     int
	MaxLogicalPathBytes           int
	MaxPhysicalEntryPathBytes     int
	MaxDuration                   time.Duration
}

func DefaultLimits() Limits {
	return Limits{MaxAttempts: MaxAttempts, MaxBundleEntries: MaxBundleEntries, MaxBundleBytes: MaxBundleBytes, MaxManifestBytes: MaxManifestBytes, MaxManifestEntries: MaxManifestEntries, MaxEventJSONBytes: MaxEventJSONBytes, MaxEventsPerAttempt: MaxEventsPerAttempt, MaxEventsPerBundle: MaxEventsPerBundle, MaxEventStreamBytesPerAttempt: MaxEventStreamBytesPerAttempt, MaxLogBytesPerStream: MaxLogBytesPerStream, MaxTotalLogBytes: MaxTotalLogBytes, MaxArtifactsPerAttempt: MaxArtifactsPerAttempt, MaxArtifactsPerBundle: MaxArtifactsPerBundle, MaxSingleArtifactBytes: MaxSingleArtifactBytes, MaxTotalArtifactBytes: MaxTotalArtifactBytes, MaxArtifactIndexBytes: MaxArtifactIndexBytes, MaxLimitations: MaxLimitations, MaxLimitationCodeBytes: MaxLimitationCodeBytes, MaxLimitationMessageBytes: MaxLimitationMessageBytes, MaxLogicalPathBytes: MaxLogicalPathBytes, MaxPhysicalEntryPathBytes: MaxPhysicalEntryPathBytes, MaxDuration: MaxBundleWriteDuration}
}

func validateLimits(l Limits) error {
	d := DefaultLimits()
	if l.MaxAttempts <= 0 || l.MaxAttempts > d.MaxAttempts || l.MaxBundleEntries <= 0 || l.MaxBundleEntries > d.MaxBundleEntries || l.MaxBundleBytes <= 0 || l.MaxBundleBytes > d.MaxBundleBytes || l.MaxManifestBytes <= 0 || l.MaxManifestBytes > d.MaxManifestBytes || l.MaxManifestEntries <= 0 || l.MaxManifestEntries > d.MaxManifestEntries || l.MaxEventJSONBytes <= 0 || l.MaxEventJSONBytes > d.MaxEventJSONBytes || l.MaxEventsPerAttempt <= 0 || l.MaxEventsPerAttempt > d.MaxEventsPerAttempt || l.MaxEventsPerBundle <= 0 || l.MaxEventsPerBundle > d.MaxEventsPerBundle || l.MaxEventStreamBytesPerAttempt <= 0 || l.MaxEventStreamBytesPerAttempt > d.MaxEventStreamBytesPerAttempt || l.MaxLogBytesPerStream <= 0 || l.MaxLogBytesPerStream > d.MaxLogBytesPerStream || l.MaxTotalLogBytes <= 0 || l.MaxTotalLogBytes > d.MaxTotalLogBytes || l.MaxArtifactsPerAttempt <= 0 || l.MaxArtifactsPerAttempt > d.MaxArtifactsPerAttempt || l.MaxArtifactsPerBundle <= 0 || l.MaxArtifactsPerBundle > d.MaxArtifactsPerBundle || l.MaxSingleArtifactBytes <= 0 || l.MaxSingleArtifactBytes > d.MaxSingleArtifactBytes || l.MaxTotalArtifactBytes <= 0 || l.MaxTotalArtifactBytes > d.MaxTotalArtifactBytes || l.MaxArtifactIndexBytes <= 0 || l.MaxArtifactIndexBytes > d.MaxArtifactIndexBytes || l.MaxLimitations <= 0 || l.MaxLimitations > d.MaxLimitations || l.MaxLimitationCodeBytes <= 0 || l.MaxLimitationCodeBytes > d.MaxLimitationCodeBytes || l.MaxLimitationMessageBytes <= 0 || l.MaxLimitationMessageBytes > d.MaxLimitationMessageBytes || l.MaxLogicalPathBytes <= 0 || l.MaxLogicalPathBytes > d.MaxLogicalPathBytes || l.MaxPhysicalEntryPathBytes <= 0 || l.MaxPhysicalEntryPathBytes > d.MaxPhysicalEntryPathBytes || l.MaxDuration <= 0 || l.MaxDuration > d.MaxDuration {
		return errCode(CodeInvalidLimits, "limits", "validate", "limits must be positive and no greater than absolute ceilings", nil)
	}
	return nil
}
