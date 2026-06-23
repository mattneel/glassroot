package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCredentialBrokerCLIHelpHasNoSideEffects(t *testing.T) {
	var out, err bytes.Buffer
	if code := run([]string{"serve", "--help"}, &out, &err); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, err.String())
	}
	if !strings.Contains(out.String(), "--private-key-file") || err.Len() != 0 {
		t.Fatalf("unexpected output out=%q err=%q", out.String(), err.String())
	}
}

func TestCredentialBrokerCLIRejectsDuplicateUnknownAndMissingFlags(t *testing.T) {
	cases := [][]string{
		{"serve", "--listen-unix", "/tmp/a", "--listen-unix", "/tmp/b", "--private-key-file", "/tmp/k", "--app-id", "1", "--app-client-id", "Iv1.x"},
		{"serve", "--listen-unix", "/tmp/a", "--private-key-file", "/tmp/k", "--app-id", "1", "--app-client-id", "Iv1.x", "--github-host", "evil.example"},
		{"serve", "--listen-unix", "/tmp/a", "--app-id", "1", "--app-client-id", "Iv1.x"},
		{"serve", "--listen-unix", "/tmp/a", "--private-key-file", "/tmp/k", "--app-id", "0", "--app-client-id", "Iv1.x"},
	}
	for _, args := range cases {
		var out, stderr bytes.Buffer
		if code := run(args, &out, &stderr); code != 2 {
			t.Fatalf("%v code=%d out=%q err=%q", args, code, out.String(), stderr.String())
		}
	}
}
