package config

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"
)

var (
	errYAMLDepth = errors.New("glassroot yaml depth limit exceeded")
	errYAMLAlias = errors.New("glassroot yaml aliases are not supported")
)

type strictYAMLLimit struct{}

func (strictYAMLLimit) CheckDepth(depth int, _ *yaml.DepthContext) error {
	if depth > MaxYAMLDepth {
		return errYAMLDepth
	}
	return nil
}

func (strictYAMLLimit) CheckAlias(aliasCount, _ int) error {
	if aliasCount > 0 {
		return errYAMLAlias
	}
	return nil
}

func Parse(data []byte) (Document, error) {
	if diags := preflightInput(data); len(diags) > 0 {
		return Document{}, diags
	}

	docNode, err := loadSingleDocumentNode(data)
	if err != nil {
		return Document{}, classifyYAMLError(err)
	}
	if docNode == nil {
		return Document{}, Diagnostics{newDiagnostic(CodeYAMLSyntax, "", 0, 0, "input must contain one non-empty YAML document")}
	}

	if diags := inspectYAMLNode(docNode, "", 1, newNodeBudget()); len(diags) > 0 {
		return Document{}, capDiagnostics(diags)
	}
	if diags := inspectKnownFields(docNode, ""); len(diags) > 0 {
		return Document{}, capDiagnostics(diags)
	}

	doc, diags := decodeDocument(docNode)
	if len(diags) > 0 {
		return Document{}, capDiagnostics(diags)
	}
	return doc, nil
}

func ParseAndValidate(data []byte) (ValidatedPipeline, error) {
	doc, err := Parse(data)
	if err != nil {
		return ValidatedPipeline{}, err
	}
	return Validate(doc)
}

func preflightInput(data []byte) Diagnostics {
	var diags Diagnostics
	if len(data) > MaxPipelineBytes {
		diags = append(diags, newDiagnostic(CodeInputTooLarge, "", 0, 0, fmt.Sprintf("pipeline input exceeds %d bytes", MaxPipelineBytes)))
	}
	if len(bytes.TrimSpace(data)) == 0 {
		diags = append(diags, newDiagnostic(CodeYAMLSyntax, "", 0, 0, "input must contain one non-empty YAML document"))
	}
	if !utf8.Valid(data) {
		diags = append(diags, newDiagnostic(CodeInvalidUTF8, "", 0, 0, "input must be valid UTF-8"))
	}
	if i := bytes.IndexByte(data, 0); i >= 0 {
		line, col := byteLineColumn(data, i)
		diags = append(diags, newDiagnostic(CodeNULByte, "", line, col, "input must not contain NUL bytes"))
	}
	if directiveLine, directiveCol := firstDirective(data); directiveLine > 0 {
		diags = append(diags, newDiagnostic(CodeUnsupportedYAMLFeature, "", directiveLine, directiveCol, "YAML directives are not supported"))
	}
	return capDiagnostics(diags)
}

