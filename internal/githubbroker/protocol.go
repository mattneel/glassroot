package githubbroker

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubapp"
)

const (
	SchemaTokenRequestV1Alpha1  = "glassroot.dev/github-token-request/v1alpha1"
	SchemaTokenResponseV1Alpha1 = "glassroot.dev/github-token-response/v1alpha1"
)

type TokenPurpose = githubapi.TokenPurpose

const (
	PurposePullRequestRead TokenPurpose = githubapi.PurposePullRequestRead
	PurposeSourceRead      TokenPurpose = githubapi.PurposeSourceRead
)

type TokenRequest struct {
	SchemaVersion  string       `json:"schemaVersion"`
	Purpose        TokenPurpose `json:"purpose"`
	InstallationID int64        `json:"installationId"`
	RepositoryID   int64        `json:"repositoryId"`
}
type TokenResponse struct {
	SchemaVersion      string                 `json:"schemaVersion"`
	Purpose            TokenPurpose           `json:"purpose,omitempty"`
	InstallationID     int64                  `json:"installationId,omitempty"`
	RepositoryID       int64                  `json:"repositoryId,omitempty"`
	ExpiresAt          *time.Time             `json:"expiresAt,omitempty"`
	GrantedPermissions []githubapi.Permission `json:"grantedPermissions,omitempty"`
	Token              string                 `json:"token,omitempty"`
	ErrorCode          ErrorCode              `json:"errorCode,omitempty"`
	Message            string                 `json:"message,omitempty"`
}

func ValidateTokenRequest(req TokenRequest, limits Limits) error {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return err
	}
	if req.SchemaVersion != SchemaTokenRequestV1Alpha1 {
		return errCode(CodeUnsupportedSchemaVersion, "request", "schema version rejected", nil)
	}
	if req.Purpose != PurposePullRequestRead && req.Purpose != PurposeSourceRead {
		return errCode(CodeInvalidTokenPurpose, "request", "token purpose rejected", nil)
	}
	if req.InstallationID <= 0 {
		return errCode(CodeInvalidInstallationID, "request", "installation id rejected", nil)
	}
	if req.RepositoryID <= 0 {
		return errCode(CodeInvalidRepositoryID, "request", "repository id rejected", nil)
	}
	return nil
}
func toAPIRequest(req TokenRequest) githubapi.TokenRequest {
	return githubapi.TokenRequest{Purpose: githubapi.TokenPurpose(req.Purpose), InstallationID: req.InstallationID, RepositoryID: req.RepositoryID}
}
func fromAPIMetadata(meta githubapi.TokenMetadata) TokenMetadata {
	return TokenMetadata{Purpose: TokenPurpose(meta.Purpose), InstallationID: meta.InstallationID, RepositoryID: meta.RepositoryID, ExpiresAt: meta.ExpiresAt, GrantedPermissions: append([]githubapi.Permission(nil), meta.GrantedPermissions...)}
}

func decodeRequestPayload(payload []byte, limits Limits) (TokenRequest, error) {
	if len(payload) == 0 {
		return TokenRequest{}, errCode(CodeRequestFrameInvalid, "request", "empty request", nil)
	}
	gl := githubapp.DefaultLimits()
	gl.MaxWebhookBodyBytes = limits.MaxRequestFrameBytes
	if err := githubapp.PreflightGitHubWebhookJSON(payload, gl); err != nil {
		return TokenRequest{}, wrap(CodeRequestFrameInvalid, "request", "request JSON rejected", err)
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var req TokenRequest
	if err := dec.Decode(&req); err != nil {
		return TokenRequest{}, wrap(CodeRequestFrameInvalid, "request", "request decode failed", err)
	}
	if tok, err := dec.Token(); err != io.EOF {
		_ = tok
		return TokenRequest{}, errCode(CodeRequestFrameInvalid, "request", "trailing request rejected", err)
	}
	if err := ValidateTokenRequest(req, limits); err != nil {
		return TokenRequest{}, err
	}
	return req, nil
}
func encodeFrame(payload []byte, max int) ([]byte, error) {
	if len(payload) == 0 || len(payload) > max {
		return nil, errCode(CodeResponseWriteFailed, "frame", "frame size rejected", nil)
	}
	out := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(out[:4], uint32(len(payload)))
	copy(out[4:], payload)
	return out, nil
}
func readFrame(r io.Reader, max int) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, wrap(CodeRequestFrameInvalid, "frame", "frame header rejected", err)
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 {
		return nil, errCode(CodeRequestFrameInvalid, "frame", "empty frame rejected", nil)
	}
	if n > uint32(max) {
		return nil, errCode(CodeRequestTooLarge, "frame", "request frame too large", nil)
	}
	payload := make([]byte, int(n))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, wrap(CodeRequestFrameInvalid, "frame", "frame payload rejected", err)
	}
	return payload, nil
}
func DecodeRequestFrameForTest(frame []byte, limits Limits) (TokenRequest, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	p, err := readFrame(bytes.NewReader(frame), limits.MaxRequestFrameBytes)
	if err != nil {
		return TokenRequest{}, err
	}
	if 4+len(p) != len(frame) {
		return TokenRequest{}, errCode(CodeRequestFrameInvalid, "frame", "trailing frame bytes rejected", nil)
	}
	return decodeRequestPayload(p, limits)
}
func encodeJSONFrame(v any, max int) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, wrap(CodeResponseWriteFailed, "response", "response JSON failed", err)
	}
	return encodeFrame(b, max)
}
func decodeResponsePayload(payload []byte, limits Limits) (TokenResponse, error) {
	gl := githubapp.DefaultLimits()
	gl.MaxWebhookBodyBytes = limits.MaxResponseFrameBytes
	if err := githubapp.PreflightGitHubWebhookJSON(payload, gl); err != nil {
		return TokenResponse{}, wrap(CodeClientResponseInvalid, "response", "response JSON rejected", err)
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var resp TokenResponse
	if err := dec.Decode(&resp); err != nil {
		return TokenResponse{}, wrap(CodeClientResponseInvalid, "response", "response decode failed", err)
	}
	if resp.SchemaVersion != SchemaTokenResponseV1Alpha1 {
		return TokenResponse{}, errCode(CodeClientResponseInvalid, "response", "response schema rejected", nil)
	}
	return resp, nil
}
