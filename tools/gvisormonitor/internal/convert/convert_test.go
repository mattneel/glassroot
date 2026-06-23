package convert

import (
	"testing"

	"github.com/mattneel/glassroot/tools/gvisormonitor/internal/protocol"
)

func TestConvertTracePointExtractsBoundedLifecycleFields(t *testing.T) {
	env, err := protocol.DecodeRemoteEnvelope(protocol.PacketForTest(protocol.MessageSentryClone, 0, protocol.PayloadForTest(map[uint64]any{
		1: map[uint64]any{1: int64(100), 2: int64(11), 4: int64(11), 6: "container-raw-id", 9: "parent", 10: int64(1)},
		3: int64(22),
		4: int64(22),
	})), protocol.DefaultLimits())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	ev, err := ConvertTracePoint(env, DefaultLimits())
	if err != nil {
		t.Fatalf("ConvertTracePoint() error = %v", err)
	}
	if ev.Operation != OperationClone || ev.ThreadGroupID != 11 || ev.ChildThreadGroupID != 22 || ev.ProcessName != "parent" || ev.ContainerIdentity == "container-raw-id" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestConvertTracePointTreatsUnknownAsLimitation(t *testing.T) {
	env, err := protocol.DecodeRemoteEnvelope(protocol.PacketForTest(65535, 0, []byte{1, 2, 3}), protocol.DefaultLimits())
	if err != nil {
		t.Fatalf("decode unknown envelope: %v", err)
	}
	ev, err := ConvertTracePoint(env, DefaultLimits())
	if err != nil {
		t.Fatalf("unknown tracepoint should be bounded limitation, got %v", err)
	}
	if ev.Operation != OperationUnsupported || len(ev.Limitations) == 0 {
		t.Fatalf("unknown message not represented as limitation: %+v", ev)
	}
}
