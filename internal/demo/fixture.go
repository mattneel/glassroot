package demo

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"

	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/pipeline"
	"github.com/mattneel/glassroot/internal/report"
	"github.com/mattneel/glassroot/internal/runner"
	"github.com/mattneel/glassroot/internal/runner/fake"
)

const demoPipelineYAML = `apiVersion: glassroot.dev/v1alpha1
kind: Pipeline
metadata:
  name: default
spec:
  environment:
    image: ghcr.io/glassroot/fake-demo@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    workdir: /workspace
  resources:
    cpu: 2
    memory: 1GiB
    disk: 2GiB
    processes: 64
    timeout: 5m
  network:
    mode: deny
    allow: []
  scenarios:
    - id: install
      name: Synthetic install check
      shell: /bin/sh
      run: echo glassroot-demo-fake-canary
      timeout: 2m
  collect:
    filesystem:
      roots:
        - /workspace
      contents: metadata-and-digests
    artifacts:
      - path: /workspace/out/**
        maxBytes: 1MiB
      - path: /workspace/bin/**
        maxBytes: 1MiB
    logs:
      maxBytesPerStream: 1MiB
  compare:
    ignore:
      - field: event.timestamp
      - field: process.pid
    repetitions: 2
  policy:
    profile: strict
`

var (
	behaviorBaseOutput = []byte("glassroot fake demo base output\n")
	behaviorHeadOutput = []byte("glassroot fake demo head output\n")
	controlOutput      = []byte("glassroot fake demo control output\n")
	executableArtifact = []byte("inert executable artifact bytes for synthetic demo\n")
	stdoutBytes        = []byte("glassroot fake demo stdout\n")
)

func fixedTime(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t.UTC().Round(0) }

func sourceFiles(f Fixture, head bool) map[string][]byte {
	marker := "base"
	if head {
		marker = "head"
	}
	fixture := string(f)
	return map[string][]byte{
		".glassroot/pipeline.yaml": []byte(demoPipelineYAML),
		"README.md":                []byte("# Glassroot fake demo fixture\n\nThis inert fixture is never executed.\n"),
		"src/fixture.txt":          []byte("fixture=" + fixture + "\nrevision=" + marker + "\n"),
	}
}

func digestBytes(data []byte) model.Digest {
	sum := sha256.Sum256(data)
	return model.Digest("sha256:" + hex.EncodeToString(sum[:]))
}
func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }

func buildFakeProgram(plan *pipeline.FrozenPlan, fixture Fixture) (fake.Program, error) {
	attempts, err := runner.ExpandPlanAttempts(plan)
	if err != nil {
		return fake.Program{}, err
	}
	scripts := make([]fake.AttemptScript, 0, len(attempts))
	for _, a := range attempts {
		var events []fake.SyntheticEvent
		switch fixture {
		case FixtureBehaviorChange:
			events = behaviorEvents(a.Revision)
		case FixtureControl:
			events = controlEvents()
		default:
			return fake.Program{}, errCode(CodeInvalidFixture, "program", "unknown fixture", nil)
		}
		scripts = append(scripts, fake.AttemptScript{Revision: a.Revision, ScenarioID: a.ScenarioID, Repetition: a.Repetition, Events: events, Outcome: runner.AttemptOutcome{Status: runner.AttemptStatusSucceeded, ExitCode: intPtr(0), DurationMillis: 100}})
	}
	sort.SliceStable(scripts, func(i, j int) bool { return scriptKey(scripts[i]) < scriptKey(scripts[j]) })
	program := fake.Program{PlanDigest: plan.Digest(), Attempts: scripts}
	if err := validateFakeProgramCoverage(plan, program); err != nil {
		return fake.Program{}, err
	}
	return program, nil
}

