package waiver

import (
	"context"
	"errors"
	"io/fs"
	"sort"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
)

func LoadTrusted(ctx context.Context, source config.RevisionFileSource, req TrustedLoadRequest, limits Limits) (TrustedLoadResult, error) {
	l, err := validateLimits(limits)
	if err != nil {
		return TrustedLoadResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return TrustedLoadResult{}, contextErr(err)
	}
	if source == nil {
		return TrustedLoadResult{}, errCode(CodeBaseReadFailed, "base", "nil revision file source")
	}
	res := TrustedLoadResult{BaseRevision: req.Base, HeadRevision: req.Head}
	baseFile, berr := source.ReadFile(ctx, req.Base, WaiverPath, l.MaxWaiverFileBytes)
	if err := ctx.Err(); err != nil {
		return TrustedLoadResult{}, contextErr(err)
	}
	baseRaw := []byte(nil)
	var baseSet WaiverSet
	basePresent := true
	if berr != nil {
		if errors.Is(berr, fs.ErrNotExist) {
			res.Base.State = BaseStateAbsent
			basePresent = false
		} else if errors.Is(berr, config.ErrRevisionFileTooLarge) {
			res.Base.State = BaseStateInvalid
			res.Base.ErrorCode = CodeInputTooLarge
		} else {
			return TrustedLoadResult{}, wrapCode(CodeBaseReadFailed, "base", "base waiver read failed", berr)
		}
	} else {
		res.Base.File = fileMeta(req.Base, baseFile)
		if baseFile.Kind != config.EntryKindRegularFile || baseFile.Executable {
			res.Base.State = BaseStateUnsupportedEntryKind
			res.Base.ErrorCode = CodeUnsupportedEntryKind
		} else if int64(len(baseFile.Data)) > l.MaxWaiverFileBytes {
			res.Base.State = BaseStateInvalid
			res.Base.ErrorCode = CodeInputTooLarge
		} else {
			baseRaw = append([]byte(nil), baseFile.Data...)
			baseSet, err = Parse(baseFile.Data, l)
			if err != nil {
				res.Base.State = BaseStateInvalid
				if we, ok := err.(*Error); ok {
					res.Base.ErrorCode = we.Code
				}
			} else {
				res.Base.State = BaseStateValid
				res.Base.Waivers = cloneWaivers(baseSet.Waivers)
				res.Base.SemanticDigest = baseSet.SemanticDigest
			}
		}
	}
	headFile, herr := source.ReadFile(ctx, req.Head, WaiverPath, l.MaxWaiverFileBytes)
	if err := ctx.Err(); err != nil {
		return TrustedLoadResult{}, contextErr(err)
	}
	if herr != nil {
		if errors.Is(herr, fs.ErrNotExist) {
			if basePresent {
				res.Head.State = HeadStateRemoved
			} else {
				res.Head.State = HeadStateAbsentOnBoth
			}
			return cloneLoadResult(res), nil
		}
		if errors.Is(herr, config.ErrRevisionFileTooLarge) {
			res.Head.State = HeadStateModifiedInvalid
			res.Head.ErrorCode = CodeInputTooLarge
			return cloneLoadResult(res), nil
		}
		return TrustedLoadResult{}, wrapCode(CodeHeadInspectionFailed, "head", "head waiver inspection failed", herr)
	}
	res.Head.File = fileMeta(req.Head, headFile)
	if headFile.Kind != config.EntryKindRegularFile || headFile.Executable {
		res.Head.State = HeadStateUnsupportedEntryKind
		res.Head.ErrorCode = CodeUnsupportedEntryKind
		return cloneLoadResult(res), nil
	}
	if !basePresent {
		headSet, err := Parse(headFile.Data, l)
		if err != nil {
			res.Head.State = HeadStateModifiedInvalid
			if we, ok := err.(*Error); ok {
				res.Head.ErrorCode = we.Code
			}
		} else {
			res.Head.State = HeadStateAdded
			res.Head.Waivers = cloneWaivers(headSet.Waivers)
			res.Head.SemanticDigest = headSet.SemanticDigest
			res.Head.Changes = addedChanges(headSet.Waivers)
		}
		return cloneLoadResult(res), nil
	}
	if bytesEqual(baseRaw, headFile.Data) {
		res.Head.State = HeadStateUnchanged
		if baseSet.SemanticDigest != "" {
			res.Head.Waivers = cloneWaivers(baseSet.Waivers)
			res.Head.SemanticDigest = baseSet.SemanticDigest
		}
		return cloneLoadResult(res), nil
	}
	headSet, err := Parse(headFile.Data, l)
	if err != nil {
		res.Head.State = HeadStateModifiedInvalid
		if we, ok := err.(*Error); ok {
			res.Head.ErrorCode = we.Code
		}
		return cloneLoadResult(res), nil
	}
	res.Head.Waivers = cloneWaivers(headSet.Waivers)
	res.Head.SemanticDigest = headSet.SemanticDigest
	if res.Base.State == BaseStateValid && baseSet.SemanticDigest == headSet.SemanticDigest {
		res.Head.State = HeadStateContentChangedSemanticallyEquivalent
	} else {
		if res.Base.State == BaseStateValid {
			res.Head.State = HeadStateModifiedValid
			res.Head.Changes = compareSets(baseSet.Waivers, headSet.Waivers)
		} else {
			res.Head.State = HeadStateAdded
			res.Head.Changes = addedChanges(headSet.Waivers)
		}
	}
	return cloneLoadResult(res), nil
}

