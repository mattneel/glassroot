package monitor

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/mattneel/glassroot/tools/gvisormonitor/internal/model"
)

type replayDoc struct {
	SchemaVersion string        `json:"schemaVersion"`
	PinnedRelease string        `json:"pinnedRelease"`
	Events        []model.Event `json:"events"`
}

func TestReplayFixtureMapsCompleteLifecycle(t *testing.T) {
	data, err := os.ReadFile("testdata/v1alpha1/replay.json")
	if err != nil {
		t.Fatalf("read replay: %v", err)
	}
	var doc replayDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode replay: %v", err)
	}
	if doc.SchemaVersion != "glassroot.dev/gvisor-monitor-replay/v1alpha1" || doc.PinnedRelease != "release-20260615.0" || len(doc.Events) == 0 {
		t.Fatalf("unexpected replay metadata: %+v", doc)
	}
	m := NewStateMachine()
	for _, ev := range doc.Events {
		if len(ev.Limitations) != 0 {
			t.Fatalf("replay should have zero dropped/unsupported limitations: %+v", ev)
		}
		if err := m.Apply(ev); err != nil {
			t.Fatalf("apply replay event %+v: %v", ev, err)
		}
	}
	summary := m.Summary()
	if !summary.Complete || summary.ConnectionCount != 1 || summary.ProcessCreations != 1 || summary.Execs != 1 || summary.Exits != 2 {
		t.Fatalf("unexpected replay summary: %+v", summary)
	}
}
