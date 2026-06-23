package report

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestBuildRejectsMissingImmutableInputs(t *testing.T) {
	b, err := New(DefaultLimits())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = b.Build(context.Background(), BuildRequest{})
	assertReportError(t, err, CodeNilBundle)
}

func TestFrozenReportAndRenderOutputsAreOwned(t *testing.T) {
	doc := Document{
		SchemaVersion:        SchemaVersionReportV1Alpha1,
		ReportProfileVersion: ReportProfileVersionV1Alpha1,
		RunID:                "run-0001",
		Notices:              []Notice{{Code: NoticePassedNotProofOfSafety, Text: noticeText(NoticePassedNotProofOfSafety)}},
		Policy:               PolicySection{OverallEffectiveDisposition: "passed", AppliedFindings: []AppliedFindingReport{}},
		Behavior:             BehaviorSection{Records: []DeltaRecordReport{}},
		Limitations:          []LimitationReport{},
	}
	frozen, err := freezeReportDocument(doc, DefaultLimits())
	if err != nil {
		t.Fatalf("freezeReportDocument() error = %v", err)
	}
	firstDoc := frozen.Document()
	firstJSON := frozen.JSON()
	if len(firstJSON) == 0 || !strings.HasPrefix(string(frozen.Digest()), "sha256:") {
		t.Fatalf("frozen report missing JSON/digest")
	}
	firstDoc.Notices = nil
	firstJSON[0] = '{'
	if len(frozen.Document().Notices) == 0 || !bytes.Equal(frozen.JSON(), frozen.JSON()) {
		t.Fatalf("FrozenReport exposed mutable internals")
	}
	md, err := RenderMarkdown(context.Background(), frozen, DefaultRenderLimits())
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	md.Bytes[0] = 'X'
	md2, err := RenderMarkdown(context.Background(), frozen, DefaultRenderLimits())
	if err != nil {
		t.Fatalf("RenderMarkdown repeat error = %v", err)
	}
	if len(md2.Bytes) == 0 || md2.Bytes[0] == 'X' {
		t.Fatalf("RenderedOutput exposed mutable bytes")
	}
}

func assertReportError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected report error %s, got nil", code)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error %T is not *report.Error: %v", err, err)
	}
	if e.Code != code {
		t.Fatalf("error code=%s want=%s err=%v", e.Code, code, err)
	}
	if strings.ContainsAny(err.Error(), "\x1b\r\n") {
		t.Fatalf("error contains raw controls: %q", err.Error())
	}
}
