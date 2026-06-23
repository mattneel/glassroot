package githubinbox

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const (
	DomainIntakeFingerprint = "glassroot.dev/github-webhook-intake/v1\x00"
	DomainOutboxRecordID    = "glassroot.dev/github-outbox-record-id/v1\x00"
)

func ComputeIntakeFingerprint(receiverID, deliveryID, eventName, bodyDigest, projectionKind string) string {
	return "sha256:" + domainHash(DomainIntakeFingerprint, receiverID, deliveryID, eventName, bodyDigest, projectionKind)
}

func computeOutboxID(receiverID, deliveryID, intakeFingerprint, projectionKind string) string {
	return "outbox-" + domainHash(DomainOutboxRecordID, receiverID, deliveryID, intakeFingerprint, projectionKind)
}

func domainHash(domain string, fields ...string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	for _, f := range fields {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(len(f)))
		_, _ = h.Write(b[:])
		_, _ = h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func digestBytes(domain string, body []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(len(body)))
	_, _ = h.Write(b[:])
	_, _ = h.Write(body)
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(h.Sum(nil)))
}

func validDigest(s string) bool {
	if len(s) != len("sha256:")+64 || s[:7] != "sha256:" {
		return false
	}
	for _, r := range s[7:] {
		if r < '0' || (r > '9' && r < 'a') || r > 'f' {
			return false
		}
	}
	return true
}
