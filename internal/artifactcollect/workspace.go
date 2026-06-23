package artifactcollect

import (
	"context"
	"errors"
	"os"
)

func checkContext(ctx context.Context, stage string) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return errCode(CodeContextCancelled, stage, "", "context cancelled", err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return errCode(CodeCollectionTimeout, stage, "", "collection deadline exceeded", err)
		}
		return err
	}
	return nil
}

func (w *BoundWorkspace) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.root == nil {
		return nil
	}
	if err := w.root.Close(); err != nil {
		return errCode(CodeCloseFailed, "close", "", "close workspace root", err)
	}
	return nil
}

func (w *BoundWorkspace) Collect(ctx context.Context, plan CollectionPlan, sink ArtifactSink) (*Result, error) {
	if w == nil {
		return nil, errCode(CodeWorkspaceOpenFailed, "collect", "", "workspace is nil", nil)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, w.limits.MaxCollectionDuration)
	defer cancel()
	if sink == nil {
		return nil, errCode(CodeInvalidCollectionPlan, "collect", "", "artifact sink is required", nil)
	}
	w.mu.Lock()
	if w.closed || w.root == nil {
		w.mu.Unlock()
		return nil, errCode(CodeWorkspaceOpenFailed, "collect", "", "workspace is closed", nil)
	}
	if w.collected {
		w.mu.Unlock()
		return nil, errCode(CodeInvalidCollectionPlan, "collect", "", "workspace collection already completed", nil)
	}
	w.collected = true
	w.mu.Unlock()

	if err := checkContext(ctx, "collect"); err != nil {
		return nil, err
	}
	vp, err := validatePlan(ctx, plan, w.limits)
	if err != nil {
		return nil, err
	}
	if err := w.checkRootStable("preflight"); err != nil {
		return nil, err
	}
	pre, err := w.inventory(ctx)
	if err != nil {
		return nil, err
	}
	if w.hooks.AfterPreflight != nil {
		if err := w.hooks.AfterPreflight(); err != nil {
			return nil, errCode(CodeWorkspaceChanged, "preflight", "", "preflight hook reported mutation", err)
		}
	}
	result, err := w.collectFromInventory(ctx, vp, pre, sink)
	if err != nil {
		return nil, err
	}
	post, err := w.inventory(ctx)
	if err != nil {
		return nil, err
	}
	if err := reconcileInventories(pre, post); err != nil {
		return nil, err
	}
	if err := w.checkRootStable("final"); err != nil {
		return nil, err
	}
	return result, nil
}

func (w *BoundWorkspace) checkRootStable(stage string) error {
	info, err := w.root.Stat(".")
	if err != nil {
		return errCode(CodeWorkspaceChanged, stage, "", "stat opened workspace root", err)
	}
	id, err := identityFromInfo(info)
	if err != nil {
		return err
	}
	if !sameFileIdentity(w.identity, id) || info.Mode().Perm() != 0o700 {
		return errCode(CodeWorkspaceChanged, stage, "", "opened workspace root changed", nil)
	}
	if w.path != "" {
		pathInfo, err := os.Lstat(w.path)
		if err != nil {
			return errCode(CodeWorkspaceChanged, stage, "", "workspace path no longer identifies the root", err)
		}
		pathID, err := identityFromInfo(pathInfo)
		if err != nil {
			return err
		}
		if !sameFileIdentity(w.identity, pathID) || pathInfo.Mode().Perm() != 0o700 {
			return errCode(CodeWorkspaceChanged, stage, "", "workspace path identity changed", nil)
		}
	}
	return nil
}
