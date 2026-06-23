package materialize

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/gitstore"
)

const maxParentPathBytes = 4096
const workspaceCreateAttempts = 16

type Workspace struct {
	path      string
	root      *os.Root
	removeAll func(string) error
	once      sync.Once
	closeErr  error
}

func (w *Workspace) Path() string {
	if w == nil {
		return ""
	}
	return w.path
}

func (w *Workspace) Close() error {
	if w == nil {
		return nil
	}
	w.once.Do(func() {
		if w.root != nil {
			if err := w.root.Close(); err != nil {
				w.closeErr = err
			}
			w.root = nil
		}
		removeAll := w.removeAll
		if removeAll == nil {
			removeAll = os.RemoveAll
		}
		if err := removeAll(w.path); err != nil && w.closeErr == nil {
			w.closeErr = err
		}
	})
	return w.closeErr
}

type Materializer struct {
	parentDir string
	limits    Limits
	hooks     materializationHooks
	removeAll func(string) error
}

type materializationHooks struct {
	BeforeOpenFile  func(workspacePath, repoPath string) error
	BeforeFileChmod func(workspacePath, repoPath string) error
}

type options struct {
	limits Limits
}

type Option func(*options)

func WithLimits(limits Limits) Option {
	return func(o *options) { o.limits = limits }
}

func New(parentDir string, opts ...Option) (*Materializer, error) {
	if runtime.GOOS != "linux" {
		return nil, errCode(CodeUnsupportedPlatform, "platform", "new", "materialization is initially supported only on Linux", nil)
	}
	if err := validateParent(parentDir); err != nil {
		return nil, err
	}
	options := options{limits: DefaultLimits()}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if err := validateLimits(options.limits); err != nil {
		return nil, err
	}
	return &Materializer{parentDir: parentDir, limits: options.limits, removeAll: os.RemoveAll}, nil
}

func validateParent(parent string) error {
	if parent == "" {
		return pathErr(CodeInvalidParent, "workspace", "parent", parent, "parent directory is required", nil)
	}
	if len(parent) > maxParentPathBytes || !utf8.ValidString(parent) || strings.ContainsRune(parent, 0) || containsControl(parent) {
		return pathErr(CodeInvalidParent, "workspace", "parent", parent, "parent path is invalid or too large", nil)
	}
	if !filepath.IsAbs(parent) {
		return pathErr(CodeInvalidParent, "workspace", "parent", parent, "parent path must be absolute", nil)
	}
	if filepath.Clean(parent) != parent {
		return pathErr(CodeInvalidParent, "workspace", "parent", parent, "parent path must be clean", nil)
	}
	st, err := os.Lstat(parent)
	if err != nil {
		return pathErr(CodeInvalidParent, "workspace", "parent", parent, "stat parent", err)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return pathErr(CodeParentSymlink, "workspace", "parent", parent, "parent final component is a symlink", nil)
	}
	if !st.IsDir() {
		return pathErr(CodeParentNotDirectory, "workspace", "parent", parent, "parent is not a directory", nil)
	}
	return nil
}

func validateParentAgainstRepository(parent string, repo *gitstore.Repository) error {
	if repo == nil {
		return errCode(CodeSourceTreeFailed, "source", "repository", "repository is nil", nil)
	}
	gitDir := repo.Metadata().GitDir
	if gitDir == "" {
		return nil
	}
	rel, err := filepath.Rel(gitDir, parent)
	if err != nil {
		return pathErr(CodeParentOverlapsRepository, "workspace", "parent", parent, "compare parent with repository", err)
	}
	if rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
		return pathErr(CodeParentOverlapsRepository, "workspace", "parent", parent, "parent is within source Git store", nil)
	}
	return nil
}

func (m *Materializer) createWorkspace() (*Workspace, error) {
	for i := 0; i < workspaceCreateAttempts; i++ {
		name, err := randomWorkspaceName()
		if err != nil {
			return nil, err
		}
		workspacePath := filepath.Join(m.parentDir, name)
		err = os.Mkdir(workspacePath, 0o700)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, pathErr(CodeWorkspaceCreateFailed, "workspace", "mkdir", workspacePath, "create workspace", err)
		}
		root, err := os.OpenRoot(workspacePath)
		if err != nil {
			_ = m.removeAll(workspacePath)
			return nil, pathErr(CodeWorkspaceOpenFailed, "workspace", "open-root", workspacePath, "open workspace root", err)
		}
		if _, err := root.Stat("."); err != nil {
			_ = root.Close()
			_ = m.removeAll(workspacePath)
			return nil, pathErr(CodeWorkspaceOpenFailed, "workspace", "stat-root", workspacePath, "verify workspace root", err)
		}
		return &Workspace{path: workspacePath, root: root, removeAll: m.removeAll}, nil
	}
	return nil, errCode(CodeWorkspaceCreateFailed, "workspace", "mkdir", "workspace name collisions exceeded bounded retry count", nil)
}

func randomWorkspaceName() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errCode(CodeWorkspaceCreateFailed, "workspace", "random", "generate workspace name", err)
	}
	return "glassroot-mat-" + hex.EncodeToString(b[:]), nil
}

func cleanupAfterFailure(w *Workspace, primary error) error {
	if w == nil {
		return primary
	}
	if cleanupErr := w.Close(); cleanupErr != nil {
		return &Error{Code: CodeCleanupFailed, Stage: "cleanup", Op: "remove", Path: w.path, Msg: "partial workspace cleanup failed", Err: primary}
	}
	return primary
}
