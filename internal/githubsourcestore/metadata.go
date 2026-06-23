package githubsourcestore

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
)

type FixedRef struct {
	Name     string `json:"name"`
	ObjectID string `json:"objectId"`
}

type Metadata struct {
	SchemaVersion        string     `json:"schemaVersion"`
	ImportProfileVersion string     `json:"importProfileVersion"`
	SourceStoreID        string     `json:"sourceStoreId"`
	TargetID             string     `json:"targetId"`
	BaseRepositoryID     int64      `json:"baseRepositoryId"`
	HeadRepositoryID     int64      `json:"headRepositoryId"`
	PullRequestNumber    int64      `json:"pullRequestNumber"`
	ObjectFormat         string     `json:"objectFormat"`
	BaseCommitID         string     `json:"baseCommitId"`
	BaseTreeID           string     `json:"baseTreeId"`
	HeadCommitID         string     `json:"headCommitId"`
	HeadTreeID           string     `json:"headTreeId"`
	Shallow              bool       `json:"shallow"`
	FixedRefs            []FixedRef `json:"fixedRefs"`
	Limitations          []string   `json:"limitations"`
}

func NewMetadata(id Identity, objectFormat, baseTreeID, headTreeID string) (Metadata, error) {
	if err := validateIdentity(id); err != nil {
		return Metadata{}, err
	}
	if objectFormat != "sha1" && objectFormat != "sha256" {
		return Metadata{}, errCode(CodeMetadataInvalid, "metadata", "object format rejected", nil)
	}
	wantLen := 40
	if objectFormat == "sha256" {
		wantLen = 64
	}
	if len(id.BaseCommitID) != wantLen || len(id.HeadCommitID) != wantLen || !isLowerHex(baseTreeID, wantLen) || !isLowerHex(headTreeID, wantLen) {
		return Metadata{}, errCode(CodeMetadataInvalid, "metadata", "tree identity rejected", nil)
	}
	storeID, err := ComputeSourceStoreID(id)
	if err != nil {
		return Metadata{}, err
	}
	return Metadata{SchemaVersion: SchemaSourceStoreV1Alpha1, ImportProfileVersion: id.ImportProfileVersion, SourceStoreID: storeID, TargetID: id.TargetID, BaseRepositoryID: id.BaseRepositoryID, HeadRepositoryID: id.HeadRepositoryID, PullRequestNumber: id.PullRequestNumber, ObjectFormat: objectFormat, BaseCommitID: id.BaseCommitID, BaseTreeID: baseTreeID, HeadCommitID: id.HeadCommitID, HeadTreeID: headTreeID, Shallow: true, FixedRefs: []FixedRef{{Name: "refs/glassroot/base", ObjectID: id.BaseCommitID}, {Name: "refs/glassroot/head", ObjectID: id.HeadCommitID}}, Limitations: DefaultLimitations()}, nil
}

func DefaultLimitations() []string {
	return []string{"history outside selected shallow commits not imported", "tags not imported", "submodules not traversed", "LFS objects not fetched"}
}

func EncodeMetadata(m Metadata) ([]byte, string, error) {
	if err := ValidateMetadata(m); err != nil {
		return nil, "", err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, "", wrap(CodeMetadataInvalid, "metadata", "metadata JSON rejected", err)
	}
	h := sha256.New()
	_, _ = h.Write([]byte(domainMetadataJSONDigest))
	var lenbuf [8]byte
	binary.BigEndian.PutUint64(lenbuf[:], uint64(len(b)))
	_, _ = h.Write(lenbuf[:])
	_, _ = h.Write(b)
	return b, "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func ValidateMetadata(m Metadata) error {
	id := Identity{ImportProfileVersion: m.ImportProfileVersion, TargetID: m.TargetID, BaseRepositoryID: m.BaseRepositoryID, HeadRepositoryID: m.HeadRepositoryID, PullRequestNumber: m.PullRequestNumber, BaseCommitID: m.BaseCommitID, HeadCommitID: m.HeadCommitID}
	storeID, err := ComputeSourceStoreID(id)
	if err != nil {
		return err
	}
	if m.SchemaVersion != SchemaSourceStoreV1Alpha1 || m.SourceStoreID != storeID || !m.Shallow || (m.ObjectFormat != "sha1" && m.ObjectFormat != "sha256") || len(m.FixedRefs) != 2 || len(m.Limitations) == 0 {
		return errCode(CodeMetadataInvalid, "metadata", "metadata rejected", nil)
	}
	if m.FixedRefs[0].Name != "refs/glassroot/base" || m.FixedRefs[0].ObjectID != m.BaseCommitID || m.FixedRefs[1].Name != "refs/glassroot/head" || m.FixedRefs[1].ObjectID != m.HeadCommitID {
		return errCode(CodeMetadataInvalid, "metadata", "fixed refs rejected", nil)
	}
	wantLen := 40
	if m.ObjectFormat == "sha256" {
		wantLen = 64
	}
	if len(m.BaseCommitID) != wantLen || len(m.HeadCommitID) != wantLen || !isLowerHex(m.BaseTreeID, wantLen) || !isLowerHex(m.HeadTreeID, wantLen) {
		return errCode(CodeMetadataInvalid, "metadata", "object width rejected", nil)
	}
	return nil
}
