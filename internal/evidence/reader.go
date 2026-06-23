package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

type ReaderOption func(*readerOptions)
type readerOptions struct{ expectedManifestDigest *model.Digest }

func WithExpectedManifestDigest(digest model.Digest) ReaderOption {
	return func(o *readerOptions) { d := digest; o.expectedManifestDigest = &d }
}

func OpenAndVerify(ctx context.Context, bundleDir string, limits ReaderLimits, options ...ReaderOption) (*Bundle, error) {
	if runtime.GOOS != "linux" {
		return nil, errCode(CodeUnsupportedPlatform, "reader", "platform", "evidence bundle verification is initially supported only on Linux", nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, readerContextErr(err)
	}
	if err := validateReaderLimits(limits); err != nil {
		return nil, err
	}
	readCtx, cancel := context.WithTimeout(ctx, limits.MaxReadDuration)
	defer cancel()
	if err := validateBundlePath(bundleDir, limits); err != nil {
		return nil, err
	}
	opts := readerOptions{}
	for _, opt := range options {
		if opt != nil {
			opt(&opts)
		}
	}
	if opts.expectedManifestDigest != nil && !validDigest(*opts.expectedManifestDigest) {
		return nil, errCode(CodeExpectedDigestMismatch, "reader", "expected-digest", "expected manifest digest is malformed", nil)
	}

	pre, err := os.Lstat(bundleDir)
	if err != nil {
		return nil, pathErr(CodeInvalidBundlePath, "reader", "lstat", bundleDir, "stat bundle directory", err)
	}
	if pre.Mode()&os.ModeSymlink != 0 {
		return nil, pathErr(CodeBundleRootSymlink, "reader", "lstat", bundleDir, "bundle root final component is a symlink", nil)
	}
	if !pre.IsDir() {
		return nil, pathErr(CodeBundleNotDirectory, "reader", "lstat", bundleDir, "bundle root is not a directory", nil)
	}
	preID, err := fileInfoIdentity(pre)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(bundleDir)
	if err != nil {
		return nil, pathErr(CodeInvalidBundlePath, "reader", "open-root", bundleDir, "open bundle root", err)
	}
	ok := false
	defer func() {
		if !ok && root != nil {
			_ = root.Close()
		}
	}()
	opened, err := root.Stat(".")
	if err != nil {
		return nil, pathErr(CodeBundleRootChanged, "reader", "stat-root", bundleDir, "stat opened root", err)
	}
	openedID, err := fileInfoIdentity(opened)
	if err != nil {
		return nil, err
	}
	if !sameIdentity(preID, openedID) {
		return nil, pathErr(CodeBundleRootChanged, "reader", "identity", bundleDir, "bundle root changed during open", nil)
	}
	post, err := os.Lstat(bundleDir)
	if err != nil {
		return nil, pathErr(CodeBundleRootChanged, "reader", "restat-root", bundleDir, "restat bundle root", err)
	}
	postID, err := fileInfoIdentity(post)
	if err != nil {
		return nil, err
	}
	if !post.IsDir() || post.Mode()&os.ModeSymlink != 0 || !sameIdentity(preID, postID) {
		return nil, pathErr(CodeBundleRootChanged, "reader", "restat-root", bundleDir, "bundle root changed after open", nil)
	}

	inv, err := collectInventory(readCtx, root, opened, limits)
	if err != nil {
		return nil, err
	}
	if _, ok := inv.files["manifest.json"]; !ok {
		return nil, pathErr(CodeManifestMissing, "manifest", "inventory", "manifest.json", "manifest.json is missing", nil)
	}
	manifestBytes, mdRaw, err := stableReadAll(readCtx, root, "manifest.json", inv.files["manifest.json"].info.Size(), limits.MaxManifestBytes)
	if err != nil {
		return nil, err
	}
	manifest, err := decodeManifestStrict(manifestBytes, limits)
	if err != nil {
		return nil, err
	}
	manifestDigestValue := manifestDigest(manifestBytes)
	if opts.expectedManifestDigest != nil && *opts.expectedManifestDigest != manifestDigestValue {
		return nil, errCode(CodeExpectedDigestMismatch, "manifest", "expected-digest", "expected manifest digest does not match computed digest", nil)
	}
	if mdRaw != digestBytes(manifestBytes) {
		return nil, errCode(CodeManifestInvalid, "manifest", "digest", "manifest raw digest computation mismatch", nil)
	}

	ctxVerify := &verifyContext{ctx: readCtx, root: root, limits: limits, inv: inv, manifest: manifest, manifestDigest: manifestDigestValue, entries: map[string]ManifestEntry{}, attemptsByKey: map[string]AttemptManifest{}, eventEntries: map[string]ManifestEntry{}, logEntries: map[string]ManifestEntry{}, artifactObjects: map[model.Digest]ManifestEntry{}, artifactRecords: map[string]ArtifactRecord{}, artifactIndexes: map[string]ArtifactIndexDocument{}, payloadData: map[string][]byte{}}
	if err := ctxVerify.verifyAll(); err != nil {
		return nil, err
	}

	verification := VerificationSummary{ManifestDigest: manifestDigestValue, InternallyConsistent: true, Limitations: []model.Limitation{}}
	if opts.expectedManifestDigest != nil {
		verification.Mode = VerificationModeExpectedManifestDigest
		verification.ExpectedManifestDigestSupplied = true
		verification.ExpectedManifestDigestMatched = true
	} else {
		verification.Mode = VerificationModeInternalConsistencyOnly
	}
	attempts := make([]VerifiedAttempt, 0, len(manifest.Attempts))
	for _, a := range manifest.Attempts {
		attempts = append(attempts, verifiedAttemptFromManifest(a))
	}
	b := &Bundle{root: root, limits: limits, manifest: cloneManifest(manifest), manifestDigest: manifestDigestValue, plan: cloneRunPlan(ctxVerify.plan), execution: cloneExecutionDocument(ctxVerify.execution), attempts: attempts, verification: verification, entries: ctxVerify.entries, attemptsByKey: map[string]VerifiedAttempt{}, eventsByAttempt: ctxVerify.eventEntries, logs: ctxVerify.logEntries, artifactRecords: ctxVerify.artifactRecords, artifactObjects: ctxVerify.artifactObjects, artifactIndexes: ctxVerify.artifactIndexes, eventOrder: ctxVerify.eventOrder, planDigest: ctxVerify.planDigest, runID: ctxVerify.plan.RunID}
	for _, a := range attempts {
		b.attemptsByKey[attemptKeyString(AttemptKey{Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition})] = a
	}
	ok = true
	return b, nil
}

func readerContextErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errCode(CodeReadTimeout, "reader", "deadline", "evidence read deadline exceeded", err)
	}
	return errCode(CodeContextCancelled, "reader", "cancelled", "context cancelled", err)
}

