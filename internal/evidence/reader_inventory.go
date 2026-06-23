package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/model"
)

type inventoryEntry struct {
	path string
	info os.FileInfo
	id   statIdentity
	dir  bool
}

type bundleInventory struct {
	rootID statIdentity
	dirs   map[string]inventoryEntry
	files  map[string]inventoryEntry
	all    []inventoryEntry
	bytes  int64
}

func collectInventory(ctx context.Context, root *os.Root, rootInfo os.FileInfo, limits ReaderLimits) (bundleInventory, error) {
	if err := ctx.Err(); err != nil {
		return bundleInventory{}, readerContextErr(err)
	}
	rootID, err := fileInfoIdentity(rootInfo)
	if err != nil {
		return bundleInventory{}, err
	}
	if rootInfo.Mode().Perm() != 0o700 {
		return bundleInventory{}, pathErr(CodeUnexpectedEntryMode, "inventory", "root-mode", ".", "bundle root mode must be 0700", nil)
	}
	inv := bundleInventory{rootID: rootID, dirs: map[string]inventoryEntry{}, files: map[string]inventoryEntry{}}
	if err := walkInventory(ctx, root, ".", 0, rootID, limits, &inv); err != nil {
		return bundleInventory{}, err
	}
	sort.Slice(inv.all, func(i, j int) bool { return inv.all[i].path < inv.all[j].path })
	return inv, nil
}

func walkInventory(ctx context.Context, root *os.Root, rel string, depth int, rootID statIdentity, limits ReaderLimits, inv *bundleInventory) error {
	if err := ctx.Err(); err != nil {
		return readerContextErr(err)
	}
	if depth > limits.MaxPhysicalDepth {
		return pathErr(CodeInventoryLimit, "inventory", "depth", rel, "physical path depth exceeded", nil)
	}
	f, err := root.Open(rel)
	if err != nil {
		return pathErr(CodeBundleChanged, "inventory", "open-dir", rel, "open directory", err)
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return pathErr(CodeBundleChanged, "inventory", "read-dir", rel, "read directory", err)
	}
	for _, de := range entries {
		if err := ctx.Err(); err != nil {
			return readerContextErr(err)
		}
		name := de.Name()
		if err := validateEntryName(name, limits); err != nil {
			return err
		}
		child := name
		if rel != "." {
			child = rel + "/" + name
		}
		if err := ValidateEvidenceEntryPath(child); err != nil {
			return err
		}
		info, err := root.Lstat(child)
		if err != nil {
			return pathErr(CodeBundleChanged, "inventory", "lstat", child, "stat entry", err)
		}
		id, err := fileInfoIdentity(info)
		if err != nil {
			return err
		}
		if id.dev != rootID.dev {
			return pathErr(CodeFilesystemBoundary, "inventory", "device", child, "entry is on a different device", nil)
		}
		mode := info.Mode()
		switch {
		case mode&os.ModeSymlink != 0:
			return pathErr(CodeSymlinkEntry, "inventory", "lstat", child, "symlink entries are not allowed", nil)
		case mode.IsDir():
			if mode.Perm() != 0o700 {
				return pathErr(CodeUnexpectedEntryMode, "inventory", "dir-mode", child, "directory mode must be 0700", nil)
			}
			opened, err := root.Open(child)
			if err != nil {
				return pathErr(CodeBundleChanged, "inventory", "open-dir", child, "open child directory", err)
			}
			openedInfo, statErr := opened.Stat()
			_ = opened.Close()
			if statErr != nil {
				return pathErr(CodeBundleChanged, "inventory", "stat-dir", child, "stat opened child directory", statErr)
			}
			openedID, err := fileInfoIdentity(openedInfo)
			if err != nil {
				return err
			}
			if !sameIdentity(id, openedID) {
				return pathErr(CodeBundleChanged, "inventory", "dir-identity", child, "directory identity changed during open", nil)
			}
			again, err := root.Lstat(child)
			if err != nil {
				return pathErr(CodeBundleChanged, "inventory", "lstat-dir", child, "restat child directory", err)
			}
			againID, err := fileInfoIdentity(again)
			if err != nil {
				return err
			}
			if !sameIdentity(id, againID) {
				return pathErr(CodeBundleChanged, "inventory", "dir-restat", child, "directory identity changed", nil)
			}
			if len(inv.dirs) >= limits.MaxDirectories || len(inv.all) >= limits.MaxInventoryEntries {
				return pathErr(CodeInventoryLimit, "inventory", "count", child, "inventory entry limit exceeded", nil)
			}
			ent := inventoryEntry{path: child, info: info, id: id, dir: true}
			inv.dirs[child] = ent
			inv.all = append(inv.all, ent)
			if err := walkInventory(ctx, root, child, depth+1, rootID, limits, inv); err != nil {
				return err
			}
		case mode.IsRegular():
			if mode.Perm() != 0o600 || mode&(^fs.FileMode(0o777)) != 0 {
				return pathErr(CodeUnexpectedEntryMode, "inventory", "file-mode", child, "payload file mode must be 0600", nil)
			}
			if id.nlink != 1 {
				return pathErr(CodeHardlinkEntry, "inventory", "nlink", child, "regular payload hard links are not allowed", nil)
			}
			if info.Size() < 0 || inv.bytes > limits.MaxBundleBytes-info.Size() {
				return pathErr(CodeTotalBytesLimit, "inventory", "bytes", child, "bundle byte limit exceeded", nil)
			}
			if len(inv.files) >= limits.MaxFiles || len(inv.all) >= limits.MaxInventoryEntries {
				return pathErr(CodeInventoryLimit, "inventory", "count", child, "file count limit exceeded", nil)
			}
			ent := inventoryEntry{path: child, info: info, id: id}
			inv.files[child] = ent
			inv.all = append(inv.all, ent)
			inv.bytes += info.Size()
		default:
			return pathErr(CodeSpecialEntry, "inventory", "type", child, "special entries are not allowed", nil)
		}
	}
	return nil
}