func loadSingleDocumentNode(data []byte) (*yaml.Node, error) {
	opts := []yaml.Option{
		yaml.WithAllDocuments(),
		yaml.WithKnownFields(),
		yaml.WithUniqueKeys(),
		yaml.WithPlugin(strictYAMLLimit{}),
	}
	var docs []yaml.Node
	if err := yaml.Load(data, &docs, opts...); err != nil {
		return nil, err
	}
	if len(docs) != 1 {
		return nil, errMultipleDocuments{count: len(docs)}
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

type errMultipleDocuments struct{ count int }

func (e errMultipleDocuments) Error() string {
	return fmt.Sprintf("expected exactly one YAML document, got %d", e.count)
}

func classifyYAMLError(err error) Diagnostics {
	msg := err.Error()
	switch {
	case errors.Is(err, errYAMLDepth) || strings.Contains(msg, "depth"):
		return Diagnostics{newDiagnostic(CodeOutOfRange, "", 0, 0, "YAML nesting depth exceeds limit")}
	case errors.Is(err, errYAMLAlias) || strings.Contains(strings.ToLower(msg), "alias"):
		return Diagnostics{newDiagnostic(CodeUnsupportedYAMLFeature, "", 0, 0, "YAML aliases are not supported")}
	case isMultipleDocumentsError(err):
		return Diagnostics{newDiagnostic(CodeMultipleDocuments, "", 0, 0, "input must contain exactly one non-empty YAML document")}
	case strings.Contains(strings.ToLower(msg), "duplicate"):
		return Diagnostics{newDiagnostic(CodeDuplicateKey, "", 0, 0, "duplicate mapping keys are not supported")}
	default:
		return Diagnostics{newDiagnostic(CodeYAMLSyntax, "", 0, 0, msg)}
	}
}

type nodeBudget struct{ count int }

func newNodeBudget() *nodeBudget { return &nodeBudget{} }

func inspectYAMLNode(n *yaml.Node, logicalPath string, depth int, budget *nodeBudget) Diagnostics {
	if n == nil {
		return nil
	}
	var diags Diagnostics
	budget.count++
	if budget.count > MaxYAMLNodes {
		return Diagnostics{newDiagnostic(CodeOutOfRange, logicalPath, n.Line, n.Column, "YAML node count exceeds limit")}
	}
	if depth > MaxYAMLDepth {
		return Diagnostics{newDiagnostic(CodeOutOfRange, logicalPath, n.Line, n.Column, "YAML nesting depth exceeds limit")}
	}
	if n.Kind == yaml.AliasNode {
		diags = append(diags, newDiagnostic(CodeUnsupportedYAMLFeature, logicalPath, n.Line, n.Column, "YAML aliases are not supported"))
	}
	if n.Anchor != "" {
		diags = append(diags, newDiagnostic(CodeUnsupportedYAMLFeature, logicalPath, n.Line, n.Column, "YAML anchors are not supported"))
	}
	if n.Style&yaml.TaggedStyle != 0 || isCustomTag(n.Tag) {
		diags = append(diags, newDiagnostic(CodeUnsupportedYAMLFeature, logicalPath, n.Line, n.Column, "explicit or custom YAML tags are not supported"))
	}
	if len(diags) >= MaxDiagnostics {
		return capDiagnostics(diags)
	}

	switch n.Kind {
	case yaml.MappingNode:
		seen := make(map[string]struct{}, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			value := n.Content[i+1]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				diags = append(diags, newDiagnostic(CodeUnsupportedYAMLFeature, logicalPath, key.Line, key.Column, "mapping keys must be scalar strings"))
				continue
			}
			if key.Value == "<<" {
				diags = append(diags, newDiagnostic(CodeUnsupportedYAMLFeature, childPath(logicalPath, key.Value), key.Line, key.Column, "YAML merge keys are not supported"))
			}
			if _, ok := seen[key.Value]; ok {
				diags = append(diags, newDiagnostic(CodeDuplicateKey, childPath(logicalPath, key.Value), key.Line, key.Column, "duplicate mapping key"))
			} else {
				seen[key.Value] = struct{}{}
			}
			diags = append(diags, inspectYAMLNode(value, childPath(logicalPath, key.Value), depth+1, budget)...)
			if len(diags) >= MaxDiagnostics {
				return capDiagnostics(diags)
			}
		}
	case yaml.SequenceNode:
		for i, child := range n.Content {
			diags = append(diags, inspectYAMLNode(child, indexedPath(logicalPath, i), depth+1, budget)...)
			if len(diags) >= MaxDiagnostics {
				return capDiagnostics(diags)
			}
		}
	case yaml.ScalarNode:
		limit := MaxGeneralStringBytes
		if strings.HasSuffix(logicalPath, ".run") || logicalPath == "spec.scenarios[0].run" || strings.Contains(logicalPath, ".run") {
			limit = MaxRunBytes
		}
		if len(n.Value) > limit {
			diags = append(diags, newDiagnostic(CodeOutOfRange, logicalPath, n.Line, n.Column, fmt.Sprintf("scalar exceeds %d bytes", limit)))
		}
	}
	return capDiagnostics(diags)
}

func isCustomTag(tag string) bool {
	switch tag {
	case "", "!!map", "!!seq", "!!str", "!!int", "!!null", "!!bool":
		return false
	default:
		return strings.HasPrefix(tag, "!")
	}
}

func decodeDocument(n *yaml.Node) (Document, Diagnostics) {
	var doc Document
	fields, diags := mappingFields(n, "", []string{"apiVersion", "kind", "metadata", "spec"})
	if len(diags) > 0 {
		return doc, diags
	}
	doc.APIVersion = stringField(fields["apiVersion"])
	doc.Kind = stringField(fields["kind"])
	doc.Metadata = decodeMetadata(fields["metadata"])
	doc.Spec = decodeSpec(fields["spec"])
	return doc, nil
}

func decodeMetadata(n *yaml.Node) Metadata {
	m := Metadata{}
	if n == nil {
		return m
	}
	m.Present, m.Null, m.Line, m.Column = true, isNullNode(n), n.Line, n.Column
	if m.Null || n.Kind != yaml.MappingNode {
		return m
	}
	fields, _ := mappingFields(n, "metadata", []string{"name"})
	m.Name = stringField(fields["name"])
	return m
}

func decodeSpec(n *yaml.Node) Spec {
	s := Spec{}
	if n == nil {
		return s
	}
	s.Present, s.Null, s.Line, s.Column = true, isNullNode(n), n.Line, n.Column
	if s.Null || n.Kind != yaml.MappingNode {
		return s
	}
	fields, _ := mappingFields(n, "spec", []string{"environment", "resources", "network", "scenarios", "collect", "compare", "policy"})
	s.Environment = decodeEnvironment(fields["environment"])
	s.Resources = decodeResources(fields["resources"])
	s.Network = decodeNetwork(fields["network"])
	s.ScenariosPresent, s.ScenariosNull = fields["scenarios"] != nil, isNullNode(fields["scenarios"])
	if fields["scenarios"] != nil {
		s.ScenariosLine, s.ScenariosColumn = fields["scenarios"].Line, fields["scenarios"].Column
		if fields["scenarios"].Kind == yaml.SequenceNode {
			for _, item := range fields["scenarios"].Content {
				s.Scenarios = append(s.Scenarios, decodeScenario(item))
			}
		}
	}
	s.Collect = decodeCollect(fields["collect"])
	s.Compare = decodeCompare(fields["compare"])
	s.Policy = decodePolicy(fields["policy"])
	return s
}

func decodeEnvironment(n *yaml.Node) Environment {
	e := Environment{}
	if n == nil {
		return e
	}
	e.Present, e.Null, e.Line, e.Column = true, isNullNode(n), n.Line, n.Column
	if e.Null || n.Kind != yaml.MappingNode {
		return e
	}
	fields, _ := mappingFields(n, "spec.environment", []string{"image", "workdir"})
	e.Image = stringField(fields["image"])
	e.Workdir = stringField(fields["workdir"])
	return e
}

func decodeResources(n *yaml.Node) Resources {
	r := Resources{}
	if n == nil {
		return r
	}
	r.Present, r.Null, r.Line, r.Column = true, isNullNode(n), n.Line, n.Column
	if r.Null || n.Kind != yaml.MappingNode {
		return r
	}
	fields, _ := mappingFields(n, "spec.resources", []string{"cpu", "memory", "disk", "processes", "timeout"})
	r.CPU = intField(fields["cpu"])
	r.Memory = stringField(fields["memory"])
	r.Disk = stringField(fields["disk"])
	r.Processes = intField(fields["processes"])
	r.Timeout = stringField(fields["timeout"])
	return r
}

func decodeNetwork(n *yaml.Node) Network {
	net := Network{}
	if n == nil {
		return net
	}
	net.Present, net.Null, net.Line, net.Column = true, isNullNode(n), n.Line, n.Column
	if net.Null || n.Kind != yaml.MappingNode {
		return net
	}
	fields, _ := mappingFields(n, "spec.network", []string{"mode", "allow"})
	net.Mode = stringField(fields["mode"])
	net.Allow = sequencePresence(fields["allow"])
	if fields["allow"] != nil && fields["allow"].Kind == yaml.SequenceNode {
		net.AllowLen = len(fields["allow"].Content)
	}
	return net
}

func decodeScenario(n *yaml.Node) Scenario {
	s := Scenario{Line: n.Line, Column: n.Column, Null: isNullNode(n)}
	if s.Null || n.Kind != yaml.MappingNode {
		return s
	}
	fields, _ := mappingFields(n, "spec.scenarios[]", []string{"id", "name", "shell", "run", "timeout"})
	s.ID = stringField(fields["id"])
	s.Name = stringField(fields["name"])
	s.Shell = stringField(fields["shell"])
	s.Run = stringField(fields["run"])
	s.Timeout = stringField(fields["timeout"])
	return s
}

func decodeCollect(n *yaml.Node) Collect {
	c := Collect{}
	if n == nil {
		return c
	}
	c.Present, c.Null, c.Line, c.Column = true, isNullNode(n), n.Line, n.Column
	if c.Null || n.Kind != yaml.MappingNode {
		return c
	}
	fields, _ := mappingFields(n, "spec.collect", []string{"filesystem", "artifacts", "logs"})
	c.Filesystem = decodeFilesystem(fields["filesystem"])
	c.ArtifactsPresent, c.ArtifactsNull = fields["artifacts"] != nil, isNullNode(fields["artifacts"])
	if fields["artifacts"] != nil {
		c.ArtifactsLine, c.ArtifactsColumn = fields["artifacts"].Line, fields["artifacts"].Column
		if fields["artifacts"].Kind == yaml.SequenceNode {
			for _, item := range fields["artifacts"].Content {
				c.Artifacts = append(c.Artifacts, decodeArtifact(item))
			}
		}
	}
	c.Logs = decodeLogs(fields["logs"])
	return c
}

func decodeFilesystem(n *yaml.Node) FilesystemCollect {
	f := FilesystemCollect{}
	if n == nil {
		return f
	}
	f.Present, f.Null, f.Line, f.Column = true, isNullNode(n), n.Line, n.Column
	if f.Null || n.Kind != yaml.MappingNode {
		return f
	}
	fields, _ := mappingFields(n, "spec.collect.filesystem", []string{"roots", "contents"})
	f.RootsPresent, f.RootsNull = fields["roots"] != nil, isNullNode(fields["roots"])
	if fields["roots"] != nil {
		f.RootsLine, f.RootsColumn = fields["roots"].Line, fields["roots"].Column
		if fields["roots"].Kind == yaml.SequenceNode {
			for _, item := range fields["roots"].Content {
				f.Roots = append(f.Roots, stringField(item))
			}
		}
	}
	f.Contents = stringField(fields["contents"])
	return f
}

func decodeArtifact(n *yaml.Node) ArtifactCollect {
	a := ArtifactCollect{Line: n.Line, Column: n.Column, Null: isNullNode(n)}
	if a.Null || n.Kind != yaml.MappingNode {
		return a
	}
	fields, _ := mappingFields(n, "spec.collect.artifacts[]", []string{"path", "maxBytes"})
	a.Path = stringField(fields["path"])
	a.MaxBytes = stringField(fields["maxBytes"])
	return a
}

func decodeLogs(n *yaml.Node) LogsCollect {
	l := LogsCollect{}
	if n == nil {
		return l
	}
	l.Present, l.Null, l.Line, l.Column = true, isNullNode(n), n.Line, n.Column
	if l.Null || n.Kind != yaml.MappingNode {
		return l
	}
	fields, _ := mappingFields(n, "spec.collect.logs", []string{"maxBytesPerStream"})
	l.MaxBytesPerStream = stringField(fields["maxBytesPerStream"])
	return l
}

func decodeCompare(n *yaml.Node) Compare {
	c := Compare{}
	if n == nil {
		return c
	}
	c.Present, c.Null, c.Line, c.Column = true, isNullNode(n), n.Line, n.Column
	if c.Null || n.Kind != yaml.MappingNode {
		return c
	}
	fields, _ := mappingFields(n, "spec.compare", []string{"ignore", "repetitions"})
	c.IgnorePresent, c.IgnoreNull = fields["ignore"] != nil, isNullNode(fields["ignore"])
	if fields["ignore"] != nil {
		c.IgnoreLine, c.IgnoreColumn = fields["ignore"].Line, fields["ignore"].Column
		if fields["ignore"].Kind == yaml.SequenceNode {
			for _, item := range fields["ignore"].Content {
				c.Ignore = append(c.Ignore, decodeIgnoreField(item))
			}
		}
	}
	c.Repetitions = intField(fields["repetitions"])
	return c
}

func decodeIgnoreField(n *yaml.Node) IgnoreField {
	i := IgnoreField{Line: n.Line, Column: n.Column, Null: isNullNode(n)}
	if i.Null || n.Kind != yaml.MappingNode {
		return i
	}
	fields, _ := mappingFields(n, "spec.compare.ignore[]", []string{"field"})
	i.Field = stringField(fields["field"])
	return i
}

func decodePolicy(n *yaml.Node) Policy {
	p := Policy{}
	if n == nil {
		return p
	}
	p.Present, p.Null, p.Line, p.Column = true, isNullNode(n), n.Line, n.Column
	if p.Null || n.Kind != yaml.MappingNode {
		return p
	}
	fields, _ := mappingFields(n, "spec.policy", []string{"profile"})
	p.Profile = stringField(fields["profile"])
	return p
}

func mappingFields(n *yaml.Node, logicalPath string, allowed []string) (map[string]*yaml.Node, Diagnostics) {
	out := make(map[string]*yaml.Node, len(allowed))
	if n == nil || n.Kind != yaml.MappingNode {
		return out, nil
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}
	var diags Diagnostics
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i]
		value := n.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			diags = append(diags, newDiagnostic(CodeUnsupportedYAMLFeature, logicalPath, key.Line, key.Column, "mapping keys must be scalar strings"))
			continue
		}
		if _, ok := allowedSet[key.Value]; !ok {
			diags = append(diags, newDiagnostic(CodeUnknownField, childPath(logicalPath, key.Value), key.Line, key.Column, "unknown field"))
			continue
		}
		out[key.Value] = value
	}
	return out, capDiagnostics(diags)
}

