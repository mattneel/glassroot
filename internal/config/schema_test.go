package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestPipelineSchemaCompilesAndValidatesFixtureWithoutNetwork(t *testing.T) {
	schema := compilePipelineSchema(t)
	instance := yamlFixtureAsJSONValue(t, readFixture(t, "valid/pipeline.yaml"))
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("schema rejected valid fixture: %v", err)
	}
}

func TestPipelineSchemaRejectsRepresentativeInvalidInstances(t *testing.T) {
	schema := compilePipelineSchema(t)
	base := yamlFixtureAsJSONValue(t, readFixture(t, "valid/pipeline.yaml")).(map[string]any)

	cases := map[string]func(map[string]any){
		"unknown root property": func(m map[string]any) { m["extra"] = true },
		"wrong apiVersion":      func(m map[string]any) { m["apiVersion"] = "glassroot.dev/v2" },
		"bad image digest": func(m map[string]any) {
			spec(m)["environment"].(map[string]any)["image"] = "docker.io/library/golang:1.26"
		},
		"non-deny network": func(m map[string]any) { spec(m)["network"].(map[string]any)["mode"] = "host" },
		"invalid unit":     func(m map[string]any) { spec(m)["resources"].(map[string]any)["memory"] = "2GB" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			copy := cloneJSONMap(base)
			mutate(copy)
			if err := schema.Validate(copy); err == nil {
				t.Fatalf("schema accepted invalid instance")
			}
		})
	}
}

func TestSchemaDocumentsComplementaryGoValidation(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "api", "v1alpha1", "pipeline.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	for _, want := range []string{
		"https://glassroot.dev/schemas/v1alpha1/pipeline.schema.json",
		"https://json-schema.org/draft/2020-12/schema",
		"additionalProperties",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("schema missing %q", want)
		}
	}
}

func compilePipelineSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "api", "v1alpha1", "pipeline.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("https://glassroot.dev/schemas/v1alpha1/pipeline.schema.json", doc); err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	schema, err := compiler.Compile("https://glassroot.dev/schemas/v1alpha1/pipeline.schema.json")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return schema
}

func yamlFixtureAsJSONValue(t *testing.T, data []byte) any {
	t.Helper()
	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse fixture: %v", err)
	}
	encoded, err := MarshalJSONShape(doc)
	if err != nil {
		t.Fatalf("MarshalJSONShape: %v", err)
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatalf("unmarshal JSON shape: %v", err)
	}
	return value
}

func spec(m map[string]any) map[string]any { return m["spec"].(map[string]any) }

func cloneJSONMap(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}