func validateBundlePath(p string, limits ReaderLimits) error {
	if p == "" || len(p) > limits.MaxBundlePathBytes || !utf8.ValidString(p) || strings.ContainsRune(p, 0) || containsControl(p) {
		return pathErr(CodeInvalidBundlePath, "reader", "path", p, "invalid bundle path", nil)
	}
	if !filepath.IsAbs(p) {
		return pathErr(CodeInvalidBundlePath, "reader", "path", p, "bundle path must be absolute", nil)
	}
	if filepath.Clean(p) != p {
		return pathErr(CodeInvalidBundlePath, "reader", "path", p, "bundle path must be clean", nil)
	}
	return nil
}

type verifyContext struct {
	ctx              context.Context
	root             *os.Root
	limits           ReaderLimits
	inv              bundleInventory
	manifest         Manifest
	manifestDigest   model.Digest
	entries          map[string]ManifestEntry
	attemptsByKey    map[string]AttemptManifest
	eventEntries     map[string]ManifestEntry
	logEntries       map[string]ManifestEntry
	artifactObjects  map[model.Digest]ManifestEntry
	artifactRecords  map[string]ArtifactRecord
	artifactIndexes  map[string]ArtifactIndexDocument
	payloadData      map[string][]byte
	plan             model.RunPlan
	planDigest       model.Digest
	execution        ExecutionDocument
	expectedAttempts []runner.AttemptRequest
	eventOrder       []string
}

func (v *verifyContext) verifyAll() error {
	if err := v.ctx.Err(); err != nil {
		return readerContextErr(err)
	}
	if err := v.verifyManifest(); err != nil {
		return err
	}
	if err := v.reconcileLayout(); err != nil {
		return err
	}
	if err := v.verifyPayloads(); err != nil {
		return err
	}
	if err := v.verifyPlan(); err != nil {
		return err
	}
	if err := v.verifyExecutionAndResults(); err != nil {
		return err
	}
	if err := v.verifyArtifacts(); err != nil {
		return err
	}
	if err := v.verifyEvents(); err != nil {
		return err
	}
	if err := v.verifyCompletion(); err != nil {
		return err
	}
	return nil
}

func (v *verifyContext) verifyManifest() error {
	m := v.manifest
	if err := requireSchema(m.SchemaVersion, model.SchemaVersionEvidenceManifestV1Alpha1, "manifest"); err != nil {
		return err
	}
	if m.BundleFormatVersion != BundleFormatV1Alpha1 || m.ID == "" || m.RunID == "" || !validDigest(m.PlanDigest) || !m.BundleTransactionValid {
		return errCode(CodeManifestInvalid, "manifest", "identity", "manifest identity or transaction state is invalid", nil)
	}
	if m.CreatedAt.IsZero() || m.CreatedAt.Location() != nil && m.CreatedAt.UTC() != m.CreatedAt {
		return errCode(CodeManifestInvalid, "manifest", "createdAt", "manifest createdAt must be UTC", nil)
	}
	if m.Entries == nil || m.Artifacts == nil || m.Attempts == nil || m.Limitations == nil {
		return errCode(CodeManifestInvalid, "manifest", "arrays", "required manifest arrays must be present", nil)
	}
	if len(m.Entries) > v.limits.MaxManifestEntries {
		return errCode(CodeManifestInvalid, "manifest", "entries", "manifest entry count exceeds limit", nil)
	}
	last := ""
	for i, e := range m.Entries {
		if e.Path == "manifest.json" {
			return pathErr(CodeManifestInvalid, "manifest", "entry", e.Path, "manifest must not list itself", nil)
		}
		if i > 0 && e.Path <= last {
			return pathErr(CodeManifestInvalid, "manifest", "entry-order", e.Path, "manifest entries must be sorted and unique", nil)
		}
		last = e.Path
		if err := validatePhysicalPathForReader(e.Path, v.limits); err != nil {
			return err
		}
		if !validDigest(e.Digest) || e.SizeBytes < 0 {
			return pathErr(CodeManifestInvalid, "manifest", "entry", e.Path, "manifest entry digest or size is invalid", nil)
		}
		if err := validateRolePath(e); err != nil {
			return err
		}
		if _, ok := v.entries[e.Path]; ok {
			return pathErr(CodeManifestInvalid, "manifest", "entry", e.Path, "duplicate manifest path", nil)
		}
		v.entries[e.Path] = e
		switch e.Role {
		case EntryRoleEvents:
			v.eventEntries[attemptKeyString(AttemptKey{Revision: e.Revision, ScenarioID: e.ScenarioID, Repetition: e.Repetition})] = e
		case EntryRoleStdout, EntryRoleStderr:
			v.logEntries[logKey(AttemptKey{Revision: e.Revision, ScenarioID: e.ScenarioID, Repetition: e.Repetition}, streamForRole(e.Role))] = e
		case EntryRoleArtifactObject:
			v.artifactObjects[e.Digest] = e
		}
	}
	for _, a := range m.Attempts {
		if a.AttemptID == "" || a.Ordinal == 0 || !validScenarioID(a.ScenarioID) || a.Repetition == 0 || (a.Revision != model.RevisionKindBase && a.Revision != model.RevisionKindHead) || a.Directory != attemptDir(AttemptKey{Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition}) {
			return errCode(CodeAttemptInvalid, "manifest", "attempt", "manifest attempt is invalid", nil)
		}
		k := attemptKeyString(AttemptKey{Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition})
		if _, ok := v.attemptsByKey[k]; ok {
			return errCode(CodeAttemptInvalid, "manifest", "attempt", "duplicate manifest attempt", nil)
		}
		v.attemptsByKey[k] = a
	}
	return nil
}

