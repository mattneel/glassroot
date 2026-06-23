package report

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/mattneel/glassroot/internal/model"
)

const (
	runPlanJSONDigestDomain = "glassroot.dev/run-plan-json/v1\x00"
	reportJSONDomain        = "glassroot.dev/report-json/v1\x00"
	markdownDigestDomain    = "glassroot.dev/report-markdown/v1\x00"
	terminalDigestDomain    = "glassroot.dev/report-terminal/v1\x00"
)

func digestBytes(domain string, data []byte) model.Digest {
	h := sha256.New()
	_, _ = h.Write([]byte(domain))
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(data)))
	_, _ = h.Write(n[:])
	_, _ = h.Write(data)
	return model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil)))
}

func validDigest(d model.Digest) bool {
	s := string(d)
	if len(s) != len("sha256:")+64 || s[:7] != "sha256:" {
		return false
	}
	for _, c := range s[7:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
