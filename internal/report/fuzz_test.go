package report

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/model"
)

func FuzzVisibleDisplayText(f *testing.F) {
	for _, seed := range [][]byte{
		{},
		[]byte("plain"),
		[]byte("# heading\n[text](javascript:alert(1))"),
		[]byte("\x1b[31m\x1b]52;c;AAAA\a\b\r\t\x00"),
		[]byte("\xe2\x80\xae\ufeff\u200b"),
		{0xff, 0xfe, 0xfd},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		out, err := visibleDisplayBytes(input, DefaultRenderLimits())
		if err != nil {
			var re *Error
			if !errors.As(err, &re) {
				t.Fatalf("unexpected error type: %T %v", err, err)
			}
			return
		}
		if !utf8.ValidString(out) {
			t.Fatalf("visible output is invalid UTF-8")
		}
		if !terminalSafe(out) {
			t.Fatalf("visible output retained terminal-active controls: %q", out)
		}
	})
}

func FuzzMarkdownCodeSpan(f *testing.F) {
	for _, seed := range []string{"", "value", "`", "``` fence ```", " leading", "trailing ", "\n", "\x1b[31m"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			input = input[:4096]
		}
		visible, err := visibleDisplayBytes([]byte(input), DefaultRenderLimits())
		if err != nil {
			return
		}
		span := markdownCodeSpan(visible)
		if !utf8.ValidString(span) {
			t.Fatalf("code span is invalid UTF-8")
		}
		if strings.ContainsAny(span, "\r\n\x00\x1b") {
			t.Fatalf("code span retained raw controls: %q", span)
		}
		if !strings.Contains(span, visible) {
			t.Fatalf("code span omitted visible value")
		}
	})
}

func FuzzRenderReportValue(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(""),
		[]byte("`ticks`"),
		[]byte("[link](file:///etc/passwd)"),
		[]byte("<script>"),
		[]byte("\x1b]8;;https://evil\aX\x1b]8;;\a"),
		{0xff},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 4096 {
			input = input[:4096]
		}
		l := DefaultRenderLimits()
		visible, err := visibleDisplayBytes(input, l)
		if err != nil {
			return
		}
		var b renderBuilder
		b.limits = l
		b.maxBytes = l.MaxMarkdownBytes
		b.tooLargeCode = CodeMarkdownTooLarge
		b.kv("Value", string(input))
		if b.err != nil {
			return
		}
		rendered := b.String()
		if !utf8.ValidString(rendered) {
			t.Fatalf("rendered value is invalid UTF-8")
		}
		if strings.ContainsAny(rendered, "\r\x00\x1b") {
			t.Fatalf("rendered value retained raw controls: %q", rendered)
		}
		if !strings.Contains(rendered, visible) {
			t.Fatalf("rendered value omitted visible representation")
		}
	})
}

func FuzzBuildReportBindings(f *testing.F) {
	f.Add("run-1", "sha256:"+strings.Repeat("a", 64), "base", "head")
	f.Add("\x1b[31m", "sha256:"+strings.Repeat("g", 64), "", "")
	f.Add("", "", "same", "same")
	f.Fuzz(func(t *testing.T, runID, digest, baseID, headID string) {
		runID = boundedFuzzString(runID)
		digest = boundedFuzzString(digest)
		baseID = boundedFuzzString(baseID)
		headID = boundedFuzzString(headID)
		_ = validDigest(model.Digest(digest))
		base := model.CommitRef{Kind: model.RevisionKindBase, CommitID: baseID, TreeID: baseID, ObjectFormat: model.GitObjectFormatSHA1}
		head := model.CommitRef{Kind: model.RevisionKindHead, CommitID: headID, TreeID: headID, ObjectFormat: model.GitObjectFormatSHA1}
		_ = sameCommit(base, head)
		_ = deltaCommitCompatible(base, model.CommitRef{})
		doc := Document{
			SchemaVersion:        SchemaVersionReportV1Alpha1,
			ReportProfileVersion: ReportProfileVersionV1Alpha1,
			RunID:                runID,
			PlanDigest:           model.Digest(digest),
			Source:               SourceIdentity{Base: base, Head: head},
			Policy:               PolicySection{AppliedFindings: []AppliedFindingReport{}},
			Behavior:             BehaviorSection{Records: []DeltaRecordReport{}},
			Notices:              []Notice{{Code: NoticePassedNotProofOfSafety, Text: noticeText(NoticePassedNotProofOfSafety)}},
			Limitations:          []LimitationReport{},
		}
		frozen, err := freezeReportDocument(doc, DefaultLimits())
		if err != nil {
			return
		}
		_, _ = RenderMarkdown(context.Background(), frozen, DefaultRenderLimits())
		_, _ = RenderTerminal(context.Background(), frozen, DefaultRenderLimits())
	})
}

func boundedFuzzString(s string) string {
	if len(s) > 4096 {
		return s[:4096]
	}
	return s
}
