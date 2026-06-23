package waiver

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

type strictLimit struct{ limits Limits }

func (s strictLimit) CheckDepth(depth int, _ *yaml.DepthContext) error {
	if depth > s.limits.MaxYAMLDepth {
		return errors.New("waiver yaml depth limit exceeded")
	}
	return nil
}
func (s strictLimit) CheckAlias(aliasCount, _ int) error {
	if aliasCount > 0 {
		return errors.New("waiver yaml aliases are not supported")
	}
	return nil
}

type parsedDoc struct {
	apiVersion, kind, metadataName string
	waivers                        []parsedWaiver
}
type parsedWaiver struct{ id, findingID, ruleID, owner, reason, issuedAt, expiresAt string }

func Parse(data []byte, limits Limits) (WaiverSet, error) {
	l, err := validateLimits(limits)
	if err != nil {
		return WaiverSet{}, err
	}
	if err := preflight(data, l); err != nil {
		return WaiverSet{}, err
	}
	n, err := loadNode(data, l)
	if err != nil {
		return WaiverSet{}, err
	}
	if n == nil {
		return WaiverSet{}, errCode(CodeYAMLSyntax, "parse", "empty document")
	}
	if err := inspectNode(n, "", 1, l, new(int)); err != nil {
		return WaiverSet{}, err
	}
	p, err := decodeDoc(n)
	if err != nil {
		return WaiverSet{}, err
	}
	set, err := validateParsed(p, l)
	if err != nil {
		return WaiverSet{}, err
	}
	set.RawDigest = rawDigest(data)
	set.RawSizeBytes = int64(len(data))
	set.SemanticDigest = semanticDigest(set)
	return cloneSet(set), nil
}

func preflight(data []byte, l Limits) error {
	if int64(len(data)) > l.MaxWaiverFileBytes {
		return errCode(CodeInputTooLarge, "preflight", "waiver file exceeds byte limit")
	}
	if !utf8.Valid(data) {
		return errCode(CodeInvalidUTF8, "preflight", "input must be valid UTF-8")
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return errCode(CodeNULByte, "preflight", "input must not contain NUL")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return errCode(CodeYAMLSyntax, "preflight", "input must contain one YAML document")
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "%YAML") || strings.HasPrefix(trimmed, "%TAG") {
			return errCode(CodeUnsupportedYAMLFeature, "preflight", "YAML directives are not supported")
		}
	}
	return nil
}

func loadNode(data []byte, l Limits) (*yaml.Node, error) {
	var docs []yaml.Node
	err := yaml.Load(data, &docs, yaml.WithAllDocuments(), yaml.WithKnownFields(), yaml.WithUniqueKeys(), yaml.WithPlugin(strictLimit{l}))
	if err != nil {
		return nil, classifyYAMLError(err)
	}
	if len(docs) != 1 {
		return nil, errCode(CodeMultipleDocuments, "parse", fmt.Sprintf("expected one document, got %d", len(docs)))
	}
	root := &docs[0]
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil, nil
		}
		return root.Content[0], nil
	}
	return root, nil
}
func classifyYAMLError(err error) error {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "duplicate"):
		return errCode(CodeDuplicateKey, "parse", "duplicate mapping key")
	case strings.Contains(msg, "alias") || strings.Contains(msg, "anchor"):
		return errCode(CodeUnsupportedYAMLFeature, "parse", "aliases and anchors are not supported")
	case strings.Contains(msg, "depth"):
		return errCode(CodeUnsupportedYAMLFeature, "parse", "YAML depth exceeded")
	default:
		return wrapCode(CodeYAMLSyntax, "parse", "YAML syntax error", err)
	}
}

func inspectNode(n *yaml.Node, path string, depth int, l Limits, count *int) error {
	if n == nil {
		return nil
	}
	*count++
	if *count > l.MaxYAMLNodes {
		return errCode(CodeUnsupportedYAMLFeature, "parse", "YAML node limit exceeded")
	}
	if depth > l.MaxYAMLDepth {
		return errCode(CodeUnsupportedYAMLFeature, "parse", "YAML depth exceeded")
	}
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return errCode(CodeUnsupportedYAMLFeature, "parse", "aliases and anchors are not supported")
	}
	if n.Style&yaml.TaggedStyle != 0 || isCustomTag(n.Tag) {
		return errCode(CodeUnsupportedYAMLFeature, "parse", "YAML tags are not supported")
	}
	if n.Kind == yaml.ScalarNode && len(n.Value) > l.MaxScalarBytes {
		return errCode(CodeInvalidValue, "parse", "scalar exceeds byte limit")
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]struct{}{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
				return errCode(CodeUnsupportedYAMLFeature, "parse", "mapping keys must be strings")
			}
			if k.Value == "<<" {
				return errCode(CodeUnsupportedYAMLFeature, "parse", "merge keys are not supported")
			}
			if _, ok := seen[k.Value]; ok {
				return errCode(CodeDuplicateKey, "parse", "duplicate mapping key")
			}
			seen[k.Value] = struct{}{}
			if err := inspectNode(v, child(path, k.Value), depth+1, l, count); err != nil {
				return err
			}
		}
	}
	if n.Kind == yaml.SequenceNode {
		for i, c := range n.Content {
			_ = i
			if err := inspectNode(c, path, depth+1, l, count); err != nil {
				return err
			}
		}
	}
	return nil
}
func isCustomTag(tag string) bool {
	switch tag {
	case "", "!!map", "!!seq", "!!str", "!!int", "!!null", "!!bool":
		return false
	default:
		return strings.HasPrefix(tag, "!") || tag == "!!timestamp"
	}
}
func child(p, k string) string {
	if p == "" {
		return k
	}
	return p + "." + k
}

