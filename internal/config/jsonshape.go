package config

import "encoding/json"

type jsonPipeline struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   jsonMetadata `json:"metadata"`
	Spec       jsonSpec     `json:"spec"`
}

type jsonMetadata struct {
	Name string `json:"name"`
}

type jsonSpec struct {
	Environment jsonEnvironment `json:"environment"`
	Resources   jsonResources   `json:"resources"`
	Network     jsonNetwork     `json:"network"`
	Scenarios   []jsonScenario  `json:"scenarios"`
	Collect     jsonCollect     `json:"collect"`
	Compare     jsonCompare     `json:"compare"`
	Policy      jsonPolicy      `json:"policy"`
}

type jsonEnvironment struct {
	Image   string `json:"image"`
	Workdir string `json:"workdir"`
}

type jsonResources struct {
	CPU       int64  `json:"cpu"`
	Memory    string `json:"memory"`
	Disk      string `json:"disk"`
	Processes int64  `json:"processes"`
	Timeout   string `json:"timeout"`
}

type jsonNetwork struct {
	Mode  string   `json:"mode"`
	Allow []string `json:"allow"`
}

type jsonScenario struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Shell   string `json:"shell"`
	Run     string `json:"run"`
	Timeout string `json:"timeout"`
}

type jsonCollect struct {
	Filesystem jsonFilesystem `json:"filesystem"`
	Artifacts  []jsonArtifact `json:"artifacts"`
	Logs       jsonLogs       `json:"logs"`
}

type jsonFilesystem struct {
	Roots    []string `json:"roots"`
	Contents string   `json:"contents"`
}

type jsonArtifact struct {
	Path     string `json:"path"`
	MaxBytes string `json:"maxBytes"`
}

type jsonLogs struct {
	MaxBytesPerStream string `json:"maxBytesPerStream"`
}

type jsonCompare struct {
	Ignore      []jsonIgnore `json:"ignore"`
	Repetitions int64        `json:"repetitions"`
}

type jsonIgnore struct {
	Field string `json:"field"`
}

type jsonPolicy struct {
	Profile string `json:"profile"`
}

func MarshalJSONShape(doc Document) ([]byte, error) {
	shape := jsonPipeline{
		APIVersion: doc.APIVersion.Value,
		Kind:       doc.Kind.Value,
		Metadata:   jsonMetadata{Name: doc.Metadata.Name.Value},
		Spec: jsonSpec{
			Environment: jsonEnvironment{Image: doc.Spec.Environment.Image.Value, Workdir: doc.Spec.Environment.Workdir.Value},
			Resources: jsonResources{
				CPU:       doc.Spec.Resources.CPU.Value,
				Memory:    doc.Spec.Resources.Memory.Value,
				Disk:      doc.Spec.Resources.Disk.Value,
				Processes: doc.Spec.Resources.Processes.Value,
				Timeout:   doc.Spec.Resources.Timeout.Value,
			},
			Network: jsonNetwork{Mode: doc.Spec.Network.Mode.Value, Allow: []string{}},
			Collect: jsonCollect{
				Filesystem: jsonFilesystem{Contents: doc.Spec.Collect.Filesystem.Contents.Value},
				Artifacts:  []jsonArtifact{},
				Logs:       jsonLogs{MaxBytesPerStream: doc.Spec.Collect.Logs.MaxBytesPerStream.Value},
			},
			Compare: jsonCompare{Ignore: []jsonIgnore{}, Repetitions: doc.Spec.Compare.Repetitions.Value},
			Policy:  jsonPolicy{Profile: doc.Spec.Policy.Profile.Value},
		},
	}
	for _, scenario := range doc.Spec.Scenarios {
		shape.Spec.Scenarios = append(shape.Spec.Scenarios, jsonScenario{
			ID: scenario.ID.Value, Name: scenario.Name.Value, Shell: scenario.Shell.Value, Run: scenario.Run.Value, Timeout: scenario.Timeout.Value,
		})
	}
	for _, root := range doc.Spec.Collect.Filesystem.Roots {
		shape.Spec.Collect.Filesystem.Roots = append(shape.Spec.Collect.Filesystem.Roots, root.Value)
	}
	for _, artifact := range doc.Spec.Collect.Artifacts {
		shape.Spec.Collect.Artifacts = append(shape.Spec.Collect.Artifacts, jsonArtifact{Path: artifact.Path.Value, MaxBytes: artifact.MaxBytes.Value})
	}
	for _, ignore := range doc.Spec.Compare.Ignore {
		shape.Spec.Compare.Ignore = append(shape.Spec.Compare.Ignore, jsonIgnore{Field: ignore.Field.Value})
	}
	return json.Marshal(shape)
}