func validateRolePath(e ManifestEntry) error {
	k := AttemptKey{Revision: e.Revision, ScenarioID: e.ScenarioID, Repetition: e.Repetition}
	switch e.Role {
	case EntryRolePlan:
		if e.Path != "plan.json" || e.MediaType != "application/json" {
			return pathErr(CodeManifestInvalid, "manifest", "role", e.Path, "invalid plan entry", nil)
		}
	case EntryRoleExecutionResult:
		if e.Path != "execution.json" || e.MediaType != "application/json" {
			return pathErr(CodeManifestInvalid, "manifest", "role", e.Path, "invalid execution entry", nil)
		}
	case EntryRoleAttemptResult:
		if e.Path != attemptDir(k)+"/result.json" || e.MediaType != "application/json" {
			return pathErr(CodeManifestInvalid, "manifest", "role", e.Path, "invalid attempt result entry", nil)
		}
	case EntryRoleEvents:
		if e.Path != attemptDir(k)+"/events.jsonl" || e.MediaType != "application/x-ndjson" {
			return pathErr(CodeManifestInvalid, "manifest", "role", e.Path, "invalid events entry", nil)
		}
	case EntryRoleStdout, EntryRoleStderr:
		stream := streamForRole(e.Role)
		if e.Path != attemptDir(k)+"/"+string(stream)+".log" || e.MediaType != "application/octet-stream" {
			return pathErr(CodeManifestInvalid, "manifest", "role", e.Path, "invalid log entry", nil)
		}
	case EntryRoleArtifactIndex:
		if e.Path != attemptDir(k)+"/artifacts.json" || e.MediaType != "application/json" {
			return pathErr(CodeManifestInvalid, "manifest", "role", e.Path, "invalid artifact index entry", nil)
		}
	case EntryRoleArtifactObject:
		if e.MediaType != "application/octet-stream" || e.Path != objectPathForDigest(e.Digest) {
			return pathErr(CodeArtifactRefInvalid, "manifest", "object", e.Path, "artifact object path must be digest-derived", nil)
		}
	default:
		return pathErr(CodeManifestInvalid, "manifest", "role", e.Path, "unknown manifest entry role", nil)
	}
	return nil
}
func streamForRole(role EntryRole) LogStream {
	if role == EntryRoleStderr {
		return LogStreamStderr
	}
	return LogStreamStdout
}
func logKey(k AttemptKey, stream LogStream) string {
	return attemptKeyString(k) + "\x00" + string(stream)
}

func (v *verifyContext) reconcileLayout() error {
	expectedFiles := map[string]struct{}{"manifest.json": {}}
	for p := range v.entries {
		expectedFiles[p] = struct{}{}
	}
	if p, code := comparePathSets(inventoryPathSet(v.inv.files), expectedFiles); code != "" {
		return pathErr(code, "layout", "files", p, "physical file set does not match manifest", nil)
	}
	expectedDirs := expectedDirsForFiles(expectedFiles)
	// GR-8A always creates these fixed directories even when no objects are present.
	expectedDirs["attempts"] = struct{}{}
	expectedDirs["attempts/base"] = struct{}{}
	expectedDirs["attempts/head"] = struct{}{}
	expectedDirs["objects"] = struct{}{}
	expectedDirs["objects/sha256"] = struct{}{}
	if p, code := compareDirSets(v.inv.dirs, expectedDirs); code != "" {
		return pathErr(code, "layout", "dirs", p, "physical directory set does not match expected layout", nil)
	}
	return nil
}

func (v *verifyContext) verifyPayloads() error {
	for _, e := range sortedManifestEntries(v.manifest.Entries) {
		if err := v.ctx.Err(); err != nil {
			return readerContextErr(err)
		}
		max := v.roleMax(e.Role)
		data, digest, err := stableReadAll(v.ctx, v.root, e.Path, e.SizeBytes, max)
		if err != nil {
			return err
		}
		if digest != e.Digest {
			return pathErr(CodePayloadDigestMismatch, "payload", "digest", e.Path, "payload digest does not match manifest", nil)
		}
		switch e.Role {
		case EntryRolePlan, EntryRoleExecutionResult, EntryRoleAttemptResult, EntryRoleArtifactIndex:
			v.payloadData[e.Path] = data
		}
	}
	return nil
}
func (v *verifyContext) roleMax(role EntryRole) int64 {
	switch role {
	case EntryRolePlan:
		return v.limits.MaxPlanBytes
	case EntryRoleExecutionResult:
		return v.limits.MaxExecutionBytes
	case EntryRoleAttemptResult:
		return v.limits.MaxAttemptResultBytes
	case EntryRoleArtifactIndex:
		return v.limits.MaxArtifactIndexBytes
	case EntryRoleEvents:
		return v.limits.MaxEventStreamBytesPerAttempt
	case EntryRoleStdout, EntryRoleStderr:
		return v.limits.MaxLogBytesPerStream
	case EntryRoleArtifactObject:
		return v.limits.MaxSingleArtifactBytes
	default:
		return v.limits.MaxBundleBytes
	}
}

