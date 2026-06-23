package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

type Session struct {
	writer             *Writer
	state              State
	staging            string
	final              string
	root               *os.Root
	runID              string
	createdAt          time.Time
	planDigest         model.Digest
	planJSON           []byte
	attempts           map[string]*attemptState
	attemptOrder       []string
	currentAttempt     int
	nextSequence       uint64
	eventIDs           map[string]struct{}
	entries            []ManifestEntry
	payloads           map[string]struct{}
	artifactRecords    []ArtifactRecord
	objectDigests      map[model.Digest]string
	artifactLogical    map[string]struct{}
	limitations        []model.Limitation
	evidenceIncomplete bool
	failed             error
	totalBytes         int64
	totalEvents        uint64
	totalLogBytes      int64
	totalArtifactBytes int64
	manifestBytes      []byte
}

type attemptState struct {
	key             AttemptKey
	attemptID       string
	ordinal         uint64
	dir             string
	eventsFile      *os.File
	eventsHash      hashWriter
	eventsBytes     int64
	eventsCount     uint64
	firstSeq        uint64
	lastSeq         uint64
	eventsState     CaptureState
	stdoutState     CaptureState
	stderrState     CaptureState
	artifactsState  CaptureState
	resultState     CaptureState
	stdout          *LogCapture
	stderr          *LogCapture
	artifactRecords []ArtifactRecord
}

type hashWriter struct{ buf bytes.Buffer }

func (s *Session) State() State {
	if s == nil {
		return ""
	}
	return s.state
}
func (s *Session) stagingPathForTest() string {
	if s == nil {
		return ""
	}
	return s.staging
}
func (s *Session) finalPathForTest() string {
	if s == nil {
		return ""
	}
	return s.final
}

func (s *Session) Emit(ctx context.Context, event model.ObservationEvent) error {
	if err := s.ensureActive("event"); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return s.fail(contextErr(err))
	}
	line, err := encodeEventLine(event, s.writer.limits)
	if err != nil {
		return s.fail(err)
	}
	as, err := s.validateEventRoute(event)
	if err != nil {
		return s.fail(err)
	}
	if as.eventsFile == nil {
		if err := s.openEvents(as); err != nil {
			return s.fail(err)
		}
	}
	if int64(len(line)) > s.writer.limits.MaxEventJSONBytes || as.eventsCount >= uint64(s.writer.limits.MaxEventsPerAttempt) || s.totalEvents >= uint64(s.writer.limits.MaxEventsPerBundle) {
		return s.fail(attemptErr(CodeEventLimit, "event", "limit", as.attemptID, "event limit exceeded", nil))
	}
	if as.eventsBytes+int64(len(line)) > s.writer.limits.MaxEventStreamBytesPerAttempt {
		return s.fail(attemptErr(CodeEventLimit, "event", "bytes", as.attemptID, "event stream byte limit exceeded", nil))
	}
	n, err := as.eventsFile.Write(line)
	if err != nil || n != len(line) {
		if err == nil {
			err = io.ErrShortWrite
		}
		return s.fail(attemptErr(CodeEventWriteFailed, "event", "write", as.attemptID, "write event line", err))
	}
	as.eventsBytes += int64(n)
	as.eventsHash.buf.Write(line)
	as.eventsCount++
	s.totalEvents++
	if as.firstSeq == 0 {
		as.firstSeq = uint64(event.SequenceNumber)
	}
	as.lastSeq = uint64(event.SequenceNumber)
	as.eventsState = CaptureStateCaptured
	return nil
}

func encodeEventLine(event model.ObservationEvent, limits Limits) ([]byte, error) {
	if event.SchemaVersion != model.SchemaVersionObservationEventV1Alpha1 || event.ID == "" || event.RunID == "" || event.SequenceNumber <= 0 || event.Repetition == 0 {
		return nil, errCode(CodeInvalidEvent, "event", "validate", "invalid event envelope", nil)
	}
	draft := runner.EventDraft{ObservedAt: event.ObservedAt, Source: event.Source, Kind: event.Kind, Process: event.Process, Filesystem: event.Filesystem, Network: event.Network, Artifact: event.Artifact, Scenario: event.Scenario, ObserverWarning: event.ObserverWarning, ResourceLimit: event.ResourceLimit}
	if err := runner.ValidateEventDraft(draft, runner.Limits{MaxEventJSONBytes: limits.MaxEventJSONBytes, MaxEventsPerAttempt: int64(limits.MaxEventsPerAttempt), MaxEventsPerExecution: int64(limits.MaxEventsPerBundle), MaxAttempts: int64(limits.MaxAttempts), MaxCapabilityMismatches: runner.MaxCapabilityMismatches, MaxResultLimitations: runner.MaxResultLimitations}); err != nil {
		return nil, errCode(CodeInvalidEvent, "event", "payload", "invalid event payload", err)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return nil, errCode(CodeSerializationFailed, "event", "json", "marshal event", err)
	}
	if int64(len(data)) > limits.MaxEventJSONBytes {
		return nil, errCode(CodeEventTooLarge, "event", "json", "event JSON exceeds size limit", nil)
	}
	out := append([]byte(nil), data...)
	out = append(out, '\n')
	return out, nil
}