func decodeDoc(n *yaml.Node) (parsedDoc, error) {
	fields, err := mapping(n, "", []string{"apiVersion", "kind", "metadata", "spec"})
	if err != nil {
		return parsedDoc{}, err
	}
	api, err := reqString(fields, "apiVersion", "apiVersion")
	if err != nil {
		return parsedDoc{}, err
	}
	kind, err := reqString(fields, "kind", "kind")
	if err != nil {
		return parsedDoc{}, err
	}
	mf, err := mapping(fields["metadata"], "metadata", []string{"name"})
	if err != nil {
		return parsedDoc{}, err
	}
	name, err := reqString(mf, "name", "metadata.name")
	if err != nil {
		return parsedDoc{}, err
	}
	sf, err := mapping(fields["spec"], "spec", []string{"waivers"})
	if err != nil {
		return parsedDoc{}, err
	}
	waiverNode := sf["waivers"]
	if waiverNode == nil || isNull(waiverNode) || waiverNode.Kind != yaml.SequenceNode {
		return parsedDoc{}, errCode(CodeMissingRequiredField, "decode", "spec.waivers is required")
	}
	out := parsedDoc{apiVersion: api, kind: kind, metadataName: name}
	for _, item := range waiverNode.Content {
		wf, err := mapping(item, "spec.waivers[]", []string{"id", "target", "owner", "reason", "issuedAt", "expiresAt"})
		if err != nil {
			return parsedDoc{}, err
		}
		tf, err := mapping(wf["target"], "target", []string{"findingId", "ruleId"})
		if err != nil {
			return parsedDoc{}, err
		}
		id, err := reqString(wf, "id", "waiver.id")
		if err != nil {
			return parsedDoc{}, err
		}
		fid, err := reqString(tf, "findingId", "target.findingId")
		if err != nil {
			return parsedDoc{}, err
		}
		rid, err := reqString(tf, "ruleId", "target.ruleId")
		if err != nil {
			return parsedDoc{}, err
		}
		owner, err := reqString(wf, "owner", "owner")
		if err != nil {
			return parsedDoc{}, err
		}
		reason, err := reqString(wf, "reason", "reason")
		if err != nil {
			return parsedDoc{}, err
		}
		issued, err := reqString(wf, "issuedAt", "issuedAt")
		if err != nil {
			return parsedDoc{}, err
		}
		expires, err := reqString(wf, "expiresAt", "expiresAt")
		if err != nil {
			return parsedDoc{}, err
		}
		out.waivers = append(out.waivers, parsedWaiver{id: id, findingID: fid, ruleID: rid, owner: owner, reason: reason, issuedAt: issued, expiresAt: expires})
	}
	return out, nil
}
func mapping(n *yaml.Node, path string, allowed []string) (map[string]*yaml.Node, error) {
	if n == nil || isNull(n) || n.Kind != yaml.MappingNode {
		return nil, errCode(CodeMissingRequiredField, "decode", path+" mapping is required")
	}
	allow := map[string]struct{}{}
	for _, a := range allowed {
		allow[a] = struct{}{}
	}
	out := map[string]*yaml.Node{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if _, ok := allow[k.Value]; !ok {
			return nil, errCode(CodeUnknownField, "decode", "unknown field")
		}
		out[k.Value] = v
	}
	return out, nil
}
func reqString(fields map[string]*yaml.Node, key, path string) (string, error) {
	n := fields[key]
	if n == nil || isNull(n) {
		return "", errCode(CodeMissingRequiredField, "decode", path+" is required")
	}
	if n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
		return "", errCode(CodeInvalidValue, "decode", path+" must be string")
	}
	return n.Value, nil
}
func isNull(n *yaml.Node) bool { return n != nil && n.Kind == yaml.ScalarNode && n.Tag == "!!null" }

