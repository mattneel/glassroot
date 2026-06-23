package model

// Digest is a serialized digest string. The model package records digest values
// but does not compute, parse, verify, or dereference them.
type Digest string

// RevisionKind identifies the trusted base or proposed head revision in data
// that compares two revisions.
type RevisionKind string

const (
	RevisionKindBase RevisionKind = "base"
	RevisionKindHead RevisionKind = "head"
)

// GitObjectFormat identifies the object identity algorithm used by a Git
// object store. It records Git object identity only; it is not a Glassroot
// attestation or evidence digest.
type GitObjectFormat string

const (
	GitObjectFormatSHA1   GitObjectFormat = "sha1"
	GitObjectFormatSHA256 GitObjectFormat = "sha256"
)

// Limitation records incomplete evidence, unsupported observations, truncation,
// or other uncertainty that must remain visible to policy and reporting layers.
type Limitation struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"`
}

// EnvEntry is an ordered key/value environment representation. Ordered slices
// avoid depending on map iteration when values later affect reproducibility.
type EnvEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// KeyValue is an ordered generic key/value pair for descriptive metadata.
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
