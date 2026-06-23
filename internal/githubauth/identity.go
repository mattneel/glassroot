package githubauth

import "unicode/utf8"

type AppIdentity struct {
	AppID    int64
	ClientID string
}

func ValidateAppIdentity(id AppIdentity, limits Limits) error {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return err
	}
	if id.AppID <= 0 || id.ClientID == "" || len(id.ClientID) > limits.MaxClientIDBytes || !utf8.ValidString(id.ClientID) || hasControl(id.ClientID) || !isASCII(id.ClientID) {
		return errCode(CodeInvalidAppIdentity, "identity", "app identity rejected", nil)
	}
	return nil
}
func isASCII(s string) bool {
	for _, r := range s {
		if r > 0x7f {
			return false
		}
	}
	return true
}
func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
