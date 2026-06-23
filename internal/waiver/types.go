package waiver

import (
	"time"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/model"
)

const (
	WaiverPath           = ".glassroot/waivers.yaml"
	APIVersionV1Alpha1   = "glassroot.dev/v1alpha1"
	KindWaiverSet        = "WaiverSet"
	semanticDigestDomain = "glassroot.dev/waiver-set-semantic/v1\x00"
)

type WaiverSet struct {
	APIVersion     string
	Kind           string
	MetadataName   string
	Waivers        []Waiver
	RawDigest      model.Digest
	SemanticDigest model.Digest
	RawSizeBytes   int64
}

type Waiver struct {
	ID        string
	Target    Target
	Owner     string
	Reason    string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type Target struct {
	FindingID string
	RuleID    string
}

type TrustedLoadRequest struct {
	Base model.CommitRef
	Head model.CommitRef
}

type TrustedLoadResult struct {
	BaseRevision model.CommitRef
	HeadRevision model.CommitRef
	Base         BaseAssessment
	Head         HeadAssessment
}

type BaseState string

const (
	BaseStateAbsent               BaseState = "absent"
	BaseStateValid                BaseState = "valid"
	BaseStateInvalid              BaseState = "invalid"
	BaseStateUnsupportedEntryKind BaseState = "unsupported-entry-kind"
)

type HeadState string

const (
	HeadStateAbsentOnBoth                         HeadState = "absent-on-both"
	HeadStateUnchanged                            HeadState = "unchanged"
	HeadStateAdded                                HeadState = "added"
	HeadStateRemoved                              HeadState = "removed"
	HeadStateContentChangedSemanticallyEquivalent HeadState = "content-changed-but-semantically-equivalent"
	HeadStateModifiedValid                        HeadState = "modified-valid"
	HeadStateModifiedInvalid                      HeadState = "modified-invalid"
	HeadStateUnsupportedEntryKind                 HeadState = "unsupported-entry-kind"
)

type FileMetadata struct {
	Revision   model.CommitRef
	Path       string
	Kind       config.EntryKind
	Digest     model.Digest
	SizeBytes  int64
	Executable bool
	ObjectID   string
}

type BaseAssessment struct {
	State          BaseState
	File           FileMetadata
	Waivers        []Waiver
	SemanticDigest model.Digest
	ErrorCode      ErrorCode
}

type HeadAssessment struct {
	State          HeadState
	File           FileMetadata
	Waivers        []Waiver
	SemanticDigest model.Digest
	Changes        []Change
	ErrorCode      ErrorCode
}

type ChangeKind string

const (
	ChangeWaiverAdded           ChangeKind = "waiver-added"
	ChangeWaiverRemoved         ChangeKind = "waiver-removed"
	ChangeWaiverTargetChanged   ChangeKind = "waiver-target-changed"
	ChangeWaiverOwnerChanged    ChangeKind = "waiver-owner-changed"
	ChangeWaiverReasonChanged   ChangeKind = "waiver-reason-changed"
	ChangeWaiverIssuedAtChanged ChangeKind = "waiver-issued-at-changed"
	ChangeWaiverExpiryChanged   ChangeKind = "waiver-expiry-changed"
)

type Change struct {
	WaiverID string
	Kind     ChangeKind
}
