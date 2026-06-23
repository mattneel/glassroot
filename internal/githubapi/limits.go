package githubapi

import "time"

const (
	GitHubRESTOrigin = "https://api.github.com"
	GitHubAPIVersion = "2026-03-10"
	UserAgent        = "glassroot-credential-broker/1"
	MaxTokenBytes    = 16 << 10
)

type Limits struct {
	MaxAPIResponseBytes       int
	MaxJSONDepth              int
	MaxJSONTokens             int
	MaxJSONStringBytes        int
	MaxHeaderValueBytes       int
	MaxInstallationTokenBytes int
	RequestTimeout            time.Duration
}

func DefaultLimits() Limits {
	return Limits{MaxAPIResponseBytes: 1 << 20, MaxJSONDepth: 32, MaxJSONTokens: 100000, MaxJSONStringBytes: 256 << 10, MaxHeaderValueBytes: 8 << 10, MaxInstallationTokenBytes: MaxTokenBytes, RequestTimeout: 20 * time.Second}
}
func validateLimits(l Limits) error {
	if l.MaxAPIResponseBytes <= 0 || l.MaxAPIResponseBytes > 1<<20 || l.MaxJSONDepth <= 0 || l.MaxJSONDepth > 64 || l.MaxJSONTokens <= 0 || l.MaxJSONTokens > 250000 || l.MaxJSONStringBytes <= 0 || l.MaxJSONStringBytes > 1<<20 || l.MaxHeaderValueBytes <= 0 || l.MaxHeaderValueBytes > 8<<10 || l.MaxInstallationTokenBytes <= 0 || l.MaxInstallationTokenBytes > MaxTokenBytes || l.RequestTimeout <= 0 || l.RequestTimeout > 60*time.Second {
		return errCode(CodeResponseInvalid, "limits", "limits rejected", nil)
	}
	return nil
}
