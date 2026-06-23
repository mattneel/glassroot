package report

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/policy"
)

func TestRenderersContainHostileValuesAsInertVisibleText(t *testing.T) {
	hostile := "# heading\n> quote\n- list\n| table |\n[text](javascript:alert(1))\n[text](data:text/html,boom)\n[text](file:///etc/passwd)\n<script><img src=x onerror=alert(1)><!--x--><details>\nhttps://example.invalid/@maintainer\n``` fence ```\x1b[31m\x1b]8;;https://evil.invalid\aX\x1b]8;;\a\x1b]52;c;AAAA\a\b\r\t\x00\u202e\ufeff\u200b"
	frozen, err := freezeReportDocument(hostileReportDocument(hostile), DefaultLimits())
	if err != nil {
		t.Fatalf("freezeReportDocument() error = %v", err)
	}
	md, err := RenderMarkdown(context.Background(), frozen, DefaultRenderLimits())
	if err != nil {
		t.Fatalf("RenderMarkdown() error = %v", err)
	}
	txt, err := RenderTerminal(context.Background(), frozen, DefaultRenderLimits())
	if err != nil {
		t.Fatalf("RenderTerminal() error = %v", err)
	}
	if !utf8.Valid(md.Bytes) || !utf8.Valid(txt.Bytes) {
		t.Fatalf("renderer emitted invalid UTF-8")
	}
	if !terminalSafe(string(txt.Bytes)) {
		t.Fatalf("terminal output retained an active terminal or Unicode control: %q", txt.Bytes)
	}
	for _, raw := range [][]byte{
		[]byte("\x1b["), []byte("\x1b]"), []byte("\a"), []byte("\b"), []byte("\r"), []byte("\t"), []byte("\x00"),
		[]byte("<script>"), []byte("<img"), []byte("<!--"), []byte("<details>"), []byte("[text](javascript:"), []byte("[text](data:"), []byte("[text](file:"),
		[]byte("\u202e"), []byte("\ufeff"), []byte("\u200b"),
	} {
		if bytes.Contains(md.Bytes, raw) {
			t.Fatalf("markdown retained raw hostile fragment %q in %s", raw, md.Bytes)
		}
		if bytes.Contains(txt.Bytes, raw) {
			t.Fatalf("terminal retained raw hostile fragment %q in %s", raw, txt.Bytes)
		}
	}
	for _, visible := range []string{`\x1B`, `\x07`, `\x08`, `\x0D`, `\x09`, `\x00`, `\u{003C}script\u{003E}`, `\u{005B}text\u{005D}`, `\u{202E}`, `\u{FEFF}`, `\u{200B}`} {
		if !strings.Contains(string(md.Bytes), visible) {
			t.Fatalf("markdown missing visible hostile marker %q in %s", visible, md.Bytes)
		}
		if !strings.Contains(string(txt.Bytes), visible) {
			t.Fatalf("terminal missing visible hostile marker %q in %s", visible, txt.Bytes)
		}
	}
	if !bytes.HasSuffix(md.Bytes, []byte("\n")) || bytes.HasSuffix(md.Bytes, []byte("\n\n")) {
		t.Fatalf("markdown final newline contract violated")
	}
	if !bytes.HasSuffix(txt.Bytes, []byte("\n")) || bytes.HasSuffix(txt.Bytes, []byte("\n\n")) {
		t.Fatalf("terminal final newline contract violated")
	}
}

