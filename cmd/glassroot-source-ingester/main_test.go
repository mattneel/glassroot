package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSourceIngesterCLIServeRequiresExplicitLeastPrivilegeInputs(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"serve", "--controller-state-dir", "/state/controller", "--receiver-id", "receiver-1", "--controller-id", "controller-1", "--app-id", "123", "--source-root", "/state/source", "--source-ingester-id", "source-1", "--credential-broker-unix", "/run/broker.sock", "--git-executable", "/usr/bin/git", "--token", "nope"}, &out, &errOut)
	if code != 2 || !strings.Contains(errOut.String(), "unknown flag") {
		t.Fatalf("unexpected token flag result code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	code = run([]string{"serve", "--help"}, &out, &errOut)
	if code != 0 || !strings.Contains(out.String(), "--git-executable") || strings.Contains(out.String(), "--token") || errOut.Len() != 0 {
		t.Fatalf("bad help code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
}
