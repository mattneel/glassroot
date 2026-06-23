package runner

import "context"

// LogStream identifies a raw attempt log stream. The bytes are evidence data;
// runner implementations must not decode, sanitize, or render them.
type LogStream string

const (
	LogStreamStdout LogStream = "stdout"
	LogStreamStderr LogStream = "stderr"
)

// AttemptOutputSink receives raw stdout/stderr bytes for one already-bound
// attempt. The backend cannot choose another attempt identity through this
// interface.
type AttemptOutputSink interface {
	WriteLog(context.Context, LogStream, []byte) error
}

type discardOutputSink struct{}

func (discardOutputSink) WriteLog(context.Context, LogStream, []byte) error { return nil }

// DiscardOutputSink returns a bounded no-op log sink for callers that have not
// yet opted into evidence log capture.
func DiscardOutputSink() AttemptOutputSink { return discardOutputSink{} }
