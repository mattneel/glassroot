package githubauth

const (
	AbsoluteMaxPathBytes       = 4096
	AbsoluteMaxPrivateKeyBytes = 64 << 10
	AbsoluteMinRSABits         = 2048
	AbsoluteMaxRSABits         = 8192
	AbsoluteMaxClientIDBytes   = 128
	AbsoluteMaxJWTBytes        = 16 << 10
)

type Limits struct {
	MaxPathBytes       int
	MaxPrivateKeyBytes int
	MinRSABits         int
	MaxRSABits         int
	MaxClientIDBytes   int
	MaxJWTBytes        int
}

func DefaultLimits() Limits {
	return Limits{MaxPathBytes: AbsoluteMaxPathBytes, MaxPrivateKeyBytes: AbsoluteMaxPrivateKeyBytes, MinRSABits: AbsoluteMinRSABits, MaxRSABits: AbsoluteMaxRSABits, MaxClientIDBytes: AbsoluteMaxClientIDBytes, MaxJWTBytes: AbsoluteMaxJWTBytes}
}
func validateLimits(l Limits) error {
	if l.MaxPathBytes <= 0 || l.MaxPathBytes > AbsoluteMaxPathBytes || l.MaxPrivateKeyBytes <= 0 || l.MaxPrivateKeyBytes > AbsoluteMaxPrivateKeyBytes || l.MinRSABits < AbsoluteMinRSABits || l.MaxRSABits > AbsoluteMaxRSABits || l.MinRSABits > l.MaxRSABits || l.MaxClientIDBytes <= 0 || l.MaxClientIDBytes > AbsoluteMaxClientIDBytes || l.MaxJWTBytes <= 0 || l.MaxJWTBytes > AbsoluteMaxJWTBytes {
		return errCode(CodeInvalidAppIdentity, "limits", "limits rejected", nil)
	}
	return nil
}
