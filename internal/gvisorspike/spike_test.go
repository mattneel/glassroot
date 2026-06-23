package gvisorspike

import (
	"strings"
	"testing"

	"github.com/mattneel/glassroot/internal/dockerengine"
)

func TestBuildPodInitConfigurationIsBoundedAndDeterministic(t *testing.T) {
	cfg, err := BuildPodInitConfiguration(PodInitRequest{
		Endpoint:        "/tmp/glassroot-gvisor/events.sock",
		TraceInventory:  DefaultTracePoints(),
		Retries:         3,
		InitialBackoff:  "25us",
		MaximumBackoff:  "10ms",
		IncludeOptional: []string{"time", "thread_id", "thread_group_id", "container_id", "process_name", "parent_thread_group_id"},
	})
	if err != nil {
		t.Fatalf("BuildPodInitConfiguration() error = %v", err)
	}
	if cfg.SessionName != "Default" || cfg.SinkName != "remote" || cfg.IgnoreSetupError {
		t.Fatalf("unexpected session/sink setup: %+v", cfg)
	}
	if !contains(cfg.TracePoints, "container/start") || !contains(cfg.TracePoints, "sentry/clone") || !contains(cfg.TracePoints, "syscall/execve/enter") || !contains(cfg.TracePoints, "sentry/task_exit") {
		t.Fatalf("required trace points missing: %+v", cfg.TracePoints)
	}
	if contains(cfg.ContextFields, "credentials") || contains(cfg.ContextFields, "env") || contains(cfg.ContextFields, "file_contents") {
		t.Fatalf("unsafe context fields enabled: %+v", cfg.ContextFields)
	}
	encoded, err := cfg.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if strings.Contains(string(encoded), "null") || strings.Contains(string(encoded), "credentials") {
		t.Fatalf("configuration encoded unsafe/null fields: %s", encoded)
	}
}

func TestBuildPodInitConfigurationRejectsMissingTracePoint(t *testing.T) {
	_, err := BuildPodInitConfiguration(PodInitRequest{Endpoint: "/tmp/glassroot-gvisor/events.sock", TraceInventory: []string{"container/start"}})
	assertCode(t, err, CodeTracepointMissing)
}

func TestBuildFixtureContainerSpecSelectsDedicatedRuntimeOnly(t *testing.T) {
	spec, err := BuildFixtureContainerSpec(FixtureContainerRequest{
		RuntimeName: "runsc-glassroot-spike",
		Image:       "registry.example.invalid/gvisor-fixture@sha256:" + strings.Repeat("a", 64),
		MemoryBytes: 128 << 20,
		PidsLimit:   64,
		NanoCPUs:    1_000_000_000,
	})
	if err != nil {
		t.Fatalf("BuildFixtureContainerSpec() error = %v", err)
	}
	if spec.Runtime != "runsc-glassroot-spike" || spec.Runtime == "" {
		t.Fatalf("runtime not selected exactly: %+v", spec)
	}
	if spec.NetworkMode != "none" || !spec.NetworkDisabled || spec.Privileged || !spec.ReadOnlyRootfs || !spec.NoNewPrivileges || len(spec.Binds) != 0 || len(spec.Devices) != 0 || len(spec.CapDrop) != 1 || spec.CapDrop[0] != "ALL" {
		t.Fatalf("container confinement mismatch: %+v", spec)
	}
}

func TestDockerDevCannotSelectRuntime(t *testing.T) {
	var spec dockerengine.ContainerSpec
	if spec.Runtime != "" {
		t.Fatalf("zero docker container spec unexpectedly selects runtime: %+v", spec)
	}
}

func TestProcessLifecycleStateMachine(t *testing.T) {
	tracker := NewProcessTracker()
	events := []MonitorEvent{
		{ConnectionID: "sandbox-a", Sequence: 1, Operation: OperationContainerStart, ThreadGroupID: 1, ProcessName: "parent"},
		{ConnectionID: "sandbox-a", Sequence: 2, Operation: OperationClone, ThreadGroupID: 1, ChildThreadGroupID: 2, ProcessName: "parent"},
		{ConnectionID: "sandbox-a", Sequence: 3, Operation: OperationExec, ThreadGroupID: 2, ExecutablePath: "/child", ProcessName: "child"},
		{ConnectionID: "sandbox-a", Sequence: 4, Operation: OperationExit, ThreadGroupID: 2, ExitStatus: intPtr(7)},
		{ConnectionID: "sandbox-a", Sequence: 5, Operation: OperationExit, ThreadGroupID: 1, ExitStatus: intPtr(0)},
	}
	for _, ev := range events {
		if err := tracker.Apply(ev); err != nil {
			t.Fatalf("Apply(%+v) error = %v", ev, err)
		}
	}
	summary := tracker.Summary()
	if !summary.ContainerStarted || summary.ProcessCreations != 1 || summary.Execs != 1 || summary.Exits != 2 || !summary.Complete {
		t.Fatalf("unexpected lifecycle summary: %+v", summary)
	}
}

func TestProcessLifecycleRejectsActiveReuse(t *testing.T) {
	tracker := NewProcessTracker()
	if err := tracker.Apply(MonitorEvent{ConnectionID: "sandbox-a", Sequence: 1, Operation: OperationContainerStart, ThreadGroupID: 1}); err != nil {
		t.Fatalf("start error = %v", err)
	}
	err := tracker.Apply(MonitorEvent{ConnectionID: "sandbox-a", Sequence: 2, Operation: OperationContainerStart, ThreadGroupID: 1})
	assertCode(t, err, CodeProcessStateInvalid)
}

func contains(in []string, want string) bool {
	for _, got := range in {
		if got == want {
			return true
		}
	}
	return false
}

func intPtr(v int) *int { return &v }

func assertCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected code %s, got nil", code)
	}
	var got *Error
	if !As(err, &got) || got.Code != code {
		t.Fatalf("expected code %s, got %v", code, err)
	}
}
