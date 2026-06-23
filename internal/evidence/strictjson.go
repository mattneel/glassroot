package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/model"
)

type strictDocument string

const (
	strictManifest strictDocument = "manifest"
	strictPlan     strictDocument = "plan"
	strictExec     strictDocument = "execution"
	strictResult   strictDocument = "attempt-result"
	strictEvent    strictDocument = "event"
	strictArtifact strictDocument = "artifact-index"
)

func decodeManifestStrict(data []byte, limits ReaderLimits) (Manifest, error) {
	var out Manifest
	if err := decodeStrictJSON(data, limits, strictManifest, &out); err != nil {
		return Manifest{}, err
	}
	return out, nil
}
func decodePlanStrict(data []byte, limits ReaderLimits) (model.RunPlan, error) {
	var out model.RunPlan
	if err := decodeStrictJSON(data, limits, strictPlan, &out); err != nil {
		return model.RunPlan{}, err
	}
	return out, nil
}
func decodeExecutionStrict(data []byte, limits ReaderLimits) (ExecutionDocument, error) {
	var out ExecutionDocument
	if err := decodeStrictJSON(data, limits, strictExec, &out); err != nil {
		return ExecutionDocument{}, err
	}
	return out, nil
}
func decodeAttemptResultStrict(data []byte, limits ReaderLimits) (AttemptResultDocument, error) {
	var out AttemptResultDocument
	if err := decodeStrictJSON(data, limits, strictResult, &out); err != nil {
		return AttemptResultDocument{}, err
	}
	return out, nil
}
func decodeEventStrict(data []byte, limits ReaderLimits) (model.ObservationEvent, error) {
	var out model.ObservationEvent
	if err := decodeStrictJSON(data, limits, strictEvent, &out); err != nil {
		return model.ObservationEvent{}, err
	}
	return out, nil
}
func decodeArtifactIndexStrict(data []byte, limits ReaderLimits) (ArtifactIndexDocument, error) {
	var out ArtifactIndexDocument
	if err := decodeStrictJSON(data, limits, strictArtifact, &out); err != nil {
		return ArtifactIndexDocument{}, err
	}
	return out, nil
}

func decodeStrictJSON(data []byte, limits ReaderLimits, doc strictDocument, dst any) error {
	if err := strictJSONPreflight(data, limits, doc); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return mapJSONDecodeError(err, doc)
	}
	if dec.More() {
		return errCode(CodeTrailingJSON, string(doc), "json", "trailing JSON value", nil)
	}
	if tok, err := dec.Token(); err != io.EOF || tok != nil {
		if err == nil {
			err = errors.New("trailing JSON")
		}
		return errCode(CodeTrailingJSON, string(doc), "json", "trailing JSON value", err)
	}
	encoded, err := json.Marshal(dst)
	if err != nil {
		return errCode(CodeSerializationFailed, string(doc), "json", "marshal decoded document", err)
	}
	if !bytes.Equal(encoded, data) {
		return errCode(CodeNoncanonicalJSON, string(doc), "json", "document is not the exact writer-normalized compact JSON", nil)
	}
	return nil
}

func strictJSONPreflight(data []byte, limits ReaderLimits, doc strictDocument) error {
	if len(data) == 0 {
		return errCode(CodeInvalidJSON, string(doc), "json", "empty JSON document", nil)
	}
	if !utf8.Valid(data) {
		return errCode(CodeInvalidUTF8, string(doc), "json", "JSON bytes are not valid UTF-8", nil)
	}
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		return errCode(CodeInvalidUTF8, string(doc), "json", "UTF-8 BOM is not accepted", nil)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return errCode(CodeInvalidUTF8, string(doc), "json", "JSON contains raw NUL", nil)
	}
	if hasSurrogateEscape(data) {
		return errCode(CodeInvalidUTF8, string(doc), "json", "surrogate escapes are not accepted in writer-normalized JSON", nil)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v := &jsonVisitor{limits: limits, doc: doc}
	tok, err := dec.Token()
	if err != nil {
		return errCode(CodeInvalidJSON, string(doc), "json", "invalid JSON", err)
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		return errCode(CodeInvalidJSON, string(doc), "json", "top-level JSON value must be object", nil)
	}
	if err := v.object(dec, 1); err != nil {
		return err
	}
	if tok, err := dec.Token(); err != io.EOF || tok != nil {
		if err == nil {
			err = errors.New("trailing JSON")
		}
		return errCode(CodeTrailingJSON, string(doc), "json", "trailing JSON value", err)
	}
	return nil
}

type jsonVisitor struct {
	limits ReaderLimits
	doc    strictDocument
	tokens int64
}

