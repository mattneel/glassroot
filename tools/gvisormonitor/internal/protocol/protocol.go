package protocol

import (
	"encoding/binary"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"gvisor.dev/gvisor/pkg/sentry/seccheck/sinks/remote/wire"
)

var _ = wire.CurrentVersion

type Envelope struct {
	MessageType  uint16
	DroppedCount uint32
	Payload      []byte
}

func DecodeHandshake(in []byte, limits Limits) (uint32, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if len(in) == 0 || int64(len(in)) > limits.MaxHandshakeBytes {
		return 0, errCode(CodeHandshakeInvalid, "handshake", "size", "handshake size invalid", nil)
	}
	fields, err := DecodeFields(in, limits)
	if err != nil {
		return 0, errCode(CodeHandshakeInvalid, "handshake", "protobuf", "handshake protobuf invalid", err)
	}
	v, ok := fields[1].(uint64)
	if !ok || v == 0 {
		return 0, errCode(CodeHandshakeInvalid, "handshake", "version", "missing protocol version", nil)
	}
	if uint32(v) != CurrentVersion {
		return 0, errCode(CodeProtocolVersionUnsupported, "handshake", "version", "unsupported protocol version", nil)
	}
	return uint32(v), nil
}

func DecodeRemoteEnvelope(packet []byte, limits Limits) (Envelope, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if len(packet) < HeaderSize {
		return Envelope{}, errCode(CodeHeaderInvalid, "message", "header", "packet smaller than gVisor remote header", nil)
	}
	if int64(HeaderSize) > limits.MaxHeaderBytes {
		return Envelope{}, errCode(CodeHeaderInvalid, "message", "header", "header exceeds configured bound", nil)
	}
	if int64(len(packet)) > limits.MaxMessageBytes {
		return Envelope{}, errCode(CodeMessageTooLarge, "message", "size", "message exceeds configured bound", nil)
	}
	headerSize := binary.LittleEndian.Uint16(packet[0:2])
	msgType := binary.LittleEndian.Uint16(packet[2:4])
	dropped := binary.LittleEndian.Uint32(packet[4:8])
	if headerSize < HeaderSize || int(headerSize) > len(packet) || int64(headerSize) > limits.MaxHeaderBytes {
		return Envelope{}, errCode(CodeHeaderInvalid, "message", "header", "invalid header size", nil)
	}
	if dropped != 0 {
		return Envelope{}, errCode(CodeDroppedEvents, "message", "dropped", "gVisor remote sink reported dropped messages", nil)
	}
	payload := append([]byte(nil), packet[headerSize:]...)
	return Envelope{MessageType: msgType, DroppedCount: dropped, Payload: payload}, nil
}

func DecodeFields(payload []byte, limits Limits) (map[uint64]any, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if int64(len(payload)) > limits.MaxMessageBytes {
		return nil, errCode(CodeMessageTooLarge, "protobuf", "size", "payload too large", nil)
	}
	out := map[uint64]any{}
	for len(payload) > 0 {
		num, typ, n := protowire.ConsumeTag(payload)
		if n < 0 {
			return nil, errCode(CodeProtobufInvalid, "protobuf", "tag", "invalid protobuf tag", protowire.ParseError(n))
		}
		payload = payload[n:]
		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(payload)
			if n < 0 {
				return nil, errCode(CodeProtobufInvalid, "protobuf", "varint", "invalid varint", protowire.ParseError(n))
			}
			payload = payload[n:]
			out[uint64(num)] = v
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(payload)
			if n < 0 {
				return nil, errCode(CodeProtobufInvalid, "protobuf", "bytes", "invalid bytes", protowire.ParseError(n))
			}
			payload = payload[n:]
			if int64(len(v)) > limits.MaxStringBytes {
				return nil, errCode(CodeFieldLimit, "protobuf", "bytes", "bytes field too large", nil)
			}
			if nested, err := DecodeFields(v, limits); err == nil && len(nested) > 0 {
				out[uint64(num)] = nested
			} else {
				out[uint64(num)] = string(append([]byte(nil), v...))
			}
		case protowire.Fixed32Type:
			v, n := protowire.ConsumeFixed32(payload)
			if n < 0 {
				return nil, errCode(CodeProtobufInvalid, "protobuf", "fixed32", "invalid fixed32", protowire.ParseError(n))
			}
			payload = payload[n:]
			out[uint64(num)] = uint64(v)
		case protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(payload)
			if n < 0 {
				return nil, errCode(CodeProtobufInvalid, "protobuf", "fixed64", "invalid fixed64", protowire.ParseError(n))
			}
			payload = payload[n:]
			out[uint64(num)] = v
		default:
			return nil, errCode(CodeProtobufInvalid, "protobuf", "wire", fmt.Sprintf("unsupported wire type %d", typ), nil)
		}
	}
	return out, nil
}

func EncodeHandshakeForTest(version uint32) []byte {
	return protowire.AppendVarint(protowire.AppendTag(nil, 1, protowire.VarintType), uint64(version))
}
func PacketForTest(msgType uint16, dropped uint32, payload []byte) []byte {
	out := make([]byte, HeaderSize+len(payload))
	binary.LittleEndian.PutUint16(out[0:2], HeaderSize)
	binary.LittleEndian.PutUint16(out[2:4], msgType)
	binary.LittleEndian.PutUint32(out[4:8], dropped)
	copy(out[8:], payload)
	return out
}
func PayloadForTest(fields map[uint64]any) []byte {
	var out []byte
	keys := make([]uint64, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	for _, k := range keys {
		v := fields[k]
		switch x := v.(type) {
		case int64:
			out = protowire.AppendVarint(protowire.AppendTag(out, protowire.Number(k), protowire.VarintType), uint64(x))
		case uint64:
			out = protowire.AppendVarint(protowire.AppendTag(out, protowire.Number(k), protowire.VarintType), x)
		case string:
			out = protowire.AppendString(protowire.AppendTag(out, protowire.Number(k), protowire.BytesType), x)
		case []byte:
			out = protowire.AppendBytes(protowire.AppendTag(out, protowire.Number(k), protowire.BytesType), x)
		case map[uint64]any:
			out = protowire.AppendBytes(protowire.AppendTag(out, protowire.Number(k), protowire.BytesType), PayloadForTest(x))
		}
	}
	return out
}