func validateEntryName(name string, limits ReaderLimits) error {
	if name == "" || len(name) > limits.MaxPhysicalPathComponentBytes || strings.ContainsRune(name, 0) || containsControl(name) || strings.Contains(name, "/") || strings.Contains(name, "\\") || name == "." || name == ".." || !validUTF8(name) {
		return pathErr(CodeInvalidEntryName, "inventory", "name", sanitize(name, 160), "invalid physical entry name", nil)
	}
	return nil
}

func validatePhysicalPathForReader(p string, limits ReaderLimits) error {
	if p == "manifest.json" {
		return nil
	}
	if !validRelativePath(p, limits.MaxPhysicalPathBytes, false) {
		return pathErr(CodeInvalidEntryPath, "manifest", "path", p, "invalid manifest entry path", nil)
	}
	if strings.HasPrefix(p, "./") || path.Clean(p) != p {
		return pathErr(CodeInvalidEntryPath, "manifest", "path", p, "non-clean manifest entry path", nil)
	}
	parts := strings.Split(p, "/")
	if len(parts) > limits.MaxPhysicalDepth {
		return pathErr(CodeInvalidEntryPath, "manifest", "depth", p, "manifest entry depth exceeded", nil)
	}
	for _, part := range parts {
		if len(part) > limits.MaxPhysicalPathComponentBytes {
			return pathErr(CodeInvalidEntryPath, "manifest", "component", p, "manifest entry component too long", nil)
		}
	}
	return nil
}

func stableReadAll(ctx context.Context, root *os.Root, rel string, expectedSize int64, max int64) ([]byte, model.Digest, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", readerContextErr(err)
	}
	f, id, size, err := openStableFile(root, rel, max)
	if err != nil {
		return nil, "", err
	}
	if expectedSize >= 0 && size != expectedSize {
		_ = f.Close()
		return nil, "", pathErr(CodePayloadSizeMismatch, "payload", "size", rel, "payload size mismatch", nil)
	}
	data, err := readAllContext(ctx, rel, f, size, max)
	if err != nil {
		_ = f.Close()
		return nil, "", err
	}
	if int64(len(data)) != size {
		_ = f.Close()
		return nil, "", pathErr(CodePayloadSizeMismatch, "payload", "read-size", rel, "payload read size mismatch", nil)
	}
	if int64(len(data)) > max {
		_ = f.Close()
		return nil, "", pathErr(CodePayloadSizeMismatch, "payload", "limit", rel, "payload exceeds role limit", nil)
	}
	if err := checkStableFile(root, rel, f, id); err != nil {
		_ = f.Close()
		return nil, "", err
	}
	if err := f.Close(); err != nil {
		return nil, "", pathErr(CodeCloseFailed, "payload", "close", rel, "close payload", err)
	}
	return append([]byte(nil), data...), digestBytes(data), nil
}

