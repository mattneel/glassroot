package githubapp

import (
	"mime"
	"strings"
)

type HeaderValue struct {
	Name  string
	Value string
}

type WebhookHeaders struct {
	DeliveryID             string
	Event                  string
	Signature256           string
	ContentType            string
	Charset                string
	HookID                 string
	InstallationTargetID   string
	InstallationTargetType string
}

func ParseWebhookHeaders(values []HeaderValue, limits Limits) (WebhookHeaders, error) {
	var out WebhookHeaders
	if err := validateLimits(limits); err != nil {
		return out, err
	}
	seenRequired := map[string]string{}
	optional := map[string]string{}
	for _, h := range values {
		name := strings.ToLower(strings.TrimSpace(h.Name))
		if name == "" {
			continue
		}
		if len(h.Value) > limits.MaxHeaderValueBytes || hasControl(h.Value) {
			return out, errCode(CodeInvalidEventName, "headers", "header value rejected", nil)
		}
		switch name {
		case "x-github-delivery", "x-github-event", "x-hub-signature-256", "content-type":
			if _, ok := seenRequired[name]; ok {
				code := CodeDuplicateRequiredHeader
				if name == "x-hub-signature-256" {
					code = CodeDuplicateSignatureHeader
				}
				return out, errCode(code, "headers", "required header appears more than once", nil)
			}
			seenRequired[name] = h.Value
		case "content-encoding":
			if strings.ToLower(strings.TrimSpace(h.Value)) != "identity" {
				return out, errCode(CodeUnsupportedContentEncoding, "headers", "content encoding must be identity", nil)
			}
		case "x-github-hook-id", "x-github-hook-installation-target-id", "x-github-hook-installation-target-type":
			optional[name] = h.Value
		}
	}
	for _, name := range []string{"x-github-delivery", "x-github-event", "x-hub-signature-256", "content-type"} {
		if _, ok := seenRequired[name]; !ok {
			if name == "x-hub-signature-256" {
				return out, errCode(CodeMissingSignature, "headers", "signature header is required", nil)
			}
			return out, errCode(CodeMissingRequiredHeader, "headers", "required header missing", nil)
		}
	}
	if err := validateDeliveryID(seenRequired["x-github-delivery"], limits); err != nil {
		return out, err
	}
	if err := validateEventName(seenRequired["x-github-event"], limits); err != nil {
		return out, err
	}
	mediaType, params, err := mime.ParseMediaType(seenRequired["content-type"])
	if err != nil || strings.ToLower(mediaType) != "application/json" {
		return out, errCode(CodeInvalidContentType, "headers", "content type must be application/json", nil)
	}
	charset := ""
	if len(params) > 0 {
		if len(params) != 1 || strings.ToLower(params["charset"]) != "utf-8" {
			return out, errCode(CodeInvalidContentType, "headers", "only charset=utf-8 parameter is permitted", nil)
		}
		charset = "utf-8"
	}
	out = WebhookHeaders{DeliveryID: seenRequired["x-github-delivery"], Event: seenRequired["x-github-event"], Signature256: seenRequired["x-hub-signature-256"], ContentType: "application/json", Charset: charset, HookID: optional["x-github-hook-id"], InstallationTargetID: optional["x-github-hook-installation-target-id"], InstallationTargetType: optional["x-github-hook-installation-target-type"]}
	return out, nil
}

func validateDeliveryID(s string, limits Limits) error {
	if s == "" || len(s) > limits.MaxDeliveryIDBytes || hasControl(s) {
		return errCode(CodeInvalidDeliveryID, "headers", "delivery id is invalid", nil)
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return errCode(CodeInvalidDeliveryID, "headers", "delivery id is invalid", nil)
		}
	}
	return nil
}

func validateEventName(s string, limits Limits) error {
	if s == "" || len(s) > limits.MaxEventNameBytes || hasControl(s) {
		return errCode(CodeInvalidEventName, "headers", "event name is invalid", nil)
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return errCode(CodeInvalidEventName, "headers", "event name is invalid", nil)
		}
	}
	return nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
