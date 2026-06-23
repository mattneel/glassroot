package evidence

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/runner"
)

const maxParentPathBytes = 4096
const createAttempts = 16

type Writer struct {
	parent string
	limits Limits
	hooks  testHooks
}

type testHooks struct {
	failStagingSync bool
	failParentSync  bool
	failFileSync    bool
	failRename      bool
	failCleanup     bool
}

func NewWriter(parentDir string, limits Limits, options ...Option) (*Writer, error) {
	if runtime.GOOS != "linux" {
		return nil, errCode(CodeUnsupportedPlatform, "platform", "new", "evidence bundle writing is initially supported only on Linux", nil)
	}
	if err := validateParent(parentDir); err != nil {
		return nil, err
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	w := &Writer{parent: parentDir, limits: limits}
	for _, opt := range options {
		if opt != nil {
			opt(w)
		}
	}
	return w, nil
}

type Option func(*Writer)

func newWriterForTest(parentDir string, limits Limits, hooks testHooks) (*Writer, error) {
	w, err := NewWriter(parentDir, limits)
	if err != nil {
		return nil, err
	}
	w.hooks = hooks
	return w, nil
}

func validateParent(parent string) error {
	if parent == "" {
		return pathErr(CodeInvalidParent, "parent", "validate", parent, "parent directory is required", nil)
	}
	if len(parent) > maxParentPathBytes || !utf8.ValidString(parent) || strings.ContainsRune(parent, 0) || containsControl(parent) {
		return pathErr(CodeInvalidParent, "parent", "validate", parent, "parent path is invalid or too large", nil)
	}
	if !filepath.IsAbs(parent) {
		return pathErr(CodeInvalidParent, "parent", "validate", parent, "parent path must be absolute", nil)
	}
	if filepath.Clean(parent) != parent {
		return pathErr(CodeInvalidParent, "parent", "validate", parent, "parent path must be clean", nil)
	}
	st, err := os.Lstat(parent)
	if err != nil {
		return pathErr(CodeInvalidParent, "parent", "lstat", parent, "stat parent", err)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return pathErr(CodeParentSymlink, "parent", "lstat", parent, "parent final component is symlink", nil)
	}
	if !st.IsDir() {
		return pathErr(CodeParentNotDirectory, "parent", "lstat", parent, "parent is not directory", nil)
	}
	return nil
}

func (w *Writer) Begin(ctx context.Context, plan *pipeline.FrozenPlan) (*Session, error) {
	if w == nil {
		return nil, errCode(CodeInvalidParent, "writer", "begin", "writer is nil", nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, contextErr(err)
	}
	if plan == nil {
		return nil, errCode(CodeInvalidPlan, "plan", "begin", "FrozenPlan is required", nil)
	}
	jsonBytes := plan.JSON()
	planDigest := plan.Digest()
	if planJSONDigest(jsonBytes) != planDigest {
		return nil, errCode(CodeInvalidPlan, "plan", "digest", "FrozenPlan JSON does not match plan digest", nil)
	}
	doc := plan.Document()
	if doc.SchemaVersion != model.SchemaVersionRunPlanV1Alpha1 || doc.RunID == "" {
		return nil, errCode(CodeInvalidPlan, "plan", "validate", "invalid plan schema or run id", nil)
	}
	if doc.Runner != (model.RunnerCapabilities{}) {
		return nil, errCode(CodeInvalidPlan, "plan", "runner", "legacy runner field must be empty", nil)
	}
	attempts, err := runner.ExpandPlanAttempts(plan)
	if err != nil {
		return nil, errCode(CodeInvalidPlan, "plan", "attempts", "expand plan attempts", err)
	}
	if len(attempts) == 0 || len(attempts) > w.limits.MaxAttempts {
		return nil, errCode(CodeInvalidPlan, "plan", "attempts", "attempt count exceeds evidence limits", nil)
	}
	staging, root, err := w.createStaging(doc.RunID)
	if err != nil {
		return nil, err
	}
	s := &Session{writer: w, state: StateActive, staging: staging, root: root, runID: doc.RunID, createdAt: doc.CreatedAt.UTC(), planDigest: planDigest, planJSON: append([]byte(nil), jsonBytes...), attempts: map[string]*attemptState{}, attemptOrder: []string{}, eventIDs: map[string]struct{}{}, objectDigests: map[model.Digest]string{}, artifactLogical: map[string]struct{}{}}
	for _, a := range attempts {
		key := AttemptKey{Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition}
		if !validScenarioID(key.ScenarioID) || (key.Revision != model.RevisionKindBase && key.Revision != model.RevisionKindHead) || key.Repetition == 0 {
			_ = s.Abort()
			return nil, errCode(CodeInvalidAttempt, "plan", "attempt", "invalid attempt coordinate", nil)
		}
		ak := attemptKeyString(key)
		if _, ok := s.attempts[ak]; ok {
			_ = s.Abort()
			return nil, errCode(CodeInvalidAttempt, "plan", "attempt", "duplicate attempt coordinate", nil)
		}
		dir := attemptDir(key)
		state := &attemptState{key: key, attemptID: a.AttemptID, ordinal: a.GlobalOrdinal, dir: dir, eventsState: CaptureStateNotProvided, stdoutState: CaptureStateNotProvided, stderrState: CaptureStateNotProvided, artifactsState: CaptureStateNotProvided, resultState: CaptureStateNotProvided, artifactRecords: []ArtifactRecord{}}
		s.attempts[ak] = state
		s.attemptOrder = append(s.attemptOrder, ak)
	}
	for _, dir := range []string{"attempts", "attempts/base", "attempts/head", "objects", "objects/sha256"} {
		if err := s.mkdir(dir); err != nil {
			return nil, cleanupSession(s, err)
		}
	}
	for _, ak := range s.attemptOrder {
		as := s.attempts[ak]
		parts := strings.Split(as.dir, "/")
		cur := ""
		for _, part := range parts {
			if cur == "" {
				cur = part
			} else {
				cur += "/" + part
			}
			if err := s.mkdir(cur); err != nil && !isExist(err) {
				return nil, cleanupSession(s, err)
			}
		}
	}
	if err := s.writePayload("plan.json", EntryRolePlan, "application/json", nil, append([]byte(nil), jsonBytes...)); err != nil {
		return nil, cleanupSession(s, err)
	}
	return s, nil
}

func (w *Writer) createStaging(runID string) (string, *os.Root, error) {
	for i := 0; i < createAttempts; i++ {
		name, err := randomName("glassroot-evidence-staging-", runID)
		if err != nil {
			return "", nil, err
		}
		p := filepath.Join(w.parent, name)
		err = os.Mkdir(p, 0o700)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", nil, pathErr(CodeStagingCreateFailed, "staging", "mkdir", p, "create staging directory", err)
		}
		root, err := os.OpenRoot(p)
		if err != nil {
			_ = os.RemoveAll(p)
			return "", nil, pathErr(CodeStagingOpenFailed, "staging", "open-root", p, "open staging root", err)
		}
		if _, err := root.Stat("."); err != nil {
			_ = root.Close()
			_ = os.RemoveAll(p)
			return "", nil, pathErr(CodeStagingOpenFailed, "staging", "stat-root", p, "verify staging root", err)
		}
		return p, root, nil
	}
	return "", nil, errCode(CodeStagingCreateFailed, "staging", "mkdir", "staging name collision retry limit exceeded", nil)
}

func randomName(prefix, runID string) (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errCode(CodeStagingCreateFailed, "random", "read", "generate random bundle name", err)
	}
	base := prefix + hex.EncodeToString(b[:])
	if runID != "" && validScenarioID(runID) {
		base = prefix + runID + "-" + hex.EncodeToString(b[:])
	}
	return base, nil
}