func openStableFile(root *os.Root, rel string, max int64) (*os.File, statIdentity, int64, error) {
	if err := ValidateEvidenceEntryPath(rel); err != nil {
		return nil, statIdentity{}, 0, err
	}
	info, err := root.Lstat(rel)
	if err != nil {
		return nil, statIdentity{}, 0, pathErr(CodeMissingEntry, "payload", "lstat", rel, "payload missing", err)
	}
	if !info.Mode().IsRegular() {
		return nil, statIdentity{}, 0, pathErr(CodeSpecialEntry, "payload", "type", rel, "payload is not regular file", nil)
	}
	id, err := fileInfoIdentity(info)
	if err != nil {
		return nil, statIdentity{}, 0, err
	}
	if id.nlink != 1 {
		return nil, statIdentity{}, 0, pathErr(CodeHardlinkEntry, "payload", "nlink", rel, "payload hard link rejected", nil)
	}
	if info.Mode().Perm() != 0o600 {
		return nil, statIdentity{}, 0, pathErr(CodeUnexpectedEntryMode, "payload", "mode", rel, "payload mode must be 0600", nil)
	}
	if info.Size() < 0 || info.Size() > max {
		return nil, statIdentity{}, 0, pathErr(CodePayloadSizeMismatch, "payload", "limit", rel, "payload exceeds limit", nil)
	}
	f, err := root.Open(rel)
	if err != nil {
		return nil, statIdentity{}, 0, pathErr(CodeBundleChanged, "payload", "open", rel, "open payload", err)
	}
	opened, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, statIdentity{}, 0, pathErr(CodeBundleChanged, "payload", "stat-open", rel, "stat opened payload", err)
	}
	openedID, err := fileInfoIdentity(opened)
	if err != nil {
		_ = f.Close()
		return nil, statIdentity{}, 0, err
	}
	if !sameIdentity(id, openedID) {
		_ = f.Close()
		return nil, statIdentity{}, 0, pathErr(CodeBundleChanged, "payload", "identity", rel, "payload identity changed during open", nil)
	}
	again, err := root.Lstat(rel)
	if err != nil {
		_ = f.Close()
		return nil, statIdentity{}, 0, pathErr(CodeBundleChanged, "payload", "lstat-open", rel, "restat payload", err)
	}
	againID, err := fileInfoIdentity(again)
	if err != nil {
		_ = f.Close()
		return nil, statIdentity{}, 0, err
	}
	if !sameIdentity(id, againID) {
		_ = f.Close()
		return nil, statIdentity{}, 0, pathErr(CodeBundleChanged, "payload", "restat", rel, "payload path changed", nil)
	}
	return f, id, info.Size(), nil
}

func checkStableFile(root *os.Root, rel string, f *os.File, before statIdentity) error {
	afterInfo, err := f.Stat()
	if err != nil {
		return pathErr(CodeBundleChanged, "payload", "post-stat", rel, "post-read stat failed", err)
	}
	after, err := fileInfoIdentity(afterInfo)
	if err != nil {
		return err
	}
	if !sameStableFile(before, after) {
		return pathErr(CodeBundleChanged, "payload", "post-stat", rel, "payload changed during read", nil)
	}
	again, err := root.Lstat(rel)
	if err != nil {
		return pathErr(CodeBundleChanged, "payload", "post-lstat", rel, "post-read lstat failed", err)
	}
	againID, err := fileInfoIdentity(again)
	if err != nil {
		return err
	}
	if !sameStableFile(before, againID) {
		return pathErr(CodeBundleChanged, "payload", "post-lstat", rel, "payload path changed during read", nil)
	}
	return nil
}

func readAllContext(ctx context.Context, rel string, r io.Reader, size int64, max int64) ([]byte, error) {
	capHint := size
	if capHint < 0 || capHint > max {
		capHint = max
	}
	if capHint > 1<<20 {
		capHint = 1 << 20
	}
	data := make([]byte, 0, int(capHint))
	buf := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return nil, readerContextErr(err)
		}
		n, err := r.Read(buf)
		if n > 0 {
			if total > max-int64(n) {
				return nil, pathErr(CodePayloadSizeMismatch, "payload", "limit", rel, "payload exceeds role limit", nil)
			}
			total += int64(n)
			data = append(data, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, pathErr(CodeBundleChanged, "payload", "read", rel, "read payload", err)
		}
	}
	return data, nil
}

