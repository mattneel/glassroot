package dockerengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	containerapi "github.com/moby/moby/api/types/container"
	networkapi "github.com/moby/moby/api/types/network"
	mobyclient "github.com/moby/moby/client"
)

const defaultRequestTimeout = 10 * time.Second

type Client struct {
	cli      *mobyclient.Client
	metadata ServerMetadata
	closed   bool
}

var _ Interface = (*Client)(nil)

func Open(ctx context.Context, cfg Config) (Interface, error) {
	if runtime.GOOS != "linux" {
		return nil, errCode(CodeUnsupportedPlatform, "open", "platform", "docker-dev is supported only on Linux", nil)
	}
	if err := validateSocketPathSyntax(cfg.SocketPath); err != nil {
		return nil, err
	}
	before, err := os.Lstat(cfg.SocketPath)
	if err != nil {
		return nil, errCode(CodeInvalidSocketPath, "socket", "lstat", "socket is not accessible", err)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, errCode(CodeSocketSymlink, "socket", "lstat", "socket final component is a symlink", nil)
	}
	if before.Mode()&os.ModeSocket == 0 {
		return nil, errCode(CodeSocketNotUnix, "socket", "lstat", "socket final component is not a Unix socket", nil)
	}
	minimum := cfg.MinimumAPIVersion
	if minimum == "" {
		minimum = MinimumAPIVersion
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	openCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dialer := &net.Dialer{Timeout: timeout}
	cli, err := mobyclient.New(
		mobyclient.WithHost("unix://"+cfg.SocketPath),
		mobyclient.WithDialContext(func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", cfg.SocketPath)
		}),
	)
	if err != nil {
		return nil, errCode(CodeDaemonUnreachable, "open", "client", "create Docker API client", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = cli.Close()
		}
	}()
	ping, err := cli.Ping(openCtx, mobyclient.PingOptions{NegotiateAPIVersion: true, ForceNegotiate: true})
	if err != nil {
		return nil, errCode(CodeDaemonUnreachable, "preflight", "ping", "Docker daemon did not respond", err)
	}
	version, err := cli.ServerVersion(openCtx, mobyclient.ServerVersionOptions{})
	if err != nil {
		return nil, errCode(CodeDaemonUnreachable, "preflight", "version", "Docker daemon version unavailable", err)
	}
	info, err := cli.Info(openCtx, mobyclient.InfoOptions{})
	if err != nil {
		return nil, errCode(CodeDaemonUnreachable, "preflight", "info", "Docker daemon info unavailable", err)
	}
	negotiated := cli.ClientVersion()
	if cmp, err := compareAPIVersion(negotiated, minimum); err != nil || cmp < 0 {
		return nil, errCode(CodeUnsupportedAPIVersion, "preflight", "api", "Docker Engine API version is below required minimum", err)
	}
	if strings.ToLower(version.Os) != "linux" || strings.ToLower(info.Info.OSType) != "linux" || strings.ToLower(ping.OSType) != "linux" {
		return nil, errCode(CodeUnsupportedDaemonOS, "preflight", "os", "Docker daemon must run Linux containers", nil)
	}
	metadata := ServerMetadata{
		OSType:          "linux",
		Architecture:    firstNonEmpty(version.Arch, info.Info.Architecture),
		EngineVersion:   firstNonEmpty(version.Version, info.Info.ServerVersion),
		APIVersion:      negotiated,
		CgroupVersion:   info.Info.CgroupVersion,
		CgroupDriver:    info.Info.CgroupDriver,
		Rootless:        hasSecurityOption(info.Info.SecurityOptions, "rootless"),
		SecurityOptions: append([]string(nil), info.Info.SecurityOptions...),
	}
	if !info.Info.MemoryLimit || !info.Info.CPUCfsQuota || !info.Info.CPUCfsPeriod || !info.Info.PidsLimit {
		return nil, errCode(CodeResourceEnforcementUnavailable, "preflight", "resources", "Docker daemon does not report required resource enforcement", nil)
	}
	if metadata.Rootless && (metadata.CgroupVersion == "" || metadata.CgroupDriver == "") {
		return nil, errCode(CodeResourceEnforcementUnavailable, "preflight", "rootless", "rootless daemon resource enforcement is not established", nil)
	}
	if !hasSecurityOption(info.Info.SecurityOptions, "seccomp") || hasSecurityOption(info.Info.SecurityOptions, "seccomp=unconfined") {
		return nil, errCode(CodeSeccompUnavailable, "preflight", "seccomp", "Docker daemon must report seccomp support", nil)
	}
	after, err := os.Lstat(cfg.SocketPath)
	if err != nil {
		return nil, errCode(CodeSocketChanged, "socket", "lstat", "socket identity changed", err)
	}
	if !os.SameFile(before, after) {
		return nil, errCode(CodeSocketChanged, "socket", "lstat", "socket identity changed", nil)
	}
	cleanup = false
	return &Client{cli: cli, metadata: metadata}, nil
}

