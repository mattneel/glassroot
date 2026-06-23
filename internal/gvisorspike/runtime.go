package gvisorspike

import (
	"strings"

	"github.com/mattneel/glassroot/internal/dockerengine"
)

type FixtureContainerRequest struct {
	RuntimeName string
	Image       string
	MemoryBytes int64
	PidsLimit   int64
	NanoCPUs    int64
}

func BuildFixtureContainerSpec(req FixtureContainerRequest) (dockerengine.ContainerSpec, error) {
	if req.RuntimeName == "" || req.RuntimeName == "runc" || req.RuntimeName == "io.containerd.runc.v2" {
		return dockerengine.ContainerSpec{}, errCode(CodeRuntimeNotConfigured, "runtime", "name", "dedicated gVisor runtime name is required", nil)
	}
	if _, err := dockerengine.ValidateImmutableLocalImage(req.Image); err != nil {
		return dockerengine.ContainerSpec{}, errCode(CodeImageNotPresent, "image", "digest", "immutable local fixture image is required", err)
	}
	if req.MemoryBytes <= 0 || req.PidsLimit <= 0 || req.NanoCPUs <= 0 {
		return dockerengine.ContainerSpec{}, errCode(CodeInvalidContainerRequest, "container", "resources", "bounded resources are required", nil)
	}
	return dockerengine.ContainerSpec{Name: "glassroot-gvisor-spike", Runtime: req.RuntimeName, Image: req.Image, Entrypoint: []string{"/glassroot-parent"}, Command: []string{}, Workdir: "/", Env: []string{}, Hostname: "glassroot", User: "0", NetworkDisabled: true, NetworkMode: "none", ExposedPorts: []string{}, PublishedPorts: []string{}, Privileged: false, NoNewPrivileges: true, SeccompDefault: true, CapDrop: []string{"ALL"}, CapAdd: []string{}, Devices: []string{}, DeviceRequests: []string{}, GroupAdd: []string{}, HostPID: false, HostIPC: false, HostUTS: false, ReadOnlyRootfs: true, Init: true, TTY: false, OpenStdin: false, AutoRemove: false, RestartPolicy: "no", HealthcheckDisabled: true, LogDriver: "none", Binds: []dockerengine.BindMount{}, Tmpfs: []dockerengine.TmpfsMount{}, Resources: dockerengine.Resources{NanoCPUs: req.NanoCPUs, MemoryBytes: req.MemoryBytes, MemorySwapBytes: req.MemoryBytes, PidsLimit: req.PidsLimit, ShmSizeBytes: 16 << 20}, Labels: map[string]string{"glassroot.dev/owned": "true", "glassroot.dev/runner": "gvisor-spike", "glassroot.dev/runtime": boundedLabel(req.RuntimeName)}}, nil
}

func boundedLabel(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 63 {
		return v[:63]
	}
	return v
}