func hostileReportDocument(hostile string) Document {
	now := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)
	ref := model.EvidenceRef{
		EventIDs:          []string{"evt-" + strings.Repeat("a", 64), hostile},
		EventStreamDigest: model.Digest("sha256:" + strings.Repeat("1", 64)),
		EventStreamPath:   hostile,
		EventSequence:     1,
		Revision:          model.RevisionKindHead,
		ScenarioID:        "scenario",
		Repetition:        1,
	}
	finding := policy.AppliedFinding{
		Origin: policy.FindingOriginBuiltinPolicy,
		Original: model.Finding{
			SchemaVersion:  model.SchemaVersionFindingV1Alpha1,
			ID:             "finding-" + strings.Repeat("2", 64),
			RuleID:         "GR-NET-001",
			RuleVersion:    "v1alpha1",
			Title:          hostile,
			Severity:       model.SeverityHigh,
			Confidence:     model.ConfidenceLow,
			Disposition:    model.DispositionRequiresReview,
			Summary:        hostile,
			DeltaRecordIDs: []string{"delta-" + strings.Repeat("3", 64)},
			Evidence:       []model.EvidenceRef{ref},
			ScenarioIDs:    []string{"scenario"},
			HeadObserved:   true,
			Limitations:    []model.Limitation{{ID: "hostile", Summary: hostile}},
		},
		EffectiveDisposition: model.DispositionWaived,
		AppliedWaiver: &policy.AppliedWaiver{
			ID:                      "waiver-hostile",
			TargetFindingID:         "finding-" + strings.Repeat("2", 64),
			RuleID:                  "GR-NET-001",
			Owner:                   hostile,
			Reason:                  hostile,
			IssuedAt:                now,
			ExpiresAt:               now.Add(24 * time.Hour),
			BaseRawDigest:           model.Digest("sha256:" + strings.Repeat("4", 64)),
			SemanticWaiverSetDigest: model.Digest("sha256:" + strings.Repeat("5", 64)),
		},
	}
	record := model.DeltaRecord{
		ID:           "delta-" + strings.Repeat("3", 64),
		Kind:         model.DeltaKindAdded,
		FactKind:     string(model.ObservationKindNetworkConnection),
		Source:       model.ObservationSourceSyntheticTestGenerated,
		Basis:        model.ComparisonBasisCoverageLimited,
		ScenarioIDs:  []string{"scenario"},
		HeadObserved: true,
		HeadOccurrence: model.OccurrenceProfile{
			PlannedRepetitionCount:  1,
			CompleteRepetitionCount: 1,
			MinimumKnownCount:       1,
			MaximumKnownCount:       1,
			TotalKnownCount:         1,
			Coverage:                model.CoverageAssessmentComplete,
			Repeatability:           model.RepeatabilitySingleSample,
		},
		HeadFacts: []model.DeltaFactSnapshot{{
			ID:             "fact-" + strings.Repeat("6", 64),
			SemanticDigest: model.Digest("sha256:" + strings.Repeat("7", 64)),
			Kind:           string(model.ObservationKindNetworkConnection),
			Source:         model.ObservationSourceSyntheticTestGenerated,
			Network:        &model.DeltaNetworkFact{Operation: "connect", Protocol: "tcp", DestinationHost: hostile, DestinationPort: 443, Result: "denied"},
			Evidence:       []model.EvidenceRef{ref},
			Limitations:    []model.Limitation{},
		}},
		HeadEvidence: []model.EvidenceRef{ref},
		Limitations:  []model.Limitation{{ID: "hostile", Summary: hostile}},
	}
	return Document{
		SchemaVersion:                 SchemaVersionReportV1Alpha1,
		ReportProfileVersion:          ReportProfileVersionV1Alpha1,
		RunID:                         "run-hostile",
		EvaluatedAt:                   now,
		PlanDigest:                    model.Digest("sha256:" + strings.Repeat("8", 64)),
		ManifestDigest:                model.Digest("sha256:" + strings.Repeat("9", 64)),
		BehavioralDeltaDigest:         model.Digest("sha256:" + strings.Repeat("a", 64)),
		BuiltinPolicyEvaluationDigest: model.Digest("sha256:" + strings.Repeat("b", 64)),
		PolicyApplicationDigest:       model.Digest("sha256:" + strings.Repeat("c", 64)),
		ManifestVerificationMode:      "internal-consistency-only",
		Runner:                        RunnerSection{Name: "fake", Version: "v1", IsolationTier: model.IsolationTierFake, SyntheticEvidence: true},
		Completeness:                  CompletenessSection{BundleTransactionValid: true, ExecutionComplete: true, EvidenceComplete: true, SyntheticEvidence: true, AttemptCoverage: []AttemptCoverageReport{}},
		Policy: PolicySection{
			OverallEffectiveDisposition: model.DispositionWaived,
			Summary:                     policy.ApplicationSummary{TotalFindings: 1, BuiltinFindings: 1, EffectiveWaived: 1, High: 1, AppliedWaivers: 1},
			AppliedFindings:             []AppliedFindingReport{finding},
			WaiverStatuses:              []policy.WaiverStatusRecord{},
		},
		Behavior: BehaviorSection{ScenarioIDs: []string{"scenario"}, Summary: model.DeltaSummary{TotalRecords: 1, Added: 1}, Records: []DeltaRecordReport{record}},
		Notices:  []Notice{{Code: NoticePassedNotProofOfSafety, Text: noticeText(NoticePassedNotProofOfSafety)}, {Code: NoticeWaiversApplied, Text: noticeText(NoticeWaiversApplied)}},
		Limitations: []LimitationReport{
			{ID: "hostile", Summary: hostile},
		},
	}
}
