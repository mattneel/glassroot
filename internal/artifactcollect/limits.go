package artifactcollect

import "time"

const (
	MaxWorkspacePathBytes           = 4096
	MaxInventoryEntries             = 100000
	MaxDirectories                  = 50000
	MaxRegularFiles                 = 75000
	MaxSymlinks                     = 25000
	MaxSpecialEntries               = 10000
	MaxInventoryMetadataBytes       = 64 << 20
	MaxPathBytes                    = 4096
	MaxPathComponentBytes           = 255
	MaxPathDepth                    = 128
	MaxArtifactRules                = 64
	MaxPatternBytes                 = 4096
	MaxPatternComponents            = 128
	MaxPatternStates                = 1000000
	MaxMatchedArtifacts             = 1000
	MaxSingleArtifactBytes    int64 = 1 << 30
	MaxTotalArtifactBytes     int64 = 8 << 30
	MaxLimitations                  = 1000
	MaxCollectionDuration           = 5 * time.Minute
)

type Limits struct {
	MaxWorkspacePathBytes     int
	MaxInventoryEntries       int
	MaxDirectories            int
	MaxRegularFiles           int
	MaxSymlinks               int
	MaxSpecialEntries         int
	MaxInventoryMetadataBytes int64
	MaxPathBytes              int
	MaxPathComponentBytes     int
	MaxPathDepth              int
	MaxArtifactRules          int
	MaxPatternBytes           int
	MaxPatternComponents      int
	MaxPatternStates          int
	MaxMatchedArtifacts       int
	MaxSingleArtifactBytes    int64
	MaxTotalArtifactBytes     int64
	MaxLimitations            int
	MaxCollectionDuration     time.Duration
}

func DefaultLimits() Limits {
	return Limits{
		MaxWorkspacePathBytes:     MaxWorkspacePathBytes,
		MaxInventoryEntries:       MaxInventoryEntries,
		MaxDirectories:            MaxDirectories,
		MaxRegularFiles:           MaxRegularFiles,
		MaxSymlinks:               MaxSymlinks,
		MaxSpecialEntries:         MaxSpecialEntries,
		MaxInventoryMetadataBytes: MaxInventoryMetadataBytes,
		MaxPathBytes:              MaxPathBytes,
		MaxPathComponentBytes:     MaxPathComponentBytes,
		MaxPathDepth:              MaxPathDepth,
		MaxArtifactRules:          MaxArtifactRules,
		MaxPatternBytes:           MaxPatternBytes,
		MaxPatternComponents:      MaxPatternComponents,
		MaxPatternStates:          MaxPatternStates,
		MaxMatchedArtifacts:       MaxMatchedArtifacts,
		MaxSingleArtifactBytes:    MaxSingleArtifactBytes,
		MaxTotalArtifactBytes:     MaxTotalArtifactBytes,
		MaxLimitations:            MaxLimitations,
		MaxCollectionDuration:     MaxCollectionDuration,
	}
}

func (l Limits) validate() error {
	d := DefaultLimits()
	if l.MaxWorkspacePathBytes <= 0 || l.MaxWorkspacePathBytes > d.MaxWorkspacePathBytes ||
		l.MaxInventoryEntries <= 0 || l.MaxInventoryEntries > d.MaxInventoryEntries ||
		l.MaxDirectories <= 0 || l.MaxDirectories > d.MaxDirectories ||
		l.MaxRegularFiles <= 0 || l.MaxRegularFiles > d.MaxRegularFiles ||
		l.MaxSymlinks <= 0 || l.MaxSymlinks > d.MaxSymlinks ||
		l.MaxSpecialEntries <= 0 || l.MaxSpecialEntries > d.MaxSpecialEntries ||
		l.MaxInventoryMetadataBytes <= 0 || l.MaxInventoryMetadataBytes > d.MaxInventoryMetadataBytes ||
		l.MaxPathBytes <= 0 || l.MaxPathBytes > d.MaxPathBytes ||
		l.MaxPathComponentBytes <= 0 || l.MaxPathComponentBytes > d.MaxPathComponentBytes ||
		l.MaxPathDepth <= 0 || l.MaxPathDepth > d.MaxPathDepth ||
		l.MaxArtifactRules <= 0 || l.MaxArtifactRules > d.MaxArtifactRules ||
		l.MaxPatternBytes <= 0 || l.MaxPatternBytes > d.MaxPatternBytes ||
		l.MaxPatternComponents <= 0 || l.MaxPatternComponents > d.MaxPatternComponents ||
		l.MaxPatternStates <= 0 || l.MaxPatternStates > d.MaxPatternStates ||
		l.MaxMatchedArtifacts <= 0 || l.MaxMatchedArtifacts > d.MaxMatchedArtifacts ||
		l.MaxSingleArtifactBytes <= 0 || l.MaxSingleArtifactBytes > d.MaxSingleArtifactBytes ||
		l.MaxTotalArtifactBytes <= 0 || l.MaxTotalArtifactBytes > d.MaxTotalArtifactBytes ||
		l.MaxLimitations <= 0 || l.MaxLimitations > d.MaxLimitations ||
		l.MaxCollectionDuration <= 0 || l.MaxCollectionDuration > d.MaxCollectionDuration {
		return errCode(CodeInvalidLimits, "limits", "", "collector limits are outside supported bounds", nil)
	}
	return nil
}
