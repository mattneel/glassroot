package gitstore

import (
	"errors"
	"fmt"
	"strings"
)

type ErrorCode string

const (
	CodeGitNotFound                 ErrorCode = "git-not-found"
	CodeUnsupportedGitVersion       ErrorCode = "unsupported-git-version"
	CodeInvalidRepositoryPath       ErrorCode = "invalid-repository-path"
	CodeRepositoryNotBare           ErrorCode = "repository-not-bare"
	CodeUnsupportedRepositoryLayout ErrorCode = "unsupported-repository-layout"
	CodeRepositoryConfigInvalid     ErrorCode = "repository-config-invalid"
	CodeRepositoryConfigInclude     ErrorCode = "repository-config-include"
	CodeAlternateObjectStore        ErrorCode = "alternate-object-store"
	CodePartialCloneUnsupported     ErrorCode = "partial-clone-unsupported"
	CodeUnsupportedObjectFormat     ErrorCode = "unsupported-object-format"
	CodeInvalidRevisionSelector     ErrorCode = "invalid-revision-selector"
	CodeRevisionNotFound            ErrorCode = "revision-not-found"
	CodeRevisionNotCommit           ErrorCode = "revision-not-commit"
	CodeGitCommandFailed            ErrorCode = "git-command-failed"
	CodeGitCommandTimeout           ErrorCode = "git-command-timeout"
	CodeGitOutputTooLarge           ErrorCode = "git-output-too-large"
	CodeMalformedGitOutput          ErrorCode = "malformed-git-output"
	CodeInvalidObjectID             ErrorCode = "invalid-object-id"
	CodeTreeEntryLimit              ErrorCode = "tree-entry-limit"
	CodeTreeInvalid                 ErrorCode = "tree-invalid"
	CodeUnsupportedEntryMode        ErrorCode = "unsupported-entry-mode"
	CodeInvalidTreePath             ErrorCode = "invalid-tree-path"
	CodeDuplicateTreePath           ErrorCode = "duplicate-tree-path"
	CodeTreePathConflict            ErrorCode = "tree-path-conflict"
	CodeBlobTooLarge                ErrorCode = "blob-too-large"
	CodeBlobTypeMismatch            ErrorCode = "blob-type-mismatch"
	CodeBlobSizeMismatch            ErrorCode = "blob-size-mismatch"
	CodeBlobObjectIDMismatch        ErrorCode = "blob-object-id-mismatch"
	CodeContextCancelled            ErrorCode = "context-cancelled"
)

func (c ErrorCode) Error() string { return string(c) }

var (
	ErrGitNotFound                 error = CodeGitNotFound
	ErrUnsupportedGitVersion       error = CodeUnsupportedGitVersion
	ErrInvalidRepositoryPath       error = CodeInvalidRepositoryPath
	ErrRepositoryNotBare           error = CodeRepositoryNotBare
	ErrUnsupportedRepositoryLayout error = CodeUnsupportedRepositoryLayout
	ErrRepositoryConfigInvalid     error = CodeRepositoryConfigInvalid
	ErrRepositoryConfigInclude     error = CodeRepositoryConfigInclude
	ErrAlternateObjectStore        error = CodeAlternateObjectStore
	ErrPartialCloneUnsupported     error = CodePartialCloneUnsupported
	ErrUnsupportedObjectFormat     error = CodeUnsupportedObjectFormat
	ErrInvalidRevisionSelector     error = CodeInvalidRevisionSelector
	ErrRevisionNotFound            error = CodeRevisionNotFound
	ErrRevisionNotCommit           error = CodeRevisionNotCommit
	ErrGitCommandFailed            error = CodeGitCommandFailed
	ErrGitCommandTimeout           error = CodeGitCommandTimeout
	ErrGitOutputTooLarge           error = CodeGitOutputTooLarge
	ErrMalformedGitOutput          error = CodeMalformedGitOutput
	ErrInvalidObjectID             error = CodeInvalidObjectID
	ErrTreeEntryLimit              error = CodeTreeEntryLimit
	ErrTreeInvalid                 error = CodeTreeInvalid
	ErrUnsupportedEntryMode        error = CodeUnsupportedEntryMode
	ErrInvalidTreePath             error = CodeInvalidTreePath
	ErrDuplicateTreePath           error = CodeDuplicateTreePath
	ErrTreePathConflict            error = CodeTreePathConflict
	ErrBlobTooLarge                error = CodeBlobTooLarge
	ErrBlobTypeMismatch            error = CodeBlobTypeMismatch
	ErrBlobSizeMismatch            error = CodeBlobSizeMismatch
	ErrBlobObjectIDMismatch        error = CodeBlobObjectIDMismatch
	ErrContextCancelled            error = CodeContextCancelled
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
		return "gitstore error"
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

func gitErr(code ErrorCode, stage, op, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Op: op, Msg: msg, Err: err}
}

func pathErr(code ErrorCode, stage, op, path, msg string, err error) error {
	return &Error{Code: code, Stage: stage, Op: op, Path: path, Msg: msg, Err: err}
}

func codeForContext(err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: CodeContextCancelled, Stage: "context", Err: err}
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

func remapCommandErr(err error, fallback ErrorCode) error {
	if err == nil {
		return nil
	}
	var ge *Error
	if errors.As(err, &ge) {
		return err
	}
	return gitErr(fallback, "git", "command", "", err)
}
