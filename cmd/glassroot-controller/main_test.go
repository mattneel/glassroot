package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpHasNoSideEffectsAndDuplicateFlagIsUsageError(t *testing.T) {
	var out, err bytes.Buffer
	if code := run([]string{"serve", "--help"}, &out, &err); code != 0 {
		t.Fatalf("help code=%d err=%s", code, err.String())
	}
	if !strings.Contains(out.String(), "glassroot-controller serve") {
		t.Fatalf("help missing usage: %s", out.String())
	}
	out.Reset()
	err.Reset()
	code := run([]string{"serve", "--controller-id", "controller-1", "--controller-id", "controller-2"}, &out, &err)
	if code != 2 || !strings.Contains(err.String(), "duplicate flag") {
		t.Fatalf("duplicate code=%d stderr=%s", code, err.String())
	}
}

func TestServeRejectsPathOverlapAtUsageLayer(t *testing.T) {
	var out, err bytes.Buffer
	code := run([]string{"serve",
		"--inbox-state-dir", "/tmp/glassroot-controller-test",
		"--receiver-id", "receiver-1",
		"--controller-state-dir", "/tmp/glassroot-controller-test/controller",
		"--controller-id", "controller-1",
		"--credential-broker-unix", "/tmp/glassroot-controller-test-broker.sock",
		"--app-id", "123",
	}, &out, &err)
	if code != 2 || !strings.Contains(err.String(), "overlap") {
		t.Fatalf("code=%d stderr=%s", code, err.String())
	}
}
