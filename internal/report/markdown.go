package report

import (
	"context"
	"strings"

	"github.com/mattneel/glassroot/internal/model"
)

func RenderMarkdown(ctx context.Context, report *FrozenReport, limits RenderLimits) (RenderedOutput, error) {
	if err := ctx.Err(); err != nil {
		return RenderedOutput{}, contextErr("markdown", err)
	}
	l, err := validateRenderLimits(limits)
	if err != nil {
		return RenderedOutput{}, err
	}
	if report == nil {
		return RenderedOutput{}, errCode(CodeInvalidReportModel, "markdown", "report", "FrozenReport is required", nil)
	}
	doc := report.Document()
	if err := validateRenderEvidenceRefs(doc, l); err != nil {
		return RenderedOutput{}, err
	}
	var b renderBuilder
	b.limits = l
	b.maxBytes = l.MaxMarkdownBytes
	b.tooLargeCode = CodeMarkdownTooLarge
	b.heading("# Glassroot report")
	b.kv("Report digest", string(report.Digest()))
	b.kv("Run ID", doc.RunID)
	b.kv("Overall effective disposition", string(doc.Policy.OverallEffectiveDisposition))
	b.kv("Manifest verification", doc.ManifestVerificationMode)
	b.heading("## Notices")
	if len(doc.Notices) == 0 {
		b.line("- none")
	}
	for _, n := range doc.Notices {
		b.listKV(string(n.Code), n.Text)
	}
	b.heading("## Runner")
	b.kv("Name", doc.Runner.Name)
	b.kv("Version", doc.Runner.Version)
	b.kv("Isolation tier", string(doc.Runner.IsolationTier))
	b.kv("Executes target code", boolText(doc.Runner.ExecutesTargetCode))
	b.kv("Synthetic evidence", boolText(doc.Runner.SyntheticEvidence))
	b.kv("Enforces network deny", boolText(doc.Runner.EnforcesNetworkDeny))
	b.heading("## Completeness")
	b.kv("Execution complete", boolText(doc.Completeness.ExecutionComplete))
	b.kv("Evidence complete", boolText(doc.Completeness.EvidenceComplete))
	b.kv("Bundle transaction valid", boolText(doc.Completeness.BundleTransactionValid))
	b.heading("## Findings")
	b.kv("Total findings", intText(doc.Policy.Summary.TotalFindings))
	for _, f := range doc.Policy.AppliedFindings {
		if err := b.finding(f); err != nil {
			return RenderedOutput{}, err
		}
	}
	b.heading("## Behavioral delta")
	b.kv("Total delta records", intText(doc.Behavior.Summary.TotalRecords))
	for _, r := range doc.Behavior.Records {
		if err := b.deltaRecord(r); err != nil {
			return RenderedOutput{}, err
		}
	}
	b.heading("## Authorities")
	b.kv("Trusted config path", doc.Policy.TrustedConfigAuthority.Path)
	b.kv("Trusted config head state", string(doc.Policy.TrustedConfigAuthority.HeadState))
	b.kv("Trusted waiver path", doc.Policy.TrustedWaiverAuthority.Path)
	b.kv("Trusted waiver base state", string(doc.Policy.TrustedWaiverAuthority.BaseState))
	b.kv("Trusted waiver head state", string(doc.Policy.TrustedWaiverAuthority.HeadState))
	b.heading("## Limitations")
	for _, lim := range doc.Limitations {
		b.listKV(lim.ID, lim.Summary)
	}
	b.heading("## Safety statement")
	b.line("A passed disposition does not prove the code is safe. Report digests are deterministic equality values only.")
	if err := b.err; err != nil {
		return RenderedOutput{}, err
	}
	out := b.String()
	out = strings.TrimRight(out, "\n") + "\n"
	if strings.Contains(out, "\r") {
		return RenderedOutput{}, errCode(CodeRenderFailed, "markdown", "newline", "markdown contains carriage return", nil)
	}
	return RenderedOutput{RendererVersion: MarkdownRendererVersionV1Alpha1, Bytes: []byte(out), Digest: digestBytes(markdownDigestDomain, []byte(out))}, nil
}

func (b *renderBuilder) heading(s string) {
	if b.Len() > 0 {
		b.line("")
	}
	b.line(s)
}
func (b *renderBuilder) kv(label, value string) { b.line("- " + label + ": " + b.mdValue(value)) }
func (b *renderBuilder) listKV(label, value string) {
	b.line("- " + b.mdValue(label) + ": " + b.mdValue(value))
}

func (b *renderBuilder) mdValue(value string) string {
	escaped, err := visibleDisplayBytes([]byte(value), b.limits)
	if err != nil {
		b.err = err
		return "``"
	}
	return markdownCodeSpan(escaped)
}

