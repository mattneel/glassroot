package dockerdev

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path"
	"strings"

	"github.com/mattneel/glassroot/internal/dockerengine"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/runner"
)

const containerWorkPID int64 = 1

func (r *Runner) RunAttempt(ctx context.Context, req runner.AttemptRequest, sink runner.DraftSink) (runner.AttemptOutcome, error) {
	return r.RunAttemptWithOutput(ctx, req, sink, runner.DiscardOutputSink())
}

func (r *Runner) RunAttemptWithOutput(ctx context.Context, req runner.AttemptRequest, sink runner.DraftSink, logs runner.AttemptOutputSink) (runner.AttemptOutcome, error) {
	if err := ctx.Err(); err != nil {
		return runner.AttemptOutcome{}, errCode(CodeContextCancelled, "attempt", req.AttemptID, "context cancelled", err)
	}
	if sink == nil {
		return runner.AttemptOutcome{}, errCode(CodeInvalidRunnerConfig, "attempt", req.AttemptID, "event sink is required", nil)
	}
	if logs == nil {
		return runner.AttemptOutcome{}, errCode(CodeInvalidRunnerConfig, "attempt", req.AttemptID, "log sink is required", nil)
	}
	binding, ok := r.workspaces[req.AttemptID]
	if !ok {
		return runner.AttemptOutcome{}, errCode(CodeMissingWorkspaceBinding, "attempt", req.AttemptID, "missing workspace binding", nil)
	}
	if binding.Revision != req.Revision || binding.ScenarioID != req.ScenarioID || binding.Repetition != req.Repetition || (binding.MaterializedTreeDigest != "" && req.MaterializedTreeDigest != "" && binding.MaterializedTreeDigest != req.MaterializedTreeDigest) {
		return runner.AttemptOutcome{}, errCode(CodeInvalidWorkspaceBinding, "attempt", req.AttemptID, "workspace binding does not match attempt", nil)
	}
	if err := validateAttemptRequest(req); err != nil {
		return runner.AttemptOutcome{}, err
	}
	image, err := r.engine.InspectImage(ctx, req.Image)
	if err != nil {
		return runner.AttemptOutcome{}, err
	}
	if err := verifyImageMetadata(req.Image, image); err != nil {
		return runner.AttemptOutcome{}, err
	}
	spec, err := r.buildContainerSpec(req, binding)
	if err != nil {
		return runner.AttemptOutcome{}, err
	}
	created, err := r.engine.CreateContainer(ctx, spec)
	if err != nil {
		return runner.AttemptOutcome{}, errCode(CodeContainerCreateFailed, "container", req.AttemptID, "container create failed", err)
	}
	containerID := created.ID
	createdOK := containerID != ""
	if !createdOK {
		return runner.AttemptOutcome{}, errCode(CodeContainerCreateFailed, "container", req.AttemptID, "container create returned no ID", nil)
	}
	cleanup := func(primary error, kill bool) error {
		var cleanupErr error
		if kill {
			if err := r.engine.KillContainer(context.Background(), containerID); err != nil {
				cleanupErr = err
			}
		}
		if err := r.engine.RemoveContainer(context.Background(), containerID); err != nil {
			if cleanupErr == nil {
				cleanupErr = err
			} else {
				cleanupErr = errors.Join(cleanupErr, err)
			}
		}
		if primary != nil {
			if cleanupErr != nil {
				return errCode(CodeCleanupFailed, "cleanup", req.AttemptID, "container cleanup failed", errors.Join(primary, cleanupErr))
			}
			return primary
		}
		if cleanupErr != nil {
			return errCode(CodeCleanupFailed, "cleanup", req.AttemptID, "container cleanup failed", cleanupErr)
		}
		return nil
	}
	attach, err := r.engine.AttachContainer(ctx, containerID)
	if err != nil {
		return runner.AttemptOutcome{}, cleanup(errCode(CodeAttachFailed, "container", req.AttemptID, "container attach failed", err), true)
	}
	attachOpen := true
	closeAttach := func() error {
		if attachOpen {
			attachOpen = false
			return attach.Close()
		}
		return nil
	}
	if err := r.engine.StartContainer(ctx, containerID); err != nil {
		_ = closeAttach()
		return runner.AttemptOutcome{}, cleanup(errCode(CodeStartFailed, "container", req.AttemptID, "container start failed", err), true)
	}
	if err := emitObserverWarning(ctx, sink); err != nil {
		_ = closeAttach()
		return runner.AttemptOutcome{}, cleanup(errCode(CodeOutputSinkFailed, "event", req.AttemptID, "event sink failed", err), true)
	}
	if err := emitProcessStart(ctx, sink, req); err != nil {
		_ = closeAttach()
		return runner.AttemptOutcome{}, cleanup(errCode(CodeOutputSinkFailed, "event", req.AttemptID, "event sink failed", err), true)
	}
	var stdoutWritten, stderrWritten, totalWritten int64
	var totalTruncated bool
	counts, err := dockerengine.DecodeDockerAttachFrames(ctx, attach, maxPositive(r.limits.MaxStdoutBytes, r.limits.MaxStderrBytes), func(ctx context.Context, stream dockerengine.LogStream, data []byte) error {
		rs := runner.LogStreamStdout
		allowed := r.limits.MaxStdoutBytes - stdoutWritten
		if stream == dockerengine.LogStreamStderr {
			rs = runner.LogStreamStderr
			allowed = r.limits.MaxStderrBytes - stderrWritten
		}
		if totalRemaining := r.limits.MaxTotalOutputBytes - totalWritten; totalRemaining < allowed {
			allowed = totalRemaining
		}
		if allowed <= 0 {
			totalTruncated = true
			return nil
		}
		if int64(len(data)) > allowed {
			totalTruncated = true
			data = data[:allowed]
		}
		if err := logs.WriteLog(ctx, rs, data); err != nil {
			return err
		}
		if stream == dockerengine.LogStreamStderr {
			stderrWritten += int64(len(data))
		} else {
			stdoutWritten += int64(len(data))
		}
		totalWritten += int64(len(data))
		return nil
	})
	if closeErr := closeAttach(); closeErr != nil && err == nil {
		err = errCode(CodeAttachFailed, "container", req.AttemptID, "attach close failed", closeErr)
	}
	if err != nil {
		return runner.AttemptOutcome{}, cleanup(errCode(CodeOutputSinkFailed, "output", req.AttemptID, "output processing failed", err), true)
	}
	wait, err := r.engine.WaitContainer(ctx, containerID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			var stopErr error
			if serr := r.engine.StopContainer(context.Background(), containerID, r.limits.StopGracePeriod); serr != nil {
				stopErr = errCode(CodeStopFailed, "container", req.AttemptID, "container stop failed", serr)
			}
			if cerr := cleanup(stopErr, true); cerr != nil {
				return runner.AttemptOutcome{}, cerr
			}
			exit := 137
			return runner.AttemptOutcome{Status: runner.AttemptStatusTimedOut, ExitCode: &exit, Limitations: outputLimitations(counts, totalTruncated)}, nil
		}
		return runner.AttemptOutcome{}, cleanup(errCode(CodeWaitFailed, "container", req.AttemptID, "container wait failed", err), true)
	}
	state, err := r.engine.InspectContainer(ctx, containerID)
	if err != nil {
		return runner.AttemptOutcome{}, cleanup(errCode(CodeInspectFailed, "container", req.AttemptID, "container inspect failed", err), true)
	}
	if err := verifyContainerConfig(spec, state.HostConfig); err != nil {
		return runner.AttemptOutcome{}, cleanup(err, true)
	}
	if state.OOMKilled || wait.OOMKilled {
		_ = emitResourceLimit(ctx, sink, req)
	}
	exit := wait.ExitCode
	if state.ExitCode != 0 || wait.ExitCode == 0 {
		exit = state.ExitCode
	}
	if err := emitProcessExit(ctx, sink, req, exit); err != nil {
		return runner.AttemptOutcome{}, cleanup(errCode(CodeOutputSinkFailed, "event", req.AttemptID, "event sink failed", err), true)
	}
	status := runner.AttemptStatusSucceeded
	if state.OOMKilled || wait.OOMKilled {
		status = runner.AttemptStatusResourceLimited
	} else if exit != 0 {
		status = runner.AttemptStatusFailed
	}
	out := runner.AttemptOutcome{Status: status, ExitCode: &exit, DurationMillis: 0, Limitations: outputLimitations(counts, totalTruncated)}
	if err := cleanup(nil, false); err != nil {
		return runner.AttemptOutcome{}, err
	}
	return out, nil
}

