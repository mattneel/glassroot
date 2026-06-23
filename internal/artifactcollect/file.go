package artifactcollect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"sort"

	"github.com/mattneel/glassroot/internal/model"
)

type matchState struct {
	regular     map[string]map[string]validatedRule
	omitted     map[string]*ArtifactResult
	patterns    []PatternResult
	complete    bool
	limitations []model.Limitation
}

func (w *BoundWorkspace) collectFromInventory(ctx context.Context, vp validatedPlan, inv inventory, sink ArtifactSink) (*Result, error) {
	state, err := planMatches(vp, inv, w.limits)
	if err != nil {
		return nil, err
	}
	artifacts := make([]ArtifactResult, 0)
	totalStored := int64(0)
	regularPaths := make([]string, 0, len(state.regular))
	for rel := range state.regular {
		regularPaths = append(regularPaths, rel)
	}
	sort.Strings(regularPaths)
	for _, rel := range regularPaths {
		if err := checkContext(ctx, "file"); err != nil {
			return nil, err
		}
		entry := inv.byRel[rel]
		rules := rulesForPath(state.regular[rel])
		ruleIDs := ruleIDs(rules)
		limit := narrowestLimit(rules)
		modeFacts := modeFacts(entry.mode)
		logical := logicalFromRel(vp.Workdir, rel)
		if entry.identity.Nlink != 1 {
			return nil, errCode(CodeHardlinkEntry, "file", logical, "hard-linked regular files are not collected", nil)
		}
		if entry.size > limit || entry.size > w.limits.MaxSingleArtifactBytes || entry.size > w.limits.MaxTotalArtifactBytes-totalStored {
			applicable := limit
			if w.limits.MaxSingleArtifactBytes < applicable {
				applicable = w.limits.MaxSingleArtifactBytes
			}
			remaining := w.limits.MaxTotalArtifactBytes - totalStored
			if remaining < applicable {
				applicable = remaining
			}
			lim := limitation("artifact-omitted-limit", "artifact bytes exceeded a configured collection limit")
			artifacts = append(artifacts, ArtifactResult{LogicalPath: logical, Disposition: ArtifactDispositionOmittedLimit, KnownSizeBytes: entry.size, ApplicableLimit: applicable, Executable: modeFacts.Executable, SourceMode: modeFacts, MatchingRuleIDs: ruleIDs, Limitations: []model.Limitation{lim}})
			state.complete = false
			state.limitations = append(state.limitations, lim)
			continue
		}
		stored, err := w.storeRegularFile(ctx, vp, rel, entry, limit, sink, ruleIDs)
		if err != nil {
			return nil, err
		}
		totalStored += stored.SizeBytes
		artifacts = append(artifacts, stored)
	}
	for _, omitted := range state.omitted {
		artifacts = append(artifacts, cloneArtifactResult(*omitted))
	}
	sortArtifacts(artifacts)
	if len(artifacts) > w.limits.MaxMatchedArtifacts {
		return nil, errCode(CodeMatchedArtifactLimit, "collect", "", "matched artifact count exceeds limit", nil)
	}
	return &Result{PlanDigest: vp.PlanDigest, Attempt: vp.Attempt, CollectionComplete: state.complete, Inventory: inv.summary, Patterns: clonePatternResults(state.patterns), Artifacts: cloneArtifactResults(artifacts), Limitations: dedupeLimitations(state.limitations)}, nil
}

