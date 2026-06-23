package githubsource

import "time"

type Limits struct {
	MaxRouteSegmentBytes int
	SourceLeaseDuration  time.Duration
	ImportTimeout        time.Duration
	MaxImportAttempts    int64
}

func DefaultLimits() Limits {
	return Limits{MaxRouteSegmentBytes: 256, SourceLeaseDuration: 15 * time.Minute, ImportTimeout: 10 * time.Minute, MaxImportAttempts: 3}
}

func validateLimits(l Limits) error {
	if l.MaxRouteSegmentBytes <= 0 || l.MaxRouteSegmentBytes > 256 || l.SourceLeaseDuration <= 0 || l.SourceLeaseDuration > 15*time.Minute || l.ImportTimeout <= 0 || l.ImportTimeout > 10*time.Minute || l.MaxImportAttempts <= 0 || l.MaxImportAttempts > 3 {
		return errCode(CodeInvalidLimits, "limits", "limits rejected", nil)
	}
	return nil
}
