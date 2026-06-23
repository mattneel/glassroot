package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestReceiverCLIUsage(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"serve", "--help"}, {"--help"}} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("args %v exit=%d stderr=%q", args, code, stderr.String())
		}
		if stdout.Len() == 0 {
			t.Fatalf("args %v produced empty stdout", args)
		}
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"serve", "--listen-unix", "/tmp/a.sock", "--listen-unix", "/tmp/b.sock"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "duplicate flag") {
		t.Fatalf("duplicate exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