func validateParsed(p parsedDoc, l Limits) (WaiverSet, error) {
	if p.apiVersion != APIVersionV1Alpha1 {
		return WaiverSet{}, errCode(CodeInvalidAPIVersion, "validate", "invalid apiVersion")
	}
	if p.kind != KindWaiverSet {
		return WaiverSet{}, errCode(CodeInvalidKind, "validate", "invalid kind")
	}
	if p.metadataName != "default" {
		return WaiverSet{}, errCode(CodeInvalidValue, "validate", "metadata.name must be default")
	}
	if int64(len(p.waivers)) > l.MaxWaivers {
		return WaiverSet{}, errCode(CodeInputTooLarge, "validate", "too many waivers")
	}
	set := WaiverSet{APIVersion: p.apiVersion, Kind: p.kind, MetadataName: p.metadataName, Waivers: []Waiver{}}
	ids := map[string]struct{}{}
	targets := map[string]struct{}{}
	for _, pw := range p.waivers {
		w, err := validateWaiver(pw, l)
		if err != nil {
			return WaiverSet{}, err
		}
		if _, ok := ids[w.ID]; ok {
			return WaiverSet{}, errCode(CodeDuplicateWaiverID, "validate", w.ID)
		}
		ids[w.ID] = struct{}{}
		tk := w.Target.FindingID + "\x00" + w.Target.RuleID
		if _, ok := targets[tk]; ok {
			return WaiverSet{}, errCode(CodeDuplicateWaiverTarget, "validate", w.Target.FindingID)
		}
		for existing := range targets {
			if strings.HasPrefix(existing, w.Target.FindingID+"\x00") {
				return WaiverSet{}, errCode(CodeDuplicateWaiverTarget, "validate", w.Target.FindingID)
			}
		}
		targets[tk] = struct{}{}
		set.Waivers = append(set.Waivers, w)
	}
	sort.SliceStable(set.Waivers, func(i, j int) bool { return set.Waivers[i].ID < set.Waivers[j].ID })
	return set, nil
}
func validateWaiver(p parsedWaiver, l Limits) (Waiver, error) {
	if !validWaiverID(p.id) {
		return Waiver{}, errCode(CodeInvalidWaiverID, "validate", p.id)
	}
	if !validFindingID(p.findingID) {
		return Waiver{}, errCode(CodeInvalidFindingID, "validate", p.id)
	}
	if !eligibleRuleSyntax(p.ruleID) {
		return Waiver{}, errCode(CodeInvalidRuleID, "validate", p.id)
	}
	if err := validateText(p.owner, 1, l.MaxOwnerBytes, false); err != nil {
		return Waiver{}, errCode(CodeInvalidOwner, "validate", p.id)
	}
	if err := validateText(p.reason, 1, l.MaxReasonBytes, false); err != nil {
		return Waiver{}, errCode(CodeInvalidReason, "validate", p.id)
	}
	issued, err := parseWaiverTime(p.issuedAt)
	if err != nil {
		return Waiver{}, errCode(CodeInvalidTime, "validate", p.id)
	}
	expires, err := parseWaiverTime(p.expiresAt)
	if err != nil {
		return Waiver{}, errCode(CodeInvalidTime, "validate", p.id)
	}
	if !issued.Before(expires) {
		return Waiver{}, errCode(CodeInvalidLifetime, "validate", p.id)
	}
	if expires.Sub(issued) > time.Duration(l.MaxLifetimeDays)*24*time.Hour {
		return Waiver{}, errCode(CodeInvalidLifetime, "validate", p.id)
	}
	return Waiver{ID: p.id, Target: Target{FindingID: p.findingID, RuleID: p.ruleID}, Owner: p.owner, Reason: p.reason, IssuedAt: issued, ExpiresAt: expires}, nil
}
func validWaiverID(s string) bool {
	if len(s) < 1 || len(s) > 64 || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}
func validFindingID(s string) bool {
	if len(s) != 72 || !strings.HasPrefix(s, "finding-") {
		return false
	}
	for _, c := range s[8:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func eligibleRuleSyntax(rule string) bool {
	switch rule {
	case "GR-PROC-001", "GR-FS-001", "GR-FS-002", "GR-NET-001", "GR-ART-001", "GR-DET-001", "GR-LIMIT-001":
		return true
	default:
		return false
	}
}
func validateText(s string, min, max int, _ bool) error {
	if len(s) < min || len(s) > max || !utf8.ValidString(s) {
		return errors.New("invalid text")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return errors.New("control")
		}
	}
	return nil
}
func parseWaiverTime(s string) (time.Time, error) {
	if len(s) != 20 || !strings.HasSuffix(s, "Z") || strings.Contains(s, ".") {
		return time.Time{}, errors.New("invalid time")
	}
	t, err := time.Parse("2006-01-02T15:04:05Z", s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC().Round(0), nil
}

func cloneSet(in WaiverSet) WaiverSet { out := in; out.Waivers = cloneWaivers(in.Waivers); return out }
func cloneWaivers(in []Waiver) []Waiver {
	if len(in) == 0 {
		return []Waiver{}
	}
	out := make([]Waiver, len(in))
	copy(out, in)
	return out
}
