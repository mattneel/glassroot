package githubapi

import "time"

type tokenRequestJSON struct {
	RepositoryIDs []int64           `json:"repository_ids"`
	Permissions   map[string]string `json:"permissions"`
}

func tokenRequestBody(req TokenRequest) tokenRequestJSON {
	return tokenRequestJSON{RepositoryIDs: []int64{req.RepositoryID}, Permissions: map[string]string{permissionForPurpose(req.Purpose): "read"}}
}

type tokenResponse struct {
	Token        string            `json:"token"`
	ExpiresAt    string            `json:"expires_at"`
	Permissions  map[string]string `json:"permissions"`
	Repositories []struct {
		ID int64 `json:"id"`
	} `json:"repositories"`
}

func validateTokenResponse(r tokenResponse, req TokenRequest, requestedAt time.Time, limits Limits) (*TokenLease, error) {
	lease, err := decodeTokenResponse(r, req, requestedAt, limits)
	if err != nil {
		zero([]byte(r.Token))
		return nil, err
	}
	return lease, nil
}
func DecodeTokenResponseForTest(body []byte, req TokenRequest, requestedAt time.Time, limits Limits) (*TokenLease, error) {
	var r tokenResponse
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := decodeStrict(body, limits, &r); err != nil {
		return nil, err
	}
	return validateTokenResponse(r, req, requestedAt, limits)
}

func decodeTokenResponse(r tokenResponse, req TokenRequest, requestedAt time.Time, limits Limits) (*TokenLease, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateTokenRequest(req); err != nil {
		return nil, err
	}
	if r.Token == "" || len(r.Token) > limits.MaxInstallationTokenBytes || hasControl(r.Token) {
		return nil, errCode(CodeTokenResponseInvalid, "token", "token rejected", nil)
	}
	exp, err := time.Parse(time.RFC3339, r.ExpiresAt)
	if err != nil || exp.Location() != time.UTC {
		return nil, errCode(CodeTokenExpiryInvalid, "token", "token expiry rejected", nil)
	}
	requestedAt = requestedAt.UTC().Round(0)
	if !exp.After(requestedAt) || exp.After(requestedAt.Add(65*time.Minute)) || exp.Before(requestedAt.Add(5*time.Minute)) {
		return nil, errCode(CodeTokenExpiryInvalid, "token", "token expiry rejected", nil)
	}
	if len(r.Repositories) != 1 || r.Repositories[0].ID != req.RepositoryID {
		return nil, errCode(CodeTokenScopeMismatch, "token", "token repository scope rejected", nil)
	}
	if err := validateTokenPermissions(r.Permissions, req.Purpose); err != nil {
		return nil, err
	}
	meta := TokenMetadata{Purpose: req.Purpose, InstallationID: req.InstallationID, RepositoryID: req.RepositoryID, ExpiresAt: exp, GrantedPermissions: permissionsSlice(r.Permissions)}
	return newTokenLease(meta, []byte(r.Token)), nil
}
func validateTokenPermissions(p map[string]string, purpose TokenPurpose) error {
	if len(p) == 0 {
		return errCode(CodeTokenScopeMismatch, "token", "token permissions missing", nil)
	}
	expected := permissionForPurpose(purpose)
	for k, v := range p {
		if k == "metadata" && v == "read" {
			continue
		}
		if k == expected && v == "read" {
			continue
		}
		return errCode(CodeTokenScopeMismatch, "token", "token permissions rejected", nil)
	}
	if p[expected] != "read" {
		return errCode(CodeTokenScopeMismatch, "token", "token permission missing", nil)
	}
	return nil
}
func permissionsSlice(p map[string]string) []Permission {
	out := make([]Permission, 0, len(p))
	if v, ok := p[permissionForPurpose(PurposePullRequestRead)]; ok {
		out = append(out, Permission{Name: "pull_requests", Access: v})
	}
	if v, ok := p[permissionForPurpose(PurposeSourceRead)]; ok {
		out = append(out, Permission{Name: "contents", Access: v})
	}
	if v, ok := p["metadata"]; ok {
		out = append(out, Permission{Name: "metadata", Access: v})
	}
	return out
}
func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