func (v *verifyContext) verifyPlan() error {
	e, ok := v.entries["plan.json"]
	if !ok || e.Role != EntryRolePlan {
		return pathErr(CodeMissingEntry, "plan", "entry", "plan.json", "plan entry missing", nil)
	}
	plan, err := decodePlanStrict(v.payloadData["plan.json"], v.limits)
	if err != nil {
		return err
	}
	if err := requireSchema(plan.SchemaVersion, model.SchemaVersionRunPlanV1Alpha1, "plan"); err != nil {
		return err
	}
	if plan.RunID != v.manifest.RunID || plan.RunID == "" || plan.CreatedAt.IsZero() || plan.Runner != (model.RunnerCapabilities{}) {
		return errCode(CodePlanInvalid, "plan", "identity", "plan identity is invalid", nil)
	}
	if v.manifest.PlanDigest != planJSONDigest(v.payloadData["plan.json"]) {
		return errCode(CodePlanDigestMismatch, "plan", "digest", "manifest plan digest does not match plan.json", nil)
	}
	if e.Digest != digestBytes(v.payloadData["plan.json"]) {
		return pathErr(CodePayloadDigestMismatch, "plan", "digest", "plan.json", "plan raw digest mismatch", nil)
	}
	if !validDigest(v.manifest.PlanDigest) {
		return errCode(CodePlanDigestMismatch, "plan", "digest", "plan digest is malformed", nil)
	}
	attempts, err := expandAttemptsFromPlan(plan, v.manifest.PlanDigest)
	if err != nil {
		return errCode(CodePlanInvalid, "plan", "attempts", "plan attempt expansion failed", err)
	}
	if len(attempts) == 0 || len(attempts) > v.limits.MaxAttempts {
		return errCode(CodePlanInvalid, "plan", "attempts", "attempt count exceeds reader limit", nil)
	}
	v.plan = plan
	v.planDigest = v.manifest.PlanDigest
	v.expectedAttempts = attempts
	return nil
}

func (v *verifyContext) verifyExecutionAndResults() error {
	e, ok := v.entries["execution.json"]
	if !ok || e.Role != EntryRoleExecutionResult {
		return pathErr(CodeMissingEntry, "execution", "entry", "execution.json", "execution entry missing", nil)
	}
	_ = e
	execDoc, err := decodeExecutionStrict(v.payloadData["execution.json"], v.limits)
	if err != nil {
		return err
	}
	if err := requireSchema(execDoc.SchemaVersion, model.SchemaVersionExecutionResultV1Alpha1, "execution"); err != nil {
		return err
	}
	if execDoc.RunID != v.plan.RunID || execDoc.PlanDigest != v.planDigest || !execDoc.BundleTransactionValid {
		return errCode(CodeExecutionInvalid, "execution", "identity", "execution identity is invalid", nil)
	}
	if err := runner.ValidateRunnerCapabilities(execDoc.Runner); err != nil {
		return errCode(CodeExecutionInvalid, "execution", "runner", "runner capabilities are invalid", err)
	}
	if execDoc.Attempts == nil || execDoc.Limitations == nil {
		return errCode(CodeExecutionInvalid, "execution", "arrays", "execution arrays must be present", nil)
	}
	if execDoc.ExecutionComplete && len(execDoc.Attempts) != len(v.expectedAttempts) {
		return errCode(CodeExecutionInvalid, "execution", "attempts", "complete execution must include every expected attempt", nil)
	}
	if len(execDoc.Attempts) > len(v.expectedAttempts) {
		return errCode(CodeExecutionInvalid, "execution", "attempts", "execution contains extra attempts", nil)
	}
	for i, sum := range execDoc.Attempts {
		if i >= len(v.expectedAttempts) {
			return errCode(CodeExecutionInvalid, "execution", "attempts", "extra attempt summary", nil)
		}
		a := v.expectedAttempts[i]
		if sum.AttemptID != a.AttemptID || sum.Revision != a.Revision || sum.ScenarioID != a.ScenarioID || sum.Repetition != a.Repetition {
			return errCode(CodeExecutionInvalid, "execution", "attempt-order", "execution attempts are not in plan order", nil)
		}
		if err := validateAttemptOutcome(sum.TargetOutcome, sum.ExitCode, sum.DurationMillis); err != nil {
			return err
		}
	}
	v.execution = execDoc
	for _, a := range v.expectedAttempts {
		key := AttemptKey{Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition}
		ak := attemptKeyString(key)
		am, ok := v.attemptsByKey[ak]
		if !ok {
			return errCode(CodeAttemptInvalid, "manifest", "attempt", "manifest missing expected attempt", nil)
		}
		if am.AttemptID != a.AttemptID || am.Ordinal != a.GlobalOrdinal {
			return errCode(CodeAttemptInvalid, "manifest", "attempt", "manifest attempt does not match deterministic plan", nil)
		}
		entryPath := attemptDir(key) + "/result.json"
		if _, ok := v.entries[entryPath]; !ok {
			if execDoc.ExecutionComplete {
				return pathErr(CodeMissingEntry, "result", "entry", entryPath, "attempt result missing", nil)
			}
			continue
		}
		res, err := decodeAttemptResultStrict(v.payloadData[entryPath], v.limits)
		if err != nil {
			return err
		}
		if err := requireSchema(res.SchemaVersion, model.SchemaVersionAttemptResultV1Alpha1, "attempt-result"); err != nil {
			return err
		}
		if res.AttemptID != a.AttemptID || res.Revision != a.Revision || res.ScenarioID != a.ScenarioID || res.Repetition != a.Repetition {
			return errCode(CodeAttemptInvalid, "result", "coordinate", "attempt result coordinate mismatch", nil)
		}
		if err := validateAttemptOutcome(res.TargetOutcome, res.ExitCode, res.DurationMillis); err != nil {
			return err
		}
		if res.AcceptedEventCount != am.AcceptedEventCount || res.FirstAcceptedSequence != am.FirstEventSequence || res.LastAcceptedSequence != am.LastEventSequence {
			return errCode(CodeEventCountMismatch, "result", "events", "result event range does not match manifest", nil)
		}
	}
	return nil
}

