package githubauth

import (
	"sync"
)

type AppJWT struct {
	mu     sync.Mutex
	read   func() []byte
	clear  func()
	closed bool
}

func newAppJWT(value []byte) *AppJWT {
	secret := append([]byte(nil), value...)
	return &AppJWT{read: func() []byte { return secret }, clear: func() { zero(secret) }}
}

func NewAppJWTForTest(value []byte) *AppJWT { return newAppJWT(value) }

func (j *AppJWT) Use(fn func([]byte) error) error {
	if j == nil || fn == nil {
		return errCode(CodeJWTSignFailed, "jwt", "jwt use rejected", nil)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return errCode(CodeJWTSignFailed, "jwt", "jwt closed", nil)
	}
	copy := append([]byte(nil), j.read()...)
	defer zero(copy)
	return fn(copy)
}
func (j *AppJWT) Close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	if j.clear != nil {
		j.clear()
	}
	j.closed = true
	return nil
}
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
