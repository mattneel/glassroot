package artifactcollect

import (
	"context"
	"os"
	"sort"
)

type entryKind string

const (
	entryDirectory       entryKind = "directory"
	entryRegular         entryKind = "regular-file"
	entrySymlink         entryKind = "symlink"
	entryFIFO            entryKind = "fifo"
	entrySocket          entryKind = "socket"
	entryBlockDevice     entryKind = "block-device"
	entryCharacterDevice entryKind = "character-device"
	entryOtherSpecial    entryKind = "other-special"
)

type entry struct {
	rel      string
	kind     entryKind
	identity fileIdentity
	size     int64
	mode     os.FileMode
}

type inventory struct {
	entries []entry
	summary InventorySummary
	byRel   map[string]entry
}

func (w *BoundWorkspace) inventory(ctx context.Context) (inventory, error) {
	inv := inventory{entries: []entry{}, byRel: map[string]entry{}}
	rootEnt := entry{rel: "", kind: entryDirectory, identity: w.identity, mode: w.identity.Mode}
	if err := w.walkDir(ctx, "", rootEnt, &inv); err != nil {
		return inventory{}, err
	}
	sort.Slice(inv.entries, func(i, j int) bool { return inv.entries[i].rel < inv.entries[j].rel })
	return inv, nil
}

