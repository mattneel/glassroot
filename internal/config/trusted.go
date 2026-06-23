package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"

	"github.com/mattneel/glassroot/internal/model"
)

type TrustedLoadRequest struct {
	Base model.CommitRef
	Head model.CommitRef
}

type TrustedLoadResult struct {
	Base              model.CommitRef
	Head              model.CommitRef
	EffectivePipeline ValidatedPipeline
	EffectiveSource   EffectiveConfigSource
	BaseFile          ConfigFileMetadata
	HeadAssessment    HeadAssessment
}

type EffectiveSource string

const EffectiveSourceBase EffectiveSource = "base"

type EffectiveConfigSource struct {
	Source EffectiveSource
	Path   string
	File   ConfigFileMetadata
}

type ConfigFileMetadata struct {
	Revision   model.CommitRef
	Path       string
	Kind       EntryKind
	Digest     model.Digest
	SizeBytes  int64
	Executable bool
	ObjectID   string
}

type HeadAssessmentState string

const (
	HeadStateUnchanged                            HeadAssessmentState = "unchanged"
	HeadStateContentChangedSemanticallyEquivalent HeadAssessmentState = "content-changed-but-semantically-equivalent"
	HeadStateModifiedValid                        HeadAssessmentState = "modified-valid"
	HeadStateModifiedInvalid                      HeadAssessmentState = "modified-invalid"
	HeadStateRemoved                              HeadAssessmentState = "removed"
	HeadStateUnsupportedEntryKind                 HeadAssessmentState = "unsupported-entry-kind"
)

type HeadAssessment struct {
	State       HeadAssessmentState
	BaseFile    ConfigFileMetadata
	HeadFile    ConfigFileMetadata
	Diagnostics Diagnostics
	Changes     []ConfigChange
}

type TrustedConfigErrorCode string

const (
	TrustedErrorBaseConfigMissing    TrustedConfigErrorCode = "base-config-missing"
	TrustedErrorBaseConfigInvalid    TrustedConfigErrorCode = "base-config-invalid"
	TrustedErrorUnsupportedBaseEntry TrustedConfigErrorCode = "unsupported-base-entry"
	TrustedErrorBaseReadFailed       TrustedConfigErrorCode = "base-read-failed"
	TrustedErrorHeadInspectionFailed TrustedConfigErrorCode = "head-inspection-failed"
	TrustedErrorContextCancelled     TrustedConfigErrorCode = "context-cancelled"
)

type trustedErrorSentinel TrustedConfigErrorCode

func (e trustedErrorSentinel) Error() string { return string(e) }

var (
	ErrBaseConfigMissing    error = trustedErrorSentinel(TrustedErrorBaseConfigMissing)
	ErrBaseConfigInvalid    error = trustedErrorSentinel(TrustedErrorBaseConfigInvalid)
	ErrUnsupportedBaseEntry error = trustedErrorSentinel(TrustedErrorUnsupportedBaseEntry)
	ErrBaseReadFailed       error = trustedErrorSentinel(TrustedErrorBaseReadFailed)
	ErrHeadInspectionFailed error = trustedErrorSentinel(TrustedErrorHeadInspectionFailed)
	ErrContextCancelled     error = trustedErrorSentinel(TrustedErrorContextCancelled)
)

type TrustedConfigError struct {
	Code        TrustedConfigErrorCode
	Role        string
	Path        string
	EntryKind   EntryKind
	Diagnostics Diagnostics
	Err         error
}

func (e *TrustedConfigError) Error() string {
	if e == nil {
		return "trusted configuration error"
	}
	msg := string(e.Code)
	if e.Role != "" {
		msg += ": " + e.Role
	}
	if e.Path != "" {
		msg += ": " + e.Path
	}
	if e.EntryKind != "" {
		msg += ": " + string(e.EntryKind)
	}
	if len(e.Diagnostics) > 0 {
		msg += fmt.Sprintf(": %d diagnostic", len(e.Diagnostics))
		if len(e.Diagnostics) != 1 {
			msg += "s"
		}
	}
	if e.Err != nil && len(e.Diagnostics) == 0 {
		msg += ": " + sanitizeForDiagnostic(e.Err.Error(), 256)
	}
	return msg
}

func (e *TrustedConfigError) Unwrap() error { return e.Err }

func (e *TrustedConfigError) Is(target error) bool {
	sentinel, ok := target.(trustedErrorSentinel)
	return ok && e != nil && TrustedConfigErrorCode(sentinel) == e.Code
}

