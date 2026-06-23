package gitstore

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"strconv"
)

type File struct {
	Path          string
	Kind          EntryKind
	ObjectID      string
	Data          []byte
	Executable    bool
	SizeBytes     int64
	ContentDigest string
}

type catFileHeader struct {
	ObjectID    string
	Type        string
	Size        int64
	HeaderBytes int
}

func (r *Repository) ReadPath(ctx context.Context, revision ResolvedRevision, requestedPath string, maxBytes int64) (File, error) {
	if maxBytes < 0 {
		return File{}, gitErr(CodeBlobTooLarge, "blob", "read", "negative byte limit", nil)
	}
	entry, err := r.treeEntryForPath(ctx, revision, requestedPath)
	if err != nil {
		return File{}, err
	}
	file := File{Path: entry.Path, Kind: entry.Kind, ObjectID: entry.ObjectID, Executable: entry.Executable, SizeBytes: entry.Size}
	switch entry.Kind {
	case EntryDirectory, EntryGitlink:
		return file, nil
	case EntryRegularFile, EntryExecutableFile, EntrySymlink:
		if entry.SizeKnown && (entry.Size > maxBytes || entry.Size > MaxBlobBytes) {
			return File{}, gitErr(CodeBlobTooLarge, "blob", "read", "blob exceeds byte limit", nil)
		}
		data, err := r.readBlob(ctx, entry.ObjectID, entry.Size, entry.SizeKnown, maxBytes)
		if err != nil {
			return File{}, err
		}
		file.Data = append([]byte(nil), data...)
		file.SizeBytes = int64(len(data))
		file.ContentDigest = contentDigest(data)
		return file, nil
	default:
		return File{}, gitErr(CodeUnsupportedEntryMode, "blob", "read", "unsupported entry kind", nil)
	}
}

func (r *Repository) readBlob(ctx context.Context, oid string, expectedSize int64, sizeKnown bool, maxBytes int64) ([]byte, error) {
	if _, err := validateObjectID(oid, r.objectFormat, false); err != nil {
		return nil, err
	}
	limit := maxBytes
	if limit > MaxBlobBytes || limit == 0 {
		limit = MaxBlobBytes
	}
	if sizeKnown && expectedSize > limit {
		return nil, gitErr(CodeBlobTooLarge, "blob", "cat-file", "blob exceeds byte limit", nil)
	}
	stdoutLimit := limit + int64(r.objectFormat.ObjectIDLength()) + 128
	out, err := r.cmd.runRepoGit(ctx, commandSpec{op: "cat-file", args: []string{"cat-file", "--batch"}, stdin: []byte(oid + "\n"), stdoutLimit: stdoutLimit, stderrLimit: MaxGitStderrBytes, timeout: DefaultGitCommandTimeout})
	if err != nil {
		return nil, err
	}
	header, err := parseCatFileHeader(out.Stdout, r.objectFormat)
	if err != nil {
		return nil, err
	}
	if header.ObjectID != oid {
		return nil, gitErr(CodeBlobObjectIDMismatch, "blob", "cat-file", "returned object id does not match request", nil)
	}
	if header.Type != "blob" {
		return nil, gitErr(CodeBlobTypeMismatch, "blob", "cat-file", "cat-file did not return a blob", nil)
	}
	if header.Size > limit || header.Size > MaxBlobBytes {
		return nil, gitErr(CodeBlobTooLarge, "blob", "cat-file", "blob exceeds byte limit", nil)
	}
	if sizeKnown && header.Size != expectedSize {
		return nil, gitErr(CodeBlobSizeMismatch, "blob", "cat-file", "cat-file size differs from tree entry", nil)
	}
	start := header.HeaderBytes
	end := start + int(header.Size)
	if int64(int(header.Size)) != header.Size || len(out.Stdout) < end+1 {
		return nil, gitErr(CodeMalformedGitOutput, "blob", "cat-file", "truncated cat-file body", nil)
	}
	body := out.Stdout[start:end]
	if out.Stdout[end] != '\n' || len(out.Stdout) != end+1 {
		return nil, gitErr(CodeMalformedGitOutput, "blob", "cat-file", "unexpected cat-file framing", nil)
	}
	computed, err := gitObjectID(r.objectFormat, "blob", body)
	if err != nil {
		return nil, err
	}
	if computed != oid {
		return nil, gitErr(CodeBlobObjectIDMismatch, "blob", "hash", "computed Git object identity did not match", nil)
	}
	return append([]byte(nil), body...), nil
}

func parseCatFileHeader(data []byte, format ObjectFormat) (catFileHeader, error) {
	newline := bytes.IndexByte(data, '\n')
	if newline < 0 {
		return catFileHeader{}, gitErr(CodeMalformedGitOutput, "blob", "cat-file", "missing cat-file header", nil)
	}
	fields := bytes.Fields(data[:newline])
	if len(fields) != 3 {
		return catFileHeader{}, gitErr(CodeMalformedGitOutput, "blob", "cat-file", "malformed cat-file header", nil)
	}
	oid, err := validateObjectID(string(fields[0]), format, false)
	if err != nil {
		return catFileHeader{}, err
	}
	typ := string(fields[1])
	if typ != "blob" {
		return catFileHeader{}, gitErr(CodeBlobTypeMismatch, "blob", "cat-file", fmt.Sprintf("object type %s is not blob", typ), nil)
	}
	size, err := strconv.ParseInt(string(fields[2]), 10, 64)
	if err != nil || size < 0 {
		return catFileHeader{}, gitErr(CodeMalformedGitOutput, "blob", "cat-file", "malformed blob size", nil)
	}
	return catFileHeader{ObjectID: oid, Type: typ, Size: size, HeaderBytes: newline + 1}, nil
}

func fsNotExist(path string) error {
	return &fs.PathError{Op: "read", Path: sanitize(path, 160), Err: fs.ErrNotExist}
}
