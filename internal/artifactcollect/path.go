package artifactcollect

import (
	"context"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/model"
)

func validateWorkspacePath(p string, limits Limits) error {
	if p == "" || len(p) > limits.MaxWorkspacePathBytes || !utf8.ValidString(p) {
		return errCode(CodeInvalidWorkspacePath, "bind", "", "workspace path is invalid", nil)
	}
	if hasControls(p) || strings.Contains(p, "\x00") {
		return errCode(CodeInvalidWorkspacePath, "bind", "", "workspace path contains invalid characters", nil)
	}
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return errCode(CodeInvalidWorkspacePath, "bind", "", "workspace path must be absolute and clean", nil)
	}
	return nil
}

func validateDigest(d model.Digest) bool {
	s := string(d)
	if len(s) != len("sha256:")+64 || !strings.HasPrefix(s, "sha256:") {
		return false
	}
	for _, c := range s[len("sha256:"):] {
		if c < '0' || (c > '9' && c < 'a') || c > 'f' {
			return false
		}
	}
	return true
}

func validatePlan(ctx context.Context, plan CollectionPlan, limits Limits) (validatedPlan, error) {
	if err := checkContext(ctx, "plan"); err != nil {
		return validatedPlan{}, err
	}
	if !validateDigest(plan.PlanDigest) {
		return validatedPlan{}, errCode(CodeInvalidCollectionPlan, "plan", "", "plan digest is invalid", nil)
	}
	if err := validateAttempt(plan.Attempt); err != nil {
		return validatedPlan{}, err
	}
	workdir, err := validateWorkdir(plan.Workdir, limits)
	if err != nil {
		return validatedPlan{}, err
	}
	if len(plan.Rules) > limits.MaxArtifactRules {
		return validatedPlan{}, errCode(CodeInvalidCollectionPlan, "plan", "", "too many artifact rules", nil)
	}
	seenPatterns := map[string]struct{}{}
	seenIDs := map[string]struct{}{}
	rules := make([]validatedRule, 0, len(plan.Rules))
	for i, rule := range plan.Rules {
		if err := validateRuleID(rule.ID); err != nil {
			return validatedPlan{}, err
		}
		if _, ok := seenIDs[rule.ID]; ok {
			return validatedPlan{}, errCode(CodeInvalidCollectionPlan, "plan", "", "duplicate artifact rule id", nil)
		}
		seenIDs[rule.ID] = struct{}{}
		if _, ok := seenPatterns[rule.Pattern]; ok {
			return validatedPlan{}, errCode(CodeDuplicateArtifactPattern, "plan", "", "duplicate artifact pattern", nil)
		}
		seenPatterns[rule.Pattern] = struct{}{}
		vr, err := validateArtifactRule(i, rule, workdir, limits)
		if err != nil {
			return validatedPlan{}, err
		}
		rules = append(rules, vr)
	}
	return validatedPlan{PlanDigest: plan.PlanDigest, Attempt: plan.Attempt, Workdir: workdir, Rules: rules}, nil
}

func validateAttempt(a AttemptIdentity) error {
	if a.AttemptID == "" || len(a.AttemptID) > 512 || !utf8.ValidString(a.AttemptID) || hasControls(a.AttemptID) {
		return errCode(CodeInvalidAttempt, "plan", "", "attempt identity is invalid", nil)
	}
	if a.Revision != model.RevisionKindBase && a.Revision != model.RevisionKindHead {
		return errCode(CodeInvalidAttempt, "plan", "", "attempt revision is invalid", nil)
	}
	if a.ScenarioID == "" || len(a.ScenarioID) > 128 || !utf8.ValidString(a.ScenarioID) || hasControls(a.ScenarioID) {
		return errCode(CodeInvalidAttempt, "plan", "", "attempt scenario is invalid", nil)
	}
	if a.Repetition == 0 {
		return errCode(CodeInvalidAttempt, "plan", "", "attempt repetition is invalid", nil)
	}
	return nil
}

func validateRuleID(id string) error {
	if id == "" || len(id) > 128 || !utf8.ValidString(id) || hasControls(id) {
		return errCode(CodeInvalidCollectionPlan, "plan", "", "artifact rule id is invalid", nil)
	}
	return nil
}

func validateWorkdir(workdir string, limits Limits) (string, error) {
	if err := validatePOSIXPath(workdir, limits.MaxPathBytes, false); err != nil {
		return "", errCode(CodeInvalidWorkdir, "plan", "", "workdir must be an absolute clean POSIX path", err)
	}
	return workdir, nil
}

