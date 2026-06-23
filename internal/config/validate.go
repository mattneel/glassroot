package config

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

func Validate(doc Document) (ValidatedPipeline, error) {
	var out ValidatedPipeline
	var diags Diagnostics

	if missingString(doc.APIVersion) {
		diags = append(diags, missingDiag("apiVersion", doc.APIVersion, "apiVersion is required"))
	} else if doc.APIVersion.Value != APIVersionV1Alpha1 {
		diags = append(diags, diagForString(CodeInvalidAPIVersion, "apiVersion", doc.APIVersion, "apiVersion must be "+APIVersionV1Alpha1))
	}
	if missingString(doc.Kind) {
		diags = append(diags, missingDiag("kind", doc.Kind, "kind is required"))
	} else if doc.Kind.Value != KindPipeline {
		diags = append(diags, diagForString(CodeInvalidKind, "kind", doc.Kind, "kind must be Pipeline"))
	}
	if !doc.Metadata.Present || doc.Metadata.Null {
		diags = append(diags, newDiagnostic(CodeMissingRequiredField, "metadata", doc.Metadata.Line, doc.Metadata.Column, "metadata is required"))
	} else if missingString(doc.Metadata.Name) {
		diags = append(diags, missingDiag("metadata.name", doc.Metadata.Name, "metadata.name is required"))
	} else {
		if err := validateIdentifier(doc.Metadata.Name.Value); err != nil {
			diags = append(diags, diagForString(CodeInvalidValue, "metadata.name", doc.Metadata.Name, "metadata.name must match ^[a-z][a-z0-9._-]{0,63}$"))
		} else {
			out.Name = doc.Metadata.Name.Value
		}
	}
	if !doc.Spec.Present || doc.Spec.Null {
		diags = append(diags, newDiagnostic(CodeMissingRequiredField, "spec", doc.Spec.Line, doc.Spec.Column, "spec is required"))
		return out, capDiagnostics(diags)
	}

	diags = append(diags, validateEnvironment(doc.Spec.Environment, &out)...)
	diags = append(diags, validateResources(doc.Spec.Resources, &out)...)
	diags = append(diags, validateNetwork(doc.Spec.Network, &out)...)
	diags = append(diags, validateScenarios(doc.Spec, out.Resources.TimeoutMillis, &out)...)
	diags = append(diags, validateCollect(doc.Spec.Collect, &out)...)
	diags = append(diags, validateCompare(doc.Spec.Compare, &out)...)
	diags = append(diags, validatePolicy(doc.Spec.Policy, &out)...)

	if len(diags) > 0 {
		return ValidatedPipeline{}, capDiagnostics(diags)
	}
	return out, nil
}

func validateEnvironment(env Environment, out *ValidatedPipeline) Diagnostics {
	var diags Diagnostics
	if !env.Present || env.Null {
		return Diagnostics{newDiagnostic(CodeMissingRequiredField, "spec.environment", env.Line, env.Column, "spec.environment is required")}
	}
	if missingString(env.Image) {
		diags = append(diags, missingDiag("spec.environment.image", env.Image, "environment image is required"))
	} else if digest, err := validateImmutableImage(env.Image.Value); err != nil {
		diags = append(diags, diagForString(CodeInvalidValue, "spec.environment.image", env.Image, err.Error()))
	} else {
		out.Image = env.Image.Value
		out.ImageDigest = digest
	}
	if missingString(env.Workdir) {
		diags = append(diags, missingDiag("spec.environment.workdir", env.Workdir, "workdir is required"))
	} else if err := validateAbsoluteLexicalPath(env.Workdir.Value); err != nil {
		diags = append(diags, diagForString(CodeInvalidPath, "spec.environment.workdir", env.Workdir, "workdir must be an absolute clean POSIX path"))
	} else {
		out.Workdir = env.Workdir.Value
	}
	return diags
}