func (v *jsonVisitor) object(dec *json.Decoder, depth int) error {
	if depth > v.limits.MaxJSONDepth {
		return errCode(CodeJSONDepthLimit, string(v.doc), "json", "JSON depth exceeds limit", nil)
	}
	seen := map[string]struct{}{}
	members := 0
	for dec.More() {
		if err := v.bump(); err != nil {
			return err
		}
		tok, err := dec.Token()
		if err != nil {
			return errCode(CodeInvalidJSON, string(v.doc), "json", "invalid object member", err)
		}
		name, ok := tok.(string)
		if !ok {
			return errCode(CodeInvalidJSON, string(v.doc), "json", "object member name is not a string", nil)
		}
		if int64(len(name)) > v.limits.MaxJSONStringBytes {
			return errCode(CodeJSONTokenLimit, string(v.doc), "json", "JSON member name exceeds limit", nil)
		}
		if _, ok := seen[name]; ok {
			return errCode(CodeDuplicateJSONMember, string(v.doc), "json", "duplicate JSON member", nil)
		}
		seen[name] = struct{}{}
		members++
		if members > v.limits.MaxJSONMembersPerObject {
			return errCode(CodeJSONTokenLimit, string(v.doc), "json", "JSON object member count exceeds limit", nil)
		}
		if !allowedJSONField(name) {
			if fieldCaseVariant(name) {
				return errCode(CodeInvalidJSONFieldCase, string(v.doc), "json", "JSON member name uses unsupported case", nil)
			}
			return errCode(CodeUnknownJSONField, string(v.doc), "json", "unknown JSON member", nil)
		}
		if err := v.value(dec, depth+1); err != nil {
			return err
		}
	}
	end, err := dec.Token()
	if err != nil {
		return errCode(CodeInvalidJSON, string(v.doc), "json", "unterminated object", err)
	}
	if d, ok := end.(json.Delim); !ok || d != '}' {
		return errCode(CodeInvalidJSON, string(v.doc), "json", "object not terminated", nil)
	}
	return nil
}
func (v *jsonVisitor) array(dec *json.Decoder, depth int) error {
	if depth > v.limits.MaxJSONDepth {
		return errCode(CodeJSONDepthLimit, string(v.doc), "json", "JSON depth exceeds limit", nil)
	}
	count := int64(0)
	for dec.More() {
		count++
		if count > int64(v.limits.MaxJSONArrayElements) {
			return errCode(CodeJSONTokenLimit, string(v.doc), "json", "JSON array element count exceeds limit", nil)
		}
		if err := v.value(dec, depth+1); err != nil {
			return err
		}
	}
	end, err := dec.Token()
	if err != nil {
		return errCode(CodeInvalidJSON, string(v.doc), "json", "unterminated array", err)
	}
	if d, ok := end.(json.Delim); !ok || d != ']' {
		return errCode(CodeInvalidJSON, string(v.doc), "json", "array not terminated", nil)
	}
	return nil
}
func (v *jsonVisitor) value(dec *json.Decoder, depth int) error {
	if err := v.bump(); err != nil {
		return err
	}
	tok, err := dec.Token()
	if err != nil {
		return errCode(CodeInvalidJSON, string(v.doc), "json", "invalid JSON value", err)
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			return v.object(dec, depth)
		case '[':
			return v.array(dec, depth)
		default:
			return errCode(CodeInvalidJSON, string(v.doc), "json", "unexpected delimiter", nil)
		}
	case string:
		if int64(len(t)) > v.limits.MaxJSONStringBytes {
			return errCode(CodeJSONTokenLimit, string(v.doc), "json", "JSON string exceeds limit", nil)
		}
	case json.Number:
		if len(t.String()) > v.limits.MaxJSONNumberBytes {
			return errCode(CodeJSONTokenLimit, string(v.doc), "json", "JSON number exceeds limit", nil)
		}
	case bool:
	case nil:
		return errCode(CodeInvalidJSON, string(v.doc), "json", "null JSON values are not accepted in writer-normalized documents", nil)
	default:
		return errCode(CodeInvalidJSON, string(v.doc), "json", "unsupported JSON token", nil)
	}
	return nil
}
func (v *jsonVisitor) bump() error {
	v.tokens++
	if v.tokens > int64(v.limits.MaxJSONTokens) {
		return errCode(CodeJSONTokenLimit, string(v.doc), "json", "JSON token count exceeds limit", nil)
	}
	return nil
}

func mapJSONDecodeError(err error, doc strictDocument) error {
	msg := err.Error()
	if strings.Contains(msg, "unknown field") {
		return errCode(CodeUnknownJSONField, string(doc), "json", "unknown JSON member", err)
	}
	if strings.Contains(msg, "cannot unmarshal null") {
		return errCode(CodeInvalidJSON, string(doc), "json", "null is not valid for required field", err)
	}
	return errCode(CodeInvalidJSON, string(doc), "json", "typed JSON decode failed", err)
}

