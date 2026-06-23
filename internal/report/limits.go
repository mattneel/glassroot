package report

const (
	MaxFindingsAbsolute               int64 = 120000
	MaxDeltaRecordsAbsolute           int64 = 100000
	MaxEvidenceRefsPerFindingAbsolute int64 = 1024
	MaxEvidenceRefsPerDeltaAbsolute   int64 = 1024
	MaxEvidenceRefsTotalAbsolute      int64 = 250000
	MaxLimitationsTotalAbsolute       int64 = 30000
	MaxNoticesAbsolute                int64 = 128
	MaxDisplayInputBytesAbsolute      int64 = 64 << 10
	MaxEscapedDisplayBytesAbsolute    int64 = 256 << 10
	MaxReportJSONBytesAbsolute        int64 = 128 << 20
	MaxMarkdownBytesAbsolute          int64 = 128 << 20
	MaxTerminalBytesAbsolute          int64 = 64 << 20
	MaxRenderedLinesAbsolute          int64 = 2000000
)

type Limits struct {
	MaxFindings               int64
	MaxDeltaRecords           int64
	MaxEvidenceRefsPerFinding int64
	MaxEvidenceRefsPerDelta   int64
	MaxEvidenceRefsTotal      int64
	MaxLimitationsTotal       int64
	MaxNotices                int64
	MaxReportJSONBytes        int64
}

type RenderLimits struct {
	MaxDisplayInputBytes   int64
	MaxEscapedDisplayBytes int64
	MaxMarkdownBytes       int64
	MaxTerminalBytes       int64
	MaxRenderedLines       int64
	MaxEvidenceRefsTotal   int64
}

func DefaultLimits() Limits {
	return Limits{MaxFindings: MaxFindingsAbsolute, MaxDeltaRecords: MaxDeltaRecordsAbsolute, MaxEvidenceRefsPerFinding: MaxEvidenceRefsPerFindingAbsolute, MaxEvidenceRefsPerDelta: MaxEvidenceRefsPerDeltaAbsolute, MaxEvidenceRefsTotal: MaxEvidenceRefsTotalAbsolute, MaxLimitationsTotal: MaxLimitationsTotalAbsolute, MaxNotices: MaxNoticesAbsolute, MaxReportJSONBytes: MaxReportJSONBytesAbsolute}
}

func DefaultRenderLimits() RenderLimits {
	return RenderLimits{MaxDisplayInputBytes: MaxDisplayInputBytesAbsolute, MaxEscapedDisplayBytes: MaxEscapedDisplayBytesAbsolute, MaxMarkdownBytes: MaxMarkdownBytesAbsolute, MaxTerminalBytes: MaxTerminalBytesAbsolute, MaxRenderedLines: MaxRenderedLinesAbsolute, MaxEvidenceRefsTotal: MaxEvidenceRefsTotalAbsolute}
}