func validateResources(res Resources, out *ValidatedPipeline) Diagnostics {
	var diags Diagnostics
	if !res.Present || res.Null {
		return Diagnostics{newDiagnostic(CodeMissingRequiredField, "spec.resources", res.Line, res.Column, "spec.resources is required")}
	}
	if missingInt(res.CPU) {
		diags = append(diags, missingIntDiag("spec.resources.cpu", res.CPU, "cpu is required"))
	} else if res.CPU.Value < MinCPU || res.CPU.Value > MaxCPU {
		diags = append(diags, diagForInt(CodeOutOfRange, "spec.resources.cpu", res.CPU, fmt.Sprintf("cpu must be between %d and %d", MinCPU, MaxCPU)))
	} else {
		out.Resources.CPU = res.CPU.Value
	}
	memory, d := validateSizeField("spec.resources.memory", res.Memory, MinMemoryBytes, MaxMemoryBytes)
	diags = append(diags, d...)
	out.Resources.MemoryBytes = memory
	disk, d := validateSizeField("spec.resources.disk", res.Disk, MinDiskBytes, MaxDiskBytes)
	diags = append(diags, d...)
	out.Resources.DiskBytes = disk
	if missingInt(res.Processes) {
		diags = append(diags, missingIntDiag("spec.resources.processes", res.Processes, "processes is required"))
	} else if res.Processes.Value < MinProcessCount || res.Processes.Value > MaxProcessCount {
		diags = append(diags, diagForInt(CodeOutOfRange, "spec.resources.processes", res.Processes, fmt.Sprintf("processes must be between %d and %d", MinProcessCount, MaxProcessCount)))
	} else {
		out.Resources.ProcessCount = res.Processes.Value
	}
	timeout, d := validateDurationField("spec.resources.timeout", res.Timeout, MinTimeoutMillis, MaxTimeoutMillis)
	diags = append(diags, d...)
	out.Resources.TimeoutMillis = timeout
	return diags
}

func validateNetwork(network Network, out *ValidatedPipeline) Diagnostics {
	var diags Diagnostics
	if !network.Present || network.Null {
		return Diagnostics{newDiagnostic(CodeMissingRequiredField, "spec.network", network.Line, network.Column, "spec.network is required")}
	}
	if missingString(network.Mode) {
		diags = append(diags, missingDiag("spec.network.mode", network.Mode, "network.mode is required"))
	} else if network.Mode.Value != NetworkModeDeny {
		diags = append(diags, diagForString(CodeInvalidValue, "spec.network.mode", network.Mode, "only deny networking is supported in v1alpha1"))
	} else {
		out.Network.Mode = network.Mode.Value
	}
	if !network.Allow.Present || network.Allow.Null {
		diags = append(diags, newDiagnostic(CodeMissingRequiredField, "spec.network.allow", network.Allow.Line, network.Allow.Column, "network.allow must be an explicit empty array"))
	} else if network.AllowLen != 0 {
		diags = append(diags, newDiagnostic(CodeInvalidValue, "spec.network.allow", network.Allow.Line, network.Allow.Column, "network.allow must be empty for deny mode"))
	} else {
		out.Network.Allow = []string{}
	}
	return diags
}

