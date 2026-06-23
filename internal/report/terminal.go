package report

import (
	"context"
	"strings"

	"github.com/mattneel/glassroot/internal/model"
)

const timeFormat = "2006-01-02T15:04:05Z"

type renderBuilder struct {
	strings.Builder
	limits       RenderLimits
	maxBytes     int64
	lines        int64
	err          error
	tooLargeCode ErrorCode
}

func (b *renderBuilder) line(s string) {
	if b.err != nil {
		return
	}
	if b.lines >= b.limits.MaxRenderedLines {
		b.err = errCode(CodeReportLimit, "render", "lines", "rendered line limit exceeded", nil)
		return
	}
	b.WriteString(s)
	b.WriteByte('\n')
	b.lines++
	if int64(b.Len()) > b.maxBytes {
		code := b.tooLargeCode
		if code == "" {
			code = CodeRenderFailed
		}
		b.err = errCode(code, "render", "bytes", "rendered output exceeds limit", nil)
	}
}

func RenderTerminal(ctx context.Context, report *FrozenReport, limits RenderLimits) (RenderedOutput, error) {
	if err := ctx.Err(); err != nil {
		return RenderedOutput{}, contextErr("terminal", err)
	}
	l, err := validateRenderLimits(limits)
	if err != nil {
		return RenderedOutput{}, err
	}
	if report == nil {
		return RenderedOutput{}, errCode(CodeInvalidReportModel, "terminal", "report", "FrozenReport is required", nil)
	}
	doc := report.Document()
	if err := validateRenderEvidenceRefs(doc, l); err != nil {
		return RenderedOutput{}, err
	}
	var b renderBuilder
	b.limits = l
	b.maxBytes = l.MaxTerminalBytes
	b.tooLargeCode = CodeTerminalTooLarge
	b.line("GLASSROOT REPORT")
	b.termKV("report-digest", string(report.Digest()))
	b.termKV("run-id", doc.RunID)
	b.termKV("overall-effective-disposition", string(doc.Policy.OverallEffectiveDisposition))
	b.line("")
	b.line("NOTICES")
	for _, n := range doc.Notices {
		b.termKV(string(n.Code), n.Text)
	}
	b.line("")
	b.line("RUNNER")
	b.termKV("name", doc.Runner.Name)
	b.termKV("version", doc.Runner.Version)
	b.termKV("isolation-tier", string(doc.Runner.IsolationTier))
	b.termKV("executes-target-code", boolText(doc.Runner.ExecutesTargetCode))
	b.termKV("synthetic-evidence", boolText(doc.Runner.SyntheticEvidence))
	b.termKV("enforces-network-deny", boolText(doc.Runner.EnforcesNetworkDeny))
	b.line("")
	b.line("COMPLETENESS")
	b.termKV("execution-complete", boolText(doc.Completeness.ExecutionComplete))
	b.termKV("evidence-complete", boolText(doc.Completeness.EvidenceComplete))
	b.termKV("bundle-transaction-valid", boolText(doc.Completeness.BundleTransactionValid))
	b.termKV("manifest-verification", doc.ManifestVerificationMode)
	b.line("")
	b.line("FINDINGS")
	for _, f := range doc.Policy.AppliedFindings {
		if err := b.terminalFinding(f); err != nil {
			return RenderedOutput{}, err
		}
	}
	b.line("")
	b.line("DELTAS")
	for _, r := range doc.Behavior.Records {
		if err := b.terminalDelta(r); err != nil {
			return RenderedOutput{}, err
		}
	}
	b.line("")
	b.line("LIMITATIONS")
	for _, lim := range doc.Limitations {
		b.termKV("limitation", lim.ID+" "+lim.Summary)
	}
	if err := b.err; err != nil {
		return RenderedOutput{}, err
	}
	out := strings.TrimRight(b.String(), "\n") + "\n"
	if !terminalSafe(out) {
		return RenderedOutput{}, errCode(CodeRenderFailed, "terminal", "controls", "terminal output contains controls", nil)
	}
	return RenderedOutput{RendererVersion: TerminalRendererVersionV1Alpha1, Bytes: []byte(out), Digest: digestBytes(terminalDigestDomain, []byte(out))}, nil
}

func (b *renderBuilder) termKV(label, value string) { b.line(label + ": " + b.termValue(value)) }
func (b *renderBuilder) termValue(value string) string {
	escaped, err := visibleDisplayBytes([]byte(value), b.limits)
	if err != nil {
		b.err = err
		return ""
	}
	return escaped
}