func validateAttemptRequest(req runner.AttemptRequest) error {
	if _, err := dockerengine.ValidateImmutableLocalImage(req.Image); err != nil {
		return err
	}
	if req.NetworkPolicy.Mode != model.NetworkModeDeny {
		return errCode(CodeCapabilityMismatch, "attempt", req.AttemptID, "docker-dev requires network deny", nil)
	}
	if req.Shell == "" || !strings.HasPrefix(req.Shell, "/") || strings.ContainsAny(req.Shell, "\x00\n\r\t") {
		return errCode(CodeInvalidRunnerConfig, "attempt", req.AttemptID, "planned shell must be an absolute path", nil)
	}
	if req.Workdir == "" || !strings.HasPrefix(req.Workdir, "/") || path.Clean(req.Workdir) != req.Workdir {
		return errCode(CodeInvalidRunnerConfig, "attempt", req.AttemptID, "planned workdir must be absolute and clean", nil)
	}
	return nil
}

func verifyImageMetadata(want string, image dockerengine.ImageMetadata) error {
	if strings.ToLower(image.OSType) != "linux" {
		return errCode(CodeInvalidRunnerConfig, "image", "", "image OS must be linux", nil)
	}
	if len(image.DeclaredVolumes) > 0 {
		return errCode(CodeInvalidRunnerConfig, "image", "", "image-declared volumes are unsupported", nil)
	}
	for _, rd := range image.RepoDigests {
		if rd == want {
			return nil
		}
	}
	return errCode(CodeInvalidRunnerConfig, "image", "", "image digest mismatch", nil)
}