func (c *Client) Metadata() ServerMetadata { return cloneMetadata(c.metadata) }

func (c *Client) InspectImage(ctx context.Context, image string) (ImageMetadata, error) {
	image, err := ValidateImmutableLocalImage(image)
	if err != nil {
		return ImageMetadata{}, err
	}
	res, err := c.cli.ImageInspect(ctx, image)
	if err != nil {
		if isNotFoundError(err) {
			return ImageMetadata{}, errCode(CodeImageNotPresent, "image", "inspect", "image is not present locally", err)
		}
		return ImageMetadata{}, errCode(CodeDaemonUnreachable, "image", "inspect", "image inspect failed", err)
	}
	if strings.ToLower(res.Os) != "linux" {
		return ImageMetadata{}, errCode(CodeImageOSMismatch, "image", "os", "image OS must be linux", nil)
	}
	if !containsString(res.RepoDigests, image) {
		return ImageMetadata{}, errCode(CodeImageDigestMismatch, "image", "digest", "local image digest does not match requested immutable reference", nil)
	}
	vols := []string(nil)
	if res.Config != nil {
		for v := range res.Config.Volumes {
			vols = append(vols, v)
		}
		sort.Strings(vols)
	}
	if len(vols) > 0 {
		return ImageMetadata{}, errCode(CodeImageVolumeUnsupported, "image", "volumes", "image-declared volumes are unsupported", nil)
	}
	return ImageMetadata{ID: res.ID, RepoDigests: append([]string(nil), res.RepoDigests...), OSType: res.Os, Architecture: res.Architecture, DeclaredVolumes: vols}, nil
}

func (c *Client) CreateContainer(ctx context.Context, spec ContainerSpec) (CreatedContainer, error) {
	cfg := &containerapi.Config{
		Hostname:        spec.Hostname,
		User:            spec.User,
		AttachStdout:    true,
		AttachStderr:    true,
		AttachStdin:     false,
		Tty:             spec.TTY,
		OpenStdin:       spec.OpenStdin,
		Env:             append([]string(nil), spec.Env...),
		Cmd:             append([]string(nil), spec.Command...),
		Image:           spec.Image,
		Volumes:         map[string]struct{}{},
		WorkingDir:      spec.Workdir,
		Entrypoint:      append([]string(nil), spec.Entrypoint...),
		NetworkDisabled: spec.NetworkDisabled,
		Labels:          cloneStringMap(spec.Labels),
	}
	if spec.HealthcheckDisabled {
		cfg.Healthcheck = &containerapi.HealthConfig{Test: []string{"NONE"}}
	}
	init := spec.Init
	pids := spec.Resources.PidsLimit
	host := &containerapi.HostConfig{
		Runtime:         spec.Runtime,
		Binds:           bindStrings(spec.Binds),
		LogConfig:       containerapi.LogConfig{Type: spec.LogDriver},
		NetworkMode:     containerapi.NetworkMode(spec.NetworkMode),
		PortBindings:    networkapi.PortMap{},
		RestartPolicy:   containerapi.RestartPolicy{Name: containerapi.RestartPolicyMode(spec.RestartPolicy)},
		AutoRemove:      spec.AutoRemove,
		CapAdd:          append([]string(nil), spec.CapAdd...),
		CapDrop:         append([]string(nil), spec.CapDrop...),
		GroupAdd:        append([]string(nil), spec.GroupAdd...),
		Privileged:      spec.Privileged,
		PublishAllPorts: false,
		ReadonlyRootfs:  spec.ReadOnlyRootfs,
		SecurityOpt:     securityOptions(spec),
		Tmpfs:           tmpfsMap(spec.Tmpfs),
		ShmSize:         spec.Resources.ShmSizeBytes,
		Resources: containerapi.Resources{
			NanoCPUs:   spec.Resources.NanoCPUs,
			Memory:     spec.Resources.MemoryBytes,
			MemorySwap: spec.Resources.MemorySwapBytes,
			PidsLimit:  &pids,
		},
		Init: &init,
	}
	res, err := c.cli.ContainerCreate(ctx, mobyclient.ContainerCreateOptions{Config: cfg, HostConfig: host, NetworkingConfig: &networkapi.NetworkingConfig{}, Name: spec.Name})
	if err != nil {
		return CreatedContainer{}, errCode(CodeDaemonUnreachable, "container", "create", "container create failed", err)
	}
	if res.ID == "" {
		return CreatedContainer{}, errCode(CodeEngineResponseInvalid, "container", "create", "container create returned no ID", nil)
	}
	return CreatedContainer{ID: res.ID, Name: spec.Name}, nil
}