func (b *renderBuilder) terminalFinding(f AppliedFindingReport) error {
	b.termKV("finding", f.Original.ID)
	b.termKV("origin", string(f.Origin))
	b.termKV("rule", f.Original.RuleID+" "+f.Original.RuleVersion)
	b.termKV("title", f.Original.Title)
	b.termKV("severity", string(f.Original.Severity))
	b.termKV("confidence", string(f.Original.Confidence))
	b.termKV("original-disposition", string(f.Original.Disposition))
	b.termKV("effective-disposition", string(f.EffectiveDisposition))
	b.termKV("waived", boolText(f.EffectiveDisposition == model.DispositionWaived || f.Original.Waived))
	b.termKV("base-observed", boolText(f.Original.BaseObserved))
	b.termKV("head-observed", boolText(f.Original.HeadObserved))
	for _, sid := range f.Original.ScenarioIDs {
		b.termKV("scenario", sid)
	}
	for _, rid := range f.Original.DeltaRecordIDs {
		b.termKV("delta-record", rid)
	}
	if f.AppliedWaiver != nil {
		b.termKV("applied-waiver", f.AppliedWaiver.ID)
		b.termKV("waiver-owner", f.AppliedWaiver.Owner)
		b.termKV("waiver-reason", f.AppliedWaiver.Reason)
		b.termKV("waiver-issued-at", f.AppliedWaiver.IssuedAt.UTC().Format(timeFormat))
		b.termKV("waiver-expires-at", f.AppliedWaiver.ExpiresAt.UTC().Format(timeFormat))
	}
	for _, ref := range f.Original.Evidence {
		b.termKV("evidence", evidenceText(ref))
	}
	for _, lim := range f.Original.Limitations {
		b.termKV("finding-limitation", lim.ID+":"+lim.Summary)
	}
	return b.err
}
func (b *renderBuilder) terminalDelta(r DeltaRecordReport) error {
	b.termKV("delta-record", r.ID)
	b.termKV("delta-kind", string(r.Kind))
	b.termKV("fact-kind", r.FactKind)
	b.termKV("source", string(r.Source))
	b.termKV("basis", string(r.Basis))
	b.termKV("base-observed", boolText(r.BaseObserved))
	b.termKV("head-observed", boolText(r.HeadObserved))
	for _, sid := range r.ScenarioIDs {
		b.termKV("delta-scenario", sid)
	}
	b.termKV("base-occurrence", occurrenceText(r.BaseOccurrence))
	b.termKV("head-occurrence", occurrenceText(r.HeadOccurrence))
	b.termKV("evidence-reference-count", intText(int64(len(r.BaseEvidence)+len(r.HeadEvidence)+len(r.Evidence))))
	for _, lim := range r.Limitations {
		b.termKV("delta-limitation", lim.ID+":"+lim.Summary)
	}
	return b.err
}

func terminalSafe(s string) bool {
	for _, r := range s {
		if r == '\n' {
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || r == 0x2028 || r == 0x2029 {
			return false
		}
		if isFormatControl(r) {
			return false
		}
	}
	return true
}
func isFormatControl(r rune) bool {
	switch r {
	case 0x061c, 0x200b, 0x200c, 0x200d, 0x200e, 0x200f, 0x202a, 0x202b, 0x202c, 0x202d, 0x202e, 0x2060, 0x2066, 0x2067, 0x2068, 0x2069, 0xfeff:
		return true
	}
	return false
}

func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
func intText(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
func occurrenceText(o model.OccurrenceProfile) string {
	return "coverage=" + string(o.Coverage) + " repeatability=" + string(o.Repeatability) + " total=" + intText(o.TotalKnownCount) + " min=" + intText(o.MinimumKnownCount) + " max=" + intText(o.MaximumKnownCount)
}
func evidenceText(r model.EvidenceRef) string {
	return string(r.Revision) + "/" + r.ScenarioID + "/" + intText(int64(r.Repetition)) + " seq=" + intText(int64(r.EventSequence)) + " event=" + strings.Join(r.EventIDs, ",") + " stream=" + r.EventStreamPath + " digest=" + string(r.EventStreamDigest)
}

func validateRenderEvidenceRefs(doc Document, limits RenderLimits) error {
	var total int64
	for _, f := range doc.Policy.AppliedFindings {
		total += int64(len(f.Original.Evidence))
		if total > limits.MaxEvidenceRefsTotal {
			return errCode(CodeEvidenceReferenceLimit, "render", "evidence", "render evidence reference limit exceeded", nil)
		}
	}
	for _, r := range doc.Behavior.Records {
		total += int64(len(r.Evidence) + len(r.BaseEvidence) + len(r.HeadEvidence))
		if total > limits.MaxEvidenceRefsTotal {
			return errCode(CodeEvidenceReferenceLimit, "render", "evidence", "render evidence reference limit exceeded", nil)
		}
	}
	return nil
}
