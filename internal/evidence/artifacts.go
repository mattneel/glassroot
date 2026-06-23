package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/mattneel/glassroot/internal/model"
)

func (s *Session) AddArtifact(ctx context.Context, input ArtifactInput) (ArtifactCaptureResult, error) {
	if err := s.ensureActive("artifact"); err != nil {
		return ArtifactRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return ArtifactRecord{}, s.fail(contextErr(err))
	}
	as, err := s.attemptForKey(input.Attempt)
	if err != nil {
		return ArtifactRecord{}, err
	}
	if input.Reader == nil {
		return ArtifactRecord{}, errCode(CodeInvalidArtifact, "artifact", "reader", "artifact reader is required", nil)
	}
	if err := ValidateLogicalArtifactPath(input.LogicalPath); err != nil {
		return ArtifactRecord{}, err
	}
	logicalKey := attemptKeyString(input.Attempt) + "\x00" + input.LogicalPath
	if _, ok := s.artifactLogical[logicalKey]; ok {
		return ArtifactRecord{}, errCode(CodeDuplicateArtifact, "artifact", "logical-path", "duplicate logical artifact path", nil)
	}
	s.artifactLogical[logicalKey] = struct{}{}
	if len(as.artifactRecords) >= s.writer.limits.MaxArtifactsPerAttempt || len(s.artifactRecords) >= s.writer.limits.MaxArtifactsPerBundle {
		rec := ArtifactRecord{LogicalPath: input.LogicalPath, Attempt: input.Attempt, Disposition: ArtifactDispositionOmittedLimit, DeclaredSize: cloneInt64Ptr(input.DeclaredSize), ObservedAtLeast: 0, Limitations: []model.Limitation{{ID: "artifact-omitted-limit", Summary: "artifact count exceeds capture limit"}}}
		return s.recordArtifact(as, rec)
	}
	max := input.MaxBytes
	if max <= 0 || max > s.writer.limits.MaxSingleArtifactBytes {
		max = s.writer.limits.MaxSingleArtifactBytes
	}
	totalRemaining := s.writer.limits.MaxTotalArtifactBytes - s.totalArtifactBytes
	if totalRemaining < max {
		max = totalRemaining
	}
	if max <= 0 {
		rec := ArtifactRecord{LogicalPath: input.LogicalPath, Attempt: input.Attempt, Disposition: ArtifactDispositionOmittedLimit, DeclaredSize: cloneInt64Ptr(input.DeclaredSize), ObservedAtLeast: 0, Limitations: []model.Limitation{{ID: "artifact-omitted-limit", Summary: "total artifact byte limit exhausted"}}}
		return s.recordArtifact(as, rec)
	}
	if input.DeclaredSize != nil && *input.DeclaredSize > max {
		rec := ArtifactRecord{LogicalPath: input.LogicalPath, Attempt: input.Attempt, Disposition: ArtifactDispositionOmittedLimit, DeclaredSize: cloneInt64Ptr(input.DeclaredSize), ObservedAtLeast: 0, Limitations: []model.Limitation{{ID: "artifact-omitted-limit", Summary: "artifact declared size exceeds capture limit"}}}
		return s.recordArtifact(as, rec)
	}

	tmp, f, err := s.createTempObject()
	if err != nil {
		return ArtifactRecord{}, s.fail(err)
	}
	digest, stored, observed, overLimit, err := streamArtifact(ctx, input.Reader, f, max)
	if err != nil {
		_ = f.Close()
		_ = s.root.Remove(tmp)
		return ArtifactRecord{}, s.fail(err)
	}
	if overLimit {
		_ = f.Close()
		_ = s.root.Remove(tmp)
		rec := ArtifactRecord{LogicalPath: input.LogicalPath, Attempt: input.Attempt, Disposition: ArtifactDispositionOmittedLimit, DeclaredSize: cloneInt64Ptr(input.DeclaredSize), ObservedAtLeast: observed, Limitations: []model.Limitation{{ID: "artifact-omitted-limit", Summary: "artifact exceeded capture limit"}}}
		return s.recordArtifact(as, rec)
	}
	if err := syncFile(f, s.writer.hooks); err != nil {
		_ = f.Close()
		_ = s.root.Remove(tmp)
		return ArtifactRecord{}, s.fail(pathErr(CodeSyncFailed, "artifact", "sync", tmp, "sync artifact object", err))
	}
	if err := f.Close(); err != nil {
		_ = s.root.Remove(tmp)
		return ArtifactRecord{}, s.fail(pathErr(CodeArtifactWriteFailed, "artifact", "close", tmp, "close artifact object", err))
	}
	objectPath := objectPathForDigest(digest)
	if _, ok := s.objectDigests[digest]; !ok {
		if err := s.mkdir("objects/sha256/" + string(digest)[len("sha256:"):len("sha256:")+2]); err != nil && !isExist(err) {
			_ = s.root.Remove(tmp)
			return ArtifactRecord{}, s.fail(err)
		}
		if err := s.root.Rename(tmp, objectPath); err != nil {
			_ = s.root.Remove(tmp)
			code := CodeArtifactWriteFailed
			if errors.Is(err, os.ErrExist) {
				code = CodeDestinationEntryExists
			}
			return ArtifactRecord{}, s.fail(pathErr(code, "artifact", "rename", objectPath, "publish artifact object", err))
		}
		if err := s.addEntry(ManifestEntry{Path: objectPath, Role: EntryRoleArtifactObject, MediaType: "application/octet-stream", Digest: digest, SizeBytes: stored, CaptureState: CaptureStateCaptured}); err != nil {
			return ArtifactRecord{}, s.fail(err)
		}
		s.objectDigests[digest] = objectPath
	} else {
		_ = s.root.Remove(tmp)
	}
	s.totalArtifactBytes += stored
	rec := ArtifactRecord{LogicalPath: input.LogicalPath, Attempt: input.Attempt, Disposition: ArtifactDispositionStored, Digest: digest, StoredSizeBytes: stored, DeclaredSize: cloneInt64Ptr(input.DeclaredSize), ObjectPath: objectPath, MediaType: input.MediaType, Limitations: []model.Limitation{}}
	return s.recordArtifact(as, rec)
}

