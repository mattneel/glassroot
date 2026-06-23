package gitstore

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
)

type ObjectFormat string

const (
	ObjectFormatSHA1   ObjectFormat = "sha1"
	ObjectFormatSHA256 ObjectFormat = "sha256"
)

func (f ObjectFormat) ObjectIDLength() int {
	switch f {
	case ObjectFormatSHA1:
		return 40
	case ObjectFormatSHA256:
		return 64
	default:
		return 0
	}
}

func (f ObjectFormat) newHash() hash.Hash {
	switch f {
	case ObjectFormatSHA1:
		return sha1.New()
	case ObjectFormatSHA256:
		return sha256.New()
	default:
		return nil
	}
}

func validateObjectID(id string, format ObjectFormat, allowUpper bool) (string, error) {
	if len(id) != format.ObjectIDLength() {
		return "", gitErr(CodeInvalidObjectID, "object", "validate", fmt.Sprintf("object id has length %d", len(id)), nil)
	}
	for _, r := range id {
		if r >= '0' && r <= '9' {
			continue
		}
		if r >= 'a' && r <= 'f' {
			continue
		}
		if allowUpper && r >= 'A' && r <= 'F' {
			continue
		}
		return "", gitErr(CodeInvalidObjectID, "object", "validate", "object id must be hexadecimal", nil)
	}
	return strings.ToLower(id), nil
}

func gitObjectID(format ObjectFormat, typ string, data []byte) (string, error) {
	h := format.newHash()
	if h == nil {
		return "", gitErr(CodeUnsupportedObjectFormat, "object", "hash", string(format), nil)
	}
	_, _ = h.Write([]byte(typ))
	_, _ = h.Write([]byte(" "))
	_, _ = h.Write([]byte(fmt.Sprintf("%d", len(data))))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func contentDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