func validateAttemptOutcome(status runner.AttemptStatus, exit *int, duration int64) error {
	if duration < 0 {
		return errCode(CodeAttemptInvalid, "result", "duration", "attempt duration is negative", nil)
	}
	switch status {
	case runner.AttemptStatusSucceeded:
		if exit == nil || *exit != 0 {
			return errCode(CodeAttemptInvalid, "result", "exitCode", "successful target outcome requires exit code 0", nil)
		}
	case runner.AttemptStatusFailed:
		if exit == nil || *exit == 0 {
			return errCode(CodeAttemptInvalid, "result", "exitCode", "failed target outcome requires nonzero exit code", nil)
		}
	case runner.AttemptStatusTimedOut, runner.AttemptStatusResourceLimited:
		if exit != nil && *exit == 0 {
			return errCode(CodeAttemptInvalid, "result", "exitCode", "non-success target outcome cannot report exit code 0", nil)
		}
	default:
		return errCode(CodeAttemptInvalid, "result", "status", "unknown target outcome", nil)
	}
	return nil
}

func (v *verifyContext) verifyArtifacts() error {
	refs := map[model.Digest]int{}
	for _, e := range sortedManifestEntries(v.manifest.Entries) {
		if e.Role != EntryRoleArtifactIndex {
			continue
		}
		doc, err := decodeArtifactIndexStrict(v.payloadData[e.Path], v.limits)
		if err != nil {
			return err
		}
		if err := requireSchema(doc.SchemaVersion, model.SchemaVersionArtifactIndexV1Alpha1, "artifact-index"); err != nil {
			return err
		}
		if e.Path != attemptDir(doc.Attempt)+"/artifacts.json" {
			return pathErr(CodeArtifactIndexInvalid, "artifact-index", "path", e.Path, "artifact index path does not match attempt", nil)
		}
		seen := map[string]struct{}{}
		last := ""
		for _, rec := range doc.Artifacts {
			if err := ValidateLogicalArtifactPath(rec.LogicalPath); err != nil {
				return errCode(CodeArtifactIndexInvalid, "artifact-index", "logical-path", "artifact logical path is invalid", err)
			}
			if rec.LogicalPath <= last {
				return errCode(CodeArtifactIndexInvalid, "artifact-index", "order", "artifact records must be sorted by logical path", nil)
			}
			last = rec.LogicalPath
			if _, ok := seen[rec.LogicalPath]; ok {
				return errCode(CodeArtifactIndexInvalid, "artifact-index", "logical-path", "duplicate logical artifact path", nil)
			}
			seen[rec.LogicalPath] = struct{}{}
			if rec.Attempt != doc.Attempt {
				return errCode(CodeArtifactIndexInvalid, "artifact-index", "attempt", "artifact attempt mismatch", nil)
			}
			k := artifactLookupKey(rec.Attempt, rec.LogicalPath)
			v.artifactRecords[k] = cloneArtifactRecord(rec)
			switch rec.Disposition {
			case ArtifactDispositionStored:
				if !validDigest(rec.Digest) || rec.StoredSizeBytes < 0 || rec.ObjectPath != objectPathForDigest(rec.Digest) {
					return errCode(CodeArtifactRefInvalid, "artifact-index", "object", "stored artifact reference is invalid", nil)
				}
				obj, ok := v.entries[rec.ObjectPath]
				if !ok || obj.Role != EntryRoleArtifactObject || obj.Digest != rec.Digest || obj.SizeBytes != rec.StoredSizeBytes {
					return errCode(CodeArtifactRefInvalid, "artifact-index", "object", "stored artifact object entry mismatch", nil)
				}
				refs[rec.Digest]++
			case ArtifactDispositionOmittedLimit, ArtifactDispositionOmittedSymlink, ArtifactDispositionOmittedSpecial, ArtifactDispositionFailed:
				if rec.Digest != "" || rec.ObjectPath != "" {
					return errCode(CodeArtifactRefInvalid, "artifact-index", "omitted", "omitted artifact must not reference an object", nil)
				}
			default:
				return errCode(CodeArtifactIndexInvalid, "artifact-index", "disposition", "unknown artifact disposition", nil)
			}
		}
		v.artifactIndexes[attemptKeyString(doc.Attempt)] = cloneArtifactIndex(doc)
	}
	for digest := range v.artifactObjects {
		if refs[digest] == 0 {
			return errCode(CodeOrphanArtifactObject, "artifact", "object", "artifact object has no index reference", nil)
		}
	}
	return nil
}

func (v *verifyContext) verifyEvents() error {
	var global uint64
	total := uint64(0)
	for _, a := range v.expectedAttempts {
		key := AttemptKey{Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition}
		ak := attemptKeyString(key)
		am := v.attemptsByKey[ak]
		entry, ok := v.eventEntries[ak]
		if !ok {
			if am.AcceptedEventCount > 0 || v.manifest.EvidenceComplete {
				return errCode(CodeEventCountMismatch, "event", "missing", "attempt event file missing", nil)
			}
			continue
		}
		count, first, last, err := v.parseEventFile(entry, a, global)
		if err != nil {
			return err
		}
		if count != am.AcceptedEventCount || first != am.FirstEventSequence || last != am.LastEventSequence {
			return errCode(CodeEventCountMismatch, "event", "range", "event range does not match manifest", nil)
		}
		global = last
		total += count
		v.eventOrder = append(v.eventOrder, ak)
	}
	if total != v.execution.TotalAcceptedEvents {
		return errCode(CodeEventCountMismatch, "event", "total", "event total does not match execution", nil)
	}
	return nil
}

