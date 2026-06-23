package gvisorspike

import (
	"os"
	"testing"
)

func TestGVisorSpikeIntegration(t *testing.T) {
	if os.Getenv("GLASSROOT_GVISOR_SPIKE") != "1" {
		t.Skip("gVisor spike integration skipped: set GLASSROOT_GVISOR_SPIKE=1 with a pinned runsc, dedicated Docker runtime, immutable local image, Docker socket, and private monitor socket")
	}
	req := PrerequisiteRequest{
		RunscPath:       os.Getenv("GLASSROOT_GVISOR_RUNSC"),
		ExpectedSHA512:  os.Getenv("GLASSROOT_GVISOR_RUNSC_SHA512"),
		ExpectedRelease: os.Getenv("GLASSROOT_GVISOR_RELEASE"),
		Platform:        PlatformMode(os.Getenv("GLASSROOT_GVISOR_PLATFORM")),
		RuntimeName:     os.Getenv("GLASSROOT_GVISOR_RUNTIME"),
		DockerSocket:    os.Getenv("GLASSROOT_DOCKER_SOCKET"),
		MonitorSocket:   os.Getenv("GLASSROOT_GVISOR_MONITOR_SOCKET"),
		FixtureImage:    os.Getenv("GLASSROOT_GVISOR_IMAGE"),
	}
	if _, err := ValidatePrerequisiteRequest(req, Limits{}); err != nil {
		t.Fatalf("invalid gated prerequisite request: %v", err)
	}
	if err := ValidateRunscBinary(req.RunscPath, req.ExpectedSHA512, Limits{}); err != nil {
		t.Fatalf("invalid pinned runsc binary: %v", err)
	}
	t.Skip("live gVisor fixture orchestration is documented but not executed in ordinary CI; runtime validation remains pending until operator-provided runsc/runtime/image are available")
}
