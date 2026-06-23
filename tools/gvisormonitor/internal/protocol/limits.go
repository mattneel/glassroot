package protocol

import "time"

const CurrentVersion = uint32(1)
const HeaderSize = 8
const (
	MessageContainerStart uint16 = 1
	MessageSentryClone    uint16 = 2
	MessageSentryExec     uint16 = 3
	MessageSentryExit     uint16 = 4
	MessageSentryTaskExit uint16 = 5
	MessageSyscallExecve  uint16 = 11
	MessageSyscallClone   uint16 = 23
)

type Limits struct {
	MaxHandshakeBytes     int64
	MaxHeaderBytes        int64
	MaxMessageBytes       int64
	MaxStringBytes        int64
	MaxArguments          int64
	MaxArgumentBytes      int64
	MaxConnectionDuration time.Duration
}

func DefaultLimits() Limits {
	return Limits{MaxHandshakeBytes: 64 << 10, MaxHeaderBytes: 4 << 10, MaxMessageBytes: 1 << 20, MaxStringBytes: 64 << 10, MaxArguments: 4096, MaxArgumentBytes: 256 << 10, MaxConnectionDuration: 10 * time.Minute}
}