func (c *Client) AttachContainer(ctx context.Context, id string) (io.ReadCloser, error) {
	res, err := c.cli.ContainerAttach(ctx, id, mobyclient.ContainerAttachOptions{Stream: true, Stdout: true, Stderr: true, Logs: false})
	if err != nil {
		return nil, errCode(CodeDaemonUnreachable, "container", "attach", "container attach failed", err)
	}
	return &hijackedReadCloser{HijackedResponse: res.HijackedResponse}, nil
}

func (c *Client) StartContainer(ctx context.Context, id string) error {
	_, err := c.cli.ContainerStart(ctx, id, mobyclient.ContainerStartOptions{})
	if err != nil {
		return errCode(CodeDaemonUnreachable, "container", "start", "container start failed", err)
	}
	return nil
}

func (c *Client) WaitContainer(ctx context.Context, id string) (WaitResult, error) {
	res := c.cli.ContainerWait(ctx, id, mobyclient.ContainerWaitOptions{Condition: containerapi.WaitConditionNotRunning})
	select {
	case err := <-res.Error:
		return WaitResult{}, errCode(CodeDaemonUnreachable, "container", "wait", "container wait failed", err)
	case out := <-res.Result:
		return WaitResult{ExitCode: int(out.StatusCode)}, nil
	case <-ctx.Done():
		return WaitResult{}, errCode(CodeContextCancelled, "container", "wait", "context cancelled", ctx.Err())
	}
}

func (c *Client) InspectContainer(ctx context.Context, id string) (ContainerState, error) {
	res, err := c.cli.ContainerInspect(ctx, id, mobyclient.ContainerInspectOptions{})
	if err != nil {
		return ContainerState{}, errCode(CodeDaemonUnreachable, "container", "inspect", "container inspect failed", err)
	}
	state := ContainerState{ID: res.Container.ID}
	if res.Container.State != nil {
		state.ExitCode = res.Container.State.ExitCode
		state.OOMKilled = res.Container.State.OOMKilled
	}
	state.HostConfig = specFromInspect(res.Container.HostConfig, res.Container.Config)
	return state, nil
}

