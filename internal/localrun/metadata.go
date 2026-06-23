package localrun

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/mattneel/glassroot/internal/dockerengine"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/report"
	"github.com/mattneel/glassroot/internal/runner/dockerdev"
)

func encodeMetadata(md Metadata) ([]byte, error) {
	if md.Limitations == nil {
		md.Limitations = []model.Limitation{}
	}
	if md.Daemon.Security == nil {
		md.Daemon.Security = []string{}
	}
	return json.Marshal(md)
}

func buildMetadata(req Request, plan *pipeline.FrozenPlan, manifest model.Digest, fr *report.FrozenReport, markdownDigest, terminalDigest model.Digest, disposition model.Disposition, exit, attempts int, daemon dockerengine.ServerMetadata) Metadata {
	doc := fr.Document()
	planDoc := plan.Document()
	caps := dockerdev.Capabilities()
	limitations := limitationsFromReport(doc)
	return Metadata{
		SchemaVersion:           SchemaVersionLocalRunV1Alpha1,
		LocalRunProfileVersion:  LocalRunProfileDockerDevV1Alpha1,
		PlatformProfileVersion:  PlatformProfileDockerDevV1Alpha1,
		RunID:                   doc.RunID,
		CreatedAt:               req.CreatedAt.UTC().Round(0),
		EvaluatedAt:             req.EvaluatedAt.UTC().Round(0),
		BaseCommitID:            doc.Source.Base.CommitID,
		BaseTreeID:              doc.Source.Base.TreeID,
		HeadCommitID:            doc.Source.Head.CommitID,
		HeadTreeID:              doc.Source.Head.TreeID,
		ObjectFormat:            string(doc.Source.Base.ObjectFormat),
		ImmutableImage:          planDoc.ExecutionEnvironment.Image,
		PlanDigest:              plan.Digest(),
		ManifestDigest:          manifest,
		BehavioralDeltaDigest:   doc.BehavioralDeltaDigest,
		PolicyEvaluationDigest:  doc.BuiltinPolicyEvaluationDigest,
		PolicyApplicationDigest: doc.PolicyApplicationDigest,
		ReportDigest:            fr.Digest(),
		MarkdownDigest:          markdownDigest,
		TerminalDigest:          terminalDigest,
		Runner:                  metadataRunner(caps),
		ExecutionComplete:       doc.Completeness.ExecutionComplete,
		EvidenceComplete:        doc.Completeness.EvidenceComplete,
		EffectiveDisposition:    disposition,
		ExpectedCLIExitCode:     exit,
		RelativePaths:           MetadataPaths{Evidence: "evidence", ReportJSON: "report.json", ReportMarkdown: "report.md", ReportTerminal: "report.txt"},
		Daemon:                  metadataDaemon(daemon),
		AttemptCount:            attempts,
		LimitationCount:         len(limitations),
		Limitations:             limitations,
	}
}

func metadataRunner(c model.RunnerCapabilities) MetadataRunner {
	return MetadataRunner{Name: c.Name, Version: c.Version, IsolationTier: c.IsolationTier, ExecutesTargetCode: c.ExecutesTargetCode, SyntheticEvidence: c.SyntheticEvidence, EnforcesNetworkDeny: c.EnforcesNetworkDeny, ProcessEvents: c.ProcessEventCollection, FilesystemEvents: c.FilesystemEventCollection, SyscallEvents: c.SyscallEventCollection, ArtifactHashing: c.ArtifactHashing, SnapshotSupport: c.SnapshotSupport}
}

func metadataDaemon(d dockerengine.ServerMetadata) MetadataDaemon {
	security := append([]string(nil), d.SecurityOptions...)
	sort.Strings(security)
	return MetadataDaemon{EngineVersion: d.EngineVersion, APIVersion: d.APIVersion, OSType: d.OSType, Architecture: d.Architecture, CgroupVersion: d.CgroupVersion, CgroupDriver: d.CgroupDriver, Rootless: d.Rootless, Security: security}
}

func limitationsFromReport(doc report.Document) []model.Limitation {
	out := make([]model.Limitation, len(doc.Limitations))
	for i, lim := range doc.Limitations {
		out[i] = model.Limitation(lim)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		if out[i].Summary != out[j].Summary {
			return out[i].Summary < out[j].Summary
		}
		return out[i].Details < out[j].Details
	})
	return out
}

func exitCodeForDisposition(d model.Disposition) int {
	switch d {
	case model.DispositionPassed:
		return 0
	case model.DispositionRequiresReview:
		return 4
	case model.DispositionFailed:
		return 5
	default:
		return 3
	}
}

func fixedUTC(t time.Time) time.Time { return t.UTC().Round(0) }
