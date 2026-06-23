package gvisorspike

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

var requiredTracePoints = []string{
	"container/start",
	"sentry/clone",
	"syscall/clone/enter",
	"syscall/execve/enter",
	"sentry/execve",
	"sentry/exit_notify_parent",
	"sentry/task_exit",
}

var defaultContextFields = []string{"time", "thread_id", "thread_group_id", "container_id", "process_name", "parent_thread_group_id"}

func DefaultTracePoints() []string { return append([]string(nil), requiredTracePoints...) }

type PodInitRequest struct {
	Endpoint        string
	TraceInventory  []string
	Retries         int
	InitialBackoff  string
	MaximumBackoff  string
	IncludeOptional []string
	Limits          Limits
}

type PodInitConfiguration struct {
	SchemaVersion    string   `json:"schemaVersion"`
	SessionName      string   `json:"sessionName"`
	SinkName         string   `json:"sinkName"`
	Endpoint         string   `json:"endpoint"`
	IgnoreSetupError bool     `json:"ignoreSetupError"`
	Retries          int      `json:"retries"`
	InitialBackoff   string   `json:"initialBackoff"`
	MaximumBackoff   string   `json:"maximumBackoff"`
	TracePoints      []string `json:"tracePoints"`
	ContextFields    []string `json:"contextFields"`
}

func BuildPodInitConfiguration(req PodInitRequest) (PodInitConfiguration, error) {
	limits, err := validateLimits(req.Limits)
	if err != nil {
		return PodInitConfiguration{}, err
	}
	if err := validatePath(req.Endpoint, limits.MaxPathBytes); err != nil {
		return PodInitConfiguration{}, errCode(CodeInvalidPodInitConfig, "pod-init", "endpoint", "invalid monitor endpoint", err)
	}
	inventory := make(map[string]struct{}, len(req.TraceInventory))
	for _, p := range req.TraceInventory {
		if err := validateToken(p, limits.MaxStringBytes); err != nil {
			return PodInitConfiguration{}, errCode(CodeInvalidPodInitConfig, "pod-init", "tracePoint", "invalid trace point", err)
		}
		inventory[p] = struct{}{}
	}
	points := DefaultTracePoints()
	for _, p := range points {
		if _, ok := inventory[p]; !ok {
			return PodInitConfiguration{}, errCode(CodeTracepointMissing, "pod-init", p, "required gVisor trace point is missing", nil)
		}
	}
	fields := append([]string(nil), defaultContextFields...)
	if len(req.IncludeOptional) > 0 {
		fields = append([]string(nil), req.IncludeOptional...)
	}
	for _, f := range fields {
		if f == "credentials" || f == "env" || f == "file_contents" || f == "environment" {
			return PodInitConfiguration{}, errCode(CodeInvalidPodInitConfig, "pod-init", "contextFields", "unsafe context field requested", nil)
		}
		if err := validateToken(f, limits.MaxStringBytes); err != nil {
			return PodInitConfiguration{}, err
		}
	}
	sort.Strings(points)
	fields = dedupeSorted(fields)
	if req.Retries < 0 {
		return PodInitConfiguration{}, errCode(CodeInvalidPodInitConfig, "pod-init", "retries", "negative retries", nil)
	}
	if req.InitialBackoff == "" {
		req.InitialBackoff = "25us"
	}
	if req.MaximumBackoff == "" {
		req.MaximumBackoff = "10ms"
	}
	return PodInitConfiguration{SchemaVersion: "glassroot.dev/gvisor-pod-init/v1alpha1", SessionName: "Default", SinkName: "remote", Endpoint: req.Endpoint, IgnoreSetupError: false, Retries: req.Retries, InitialBackoff: req.InitialBackoff, MaximumBackoff: req.MaximumBackoff, TracePoints: points, ContextFields: fields}, nil
}

func (c PodInitConfiguration) JSON() ([]byte, error) {
	if c.TracePoints == nil {
		c.TracePoints = []string{}
	}
	if c.ContextFields == nil {
		c.ContextFields = []string{}
	}
	return json.Marshal(c)
}

func validatePath(path string, max int64) error {
	if path == "" || int64(len(path)) > max || !utf8.ValidString(path) || strings.ContainsRune(path, '\x00') || filepath.Clean(path) != path || !filepath.IsAbs(path) {
		return errCode(CodeInvalidPrerequisite, "path", "", "path must be absolute, clean, bounded UTF-8", nil)
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return errCode(CodeInvalidPrerequisite, "path", "", "path contains controls", nil)
		}
	}
	return nil
}

func validateToken(s string, max int64) error {
	if s == "" || int64(len(s)) > max || !utf8.ValidString(s) || strings.ContainsRune(s, '\x00') {
		return errCode(CodeInvalidPrerequisite, "token", "", "invalid bounded token", nil)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return errCode(CodeInvalidPrerequisite, "token", "", "token contains controls", nil)
		}
	}
	return nil
}

func dedupeSorted(in []string) []string {
	sort.Strings(in)
	out := make([]string, 0, len(in))
	for _, v := range in {
		if len(out) == 0 || out[len(out)-1] != v {
			out = append(out, v)
		}
	}
	return out
}