func (w *BoundWorkspace) walkDir(ctx context.Context, rel string, expected entry, inv *inventory) error {
	if err := checkContext(ctx, "inventory"); err != nil {
		return err
	}
	readName := "."
	if rel != "" {
		readName = rel
	}
	dir, err := w.root.Open(readName)
	if err != nil {
		return errCode(CodeWorkspaceChanged, "inventory", logicalFromRel("", rel), "open workspace directory", err)
	}
	opened, statErr := dir.Stat()
	if statErr != nil {
		_ = dir.Close()
		return errCode(CodeWorkspaceChanged, "inventory", logicalFromRel("", rel), "stat workspace directory", statErr)
	}
	openedID, idErr := identityFromInfo(opened)
	if idErr != nil {
		_ = dir.Close()
		return idErr
	}
	if !sameFileIdentity(expected.identity, openedID) || classifyEntry(opened.Mode()) != entryDirectory {
		_ = dir.Close()
		return errCode(CodeWorkspaceChanged, "inventory", logicalFromRel("", rel), "directory identity changed before read", nil)
	}
	infos, err := dir.ReadDir(-1)
	closeErr := dir.Close()
	if err != nil {
		return errCode(CodeWorkspaceChanged, "inventory", logicalFromRel("", rel), "read workspace directory", err)
	}
	if closeErr != nil {
		return errCode(CodeCloseFailed, "inventory", logicalFromRel("", rel), "close workspace directory", closeErr)
	}
	for _, dirent := range infos {
		name := dirent.Name()
		if err := validateComponent(name, w.limits); err != nil {
			return err
		}
		child := appendRel(rel, name)
		if err := validateInventoryRelativePath(child, w.limits); err != nil {
			return err
		}
		if _, exists := inv.byRel[child]; exists {
			return errCode(CodeTreePathConflict, "inventory", logicalFromRel("", child), "duplicate inventory path", nil)
		}
		info, err := w.root.Lstat(child)
		if err != nil {
			return errCode(CodeWorkspaceChanged, "inventory", logicalFromRel("", child), "lstat inventory entry", err)
		}
		id, err := identityFromInfo(info)
		if err != nil {
			return err
		}
		if id.Dev != w.identity.Dev {
			return errCode(CodeFilesystemBoundary, "inventory", logicalFromRel("", child), "entry crosses filesystem boundary", nil)
		}
		kind := classifyEntry(info.Mode())
		ent := entry{rel: child, kind: kind, identity: id, size: info.Size(), mode: info.Mode()}
		if err := inv.add(ent, w.limits); err != nil {
			return err
		}
		if kind == entryDirectory {
			if err := w.verifyDirectoryOpen(child, ent); err != nil {
				return err
			}
			if err := w.walkDir(ctx, child, ent, inv); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *BoundWorkspace) verifyDirectoryOpen(rel string, ent entry) error {
	childRoot, err := w.root.OpenRoot(rel)
	if err != nil {
		return errCode(CodeWorkspaceChanged, "inventory", logicalFromRel("", rel), "open child directory", err)
	}
	defer func() { _ = childRoot.Close() }()
	opened, err := childRoot.Stat(".")
	if err != nil {
		return errCode(CodeWorkspaceChanged, "inventory", logicalFromRel("", rel), "stat child directory", err)
	}
	openedID, err := identityFromInfo(opened)
	if err != nil {
		return err
	}
	if !sameFileIdentity(ent.identity, openedID) || openedID.Dev != w.identity.Dev {
		return errCode(CodeWorkspaceChanged, "inventory", logicalFromRel("", rel), "child directory identity changed while opening", nil)
	}
	again, err := w.root.Lstat(rel)
	if err != nil {
		return errCode(CodeWorkspaceChanged, "inventory", logicalFromRel("", rel), "lstat child directory after open", err)
	}
	againID, err := identityFromInfo(again)
	if err != nil {
		return err
	}
	if !sameStableIdentity(ent.identity, againID) {
		return errCode(CodeWorkspaceChanged, "inventory", logicalFromRel("", rel), "child directory changed while opening", nil)
	}
	return nil
}

func (inv *inventory) add(ent entry, limits Limits) error {
	if len(inv.entries)+1 > limits.MaxInventoryEntries {
		return errCode(CodeInventoryLimit, "inventory", logicalFromRel("", ent.rel), "inventory entry limit exceeded", nil)
	}
	metaBytes := 0
	for _, e := range inv.entries {
		metaBytes += len(e.rel) + 128
	}
	metaBytes += len(ent.rel) + 128
	if int64(metaBytes) > limits.MaxInventoryMetadataBytes {
		return errCode(CodeInventoryLimit, "inventory", logicalFromRel("", ent.rel), "inventory metadata limit exceeded", nil)
	}
	inv.entries = append(inv.entries, ent)
	inv.byRel[ent.rel] = ent
	inv.summary.EntryCount++
	switch ent.kind {
	case entryDirectory:
		inv.summary.DirectoryCount++
		if inv.summary.DirectoryCount > limits.MaxDirectories {
			return errCode(CodeInventoryLimit, "inventory", logicalFromRel("", ent.rel), "directory count limit exceeded", nil)
		}
	case entryRegular:
		inv.summary.RegularFileCount++
		if inv.summary.RegularFileCount > limits.MaxRegularFiles {
			return errCode(CodeInventoryLimit, "inventory", logicalFromRel("", ent.rel), "regular file count limit exceeded", nil)
		}
	case entrySymlink:
		inv.summary.SymlinkCount++
		if inv.summary.SymlinkCount > limits.MaxSymlinks {
			return errCode(CodeInventoryLimit, "inventory", logicalFromRel("", ent.rel), "symlink count limit exceeded", nil)
		}
	default:
		inv.summary.SpecialEntryCount++
		if inv.summary.SpecialEntryCount > limits.MaxSpecialEntries {
			return errCode(CodeInventoryLimit, "inventory", logicalFromRel("", ent.rel), "special entry count limit exceeded", nil)
		}
	}
	return nil
}

func classifyEntry(mode os.FileMode) entryKind {
	if mode.IsDir() {
		return entryDirectory
	}
	if mode.Type() == 0 {
		return entryRegular
	}
	if mode&os.ModeSymlink != 0 {
		return entrySymlink
	}
	if mode&os.ModeNamedPipe != 0 {
		return entryFIFO
	}
	if mode&os.ModeSocket != 0 {
		return entrySocket
	}
	if mode&os.ModeDevice != 0 {
		if mode&os.ModeCharDevice != 0 {
			return entryCharacterDevice
		}
		return entryBlockDevice
	}
	return entryOtherSpecial
}

func reconcileInventories(a, b inventory) error {
	if len(a.entries) != len(b.entries) {
		return errCode(CodeWorkspaceChanged, "reconcile", "", "workspace inventory entry count changed", nil)
	}
	for i := range a.entries {
		ae, be := a.entries[i], b.entries[i]
		if ae.rel != be.rel || ae.kind != be.kind || !sameStableIdentity(ae.identity, be.identity) {
			return errCode(CodeWorkspaceChanged, "reconcile", logicalFromRel("", ae.rel), "workspace inventory changed", nil)
		}
	}
	return nil
}

func logicalFromRel(workdir, rel string) string {
	if workdir == "" {
		workdir = "/workspace"
	}
	if rel == "" {
		return workdir
	}
	return workdir + "/" + rel
}