func hasSurrogateEscape(data []byte) bool {
	lower := bytes.ToLower(data)
	for i := 0; i+5 < len(lower); i++ {
		if lower[i] == '\\' && lower[i+1] == 'u' && lower[i+2] == 'd' {
			c := lower[i+3]
			if c >= '8' && c <= 'f' {
				return true
			}
		}
	}
	return false
}

func allowedJSONField(name string) bool { _, ok := allowedJSONFields[name]; return ok }
func fieldCaseVariant(name string) bool {
	lower := strings.ToLower(name)
	_, ok := allowedJSONFieldLower[lower]
	return ok && !allowedJSONField(name)
}

var allowedJSONFields = func() map[string]struct{} {
	fields := []string{
		"schemaVersion", "id", "runId", "createdAt", "pipelineName", "configuration", "source", "path", "digest", "sizeBytes", "objectId", "executionEnvironment", "image", "imageDigest", "workdir", "base", "head", "kind", "repository", "ref", "commitId", "objectFormat", "treeId", "treeDigest", "revisions", "commit", "materializedTreeDigest", "materializationManifestDigest", "sourceSummary", "directoryCount", "regularFileCount", "executableFileCount", "symlinkCount", "gitlinkCount", "lfsPointerCount", "totalMaterializedFileBytes", "skippedEntryCount", "sourceLimitations", "code", "summary", "scenarioIds", "scenarios", "name", "shell", "run", "repetitions", "command", "argv", "workingDirectory", "expectedArtifacts", "required", "maxSizeBytes", "resourceLimits", "cpu", "timeoutMillis", "memoryBytes", "diskBytes", "cpuMillis", "processCount", "networkPolicy", "mode", "allowed", "protocol", "host", "port", "environment", "collection", "filesystemRoots", "filesystemContents", "artifacts", "logMaxBytesPerStream", "comparison", "ignoreFields", "policy", "profile", "platform", "maxCpu", "maxMemoryBytes", "maxDiskBytes", "maxProcessCount", "maxGlobalTimeoutMillis", "maxScenarioTimeoutMillis", "maxScenarioCount", "maxRepetitions", "maxFilesystemRootCount", "maxArtifactCount", "maxArtifactBytes", "maxLogBytesPerStream", "maxPlanJsonBytes", "requiredNetworkMode", "limitations", "details", "runner", "version", "isolationTier", "freshKernel", "brokeredNetwork", "executesTargetCode", "syntheticEvidence", "enforcesNetworkDeny", "processEventCollection", "filesystemEventCollection", "syscallEventCollection", "artifactHashing", "snapshotSupport",
		"bundleFormatVersion", "planDigest", "executionComplete", "evidenceComplete", "bundleTransactionValid", "entries", "role", "mediaType", "revision", "scenarioId", "repetition", "captureState", "truncated", "omitted", "observedBytes", "observedBytesAtLeast", "attempts", "attemptId", "ordinal", "directory", "events", "stdout", "stderr", "result", "firstEventSequence", "lastEventSequence", "acceptedEventCount", "failure", "stage", "message", "category", "totalAcceptedEvents", "targetOutcome", "exitCode", "durationMillis", "firstAcceptedSequence", "lastAcceptedSequence", "disposition", "storedSizeBytes", "declaredSizeBytes", "objectPath", "logicalPath", "attempt", "sourceMode",
		"sequenceNumber", "observedAt", "process", "filesystem", "network", "artifact", "scenario", "observerWarning", "resourceLimit", "operation", "processId", "parentProcessId", "executablePath", "arguments", "value", "oldPath", "executable", "queryName", "destinationHost", "destinationPort", "resolvedAddresses", "artifactId", "sourceEventIds", "status", "startedAt", "completedAt", "exitedAt", "unsupported", "limitKind", "limitValue", "unit", "observedValue", "exceeded",
	}
	m := map[string]struct{}{}
	for _, f := range fields {
		m[f] = struct{}{}
	}
	return m
}()
var allowedJSONFieldLower = func() map[string]struct{} {
	m := map[string]struct{}{}
	for k := range allowedJSONFields {
		m[strings.ToLower(k)] = struct{}{}
	}
	return m
}()

func requireSchema(got model.SchemaVersion, want model.SchemaVersion, stage string) error {
	if got != want {
		return errCode(CodeUnsupportedSchema, stage, "schema", fmt.Sprintf("unsupported schema version for %s", stage), nil)
	}
	return nil
}
