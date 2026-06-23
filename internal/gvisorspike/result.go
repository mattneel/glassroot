package gvisorspike

type Result struct {
	SchemaVersion       string         `json:"schemaVersion"`
	RunscRelease        string         `json:"runscRelease"`
	RunscCommit         string         `json:"runscCommit"`
	RunscSHA512         string         `json:"runscSha512"`
	Architecture        string         `json:"architecture"`
	KernelVersion       string         `json:"kernelVersion"`
	Platform            string         `json:"platform"`
	RuntimeName         string         `json:"runtimeName"`
	FixtureImageDigest  string         `json:"fixtureImageDigest"`
	TracePoints         []string       `json:"tracePoints"`
	ConnectionCount     int            `json:"connectionCount"`
	MessageCount        int            `json:"messageCount"`
	DroppedMessageCount int            `json:"droppedMessageCount"`
	ProcessSummary      ProcessSummary `json:"processSummary"`
	ObservationComplete bool           `json:"observationComplete"`
	Limitations         []string       `json:"limitations"`
	CleanupComplete     bool           `json:"cleanupComplete"`
}
