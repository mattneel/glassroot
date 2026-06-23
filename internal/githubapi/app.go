package githubapi

import "sort"

type appResponse struct {
	ID          int64             `json:"id"`
	ClientID    string            `json:"client_id"`
	Permissions map[string]string `json:"permissions"`
	Events      []string          `json:"events"`
	Slug        string            `json:"slug,omitempty"`
}

func (c *Client) validateApp(r appResponse) error {
	if r.ID != c.identity.AppID || r.ClientID != c.identity.ClientID {
		return errCode(CodeAppIdentityMismatch, "app", "app identity mismatch", nil)
	}
	if err := validateAppPermissions(r.Permissions); err != nil {
		return err
	}
	if len(r.Events) > 0 {
		if err := validateWebhookEvents(r.Events); err != nil {
			return err
		}
	}
	return nil
}

func validateAppPermissions(p map[string]string) error {
	required := map[string]string{"checks": "write", "contents": "read", "pull_requests": "read", "metadata": "read"}
	if len(p) == 0 {
		return errCode(CodeAppPermissionsMismatch, "app", "app permissions missing", nil)
	}
	for k, v := range p {
		exp, ok := required[k]
		if !ok {
			return errCode(CodeAppPermissionsMismatch, "app", "unexpected app permission", nil)
		}
		if v != exp {
			return errCode(CodeAppPermissionsMismatch, "app", "app permission mismatch", nil)
		}
	}
	for k, v := range required {
		if p[k] != v {
			return errCode(CodeAppPermissionsMismatch, "app", "required app permission missing", nil)
		}
	}
	return nil
}

func validateWebhookEvents(events []string) error {
	required := []string{"check_run", "check_suite", "installation", "installation_repositories", "ping", "pull_request"}
	got := append([]string(nil), events...)
	sort.Strings(got)
	for _, e := range got {
		if !contains(required, e) {
			return errCode(CodeAppPermissionsMismatch, "app", "unexpected webhook event", nil)
		}
	}
	for _, e := range required {
		if !contains(got, e) {
			return errCode(CodeAppPermissionsMismatch, "app", "required webhook event missing", nil)
		}
	}
	return nil
}
func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
