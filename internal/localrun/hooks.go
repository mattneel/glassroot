package localrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/mattneel/glassroot/internal/artifactcollect"
	"github.com/mattneel/glassroot/internal/evidence"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

type captureHooks struct {
	session     *evidence.Session
	planDigest  model.Digest
	attempts    map[string]*attemptWorkspace
	captures    map[string]*attemptCaptures
	incomplete  bool
	limitations []model.Limitation
}

type attemptCaptures struct {
	stdout *evidence.LogCapture
	stderr *evidence.LogCapture
}

type evidenceLogSink struct {
	stdout *evidence.LogCapture
	stderr *evidence.LogCapture
}

func newCaptureHooks(session *evidence.Session, planDigest model.Digest, attempts map[string]*attemptWorkspace) *captureHooks {
	return &captureHooks{session: session, planDigest: planDigest, attempts: attempts, captures: map[string]*attemptCaptures{}, limitations: []model.Limitation{}}
}

func (h *captureHooks) BeforeAttempt(ctx context.Context, req runner.AttemptRequest) (runner.AttemptOutputSink, error) {
	key := evidenceKey(req)
	stdout, err := h.session.OpenLog(ctx, key, evidence.LogStreamStdout)
	if err != nil {
		return nil, wrap(CodeLogCaptureFailed, "logs", "open stdout capture", err)
	}
	stderr, err := h.session.OpenLog(ctx, key, evidence.LogStreamStderr)
	if err != nil {
		_ = stdout.Close()
		return nil, wrap(CodeLogCaptureFailed, "logs", "open stderr capture", err)
	}
	h.captures[req.AttemptID] = &attemptCaptures{stdout: stdout, stderr: stderr}
	return &evidenceLogSink{stdout: stdout, stderr: stderr}, nil
}

func (s *evidenceLogSink) WriteLog(ctx context.Context, stream runner.LogStream, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch stream {
	case runner.LogStreamStdout:
		_, err := s.stdout.Write(data)
		return err
	case runner.LogStreamStderr:
		_, err := s.stderr.Write(data)
		return err
	default:
		return errCode(CodeLogCaptureFailed, "logs", "unknown log stream", nil)
	}
}

func (h *captureHooks) AfterAttempt(ctx context.Context, req runner.AttemptRequest, outcome runner.AttemptOutcome, sink runner.DraftSink) (runner.AttemptOutcome, error) {
	caps := h.captures[req.AttemptID]
	if caps == nil {
		return runner.AttemptOutcome{}, errCode(CodeLogCaptureFailed, "logs", "missing attempt log captures", nil)
	}
	if limitationPresent(outcome.Limitations, "stdout-truncated") || limitationPresent(outcome.Limitations, "output-truncated") {
		if err := caps.stdout.MarkTruncated(0); err != nil {
			return runner.AttemptOutcome{}, wrap(CodeLogCaptureFailed, "logs", "mark stdout truncated", err)
		}
		h.markIncomplete(model.Limitation{ID: "stdout-truncated", Summary: "Stdout exceeded a configured capture limit."})
	}
	if limitationPresent(outcome.Limitations, "stderr-truncated") || limitationPresent(outcome.Limitations, "output-truncated") {
		if err := caps.stderr.MarkTruncated(0); err != nil {
			return runner.AttemptOutcome{}, wrap(CodeLogCaptureFailed, "logs", "mark stderr truncated", err)
		}
		h.markIncomplete(model.Limitation{ID: "stderr-truncated", Summary: "Stderr exceeded a configured capture limit."})
	}
	if err := caps.stdout.Close(); err != nil {
		return runner.AttemptOutcome{}, wrap(CodeLogCaptureFailed, "logs", "close stdout capture", err)
	}
	if err := caps.stderr.Close(); err != nil {
		return runner.AttemptOutcome{}, wrap(CodeLogCaptureFailed, "logs", "close stderr capture", err)
	}
	aw := h.attempts[req.AttemptID]
	if aw == nil || aw.collector == nil {
		return runner.AttemptOutcome{}, errCode(CodeArtifactCollectionFailed, "artifacts", "missing bound collector", nil)
	}
	plan, err := collectionPlanForAttempt(h.planDigest, req)
	if err != nil {
		return runner.AttemptOutcome{}, err
	}
	if len(plan.Rules) == 0 {
		return h.withLimitations(outcome, nil), nil
	}
	res, err := aw.collector.Collect(ctx, plan, artifactEvidenceSink{session: h.session})
	if err != nil {
		return runner.AttemptOutcome{}, wrap(CodeArtifactCollectionFailed, "artifacts", "collect post-run artifacts", err)
	}
	if res.CollectionComplete && len(res.Artifacts) == 0 {
		if err := h.session.MarkArtifactCollectionComplete(ctx, evidenceKey(req)); err != nil {
			return runner.AttemptOutcome{}, wrap(CodeArtifactEvidenceFailed, "artifacts", "record empty artifact collection", err)
		}
	}
	if err := h.recordArtifactResults(ctx, req, res, sink); err != nil {
		return runner.AttemptOutcome{}, err
	}
	if !res.CollectionComplete {
		h.markIncomplete(model.Limitation{ID: "artifact-collection-incomplete", Summary: "One or more configured artifacts were omitted or blocked during post-run collection."})
	}
	for _, lim := range res.Limitations {
		h.markLimitation(lim)
	}
	return h.withLimitations(outcome, res.Limitations), nil
}

