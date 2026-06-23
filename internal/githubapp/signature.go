package githubapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

type WebhookSecrets struct {
	Current  []byte
	Previous []byte
}

type SecretGeneration string

const (
	SecretGenerationCurrent  SecretGeneration = "current"
	SecretGenerationPrevious SecretGeneration = "previous"
)

func ParseGitHubSignatureHeader(header string, limits Limits) ([32]byte, error) {
	var out [32]byte
	if err := validateLimits(limits); err != nil {
		return out, err
	}
	if header == "" {
		return out, errCode(CodeMissingSignature, "signature", "signature header is required", nil)
	}
	if len(header) > limits.MaxSignatureHeaderBytes {
		return out, errCode(CodeInvalidSignatureFormat, "signature", "signature header is too large", nil)
	}
	const prefix = "sha256="
	if len(header) != len(prefix)+64 || header[:len(prefix)] != prefix {
		return out, errCode(CodeInvalidSignatureFormat, "signature", "signature must be sha256 lowercase hex", nil)
	}
	for _, c := range header[len(prefix):] {
		if c < '0' || (c > '9' && c < 'a') || c > 'f' {
			return out, errCode(CodeInvalidSignatureFormat, "signature", "signature must be lowercase hex", nil)
		}
	}
	decoded, err := hex.DecodeString(header[len(prefix):])
	if err != nil || len(decoded) != sha256.Size {
		return out, errCode(CodeInvalidSignatureFormat, "signature", "signature digest invalid", nil)
	}
	copy(out[:], decoded)
	return out, nil
}

func VerifyWebhookSignature(body []byte, signatureHeader string, secrets WebhookSecrets, limits Limits) (SecretGeneration, error) {
	if err := validateLimits(limits); err != nil {
		return "", err
	}
	if len(body) > limits.MaxWebhookBodyBytes {
		return "", errCode(CodeBodyTooLarge, "signature", "webhook body exceeds Glassroot intake bound", nil)
	}
	if err := validateSecret(secrets.Current, limits); err != nil {
		return "", err
	}
	if len(secrets.Previous) > 0 {
		if err := validateSecret(secrets.Previous, limits); err != nil {
			return "", err
		}
	}
	want, err := ParseGitHubSignatureHeader(signatureHeader, limits)
	if err != nil {
		return "", err
	}
	cur := hmacSHA256(body, secrets.Current)
	prev := [32]byte{}
	if len(secrets.Previous) > 0 {
		prev = hmacSHA256(body, secrets.Previous)
	}
	curMatch := subtle.ConstantTimeCompare(cur[:], want[:])
	prevMatch := 0
	if len(secrets.Previous) > 0 {
		prevMatch = subtle.ConstantTimeCompare(prev[:], want[:])
	}
	if curMatch == 1 {
		return SecretGenerationCurrent, nil
	}
	if prevMatch == 1 {
		return SecretGenerationPrevious, nil
	}
	return "", errCode(CodeSignatureMismatch, "signature", "signature did not match configured secret generations", nil)
}

func validateSecret(secret []byte, limits Limits) error {
	if len(secret) < limits.MinWebhookSecretBytes || len(secret) > limits.MaxWebhookSecretBytes {
		return errCode(CodeInvalidSecretSet, "signature", "webhook secret length is outside configured bounds", nil)
	}
	return nil
}

func hmacSHA256(body, secret []byte) [32]byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	sum := mac.Sum(nil)
	var out [32]byte
	copy(out[:], sum)
	return out
}
