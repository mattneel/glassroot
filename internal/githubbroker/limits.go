package githubbroker

import "time"

type Limits struct {
	MaxPathBytes             int
	MaxRequestFrameBytes     int
	MaxResponseFrameBytes    int
	MaxConcurrentConnections int
	MaxRequestsInFlight      int
	PerConnectionTimeout     time.Duration
	MaxTokenBytes            int
}

func DefaultLimits() Limits {
	return Limits{MaxPathBytes: 4096, MaxRequestFrameBytes: 64 << 10, MaxResponseFrameBytes: 32 << 10, MaxConcurrentConnections: 32, MaxRequestsInFlight: 16, PerConnectionTimeout: 20 * time.Second, MaxTokenBytes: 16 << 10}
}
func validateLimits(l Limits) error {
	if l.MaxPathBytes <= 0 || l.MaxPathBytes > 4096 || l.MaxRequestFrameBytes <= 0 || l.MaxRequestFrameBytes > 64<<10 || l.MaxResponseFrameBytes <= 0 || l.MaxResponseFrameBytes > 32<<10 || l.MaxConcurrentConnections <= 0 || l.MaxConcurrentConnections > 128 || l.MaxRequestsInFlight <= 0 || l.MaxRequestsInFlight > 64 || l.PerConnectionTimeout <= 0 || l.PerConnectionTimeout > 60*time.Second || l.MaxTokenBytes <= 0 || l.MaxTokenBytes > 16<<10 {
		return errCode(CodeInvalidLimits, "limits", "limits rejected", nil)
	}
	return nil
}
