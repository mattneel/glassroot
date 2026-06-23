package githubbroker

import (
	"sync"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
)

type TokenMetadata struct {
	Purpose            TokenPurpose
	InstallationID     int64
	RepositoryID       int64
	ExpiresAt          time.Time
	GrantedPermissions []githubapi.Permission
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
	cp.GrantedPermissions = append([]githubapi.Permission(nil), meta.GrantedPermissions...)
	return &TokenLease{meta: cp, read: func() []byte { return secret }, clear: func() { zero(secret) }}
}
func (t *TokenLease) Metadata() TokenMetadata {
	if t == nil {
		return TokenMetadata{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := t.meta
	cp.GrantedPermissions = append([]githubapi.Permission(nil), t.meta.GrantedPermissions...)
	return cp
}
func (t *TokenLease) Use(fn func([]byte) error) error {
	if t == nil || fn == nil {
		return errCode(CodeTokenLeaseClosed, "token", "token lease rejected", nil)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return errCode(CodeTokenLeaseClosed, "token", "token lease closed", nil)
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