func (b *renderBuilder) finding(f AppliedFindingReport) error {
	b.heading("### Finding")
	b.kv("Finding ID", f.Original.ID)
	b.kv("Origin", string(f.Origin))
	b.kv("Rule", f.Original.RuleID+" "+f.Original.RuleVersion)
	b.kv("Title", f.Original.Title)
	b.kv("Severity", string(f.Original.Severity))
	b.kv("Confidence", string(f.Original.Confidence))
	b.kv("Original disposition", string(f.Original.Disposition))
	b.kv("Effective disposition", string(f.EffectiveDisposition))
	b.kv("Waived", boolText(f.EffectiveDisposition == model.DispositionWaived || f.Original.Waived))
	b.kv("Base observed", boolText(f.Original.BaseObserved))
	b.kv("Head observed", boolText(f.Original.HeadObserved))
	for _, sid := range f.Original.ScenarioIDs {
		b.listKV("scenario", sid)
	}
	for _, rid := range f.Original.DeltaRecordIDs {
		b.listKV("delta record", rid)
	}
	if f.AppliedWaiver != nil {
		b.kv("Applied waiver", f.AppliedWaiver.ID)
		b.kv("Waiver owner", f.AppliedWaiver.Owner)
		b.kv("Waiver reason", f.AppliedWaiver.Reason)
		b.kv("Waiver issued at", f.AppliedWaiver.IssuedAt.UTC().Format(timeFormat))
		b.kv("Waiver expires at", f.AppliedWaiver.ExpiresAt.UTC().Format(timeFormat))
	}
	for _, ref := range f.Original.Evidence {
		b.evidenceRef("evidence", ref)
	}
	for _, lim := range f.Original.Limitations {
		b.listKV("limitation "+lim.ID, lim.Summary)
	}
	return b.err
}

func (b *renderBuilder) deltaRecord(r DeltaRecordReport) error {
	if err := validateDeltaKind(r.Kind); err != nil {
		return err
	}
	b.heading("### Delta record")
	b.kv("Delta record ID", r.ID)
	b.kv("Kind", string(r.Kind))
	b.kv("Fact kind", r.FactKind)
	b.kv("Source", string(r.Source))
	b.kv("Basis", string(r.Basis))
	b.kv("Base observed", boolText(r.BaseObserved))
	b.kv("Head observed", boolText(r.HeadObserved))
	b.kv("Base occurrence", occurrenceText(r.BaseOccurrence))
	b.kv("Head occurrence", occurrenceText(r.HeadOccurrence))
	for _, f := range r.BaseFacts {
		if err := b.fact("base fact", f); err != nil {
			return err
		}
	}
	for _, f := range r.HeadFacts {
		if err := b.fact("head fact", f); err != nil {
			return err
		}
	}
	for _, cf := range r.ChangedFields {
		b.listKV("changed field", cf)
	}
	for _, ref := range r.BaseEvidence {
		b.evidenceRef("base evidence", ref)
	}
	for _, ref := range r.HeadEvidence {
		b.evidenceRef("head evidence", ref)
	}
	for _, lim := range r.Limitations {
		b.listKV("limitation "+lim.ID, lim.Summary)
	}
	return b.err
}

func (b *renderBuilder) fact(label string, f model.DeltaFactSnapshot) error {
	b.listKV(label, f.Kind+" "+string(f.SemanticDigest))
	switch f.Kind {
	case "process-start", "process-exit":
		if f.Process == nil {
			return errCode(CodeUnsupportedFactKind, "markdown", "process", "missing process payload", nil)
		}
		b.listKV("process operation", f.Process.Operation)
		b.listKV("process id", f.Process.StableID)
		b.listKV("executable", f.Process.Executable.Display)
	case "filesystem-create", "filesystem-read", "filesystem-write", "filesystem-delete", "filesystem-rename", "filesystem-chmod":
		if f.Filesystem == nil {
			return errCode(CodeUnsupportedFactKind, "markdown", "filesystem", "missing filesystem payload", nil)
		}
		b.listKV("filesystem operation", f.Filesystem.Operation)
		b.listKV("filesystem path", f.Filesystem.Path.Display)
		b.listKV("path namespace", f.Filesystem.Path.Namespace)
		b.listKV("executable", boolText(f.Filesystem.Executable))
	case "dns-query", "network-connection":
		if f.Network == nil {
			return errCode(CodeUnsupportedFactKind, "markdown", "network", "missing network payload", nil)
		}
		b.listKV("network operation", f.Network.Operation)
		b.listKV("network destination", f.Network.DestinationHost)
		b.listKV("network result", f.Network.Result)
	case "artifact-activity":
		if f.Artifact == nil {
			return errCode(CodeUnsupportedFactKind, "markdown", "artifact", "missing artifact payload", nil)
		}
		b.listKV("artifact path", f.Artifact.Path.Display)
		b.listKV("artifact digest", string(f.Artifact.Digest))
		b.listKV("artifact executable", boolText(f.Artifact.Executable))
	case "scenario-started", "scenario-completed":
		if f.Scenario == nil {
			return errCode(CodeUnsupportedFactKind, "markdown", "scenario", "missing scenario payload", nil)
		}
		b.listKV("scenario status", string(f.Scenario.Status))
	case "observer-warning", "unsupported-observation":
		if f.Warning == nil {
			return errCode(CodeUnsupportedFactKind, "markdown", "warning", "missing warning payload", nil)
		}
		b.listKV("warning code", f.Warning.Code)
		b.listKV("unsupported", boolText(f.Warning.Unsupported))
	case "resource-limit":
		if f.Resource == nil {
			return errCode(CodeUnsupportedFactKind, "markdown", "resource", "missing resource payload", nil)
		}
		b.listKV("resource kind", f.Resource.LimitKind)
		b.listKV("resource exceeded", boolText(f.Resource.Exceeded))
	default:
		return errCode(CodeUnsupportedFactKind, "markdown", "factKind", "unsupported fact kind", nil)
	}
	return b.err
}

func (b *renderBuilder) evidenceRef(label string, r model.EvidenceRef) {
	b.listKV(label, string(r.Revision)+"/"+r.ScenarioID+"/"+intText(int64(r.Repetition))+" seq="+intText(int64(r.EventSequence))+" event="+strings.Join(r.EventIDs, ",")+" stream="+r.EventStreamPath+" digest="+string(r.EventStreamDigest))
}
