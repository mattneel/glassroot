package gitstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriterFailed }

var errWriterFailed = errors.New("writer failed")

func TestCopyBlobStreamsExactRawContent(t *testing.T) {
	ctx := context.Background()
	fixture := newGitFixture(t, "sha1")
	want := []byte("line1\r\n\x00line2\n")
	commit := fixture.commitFiles(t, map[string]fileSpec{"raw.bin": {data: want}})
	repo := openFixtureBare(t, ctx, fixture)
	defer repo.Close()
	rev, err := repo.ResolveCommit(ctx, ObjectIDSelector(commit))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := repo.treeEntryForPath(ctx, rev, "raw.bin")
	if err != nil {
		t.Fatal(err)
	}
	var dst bytes.Buffer
	meta, err := repo.CopyBlob(ctx, entry.ObjectID, entry.Size, 1024, &dst)
	if err != nil {
		t.Fatalf("CopyBlob: %v", err)
	}
	if !bytes.Equal(dst.Bytes(), want) {
		t.Fatalf("CopyBlob wrote %q, want %q", dst.Bytes(), want)
	}
	if meta.ObjectID != entry.ObjectID || meta.SizeBytes != int64(len(want)) || meta.ContentDigest == "" {
		t.Fatalf("metadata = %#v", meta)
	}
}

func TestCopyBlobPropagatesWriterErrors(t *testing.T) {
	ctx := context.Background()
	fixture := newGitFixture(t, "sha1")
	commit := fixture.commitFiles(t, map[string]fileSpec{"raw.bin": {data: []byte("content")}})
	repo := openFixtureBare(t, ctx, fixture)
	defer repo.Close()
	rev, err := repo.ResolveCommit(ctx, ObjectIDSelector(commit))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := repo.treeEntryForPath(ctx, rev, "raw.bin")
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CopyBlob(ctx, entry.ObjectID, entry.Size, 1024, failingWriter{})
	if !errors.Is(err, errWriterFailed) {
		t.Fatalf("CopyBlob writer err = %v, want %v", err, errWriterFailed)
	}
	_, err = repo.CopyBlob(ctx, entry.ObjectID, entry.Size, 3, io.Discard)
	if !errors.Is(err, ErrBlobTooLarge) {
		t.Fatalf("CopyBlob byte-limit err = %v, want blob-too-large", err)
	}
}
