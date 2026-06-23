package githubapi

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/githubapp"
)

func preflightJSON(body []byte, limits Limits) error {
	if len(body) > limits.MaxAPIResponseBytes {
		return errCode(CodeResponseTooLarge, "response", "response body exceeds bound", nil)
	}
	if !utf8.Valid(body) {
		return errCode(CodeResponseInvalid, "response", "response must be valid UTF-8", nil)
	}
	gl := githubapp.DefaultLimits()
	gl.MaxWebhookBodyBytes = limits.MaxAPIResponseBytes
	gl.MaxJSONDepth = limits.MaxJSONDepth
	gl.MaxJSONTokens = limits.MaxJSONTokens
	gl.MaxJSONStringBytes = limits.MaxJSONStringBytes
	if err := githubapp.PreflightGitHubWebhookJSON(body, gl); err != nil {
		return wrap(CodeResponseInvalid, "response", "response JSON rejected", err)
	}
	return nil
}
func decodeStrict(body []byte, limits Limits, out any) error {
	if err := preflightJSON(body, limits); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(out); err != nil {
		return wrap(CodeResponseInvalid, "response", "response decode failed", err)
	}
	if tok, err := dec.Token(); err != io.EOF {
		_ = tok
		return errCode(CodeResponseInvalid, "response", "trailing response rejected", err)
	}
	return nil
}
func compactJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, wrap(CodeTokenRequestRejected, "request", "request JSON failed", err)
	}
	return b, nil
}