func fileMeta(rev model.CommitRef, f config.RevisionFile) FileMetadata {
	m := FileMetadata{Revision: rev, Path: WaiverPath, Kind: f.Kind, Executable: f.Executable, ObjectID: f.ObjectID}
	if f.Data != nil {
		m.SizeBytes = int64(len(f.Data))
		m.Digest = rawDigest(f.Data)
	}
	return m
}
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func addedChanges(ws []Waiver) []Change {
	out := make([]Change, 0, len(ws))
	for _, w := range ws {
		out = append(out, Change{WaiverID: w.ID, Kind: ChangeWaiverAdded})
	}
	sortChanges(out)
	return out
}
func compareSets(base, head []Waiver) []Change {
	out := []Change{}
	b := map[string]Waiver{}
	h := map[string]Waiver{}
	for _, w := range base {
		b[w.ID] = w
	}
	for _, w := range head {
		h[w.ID] = w
	}
	for id, bw := range b {
		hw, ok := h[id]
		if !ok {
			out = append(out, Change{WaiverID: id, Kind: ChangeWaiverRemoved})
			continue
		}
		if bw.Target != hw.Target {
			out = append(out, Change{WaiverID: id, Kind: ChangeWaiverTargetChanged})
		}
		if bw.Owner != hw.Owner {
			out = append(out, Change{WaiverID: id, Kind: ChangeWaiverOwnerChanged})
		}
		if bw.Reason != hw.Reason {
			out = append(out, Change{WaiverID: id, Kind: ChangeWaiverReasonChanged})
		}
		if !bw.IssuedAt.Equal(hw.IssuedAt) {
			out = append(out, Change{WaiverID: id, Kind: ChangeWaiverIssuedAtChanged})
		}
		if !bw.ExpiresAt.Equal(hw.ExpiresAt) {
			out = append(out, Change{WaiverID: id, Kind: ChangeWaiverExpiryChanged})
		}
	}
	for id := range h {
		if _, ok := b[id]; !ok {
			out = append(out, Change{WaiverID: id, Kind: ChangeWaiverAdded})
		}
	}
	sortChanges(out)
	return out
}
func sortChanges(in []Change) {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].WaiverID != in[j].WaiverID {
			return in[i].WaiverID < in[j].WaiverID
		}
		return in[i].Kind < in[j].Kind
	})
}
func cloneLoadResult(in TrustedLoadResult) TrustedLoadResult {
	out := in
	out.Base.Waivers = cloneWaivers(in.Base.Waivers)
	out.Head.Waivers = cloneWaivers(in.Head.Waivers)
	out.Head.Changes = append([]Change(nil), in.Head.Changes...)
	if out.Base.Waivers == nil {
		out.Base.Waivers = []Waiver{}
	}
	if out.Head.Waivers == nil {
		out.Head.Waivers = []Waiver{}
	}
	if out.Head.Changes == nil {
		out.Head.Changes = []Change{}
	}
	return out
}
