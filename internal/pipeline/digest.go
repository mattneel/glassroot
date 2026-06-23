package pipeline

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/mattneel/glassroot/internal/model"
)

const runPlanJSONDigestDomain = "glassroot.dev/run-plan-json/v1\x00"

func planJSONDigest(data []byte) model.Digest {
	return planJSONDigestForTest(data)
}

func planJSONDigestForTest(data []byte) model.Digest {
	h := sha256.New()
	_, _ = h.Write([]byte(runPlanJSONDigestDomain))
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(data)))
	_, _ = h.Write(n[:])
	_, _ = h.Write(data)
	return model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil)))
}
