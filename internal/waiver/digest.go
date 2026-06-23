package waiver

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"time"

	"github.com/mattneel/glassroot/internal/model"
)

type digestWriter interface{ Write([]byte) (int, error) }

func writeU64(w digestWriter, v uint64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	_, _ = w.Write(b[:])
}
func writeString(w digestWriter, s string) { writeU64(w, uint64(len(s))); _, _ = w.Write([]byte(s)) }
func rawDigest(data []byte) model.Digest {
	sum := sha256.Sum256(data)
	return model.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
func semanticDigest(set WaiverSet) model.Digest {
	h := sha256.New()
	_, _ = h.Write([]byte(semanticDigestDomain))
	writeString(h, set.APIVersion)
	writeString(h, set.Kind)
	writeString(h, set.MetadataName)
	waivers := append([]Waiver(nil), set.Waivers...)
	sort.SliceStable(waivers, func(i, j int) bool { return waivers[i].ID < waivers[j].ID })
	writeU64(h, uint64(len(waivers)))
	for _, w := range waivers {
		writeString(h, w.ID)
		writeString(h, w.Target.FindingID)
		writeString(h, w.Target.RuleID)
		writeString(h, w.Owner)
		writeString(h, w.Reason)
		writeTime(h, w.IssuedAt)
		writeTime(h, w.ExpiresAt)
	}
	return model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil)))
}
func writeTime(w digestWriter, t time.Time) { writeString(w, t.UTC().Format(time.RFC3339)) }
