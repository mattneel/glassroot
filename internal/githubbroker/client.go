package githubbroker

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"time"
)

type Client struct {
	socketPath string
	limits     Limits
}

func Dial(ctx context.Context, socketPath string, limits Limits) (*Client, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if err := validateSocketPath(socketPath, limits.MaxPathBytes); err != nil {
		return nil, errCode(CodeInvalidListenerPath, "client", "socket path rejected", nil)
	}
	st, err := os.Lstat(socketPath)
	if err != nil {
		return nil, wrap(CodeClientConnectionFailed, "client", "socket stat failed", err)
	}
	if st.Mode()&os.ModeSocket == 0 {
		return nil, errCode(CodeClientConnectionFailed, "client", "socket type rejected", nil)
	}
	select {
	case <-ctx.Done():
		return nil, wrap(CodeContextCancelled, "client", "context cancelled", ctx.Err())
	default:
	}
	return &Client{socketPath: socketPath, limits: limits}, nil
}
func (c *Client) RequestToken(ctx context.Context, req TokenRequest) (*TokenLease, error) {
	if c == nil {
		return nil, errCode(CodeClientConnectionFailed, "client", "client unavailable", nil)
	}
	if err := ValidateTokenRequest(req, c.limits); err != nil {
		return nil, err
	}
	body, err := encodeRequest(req)
	if err != nil {
		return nil, err
	}
	frame, err := encodeFrame(body, c.limits.MaxRequestFrameBytes)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: c.limits.PerConnectionTimeout}
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, wrap(CodeClientConnectionFailed, "client", "connect failed", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(c.limits.PerConnectionTimeout))
	if _, err := conn.Write(frame); err != nil {
		return nil, wrap(CodeClientConnectionFailed, "client", "request write failed", err)
	}
	payload, err := readFrame(conn, c.limits.MaxResponseFrameBytes)
	if err != nil {
		return nil, wrap(CodeClientResponseInvalid, "client", "response frame rejected", err)
	}
	resp, err := decodeResponsePayload(payload, c.limits)
	if err != nil {
		return nil, err
	}
	if resp.ErrorCode != "" {
		return nil, errCode(resp.ErrorCode, "client", "broker rejected request", nil)
	}
	if resp.ExpiresAt == nil {
		return nil, errCode(CodeClientResponseInvalid, "client", "token expiry missing", nil)
	}
	if resp.Token == "" || len(resp.Token) > c.limits.MaxTokenBytes || hasControl(resp.Token) {
		return nil, errCode(CodeClientResponseInvalid, "client", "token response rejected", nil)
	}
	meta := TokenMetadata{Purpose: resp.Purpose, InstallationID: resp.InstallationID, RepositoryID: resp.RepositoryID, ExpiresAt: resp.ExpiresAt.UTC(), GrantedPermissions: resp.GrantedPermissions}
	return newTokenLease(meta, []byte(resp.Token)), nil
}
func encodeRequest(req TokenRequest) ([]byte, error) { return jsonMarshal(req) }
func jsonMarshal(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, wrap(CodeRequestFrameInvalid, "request", "request JSON failed", err)
	}
	return b, nil
}
