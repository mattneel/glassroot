package dockerdev

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/dockerengine"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

func TestAcknowledgementRequired(t *testing.T) {
	if _, err := AcknowledgeUnsafeDevelopmentRunner(UnsafeDevelopmentAcknowledgementText); err != nil {
		t.Fatalf("correct acknowledgement rejected: %v", err)
	}
	if _, err := AcknowledgeUnsafeDevelopmentRunner(""); err == nil {
		t.Fatalf("empty acknowledgement accepted")
	}
	eng := newFakeEngine()
	_, err := New(Config{Engine: eng, Acknowledgement: UnsafeDevelopmentAcknowledgement{}, Limits: DefaultLimits(), Workspaces: []WorkspaceBinding{binding(t, "att-base-test-r1")}})
	if err == nil {
		t.Fatalf("runner constructed without acknowledgement")
	}
}

func TestCapabilitiesAndRequirements(t *testing.T) {
	r := newRunnerForTest(t, []WorkspaceBinding{binding(t, "att-base-test-r1")})
	caps, err := r.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if caps.Name != "docker-dev" || caps.Version != "v1" || caps.IsolationTier != model.IsolationTierDevelopmentOnly || !caps.ExecutesTargetCode || caps.SyntheticEvidence || !caps.EnforcesNetworkDeny || caps.ProcessEventCollection || caps.FilesystemEventCollection || caps.SyscallEventCollection || caps.ArtifactHashing || caps.SnapshotSupport || caps.FreshKernel || caps.BrokeredNetwork {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
	if mismatches, err := runner.MatchCapabilities(runner.WorkloadRequirements([]model.IsolationTier{model.IsolationTierHardenedContainer}), caps); err != nil || len(mismatches) == 0 {
		t.Fatalf("development-only tier was not rejected: mismatches=%+v err=%v", mismatches, err)
	}
	if mismatches, err := runner.MatchCapabilities(runner.WorkloadRequirements([]model.IsolationTier{model.IsolationTierDevelopmentOnly}), caps); err != nil || len(mismatches) != 0 {
		t.Fatalf("explicit development-only tier rejected: mismatches=%+v err=%v", mismatches, err)
	}
}

func TestValidatePlanRequiresExactWorkspaceInventory(t *testing.T) {
	attempts := []runner.AttemptRequest{attempt("att-base-test-r1"), attempt("att-head-test-r1")}
	r := newRunnerForTest(t, []WorkspaceBinding{binding(t, "att-base-test-r1")})
	if err := r.ValidatePlan(context.Background(), model.Digest("sha256:"+strings.Repeat("a", 64)), attempts, runner.DefaultLimits()); err == nil {
		t.Fatalf("missing workspace binding accepted")
	}
	r = newRunnerForTest(t, []WorkspaceBinding{binding(t, "att-base-test-r1"), binding(t, "att-head-test-r1"), binding(t, "att-extra")})
	if err := r.ValidatePlan(context.Background(), model.Digest("sha256:"+strings.Repeat("a", 64)), attempts, runner.DefaultLimits()); err == nil {
		t.Fatalf("extra workspace binding accepted")
	}
}

func TestWorkspaceValidationRejectsSharingAndOverlap(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(a, "child")
	if err := os.MkdirAll(b, 0o700); err != nil {
		t.Fatal(err)
	}
	ack, _ := AcknowledgeUnsafeDevelopmentRunner(UnsafeDevelopmentAcknowledgementText)
	_, err := New(Config{Engine: newFakeEngine(), Acknowledgement: ack, Limits: DefaultLimits(), Workspaces: []WorkspaceBinding{{AttemptID: "att-a", HostPath: a}, {AttemptID: "att-b", HostPath: b}}})
	if err == nil {
		t.Fatalf("ancestor workspace overlap accepted")
	}
	_, err = New(Config{Engine: newFakeEngine(), Acknowledgement: ack, Limits: DefaultLimits(), Workspaces: []WorkspaceBinding{{AttemptID: "att-a", HostPath: a}, {AttemptID: "att-b", HostPath: a}}})
	if err == nil {
		t.Fatalf("duplicate workspace accepted")
	}
}

func TestRunAttemptBuildsStrictContainerConfig(t *testing.T) {
	eng := newFakeEngine()
	r := newRunnerForTestWithEngine(t, eng, []WorkspaceBinding{binding(t, "att-base-test-r1")})
	sink := &captureSink{}
	out, err := r.RunAttempt(context.Background(), attempt("att-base-test-r1"), sink)
	if err != nil {
		t.Fatalf("RunAttempt failed: %v", err)
	}
	if out.Status != runner.AttemptStatusSucceeded || out.ExitCode == nil || *out.ExitCode != 0 {
		t.Fatalf("bad outcome: %+v", out)
	}
	spec := eng.created
	if spec.Image != attempt("att-base-test-r1").Image || len(spec.Entrypoint) != 0 || len(spec.Command) != 3 || spec.Command[0] != "/bin/sh" || spec.Command[1] != "-c" || spec.Command[2] != "printf 'hi; rm -rf /'" {
		t.Fatalf("command not exact argv: %+v", spec)
	}
	if spec.Workdir != "/workspace" || spec.Hostname != "glassroot" || spec.NetworkMode != "none" || !spec.ReadOnlyRootfs || spec.Privileged || !spec.NoNewPrivileges || !spec.Init || spec.AutoRemove || spec.TTY || spec.OpenStdin {
		t.Fatalf("unsafe container booleans: %+v", spec)
	}
	if len(spec.CapDrop) != 1 || spec.CapDrop[0] != "ALL" || len(spec.CapAdd) != 0 || len(spec.Binds) != 1 || !strings.HasSuffix(spec.Binds[0].HostPath, string(filepath.Separator)+"att-base-test-r1") || spec.Binds[0].ContainerPath != "/workspace" || !spec.Binds[0].ReadWrite {
		t.Fatalf("bad bind/caps: %+v", spec)
	}
	if spec.Resources.MemoryBytes != 64<<20 || spec.Resources.PidsLimit != 32 || spec.Resources.NanoCPUs == 0 || spec.LogDriver != "none" || len(spec.Devices) != 0 || len(spec.PublishedPorts) != 0 {
		t.Fatalf("bad resources/log/network/device settings: %+v", spec)
	}
	if len(sink.events) != 3 || sink.events[0].Kind != model.ObservationKindObserverWarning || sink.events[1].Kind != model.ObservationKindProcessStart || sink.events[2].Kind != model.ObservationKindProcessExit {
		t.Fatalf("unexpected events: %+v", sink.events)
	}
}

func TestRunAttemptStreamsOutputAndHandlesLimits(t *testing.T) {
	eng := newFakeEngine()
	eng.attach = muxFrame(byte(1), []byte("stdout"), byte(2), []byte("stderr"))
	limits := DefaultLimits()
	limits.MaxStdoutBytes = 3
	limits.MaxStderrBytes = 4
	r := newRunnerForTestWithLimits(t, eng, []WorkspaceBinding{binding(t, "att-base-test-r1")}, limits)
	logSink := &logSink{}
	out, err := r.RunAttemptWithOutput(context.Background(), attempt("att-base-test-r1"), &captureSink{}, logSink)
	if err != nil {
		t.Fatalf("RunAttemptWithOutput failed: %v", err)
	}
	if !containsLimitation(out.Limitations, "stdout-truncated") || !containsLimitation(out.Limitations, "stderr-truncated") {
		t.Fatalf("truncation limitations missing: %+v", out.Limitations)
	}
	if string(logSink.stdout.Bytes()) != "std" || string(logSink.stderr.Bytes()) != "stde" {
		t.Fatalf("bad bounded logs stdout=%q stderr=%q", logSink.stdout.String(), logSink.stderr.String())
	}
}

func TestRunAttemptSinkFailureCleansContainer(t *testing.T) {
	eng := newFakeEngine()
	eng.attach = muxFrame(byte(1), []byte("hello"))
	boom := errors.New("log sink failed")
	r := newRunnerForTestWithEngine(t, eng, []WorkspaceBinding{binding(t, "att-base-test-r1")})
	_, err := r.RunAttemptWithOutput(context.Background(), attempt("att-base-test-r1"), &captureSink{}, failingLogSink{err: boom})
	if !errors.Is(err, boom) {
		t.Fatalf("sink failure not preserved: %v", err)
	}
	if !eng.removed || !eng.killed {
		t.Fatalf("container not killed/removed after sink failure: killed=%v removed=%v", eng.killed, eng.removed)
	}
}

func TestRunAttemptOutcomeMapping(t *testing.T) {
	for _, tc := range []struct {
		name string
		wait dockerengine.WaitResult
		want runner.AttemptStatus
	}{
		{"nonzero", dockerengine.WaitResult{ExitCode: 2}, runner.AttemptStatusFailed},
		{"oom", dockerengine.WaitResult{ExitCode: 137, OOMKilled: true}, runner.AttemptStatusResourceLimited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eng := newFakeEngine()
			eng.wait = tc.wait
			r := newRunnerForTestWithEngine(t, eng, []WorkspaceBinding{binding(t, "att-base-test-r1")})
			out, err := r.RunAttempt(context.Background(), attempt("att-base-test-r1"), &captureSink{})
			if err != nil {
				t.Fatalf("RunAttempt failed: %v", err)
			}
			if out.Status != tc.want {
				t.Fatalf("status=%s want %s", out.Status, tc.want)
			}
		})
	}
}