func (s *Session) createTempObject() (string, *os.File, error) {
	name, err := randomName("tmp-", "")
	if err != nil {
		return "", nil, err
	}
	rel := "objects/sha256/" + name
	f, err := s.openExclusive(rel)
	if err != nil {
		return "", nil, err
	}
	return rel, f, nil
}

func streamArtifact(ctx context.Context, r io.Reader, dst io.Writer, max int64) (model.Digest, int64, int64, bool, error) {
	h := sha256.New()
	var stored int64
	var observed int64
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", stored, observed, false, contextErr(err)
		}
		n, readErr := r.Read(buf)
		if n > 0 {
			observed += int64(n)
			if observed > max {
				return "", stored, observed, true, nil
			}
			chunk := buf[:n]
			wrote, writeErr := dst.Write(chunk)
			if writeErr != nil || wrote != len(chunk) {
				if writeErr == nil {
					writeErr = io.ErrShortWrite
				}
				return "", stored, observed, false, errCode(CodeArtifactWriteFailed, "artifact", "write", "write artifact object", writeErr)
			}
			_, _ = h.Write(chunk)
			stored += int64(wrote)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", stored, observed, false, errCode(CodeArtifactWriteFailed, "artifact", "read", "read artifact bytes", readErr)
		}
	}
	return model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil))), stored, observed, false, nil
}

func (s *Session) recordArtifact(as *attemptState, rec ArtifactRecord) (ArtifactRecord, error) {
	as.artifactRecords = append(as.artifactRecords, cloneArtifactRecord(rec))
	s.artifactRecords = append(s.artifactRecords, cloneArtifactRecord(rec))
	as.artifactsState = CaptureStateCaptured
	if rec.Disposition == ArtifactDispositionOmittedLimit || rec.Disposition == ArtifactDispositionFailed {
		as.artifactsState = CaptureStateOmittedLimit
		s.evidenceIncomplete = true
	}
	return cloneArtifactRecord(rec), nil
}

func (s *Session) writeArtifactIndex(as *attemptState) error {
	doc := ArtifactIndexDocument{SchemaVersion: model.SchemaVersionArtifactIndexV1Alpha1, Attempt: as.key, Artifacts: cloneArtifactRecords(as.artifactRecords)}
	data, err := json.Marshal(doc)
	if err != nil {
		return errCode(CodeSerializationFailed, "artifact", "json", "marshal artifact index", err)
	}
	if int64(len(data)) > s.writer.limits.MaxArtifactIndexBytes {
		return attemptErr(CodeArtifactLimit, "artifact", "index", as.attemptID, "artifact index too large", nil)
	}
	return s.writePayload(as.dir+"/artifacts.json", EntryRoleArtifactIndex, "application/json", as, data)
}

func objectPathForDigest(d model.Digest) string {
	hex := strings.TrimPrefix(string(d), "sha256:")
	return "objects/sha256/" + hex[:2] + "/" + hex
}
func cloneInt64Ptr(p *int64) *int64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
