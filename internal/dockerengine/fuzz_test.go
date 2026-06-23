package dockerengine

import (
	"bytes"
	"context"
	"testing"
)

func FuzzValidateDockerSocketPath(f *testing.F) {
	for _, s := range []string{"/var/run/docker.sock", "relative", "tcp://x", "/tmp/../docker.sock", "/tmp/docker\x00.sock"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { _ = validateSocketPathSyntax(s) })
}

func FuzzValidateImmutableLocalImage(f *testing.F) {
	f.Add("registry.example.invalid/dev@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	f.Add("alpine:latest")
	f.Fuzz(func(t *testing.T, s string) { _, _ = ValidateImmutableLocalImage(s) })
}

func FuzzDecodeDockerAttachFrames(f *testing.F) {
	f.Add([]byte{1, 0, 0, 0, 0, 0, 0, 1, 'x'})
	f.Add([]byte{2, 0, 0, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = DecodeDockerAttachFrames(context.Background(), bytes.NewReader(b), 1024, func(context.Context, LogStream, []byte) error { return nil })
	})
}
