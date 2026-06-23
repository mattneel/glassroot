package githubauth

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unicode/utf8"
)

type PrivateKey struct{ key *rsa.PrivateKey }

func LoadPrivateKey(path string, limits Limits) (*PrivateKey, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if err := validatePath(path, limits.MaxPathBytes); err != nil {
		return nil, errCode(CodeInvalidPrivateKeyPath, "private-key", "private key path rejected", nil)
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, wrap(CodeInvalidPrivateKeyPath, "private-key", "private key file rejected", err)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, errCode(CodePrivateKeySymlink, "private-key", "private key symlink rejected", nil)
	}
	if !before.Mode().IsRegular() {
		return nil, errCode(CodeInvalidPrivateKeyPath, "private-key", "private key file rejected", nil)
	}
	if before.Mode().Perm() != 0o400 && before.Mode().Perm() != 0o600 {
		return nil, errCode(CodePrivateKeyModeInvalid, "private-key", "private key mode rejected", nil)
	}
	if before.Size() <= 0 || before.Size() > int64(limits.MaxPrivateKeyBytes) {
		return nil, errCode(CodePrivateKeySizeInvalid, "private-key", "private key size rejected", nil)
	}
	if runtime.GOOS == "linux" {
		if sys, ok := before.Sys().(*syscall.Stat_t); ok {
			if sys.Uid != uint32(os.Geteuid()) {
				return nil, errCode(CodePrivateKeyOwnerInvalid, "private-key", "private key owner rejected", nil)
			}
			if sys.Nlink != 1 {
				return nil, errCode(CodePrivateKeyHardlink, "private-key", "private key link count rejected", nil)
			}
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, wrap(CodePrivateKeyReadFailed, "private-key", "private key open failed", err)
	}
	opened, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, wrap(CodePrivateKeyReadFailed, "private-key", "private key stat failed", err)
	}
	if !os.SameFile(before, opened) {
		_ = f.Close()
		return nil, errCode(CodePrivateKeyReadFailed, "private-key", "private key changed", nil)
	}
	buf := make([]byte, before.Size())
	_, readErr := io.ReadFull(f, buf)
	statAfterRead, statErr := f.Stat()
	closeErr := f.Close()
	if readErr != nil {
		zero(buf)
		return nil, wrap(CodePrivateKeyReadFailed, "private-key", "private key read failed", readErr)
	}
	if statErr != nil {
		zero(buf)
		return nil, wrap(CodePrivateKeyReadFailed, "private-key", "private key stat failed", statErr)
	}
	if closeErr != nil {
		zero(buf)
		return nil, wrap(CodePrivateKeyReadFailed, "private-key", "private key close failed", closeErr)
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || !sameStableFile(before, statAfterRead) || after.Mode() != before.Mode() || after.Size() != before.Size() || after.ModTime() != before.ModTime() {
		zero(buf)
		return nil, errCode(CodePrivateKeyReadFailed, "private-key", "private key changed", nil)
	}
	pk, err := ParsePrivateKeyPEM(buf, limits)
	zero(buf)
	if err != nil {
		return nil, err
	}
	return pk, nil
}

func ParsePrivateKeyPEM(data []byte, limits Limits) (*PrivateKey, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > limits.MaxPrivateKeyBytes {
		return nil, errCode(CodePrivateKeySizeInvalid, "private-key", "private key size rejected", nil)
	}
	trimmed := bytes.TrimSpace(append([]byte(nil), data...))
	block, rest := pem.Decode(trimmed)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		zero(trimmed)
		return nil, errCode(CodePrivateKeyFormatInvalid, "private-key", "private key format rejected", nil)
	}
	if x509.IsEncryptedPEMBlock(block) {
		zero(trimmed)
		return nil, errCode(CodePrivateKeyFormatInvalid, "private-key", "encrypted private key rejected", nil)
	}
	var key any
	var err error
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		zero(trimmed)
		return nil, errCode(CodePrivateKeyTypeUnsupported, "private-key", "private key type rejected", nil)
	}
	zero(trimmed)
	if err != nil {
		return nil, wrap(CodePrivateKeyFormatInvalid, "private-key", "private key parse failed", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errCode(CodePrivateKeyTypeUnsupported, "private-key", "private key type rejected", nil)
	}
	bits := rsaKey.N.BitLen()
	if bits < limits.MinRSABits || bits > limits.MaxRSABits {
		return nil, errCode(CodePrivateKeyStrengthInvalid, "private-key", "private key strength rejected", nil)
	}
	if err := rsaKey.Validate(); err != nil {
		return nil, wrap(CodePrivateKeyFormatInvalid, "private-key", "private key validation failed", err)
	}
	return &PrivateKey{key: rsaKey}, nil
}

func (p *PrivateKey) Close() error {
	if p == nil || p.key == nil {
		return nil
	}
	p.key = nil
	return nil
}

func validatePath(path string, max int) error {
	if path == "" || len(path) > max || !utf8.ValidString(path) || hasControl(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errCode(CodeInvalidPrivateKeyPath, "path", "path rejected", nil)
	}
	return nil
}

func sameStableFile(a, b os.FileInfo) bool {
	if !os.SameFile(a, b) || a.Mode() != b.Mode() || a.Size() != b.Size() || !a.ModTime().Equal(b.ModTime()) {
		return false
	}
	if runtime.GOOS == "linux" {
		as, aok := a.Sys().(*syscall.Stat_t)
		bs, bok := b.Sys().(*syscall.Stat_t)
		if aok && bok {
			return as.Ctim == bs.Ctim
		}
	}
	return true
}
