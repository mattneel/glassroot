package convert

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/mattneel/glassroot/tools/gvisormonitor/internal/model"
	"github.com/mattneel/glassroot/tools/gvisormonitor/internal/protocol"
)

type Operation = model.Operation

const (
	OperationContainerStart = model.OperationContainerStart
	OperationClone          = model.OperationClone
	OperationExec           = model.OperationExec
	OperationExit           = model.OperationExit
	OperationUnsupported    = model.OperationUnsupported
)

type Limits struct {
	MaxStringBytes   int64
	MaxArguments     int64
	MaxArgumentBytes int64
}

func DefaultLimits() Limits {
	return Limits{MaxStringBytes: 64 << 10, MaxArguments: 4096, MaxArgumentBytes: 256 << 10}
}

func ConvertTracePoint(env protocol.Envelope, limits Limits) (model.Event, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	fields, err := protocol.DecodeFields(env.Payload, protocol.DefaultLimits())
	if err != nil {
		if env.MessageType > 1000 {
			return unsupported(env), nil
		}
		return model.Event{}, errCode(CodeProtobufInvalid, "convert", "payload", "decode tracepoint payload", err)
	}
	ev := model.Event{SchemaVersion: "glassroot.dev/gvisor-monitor-event/v1alpha1", Operation: model.OperationUnsupported, Limitations: []string{}}
	ctx := map[uint64]any{}
	if m, ok := fields[1].(map[uint64]any); ok {
		ctx = m
	}
	applyContext(&ev, ctx)
	switch env.MessageType {
	case protocol.MessageContainerStart:
		ev.Operation = model.OperationContainerStart
		ev.TracePoint = "container/start"
		if id, ok := fields[2].(string); ok {
			ev.ContainerIdentity = hashIdentity(id)
		}
	case protocol.MessageSentryClone:
		ev.Operation = model.OperationClone
		ev.TracePoint = "sentry/clone"
		ev.ChildThreadGroupID = intField(fields, 4)
	case protocol.MessageSentryExec:
		ev.Operation = model.OperationExec
		ev.TracePoint = "sentry/execve"
		ev.ExecutablePath = stringField(fields, 2, limits)
	case protocol.MessageSyscallExecve:
		ev.Operation = model.OperationExec
		ev.TracePoint = "syscall/execve"
		ev.ExecutablePath = stringField(fields, 6, limits)
	case protocol.MessageSentryExit, protocol.MessageSentryTaskExit:
		ev.Operation = model.OperationExit
		ev.TracePoint = "sentry/exit"
		if v := intField(fields, 2); v != 0 {
			vv := int(v)
			ev.ExitStatus = &vv
		}
	case protocol.MessageSyscallClone:
		ev.Operation = model.OperationClone
		ev.TracePoint = "syscall/clone"
		ev.ChildThreadGroupID = intField(fields, 6)
	default:
		return unsupported(env), nil
	}
	if ev.Limitations == nil {
		ev.Limitations = []string{}
	}
	return ev, nil
}

func unsupported(env protocol.Envelope) model.Event {
	return model.Event{SchemaVersion: "glassroot.dev/gvisor-monitor-event/v1alpha1", Operation: model.OperationUnsupported, TracePoint: fmt.Sprintf("unknown:%d", env.MessageType), Limitations: []string{"unsupported-message-type"}}
}
func applyContext(ev *model.Event, ctx map[uint64]any) {
	ev.TimestampNanos = intField(ctx, 1)
	ev.ThreadID = intField(ctx, 2)
	ev.ThreadGroupID = intField(ctx, 4)
	ev.ContainerIdentity = hashIdentity(stringField(ctx, 6, DefaultLimits()))
	ev.ProcessName = stringField(ctx, 9, DefaultLimits())
	ev.ParentThreadGroupID = intField(ctx, 10)
}
func intField(m map[uint64]any, k uint64) int64 {
	switch v := m[k].(type) {
	case uint64:
		return int64(v)
	case int64:
		return v
	}
	return 0
}
func stringField(m map[uint64]any, k uint64, l Limits) string {
	s, _ := m[k].(string)
	if int64(len(s)) > l.MaxStringBytes {
		return s[:l.MaxStringBytes]
	}
	return s
}
func hashIdentity(in string) string {
	if in == "" {
		return ""
	}
	h := sha256.Sum256([]byte(in))
	return "sha256:" + hex.EncodeToString(h[:])
}
