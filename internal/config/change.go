package config

import (
	"fmt"
	"sort"
)

type ChangeKind string

const (
	ChangeKindAdded     ChangeKind = "added"
	ChangeKindRemoved   ChangeKind = "removed"
	ChangeKindModified  ChangeKind = "modified"
	ChangeKindReordered ChangeKind = "reordered"
)

type SecurityEffect string

const (
	SecurityEffectPrivilegeIncrease         SecurityEffect = "privilege-increase"
	SecurityEffectPrivilegeDecrease         SecurityEffect = "privilege-decrease"
	SecurityEffectExecutionDefinitionChange SecurityEffect = "execution-definition-change"
	SecurityEffectObservationWeakened       SecurityEffect = "observation-weakened"
	SecurityEffectObservationStrengthened   SecurityEffect = "observation-strengthened"
	SecurityEffectPolicyChange              SecurityEffect = "policy-change"
	SecurityEffectInformational             SecurityEffect = "informational"
	SecurityEffectUnknown                   SecurityEffect = "unknown"
)

type ConfigChange struct {
	Path         string
	Kind         ChangeKind
	Effect       SecurityEffect
	Before       string
	After        string
	BeforeDigest string
	AfterDigest  string
	BeforeBytes  int
	AfterBytes   int
}

func (c ConfigChange) String() string {
	return fmt.Sprintf("%s %s %s", c.Path, c.Kind, c.Effect)
}

func ComparePipelines(base, head ValidatedPipeline) []ConfigChange {
	var changes []ConfigChange
	addStringChange(&changes, "metadata.name", base.Name, head.Name, SecurityEffectInformational)
	addStringChange(&changes, "spec.environment.image", base.Image, head.Image, SecurityEffectExecutionDefinitionChange)
	addStringChange(&changes, "spec.environment.imageDigest", base.ImageDigest, head.ImageDigest, SecurityEffectExecutionDefinitionChange)
	addStringChange(&changes, "spec.environment.workdir", base.Workdir, head.Workdir, SecurityEffectExecutionDefinitionChange)
	addIntDirectional(&changes, "spec.resources.cpu", base.Resources.CPU, head.Resources.CPU, SecurityEffectPrivilegeIncrease, SecurityEffectPrivilegeDecrease)
	addIntDirectional(&changes, "spec.resources.memoryBytes", base.Resources.MemoryBytes, head.Resources.MemoryBytes, SecurityEffectPrivilegeIncrease, SecurityEffectPrivilegeDecrease)
	addIntDirectional(&changes, "spec.resources.diskBytes", base.Resources.DiskBytes, head.Resources.DiskBytes, SecurityEffectPrivilegeIncrease, SecurityEffectPrivilegeDecrease)
	addIntDirectional(&changes, "spec.resources.processCount", base.Resources.ProcessCount, head.Resources.ProcessCount, SecurityEffectPrivilegeIncrease, SecurityEffectPrivilegeDecrease)
	addIntDirectional(&changes, "spec.resources.timeoutMillis", base.Resources.TimeoutMillis, head.Resources.TimeoutMillis, SecurityEffectPrivilegeIncrease, SecurityEffectPrivilegeDecrease)
	addStringChange(&changes, "spec.network.mode", base.Network.Mode, head.Network.Mode, networkEffect(base.Network.Mode, head.Network.Mode))
	compareStringList(&changes, "spec.network.allow", base.Network.Allow, head.Network.Allow, SecurityEffectPrivilegeIncrease, SecurityEffectPrivilegeDecrease)

	compareScenarios(&changes, base.Scenarios, head.Scenarios)
	compareStringList(&changes, "spec.collect.filesystem.roots", base.Collect.FilesystemRoots, head.Collect.FilesystemRoots, SecurityEffectObservationStrengthened, SecurityEffectObservationWeakened)
	addStringChange(&changes, "spec.collect.filesystem.contents", base.Collect.FilesystemContents, head.Collect.FilesystemContents, observationModeEffect(base.Collect.FilesystemContents, head.Collect.FilesystemContents))
	compareArtifacts(&changes, base.Collect.Artifacts, head.Collect.Artifacts)
	addIntDirectional(&changes, "spec.collect.logs.maxBytesPerStream", base.Collect.LogMaxBytesPerStream, head.Collect.LogMaxBytesPerStream, SecurityEffectObservationStrengthened, SecurityEffectObservationWeakened)
	compareIgnoreFields(&changes, base.Compare.IgnoreFields, head.Compare.IgnoreFields)
	addIntDirectional(&changes, "spec.compare.repetitions", base.Compare.Repetitions, head.Compare.Repetitions, SecurityEffectObservationStrengthened, SecurityEffectObservationWeakened)
	addStringChange(&changes, "spec.policy.profile", base.Policy.Profile, head.Policy.Profile, SecurityEffectPolicyChange)
	sortChanges(changes)
	return changes
}