func (s *Session) validateEventRoute(event model.ObservationEvent) (*attemptState, error) {
	if event.RunID != s.runID {
		return nil, errCode(CodeInvalidEvent, "event", "run", "event run ID does not match plan", nil)
	}
	seq := uint64(event.SequenceNumber)
	if seq != s.nextSequence+1 {
		return nil, errCode(CodeEventOrder, "event", "sequence", "event sequence is not the expected next value", nil)
	}
	if event.ID != expectedEventID(s.planDigest, s.runID, seq) {
		return nil, errCode(CodeInvalidEvent, "event", "id", "event ID does not match deterministic encoding", nil)
	}
	if _, ok := s.eventIDs[event.ID]; ok {
		return nil, errCode(CodeDuplicateEventID, "event", "id", "duplicate event ID", nil)
	}
	ak := attemptKeyString(AttemptKey{Revision: event.Revision, ScenarioID: event.ScenarioID, Repetition: event.Repetition})
	as, ok := s.attempts[ak]
	if !ok {
		return nil, errCode(CodeInvalidAttempt, "event", "attempt", "event targets unknown attempt", nil)
	}
	idx := indexOf(s.attemptOrder, ak)
	if idx < s.currentAttempt {
		return nil, errCode(CodeEventOrder, "event", "attempt", "event returned to an earlier attempt", nil)
	}
	if idx > s.currentAttempt {
		if err := s.closeEventsForCurrent(); err != nil {
			return nil, err
		}
		s.currentAttempt = idx
	}
	s.nextSequence = seq
	s.eventIDs[event.ID] = struct{}{}
	return as, nil
}

func (s *Session) openEvents(as *attemptState) error {
	p := as.dir + "/events.jsonl"
	f, err := s.openExclusive(p)
	if err != nil {
		return err
	}
	as.eventsFile = f
	as.eventsHash = hashWriter{}
	as.eventsState = CaptureStateCapturedEmpty
	return nil
}

func (s *Session) closeEventsForCurrent() error {
	if s.currentAttempt < 0 || s.currentAttempt >= len(s.attemptOrder) {
		return nil
	}
	as := s.attempts[s.attemptOrder[s.currentAttempt]]
	return s.closeEvents(as)
}

func (s *Session) closeEvents(as *attemptState) error {
	if as == nil || as.eventsFile == nil {
		return nil
	}
	if err := syncFile(as.eventsFile, s.writer.hooks); err != nil {
		return attemptErr(CodeSyncFailed, "event", "sync", as.attemptID, "sync events file", err)
	}
	if err := as.eventsFile.Close(); err != nil {
		return attemptErr(CodeEventWriteFailed, "event", "close", as.attemptID, "close events file", err)
	}
	as.eventsFile = nil
	state := CaptureStateCaptured
	if as.eventsBytes == 0 {
		state = CaptureStateCapturedEmpty
	}
	as.eventsState = state
	return s.addEntry(ManifestEntry{Path: as.dir + "/events.jsonl", Role: EntryRoleEvents, MediaType: "application/x-ndjson", Digest: digestBytes(as.eventsHash.buf.Bytes()), SizeBytes: as.eventsBytes, Revision: as.key.Revision, ScenarioID: as.key.ScenarioID, Repetition: as.key.Repetition, CaptureState: as.eventsState})
}