func stringField(n *yaml.Node) StringValue {
	v := StringValue{}
	if n == nil {
		return v
	}
	v.Present, v.Null, v.Line, v.Column = true, isNullNode(n), n.Line, n.Column
	if !v.Null && n.Kind == yaml.ScalarNode {
		v.Value = n.Value
	}
	return v
}

func intField(n *yaml.Node) IntValue {
	v := IntValue{}
	if n == nil {
		return v
	}
	v.Present, v.Null, v.Line, v.Column = true, isNullNode(n), n.Line, n.Column
	if !v.Null && n.Kind == yaml.ScalarNode {
		parsed, err := strconv.ParseInt(n.Value, 10, 64)
		if err == nil {
			v.Value = parsed
		}
	}
	return v
}

func sequencePresence(n *yaml.Node) SequencePresence {
	v := SequencePresence{}
	if n == nil {
		return v
	}
	v.Present, v.Null, v.Line, v.Column = true, isNullNode(n), n.Line, n.Column
	return v
}

func isNullNode(n *yaml.Node) bool {
	return n != nil && n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

func childPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func indexedPath(parent string, idx int) string {
	if parent == "" {
		return fmt.Sprintf("[%d]", idx)
	}
	return fmt.Sprintf("%s[%d]", parent, idx)
}

func byteLineColumn(data []byte, offset int) (int, int) {
	line, col := 1, 1
	for i := 0; i < offset && i < len(data); i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func firstDirective(data []byte) (int, int) {
	line := 1
	for len(data) > 0 {
		row := data
		if idx := bytes.IndexByte(data, '\n'); idx >= 0 {
			row = data[:idx]
			data = data[idx+1:]
		} else {
			data = nil
		}
		trim := strings.TrimLeft(string(row), " \t")
		col := len(string(row)) - len(trim) + 1
		if strings.HasPrefix(trim, "%YAML") || strings.HasPrefix(trim, "%TAG") {
			return line, col
		}
		line++
	}
	return 0, 0
}

func isMultipleDocumentsError(err error) bool {
	var value errMultipleDocuments
	return errors.As(err, &value)
}

func inspectKnownFields(n *yaml.Node, logicalPath string) Diagnostics {
	if n == nil {
		return nil
	}
	var diags Diagnostics
	if n.Kind == yaml.MappingNode {
		allowed, ok := allowedFieldsForPath(logicalPath)
		if ok {
			allowedSet := make(map[string]struct{}, len(allowed))
			for _, field := range allowed {
				allowedSet[field] = struct{}{}
			}
			for i := 0; i+1 < len(n.Content); i += 2 {
				key := n.Content[i]
				value := n.Content[i+1]
				if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
					continue
				}
				child := childPath(logicalPath, key.Value)
				if _, exists := allowedSet[key.Value]; !exists {
					diags = append(diags, newDiagnostic(CodeUnknownField, child, key.Line, key.Column, "unknown field"))
					continue
				}
				diags = append(diags, inspectKnownFields(value, child)...)
				if len(diags) >= MaxDiagnostics {
					return capDiagnostics(diags)
				}
			}
			return capDiagnostics(diags)
		}
	}
	if n.Kind == yaml.SequenceNode {
		itemPath, ok := sequenceItemPath(logicalPath)
		if ok {
			for _, child := range n.Content {
				diags = append(diags, inspectKnownFields(child, itemPath)...)
				if len(diags) >= MaxDiagnostics {
					return capDiagnostics(diags)
				}
			}
		}
	}
	return capDiagnostics(diags)
}

func allowedFieldsForPath(logicalPath string) ([]string, bool) {
	switch logicalPath {
	case "":
		return []string{"apiVersion", "kind", "metadata", "spec"}, true
	case "metadata":
		return []string{"name"}, true
	case "spec":
		return []string{"environment", "resources", "network", "scenarios", "collect", "compare", "policy"}, true
	case "spec.environment":
		return []string{"image", "workdir"}, true
	case "spec.resources":
		return []string{"cpu", "memory", "disk", "processes", "timeout"}, true
	case "spec.network":
		return []string{"mode", "allow"}, true
	case "spec.scenarios[]":
		return []string{"id", "name", "shell", "run", "timeout"}, true
	case "spec.collect":
		return []string{"filesystem", "artifacts", "logs"}, true
	case "spec.collect.filesystem":
		return []string{"roots", "contents"}, true
	case "spec.collect.artifacts[]":
		return []string{"path", "maxBytes"}, true
	case "spec.collect.logs":
		return []string{"maxBytesPerStream"}, true
	case "spec.compare":
		return []string{"ignore", "repetitions"}, true
	case "spec.compare.ignore[]":
		return []string{"field"}, true
	case "spec.policy":
		return []string{"profile"}, true
	default:
		return nil, false
	}
}

func sequenceItemPath(logicalPath string) (string, bool) {
	switch logicalPath {
	case "spec.scenarios":
		return "spec.scenarios[]", true
	case "spec.collect.artifacts":
		return "spec.collect.artifacts[]", true
	case "spec.compare.ignore":
		return "spec.compare.ignore[]", true
	default:
		return "", false
	}
}