func planMatches(vp validatedPlan, inv inventory, limits Limits) (matchState, error) {
	state := matchState{regular: map[string]map[string]validatedRule{}, omitted: map[string]*ArtifactResult{}, patterns: make([]PatternResult, len(vp.Rules)), complete: true, limitations: []model.Limitation{}}
	for i, rule := range vp.Rules {
		state.patterns[i] = PatternResult{RuleID: rule.ID, Pattern: rule.Pattern, Disposition: PatternDispositionNoMatch, MatchedPaths: []string{}, Limitations: []model.Limitation{}}
	}
	for _, ent := range inv.entries {
		for i, rule := range vp.Rules {
			direct, err := matchRelativePattern(rule.RelPattern, ent.rel, limits)
			if err != nil {
				return matchState{}, err
			}
			blocked, err := patternCanMatchDescendant(rule.RelPattern, ent.rel, limits)
			if err != nil {
				return matchState{}, err
			}
			logical := logicalFromRel(vp.Workdir, ent.rel)
			if blocked && ent.kind == entrySymlink {
				lim := limitation("artifact-blocked-symlink", "artifact pattern traversal was blocked by a symlink")
				markPattern(&state.patterns[i], PatternDispositionBlockedSymlink, logical, lim)
				state.complete = false
				state.limitations = append(state.limitations, lim)
			}
			if blocked && isSpecial(ent.kind) {
				lim := limitation("artifact-blocked-special", "artifact pattern traversal was blocked by a special filesystem entry")
				markPattern(&state.patterns[i], PatternDispositionBlockedSpecial, logical, lim)
				state.complete = false
				state.limitations = append(state.limitations, lim)
			}
			if !direct {
				continue
			}
			switch ent.kind {
			case entryRegular:
				if state.patterns[i].Disposition == PatternDispositionNoMatch {
					state.patterns[i].Disposition = PatternDispositionMatched
				}
				state.patterns[i].MatchedPaths = appendUnique(state.patterns[i].MatchedPaths, logical)
				if state.regular[ent.rel] == nil {
					state.regular[ent.rel] = map[string]validatedRule{}
				}
				state.regular[ent.rel][rule.ID] = rule
			case entryDirectory:
				// Directory objects are never persisted as artifacts.
			case entrySymlink:
				lim := limitation("artifact-omitted-symlink", "matched artifact path is a symlink and was not followed")
				markPattern(&state.patterns[i], PatternDispositionMatched, logical, lim)
				state.complete = false
				state.limitations = append(state.limitations, lim)
				addOmitted(state.omitted, ent, vp.Workdir, ArtifactDispositionOmittedSymlink, rule.ID, lim)
			default:
				lim := limitation("artifact-omitted-special", "matched artifact path is a special filesystem entry and was not opened")
				markPattern(&state.patterns[i], PatternDispositionMatched, logical, lim)
				state.complete = false
				state.limitations = append(state.limitations, lim)
				addOmitted(state.omitted, ent, vp.Workdir, ArtifactDispositionOmittedSpecial, rule.ID, lim)
			}
		}
	}
	for i := range state.patterns {
		sort.Strings(state.patterns[i].MatchedPaths)
		state.patterns[i].Limitations = dedupeLimitations(state.patterns[i].Limitations)
	}
	return state, nil
}