func (c *Client) StopContainer(ctx context.Context, id string, grace time.Duration) error {
	seconds := int(grace / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	_, err := c.cli.ContainerStop(ctx, id, mobyclient.ContainerStopOptions{Timeout: &seconds})
	if err != nil {
		return errCode(CodeDaemonUnreachable, "container", "stop", "container stop failed", err)
	}
	return nil
}

func (c *Client) KillContainer(ctx context.Context, id string) error {
	_, err := c.cli.ContainerKill(ctx, id, mobyclient.ContainerKillOptions{Signal: "SIGKILL"})
	if err != nil {
		return errCode(CodeDaemonUnreachable, "container", "kill", "container kill failed", err)
	}
	return nil
}

func (c *Client) RemoveContainer(ctx context.Context, id string) error {
	_, err := c.cli.ContainerRemove(ctx, id, mobyclient.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
	if err != nil {
		return errCode(CodeDaemonUnreachable, "container", "remove", "container remove failed", err)
	}
	return nil
}

func (c *Client) Close() error {
	if c == nil || c.closed {
		return nil
	}
	c.closed = true
	if err := c.cli.Close(); err != nil {
		return errCode(CodeCloseFailed, "close", "client", "Docker client close failed", err)
	}
	return nil
}

type hijackedReadCloser struct{ mobyclient.HijackedResponse }

func (h *hijackedReadCloser) Read(p []byte) (int, error) { return h.Reader.Read(p) }
func (h *hijackedReadCloser) Close() error               { h.HijackedResponse.Close(); return nil }

func bindStrings(binds []BindMount) []string {
	out := make([]string, 0, len(binds))
	for _, b := range binds {
		mode := "ro"
		if b.ReadWrite {
			mode = "rw"
		}
		out = append(out, b.HostPath+":"+b.ContainerPath+":"+mode)
	}
	return out
}

func tmpfsMap(in []TmpfsMount) map[string]string {
	out := map[string]string{}
	for _, t := range in {
		opts := append([]string{fmt.Sprintf("size=%d", t.SizeBytes)}, t.Options...)
		out[t.Path] = strings.Join(opts, ",")
	}
	return out
}

func securityOptions(spec ContainerSpec) []string {
	out := []string{}
	if spec.NoNewPrivileges {
		out = append(out, "no-new-privileges=true")
	}
	return out
}

func specFromInspect(host *containerapi.HostConfig, cfg *containerapi.Config) ContainerSpec {
	var spec ContainerSpec
	if cfg != nil {
		spec.Image = cfg.Image
		spec.Command = append([]string(nil), cfg.Cmd...)
		spec.Entrypoint = append([]string(nil), cfg.Entrypoint...)
		spec.Workdir = cfg.WorkingDir
		spec.Hostname = cfg.Hostname
		spec.User = cfg.User
		spec.TTY = cfg.Tty
		spec.OpenStdin = cfg.OpenStdin
		spec.NetworkDisabled = cfg.NetworkDisabled
	}
	if host != nil {
		spec.Runtime = host.Runtime
		spec.NetworkMode = string(host.NetworkMode)
		spec.Privileged = host.Privileged
		spec.ReadOnlyRootfs = host.ReadonlyRootfs
		spec.CapAdd = append([]string(nil), host.CapAdd...)
		spec.CapDrop = append([]string(nil), host.CapDrop...)
		spec.AutoRemove = host.AutoRemove
		spec.RestartPolicy = string(host.RestartPolicy.Name)
		spec.LogDriver = host.LogConfig.Type
		if host.Init != nil {
			spec.Init = *host.Init
		}
		spec.Resources.NanoCPUs = host.Resources.NanoCPUs
		spec.Resources.MemoryBytes = host.Resources.Memory
		spec.Resources.MemorySwapBytes = host.Resources.MemorySwap
		if host.Resources.PidsLimit != nil {
			spec.Resources.PidsLimit = *host.Resources.PidsLimit
		}
		spec.Resources.ShmSizeBytes = host.ShmSize
		for _, opt := range host.SecurityOpt {
			if opt == "no-new-privileges=true" {
				spec.NoNewPrivileges = true
			}
		}
	}
	return spec
}

func cloneMetadata(in ServerMetadata) ServerMetadata {
	out := in
	out.SecurityOptions = append([]string(nil), in.SecurityOptions...)
	return out
}
func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
func hasSecurityOption(opts []string, needle string) bool {
	needle = strings.ToLower(needle)
	for _, opt := range opts {
		if strings.Contains(strings.ToLower(opt), needle) {
			return true
		}
	}
	return false
}

type notFoundError interface{ NotFound() }

func isNotFoundError(err error) bool {
	var nf notFoundError
	return errors.As(err, &nf)
}