func (r *Runner) buildContainerSpec(req runner.AttemptRequest, b WorkspaceBinding) (dockerengine.ContainerSpec, error) {
	nano, mem, pids, err := resourceValues(req.ResourceLimits)
	if err != nil {
		return dockerengine.ContainerSpec{}, err
	}
	name, nonce, err := randomContainerName()
	if err != nil {
		return dockerengine.ContainerSpec{}, err
	}
	env := make([]string, 0, len(req.Environment))
	for _, e := range req.Environment {
		env = append(env, e.Name+"="+e.Value)
	}
	return dockerengine.ContainerSpec{Name: name, Image: req.Image, Entrypoint: []string{}, Command: []string{req.Shell, "-c", req.Run}, Workdir: req.Workdir, Env: env, Hostname: "glassroot", User: "0", NetworkDisabled: true, NetworkMode: "none", ExposedPorts: []string{}, PublishedPorts: []string{}, Privileged: false, NoNewPrivileges: true, SeccompDefault: true, CapDrop: []string{"ALL"}, CapAdd: []string{}, Devices: []string{}, DeviceRequests: []string{}, GroupAdd: []string{}, HostPID: false, HostIPC: false, HostUTS: false, ReadOnlyRootfs: true, Init: true, TTY: false, OpenStdin: false, AutoRemove: false, RestartPolicy: "no", HealthcheckDisabled: true, LogDriver: "none", Binds: []dockerengine.BindMount{{HostPath: b.HostPath, ContainerPath: req.Workdir, ReadWrite: true}}, Tmpfs: []dockerengine.TmpfsMount{{Path: "/tmp", SizeBytes: r.limits.TmpfsSizeBytes, Options: []string{"nosuid", "nodev"}}, {Path: "/run", SizeBytes: r.limits.TmpfsSizeBytes, Options: []string{"nosuid", "nodev"}}, {Path: "/var/tmp", SizeBytes: r.limits.TmpfsSizeBytes, Options: []string{"nosuid", "nodev"}}}, Resources: dockerengine.Resources{NanoCPUs: nano, MemoryBytes: mem, MemorySwapBytes: mem, PidsLimit: pids, ShmSizeBytes: r.limits.ShmSizeBytes}, Labels: map[string]string{"glassroot.dev/owned": "true", "glassroot.dev/runner": "docker-dev", "glassroot.dev/operation": nonce, "glassroot.dev/attempt": boundedAttemptLabel(req.AttemptID)}}, nil
}

func resourceValues(l model.ResourceLimits) (int64, int64, int64, error) {
	if l.CPU <= 0 || l.CPU > 1024 {
		return 0, 0, 0, errCode(CodeInvalidRunnerConfig, "resources", "", "CPU limit is invalid", nil)
	}
	if l.MemoryBytes <= 0 {
		return 0, 0, 0, errCode(CodeInvalidRunnerConfig, "resources", "", "memory limit is required", nil)
	}
	if l.ProcessCount <= 0 {
		return 0, 0, 0, errCode(CodeInvalidRunnerConfig, "resources", "", "process count limit is required", nil)
	}
	return l.CPU * 1_000_000_000, l.MemoryBytes, l.ProcessCount, nil
}