func (w *BoundWorkspace) storeRegularFile(ctx context.Context, vp validatedPlan, rel string, ent entry, maxBytes int64, sink ArtifactSink, ruleIDs []string) (ArtifactResult, error) {
	logical := logicalFromRel(vp.Workdir, rel)
	if w.hooks.BeforeOpenFile != nil {
		if err := w.hooks.BeforeOpenFile(logical); err != nil {
			return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "pre-open hook reported mutation", err)
		}
	}
	info, err := w.root.Lstat(rel)
	if err != nil {
		return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "lstat before open", err)
	}
	id, err := identityFromInfo(info)
	if err != nil {
		return ArtifactResult{}, err
	}
	if !sameStableIdentity(ent.identity, id) || classifyEntry(info.Mode()) != entryRegular || id.Nlink != 1 {
		return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "file changed before open", nil)
	}
	f, err := w.root.Open(rel)
	if err != nil {
		return ArtifactResult{}, errCode(CodeFileOpenFailed, "file", logical, "open regular artifact", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()
	openedInfo, err := f.Stat()
	if err != nil {
		return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "stat opened artifact", err)
	}
	openedID, err := identityFromInfo(openedInfo)
	if err != nil {
		return ArtifactResult{}, err
	}
	if !sameFileIdentity(ent.identity, openedID) || !sameStableIdentity(ent.identity, openedID) {
		return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "opened artifact identity does not match preflight", nil)
	}
	again, err := w.root.Lstat(rel)
	if err != nil {
		return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "lstat after open", err)
	}
	againID, err := identityFromInfo(again)
	if err != nil {
		return ArtifactResult{}, err
	}
	if !sameStableIdentity(ent.identity, againID) {
		return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "artifact path changed after open", nil)
	}
	facts := modeFacts(ent.mode)
	reader := &hashingReader{r: f, h: sha256.New()}
	stored, err := sink.StoreArtifact(ctx, ArtifactInput{Attempt: vp.Attempt, LogicalPath: logical, DeclaredSize: ent.size, MaxBytes: maxBytes, Executable: facts.Executable, SourceMode: facts, Reader: reader})
	if err != nil {
		return ArtifactResult{}, errCode(CodeSinkFailed, "sink", logical, "artifact sink failed", err)
	}
	if reader.n != ent.size {
		return ArtifactResult{}, errCode(CodeSinkShortRead, "sink", logical, "artifact sink did not consume the declared size", nil)
	}
	digest := reader.digest()
	if err := validateStoredArtifact(stored, digest, ent.size); err != nil {
		return ArtifactResult{}, err
	}
	if w.hooks.AfterStoreFile != nil {
		if err := w.hooks.AfterStoreFile(logical); err != nil {
			return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "post-store hook reported mutation", err)
		}
	}
	afterInfo, err := f.Stat()
	if err != nil {
		return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "stat artifact after streaming", err)
	}
	afterID, err := identityFromInfo(afterInfo)
	if err != nil {
		return ArtifactResult{}, err
	}
	if !sameStableIdentity(ent.identity, afterID) {
		return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "artifact changed while streaming", nil)
	}
	pathInfo, err := w.root.Lstat(rel)
	if err != nil {
		return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "lstat artifact after streaming", err)
	}
	pathID, err := identityFromInfo(pathInfo)
	if err != nil {
		return ArtifactResult{}, err
	}
	if !sameStableIdentity(ent.identity, pathID) {
		return ArtifactResult{}, errCode(CodeFileChanged, "file", logical, "artifact path changed after streaming", nil)
	}
	if err := f.Close(); err != nil {
		return ArtifactResult{}, errCode(CodeCloseFailed, "file", logical, "close artifact file", err)
	}
	closed = true
	return ArtifactResult{LogicalPath: logical, Disposition: ArtifactDispositionStored, ContentDigest: digest, SizeBytes: ent.size, KnownSizeBytes: ent.size, Executable: facts.Executable, SourceMode: facts, MatchingRuleIDs: append([]string(nil), ruleIDs...), Limitations: []model.Limitation{}}, nil
}

type hashingReader struct {
	r io.Reader
	h hash.Hash
	n int64
}

func (r *hashingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		_, _ = r.h.Write(p[:n])
		r.n += int64(n)
	}
	return n, err
}

func (r *hashingReader) digest() model.Digest {
	return model.Digest("sha256:" + hex.EncodeToString(r.h.Sum(nil)))
}

func validateStoredArtifact(stored StoredArtifact, expected model.Digest, size int64) error {
	if stored.SizeBytes != size {
		return errCode(CodeSinkResultMismatch, "sink", "", "artifact sink returned mismatched size", nil)
	}
	if stored.ContentDigest != expected {
		return errCode(CodeSinkResultMismatch, "sink", "", "artifact sink returned mismatched digest", nil)
	}
	return nil
}

