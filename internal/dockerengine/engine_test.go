package dockerengine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidateSocketPathRejectsNonLocalUnixSocketInputs(t *testing.T) {
	cases := []string{"", "relative.sock", "tcp://127.0.0.1:2375", "http://docker", "https://docker", "ssh://docker", "npipe://./pipe/docker", "/tmp/../tmp/docker.sock", "/tmp/docker\x00.sock", "/tmp/docker\n.sock"}
	for _, in := range cases {
		if err := ValidateSocketPath(in); err == nil {
			t.Fatalf("ValidateSocketPath(%q) succeeded, want failure", in)
		}
	}
}

func TestValidateSocketPathRejectsSymlinkAndNonSocket(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Unix socket file mode assertions are Linux-only")
	}
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSocketPath(regular); err == nil {
		t.Fatalf("regular file accepted as socket")
	}
	link := filepath.Join(dir, "link.sock")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSocketPath(link); err == nil {
		t.Fatalf("symlink accepted as socket")
	}
}

func TestValidateImmutableLocalImage(t *testing.T) {
	valid := "registry.example.invalid/glassroot/dev@sha256:" + strings.Repeat("a", 64)
	if got, err := ValidateImmutableLocalImage(valid); err != nil || got != valid {
		t.Fatalf("valid image rejected: got=%q err=%v", got, err)
	}
	for _, in := range []string{"alpine:latest", "alpine", "repo@sha256:" + strings.Repeat("A", 64), "repo@sha256:" + strings.Repeat("a", 63), "repo@sha256:" + strings.Repeat("a", 64) + "\n"} {
		if _, err := ValidateImmutableLocalImage(in); err == nil {
			t.Fatalf("invalid image accepted: %q", in)
		}
	}
}

func TestDecodeDockerAttachFramesDemultiplexesAndBounds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	frame := func(stream byte, payload string) []byte {
		b := []byte{stream, 0, 0, 0, 0, 0, 0, byte(len(payload))}
		return append(b, []byte(payload)...)
	}
	input := append(frame(1, "out"), frame(2, "err")...)
	counts, err := DecodeDockerAttachFrames(context.Background(), bytes.NewReader(input), 10, func(_ context.Context, stream LogStream, data []byte) error {
		switch stream {
		case LogStreamStdout:
			_, _ = stdout.Write(data)
		case LogStreamStderr:
			_, _ = stderr.Write(data)
		default:
			t.Fatalf("unexpected stream %q", stream)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DecodeDockerAttachFrames failed: %v", err)
	}
	if stdout.String() != "out" || stderr.String() != "err" || counts.StdoutAccepted != 3 || counts.StderrAccepted != 3 {
		t.Fatalf("bad decoded output stdout=%q stderr=%q counts=%+v", stdout.String(), stderr.String(), counts)
	}
}

func TestDecodeDockerAttachFramesRejectsMalformedAndSinkFailure(t *testing.T) {
	if _, err := DecodeDockerAttachFrames(context.Background(), bytes.NewReader([]byte{1, 0, 0}), 10, nil); err == nil {
		t.Fatalf("truncated frame accepted")
	}
	boom := errors.New("sink boom")
	frame := []byte{1, 0, 0, 0, 0, 0, 0, 1, 'x'}
	_, err := DecodeDockerAttachFrames(context.Background(), bytes.NewReader(frame), 10, func(context.Context, LogStream, []byte) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("sink error not preserved: %v", err)
	}
}

func TestDecodeDockerAttachFramesContinuesAfterLimit(t *testing.T) {
	frame := []byte{1, 0, 0, 0, 0, 0, 0, 3, 'a', 'b', 'c'}
	var got bytes.Buffer
	counts, err := DecodeDockerAttachFrames(context.Background(), bytes.NewReader(frame), 2, func(_ context.Context, _ LogStream, data []byte) error {
		_, _ = got.Write(data)
		return nil
	})
	if err != nil {
		t.Fatalf("limit should be recorded, not a framing error: %v", err)
	}
	if got.String() != "ab" || !counts.StdoutTruncated || counts.StdoutAccepted != 2 || counts.StdoutObservedAtLeast != 3 {
		t.Fatalf("bad truncation behavior got=%q counts=%+v", got.String(), counts)
	}
}

func TestOpenRejectsUnsupportedPlatformAndInvalidConfig(t *testing.T) {
	_, err := Open(context.Background(), Config{})
	if err == nil {
		t.Fatalf("Open with zero config succeeded")
	}
	if runtime.GOOS != "linux" {
		var derr *Error
		if !errors.As(err, &derr) || derr.Code != CodeUnsupportedPlatform {
			t.Fatalf("non-linux error = %v, want unsupported-platform", err)
		}
	}
}

func TestClientInterfaceDoesNotExposeForbiddenOperations(t *testing.T) {
	var _ Interface = (*recordingEngine)(nil)
}

type recordingEngine struct{}

func (*recordingEngine) Metadata() ServerMetadata { return ServerMetadata{} }
func (*recordingEngine) InspectImage(context.Context, string) (ImageMetadata, error) {
	return ImageMetadata{}, nil
}
func (*recordingEngine) CreateContainer(context.Context, ContainerSpec) (CreatedContainer, error) {
	return CreatedContainer{}, nil
}
func (*recordingEngine) AttachContainer(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (*recordingEngine) StartContainer(context.Context, string) error { return nil }
func (*recordingEngine) WaitContainer(context.Context, string) (WaitResult, error) {
	return WaitResult{}, nil
}
func (*recordingEngine) InspectContainer(context.Context, string) (ContainerState, error) {
	return ContainerState{}, nil
}
func (*recordingEngine) StopContainer(context.Context, string, time.Duration) error { return nil }
func (*recordingEngine) KillContainer(context.Context, string) error                { return nil }
func (*recordingEngine) RemoveContainer(context.Context, string) error              { return nil }
func (*recordingEngine) Close() error                                               { return nil }
