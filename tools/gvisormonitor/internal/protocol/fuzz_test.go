package protocol

import "testing"

func FuzzDecodeRemoteEnvelope(f *testing.F) {
	f.Add(PacketForTest(MessageContainerStart, 0, PayloadForTest(map[uint64]any{1: map[uint64]any{4: int64(1)}})))
	f.Add([]byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = DecodeRemoteEnvelope(data, DefaultLimits()) })
}