func (v *verifyContext) parseEventFile(entry ManifestEntry, attempt runner.AttemptRequest, prior uint64) (uint64, uint64, uint64, error) {
	data, digest, err := stableReadAll(v.ctx, v.root, entry.Path, entry.SizeBytes, v.limits.MaxEventStreamBytesPerAttempt)
	if err != nil {
		return 0, 0, 0, err
	}
	if digest != entry.Digest {
		return 0, 0, 0, pathErr(CodePayloadDigestMismatch, "event", "digest", entry.Path, "event file digest mismatch", nil)
	}
	if len(data) == 0 {
		return 0, 0, 0, nil
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		return 0, 0, 0, pathErr(CodeEventFraming, "event", "lf", entry.Path, "events file final line is not LF terminated", nil)
	}
	if bytes.Contains(data, []byte("\r\n")) {
		return 0, 0, 0, pathErr(CodeEventFraming, "event", "crlf", entry.Path, "events file must use LF framing", nil)
	}
	lines := bytes.Split(data[:len(data)-1], []byte("\n"))
	var first, last uint64
	var firstKind, lastKind model.ObservationKind
	seenIDs := map[string]struct{}{}
	for i, line := range lines {
		if len(line) == 0 {
			return 0, 0, 0, pathErr(CodeEventFraming, "event", "blank", entry.Path, "blank JSONL event line", nil)
		}
		if int64(len(line)) > v.limits.MaxEventLineBytes {
			return 0, 0, 0, pathErr(CodeEventFraming, "event", "line-limit", entry.Path, "event line exceeds limit", nil)
		}
		event, err := decodeEventStrict(line, v.limits)
		if err != nil {
			return 0, 0, 0, err
		}
		seq := uint64(event.SequenceNumber)
		wantSeq := prior + uint64(i) + 1
		if seq != wantSeq {
			return 0, 0, 0, errCode(CodeEventOrder, "event", "sequence", "event sequence is not globally contiguous", nil)
		}
		if event.ID != expectedEventID(v.planDigest, v.plan.RunID, seq) {
			return 0, 0, 0, errCode(CodeEventIDMismatch, "event", "id", "event ID does not match deterministic encoding", nil)
		}
		if _, ok := seenIDs[event.ID]; ok {
			return 0, 0, 0, errCode(CodeDuplicateEventID, "event", "id", "duplicate event ID", nil)
		}
		seenIDs[event.ID] = struct{}{}
		if event.SchemaVersion != model.SchemaVersionObservationEventV1Alpha1 || event.RunID != v.plan.RunID || event.Revision != attempt.Revision || event.ScenarioID != attempt.ScenarioID || event.Repetition != attempt.Repetition {
			return 0, 0, 0, errCode(CodeEventOrder, "event", "coordinate", "event coordinate mismatch", nil)
		}
		draft := runner.EventDraft{ObservedAt: event.ObservedAt, Source: event.Source, Kind: event.Kind, Process: event.Process, Filesystem: event.Filesystem, Network: event.Network, Artifact: event.Artifact, Scenario: event.Scenario, ObserverWarning: event.ObserverWarning, ResourceLimit: event.ResourceLimit}
		if err := runner.ValidateEventDraft(draft, runner.DefaultLimits()); err != nil {
			return 0, 0, 0, errCode(CodeInvalidEvent, "event", "payload", "event payload invalid", err)
		}
		if first == 0 {
			first = seq
			firstKind = event.Kind
		}
		last = seq
		lastKind = event.Kind
	}
	if v.manifest.ExecutionComplete && len(lines) > 0 && (firstKind != model.ObservationKindScenarioStarted || lastKind != model.ObservationKindScenarioCompleted) {
		return 0, 0, 0, errCode(CodeCompletionInvariant, "event", "lifecycle", "complete attempt is missing lifecycle boundary events", nil)
	}
	return uint64(len(lines)), first, last, nil
}

func (v *verifyContext) verifyCompletion() error {
	if !v.manifest.BundleTransactionValid || !v.execution.BundleTransactionValid {
		return errCode(CodeCompletionInvariant, "completion", "transaction", "bundle transaction must be valid", nil)
	}
	if v.manifest.ExecutionComplete != v.execution.ExecutionComplete || v.manifest.EvidenceComplete != v.execution.EvidenceComplete {
		return errCode(CodeCompletionInvariant, "completion", "flags", "manifest and execution completion flags differ", nil)
	}
	for _, a := range v.manifest.Attempts {
		if err := v.verifyAttemptCaptureStates(a); err != nil {
			return err
		}
	}
	if err := v.verifyTopLevelArtifacts(); err != nil {
		return err
	}
	if v.manifest.EvidenceComplete {
		if !v.manifest.ExecutionComplete {
			return errCode(CodeCompletionInvariant, "completion", "flags", "evidence complete requires execution complete", nil)
		}
		for _, a := range v.manifest.Attempts {
			if a.Events == CaptureStateTruncated || a.Stdout == CaptureStateTruncated || a.Stderr == CaptureStateTruncated || a.Artifacts == CaptureStateOmittedLimit || a.Result == CaptureStateFailed {
				return errCode(CodeCompletionInvariant, "completion", "captures", "complete evidence contains incomplete capture state", nil)
			}
		}
	}
	if (!v.manifest.ExecutionComplete || !v.manifest.EvidenceComplete) && v.manifest.Failure == nil {
		return errCode(CodeCompletionInvariant, "completion", "failure", "incomplete bundle requires stable failure record", nil)
	}
	return nil
}

