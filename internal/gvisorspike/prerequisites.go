package gvisorspike

import (
	"crypto/sha512"
	"encoding/hex"
	"io"
	"os"
	"runtime"
	"strings"
)

type PlatformMode string

const (
	PlatformKVM     PlatformMode = "kvm"
	PlatformSystrap PlatformMode = "systrap"
)

type PrerequisiteRequest struct {
	RunscPath       string
	ExpectedRelease string
	ExpectedSHA512  string
	Platform        PlatformMode
	RuntimeName     string
	DockerSocket    string
	MonitorSocket   string
	FixtureImage    string
}

type PrerequisiteSummary struct {
	RunscRelease string       `json:"runscRelease"`
	RunscSHA512  string       `json:"runscSha512"`
	Architecture string       `json:"architecture"`
	Platform     PlatformMode `json:"platform"`
	RuntimeName  string       `json:"runtimeName"`
	FixtureImage string       `json:"fixtureImage"`
}

func ValidatePrerequisiteRequest(req PrerequisiteRequest, limits Limits) (PrerequisiteSummary, error) {
	limits, err := validateLimits(limits)
	if err != nil {
		return PrerequisiteSummary{}, err
	}
	if runtime.GOOS != "linux" {
		return PrerequisiteSummary{}, errCode(CodeUnsupportedPlatform, "prerequisite", "platform", "gVisor spike live validation is Linux-only", nil)
	}
	if err := validatePath(req.RunscPath, limits.MaxPathBytes); err != nil {
		return PrerequisiteSummary{}, errCode(CodeInvalidRunscPath, "prerequisite", "runsc", "invalid runsc path", err)
	}
	if err := validatePath(req.DockerSocket, limits.MaxPathBytes); err != nil {
		return PrerequisiteSummary{}, errCode(CodeInvalidPrerequisite, "prerequisite", "dockerSocket", "invalid Docker socket path", err)
	}
	if err := validatePath(req.MonitorSocket, limits.MaxPathBytes); err != nil {
		return PrerequisiteSummary{}, errCode(CodeInvalidPrerequisite, "prerequisite", "monitorSocket", "invalid monitor socket path", err)
	}
	if req.ExpectedRelease != PinnedRunscRelease {
		return PrerequisiteSummary{}, errCode(CodeRunscVersionMismatch, "prerequisite", "release", "unexpected runsc release", nil)
	}
	if len(req.ExpectedSHA512) != 128 || strings.ToLower(req.ExpectedSHA512) != req.ExpectedSHA512 {
		return PrerequisiteSummary{}, errCode(CodeRunscHashMismatch, "prerequisite", "sha512", "expected runsc SHA-512 must be lowercase hex", nil)
	}
	if req.Platform != PlatformKVM && req.Platform != PlatformSystrap {
		return PrerequisiteSummary{}, errCode(CodeUnsupportedPlatformMode, "prerequisite", "platform", "unsupported gVisor platform", nil)
	}
	if req.RuntimeName == "" || req.RuntimeName == "runc" {
		return PrerequisiteSummary{}, errCode(CodeRuntimeNotConfigured, "prerequisite", "runtime", "dedicated runtime is required", nil)
	}
	if _, err := BuildFixtureContainerSpec(FixtureContainerRequest{RuntimeName: req.RuntimeName, Image: req.FixtureImage, MemoryBytes: 1, PidsLimit: 1, NanoCPUs: 1}); err != nil {
		return PrerequisiteSummary{}, err
	}
	return PrerequisiteSummary{RunscRelease: req.ExpectedRelease, RunscSHA512: req.ExpectedSHA512, Architecture: runtime.GOARCH, Platform: req.Platform, RuntimeName: req.RuntimeName, FixtureImage: req.FixtureImage}, nil
}

func ValidateRunscBinary(path string, expectedSHA512 string, limits Limits) error {
	limits, err := validateLimits(limits)
	if err != nil {
		return err
	}
	if err := validatePath(path, limits.MaxPathBytes); err != nil {
		return errCode(CodeInvalidRunscPath, "prerequisite", "runsc", "invalid runsc path", err)
	}
	if len(expectedSHA512) != 128 || strings.ToLower(expectedSHA512) != expectedSHA512 {
		return errCode(CodeRunscHashMismatch, "prerequisite", "sha512", "expected runsc SHA-512 must be lowercase hex", nil)
	}
	st, err := os.Lstat(path)
	if err != nil {
		return errCode(CodeInvalidRunscPath, "prerequisite", "runsc", "runsc path cannot be inspected", nil)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return errCode(CodeRunscSymlink, "prerequisite", "runsc", "runsc final component is a symlink", nil)
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0o111 == 0 {
		return errCode(CodeInvalidRunscPath, "prerequisite", "runsc", "runsc must be an executable regular file", nil)
	}
	actual, err := HashFileSHA512(path)
	if err != nil {
		return errCode(CodeRunscHashMismatch, "prerequisite", "sha512", "runsc SHA-512 could not be computed", nil)
	}
	if actual != expectedSHA512 {
		return errCode(CodeRunscHashMismatch, "prerequisite", "sha512", "runsc SHA-512 mismatch", nil)
	}
	return nil
}

func HashFileSHA512(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
