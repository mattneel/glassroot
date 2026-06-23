package evidence

import (
	"context"
	"io"
	"os"
)

type LogCapture struct {
	session   *Session
	attempt   *attemptState
	stream    LogStream
	file      *os.File
	path      string
	limit     int64
	stored    int64
	observed  int64
	hash      hashWriter
	truncated bool
	closed    bool
}

func (s *Session) OpenLog(ctx context.Context, key AttemptKey, stream LogStream) (*LogCapture, error) {
	if err := s.ensureActive("open-log"); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, s.fail(contextErr(err))
	}
	if stream != LogStreamStdout && stream != LogStreamStderr {
		return nil, errCode(CodeInvalidLogStream, "log", "stream", "invalid log stream", nil)
	}
	as, err := s.attemptForKey(key)
	if err != nil {
		return nil, err
	}
	if stream == LogStreamStdout && as.stdout != nil {
		return nil, attemptErr(CodeDuplicateLogCapture, "log", "open", as.attemptID, "stdout already opened", nil)
	}
	if stream == LogStreamStderr && as.stderr != nil {
		return nil, attemptErr(CodeDuplicateLogCapture, "log", "open", as.attemptID, "stderr already opened", nil)
	}
	rel := as.dir + "/" + string(stream) + ".log"
	f, err := s.openExclusive(rel)
	if err != nil {
		return nil, s.fail(err)
	}
	cap := &LogCapture{session: s, attempt: as, stream: stream, file: f, path: rel, limit: s.writer.limits.MaxLogBytesPerStream}
	if stream == LogStreamStdout {
		as.stdout = cap
		as.stdoutState = CaptureStateCapturedEmpty
	} else {
		as.stderr = cap
		as.stderrState = CaptureStateCapturedEmpty
	}
	return cap, nil
}

func (s *Session) attemptForKey(key AttemptKey) (*attemptState, error) {
	as, ok := s.attempts[attemptKeyString(key)]
	if !ok {
		return nil, errCode(CodeInvalidAttempt, "attempt", "lookup", "unknown attempt", nil)
	}
	return as, nil
}

func (c *LogCapture) Write(p []byte) (int, error) {
	if c == nil || c.closed || c.file == nil {
		return 0, errCode(CodeLogWriteFailed, "log", "write", "log capture is closed", nil)
	}
	if c.session.state != StateActive {
		return 0, errCode(CodeInvalidSessionState, "log", "write", "session is not active", nil)
	}
	c.observed += int64(len(p))
	streamRemaining := c.limit - c.stored
	totalRemaining := c.session.writer.limits.MaxTotalLogBytes - c.session.totalLogBytes
	remaining := streamRemaining
	if totalRemaining < remaining {
		remaining = totalRemaining
	}
	if remaining > 0 {
		chunk := p
		if int64(len(chunk)) > remaining {
			chunk = chunk[:remaining]
			c.truncated = true
		}
		n, err := c.file.Write(chunk)
		if err != nil || n != len(chunk) {
			if err == nil {
				err = io.ErrShortWrite
			}
			c.session.fail(attemptErr(CodeLogWriteFailed, "log", "write", c.attempt.attemptID, "write log", err))
			return n, err
		}
		c.stored += int64(n)
		c.hash.buf.Write(chunk)
		c.session.totalLogBytes += int64(n)
	}
	if int64(len(p)) > remaining {
		c.truncated = true
		c.session.evidenceIncomplete = true
	}
	return len(p), nil
}

func (c *LogCapture) Close() error {
	if c == nil || c.closed {
		return nil
	}
	c.closed = true
	if err := syncFile(c.file, c.session.writer.hooks); err != nil {
		return c.session.fail(attemptErr(CodeSyncFailed, "log", "sync", c.attempt.attemptID, "sync log", err))
	}
	if err := c.file.Close(); err != nil {
		return c.session.fail(attemptErr(CodeLogWriteFailed, "log", "close", c.attempt.attemptID, "close log", err))
	}
	state := CaptureStateCaptured
	if c.stored == 0 {
		state = CaptureStateCapturedEmpty
	}
	if c.truncated {
		state = CaptureStateTruncated
	}
	if c.stream == LogStreamStdout {
		c.attempt.stdoutState = state
	} else {
		c.attempt.stderrState = state
	}
	entryRole := EntryRoleStdout
	if c.stream == LogStreamStderr {
		entryRole = EntryRoleStderr
	}
	entry := ManifestEntry{Path: c.path, Role: entryRole, MediaType: "application/octet-stream", Digest: digestBytes(c.hash.buf.Bytes()), SizeBytes: c.stored, Revision: c.attempt.key.Revision, ScenarioID: c.attempt.key.ScenarioID, Repetition: c.attempt.key.Repetition, CaptureState: state, Truncated: c.truncated}
	if c.truncated {
		entry.ObservedBytesAtLeast = c.observed
	} else {
		entry.ObservedBytes = c.observed
	}
	return c.session.addEntry(entry)
}