func verifyStreamPayload(ctx context.Context, root *os.Root, rel string, expectedSize int64, expectedDigest model.Digest, max int64, dst io.Writer) (CopyResult, error) {
	if err := ctx.Err(); err != nil {
		return CopyResult{}, readerContextErr(err)
	}
	f, id, size, err := openStableFile(root, rel, max)
	if err != nil {
		return CopyResult{}, err
	}
	if size != expectedSize {
		_ = f.Close()
		return CopyResult{}, pathErr(CodePayloadSizeMismatch, "payload", "size", rel, "payload size mismatch", nil)
	}
	h := sha256.New()
	buf := make([]byte, 32*1024)
	var n int64
	for {
		if err := ctx.Err(); err != nil {
			_ = f.Close()
			return CopyResult{}, readerContextErr(err)
		}
		nr, er := f.Read(buf)
		if nr > 0 {
			if n > max-int64(nr) {
				_ = f.Close()
				return CopyResult{}, pathErr(CodePayloadSizeMismatch, "payload", "limit", rel, "payload exceeds role limit", nil)
			}
			chunk := buf[:nr]
			if _, err := h.Write(chunk); err != nil {
				_ = f.Close()
				return CopyResult{}, errCode(CodeCallbackFailed, "payload", "hash", "hash payload", err)
			}
			written, err := dst.Write(chunk)
			if err != nil {
				_ = f.Close()
				return CopyResult{}, errCode(CodeCallbackFailed, "payload", "copy", "copy destination failed", err)
			}
			if written != nr {
				_ = f.Close()
				return CopyResult{}, errCode(CodeCallbackFailed, "payload", "copy", "copy destination short write", io.ErrShortWrite)
			}
			n += int64(nr)
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			_ = f.Close()
			return CopyResult{}, pathErr(CodeBundleChanged, "payload", "read", rel, "read payload", er)
		}
	}
	if n != expectedSize {
		_ = f.Close()
		return CopyResult{}, pathErr(CodePayloadSizeMismatch, "payload", "copy-size", rel, "payload copy size mismatch", nil)
	}
	got := model.Digest("sha256:" + hex.EncodeToString(h.Sum(nil)))
	if got != expectedDigest {
		_ = f.Close()
		return CopyResult{}, pathErr(CodePayloadDigestMismatch, "payload", "digest", rel, "payload digest mismatch", nil)
	}
	if err := checkStableFile(root, rel, f, id); err != nil {
		_ = f.Close()
		return CopyResult{}, err
	}
	if err := f.Close(); err != nil {
		return CopyResult{}, pathErr(CodeCloseFailed, "payload", "close", rel, "close payload", err)
	}
	return CopyResult{Bytes: n, Digest: got, CaptureState: CaptureStateCaptured}, nil
}

func inventoryPathSet(files map[string]inventoryEntry) map[string]struct{} {
	out := map[string]struct{}{}
	for p := range files {
		out[p] = struct{}{}
	}
	return out
}

func expectedDirsForFiles(files map[string]struct{}) map[string]struct{} {
	dirs := map[string]struct{}{}
	for p := range files {
		parts := strings.Split(p, "/")
		cur := ""
		for i := 0; i < len(parts)-1; i++ {
			if cur == "" {
				cur = parts[i]
			} else {
				cur += "/" + parts[i]
			}
			dirs[cur] = struct{}{}
		}
	}
	return dirs
}

func comparePathSets(actual, expected map[string]struct{}) (string, ErrorCode) {
	for p := range actual {
		if _, ok := expected[p]; !ok {
			return p, CodeUndeclaredEntry
		}
	}
	for p := range expected {
		if _, ok := actual[p]; !ok {
			return p, CodeMissingEntry
		}
	}
	return "", ""
}

func compareDirSets(actual map[string]inventoryEntry, expected map[string]struct{}) (string, ErrorCode) {
	for p := range actual {
		if _, ok := expected[p]; !ok {
			return p, CodeUnexpectedDirectory
		}
	}
	for p := range expected {
		if _, ok := actual[p]; !ok {
			return p, CodeMissingEntry
		}
	}
	return "", ""
}

func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist)
}

func validUTF8(s string) bool { return utf8.ValidString(s) }