func LoadTrusted(ctx context.Context, source RevisionFileSource, request TrustedLoadRequest) (TrustedLoadResult, error) {
	if err := ctx.Err(); err != nil {
		return TrustedLoadResult{}, trustedErr(TrustedErrorContextCancelled, "base", err)
	}
	if source == nil {
		return TrustedLoadResult{}, trustedErr(TrustedErrorBaseReadFailed, "base", errors.New("nil revision file source"))
	}

	baseFile, err := source.ReadFile(ctx, request.Base, PipelinePath, MaxPipelineBytes)
	if err != nil {
		return TrustedLoadResult{}, classifyBaseReadError(err)
	}
	if err := ctx.Err(); err != nil {
		return TrustedLoadResult{}, trustedErr(TrustedErrorContextCancelled, "base", err)
	}
	baseMeta := metadataForFile(request.Base, baseFile)
	if baseFile.Kind != EntryKindRegularFile {
		return TrustedLoadResult{}, &TrustedConfigError{Code: TrustedErrorUnsupportedBaseEntry, Role: "base", Path: PipelinePath, EntryKind: baseFile.Kind}
	}
	if len(baseFile.Data) > MaxPipelineBytes {
		return TrustedLoadResult{}, &TrustedConfigError{Code: TrustedErrorBaseConfigInvalid, Role: "base", Path: PipelinePath, Diagnostics: Diagnostics{newDiagnostic(CodeInputTooLarge, "", 0, 0, "base pipeline exceeds byte limit")}}
	}
	basePipeline, err := ParseAndValidate(baseFile.Data)
	if err != nil {
		return TrustedLoadResult{}, &TrustedConfigError{Code: TrustedErrorBaseConfigInvalid, Role: "base", Path: PipelinePath, Diagnostics: diagnosticsFromError(err), Err: err}
	}
	basePipeline = cloneValidatedPipeline(basePipeline)

	assessment, err := assessHead(ctx, source, request.Head, baseMeta, baseFile.Data, basePipeline)
	if err != nil {
		return TrustedLoadResult{}, err
	}
	baseFile.Data = nil
	return TrustedLoadResult{
		Base:              request.Base,
		Head:              request.Head,
		EffectivePipeline: cloneValidatedPipeline(basePipeline),
		EffectiveSource:   EffectiveConfigSource{Source: EffectiveSourceBase, Path: PipelinePath, File: baseMeta},
		BaseFile:          baseMeta,
		HeadAssessment:    assessment,
	}, nil
}

func assessHead(ctx context.Context, source RevisionFileSource, head model.CommitRef, baseMeta ConfigFileMetadata, baseRaw []byte, basePipeline ValidatedPipeline) (HeadAssessment, error) {
	if err := ctx.Err(); err != nil {
		return HeadAssessment{}, trustedErr(TrustedErrorContextCancelled, "head", err)
	}
	headFile, err := source.ReadFile(ctx, head, PipelinePath, MaxPipelineBytes)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return HeadAssessment{State: HeadStateRemoved, BaseFile: baseMeta}, nil
		}
		if errors.Is(err, ErrRevisionFileTooLarge) {
			return HeadAssessment{State: HeadStateModifiedInvalid, BaseFile: baseMeta, Diagnostics: Diagnostics{newDiagnostic(CodeInputTooLarge, "", 0, 0, "head pipeline exceeds byte limit")}}, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return HeadAssessment{}, trustedErr(TrustedErrorContextCancelled, "head", err)
		}
		return HeadAssessment{}, trustedErr(TrustedErrorHeadInspectionFailed, "head", err)
	}
	if err := ctx.Err(); err != nil {
		return HeadAssessment{}, trustedErr(TrustedErrorContextCancelled, "head", err)
	}
	headMeta := metadataForFile(head, headFile)
	assessment := HeadAssessment{BaseFile: baseMeta, HeadFile: headMeta}
	if headFile.Kind != EntryKindRegularFile {
		assessment.State = HeadStateUnsupportedEntryKind
		return assessment, nil
	}
	if len(headFile.Data) > MaxPipelineBytes {
		assessment.State = HeadStateModifiedInvalid
		assessment.Diagnostics = Diagnostics{newDiagnostic(CodeInputTooLarge, "", 0, 0, "head pipeline exceeds byte limit")}
		return assessment, nil
	}
	if bytesEqual(baseRaw, headFile.Data) {
		assessment.State = HeadStateUnchanged
		return assessment, nil
	}
	headPipeline, err := ParseAndValidate(headFile.Data)
	if err != nil {
		assessment.State = HeadStateModifiedInvalid
		assessment.Diagnostics = diagnosticsFromError(err)
		return assessment, nil
	}
	changes := ComparePipelines(basePipeline, headPipeline)
	if len(changes) == 0 {
		assessment.State = HeadStateContentChangedSemanticallyEquivalent
	} else {
		assessment.State = HeadStateModifiedValid
		assessment.Changes = cloneConfigChanges(changes)
	}
	headFile.Data = nil
	return assessment, nil
}

func metadataForFile(revision model.CommitRef, file RevisionFile) ConfigFileMetadata {
	meta := ConfigFileMetadata{Revision: revision, Path: PipelinePath, Kind: file.Kind, Executable: file.Executable, ObjectID: file.ObjectID}
	if file.Data != nil {
		meta.SizeBytes = int64(len(file.Data))
		meta.Digest = digestBytes(file.Data)
	}
	return meta
}

func digestBytes(data []byte) model.Digest {
	sum := sha256.Sum256(data)
	return model.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

func classifyBaseReadError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return trustedErr(TrustedErrorContextCancelled, "base", err)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return trustedErr(TrustedErrorBaseConfigMissing, "base", err)
	}
	if errors.Is(err, ErrRevisionFileTooLarge) {
		return &TrustedConfigError{Code: TrustedErrorBaseConfigInvalid, Role: "base", Path: PipelinePath, Diagnostics: Diagnostics{newDiagnostic(CodeInputTooLarge, "", 0, 0, "base pipeline exceeds byte limit")}, Err: err}
	}
	return trustedErr(TrustedErrorBaseReadFailed, "base", err)
}

func trustedErr(code TrustedConfigErrorCode, role string, err error) error {
	return &TrustedConfigError{Code: code, Role: role, Path: PipelinePath, Err: err}
}

func diagnosticsFromError(err error) Diagnostics {
	var diags Diagnostics
	if errors.As(err, &diags) {
		return capDiagnostics(diags)
	}
	return Diagnostics{newDiagnostic(CodeInvalidValue, "", 0, 0, err.Error())}
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
