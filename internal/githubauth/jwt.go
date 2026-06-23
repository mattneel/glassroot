package githubauth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

func (p *PrivateKey) SignJWT(identity AppIdentity, now time.Time, limits Limits) (*AppJWT, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := ValidateAppIdentity(identity, limits); err != nil {
		return nil, err
	}
	if p == nil || p.key == nil {
		return nil, errCode(CodeJWTSignFailed, "jwt", "private key unavailable", nil)
	}
	now = now.UTC().Round(0)
	if now.IsZero() || now.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) || now.After(time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return nil, errCode(CodeJWTTimeInvalid, "jwt", "jwt time rejected", nil)
	}
	header := struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}{Alg: "RS256", Typ: "JWT"}
	claims := struct {
		IAT int64  `json:"iat"`
		EXP int64  `json:"exp"`
		ISS string `json:"iss"`
	}{IAT: now.Add(-60 * time.Second).Unix(), EXP: now.Add(9 * time.Minute).Unix(), ISS: identity.ClientID}
	hb, err := json.Marshal(header)
	if err != nil {
		return nil, wrap(CodeJWTSignFailed, "jwt", "jwt header failed", err)
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return nil, wrap(CodeJWTSignFailed, "jwt", "jwt claims failed", err)
	}
	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(hb) + "." + enc.EncodeToString(cb)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, digest[:])
	if err != nil {
		return nil, wrap(CodeJWTSignFailed, "jwt", "jwt signature failed", err)
	}
	jwt := []byte(signingInput + "." + enc.EncodeToString(sig))
	zero(sig)
	if len(jwt) > limits.MaxJWTBytes || strings.Count(string(jwt), ".") != 2 {
		zero(jwt)
		return nil, errCode(CodeJWTSignFailed, "jwt", "jwt size rejected", nil)
	}
	return newAppJWT(jwt), nil
}
