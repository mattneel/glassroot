package evidence

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"strings"

	"github.com/mattneel/glassroot/internal/model"
)

const manifestDigestDomain = "glassroot.dev/evidence-manifest-json/v1\x00"
const runPlanJSONDigestDomain = "glassroot.dev/run-plan-json/v1\x00"
const eventIDDomain = "glassroot.dev/observation-event-id/v1\x00"

func digestBytes(data []byte) model.Digest {
	sum := sha256.Sum256(data)
	return model.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func manifestDigest(data []byte) model.Digest {
	h := sha256.New()
	_, _ = h.Write([]byte(manifestDigestDomain))
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(len(data)))
	_, _ = h.Write(b[:])
	_, _ = h.Write(data)
	return model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil)))
}

func planJSONDigest(data []byte) model.Digest {
	h := sha256.New()
	_, _ = h.Write([]byte(runPlanJSONDigestDomain))
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(len(data)))
	_, _ = h.Write(b[:])
	_, _ = h.Write(data)
	return model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil)))
}

func expectedEventID(planDigest model.Digest, runID string, seq uint64) string {
	h := sha256.New()
	_, _ = h.Write([]byte(eventIDDomain))
	writeLenBytes(h, []byte(planDigest))
	writeLenBytes(h, []byte(runID))
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], seq)
	_, _ = h.Write(b[:])
	return "evt-" + hex.EncodeToString(h.Sum(nil))
}

func writeLenBytes(w hash.Hash, data []byte) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(len(data)))
	_, _ = w.Write(b[:])
	_, _ = w.Write(data)
}

func validDigest(d model.Digest) bool {
	s := string(d)
	if len(s) != len("sha256:")+64 || !strings.HasPrefix(s, "sha256:") {
		return false
	}
	for _, r := range s[len("sha256:"):] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