func (v *verifyContext) verifyAttemptCaptureStates(a AttemptManifest) error {
	k := AttemptKey{Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition}
	ak := attemptKeyString(k)
	hasEvents := false
	_, hasEvents = v.eventEntries[ak]
	if stateNeedsFile(a.Events) != hasEvents {
		return errCode(CodeCompletionInvariant, "completion", "events", "event capture state does not match file presence", nil)
	}
	_, hasStdout := v.logEntries[logKey(k, LogStreamStdout)]
	if stateNeedsFile(a.Stdout) != hasStdout {
		return errCode(CodeCompletionInvariant, "completion", "stdout", "stdout capture state does not match file presence", nil)
	}
	_, hasStderr := v.logEntries[logKey(k, LogStreamStderr)]
	if stateNeedsFile(a.Stderr) != hasStderr {
		return errCode(CodeCompletionInvariant, "completion", "stderr", "stderr capture state does not match file presence", nil)
	}
	_, hasArtifacts := v.entries[attemptDir(k)+"/artifacts.json"]
	if stateNeedsFile(a.Artifacts) != hasArtifacts {
		return errCode(CodeCompletionInvariant, "completion", "artifacts", "artifact capture state does not match file presence", nil)
	}
	_, hasResult := v.entries[attemptDir(k)+"/result.json"]
	if stateNeedsFile(a.Result) != hasResult {
		return errCode(CodeCompletionInvariant, "completion", "result", "result capture state does not match file presence", nil)
	}
	return nil
}

func stateNeedsFile(state CaptureState) bool {
	switch state {
	case CaptureStateCaptured, CaptureStateCapturedEmpty, CaptureStateTruncated, CaptureStateOmittedLimit, CaptureStateFailed:
		return true
	default:
		return false
	}
}

func (v *verifyContext) verifyTopLevelArtifacts() error {
	if len(v.artifactRecords) != len(v.manifest.Artifacts) {
		return errCode(CodeArtifactIndexInvalid, "artifact", "manifest", "top-level artifact count does not match artifact indexes", nil)
	}
	for _, rec := range v.manifest.Artifacts {
		idx, ok := v.artifactRecords[artifactLookupKey(rec.Attempt, rec.LogicalPath)]
		if !ok {
			return errCode(CodeArtifactIndexInvalid, "artifact", "manifest", "top-level artifact missing from artifact index", nil)
		}
		if !artifactRecordEqual(idx, rec) {
			return errCode(CodeArtifactIndexInvalid, "artifact", "manifest", "top-level artifact record differs from artifact index", nil)
		}
	}
	return nil
}

func artifactRecordEqual(a, b ArtifactRecord) bool {
	if a.LogicalPath != b.LogicalPath || a.Attempt != b.Attempt || a.Disposition != b.Disposition || a.Digest != b.Digest || a.StoredSizeBytes != b.StoredSizeBytes || a.ObservedAtLeast != b.ObservedAtLeast || a.ObjectPath != b.ObjectPath || a.MediaType != b.MediaType || a.Executable != b.Executable || a.SourceMode != b.SourceMode {
		return false
	}
	if (a.DeclaredSize == nil) != (b.DeclaredSize == nil) {
		return false
	}
	if a.DeclaredSize != nil && *a.DeclaredSize != *b.DeclaredSize {
		return false
	}
	la := cloneLimitations(a.Limitations)
	lb := cloneLimitations(b.Limitations)
	if len(la) != len(lb) {
		return false
	}
	for i := range la {
		if la[i] != lb[i] {
			return false
		}
	}
	return true
}