func validateScenarios(spec Spec, globalTimeout int64, out *ValidatedPipeline) Diagnostics {
	var diags Diagnostics
	if !spec.ScenariosPresent || spec.ScenariosNull {
		return Diagnostics{newDiagnostic(CodeMissingRequiredField, "spec.scenarios", spec.ScenariosLine, spec.ScenariosColumn, "spec.scenarios is required")}
	}
	if len(spec.Scenarios) < MinScenarioCount {
		diags = append(diags, newDiagnostic(CodeOutOfRange, "spec.scenarios", spec.ScenariosLine, spec.ScenariosColumn, "at least one scenario is required"))
	}
	if len(spec.Scenarios) > MaxScenarioCount {
		diags = append(diags, newDiagnostic(CodeOutOfRange, "spec.scenarios", spec.ScenariosLine, spec.ScenariosColumn, fmt.Sprintf("scenarios must not exceed %d", MaxScenarioCount)))
	}
	seen := make(map[string]struct{}, len(spec.Scenarios))
	for i, scenario := range spec.Scenarios {
		path := fmt.Sprintf("spec.scenarios[%d]", i)
		if scenario.Null {
			diags = append(diags, newDiagnostic(CodeMissingRequiredField, path, scenario.Line, scenario.Column, "scenario must be an object"))
			continue
		}
		validated := ValidatedScenario{}
		if missingString(scenario.ID) {
			diags = append(diags, missingDiag(path+".id", scenario.ID, "scenario id is required"))
		} else if err := validateIdentifier(scenario.ID.Value); err != nil {
			diags = append(diags, diagForString(CodeInvalidValue, path+".id", scenario.ID, "scenario id must match ^[a-z][a-z0-9._-]{0,63}$"))
		} else if _, ok := seen[scenario.ID.Value]; ok {
			diags = append(diags, diagForString(CodeDuplicateScenarioID, path+".id", scenario.ID, "scenario ids must be unique"))
		} else {
			seen[scenario.ID.Value] = struct{}{}
			validated.ID = scenario.ID.Value
		}
		if missingString(scenario.Name) {
			diags = append(diags, missingDiag(path+".name", scenario.Name, "scenario name is required"))
		} else if err := validateScenarioName(scenario.Name.Value); err != nil {
			diags = append(diags, diagForString(CodeInvalidValue, path+".name", scenario.Name, err.Error()))
		} else {
			validated.Name = scenario.Name.Value
		}
		if missingString(scenario.Shell) {
			diags = append(diags, missingDiag(path+".shell", scenario.Shell, "scenario shell is required"))
		} else if !isAllowedShell(scenario.Shell.Value) {
			diags = append(diags, diagForString(CodeInvalidValue, path+".shell", scenario.Shell, "shell must be /bin/sh, /bin/bash, or /usr/bin/bash without arguments"))
		} else {
			validated.Shell = scenario.Shell.Value
		}
		if !scenario.Run.Present || scenario.Run.Null {
			diags = append(diags, missingDiag(path+".run", scenario.Run, "scenario run is required"))
		} else if err := validateRun(scenario.Run.Value); err != nil {
			code := CodeInvalidValue
			if len(scenario.Run.Value) > MaxRunBytes {
				code = CodeOutOfRange
			}
			diags = append(diags, diagForString(code, path+".run", scenario.Run, err.Error()))
		} else {
			validated.Run = scenario.Run.Value
		}
		timeout, d := validateDurationField(path+".timeout", scenario.Timeout, MinTimeoutMillis, MaxTimeoutMillis)
		diags = append(diags, d...)
		if len(d) == 0 && globalTimeout > 0 && timeout > globalTimeout {
			diags = append(diags, diagForString(CodeCrossFieldConstraint, path+".timeout", scenario.Timeout, "scenario timeout must not exceed global timeout"))
		}
		validated.TimeoutMillis = timeout
		out.Scenarios = append(out.Scenarios, validated)
	}
	return diags
}

