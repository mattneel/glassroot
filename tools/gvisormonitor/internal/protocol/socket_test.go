package protocol

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateMonitorSocketPathRejectsUnsafeEndpoint(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only unixpacket socket contract")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(parent, "events.sock")
	if err := ValidateMonitorSocketPath(valid, DefaultLimits()); err != nil {
		t.Fatalf("valid path rejected: %v", err)
	}
	assertCode(t, ValidateMonitorSocketPath("relative.sock", DefaultLimits()), CodeInvalidSocketPath)
	world := t.TempDir()
	if err := os.Chmod(world, 0o755); err != nil {
		t.Fatal(err)
	}
	assertCode(t, ValidateMonitorSocketPath(filepath.Join(world, "events.sock"), DefaultLimits()), CodeInvalidSocketPath)
	if err := os.WriteFile(valid, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertCode(t, ValidateMonitorSocketPath(valid, DefaultLimits()), CodeSocketExists)
	link := filepath.Join(parent, "link.sock")
	if err := os.Symlink("target.sock", link); err != nil {
		t.Fatal(err)
	}
	assertCode(t, ValidateMonitorSocketPath(link, DefaultLimits()), CodeSocketSymlink)
}
