package githubapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mattneel/glassroot/internal/githubauth"
)

type Clock interface{ Now() time.Time }
type JWTSigner interface {
	SignJWT(githubauth.AppIdentity, time.Time, githubauth.Limits) (*githubauth.AppJWT, error)
}

type Config struct {
	Identity   githubauth.AppIdentity
	Signer     JWTSigner
	Clock      Clock
	Transport  http.RoundTripper
	Limits     Limits
	AuthLimits githubauth.Limits
}
type Client struct {
	identity   githubauth.AppIdentity
	signer     JWTSigner
	clock      Clock
	http       *http.Client
	limits     Limits
	authLimits githubauth.Limits
}

func NewClient(cfg Config) (*Client, error) {
	limits := cfg.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	authLimits := cfg.AuthLimits
	if authLimits == (githubauth.Limits{}) {
		authLimits = githubauth.DefaultLimits()
	}
	if err := githubauth.ValidateAppIdentity(cfg.Identity, authLimits); err != nil {
		return nil, wrap(CodeAppIdentityMismatch, "identity", "app identity rejected", err)
	}
	if cfg.Signer == nil {
		return nil, errCode(CodeAPIUnavailable, "client", "jwt signer required", nil)
	}
	if cfg.Clock == nil {
		return nil, errCode(CodeAPIUnavailable, "client", "clock required", nil)
	}
	rt := cfg.Transport
	if rt == nil {
		rt = defaultTransport()
	}
	hc := &http.Client{Transport: rt, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return errCode(CodeAPIRedirectRejected, "request", "redirect rejected", nil)
	}}
	return &Client{identity: cfg.Identity, signer: cfg.Signer, clock: cfg.Clock, http: hc, limits: limits, authLimits: authLimits}, nil
}

func defaultTransport() http.RoundTripper {
	d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Transport{Proxy: nil, DialContext: d.DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, ExpectContinueTimeout: 1 * time.Second, IdleConnTimeout: 30 * time.Second, MaxIdleConns: 16, MaxIdleConnsPerHost: 4, DisableCompression: true}
}
func (c *Client) CloseIdleConnections() {
	if c != nil && c.http != nil {
		c.http.CloseIdleConnections()
	}
}

func (c *Client) VerifyApp(ctx context.Context) error {
	var out appResponse
	if err := c.getJSON(ctx, "/app", &out); err != nil {
		return err
	}
	return c.validateApp(out)
}
func (c *Client) IssueInstallationToken(ctx context.Context, req TokenRequest) (*TokenLease, error) {
	if err := validateTokenRequest(req); err != nil {
		return nil, err
	}
	var inst installationResponse
	if err := c.getJSON(ctx, "/app/installations/"+strconv.FormatInt(req.InstallationID, 10), &inst); err != nil {
		return nil, err
	}
	if err := c.validateInstallation(req, inst); err != nil {
		return nil, err
	}
	body, err := compactJSON(tokenRequestBody(req))
	if err != nil {
		return nil, err
	}
	var token tokenResponse
	if err := c.doJSON(ctx, http.MethodPost, "/app/installations/"+strconv.FormatInt(req.InstallationID, 10)+"/access_tokens", body, &token); err != nil {
		return nil, err
	}
	return validateTokenResponse(token, req, c.requestTime(), c.limits)
}

func validateTokenRequest(req TokenRequest) error {
	if req.InstallationID <= 0 {
		return errCode(CodeTokenRequestRejected, "request", "installation id rejected", nil)
	}
	if req.RepositoryID <= 0 {
		return errCode(CodeTokenRequestRejected, "request", "repository id rejected", nil)
	}
	if req.Purpose != PurposePullRequestRead && req.Purpose != PurposeSourceRead {
		return errCode(CodeTokenRequestRejected, "request", "token purpose rejected", nil)
	}
	return nil
}
func (c *Client) requestTime() time.Time { return c.clock.Now().UTC().Round(0) }

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}
func (c *Client) doJSON(ctx context.Context, method, path string, body []byte, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.limits.RequestTimeout)
	defer cancel()
	u := url.URL{Scheme: "https", Host: "api.github.com", Path: path}
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), r)
	if err != nil {
		return wrap(CodeAPIUnavailable, "request", "request create failed", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", GitHubAPIVersion)
	req.Header.Set("User-Agent", UserAgent)
	jwt, err := c.signer.SignJWT(c.identity, c.requestTime(), c.authLimits)
	if err != nil {
		return wrap(CodeAPIUnavailable, "jwt", "jwt sign failed", err)
	}
	defer jwt.Close()
	var auth string
	if err := jwt.Use(func(b []byte) error { auth = "Bearer " + string(b); return nil }); err != nil {
		return wrap(CodeAPIUnavailable, "jwt", "jwt use failed", err)
	}
	req.Header.Set("Authorization", auth)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return wrap(CodeAPITimeout, "request", "api request timed out", ctx.Err())
		}
		if strings.Contains(err.Error(), "redirect rejected") {
			return wrap(CodeAPIRedirectRejected, "request", "redirect rejected", err)
		}
		return wrap(CodeAPIUnavailable, "request", "api request failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return errCode(CodeAPIRedirectRejected, "response", "redirect rejected", nil)
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
		return errCode(CodeResponseInvalid, "response", "content encoding rejected", nil)
	}
	switch resp.StatusCode {
	case 200, 201:
	default:
		return statusError(resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.limits.MaxAPIResponseBytes)+1))
	if err != nil {
		return wrap(CodeResponseInvalid, "response", "response read failed", err)
	}
	if len(data) > c.limits.MaxAPIResponseBytes {
		return errCode(CodeResponseTooLarge, "response", "response body exceeds bound", nil)
	}
	if err := decodeStrict(data, c.limits, out); err != nil {
		return err
	}
	return nil
}
func statusError(status int) error {
	switch status {
	case 401:
		return errCode(CodeAPIUnavailable, "response", "api unauthorized", nil)
	case 403:
		return errCode(CodeAPIUnavailable, "response", "api forbidden", nil)
	case 404:
		return errCode(CodeInstallationNotFound, "response", "api resource not found", nil)
	case 422:
		return errCode(CodeTokenRequestRejected, "response", "api rejected request", nil)
	case 429:
		return errCode(CodeAPIRateLimited, "response", "api rate limited", nil)
	}
	if status == 410 {
		return errCode(CodeAPIVersionUnsupported, "response", "api version unsupported", nil)
	}
	if status >= 500 {
		return errCode(CodeAPIUnavailable, "response", "api unavailable", nil)
	}
	return errCode(CodeResponseInvalid, "response", "api status rejected", nil)
}