func validateCollect(collect Collect, out *ValidatedPipeline) Diagnostics {
	var diags Diagnostics
	if !collect.Present || collect.Null {
		return Diagnostics{newDiagnostic(CodeMissingRequiredField, "spec.collect", collect.Line, collect.Column, "spec.collect is required")}
	}
	fs := collect.Filesystem
	if !fs.Present || fs.Null {
		diags = append(diags, newDiagnostic(CodeMissingRequiredField, "spec.collect.filesystem", fs.Line, fs.Column, "collect.filesystem is required"))
	} else {
		if !fs.RootsPresent || fs.RootsNull {
			diags = append(diags, newDiagnostic(CodeMissingRequiredField, "spec.collect.filesystem.roots", fs.RootsLine, fs.RootsColumn, "filesystem roots are required"))
		} else {
			if len(fs.Roots) > MaxFilesystemRootCount {
				diags = append(diags, newDiagnostic(CodeOutOfRange, "spec.collect.filesystem.roots", fs.RootsLine, fs.RootsColumn, fmt.Sprintf("filesystem roots must not exceed %d", MaxFilesystemRootCount)))
			}
			seen := make(map[string]struct{}, len(fs.Roots))
			for i, root := range fs.Roots {
				p := fmt.Sprintf("spec.collect.filesystem.roots[%d]", i)
				if missingString(root) {
					diags = append(diags, missingDiag(p, root, "filesystem root must be a path"))
					continue
				}
				if err := validateAbsoluteLexicalPath(root.Value); err != nil {
					diags = append(diags, diagForString(CodeInvalidPath, p, root, "filesystem root must be an absolute clean POSIX path"))
					continue
				}
				if _, ok := seen[root.Value]; ok {
					diags = append(diags, diagForString(CodeInvalidPath, p, root, "filesystem roots must not contain duplicates"))
					continue
				}
				seen[root.Value] = struct{}{}
				out.Collect.FilesystemRoots = append(out.Collect.FilesystemRoots, root.Value)
			}
		}
		if missingString(fs.Contents) {
			diags = append(diags, missingDiag("spec.collect.filesystem.contents", fs.Contents, "filesystem contents mode is required"))
		} else if fs.Contents.Value != FilesystemContentsMetadata {
			diags = append(diags, diagForString(CodeInvalidValue, "spec.collect.filesystem.contents", fs.Contents, "only metadata-and-digests is supported"))
		} else {
			out.Collect.FilesystemContents = fs.Contents.Value
		}
	}
	if !collect.ArtifactsPresent || collect.ArtifactsNull {
		diags = append(diags, newDiagnostic(CodeMissingRequiredField, "spec.collect.artifacts", collect.ArtifactsLine, collect.ArtifactsColumn, "collect.artifacts is required"))
	} else {
		if len(collect.Artifacts) > MaxArtifactCount {
			diags = append(diags, newDiagnostic(CodeOutOfRange, "spec.collect.artifacts", collect.ArtifactsLine, collect.ArtifactsColumn, fmt.Sprintf("artifacts must not exceed %d", MaxArtifactCount)))
		}
		seen := make(map[string]struct{}, len(collect.Artifacts))
		for i, artifact := range collect.Artifacts {
			p := fmt.Sprintf("spec.collect.artifacts[%d]", i)
			if artifact.Null {
				diags = append(diags, newDiagnostic(CodeMissingRequiredField, p, artifact.Line, artifact.Column, "artifact entry must be an object"))
				continue
			}
			if missingString(artifact.Path) {
				diags = append(diags, missingDiag(p+".path", artifact.Path, "artifact path is required"))
			} else if err := validateArtifactGlob(artifact.Path.Value); err != nil {
				diags = append(diags, diagForString(CodeInvalidPath, p+".path", artifact.Path, "artifact path must be an absolute supported POSIX glob"))
			} else if _, ok := seen[artifact.Path.Value]; ok {
				diags = append(diags, diagForString(CodeInvalidPath, p+".path", artifact.Path, "artifact paths must not contain duplicates"))
			} else {
				seen[artifact.Path.Value] = struct{}{}
			}
			maxBytes, d := validateSizeField(p+".maxBytes", artifact.MaxBytes, MinArtifactBytes, MaxArtifactBytes)
			diags = append(diags, d...)
			out.Collect.Artifacts = append(out.Collect.Artifacts, ValidatedArtifact{Path: artifact.Path.Value, MaxBytes: maxBytes})
		}
	}
	logs := collect.Logs
	if !logs.Present || logs.Null {
		diags = append(diags, newDiagnostic(CodeMissingRequiredField, "spec.collect.logs", logs.Line, logs.Column, "collect.logs is required"))
	} else {
		maxBytes, d := validateSizeField("spec.collect.logs.maxBytesPerStream", logs.MaxBytesPerStream, MinLogBytesPerStream, MaxLogBytesPerStream)
		diags = append(diags, d...)
		out.Collect.LogMaxBytesPerStream = maxBytes
	}
	return diags
}