func (s *Session) Commit(ctx context.Context, completion Completion) (*BundleResult, error) {
	if s == nil {
		return nil, errCode(CodeInvalidSessionState, "session", "commit", "session is nil", nil)
	}
	if s.state == StateCommitted {
		return nil, errCode(CodeInvalidSessionState, "session", "commit", "session already committed", nil)
	}
	if s.state == StateAborted {
		return nil, errCode(CodeInvalidSessionState, "session", "commit", "session aborted", nil)
	}
	if s.state == StateFailed && !completion.Incomplete {
		return nil, s.failed
	}
	if err := ctx.Err(); err != nil {
		return nil, s.fail(contextErr(err))
	}
	if !completion.Incomplete && s.evidenceIncomplete {
		return nil, errCode(CodeCompletionInvalid, "completion", "evidence", "complete bundle cannot contain incomplete evidence", nil)
	}
	s.state = StateCommitting
	if err := s.finalizeCaptures(); err != nil {
		return nil, cleanupSession(s, err)
	}
	if err := s.writeResults(completion); err != nil {
		return nil, cleanupSession(s, err)
	}
	manifest, err := s.buildManifest(completion)
	if err != nil {
		if completion.Incomplete && errors.Is(err, CodeCompletionInvalid) { /* no-op */
		}
		return nil, cleanupSession(s, err)
	}
	data, err := normalizeManifest(manifest, s.writer.limits)
	if err != nil {
		return nil, cleanupSession(s, err)
	}
	if int64(len(data)) > s.writer.limits.MaxManifestBytes {
		return nil, cleanupSession(s, errCode(CodeManifestTooLarge, "manifest", "size", "manifest exceeds limit", nil))
	}
	s.manifestBytes = append([]byte(nil), data...)
	if err := s.writeManifest(data); err != nil {
		return nil, cleanupSession(s, err)
	}
	if err := syncDir(s.staging, s.writer.hooks, "staging"); err != nil {
		return nil, cleanupSession(s, errCode(CodeSyncFailed, "publish", "sync-staging", "sync staging directory", err))
	}
	final, err := s.publish()
	if err != nil {
		return nil, cleanupSession(s, err)
	}
	s.final = final
	s.staging = ""
	s.state = StateCommitted
	return &BundleResult{Path: final, ManifestDigest: manifestDigest(data), ManifestBytes: append([]byte(nil), data...), EntryCount: len(s.entries), TotalBytes: s.totalBytes}, nil
}