func (h *captureHooks) AbortAttempt(ctx context.Context, req runner.AttemptRequest, _ error) error {
	caps := h.captures[req.AttemptID]
	if caps == nil {
		return nil
	}
	var out error
	if caps.stdout != nil {
		out = errors.Join(out, caps.stdout.Close())
	}
	if caps.stderr != nil {
		out = errors.Join(out, caps.stderr.Close())
	}
	return out
}

func (h *captureHooks) recordArtifactResults(ctx context.Context, req runner.AttemptRequest, res *artifactcollect.Result, sink runner.DraftSink) error {
	for _, art := range res.Artifacts {
		switch art.Disposition {
		case artifactcollect.ArtifactDispositionStored:
			if err := sink.Emit(ctx, artifactEvent(req, art)); err != nil {
				return wrap(CodeArtifactEvidenceFailed, "artifacts", "emit artifact event", err)
			}
		case artifactcollect.ArtifactDispositionOmittedLimit, artifactcollect.ArtifactDispositionOmittedSymlink, artifactcollect.ArtifactDispositionOmittedSpecial:
			if err := h.recordOmission(ctx, req, art); err != nil {
				return err
			}
			if err := sink.Emit(ctx, omissionWarning(art)); err != nil {
				return wrap(CodeArtifactEvidenceFailed, "artifacts", "emit artifact omission warning", err)
			}
		default:
			return errCode(CodeArtifactCollectionFailed, "artifacts", "unknown artifact disposition", nil)
		}
	}
	if !res.CollectionComplete && len(res.Artifacts) == 0 {
		if err := h.recordBlockedPatterns(ctx, req, res, sink); err != nil {
			return err
		}
	}
	return nil
}

func (h *captureHooks) recordBlockedPatterns(ctx context.Context, req runner.AttemptRequest, res *artifactcollect.Result, sink runner.DraftSink) error {
	for _, pr := range res.Patterns {
		if pr.Disposition != artifactcollect.PatternDispositionBlockedSymlink && pr.Disposition != artifactcollect.PatternDispositionBlockedSpecial && pr.Disposition != artifactcollect.PatternDispositionIncomplete {
			continue
		}
		logical := req.Workdir
		if len(pr.MatchedPaths) > 0 {
			logical = pr.MatchedPaths[0]
		}
		disp := evidence.ArtifactDispositionOmittedSpecial
		if pr.Disposition == artifactcollect.PatternDispositionBlockedSymlink {
			disp = evidence.ArtifactDispositionOmittedSymlink
		}
		lim := model.Limitation{ID: "artifact-collection-blocked", Summary: "Artifact collection was blocked by an unsupported workspace entry."}
		if len(pr.Limitations) > 0 {
			lim = pr.Limitations[0]
		}
		_, err := h.session.RecordArtifactOmission(ctx, evidence.ArtifactOmissionInput{Attempt: evidenceKey(req), LogicalPath: logical, Disposition: disp, MediaType: "application/octet-stream", Limitations: []model.Limitation{lim}})
		if err != nil {
			return wrap(CodeArtifactEvidenceFailed, "artifacts", "record blocked artifact collection", err)
		}
		if err := sink.Emit(ctx, runner.EventDraft{Source: model.ObservationSourceHostObserved, Kind: model.ObservationKindObserverWarning, ObserverWarning: &model.ObserverWarningObservation{Code: "artifact-collection-blocked", Message: "Post-run artifact collection was blocked by an unsupported workspace entry.", Unsupported: true, Limitations: []model.Limitation{lim}}}); err != nil {
			return wrap(CodeArtifactEvidenceFailed, "artifacts", "emit blocked artifact warning", err)
		}
		return nil
	}
	return nil
}

func (h *captureHooks) recordOmission(ctx context.Context, req runner.AttemptRequest, art artifactcollect.ArtifactResult) error {
	disp := evidence.ArtifactDispositionOmittedLimit
	if art.Disposition == artifactcollect.ArtifactDispositionOmittedSymlink {
		disp = evidence.ArtifactDispositionOmittedSymlink
	}
	if art.Disposition == artifactcollect.ArtifactDispositionOmittedSpecial {
		disp = evidence.ArtifactDispositionOmittedSpecial
	}
	var declared *int64
	if art.KnownSizeBytes > 0 {
		v := art.KnownSizeBytes
		declared = &v
	}
	_, err := h.session.RecordArtifactOmission(ctx, evidence.ArtifactOmissionInput{Attempt: evidenceKey(req), LogicalPath: art.LogicalPath, Disposition: disp, DeclaredSize: declared, ObservedAtLeast: art.KnownSizeBytes, MediaType: "application/octet-stream", Executable: art.Executable, SourceMode: art.SourceMode.Mode, Limitations: art.Limitations})
	if err != nil {
		return wrap(CodeArtifactEvidenceFailed, "artifacts", "record artifact omission", err)
	}
	h.markIncomplete(model.Limitation{ID: string(art.Disposition), Summary: "A configured artifact was not stored."})
	return nil
}