func validateCompare(compare Compare, out *ValidatedPipeline) Diagnostics {
	var diags Diagnostics
	if !compare.Present || compare.Null {
		return Diagnostics{newDiagnostic(CodeMissingRequiredField, "spec.compare", compare.Line, compare.Column, "spec.compare is required")}
	}
	if !compare.IgnorePresent || compare.IgnoreNull {
		diags = append(diags, newDiagnostic(CodeMissingRequiredField, "spec.compare.ignore", compare.IgnoreLine, compare.IgnoreColumn, "compare.ignore is required"))
	} else {
		if len(compare.Ignore) > MaxCompareIgnoreCount {
			diags = append(diags, newDiagnostic(CodeOutOfRange, "spec.compare.ignore", compare.IgnoreLine, compare.IgnoreColumn, fmt.Sprintf("compare ignore entries must not exceed %d", MaxCompareIgnoreCount)))
		}
		seen := make(map[string]struct{}, len(compare.Ignore))
		for i, ignore := range compare.Ignore {
			p := fmt.Sprintf("spec.compare.ignore[%d].field", i)
			if ignore.Null || missingString(ignore.Field) {
				diags = append(diags, newDiagnostic(CodeMissingRequiredField, p, ignore.Line, ignore.Column, "ignore field is required"))
				continue
			}
			if ignore.Field.Value != CompareIgnoreEventTimestamp && ignore.Field.Value != CompareIgnoreProcessPID {
				diags = append(diags, diagForString(CodeInvalidValue, p, ignore.Field, "unsupported compare ignore field"))
				continue
			}
			if _, ok := seen[ignore.Field.Value]; ok {
				diags = append(diags, diagForString(CodeInvalidValue, p, ignore.Field, "compare ignore fields must not contain duplicates"))
				continue
			}
			seen[ignore.Field.Value] = struct{}{}
			out.Compare.IgnoreFields = append(out.Compare.IgnoreFields, ignore.Field.Value)
		}
	}
	if missingInt(compare.Repetitions) {
		diags = append(diags, missingIntDiag("spec.compare.repetitions", compare.Repetitions, "compare.repetitions is required"))
	} else if compare.Repetitions.Value < MinRepetitions || compare.Repetitions.Value > MaxRepetitions {
		diags = append(diags, diagForInt(CodeOutOfRange, "spec.compare.repetitions", compare.Repetitions, fmt.Sprintf("repetitions must be between %d and %d", MinRepetitions, MaxRepetitions)))
	} else {
		out.Compare.Repetitions = compare.Repetitions.Value
	}
	return diags
}

func validatePolicy(policy Policy, out *ValidatedPipeline) Diagnostics {
	if !policy.Present || policy.Null {
		return Diagnostics{newDiagnostic(CodeMissingRequiredField, "spec.policy", policy.Line, policy.Column, "spec.policy is required")}
	}
	if missingString(policy.Profile) {
		return Diagnostics{missingDiag("spec.policy.profile", policy.Profile, "policy.profile is required")}
	}
	if policy.Profile.Value != PolicyProfileStrict {
		return Diagnostics{diagForString(CodeInvalidValue, "spec.policy.profile", policy.Profile, "only strict policy profile is supported")}
	}
	out.Policy.Profile = policy.Profile.Value
	return nil
}

func validateSizeField(path string, field StringValue, min, max int64) (int64, Diagnostics) {
	if missingString(field) {
		return 0, Diagnostics{missingDiag(path, field, path+" is required")}
	}
	value, err := ParseSizeBytes(field.Value)
	if err != nil {
		return 0, Diagnostics{diagForString(CodeInvalidUnit, path, field, "size must use a positive integer and one of B, KiB, MiB, GiB, TiB")}
	}
	if value < min || value > max {
		return value, Diagnostics{diagForString(CodeOutOfRange, path, field, fmt.Sprintf("size must be between %d and %d bytes", min, max))}
	}
	return value, nil
}