func scriptKey(s fake.AttemptScript) string {
	return string(s.Revision) + "\x00" + s.ScenarioID + "\x00" + strconvUint(uint64(s.Repetition))
}
func strconvUint(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func ValidateFakeProgramCoverageForTest(reps, scripts, mode int) error {
	if reps <= 0 || reps > 10 {
		return errCode(CodeFakeProgramInvalid, "program", "invalid repetitions", nil)
	}
	expected := reps * 2
	if scripts != expected {
		return errCode(CodeFakeProgramInvalid, "program", "script coverage mismatch", nil)
	}
	return nil
}

func validateFakeProgramCoverage(plan *pipeline.FrozenPlan, program fake.Program) error {
	attempts, err := runner.ExpandPlanAttempts(plan)
	if err != nil {
		return err
	}
	if len(attempts) != len(program.Attempts) {
		return errCode(CodeFakeProgramInvalid, "program", "attempt coverage mismatch", nil)
	}
	need := map[string]bool{}
	for _, a := range attempts {
		need[string(a.Revision)+"\x00"+a.ScenarioID+"\x00"+strconvUint(uint64(a.Repetition))] = false
	}
	for _, s := range program.Attempts {
		k := scriptKey(s)
		if _, ok := need[k]; !ok {
			return errCode(CodeFakeProgramInvalid, "program", "unexpected attempt script", nil)
		}
		if need[k] {
			return errCode(CodeFakeProgramInvalid, "program", "duplicate attempt script", nil)
		}
		need[k] = true
	}
	for _, seen := range need {
		if !seen {
			return errCode(CodeFakeProgramInvalid, "program", "missing attempt script", nil)
		}
	}
	return nil
}

func behaviorEvents(rev model.RevisionKind) []fake.SyntheticEvent {
	parent := int64(100)
	child := int64(200)
	outBytes := behaviorBaseOutput
	if rev == model.RevisionKindHead {
		outBytes = behaviorHeadOutput
	}
	outDigest := digestBytes(outBytes)
	exeDigest := digestBytes(executableArtifact)
	ev := []fake.SyntheticEvent{
		event(10, processStart(parent, nil, "/workspace/bin/demo-parent", []string{"--fixture", "behavior-change"})),
		event(20, fsWrite("/workspace/out/result.txt", outDigest, int64(len(outBytes)), false, "0644")),
		event(30, artifact("create", "artifact-output", "/workspace/out/result.txt", outDigest, int64(len(outBytes)), false)),
	}
	if rev == model.RevisionKindHead {
		ev = append(ev,
			event(40, processStart(child, &parent, "/workspace/bin/demo-helper", []string{})),
			event(50, fsWrite("/workspace/bin/demo-helper", exeDigest, int64(len(executableArtifact)), true, "0755")),
			event(60, networkDenied()),
			event(70, artifact("create", "artifact-executable", "/workspace/bin/demo-helper", exeDigest, int64(len(executableArtifact)), true)),
			event(80, processExit(child, 0, 40)),
		)
	}
	ev = append(ev, event(90, processExit(parent, 0, 80)))
	return ev
}

func controlEvents() []fake.SyntheticEvent {
	parent := int64(100)
	d := digestBytes(controlOutput)
	return []fake.SyntheticEvent{
		event(10, processStart(parent, nil, "/workspace/bin/demo-parent", []string{"--fixture", "control"})),
		event(20, fsWrite("/workspace/out/result.txt", d, int64(len(controlOutput)), false, "0644")),
		event(30, artifact("create", "artifact-output", "/workspace/out/result.txt", d, int64(len(controlOutput)), false)),
		event(90, processExit(parent, 0, 80)),
	}
}

func event(offset int64, draft runner.EventDraft) fake.SyntheticEvent {
	return fake.SyntheticEvent{OffsetMillis: offset, Draft: draft}
}
func processStart(pid int64, ppid *int64, exe string, args []string) runner.EventDraft {
	outArgs := append([]string{}, args...)
	return runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: pid, ParentProcessID: ppid, ExecutablePath: exe, Arguments: outArgs, Environment: []model.EnvEntry{}}}
}
func processExit(pid int64, code int, dur int64) runner.EventDraft {
	c := code
	return runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindProcessExit, Process: &model.ProcessObservation{Operation: "exit", ProcessID: pid, ExecutablePath: "/workspace/bin/demo-parent", Arguments: []string{}, Environment: []model.EnvEntry{}, ExitCode: &c, DurationMillis: dur}}
}
func fsWrite(path string, digest model.Digest, size int64, executable bool, mode string) runner.EventDraft {
	return runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindFilesystemWrite, Filesystem: &model.FilesystemObservation{Operation: "write", Path: path, Mode: mode, Digest: digest, SizeBytes: size, Executable: executable, Truncated: false}}
}
func networkDenied() runner.EventDraft {
	return runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindNetworkConnection, Network: &model.NetworkObservation{Operation: "connect", Protocol: "tcp", DestinationHost: "canary.invalid", DestinationPort: 443, ResolvedAddresses: []string{}, Result: "denied", DurationMillis: 1}}
}
func artifact(op, id, path string, digest model.Digest, size int64, executable bool) runner.EventDraft {
	return runner.EventDraft{Source: model.ObservationSourceSyntheticTestGenerated, Kind: model.ObservationKindArtifactActivity, Artifact: &model.ArtifactObservation{Operation: op, ArtifactID: id, Path: path, Digest: digest, SizeBytes: size, Executable: executable, SourceEventIDs: []string{}}}
}

func artifactBytesFor(f Fixture, rev model.RevisionKind) [][]byte {
	var out [][]byte
	switch f {
	case FixtureBehaviorChange:
		if rev == model.RevisionKindBase {
			out = append(out, behaviorBaseOutput)
		} else {
			out = append(out, behaviorHeadOutput, executableArtifact)
		}
	case FixtureControl:
		out = append(out, controlOutput)
	}
	return out
}
func artifactPathsFor(f Fixture, rev model.RevisionKind) []string {
	switch f {
	case FixtureBehaviorChange:
		if rev == model.RevisionKindBase {
			return []string{"/workspace/out/result.txt"}
		}
		return []string{"/workspace/out/result.txt", "/workspace/bin/demo-helper"}
	case FixtureControl:
		return []string{"/workspace/out/result.txt"}
	default:
		return nil
	}
}

func keyEvidenceFromReport(doc report.Document) []KeyEvidenceRecord {
	categories := []struct{ rule, cat string }{{"GR-PROC-001", "new-child-process"}, {"GR-NET-001", "denied-network-attempt"}, {"GR-FS-001", "executable-artifact"}, {"GR-ART-001", "changed-artifact"}}
	var out []KeyEvidenceRecord
	seen := map[string]bool{}
	for _, want := range categories {
		for _, f := range doc.Policy.AppliedFindings {
			if f.Original.RuleID != want.rule || seen[want.rule] {
				continue
			}
			rec := KeyEvidenceRecord{Category: want.cat, RuleID: f.Original.RuleID, FindingID: f.Original.ID, DeltaRecordIDs: append([]string(nil), f.Original.DeltaRecordIDs...), EventIDs: []string{}}
			if len(f.Original.Evidence) > 0 {
				er := f.Original.Evidence[0]
				rec.EventIDs = append([]string(nil), er.EventIDs...)
				rec.EventStreamDigest = er.EventStreamDigest
				rec.EventStreamPath = er.EventStreamPath
				rec.Revision = er.Revision
				rec.ScenarioID = er.ScenarioID
				rec.Repetition = er.Repetition
			}
			out = append(out, rec)
			seen[want.rule] = true
		}
	}
	if out == nil {
		out = []KeyEvidenceRecord{}
	}
	return out
}
