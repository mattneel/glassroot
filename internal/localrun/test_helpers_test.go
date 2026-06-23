package localrun

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func assertLocalRunError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", code)
	}
	var lerr *Error
	if !errors.As(err, &lerr) {
		t.Fatalf("error %T is not *Error: %v", err, err)
	}
	if lerr.Code != code {
		t.Fatalf("code = %s, want %s; err=%v", lerr.Code, code, err)
	}
	if strings.ContainsAny(err.Error(), "\x1b\r\n") {
		t.Fatalf("error contains raw terminal controls: %q", err.Error())
	}
}

func mkdir0700(path string) error { return os.Mkdir(path, 0o700) }
