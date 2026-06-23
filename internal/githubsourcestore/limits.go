package githubsourcestore

import "time"

type Limits struct {
	MaxSourceRootPathBytes int
	MaxMetadataBytes       int
	MaxSourceStoreBytes    int64
	MaxPackFiles           int
	MaxLooseObjects        int
	MaxRefs                int
	MaxShallowEntries      int
	MaxImportDuration      time.Duration
}

func DefaultLimits() Limits {
	return Limits{MaxSourceRootPathBytes: 4096, MaxMetadataBytes: 1 << 20, MaxSourceStoreBytes: 8 << 30, MaxPackFiles: 128, MaxLooseObjects: 250000, MaxRefs: 2, MaxShallowEntries: 2, MaxImportDuration: 10 * time.Minute}
}

func validateLimits(l Limits) error {
	if l.MaxSourceRootPathBytes <= 0 || l.MaxSourceRootPathBytes > 4096 || l.MaxMetadataBytes <= 0 || l.MaxMetadataBytes > 1<<20 || l.MaxSourceStoreBytes <= 0 || l.MaxSourceStoreBytes > 8<<30 || l.MaxPackFiles <= 0 || l.MaxPackFiles > 128 || l.MaxLooseObjects <= 0 || l.MaxLooseObjects > 250000 || l.MaxRefs <= 0 || l.MaxRefs > 2 || l.MaxShallowEntries <= 0 || l.MaxShallowEntries > 2 || l.MaxImportDuration <= 0 || l.MaxImportDuration > 10*time.Minute {
		return errCode(CodeMetadataInvalid, "limits", "limits rejected", nil)
	}
	return nil
}
