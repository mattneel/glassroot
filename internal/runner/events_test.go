package runner

import (
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/model"
)

func TestEventIDEncodingGoldenAndBoundarySafety(t *testing.T) {
	got := eventID(model.Digest("sha256:57f760eda77f115e65281853b432099d00e6f41516faa6361672269db9d3b24c"), "run-0001", 1)
	want := "evt-d30e813684eea5e3e789c3d9f652154b7abee4f9c664d5fe60cb8190d4b13386"
	if got != want {
		t.Fatalf("event id = %s, want %s", got, want)
	}
	if !strings.HasPrefix(got, "evt-") || len(got) != len("evt-")+64 {
		t.Fatalf("event id format invalid: %s", got)
	}
	a := eventID("sha256:ab", "c", 1)
	b := eventID("sha256:a", "bc", 1)
	if a == b {
		t.Fatal("event id encoding is ambiguous across field boundaries")
	}
	if eventID("sha256:ab", "c", 1) == eventID("sha256:ab", "c", 2) {
		t.Fatal("event id did not include sequence")
	}
}
