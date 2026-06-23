package materialize

import "time"

const (
	MaxEntries                      = 100000
	MaxDirectories                  = 50000
	MaxRegularFiles                 = 75000
	MaxSymlinks                     = 10000
	MaxGitlinks                     = 10000
	MaxSingleFileBytes        int64 = 128 << 20
	MaxTotalBlobBytes         int64 = 8 << 30
	MaxSymlinkTargetBytes           = 4 << 10
	MaxInventoryMetadataBytes       = 64 << 20
	MaxPathBytes                    = 4096
	MaxPathComponentBytes           = 255
	MaxPathDepth                    = 128
	MaxReportedEntries              = 100000
	MaxReportedLimitations          = 1000
)

const MaxMaterializationDuration = 10 * time.Minute

type Limits struct {
	MaxEntries                int
	MaxDirectories            int
	MaxRegularFiles           int
	MaxSymlinks               int
	MaxGitlinks               int
	MaxSingleFileBytes        int64
	MaxTotalBlobBytes         int64
	MaxSymlinkTargetBytes     int
	MaxInventoryMetadataBytes int
	MaxPathBytes              int
	MaxPathComponentBytes     int
	MaxPathDepth              int
	MaxReportedEntries        int
	MaxReportedLimitations    int
	MaxDuration               time.Duration
}

func DefaultLimits() Limits {
	return Limits{
		MaxEntries:                MaxEntries,
		MaxDirectories:            MaxDirectories,
		MaxRegularFiles:           MaxRegularFiles,
		MaxSymlinks:               MaxSymlinks,
		MaxGitlinks:               MaxGitlinks,
		MaxSingleFileBytes:        MaxSingleFileBytes,
		MaxTotalBlobBytes:         MaxTotalBlobBytes,
		MaxSymlinkTargetBytes:     MaxSymlinkTargetBytes,
		MaxInventoryMetadataBytes: MaxInventoryMetadataBytes,
		MaxPathBytes:              MaxPathBytes,
		MaxPathComponentBytes:     MaxPathComponentBytes,
		MaxPathDepth:              MaxPathDepth,
		MaxReportedEntries:        MaxReportedEntries,
		MaxReportedLimitations:    MaxReportedLimitations,
		MaxDuration:               MaxMaterializationDuration,
	}
}

func validateLimits(l Limits) error {
	defaults := DefaultLimits()
	if l.MaxEntries <= 0 || l.MaxEntries > defaults.MaxEntries ||
		l.MaxDirectories <= 0 || l.MaxDirectories > defaults.MaxDirectories ||
		l.MaxRegularFiles <= 0 || l.MaxRegularFiles > defaults.MaxRegularFiles ||
		l.MaxSymlinks <= 0 || l.MaxSymlinks > defaults.MaxSymlinks ||
		l.MaxGitlinks <= 0 || l.MaxGitlinks > defaults.MaxGitlinks ||
		l.MaxSingleFileBytes <= 0 || l.MaxSingleFileBytes > defaults.MaxSingleFileBytes ||
		l.MaxTotalBlobBytes <= 0 || l.MaxTotalBlobBytes > defaults.MaxTotalBlobBytes ||
		l.MaxSymlinkTargetBytes <= 0 || l.MaxSymlinkTargetBytes > defaults.MaxSymlinkTargetBytes ||
		l.MaxInventoryMetadataBytes <= 0 || l.MaxInventoryMetadataBytes > defaults.MaxInventoryMetadataBytes ||
		l.MaxPathBytes <= 0 || l.MaxPathBytes > defaults.MaxPathBytes ||
		l.MaxPathComponentBytes <= 0 || l.MaxPathComponentBytes > defaults.MaxPathComponentBytes ||
		l.MaxPathDepth <= 0 || l.MaxPathDepth > defaults.MaxPathDepth ||
		l.MaxReportedEntries <= 0 || l.MaxReportedEntries > defaults.MaxReportedEntries ||
		l.MaxReportedLimitations <= 0 || l.MaxReportedLimitations > defaults.MaxReportedLimitations ||
		l.MaxDuration <= 0 || l.MaxDuration > defaults.MaxDuration {
		return errCode(CodeEntryLimit, "limits", "validate", "limits must be positive and no greater than absolute ceilings", nil)
	}
	return nil
}