func compareScenarios(changes *[]ConfigChange, base, head []ValidatedScenario) {
	baseOrder := make([]string, len(base))
	headOrder := make([]string, len(head))
	baseByID := make(map[string]ValidatedScenario, len(base))
	headByID := make(map[string]ValidatedScenario, len(head))
	for i, scenario := range base {
		baseOrder[i] = scenario.ID
		baseByID[scenario.ID] = scenario
	}
	for i, scenario := range head {
		headOrder[i] = scenario.ID
		headByID[scenario.ID] = scenario
	}
	for _, scenario := range base {
		if _, ok := headByID[scenario.ID]; !ok {
			*changes = append(*changes, ConfigChange{Path: scenarioPath(scenario.ID), Kind: ChangeKindRemoved, Effect: SecurityEffectExecutionDefinitionChange})
		}
	}
	for _, scenario := range head {
		if _, ok := baseByID[scenario.ID]; !ok {
			*changes = append(*changes, ConfigChange{Path: scenarioPath(scenario.ID), Kind: ChangeKindAdded, Effect: SecurityEffectExecutionDefinitionChange})
		}
	}
	if sameStringSet(baseOrder, headOrder) && !sameStringSlice(baseOrder, headOrder) {
		*changes = append(*changes, ConfigChange{Path: "spec.scenarios", Kind: ChangeKindReordered, Effect: SecurityEffectInformational})
	}
	common := commonScenarioKeysInBaseOrder(baseOrder, headByID)
	for _, id := range common {
		b := baseByID[id]
		h := headByID[id]
		prefix := scenarioPath(id)
		addStringChange(changes, prefix+".name", b.Name, h.Name, SecurityEffectInformational)
		addStringChange(changes, prefix+".shell", b.Shell, h.Shell, SecurityEffectExecutionDefinitionChange)
		addRunChange(changes, prefix+".run", b.Run, h.Run)
		addIntDirectional(changes, prefix+".timeoutMillis", b.TimeoutMillis, h.TimeoutMillis, SecurityEffectPrivilegeIncrease, SecurityEffectPrivilegeDecrease)
	}
}

func compareStringList(changes *[]ConfigChange, path string, base, head []string, addedEffect, removedEffect SecurityEffect) {
	minLen := len(base)
	if len(head) < minLen {
		minLen = len(head)
	}
	if sameStringSet(base, head) && !sameStringSlice(base, head) {
		*changes = append(*changes, ConfigChange{Path: path, Kind: ChangeKindReordered, Effect: SecurityEffectInformational})
		return
	}
	for i := 0; i < minLen; i++ {
		if base[i] != head[i] {
			*changes = append(*changes, ConfigChange{Path: fmt.Sprintf("%s[%d]", path, i), Kind: ChangeKindModified, Effect: SecurityEffectUnknown})
		}
	}
	for i := minLen; i < len(base); i++ {
		*changes = append(*changes, ConfigChange{Path: fmt.Sprintf("%s[%d]", path, i), Kind: ChangeKindRemoved, Effect: removedEffect})
	}
	for i := minLen; i < len(head); i++ {
		*changes = append(*changes, ConfigChange{Path: fmt.Sprintf("%s[%d]", path, i), Kind: ChangeKindAdded, Effect: addedEffect})
	}
}

