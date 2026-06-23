package materialize

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ErrorCode string

const (
	CodeUnsupportedPlatform      ErrorCode = "unsupported-platform"
	CodeInvalidParent            ErrorCode = "invalid-parent"
	CodeParentNotDirectory       ErrorCode = "parent-not-directory"
	CodeParentSymlink            ErrorCode = "parent-symlink"
	CodeParentOverlapsRepository ErrorCode = "parent-overlaps-repository"
	CodeWorkspaceCreateFailed    ErrorCode = "workspace-create-failed"
	CodeWorkspaceOpenFailed      ErrorCode = "workspace-open-failed"
	CodeSourceTreeFailed         ErrorCode = "source-tree-failed"
	CodeInvalidTreeEntry         ErrorCode = "invalid-tree-entry"
	CodeDuplicateTreeEntry       ErrorCode = "duplicate-tree-entry"
	CodeTreePathConflict         ErrorCode = "tree-path-conflict"
	CodeEntryLimit               ErrorCode = "entry-limit"
	CodeTotalBytesLimit          ErrorCode = "total-bytes-limit"
	CodeInvalidObjectID          ErrorCode = "invalid-object-id"
	CodeBlobReadFailed           ErrorCode = "blob-read-failed"
	CodeBlobSizeMismatch         ErrorCode = "blob-size-mismatch"
	CodeBlobDigestMismatch       ErrorCode = "blob-digest-mismatch"
	CodeDestinationEntryExists   ErrorCode = "destination-entry-exists"
	CodeDirectoryCreateFailed    ErrorCode = "directory-create-failed"
	CodeFileCreateFailed         ErrorCode = "file-create-failed"
	CodeFileWriteFailed          ErrorCode = "file-write-failed"
	CodeFileModeFailed           ErrorCode = "file-mode-failed"
	CodeInvalidSymlinkTarget     ErrorCode = "invalid-symlink-target"
	CodeSymlinkCreateFailed      ErrorCode = "symlink-create-failed"
	CodeManifestLimit            ErrorCode = "manifest-limit"
	CodeMaterializationTimeout   ErrorCode = "materialization-timeout"
	CodeContextCancelled         ErrorCode = "context-cancelled"
	CodeCleanupFailed            ErrorCode = "cleanup-failed"
)

func (c ErrorCode) Error() string { return string(c) }

var (
	ErrUnsupportedPlatform      error = CodeUnsupportedPlatform
	ErrInvalidParent            error = CodeInvalidParent
	ErrParentNotDirectory       error = CodeParentNotDirectory
	ErrParentSymlink            error = CodeParentSymlink
	ErrParentOverlapsRepository error = CodeParentOverlapsRepository
	ErrWorkspaceCreateFailed    error = CodeWorkspaceCreateFailed
	ErrWorkspaceOpenFailed      error = CodeWorkspaceOpenFailed
	ErrSourceTreeFailed         error = CodeSourceTreeFailed
	ErrInvalidTreeEntry         error = CodeInvalidTreeEntry
	ErrDuplicateTreeEntry       error = CodeDuplicateTreeEntry
	ErrTreePathConflict         error = CodeTreePathConflict
	ErrEntryLimit               error = CodeEntryLimit
	ErrTotalBytesLimit          error = CodeTotalBytesLimit
	ErrInvalidObjectID          error = CodeInvalidObjectID
	ErrBlobReadFailed           error = CodeBlobReadFailed
	ErrBlobSizeMismatch         error = CodeBlobSizeMismatch
	ErrBlobDigestMismatch       error = CodeBlobDigestMismatch
	ErrDestinationEntryExists   error = CodeDestinationEntryExists
	ErrDirectoryCreateFailed    error = CodeDirectoryCreateFailed
	ErrFileCreateFailed         error = CodeFileCreateFailed
	ErrFileWriteFailed          error = CodeFileWriteFailed
	ErrFileModeFailed           error = CodeFileModeFailed
	ErrInvalidSymlinkTarget     error = CodeInvalidSymlinkTarget
	ErrSymlinkCreateFailed      error = CodeSymlinkCreateFailed
	ErrManifestLimit            error = CodeManifestLimit
	ErrMaterializationTimeout   error = CodeMaterializationTimeout
	ErrContextCancelled         error = CodeContextCancelled
	ErrCleanupFailed            error = CodeCleanupFailed
)

type Error struct {
	Code  ErrorCode
	Stage string
	Op    string
	Path  string
	Err   error
	Msg   string
}

func (e *Error) Error() string {
	if e == nil {
		return "materialize error"
	}
	parts := []string{string(e.Code)}
	if e.Stage != "" {
		parts = append(parts, e.Stage)
	}
	if e.Op != "" {
		parts = append(parts, e.Op)
	}
	if e.Path != "" {
		parts = append(parts, sanitize(e.Path, 160))
	}
	if e.Msg != "" {
		parts = append(parts, sanitize(e.Msg, 256))
	} else if e.Err != nil {
		parts = append(parts, sanitize(e.Err.Error(), 256))
	}
	return strings.Join(parts, ": ")
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Is(target error) bool {
	code, ok := target.(ErrorCode)
	return ok && e != nil && e.Code == code
}

func errCode(code ErrorCode, stage, op, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Op: op, Msg: msg, Err: err}
}

func pathErr(code ErrorCode, stage, op, path, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Op: op, Path: path, Msg: msg, Err: err}
}

func contextErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errCode(CodeMaterializationTimeout, "context", "deadline", "materialization deadline exceeded", err)
	}
	return errCode(CodeContextCancelled, "context", "cancelled", "context cancelled", err)
}

func sanitize(s string, max int) string {
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= max {
			break
		}
		if r == '\n' || r == '\t' || (r >= 0x20 && r != 0x7f) {
			b.WriteRune(r)
		} else {
			b.WriteString(fmt.Sprintf("\\x%02x", r))
		}
	}
	if len(s) > b.Len() {
		b.WriteString("...")
	}
	return b.String()
}
