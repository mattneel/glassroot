package convert

import (
	"testing"

	"github.com/mattneel/glassroot/tools/gvisormonitor/internal/protocol"
)

func FuzzConvertGVisorTracePoint(f *testing.F) {
	f.Add(uint16(protocol.MessageSentryClone), protocol.PayloadForTest(map[uint64]any{1: map[uint64]any{4: int64(1)}, 4: int64(2)}))
	f.Add(uint16(65535), []byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, typ uint16, payload []byte) {
		_, _ = ConvertTracePoint(protocol.Envelope{MessageType: typ, Payload: payload}, DefaultLimits())
	})
}
