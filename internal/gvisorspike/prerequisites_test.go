package gvisorspike

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePrerequisiteRequestRejectsUnpinnedInputs(t *testing.T) {
	base := PrerequisiteRequest{RunscPath: "/opt/runsc", DockerSocket: "/run/docker.sock", MonitorSocket: "/tmp/glassroot-gvisor/events.sock", ExpectedRelease: PinnedRunscRelease, ExpectedSHA512: strings.Repeat("a", 128), Platform: PlatformSystrap, RuntimeName: "runsc-glassroot-spike", FixtureImage: "registry.example.invalid/fixture@sha256:" + strings.Repeat("b", 64)}
	if _, err := ValidatePrerequisiteRequest(base, Limits{}); err != nil {
		t.Fatalf("valid prerequisite request rejected: %v", err)
	}
	bad := base
	bad.ExpectedRelease = "latest"
	assertCode(t, mustPrereqErr(bad), CodeRunscVersionMismatch)
	bad = base
	bad.ExpectedSHA512 = strings.Repeat("A", 128)
	assertCode(t, mustPrereqErr(bad), CodeRunscHashMismatch)
	bad = base
	bad.Platform = "ptrace"
	assertCode(t, mustPrereqErr(bad), CodeUnsupportedPlatformMode)
	bad = base
	bad.RuntimeName = "runc"
	assertCode(t, mustPrereqErr(bad), CodeRuntimeNotConfigured)
}

func mustPrereqErr(req PrerequisiteRequest) error {
	_, err := ValidatePrerequisiteRequest(req, Limits{})
	return err
}

func TestValidateRunscBinaryRejectsSymlinkAndHashMismatch(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "runsc")
	if err := os.WriteFile(bin, []byte("trusted-runsc"), 0o700); err != nil {
		t.Fatal(err)
	}
	hash, err := HashFileSHA512(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRunscBinary(bin, hash, Limits{}); err != nil {
		t.Fatalf("valid runsc rejected: %v", err)
	}
	assertCode(t, ValidateRunscBinary(bin, strings.Repeat("b", 128), Limits{}), CodeRunscHashMismatch)
	link := filepath.Join(dir, "runsc-link")
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	assertCode(t, ValidateRunscBinary(link, hash, Limits{}), CodeRunscSymlink)
}
