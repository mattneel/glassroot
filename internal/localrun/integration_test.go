package localrun

import (
	"os"
	"testing"
)

func TestLocalRunIntegration(t *testing.T) {
	if os.Getenv("GLASSROOT_LOCALRUN_INTEGRATION") != "1" {
		t.Skip("set GLASSROOT_LOCALRUN_INTEGRATION=1 to run the gated localrun Docker integration suite")
	}
	if os.Getenv("GLASSROOT_DOCKERDEV_IMAGE") == "" {
		t.Skip("GLASSROOT_DOCKERDEV_IMAGE must name an already-present immutable local image; localrun never pulls images")
	}
	t.Skip("real Docker localrun fixture construction is not available in this checkout; runtime validation remains pending")
}