func artifactEvent(req runner.AttemptRequest, art artifactcollect.ArtifactResult) runner.EventDraft {
	return runner.EventDraft{Source: model.ObservationSourceHostObserved, Kind: model.ObservationKindArtifactActivity, Artifact: &model.ArtifactObservation{Operation: "post-run-collect", ArtifactID: artifactID(req, art.LogicalPath), Path: art.LogicalPath, Digest: art.ContentDigest, SizeBytes: art.SizeBytes, Executable: art.Executable, SourceEventIDs: []string{}}}
}

func omissionWarning(art artifactcollect.ArtifactResult) runner.EventDraft {
	code := "artifact-collection-omitted"
	msg := "A configured post-run artifact was omitted by the collector."
	lim := model.Limitation{ID: string(art.Disposition), Summary: msg}
	if len(art.Limitations) > 0 {
		lim = art.Limitations[0]
	}
	return runner.EventDraft{Source: model.ObservationSourceHostObserved, Kind: model.ObservationKindObserverWarning, ObserverWarning: &model.ObserverWarningObservation{Code: code, Message: msg, Unsupported: true, Limitations: []model.Limitation{lim}}}
}

func artifactID(req runner.AttemptRequest, logical string) string {
	sum := sha256.Sum256([]byte(req.AttemptID + "\x00" + logical))
	return "post-run-artifact-" + hex.EncodeToString(sum[:16])
}

func collectionPlanForAttempt(planDigest model.Digest, req runner.AttemptRequest) (artifactcollect.CollectionPlan, error) {
	rules := make([]artifactcollect.ArtifactRule, 0, len(req.Collection.Artifacts))
	for i, spec := range req.Collection.Artifacts {
		rules = append(rules, artifactcollect.ArtifactRule{ID: fmt.Sprintf("artifact-%03d", i+1), Pattern: spec.LogicalPath, MaxBytes: spec.MaxSizeBytes})
	}
	return artifactcollect.CollectionPlan{PlanDigest: planDigest, Attempt: artifactcollect.AttemptIdentity{AttemptID: req.AttemptID, Revision: req.Revision, ScenarioID: req.ScenarioID, Repetition: req.Repetition}, Workdir: req.Workdir, Rules: rules}, nil
}

type artifactEvidenceSink struct{ session *evidence.Session }

func (s artifactEvidenceSink) StoreArtifact(ctx context.Context, input artifactcollect.ArtifactInput) (artifactcollect.StoredArtifact, error) {
	declared := input.DeclaredSize
	rec, err := s.session.AddArtifact(ctx, evidence.ArtifactInput{Attempt: evidence.AttemptKey{Revision: input.Attempt.Revision, ScenarioID: input.Attempt.ScenarioID, Repetition: input.Attempt.Repetition}, LogicalPath: input.LogicalPath, DeclaredSize: &declared, MaxBytes: input.MaxBytes, Reader: input.Reader, MediaType: "application/octet-stream", Executable: input.Executable, SourceMode: input.SourceMode.Mode})
	if err != nil {
		return artifactcollect.StoredArtifact{}, err
	}
	return artifactcollect.StoredArtifact{ContentDigest: rec.Digest, SizeBytes: rec.StoredSizeBytes}, nil
}

func evidenceKey(req runner.AttemptRequest) evidence.AttemptKey {
	return evidence.AttemptKey{Revision: req.Revision, ScenarioID: req.ScenarioID, Repetition: req.Repetition}
}

func limitationPresent(limitations []model.Limitation, id string) bool {
	for _, lim := range limitations {
		if lim.ID == id {
			return true
		}
	}
	return false
}

func (h *captureHooks) markIncomplete(lim model.Limitation) {
	h.incomplete = true
	h.markLimitation(lim)
}

func (h *captureHooks) markLimitation(lim model.Limitation) {
	if lim.ID == "" {
		return
	}
	h.limitations = append(h.limitations, lim)
}

func (h *captureHooks) withLimitations(out runner.AttemptOutcome, extra []model.Limitation) runner.AttemptOutcome {
	out.Limitations = append(append([]model.Limitation(nil), out.Limitations...), extra...)
	out.Limitations = dedupeModelLimitations(out.Limitations)
	return out
}

func dedupeModelLimitations(in []model.Limitation) []model.Limitation {
	if len(in) == 0 {
		return []model.Limitation{}
	}
	seen := map[string]model.Limitation{}
	keys := []string{}
	for _, lim := range in {
		key := lim.ID + "\x00" + lim.Summary + "\x00" + lim.Details
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = lim
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]model.Limitation, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}