func binding(t *testing.T, id string) WorkspaceBinding {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, id)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return WorkspaceBinding{AttemptID: id, Revision: model.RevisionKindBase, ScenarioID: "test", Repetition: 1, HostPath: path, MaterializedTreeDigest: model.Digest("sha256:" + strings.Repeat("b", 64))}
}

func attempt(id string) runner.AttemptRequest {
	return runner.AttemptRequest{PlanDigest: model.Digest("sha256:" + strings.Repeat("a", 64)), RunID: "run", PlanCreatedAt: time.Unix(1000, 0).UTC(), AttemptID: id, GlobalOrdinal: 1, Revision: model.RevisionKindBase, Image: "registry.example.invalid/dev@sha256:" + strings.Repeat("c", 64), Workdir: "/workspace", Shell: "/bin/sh", Run: "printf 'hi; rm -rf /'", ScenarioID: "test", Repetition: 1, ResourceLimits: model.ResourceLimits{CPU: 1, MemoryBytes: 64 << 20, ProcessCount: 32, TimeoutMillis: 1000}, NetworkPolicy: model.NetworkPolicy{Mode: model.NetworkModeDeny}}
}

func newRunnerForTest(t *testing.T, bindings []WorkspaceBinding) *Runner {
	return newRunnerForTestWithEngine(t, newFakeEngine(), bindings)
}
func newRunnerForTestWithEngine(t *testing.T, eng *fakeEngine, bindings []WorkspaceBinding) *Runner {
	return newRunnerForTestWithLimits(t, eng, bindings, DefaultLimits())
}
func newRunnerForTestWithLimits(t *testing.T, eng *fakeEngine, bindings []WorkspaceBinding, limits Limits) *Runner {
	t.Helper()
	ack, err := AcknowledgeUnsafeDevelopmentRunner(UnsafeDevelopmentAcknowledgementText)
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(Config{Engine: eng, Acknowledgement: ack, Limits: limits, Workspaces: bindings})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type fakeEngine struct {
	created dockerengine.ContainerSpec
	attach  []byte
	wait    dockerengine.WaitResult
	removed bool
	killed  bool
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{attach: muxFrame(byte(1), []byte("ok\n")), wait: dockerengine.WaitResult{ExitCode: 0}}
}
func (f *fakeEngine) Metadata() dockerengine.ServerMetadata {
	return dockerengine.ServerMetadata{OSType: "linux", Architecture: "amd64", EngineVersion: "28.5.2", APIVersion: "1.55", CgroupVersion: "2", CgroupDriver: "systemd", SecurityOptions: []string{"name=seccomp"}}
}
func (f *fakeEngine) InspectImage(context.Context, string) (dockerengine.ImageMetadata, error) {
	return dockerengine.ImageMetadata{ID: "sha256:" + strings.Repeat("d", 64), RepoDigests: []string{attempt("x").Image}, OSType: "linux"}, nil
}
func (f *fakeEngine) CreateContainer(_ context.Context, spec dockerengine.ContainerSpec) (dockerengine.CreatedContainer, error) {
	f.created = spec
	return dockerengine.CreatedContainer{ID: "container-id", Name: spec.Name}, nil
}
func (f *fakeEngine) AttachContainer(context.Context, string) (dockerengine.ReadCloser, error) {
	return nopReadCloser{bytes.NewReader(f.attach)}, nil
}
func (f *fakeEngine) StartContainer(context.Context, string) error { return nil }
func (f *fakeEngine) WaitContainer(context.Context, string) (dockerengine.WaitResult, error) {
	return f.wait, nil
}
func (f *fakeEngine) InspectContainer(context.Context, string) (dockerengine.ContainerState, error) {
	return dockerengine.ContainerState{ID: "container-id", ExitCode: f.wait.ExitCode, OOMKilled: f.wait.OOMKilled, HostConfig: f.created}, nil
}
func (f *fakeEngine) StopContainer(context.Context, string, time.Duration) error { return nil }
func (f *fakeEngine) KillContainer(context.Context, string) error                { f.killed = true; return nil }
func (f *fakeEngine) RemoveContainer(context.Context, string) error              { f.removed = true; return nil }
func (f *fakeEngine) Close() error                                               { return nil }

type nopReadCloser struct{ *bytes.Reader }

func (n nopReadCloser) Close() error { return nil }

type captureSink struct{ events []runner.EventDraft }

func (c *captureSink) Emit(_ context.Context, d runner.EventDraft) error {
	c.events = append(c.events, d)
	return nil
}

type logSink struct{ stdout, stderr bytes.Buffer }

func (l *logSink) WriteLog(_ context.Context, stream runner.LogStream, data []byte) error {
	if stream == runner.LogStreamStdout {
		_, _ = l.stdout.Write(data)
	} else {
		_, _ = l.stderr.Write(data)
	}
	return nil
}

type failingLogSink struct{ err error }

func (f failingLogSink) WriteLog(context.Context, runner.LogStream, []byte) error { return f.err }

func muxFrame(stream byte, payload []byte, rest ...interface{}) []byte {
	out := append([]byte{stream, 0, 0, 0, byte(len(payload) >> 24), byte(len(payload) >> 16), byte(len(payload) >> 8), byte(len(payload))}, payload...)
	for i := 0; i < len(rest); i += 2 {
		out = append(out, muxFrame(rest[i].(byte), rest[i+1].([]byte))...)
	}
	return out
}
func containsLimitation(lims []model.Limitation, id string) bool {
	for _, l := range lims {
		if l.ID == id {
			return true
		}
	}
	return false
}
