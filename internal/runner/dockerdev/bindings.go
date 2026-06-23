package dockerdev

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

func New(cfg Config) (*Runner, error) {
	if cfg.Engine == nil {
		return nil, errCode(CodeInvalidRunnerConfig, "config", "", "Docker engine is required", nil)
	}
	if !cfg.Acknowledgement.accepted {
		return nil, errCode(CodeAcknowledgementRequired, "acknowledgement", "", "unsafe-development acknowledgement is required", nil)
	}
	limits, err := validateLimits(cfg.Limits)
	if err != nil {
		return nil, err
	}
	bindings, err := validateWorkspaceBindings(cfg.Workspaces)
	if err != nil {
		return nil, err
	}
	return &Runner{engine: cfg.Engine, limits: limits, workspaces: bindings}, nil
}

func validateLimits(l Limits) (Limits, error) {
	if l.MaxStdoutBytes < 0 || l.MaxStderrBytes < 0 || l.MaxTotalOutputBytes <= 0 || l.TmpfsSizeBytes <= 0 || l.ShmSizeBytes <= 0 || l.StopGracePeriod <= 0 {
		return Limits{}, errCode(CodeInvalidRunnerConfig, "limits", "", "limits must be positive", nil)
	}
	if l.MaxStdoutBytes > l.MaxTotalOutputBytes || l.MaxStderrBytes > l.MaxTotalOutputBytes {
		return Limits{}, errCode(CodeInvalidRunnerConfig, "limits", "", "per-stream output limits must not exceed total", nil)
	}
	return l, nil
}

func validateWorkspaceBindings(in []WorkspaceBinding) (map[string]WorkspaceBinding, error) {
	out := make(map[string]WorkspaceBinding, len(in))
	infos := make([]struct {
		path string
		info os.FileInfo
		id   string
	}, 0, len(in))
	for _, b := range in {
		if err := validateWorkspaceBindingSyntax(b); err != nil {
			return nil, err
		}
		info, err := os.Lstat(b.HostPath)
		if err != nil {
			return nil, errCode(CodeInvalidWorkspaceBinding, "workspace", b.AttemptID, "workspace directory is not accessible", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errCode(CodeInvalidWorkspaceBinding, "workspace", b.AttemptID, "workspace final component is a symlink", nil)
		}
		if !info.IsDir() {
			return nil, errCode(CodeInvalidWorkspaceBinding, "workspace", b.AttemptID, "workspace is not a directory", nil)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return nil, errCode(CodeInvalidWorkspaceBinding, "workspace", b.AttemptID, "workspace must be private", nil)
		}
		if _, exists := out[b.AttemptID]; exists {
			return nil, errCode(CodeDuplicateWorkspace, "workspace", b.AttemptID, "duplicate attempt workspace binding", nil)
		}
		for _, prev := range infos {
			if os.SameFile(info, prev.info) {
				return nil, errCode(CodeDuplicateWorkspace, "workspace", b.AttemptID, "duplicate workspace directory", nil)
			}
			if pathsOverlap(prev.path, b.HostPath) {
				return nil, errCode(CodeWorkspaceOverlap, "workspace", b.AttemptID, "workspace directories overlap", nil)
			}
		}
		cp := b
		cp.HostPath = string([]byte(b.HostPath))
		cp.AttemptID = string([]byte(b.AttemptID))
		cp.ScenarioID = string([]byte(b.ScenarioID))
		out[cp.AttemptID] = cp
		infos = append(infos, struct {
			path string
			info os.FileInfo
			id   string
		}{cp.HostPath, info, cp.AttemptID})
	}
	return out, nil
}

func validateWorkspaceBindingSyntax(b WorkspaceBinding) error {
	if b.AttemptID == "" || len(b.AttemptID) > runner.MaxAttemptIDBytes || !utf8.ValidString(b.AttemptID) {
		return errCode(CodeInvalidWorkspaceBinding, "workspace", "", "attempt ID is required", nil)
	}
	if b.HostPath == "" || len(b.HostPath) > 4096 || !utf8.ValidString(b.HostPath) {
		return errCode(CodeInvalidWorkspaceBinding, "workspace", b.AttemptID, "workspace path is invalid", nil)
	}
	for _, r := range b.HostPath {
		if r == 0 || r < 0x20 || r == 0x7f {
			return errCode(CodeInvalidWorkspaceBinding, "workspace", b.AttemptID, "workspace path contains a control character", nil)
		}
	}
	if !filepath.IsAbs(b.HostPath) || filepath.Clean(b.HostPath) != b.HostPath {
		return errCode(CodeInvalidWorkspaceBinding, "workspace", b.AttemptID, "workspace path must be absolute and clean", nil)
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	rel, err := filepath.Rel(a, b)
	if err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return true
	}
	rel, err = filepath.Rel(b, a)
	if err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return true
	}
	return false
}

func (r *Runner) ValidatePlan(ctx context.Context, digest model.Digest, attempts []runner.AttemptRequest, _ runner.Limits) error {
	if err := ctx.Err(); err != nil {
		return errCode(CodeContextCancelled, "validate-plan", "", "context cancelled", err)
	}
	seen := make(map[string]struct{}, len(attempts))
	for _, a := range attempts {
		if a.PlanDigest != digest {
			return errCode(CodeInvalidRunnerConfig, "validate-plan", a.AttemptID, "attempt plan digest mismatch", nil)
		}
		b, ok := r.workspaces[a.AttemptID]
		if !ok {
			return errCode(CodeMissingWorkspaceBinding, "validate-plan", a.AttemptID, "missing workspace binding", nil)
		}
		if b.Revision != a.Revision || b.ScenarioID != a.ScenarioID || b.Repetition != a.Repetition || (b.MaterializedTreeDigest != "" && b.MaterializedTreeDigest != a.MaterializedTreeDigest) {
			return errCode(CodeInvalidWorkspaceBinding, "validate-plan", a.AttemptID, "workspace binding does not match planned attempt", nil)
		}
		seen[a.AttemptID] = struct{}{}
	}
	for id := range r.workspaces {
		if _, ok := seen[id]; !ok {
			return errCode(CodeExtraWorkspaceBinding, "validate-plan", id, "extra workspace binding", nil)
		}
	}
	return nil
}