func compareArtifacts(changes *[]ConfigChange, base, head []ValidatedArtifact) {
	baseOrder := make([]string, len(base))
	headOrder := make([]string, len(head))
	baseByPath := make(map[string]ValidatedArtifact, len(base))
	headByPath := make(map[string]ValidatedArtifact, len(head))
	for i, artifact := range base {
		baseOrder[i] = artifact.Path
		baseByPath[artifact.Path] = artifact
	}
	for i, artifact := range head {
		headOrder[i] = artifact.Path
		headByPath[artifact.Path] = artifact
	}
	for _, artifact := range base {
		if _, ok := headByPath[artifact.Path]; !ok {
			*changes = append(*changes, ConfigChange{Path: artifactPath(artifact.Path), Kind: ChangeKindRemoved, Effect: SecurityEffectObservationWeakened})
		}
	}
	for _, artifact := range head {
		if _, ok := baseByPath[artifact.Path]; !ok {
			*changes = append(*changes, ConfigChange{Path: artifactPath(artifact.Path), Kind: ChangeKindAdded, Effect: SecurityEffectObservationStrengthened})
		}
	}
	if sameStringSet(baseOrder, headOrder) && !sameStringSlice(baseOrder, headOrder) {
		*changes = append(*changes, ConfigChange{Path: "spec.collect.artifacts", Kind: ChangeKindReordered, Effect: SecurityEffectInformational})
	}
	for _, path := range commonArtifactKeysInBaseOrder(baseOrder, headByPath) {
		b := baseByPath[path]
		h := headByPath[path]
		addIntDirectional(changes, artifactPath(path)+".maxBytes", b.MaxBytes, h.MaxBytes, SecurityEffectObservationStrengthened, SecurityEffectObservationWeakened)
	}
}

func compareIgnoreFields(changes *[]ConfigChange, base, head []string) {
	baseSet := make(map[string]struct{}, len(base))
	headSet := make(map[string]struct{}, len(head))
	for _, field := range base {
		baseSet[field] = struct{}{}
	}
	for _, field := range head {
		headSet[field] = struct{}{}
	}
	for _, field := range base {
		if _, ok := headSet[field]; !ok {
			*changes = append(*changes, ConfigChange{Path: ignorePath(field), Kind: ChangeKindRemoved, Effect: SecurityEffectObservationStrengthened})
		}
	}
	for _, field := range head {
		if _, ok := baseSet[field]; !ok {
			*changes = append(*changes, ConfigChange{Path: ignorePath(field), Kind: ChangeKindAdded, Effect: SecurityEffectObservationWeakened})
		}
	}
	if sameStringSet(base, head) && !sameStringSlice(base, head) {
		*changes = append(*changes, ConfigChange{Path: "spec.compare.ignore", Kind: ChangeKindReordered, Effect: SecurityEffectInformational})
	}
}

func addStringChange(changes *[]ConfigChange, path, before, after string, effect SecurityEffect) {
	if before == after {
		return
	}
	*changes = append(*changes, ConfigChange{Path: path, Kind: ChangeKindModified, Effect: effect, Before: boundValue(before), After: boundValue(after)})
}

func addIntDirectional(changes *[]ConfigChange, path string, before, after int64, increaseEffect, decreaseEffect SecurityEffect) {
	if before == after {
		return
	}
	effect := decreaseEffect
	if after > before {
		effect = increaseEffect
	}
	*changes = append(*changes, ConfigChange{Path: path, Kind: ChangeKindModified, Effect: effect, Before: fmt.Sprintf("%d", before), After: fmt.Sprintf("%d", after)})
}

