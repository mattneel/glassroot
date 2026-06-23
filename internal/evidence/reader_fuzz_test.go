package evidence

import (
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/model"
)

func FuzzStrictJSONPreflight(f *testing.F) {
	f.Add([]byte(`{"schemaVersion":"glassroot.dev/evidence-manifest/v1alpha1","id":"bundle-run-0001","runId":"run-0001","createdAt":"2026-02-03T04:05:06Z","bundleFormatVersion":"directory-v1alpha1","planDigest":"sha256:` + strings.Repeat("1", 64) + `","executionComplete":true,"evidenceComplete":true,"bundleTransactionValid":true,"entries":[],"artifacts":[],"attempts":[],"limitations":[]}`))
	f.Add([]byte(`{"runId":"a","runId":"b"}`))
	f.Add([]byte("\xef\xbb\xbf{}"))
	f.Add([]byte(`{"RunId":"run-0001"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = strictJSONPreflight(data, DefaultReaderLimits(), strictManifest)
	})
}

func FuzzParseEventJSONLine(f *testing.F) {
	f.Add([]byte(`{"schemaVersion":"glassroot.dev/observation-event/v1alpha1","id":"evt-` + strings.Repeat("1", 64) + `","runId":"run-0001","revision":"base","scenarioId":"test","repetition":1,"sequenceNumber":1,"observedAt":"2026-02-03T04:05:06Z","source":"synthetic-test-generated","kind":"observer-warning","observerWarning":{"code":"seed","message":"seed","unsupported":false,"limitations":[]}}`))
	f.Add([]byte(`{"schemaVersion":"bad"}`))
	f.Add([]byte(`{"schemaVersion":"glassroot.dev/observation-event/v1alpha1","schemaVersion":"x"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = decodeEventStrict(data, DefaultReaderLimits())
	})
}

func FuzzReconcileBundleInventory(f *testing.F) {
	f.Add("plan.json")
	f.Add("../escape")
	f.Add("attempts/base/test/repetition-0001/events.jsonl")
	f.Add("objects/sha256/aa/" + strings.Repeat("a", 64))
	f.Fuzz(func(t *testing.T, p string) {
		_ = validatePhysicalPathForReader(p, DefaultReaderLimits())
		files := map[string]struct{}{p: {}}
		_ = expectedDirsForFiles(files)
	})
}

func FuzzValidateArtifactReferences(f *testing.F) {
	f.Add("/workspace/out.bin", "sha256:"+strings.Repeat("a", 64), int64(3))
	f.Add("../bad", "nope", int64(-1))
	f.Fuzz(func(t *testing.T, logical string, digest string, size int64) {
		_ = ValidateLogicalArtifactPath(logical)
		d := model.Digest(digest)
		if validDigest(d) {
			p := objectPathForDigest(d)
			_ = validatePhysicalPathForReader(p, DefaultReaderLimits())
		}
		_ = ArtifactRecord{LogicalPath: logical, Digest: d, StoredSizeBytes: size}
	})
}