func randomContainerName() (string, string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", err
	}
	nonce := hex.EncodeToString(b[:])
	return "glassroot-docker-dev-" + nonce, nonce, nil
}

func boundedAttemptLabel(id string) string {
	if len(id) > 48 {
		return id[:48]
	}
	return id
}

func verifyContainerConfig(want, got dockerengine.ContainerSpec) error {
	if got.NetworkMode != "none" || got.Privileged || !got.ReadOnlyRootfs || !got.NoNewPrivileges || !got.Init || got.AutoRemove || len(got.CapDrop) != 1 || got.CapDrop[0] != "ALL" || got.Resources.MemoryBytes != want.Resources.MemoryBytes || got.Resources.PidsLimit != want.Resources.PidsLimit || got.Resources.NanoCPUs != want.Resources.NanoCPUs {
		return errCode(CodeContainerConfigMismatch, "container", "", "created container configuration did not match requested confinement", nil)
	}
	return nil
}

func emitObserverWarning(ctx context.Context, sink runner.DraftSink) error {
	return sink.Emit(ctx, runner.EventDraft{Source: model.ObservationSourceSandboxRuntimeObserved, Kind: model.ObservationKindObserverWarning, ObserverWarning: &model.ObserverWarningObservation{Code: "docker-dev-observation-gap", Message: "docker-dev is development-only and does not provide comprehensive child-process, filesystem, syscall, or network observation.", Unsupported: true, Limitations: []model.Limitation{{ID: "docker-dev-development-only", Summary: "Docker development runner is not a hardened security boundary."}}}})
}
func emitProcessStart(ctx context.Context, sink runner.DraftSink, req runner.AttemptRequest) error {
	return sink.Emit(ctx, runner.EventDraft{Source: model.ObservationSourceSandboxRuntimeObserved, Kind: model.ObservationKindProcessStart, Process: &model.ProcessObservation{Operation: "start", ProcessID: containerWorkPID, ExecutablePath: req.Shell, Arguments: []string{}, Environment: []model.EnvEntry{}, DurationMillis: 0}})
}
func emitProcessExit(ctx context.Context, sink runner.DraftSink, req runner.AttemptRequest, code int) error {
	return sink.Emit(ctx, runner.EventDraft{Source: model.ObservationSourceSandboxRuntimeObserved, Kind: model.ObservationKindProcessExit, Process: &model.ProcessObservation{Operation: "exit", ProcessID: containerWorkPID, ExecutablePath: req.Shell, Arguments: []string{}, Environment: []model.EnvEntry{}, ExitCode: &code, DurationMillis: 0}})
}
func emitResourceLimit(ctx context.Context, sink runner.DraftSink, req runner.AttemptRequest) error {
	return sink.Emit(ctx, runner.EventDraft{Source: model.ObservationSourceSandboxRuntimeObserved, Kind: model.ObservationKindResourceLimit, ResourceLimit: &model.ResourceLimitObservation{LimitKind: "memory", LimitValue: req.ResourceLimits.MemoryBytes, Unit: "bytes", Exceeded: true}})
}

func outputLimitations(c dockerengine.OutputCounts, totalTruncated bool) []model.Limitation {
	out := []model.Limitation{}
	if c.StdoutTruncated {
		out = append(out, model.Limitation{ID: "stdout-truncated", Summary: "Stdout exceeded the docker-dev per-stream byte limit."})
	}
	if c.StderrTruncated {
		out = append(out, model.Limitation{ID: "stderr-truncated", Summary: "Stderr exceeded the docker-dev per-stream byte limit."})
	}
	if totalTruncated {
		out = append(out, model.Limitation{ID: "output-truncated", Summary: "Combined stdout/stderr exceeded the docker-dev total byte limit."})
	}
	return out
}
func maxPositive(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func buildContainerSpecForFuzz(shell, run string, cpu, mem, pids int64) (dockerengine.ContainerSpec, error) {
	r := &Runner{limits: DefaultLimits()}
	return r.buildContainerSpec(runner.AttemptRequest{AttemptID: "fuzz", Image: "registry.example.invalid/dev@sha256:" + strings.Repeat("a", 64), Shell: shell, Run: run, Workdir: "/workspace", ResourceLimits: model.ResourceLimits{CPU: cpu, MemoryBytes: mem, ProcessCount: pids}, NetworkPolicy: model.NetworkPolicy{Mode: model.NetworkModeDeny}}, WorkspaceBinding{AttemptID: "fuzz", HostPath: "/tmp/fuzz"})
}