func addRunChange(changes *[]ConfigChange, path, before, after string) {
	if before == after {
		return
	}
	*changes = append(*changes, ConfigChange{
		Path:         path,
		Kind:         ChangeKindModified,
		Effect:       SecurityEffectExecutionDefinitionChange,
		BeforeDigest: string(digestBytes([]byte(before))),
		AfterDigest:  string(digestBytes([]byte(after))),
		BeforeBytes:  len(before),
		AfterBytes:   len(after),
	})
}

func networkEffect(before, after string) SecurityEffect {
	if before == after {
		return SecurityEffectInformational
	}
	if before == NetworkModeDeny && after != NetworkModeDeny {
		return SecurityEffectPrivilegeIncrease
	}
	if before != NetworkModeDeny && after == NetworkModeDeny {
		return SecurityEffectPrivilegeDecrease
	}
	return SecurityEffectUnknown
}

func observationModeEffect(before, after string) SecurityEffect {
	if before == after {
		return SecurityEffectInformational
	}
	if before == FilesystemContentsMetadata && after != FilesystemContentsMetadata {
		return SecurityEffectObservationWeakened
	}
	if before != FilesystemContentsMetadata && after == FilesystemContentsMetadata {
		return SecurityEffectObservationStrengthened
	}
	return SecurityEffectUnknown
}

func scenarioPath(id string) string { return "spec.scenarios[id=" + boundPathToken(id) + "]" }

func artifactPath(path string) string {
	return "spec.collect.artifacts[path=" + boundPathToken(path) + "]"
}

func ignorePath(field string) string {
	return "spec.compare.ignore[field=" + boundPathToken(field) + "]"
}

func boundPathToken(s string) string { return sanitizeForDiagnostic(s, 160) }

func boundValue(s string) string { return sanitizeForDiagnostic(s, 160) }

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}
	return true
}

func commonScenarioKeysInBaseOrder(baseOrder []string, head map[string]ValidatedScenario) []string {
	out := make([]string, 0, len(baseOrder))
	for _, key := range baseOrder {
		if _, ok := head[key]; ok {
			out = append(out, key)
		}
	}
	return out
}

func commonArtifactKeysInBaseOrder(baseOrder []string, head map[string]ValidatedArtifact) []string {
	out := make([]string, 0, len(baseOrder))
	for _, key := range baseOrder {
		if _, ok := head[key]; ok {
			out = append(out, key)
		}
	}
	return out
}

func sortChanges(changes []ConfigChange) {
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Path != changes[j].Path {
			return changes[i].Path < changes[j].Path
		}
		if changes[i].Kind != changes[j].Kind {
			return changes[i].Kind < changes[j].Kind
		}
		return changes[i].Effect < changes[j].Effect
	})
}

func cloneValidatedPipeline(in ValidatedPipeline) ValidatedPipeline {
	out := in
	out.Network.Allow = cloneStrings(in.Network.Allow)
	out.Scenarios = cloneScenarios(in.Scenarios)
	out.Collect.FilesystemRoots = cloneStrings(in.Collect.FilesystemRoots)
	out.Collect.Artifacts = cloneArtifacts(in.Collect.Artifacts)
	out.Compare.IgnoreFields = cloneStrings(in.Compare.IgnoreFields)
	return out
}

func cloneConfigChanges(in []ConfigChange) []ConfigChange {
	return append([]ConfigChange(nil), in...)
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	return append(in[:0:0], in...)
}

func cloneScenarios(in []ValidatedScenario) []ValidatedScenario {
	if in == nil {
		return nil
	}
	return append(in[:0:0], in...)
}

func cloneArtifacts(in []ValidatedArtifact) []ValidatedArtifact {
	if in == nil {
		return nil
	}
	return append(in[:0:0], in...)
}

func validatedPipelineFieldInventory() []string {
	return []string{
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
}