func sortedManifestEntries(in []ManifestEntry) []ManifestEntry {
	out := cloneManifestEntries(in)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
func verifiedAttemptFromManifest(a AttemptManifest) VerifiedAttempt {
	return VerifiedAttempt{AttemptID: a.AttemptID, Ordinal: a.Ordinal, Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition, Directory: a.Directory, Events: a.Events, Stdout: a.Stdout, Stderr: a.Stderr, Artifacts: a.Artifacts, Result: a.Result, FirstEventSequence: a.FirstEventSequence, LastEventSequence: a.LastEventSequence, AcceptedEventCount: a.AcceptedEventCount}
}
func artifactLookupKey(k AttemptKey, logical string) string {
	return attemptKeyString(k) + "\x00" + logical
}

func expandAttemptsFromPlan(doc model.RunPlan, digest model.Digest) ([]runner.AttemptRequest, error) {
	if doc.SchemaVersion != model.SchemaVersionRunPlanV1Alpha1 || doc.RunID == "" || doc.CreatedAt.IsZero() || doc.ExecutionEnvironment == nil || len(doc.Revisions) != 2 || doc.Revisions[0].Kind != model.RevisionKindBase || doc.Revisions[1].Kind != model.RevisionKindHead {
		return nil, errors.New("invalid plan")
	}
	var out []runner.AttemptRequest
	var ordinal uint64
	for _, rev := range doc.Revisions {
		for _, sc := range doc.Scenarios {
			for rep := int64(1); rep <= sc.Repetitions; rep++ {
				ordinal++
				id := fmt.Sprintf("att-%s-%s-r%d", rev.Kind, sc.ID, rep)
				out = append(out, runner.AttemptRequest{PlanDigest: digest, RunID: doc.RunID, PlanCreatedAt: doc.CreatedAt, AttemptID: id, GlobalOrdinal: ordinal, Revision: rev.Kind, CommitID: rev.Commit.CommitID, TreeID: rev.TreeID, ObjectFormat: rev.ObjectFormat, MaterializedTreeDigest: rev.MaterializedTreeDigest, MaterializationManifestDigest: rev.MaterializationManifestDigest, Image: doc.ExecutionEnvironment.Image, Workdir: doc.ExecutionEnvironment.Workdir, ResourceLimits: sc.ResourceLimits, NetworkPolicy: sc.NetworkPolicy, ScenarioID: sc.ID, ScenarioName: sc.Name, Shell: sc.Shell, Run: sc.Run, ScenarioTimeoutMillis: sc.ResourceLimits.TimeoutMillis, Repetition: uint32(rep)})
			}
		}
	}
	return out, nil
}

func (b *Bundle) WalkEvents(ctx context.Context, visit func(model.ObservationEvent) error) error {
	if visit == nil {
		return errCode(CodeCallbackFailed, "events", "callback", "event visitor is nil", nil)
	}
	if err := ctx.Err(); err != nil {
		return readerContextErr(err)
	}
	b.mu.Lock()
	if err := b.ensureOpen("events"); err != nil {
		b.mu.Unlock()
		return err
	}
	root := b.root
	eventOrder := append([]string(nil), b.eventOrder...)
	entries := map[string]ManifestEntry{}
	attempts := map[string]VerifiedAttempt{}
	for k, v := range b.eventsByAttempt {
		entries[k] = v
	}
	for k, v := range b.attemptsByKey {
		attempts[k] = v
	}
	planDigest := b.planDigest
	runID := b.runID
	limits := b.limits
	b.mu.Unlock()
	var prior uint64
	for _, ak := range eventOrder {
		entry := entries[ak]
		attempt := attempts[ak]
		if err := walkEventFile(ctx, root, entry, attempt, planDigest, runID, prior, limits, visit); err != nil {
			return err
		}
		prior = attempt.LastEventSequence
	}
	return nil
}

func walkEventFile(ctx context.Context, root *os.Root, entry ManifestEntry, attempt VerifiedAttempt, planDigest model.Digest, runID string, prior uint64, limits ReaderLimits, visit func(model.ObservationEvent) error) error {
	data, digest, err := stableReadAll(ctx, root, entry.Path, entry.SizeBytes, limits.MaxEventStreamBytesPerAttempt)
	if err != nil {
		return err
	}
	if digest != entry.Digest {
		return pathErr(CodePayloadDigestMismatch, "event", "digest", entry.Path, "event file digest mismatch", nil)
	}
	if len(data) == 0 {
		return nil
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		return pathErr(CodeEventFraming, "event", "lf", entry.Path, "event file not LF terminated", nil)
	}
	lines := bytes.Split(data[:len(data)-1], []byte("\n"))
	for i, line := range lines {
		if err := ctx.Err(); err != nil {
			return readerContextErr(err)
		}
		event, err := decodeEventStrict(line, limits)
		if err != nil {
			return err
		}
		seq := uint64(event.SequenceNumber)
		if seq != prior+uint64(i)+1 || event.ID != expectedEventID(planDigest, runID, seq) || event.Revision != attempt.Revision || event.ScenarioID != attempt.ScenarioID || event.Repetition != attempt.Repetition {
			return errCode(CodeEventOrder, "event", "walk", "event invariant changed during walk", nil)
		}
		if err := visit(cloneObservationEventLocal(event)); err != nil {
			return errCode(CodeCallbackFailed, "events", "callback", "event callback failed", err)
		}
	}
	return nil
}

func (b *Bundle) CopyLog(ctx context.Context, attempt AttemptKey, stream LogStream, dst io.Writer) (CopyResult, error) {
	if err := ctx.Err(); err != nil {
		return CopyResult{}, readerContextErr(err)
	}
	if dst == nil {
		return CopyResult{}, errCode(CodeCallbackFailed, "log", "writer", "destination writer is nil", nil)
	}
	if stream != LogStreamStdout && stream != LogStreamStderr {
		return CopyResult{}, errCode(CodeInvalidLogStream, "log", "stream", "invalid log stream", nil)
	}
	b.mu.Lock()
	if err := b.ensureOpen("log"); err != nil {
		b.mu.Unlock()
		return CopyResult{}, err
	}
	root := b.root
	entry, ok := b.logs[logKey(attempt, stream)]
	attempts := b.attemptsByKey
	limits := b.limits
	b.mu.Unlock()
	if _, exists := attempts[attemptKeyString(attempt)]; !exists {
		return CopyResult{}, errCode(CodeInvalidAttempt, "log", "attempt", "unknown attempt", nil)
	}
	if !ok {
		return CopyResult{CaptureState: CaptureStateNotProvided, NotStored: true}, nil
	}
	res, err := verifyStreamPayload(ctx, root, entry.Path, entry.SizeBytes, entry.Digest, limits.MaxLogBytesPerStream, dst)
	if err != nil {
		return CopyResult{}, err
	}
	res.CaptureState = entry.CaptureState
	return res, nil
}

func (b *Bundle) CopyArtifact(ctx context.Context, attempt AttemptKey, logicalPath string, dst io.Writer) (CopyResult, error) {
	if err := ctx.Err(); err != nil {
		return CopyResult{}, readerContextErr(err)
	}
	if dst == nil {
		return CopyResult{}, errCode(CodeCallbackFailed, "artifact", "writer", "destination writer is nil", nil)
	}
	if err := ValidateLogicalArtifactPath(logicalPath); err != nil {
		return CopyResult{}, err
	}
	b.mu.Lock()
	if err := b.ensureOpen("artifact"); err != nil {
		b.mu.Unlock()
		return CopyResult{}, err
	}
	root := b.root
	rec, ok := b.artifactRecords[artifactLookupKey(attempt, logicalPath)]
	obj := b.artifactObjects[rec.Digest]
	attempts := b.attemptsByKey
	limits := b.limits
	b.mu.Unlock()
	if _, exists := attempts[attemptKeyString(attempt)]; !exists {
		return CopyResult{}, errCode(CodeInvalidAttempt, "artifact", "attempt", "unknown attempt", nil)
	}
	if !ok {
		return CopyResult{}, errCode(CodeInvalidArtifact, "artifact", "logical-path", "unknown artifact logical path", nil)
	}
	if rec.Disposition != ArtifactDispositionStored {
		return CopyResult{NotStored: true, Disposition: rec.Disposition, CaptureState: CaptureStateOmittedLimit}, nil
	}
	res, err := verifyStreamPayload(ctx, root, obj.Path, obj.SizeBytes, obj.Digest, limits.MaxSingleArtifactBytes, dst)
	if err != nil {
		return CopyResult{}, err
	}
	res.Disposition = rec.Disposition
	return res, nil
}

func cloneObservationEventLocal(in model.ObservationEvent) model.ObservationEvent {
	data, _ := json.Marshal(in)
	var out model.ObservationEvent
	_ = json.Unmarshal(data, &out)
	return out
}