func (s *Session) finalizeCaptures() error {
	for _, ak := range s.attemptOrder {
		as := s.attempts[ak]
		if err := s.closeEvents(as); err != nil {
			return err
		}
		if as.stdout != nil {
			if err := as.stdout.Close(); err != nil {
				return err
			}
		}
		if as.stderr != nil {
			if err := as.stderr.Close(); err != nil {
				return err
			}
		}
		if len(as.artifactRecords) > 0 {
			if err := s.writeArtifactIndex(as); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Session) Abort() error {
	if s == nil {
		return nil
	}
	if s.state == StateCommitted {
		return errCode(CodeInvalidSessionState, "session", "abort", "cannot abort committed session", nil)
	}
	if s.state == StateAborted {
		return nil
	}
	if s.root != nil {
		_ = s.root.Close()
		s.root = nil
	}
	p := s.staging
	s.staging = ""
	s.state = StateAborted
	if p != "" {
		if s.writer != nil && s.writer.hooks.failCleanup {
			return errCode(CodeCleanupFailed, "cleanup", "remove", "cleanup failed by test hook", nil)
		}
		if err := os.RemoveAll(p); err != nil {
			return pathErr(CodeCleanupFailed, "cleanup", "remove", p, "remove staging", err)
		}
	}
	return nil
}

func (s *Session) ensureActive(op string) error {
	if s == nil {
		return errCode(CodeInvalidSessionState, "session", op, "session is nil", nil)
	}
	if s.state != StateActive {
		return errCode(CodeInvalidSessionState, "session", op, "session is not active", nil)
	}
	return nil
}

func (s *Session) fail(err error) error {
	if err == nil {
		return nil
	}
	s.state = StateFailed
	s.failed = err
	return err
}

func cleanupSession(s *Session, primary error) error {
	if s == nil {
		return primary
	}
	cleanupErr := s.Abort()
	if cleanupErr != nil {
		return &Error{Code: CodeCleanupFailed, Stage: "cleanup", Op: "remove", Msg: "cleanup failed after primary error", Err: primary}
	}
	return primary
}

func (s *Session) mkdir(rel string) error {
	if !validRelativePath(rel, s.writer.limits.MaxPhysicalEntryPathBytes, false) {
		return pathErr(CodeInvalidEntryPath, "mkdir", "path", rel, "invalid directory path", nil)
	}
	if err := s.root.Mkdir(rel, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return pathErr(CodeStagingCreateFailed, "mkdir", "mkdir", rel, "create bundle directory", err)
	}
	return nil
}

func (s *Session) openExclusive(rel string) (*os.File, error) {
	if err := ValidateEvidenceEntryPath(rel); err != nil {
		return nil, err
	}
	f, err := s.root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil, pathErr(CodeDestinationEntryExists, "file", "open", rel, "destination entry already exists", err)
	}
	if err != nil {
		return nil, pathErr(CodeStagingCreateFailed, "file", "open", rel, "create payload file", err)
	}
	return f, nil
}

func (s *Session) writePayload(rel string, role EntryRole, media string, as *attemptState, data []byte) error {
	f, err := s.openExclusive(rel)
	if err != nil {
		return err
	}
	if n, err := f.Write(data); err != nil || n != len(data) {
		if err == nil {
			err = io.ErrShortWrite
		}
		_ = f.Close()
		return pathErr(CodeEventWriteFailed, "file", "write", rel, "write payload", err)
	}
	if err := syncFile(f, s.writer.hooks); err != nil {
		_ = f.Close()
		return pathErr(CodeSyncFailed, "file", "sync", rel, "sync payload", err)
	}
	if err := f.Close(); err != nil {
		return pathErr(CodeEventWriteFailed, "file", "close", rel, "close payload", err)
	}
	entry := ManifestEntry{Path: rel, Role: role, MediaType: media, Digest: digestBytes(data), SizeBytes: int64(len(data)), CaptureState: CaptureStateCaptured}
	if as != nil {
		entry.Revision = as.key.Revision
		entry.ScenarioID = as.key.ScenarioID
		entry.Repetition = as.key.Repetition
	}
	return s.addEntry(entry)
}

func (s *Session) addEntry(entry ManifestEntry) error {
	if err := ValidateEvidenceEntryPath(entry.Path); err != nil {
		return err
	}
	if len(s.entries) >= s.writer.limits.MaxManifestEntries || len(s.entries) >= s.writer.limits.MaxBundleEntries {
		return pathErr(CodeManifestInvariant, "manifest", "entry", entry.Path, "manifest entry limit exceeded", nil)
	}
	if _, ok := s.payloads[entry.Path]; ok {
		return pathErr(CodeManifestInvariant, "manifest", "entry", entry.Path, "duplicate manifest path", nil)
	}
	if entry.SizeBytes < 0 || s.totalBytes > s.writer.limits.MaxBundleBytes-entry.SizeBytes {
		return pathErr(CodeManifestInvariant, "manifest", "entry", entry.Path, "bundle byte limit exceeded", nil)
	}
	if s.payloads == nil {
		s.payloads = map[string]struct{}{}
	}
	s.payloads[entry.Path] = struct{}{}
	s.entries = append(s.entries, entry)
	s.totalBytes += entry.SizeBytes
	return nil
}

func syncFile(f *os.File, hooks testHooks) error {
	if hooks.failFileSync {
		return errors.New("file sync failed by test hook")
	}
	return f.Sync()
}
func syncDir(path string, hooks testHooks, role string) error {
	if role == "staging" && hooks.failStagingSync {
		return errors.New("staging directory sync failed by test hook")
	}
	if role == "parent" && hooks.failParentSync {
		return errors.New("parent directory sync failed by test hook")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func isExist(err error) bool { return errors.Is(err, fs.ErrExist) || errors.Is(err, os.ErrExist) }
func indexOf(xs []string, v string) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return -1
}
func attemptKeyString(k AttemptKey) string {
	return string(k.Revision) + "\x00" + k.ScenarioID + "\x00" + uintToString(k.Repetition)
}

func (s *Session) publish() (string, error) {
	for i := 0; i < createAttempts; i++ {
		name, err := randomName("glassroot-evidence-", s.runID)
		if err != nil {
			return "", err
		}
		final := filepath.Join(s.writer.parent, name)
		if _, err := os.Lstat(final); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", pathErr(CodePublishFailed, "publish", "lstat", final, "check final path", err)
		}
		if s.writer.hooks.failRename {
			return "", pathErr(CodePublishFailed, "publish", "rename", final, "rename failed by test hook", nil)
		}
		if err := os.Rename(s.staging, final); err != nil {
			return "", pathErr(CodePublishFailed, "publish", "rename", final, "publish staging", err)
		}
		if err := syncDir(s.writer.parent, s.writer.hooks, "parent"); err != nil {
			_ = os.RemoveAll(final)
			return "", errCode(CodeSyncFailed, "publish", "sync-parent", "sync parent directory", err)
		}
		return final, nil
	}
	return "", errCode(CodePublishCollision, "publish", "name", "final name collision retry limit exceeded", nil)
}

func sortedEntries(entries []ManifestEntry) []ManifestEntry {
	out := append([]ManifestEntry(nil), entries...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