func validateDurationField(path string, field StringValue, min, max int64) (int64, Diagnostics) {
	if missingString(field) {
		return 0, Diagnostics{missingDiag(path, field, path+" is required")}
	}
	value, err := ParseDurationMillis(field.Value)
	if err != nil {
		return 0, Diagnostics{diagForString(CodeInvalidUnit, path, field, "duration must use a positive integer and one of ms, s, m, h")}
	}
	if value < min || value > max {
		return value, Diagnostics{diagForString(CodeOutOfRange, path, field, fmt.Sprintf("duration must be between %d and %d milliseconds", min, max))}
	}
	return value, nil
}

func validateIdentifier(s string) error {
	if len(s) == 0 || len(s) > MaxIdentifierBytes || !identifierPattern.MatchString(s) {
		return errInvalidValue{}
	}
	return nil
}

func validateScenarioName(s string) error {
	if len(s) == 0 || len(s) > MaxScenarioNameBytes || !utf8.ValidString(s) {
		return errInvalidValue{msg: "scenario name length is invalid"}
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return errInvalidValue{msg: "scenario name must not contain control characters"}
		}
	}
	return nil
}

func validateImmutableImage(image string) (string, error) {
	if image == "" || len(image) > MaxImageBytes || strings.Contains(image, "://") || strings.Contains(image, "REPLACE_WITH_REAL_DIGEST") {
		return "", errInvalidValue{msg: "image must be a non-empty immutable reference with @sha256 digest"}
	}
	for _, r := range image {
		if r <= 0x20 || r == 0x7f {
			return "", errInvalidValue{msg: "image must not contain whitespace or control characters"}
		}
	}
	idx := strings.LastIndex(image, "@sha256:")
	if idx < 0 {
		return "", errInvalidValue{msg: "image must include @sha256 digest"}
	}
	digest := image[idx+len("@sha256:"):]
	if len(digest) != 64 {
		return "", errInvalidValue{msg: "sha256 digest must contain exactly 64 lowercase hex characters"}
	}
	for _, r := range digest {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return "", errInvalidValue{msg: "sha256 digest must contain exactly 64 lowercase hex characters"}
		}
	}
	return digest, nil
}

func isAllowedShell(shell string) bool {
	if strings.ContainsAny(shell, " \t\n\r") {
		return false
	}
	switch shell {
	case ShellBinSH, ShellBinBash, ShellUsrBinBash:
		return true
	default:
		return false
	}
}

func validateRun(run string) error {
	if run == "" {
		return errInvalidValue{msg: "run must be non-empty"}
	}
	if len(run) > MaxRunBytes {
		return errInvalidValue{msg: fmt.Sprintf("run must not exceed %d bytes", MaxRunBytes)}
	}
	if strings.ContainsRune(run, 0) {
		return errInvalidValue{msg: "run must not contain NUL bytes"}
	}
	return nil
}

func missingString(v StringValue) bool { return !v.Present || v.Null || v.Value == "" }

func missingInt(v IntValue) bool { return !v.Present || v.Null }

func missingDiag(path string, v StringValue, msg string) Diagnostic {
	return newDiagnostic(CodeMissingRequiredField, path, v.Line, v.Column, msg)
}

func missingIntDiag(path string, v IntValue, msg string) Diagnostic {
	return newDiagnostic(CodeMissingRequiredField, path, v.Line, v.Column, msg)
}

func diagForString(code Code, path string, v StringValue, msg string) Diagnostic {
	return newDiagnostic(code, path, v.Line, v.Column, msg)
}

func diagForInt(code Code, path string, v IntValue, msg string) Diagnostic {
	return newDiagnostic(code, path, v.Line, v.Column, msg)
}

type errInvalidValue struct{ msg string }

func (e errInvalidValue) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return "invalid value"
}
