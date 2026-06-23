package githubsourcestore

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"
)

const (
	SchemaSourceStoreV1Alpha1             = "glassroot.dev/github-source-store/v1alpha1"
	ImportProfileSmartHTTPShallowV1Alpha1 = "glassroot.dev/github-source-import/smart-http-shallow/v1alpha1"
	domainSourceStoreID                   = "glassroot.dev/github-source-store-id/v1\x00"
	domainMetadataJSONDigest              = "glassroot.dev/github-source-store-metadata-json/v1\x00"
)

type Identity struct {
	ImportProfileVersion string
	TargetID             string
	BaseRepositoryID     int64
	HeadRepositoryID     int64
	PullRequestNumber    int64
	BaseCommitID         string
	HeadCommitID         string
}

func ComputeSourceStoreID(id Identity) (string, error) {
	if err := validateIdentity(id); err != nil {
		return "", err
	}
	return "source-" + hashDomain(domainSourceStoreID,
		id.ImportProfileVersion,
		id.TargetID,
		strconv.FormatInt(id.BaseRepositoryID, 10),
		strconv.FormatInt(id.HeadRepositoryID, 10),
		strconv.FormatInt(id.PullRequestNumber, 10),
		id.BaseCommitID,
		id.HeadCommitID,
	), nil
}

func ValidateSourceStoreID(s string) error {
	if len(s) != len("source-")+64 || !strings.HasPrefix(s, "source-") || !isLowerHex(s[len("source-"):], 64) {
		return errCode(CodeInvalidSourceStoreID, "identity", "source store id rejected", nil)
	}
	return nil
}

func validateIdentity(id Identity) error {
	if id.ImportProfileVersion != ImportProfileSmartHTTPShallowV1Alpha1 || !validPrefixedHash(id.TargetID, "target-") || id.BaseRepositoryID <= 0 || id.HeadRepositoryID <= 0 || id.PullRequestNumber <= 0 || !validObjectIDAny(id.BaseCommitID) || !validObjectIDAny(id.HeadCommitID) || len(id.BaseCommitID) != len(id.HeadCommitID) {
		return errCode(CodeMetadataInvalid, "identity", "source identity rejected", nil)
	}
	return nil
}

func validPrefixedHash(s, prefix string) bool {
	return len(s) == len(prefix)+64 && strings.HasPrefix(s, prefix) && isLowerHex(s[len(prefix):], 64)
}
func validObjectIDAny(s string) bool { return isLowerHex(s, 40) || isLowerHex(s, 64) }
func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if r < '0' || (r > '9' && r < 'a') || r > 'f' {
			return false
		}
	}
	return true
}
func hashDomain(domain string, fields ...string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	var buf [8]byte
	for _, f := range fields {
		binary.BigEndian.PutUint64(buf[:], uint64(len(f)))
		_, _ = h.Write(buf[:])
		_, _ = h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}
