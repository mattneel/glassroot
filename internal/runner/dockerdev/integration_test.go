package dockerdev

import (
	"os"
	"testing"
)

func TestDockerDevIntegration(t *testing.T) {
	if os.Getenv("GLASSROOT_DOCKERDEV_INTEGRATION") != "1" {
		t.Skip("set GLASSROOT_DOCKERDEV_INTEGRATION=1 with a reviewed local immutable image to run docker-dev integration tests")
	}
	image := os.Getenv("GLASSROOT_DOCKERDEV_IMAGE")
	if image == "" {
		t.Skip("GLASSROOT_DOCKERDEV_IMAGE must name an already-present immutable image; the test never pulls")
	}
	if _, err := buildContainerSpecForFuzz("/bin/sh", "true", 1, 64<<20, 16); err != nil {
		t.Fatalf("trusted fixture configuration invalid: %v", err)
	}
	t.Skip("real-daemon integration harness requires a reviewed local daemon fixture; ordinary tests remain recorder-based")
}