func rulesForPath(m map[string]validatedRule) []validatedRule {
	rules := make([]validatedRule, 0, len(m))
	for _, r := range m {
		rules = append(rules, r)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return rules
}

func ruleIDs(rules []validatedRule) []string {
	ids := make([]string, len(rules))
	for i, r := range rules {
		ids[i] = r.ID
	}
	sort.Strings(ids)
	return ids
}

func narrowestLimit(rules []validatedRule) int64 {
	var limit int64
	for i, r := range rules {
		if i == 0 || r.MaxBytes < limit {
			limit = r.MaxBytes
		}
	}
	return limit
}

func modeFacts(mode os.FileMode) SourceModeFacts {
	return SourceModeFacts{Mode: uint32(mode.Perm()), Executable: mode.Perm()&0o111 != 0, SetUID: mode&os.ModeSetuid != 0, SetGID: mode&os.ModeSetgid != 0, Sticky: mode&os.ModeSticky != 0}
}

func isSpecial(kind entryKind) bool {
	return kind != entryDirectory && kind != entryRegular && kind != entrySymlink
}

func markPattern(p *PatternResult, d PatternDisposition, logical string, lim model.Limitation) {
	if p.Disposition == PatternDispositionNoMatch || d == PatternDispositionBlockedSymlink || d == PatternDispositionBlockedSpecial {
		p.Disposition = d
	}
	p.MatchedPaths = appendUnique(p.MatchedPaths, logical)
	p.Limitations = append(p.Limitations, lim)
}

func addOmitted(m map[string]*ArtifactResult, ent entry, workdir string, disposition ArtifactDisposition, ruleID string, lim model.Limitation) {
	logical := logicalFromRel(workdir, ent.rel)
	facts := modeFacts(ent.mode)
	if existing := m[ent.rel]; existing != nil {
		existing.MatchingRuleIDs = appendUnique(existing.MatchingRuleIDs, ruleID)
		existing.Limitations = dedupeLimitations(append(existing.Limitations, lim))
		return
	}
	m[ent.rel] = &ArtifactResult{LogicalPath: logical, Disposition: disposition, Executable: facts.Executable, SourceMode: facts, MatchingRuleIDs: []string{ruleID}, Limitations: []model.Limitation{lim}}
}

func appendUnique(in []string, s string) []string {
	for _, v := range in {
		if v == s {
			return in
		}
	}
	out := append(in, s)
	sort.Strings(out)
	return out
}

func limitation(id, summary string) model.Limitation {
	return model.Limitation{ID: id, Summary: summary}
}

func dedupeLimitations(in []model.Limitation) []model.Limitation {
	m := map[string]model.Limitation{}
	for _, l := range in {
		if l.ID == "" {
			continue
		}
		m[l.ID+"\x00"+l.Summary+"\x00"+l.Details] = l
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]model.Limitation, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

func sortArtifacts(artifacts []ArtifactResult) {
	sort.SliceStable(artifacts, func(i, j int) bool {
		if artifacts[i].LogicalPath != artifacts[j].LogicalPath {
			return artifacts[i].LogicalPath < artifacts[j].LogicalPath
		}
		if artifacts[i].Disposition != artifacts[j].Disposition {
			return artifacts[i].Disposition < artifacts[j].Disposition
		}
		return joinIDs(artifacts[i].MatchingRuleIDs) < joinIDs(artifacts[j].MatchingRuleIDs)
	})
}

func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += id
	}
	return out
}

func clonePatternResults(in []PatternResult) []PatternResult {
	if in == nil {
		return []PatternResult{}
	}
	out := make([]PatternResult, len(in))
	for i, v := range in {
		out[i] = v
		out[i].MatchedPaths = append([]string(nil), v.MatchedPaths...)
		if out[i].MatchedPaths == nil {
			out[i].MatchedPaths = []string{}
		}
		out[i].Limitations = append([]model.Limitation(nil), v.Limitations...)
		if out[i].Limitations == nil {
			out[i].Limitations = []model.Limitation{}
		}
	}
	return out
}

func cloneArtifactResults(in []ArtifactResult) []ArtifactResult {
	if in == nil {
		return []ArtifactResult{}
	}
	out := make([]ArtifactResult, len(in))
	for i, v := range in {
		out[i] = cloneArtifactResult(v)
	}
	return out
}

func cloneArtifactResult(v ArtifactResult) ArtifactResult {
	v.MatchingRuleIDs = append([]string(nil), v.MatchingRuleIDs...)
	if v.MatchingRuleIDs == nil {
		v.MatchingRuleIDs = []string{}
	}
	v.Limitations = append([]model.Limitation(nil), v.Limitations...)
	if v.Limitations == nil {
		v.Limitations = []model.Limitation{}
	}
	return v
}
