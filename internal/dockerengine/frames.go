package dockerengine

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
)

const maxAttachFrameBytes = 16 << 20

func DecodeDockerAttachFrames(ctx context.Context, r io.Reader, maxPerStream int64, cb func(context.Context, LogStream, []byte) error) (OutputCounts, error) {
	if err := ctx.Err(); err != nil {
		return OutputCounts{}, errCode(CodeContextCancelled, "attach", "context", "context cancelled", err)
	}
	if r == nil {
		return OutputCounts{}, errCode(CodeEngineResponseInvalid, "attach", "reader", "attach reader is required", nil)
	}
	if maxPerStream < 0 {
		return OutputCounts{}, errCode(CodeEngineOutputTooLarge, "attach", "limits", "output limit is invalid", nil)
	}
	var counts OutputCounts
	header := make([]byte, 8)
	buf := make([]byte, 32<<10)
	for {
		_, err := io.ReadFull(r, header)
		if errors.Is(err, io.EOF) {
			return counts, nil
		}
		if err != nil {
			return counts, errCode(CodeEngineResponseInvalid, "attach", "frame", "malformed Docker attach frame", err)
		}
		var stream LogStream
		switch header[0] {
		case 1:
			stream = LogStreamStdout
		case 2:
			stream = LogStreamStderr
		default:
			return counts, errCode(CodeEngineResponseInvalid, "attach", "stream", "unknown Docker attach stream", nil)
		}
		size := int64(binary.BigEndian.Uint32(header[4:8]))
		if size > maxAttachFrameBytes {
			return counts, errCode(CodeEngineOutputTooLarge, "attach", "frame", "attach frame exceeds bounded size", nil)
		}
		remaining := size
		for remaining > 0 {
			if err := ctx.Err(); err != nil {
				return counts, errCode(CodeContextCancelled, "attach", "context", "context cancelled", err)
			}
			chunk := int64(len(buf))
			if remaining < chunk {
				chunk = remaining
			}
			if _, err := io.ReadFull(r, buf[:chunk]); err != nil {
				return counts, errCode(CodeEngineResponseInvalid, "attach", "payload", "malformed Docker attach payload", err)
			}
			data := buf[:chunk]
			accepted := int64(0)
			switch stream {
			case LogStreamStdout:
				counts.StdoutObservedAtLeast += chunk
				available := maxPerStream - counts.StdoutAccepted
				if available > 0 {
					accepted = min64(chunk, available)
					counts.StdoutAccepted += accepted
				}
				if accepted < chunk {
					counts.StdoutTruncated = true
				}
			case LogStreamStderr:
				counts.StderrObservedAtLeast += chunk
				available := maxPerStream - counts.StderrAccepted
				if available > 0 {
					accepted = min64(chunk, available)
					counts.StderrAccepted += accepted
				}
				if accepted < chunk {
					counts.StderrTruncated = true
				}
			}
			if accepted > 0 {
				if cb == nil {
					return counts, errCode(CodeEngineResponseInvalid, "attach", "sink", "output sink is required", nil)
				}
				if err := cb(ctx, stream, append([]byte(nil), data[:accepted]...)); err != nil {
					return counts, errCode(CodeEngineResponseInvalid, "attach", "sink", "output sink failed", err)
				}
			}
			remaining -= chunk
		}
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