func validateArtifactRule(index int, rule ArtifactRule, workdir string, limits Limits) (validatedRule, error) {
	if rule.MaxBytes <= 0 || rule.MaxBytes > limits.MaxSingleArtifactBytes {
		return validatedRule{}, errCode(CodeInvalidCollectionPlan, "plan", "", "artifact maxBytes is outside supported bounds", nil)
	}
	if len(rule.Pattern) > limits.MaxPatternBytes {
		return validatedRule{}, errCode(CodeInvalidArtifactPattern, "plan", "", "artifact pattern exceeds limit", nil)
	}
	if err := validatePOSIXPath(rule.Pattern, limits.MaxPatternBytes, true); err != nil {
		return validatedRule{}, errCode(CodeInvalidArtifactPattern, "plan", "", "artifact pattern is invalid", err)
	}
	if !posixEqualOrBeneath(rule.Pattern, workdir) {
		return validatedRule{}, errCode(CodeArtifactOutsideWorkdir, "plan", "", "artifact pattern is outside the planned workdir", nil)
	}
	rel := ""
	if rule.Pattern != workdir {
		rel = strings.TrimPrefix(rule.Pattern, workdir+"/")
		if rel == rule.Pattern || rel == "" {
			return validatedRule{}, errCode(CodeArtifactOutsideWorkdir, "plan", "", "artifact pattern cannot be mapped beneath workdir", nil)
		}
	}
	comps := splitRel(rel)
	if len(comps) > limits.MaxPatternComponents {
		return validatedRule{}, errCode(CodePatternLimit, "plan", "", "artifact pattern has too many components", nil)
	}
	for _, comp := range comps {
		if comp == "**" {
			continue
		}
		if strings.Contains(comp, "**") {
			return validatedRule{}, errCode(CodeInvalidArtifactPattern, "plan", "", "** is supported only as a complete component", nil)
		}
		if _, err := path.Match(comp, ""); err != nil {
			return validatedRule{}, errCode(CodeInvalidArtifactPattern, "plan", "", "artifact pattern component is malformed", err)
		}
	}
	return validatedRule{Index: index, ID: rule.ID, Pattern: rule.Pattern, RelPattern: rel, MaxBytes: rule.MaxBytes}, nil
}

func validatePOSIXPath(p string, max int, allowGlob bool) error {
	if p == "" || len(p) > max || !utf8.ValidString(p) || hasControls(p) || strings.Contains(p, "\\") {
		return errCode(CodeInvalidEntryPath, "path", "", "path contains invalid characters", nil)
	}
	if !strings.HasPrefix(p, "/") {
		return errCode(CodeInvalidEntryPath, "path", "", "path must be absolute", nil)
	}
	if path.Clean(p) != p {
		return errCode(CodeInvalidEntryPath, "path", "", "path must be clean", nil)
	}
	for _, comp := range strings.Split(p, "/") {
		if comp == "." || comp == ".." || (!allowGlob && strings.ContainsAny(comp, "*?[]")) {
			return errCode(CodeInvalidEntryPath, "path", "", "path component is invalid", nil)
		}
	}
	return nil
}

func posixEqualOrBeneath(p, parent string) bool {
	if p == parent {
		return true
	}
	return strings.HasPrefix(p, parent+"/")
}

func validateInventoryRelativePath(rel string, limits Limits) error {
	if rel == "" || len(rel) > limits.MaxPathBytes || !utf8.ValidString(rel) || hasControls(rel) || strings.Contains(rel, "\\") {
		return errCode(CodeInvalidEntryPath, "inventory", "", "inventory path is invalid", nil)
	}
	if strings.HasPrefix(rel, "/") || path.Clean(rel) != rel {
		return errCode(CodeInvalidEntryPath, "inventory", "", "inventory path must be relative and clean", nil)
	}
	comps := strings.Split(rel, "/")
	if len(comps) > limits.MaxPathDepth {
		return errCode(CodeInvalidEntryPath, "inventory", "", "inventory path exceeds depth limit", nil)
	}
	for _, comp := range comps {
		if err := validateComponent(comp, limits); err != nil {
			return err
		}
	}
	return nil
}

func validateComponent(name string, limits Limits) error {
	if name == "" || name == "." || name == ".." || len(name) > limits.MaxPathComponentBytes || !utf8.ValidString(name) || hasControls(name) || strings.Contains(name, "\\") {
		return errCode(CodeInvalidEntryName, "inventory", "", "entry name is invalid", nil)
	}
	if isDotGit(name) {
		return errCode(CodeInvalidEntryPath, "inventory", "", "workspace .git entries are not accepted", nil)
	}
	return nil
}

func hasControls(s string) bool {
	for _, r := range s {
		if r == 0 || r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func isDotGit(name string) bool {
	return len(name) == 4 && strings.EqualFold(name, ".git")
}

func splitRel(rel string) []string {
	if rel == "" {
		return []string{}
	}
	return strings.Split(rel, "/")
}

func appendRel(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

type validatedPlan struct {
	PlanDigest model.Digest
	Attempt    AttemptIdentity
	Workdir    string
	Rules      []validatedRule
}

type validatedRule struct {
	Index      int
	ID         string
	Pattern    string
	RelPattern string
	MaxBytes   int64
}
