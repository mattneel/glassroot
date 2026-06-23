package protocol

import "testing"

func TestDecodeHandshakeValidatesVersion(t *testing.T) {
	version, err := DecodeHandshake(EncodeHandshakeForTest(CurrentVersion), DefaultLimits())
	if err != nil {
		t.Fatalf("DecodeHandshake() error = %v", err)
	}
	if version != CurrentVersion {
		t.Fatalf("version = %d", version)
	}
	_, err = DecodeHandshake(EncodeHandshakeForTest(CurrentVersion+1), DefaultLimits())
	assertCode(t, err, CodeProtocolVersionUnsupported)
}

func TestDecodeRemoteEnvelopeBoundsHeaderPayloadAndDrops(t *testing.T) {
	packet := PacketForTest(1, 0, PayloadForTest(map[uint64]any{1: map[uint64]any{4: int64(12), 9: "parent"}, 2: "sandbox"}))
	env, err := DecodeRemoteEnvelope(packet, DefaultLimits())
	if err != nil {
		t.Fatalf("DecodeRemoteEnvelope() error = %v", err)
	}
	if env.MessageType != MessageContainerStart || env.DroppedCount != 0 || len(env.Payload) == 0 {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	packet[6] = 1
	_, err = DecodeRemoteEnvelope(packet, DefaultLimits())
	assertCode(t, err, CodeDroppedEvents)
}

func TestDecodeRemoteEnvelopeRejectsMalformedPackets(t *testing.T) {
	_, err := DecodeRemoteEnvelope([]byte{1, 2, 3}, DefaultLimits())
	assertCode(t, err, CodeHeaderInvalid)
	limits := DefaultLimits()
	limits.MaxMessageBytes = 8
	_, err = DecodeRemoteEnvelope(PacketForTest(1, 0, []byte("0123456789")), limits)
	assertCode(t, err, CodeMessageTooLarge)
}

func assertCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected code %s, got nil", code)
	}
	var got *Error
	if !As(err, &got) || got.Code != code {
		t.Fatalf("expected code %s, got %v", code, err)
	}
}
