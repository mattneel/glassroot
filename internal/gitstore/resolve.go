package gitstore

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

type SelectorKind string

const (
	SelectorObjectID SelectorKind = "object-id"
	SelectorRef      SelectorKind = "ref"
	selectorRaw      SelectorKind = "raw"
)

type RevisionSelector struct {
	Kind  SelectorKind
	Value string
}

func ObjectIDSelector(value string) RevisionSelector {
	return RevisionSelector{Kind: SelectorObjectID, Value: value}
}
func RefSelector(value string) RevisionSelector {
	return RevisionSelector{Kind: SelectorRef, Value: value}
}
func RawSelector(value string) RevisionSelector {
	return RevisionSelector{Kind: selectorRaw, Value: value}
}

type ResolvedRevision struct {
	ObjectFormat     ObjectFormat
	CommitID         string
	TreeID           string
	GitVersion       GitVersion
	OriginalSelector string
}

func (r *Repository) ResolveCommit(ctx context.Context, selector RevisionSelector) (ResolvedRevision, error) {
	value, err := r.validateSelector(ctx, selector)
	if err != nil {
		return ResolvedRevision{}, err
	}
	arg := value + "^{commit}"
	out, err := r.cmd.runRepoGit(ctx, commandSpec{op: "rev-parse", args: []string{"rev-parse", "--verify", "--end-of-options", arg}, stdoutLimit: MaxGitStdoutBytes, stderrLimit: MaxGitStderrBytes, timeout: DefaultGitCommandTimeout})
	if err != nil {
		if selector.Kind == SelectorObjectID {
			if typ, typErr := r.objectType(ctx, value); typErr == nil && typ != "commit" && typ != "tag" {
				return ResolvedRevision{}, gitErr(CodeRevisionNotCommit, "resolve", "rev-parse", "selector does not peel to a commit", nil)
			}
		}
		return ResolvedRevision{}, gitErr(CodeRevisionNotFound, "resolve", "rev-parse", "revision not found or not a commit", err)
	}
	commitID := strings.TrimSpace(string(out.Stdout))
	commitID, err = validateObjectID(commitID, r.objectFormat, false)
	if err != nil {
		return ResolvedRevision{}, err
	}
	treeID, err := r.treeForCommit(ctx, commitID)
	if err != nil {
		return ResolvedRevision{}, err
	}
	return ResolvedRevision{ObjectFormat: r.objectFormat, CommitID: commitID, TreeID: treeID, GitVersion: r.version, OriginalSelector: value}, nil
}

func (r *Repository) validateSelector(ctx context.Context, selector RevisionSelector) (string, error) {
	if len(selector.Value) == 0 || len(selector.Value) > MaxRevisionSelectorBytes || !utf8.ValidString(selector.Value) || strings.ContainsRune(selector.Value, 0) || containsControl(selector.Value) {
		return "", gitErr(CodeInvalidRevisionSelector, "resolve", "selector", "selector is empty, invalid, or too large", nil)
	}
	switch selector.Kind {
	case SelectorObjectID:
		return validateObjectID(selector.Value, r.objectFormat, true)
	case SelectorRef:
		if !strings.HasPrefix(selector.Value, "refs/") || hasRevisionSyntax(selector.Value) {
			return "", gitErr(CodeInvalidRevisionSelector, "resolve", "selector", "ref selector must be fully qualified without revision syntax", nil)
		}
		out, err := r.cmd.runGit(ctx, commandSpec{op: "check-ref-format", args: []string{"check-ref-format", "--normalize", selector.Value}, stdoutLimit: MaxGitStdoutBytes, stderrLimit: MaxGitStderrBytes, timeout: DefaultGitCommandTimeout})
		if err != nil {
			return "", gitErr(CodeInvalidRevisionSelector, "resolve", "check-ref-format", "invalid ref selector", err)
		}
		normalized := strings.TrimSpace(string(out.Stdout))
		if normalized != selector.Value {
			return "", gitErr(CodeInvalidRevisionSelector, "resolve", "check-ref-format", "ref normalization changed selector", nil)
		}
		return selector.Value, nil
	default:
		return "", gitErr(CodeInvalidRevisionSelector, "resolve", "selector", "selector must be an object ID or fully qualified ref", nil)
	}
}

func hasRevisionSyntax(s string) bool {
	return strings.ContainsAny(s, "~^:?*[\\{} ") || strings.Contains(s, "..") || strings.Contains(s, "@{")
}

func (r *Repository) objectType(ctx context.Context, oid string) (string, error) {
	out, err := r.cmd.runRepoGit(ctx, commandSpec{op: "cat-file", args: []string{"cat-file", "-t", oid}, stdoutLimit: MaxGitStdoutBytes, stderrLimit: MaxGitStderrBytes, timeout: DefaultGitCommandTimeout})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out.Stdout)), nil
}

func (r *Repository) treeForCommit(ctx context.Context, commitID string) (string, error) {
	out, err := r.cmd.runRepoGit(ctx, commandSpec{op: "rev-parse", args: []string{"rev-parse", "--verify", "--end-of-options", commitID + "^{tree}"}, stdoutLimit: MaxGitStdoutBytes, stderrLimit: MaxGitStderrBytes, timeout: DefaultGitCommandTimeout})
	if err != nil {
		return "", gitErr(CodeRevisionNotCommit, "resolve", "tree", "commit tree not found", err)
	}
	treeID := strings.TrimSpace(string(out.Stdout))
	return validateObjectID(treeID, r.objectFormat, false)
}

func resolvedFromCommitRef(format ObjectFormat, version GitVersion, commitID, treeID string) (ResolvedRevision, error) {
	commit, err := validateObjectID(commitID, format, true)
	if err != nil {
		return ResolvedRevision{}, gitErr(CodeInvalidRevisionSelector, "source", "commit-ref", "CommitRef.CommitID must be a full object ID", err)
	}
	resolved := ResolvedRevision{ObjectFormat: format, CommitID: commit, GitVersion: version, OriginalSelector: commit}
	if treeID != "" {
		tree, err := validateObjectID(treeID, format, true)
		if err != nil {
			return ResolvedRevision{}, gitErr(CodeInvalidRevisionSelector, "source", "commit-ref", "CommitRef.TreeDigest must be a full tree object ID when present", err)
		}
		resolved.TreeID = tree
	}
	return resolved, nil
}

func isNotFoundGitError(err error) bool {
	return errors.Is(err, ErrRevisionNotFound)
}
