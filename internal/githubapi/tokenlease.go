package githubapi

import (
	"sync"
	"time"
)

type TokenPurpose string

const (
	PurposePullRequestRead TokenPurpose = "pull-request-read"
	PurposeSourceRead      TokenPurpose = "source-read"
)

type Permission struct {
	Name   string `json:"name"`
	Access string `json:"access"`
}
type TokenMetadata struct {
	Purpose            TokenPurpose
	InstallationID     int64
	RepositoryID       int64
	ExpiresAt          time.Time
	GrantedPermissions []Permission
}

type TokenRequest struct {
	Purpose        TokenPurpose
	InstallationID int64
	RepositoryID   int64
}

type TokenLease struct {
	mu     sync.Mutex
	meta   TokenMetadata
	read   func() []byte
	clear  func()
	closed bool
}

func newTokenLease(meta TokenMetadata, token []byte) *TokenLease {
	secret := append([]byte(nil), token...)
	cp := meta
	cp.GrantedPermissions = append([]Permission(nil), meta.GrantedPermissions...)
	return &TokenLease{meta: cp, read: func() []byte { return secret }, clear: func() { zero(secret) }}
}
func NewTokenLeaseForTest(meta TokenMetadata, token []byte) *TokenLease {
	return newTokenLease(meta, token)
}
func (t *TokenLease) Metadata() TokenMetadata {
	if t == nil {
		return TokenMetadata{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := t.meta
	cp.GrantedPermissions = append([]Permission(nil), t.meta.GrantedPermissions...)
	return cp
}
func (t *TokenLease) Use(fn func([]byte) error) error {
	if t == nil || fn == nil {
		return errCode(CodeTokenResponseInvalid, "token", "token use rejected", nil)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return errCode(CodeTokenResponseInvalid, "token", "token lease closed", nil)
	}
	cp := append([]byte(nil), t.read()...)
	defer zero(cp)
	return fn(cp)
}
func (t *TokenLease) Close() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	if t.clear != nil {
		t.clear()
	}
	t.closed = true
	return nil
}
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
