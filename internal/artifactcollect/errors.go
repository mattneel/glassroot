package artifactcollect

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ErrorCode string

const (
	CodeUnsupportedPlatform      ErrorCode = "unsupported-platform"
	CodeInvalidLimits            ErrorCode = "invalid-limits"
	CodeInvalidWorkspacePath     ErrorCode = "invalid-workspace-path"
	CodeWorkspaceNotDirectory    ErrorCode = "workspace-not-directory"
	CodeWorkspaceSymlink         ErrorCode = "workspace-symlink"
	CodeWorkspaceModeInvalid     ErrorCode = "workspace-mode-invalid"
	CodeWorkspaceOpenFailed      ErrorCode = "workspace-open-failed"
	CodeWorkspaceChanged         ErrorCode = "workspace-changed"
	CodeInvalidCollectionPlan    ErrorCode = "invalid-collection-plan"
	CodeInvalidAttempt           ErrorCode = "invalid-attempt"
	CodeInvalidWorkdir           ErrorCode = "invalid-workdir"
	CodeInvalidArtifactPattern   ErrorCode = "invalid-artifact-pattern"
	CodeArtifactOutsideWorkdir   ErrorCode = "artifact-outside-workdir"
	CodeDuplicateArtifactPattern ErrorCode = "duplicate-artifact-pattern"
	CodeInventoryLimit           ErrorCode = "inventory-limit"
	CodeInvalidEntryName         ErrorCode = "invalid-entry-name"
	CodeInvalidEntryPath         ErrorCode = "invalid-entry-path"
	CodeFilesystemBoundary       ErrorCode = "filesystem-boundary"
	CodeUnsupportedEntryType     ErrorCode = "unsupported-entry-type"
	CodeHardlinkEntry            ErrorCode = "hardlink-entry"
	CodeTreePathConflict         ErrorCode = "tree-path-conflict"
	CodePatternLimit             ErrorCode = "pattern-limit"
	CodeMatchedArtifactLimit     ErrorCode = "matched-artifact-limit"
	CodeTotalBytesLimit          ErrorCode = "total-bytes-limit"
	CodeFileOpenFailed           ErrorCode = "file-open-failed"
	CodeFileChanged              ErrorCode = "file-changed"
	CodeFileSizeMismatch         ErrorCode = "file-size-mismatch"
	CodeFileDigestMismatch       ErrorCode = "file-digest-mismatch"
	CodeSinkFailed               ErrorCode = "sink-failed"
	CodeSinkShortRead            ErrorCode = "sink-short-read"
	CodeSinkResultMismatch       ErrorCode = "sink-result-mismatch"
	CodeCloseFailed              ErrorCode = "close-failed"
	CodeContextCancelled         ErrorCode = "context-cancelled"
	CodeCollectionTimeout        ErrorCode = "collection-timeout"
)

type Error struct {
	Code    ErrorCode
	Stage   string
	Path    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return "artifactcollect: <nil>"
	}
	parts := []string{"artifactcollect", string(e.Code)}
	if e.Stage != "" {
		parts = append(parts, "stage="+sanitize(e.Stage))
	}
	if e.Path != "" {
		parts = append(parts, "path="+sanitize(e.Path))
	}
	if e.Message != "" {
		parts = append(parts, sanitize(e.Message))
	}
	return strings.Join(parts, ": ")
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) || t.Code == "" {
		return false
	}
	return e.Code == t.Code
}

func errCode(code ErrorCode, stage, logicalPath, msg string, cause error) error {
	return &Error{Code: code, Stage: stage, Path: bounded(logicalPath, 256), Message: bounded(msg, 256), Err: cause}
}

func sanitize(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 {
			b.WriteString(fmt.Sprintf("\\x%02x", s[0]))
			s = s[1:]
			continue
		}
		s = s[size:]
		if r == 0 || r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			b.WriteString(fmt.Sprintf("\\u{%X}", r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func bounded(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
