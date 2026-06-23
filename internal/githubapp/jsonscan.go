package githubapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

func PreflightGitHubWebhookJSON(body []byte, limits Limits) error {
	if err := validateLimits(limits); err != nil {
		return err
	}
	if len(body) > limits.MaxWebhookBodyBytes {
		return errCode(CodeBodyTooLarge, "json", "webhook body exceeds Glassroot intake bound", nil)
	}
	if !utf8.Valid(body) {
		return errCode(CodeInvalidUTF8, "json", "webhook body must be valid UTF-8", nil)
	}
	if len(body) >= 3 && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		return errCode(CodeInvalidJSON, "json", "UTF-8 BOM is rejected", nil)
	}
	for _, b := range body {
		if b == 0 {
			return errCode(CodeInvalidJSON, "json", "raw NUL is rejected", nil)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := scanJSONValue(dec, limits, 1, newJSONCounters(limits), true); err != nil {
		return err
	}
	if tok, err := dec.Token(); err != io.EOF {
		if err == nil {
			_ = tok
			return errCode(CodeTrailingJSON, "json", "trailing JSON value rejected", nil)
		}
		return errCode(CodeInvalidJSON, "json", "invalid trailing JSON", err)
	}
	return nil
}

type jsonCounters struct {
	tokens int
	limit  Limits
}

func newJSONCounters(l Limits) *jsonCounters { return &jsonCounters{limit: l} }

func (c *jsonCounters) add() error {
	c.tokens++
	if c.tokens > c.limit.MaxJSONTokens {
		return errCode(CodeJSONTokenLimit, "json", "JSON token limit exceeded", nil)
	}
	return nil
}

func scanJSONValue(dec *json.Decoder, limits Limits, depth int, counters *jsonCounters, top bool) error {
	if depth > limits.MaxJSONDepth {
		return errCode(CodeJSONDepthLimit, "json", "JSON depth limit exceeded", nil)
	}
	tok, err := dec.Token()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return errCode(CodeInvalidJSON, "json", "missing JSON value", nil)
		}
		return errCode(CodeInvalidJSON, "json", "invalid JSON", err)
	}
	if err := counters.add(); err != nil {
		return err
	}
	switch v := tok.(type) {
	case json.Delim:
		switch v {
		case '{':
			members := map[string]struct{}{}
			count := 0
			for dec.More() {
				ktok, err := dec.Token()
				if err != nil {
					return errCode(CodeInvalidJSON, "json", "invalid object key", err)
				}
				if err := counters.add(); err != nil {
					return err
				}
				key, ok := ktok.(string)
				if !ok {
					return errCode(CodeInvalidJSON, "json", "object key must be string", nil)
				}
				if len(key) > limits.MaxJSONStringBytes || containsNUL(key) {
					return errCode(CodeInvalidJSON, "json", "object key rejected", nil)
				}
				if _, ok := members[key]; ok {
					return errCode(CodeDuplicateJSONMember, "json", "duplicate object member rejected", nil)
				}
				members[key] = struct{}{}
				count++
				if count > limits.MaxMembersPerObject {
					return errCode(CodeJSONTokenLimit, "json", "object member limit exceeded", nil)
				}
				if err := scanJSONValue(dec, limits, depth+1, counters, false); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil {
				return errCode(CodeInvalidJSON, "json", "unterminated object", err)
			}
			if d, ok := end.(json.Delim); !ok || d != '}' {
				return errCode(CodeInvalidJSON, "json", "object close expected", nil)
			}
			return counters.add()
		case '[':
			if top {
				return errCode(CodeInvalidJSON, "json", "top-level JSON must be an object", nil)
			}
			count := 0
			for dec.More() {
				count++
				if count > limits.MaxArrayElements {
					return errCode(CodeJSONTokenLimit, "json", "array element limit exceeded", nil)
				}
				if err := scanJSONValue(dec, limits, depth+1, counters, false); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil {
				return errCode(CodeInvalidJSON, "json", "unterminated array", err)
			}
			if d, ok := end.(json.Delim); !ok || d != ']' {
				return errCode(CodeInvalidJSON, "json", "array close expected", nil)
			}
			return counters.add()
		default:
			return errCode(CodeInvalidJSON, "json", "unexpected delimiter", nil)
		}
	case string:
		if top {
			return errCode(CodeInvalidJSON, "json", "top-level JSON must be an object", nil)
		}
		if len(v) > limits.MaxJSONStringBytes || containsNUL(v) {
			return errCode(CodeInvalidJSON, "json", "string exceeds bounds or contains NUL", nil)
		}
	case json.Number:
		if top {
			return errCode(CodeInvalidJSON, "json", "top-level JSON must be an object", nil)
		}
		if len(v.String()) > limits.MaxJSONNumberBytes {
			return errCode(CodeInvalidJSON, "json", "number exceeds bounds", nil)
		}
	case bool, nil:
		if top {
			return errCode(CodeInvalidJSON, "json", "top-level JSON must be an object", nil)
		}
	default:
		return errCode(CodeInvalidJSON, "json", "unsupported JSON token", nil)
	}
	return nil
}

func containsNUL(s string) bool {
	for _, r := range s {
		if r == 0 {
			return true
		}
	}
	return false
}
