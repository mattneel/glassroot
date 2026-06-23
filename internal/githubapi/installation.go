package githubapi

type installationResponse struct {
	ID                  int64             `json:"id"`
	AppID               int64             `json:"app_id"`
	TargetID            int64             `json:"target_id"`
	RepositorySelection string            `json:"repository_selection"`
	Permissions         map[string]string `json:"permissions"`
	SuspendedAt         *string           `json:"suspended_at"`
}

func (c *Client) validateInstallation(req TokenRequest, r installationResponse) error {
	if r.ID != req.InstallationID || r.AppID != c.identity.AppID || r.TargetID <= 0 {
		return errCode(CodeInstallationMismatch, "installation", "installation mismatch", nil)
	}
	if r.SuspendedAt != nil {
		return errCode(CodeInstallationSuspended, "installation", "installation suspended", nil)
	}
	if r.RepositorySelection != "all" && r.RepositorySelection != "selected" {
		return errCode(CodeInstallationMismatch, "installation", "repository selection rejected", nil)
	}
	if err := validateKnownPermissionMap(r.Permissions); err != nil {
		return err
	}
	name := permissionForPurpose(req.Purpose)
	access := r.Permissions[name]
	if access != "read" && access != "write" {
		return errCode(CodeInstallationPermissionInsufficient, "installation", "installation permission insufficient", nil)
	}
	return nil
}

func permissionForPurpose(p TokenPurpose) string {
	if p == PurposePullRequestRead {
		return "pull_requests"
	}
	if p == PurposeSourceRead {
		return "contents"
	}
	return ""
}
func validateKnownPermissionMap(p map[string]string) error {
	for k, v := range p {
		if k != "checks" && k != "contents" && k != "pull_requests" && k != "metadata" {
			return errCode(CodeInstallationPermissionInsufficient, "installation", "unknown installation permission", nil)
		}
		if v != "read" && v != "write" && v != "none" {
			return errCode(CodeInstallationPermissionInsufficient, "installation", "unknown installation permission access", nil)
		}
	}
	return nil
}
