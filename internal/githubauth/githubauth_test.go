package githubauth_test

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubauth"
)

func TestLoadPrivateKeyAcceptsPKCS1AndSignsBoundedRS256JWT(t *testing.T) {
	key := mustRSA(t, 2048)
	path := writeKeyFile(t, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600)
	loaded, err := githubauth.LoadPrivateKey(path, githubauth.DefaultLimits())
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	defer loaded.Close()

	fixed := time.Date(2026, 6, 23, 12, 34, 56, 0, time.UTC)
	jwt, err := loaded.SignJWT(githubauth.AppIdentity{AppID: 123, ClientID: "Iv1.abcdef0123456789"}, fixed, githubauth.DefaultLimits())
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}
	defer jwt.Close()

	var token []byte
	if err := jwt.Use(func(b []byte) error {
		token = append([]byte(nil), b...)
		return nil
	}); err != nil {
		t.Fatalf("Use JWT: %v", err)
	}
	parts := strings.Split(string(token), ".")
	if len(parts) != 3 {
		t.Fatalf("JWT parts = %d", len(parts))
	}
	var header map[string]string
	decodeJSONPart(t, parts[0], &header)
	if header["alg"] != "RS256" || header["typ"] != "JWT" || len(header) != 2 {
		t.Fatalf("unexpected header: %#v", header)
	}
	var claims map[string]any
	decodeJSONPart(t, parts[1], &claims)
	if got := int64(claims["iat"].(float64)); got != fixed.Add(-60*time.Second).Unix() {
		t.Fatalf("iat = %d", got)
	}
	if got := int64(claims["exp"].(float64)); got != fixed.Add(9*time.Minute).Unix() {
		t.Fatalf("exp = %d", got)
	}
	if claims["iss"] != "Iv1.abcdef0123456789" || len(claims) != 3 {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("signature b64: %v", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, cryptoHashSHA256(), digest[:], sig); err != nil {
		t.Fatalf("RS256 signature did not verify: %v", err)
	}
	formatted := fmt.Sprintf("%v %+v", jwt, jwt)
	if strings.Contains(formatted, string(token)) || strings.Contains(formatted, parts[2]) {
		t.Fatalf("JWT leaked through formatting: %q", formatted)
	}
}

func TestLoadPrivateKeyAcceptsPKCS8RSA(t *testing.T) {
	key := mustRSA(t, 2048)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	path := writeKeyFile(t, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o400)
	loaded, err := githubauth.LoadPrivateKey(path, githubauth.DefaultLimits())
	if err != nil {
		t.Fatalf("LoadPrivateKey PKCS8: %v", err)
	}
	loaded.Close()
}

func TestLoadPrivateKeyRejectsWeakSymlinkAndPermissiveFiles(t *testing.T) {
	weak := mustRSA(t, 1024)
	weakPath := writeKeyFile(t, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(weak)}), 0o600)
	if _, err := githubauth.LoadPrivateKey(weakPath, githubauth.DefaultLimits()); err == nil {
		t.Fatalf("weak RSA key accepted")
	}

	strong := mustRSA(t, 2048)
	permissive := writeKeyFile(t, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(strong)}), 0o644)
	if _, err := githubauth.LoadPrivateKey(permissive, githubauth.DefaultLimits()); err == nil {
		t.Fatalf("group/world readable key accepted")
	}

	dir := t.TempDir()
	target := writeKeyFile(t, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(strong)}), 0o600)
	link := filepath.Join(dir, "key.pem")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := githubauth.LoadPrivateKey(link, githubauth.DefaultLimits()); err == nil {
		t.Fatalf("symlink key accepted")
	}
}

func TestParsePrivateKeyRejectsNonRSAEncryptedAndMultiplePEM(t *testing.T) {
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{1, 2, 3}})
	if _, err := githubauth.ParsePrivateKeyPEM(cert, githubauth.DefaultLimits()); err == nil {
		t.Fatalf("certificate accepted as private key")
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecDER, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := githubauth.ParsePrivateKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: ecDER}), githubauth.DefaultLimits()); err == nil {
		t.Fatalf("EC key accepted")
	}
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edDER, err := x509.MarshalPKCS8PrivateKey(edKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := githubauth.ParsePrivateKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: edDER}), githubauth.DefaultLimits()); err == nil {
		t.Fatalf("Ed25519 key accepted")
	}
	key := mustRSA(t, 2048)
	one := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if _, err := githubauth.ParsePrivateKeyPEM(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Headers: map[string]string{"Proc-Type": "4,ENCRYPTED"}, Bytes: []byte{1, 2, 3}}), githubauth.DefaultLimits()); err == nil {
		t.Fatalf("encrypted PEM accepted")
	}
	if _, err := githubauth.ParsePrivateKeyPEM(append(one, one...), githubauth.DefaultLimits()); err == nil {
		t.Fatalf("multiple PEM blocks accepted")
	}
	if _, err := githubauth.ParsePrivateKeyPEM(append(one, []byte("trailing")...), githubauth.DefaultLimits()); err == nil {
		t.Fatalf("trailing non-whitespace accepted")
	}
}

func FuzzParseGitHubAppPrivateKey(f *testing.F) {
	f.Add([]byte("not pem"))
	f.Add([]byte("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = githubauth.ParsePrivateKeyPEM(b, githubauth.DefaultLimits())
	})
}

func FuzzEncodeGitHubAppJWT(f *testing.F) {
	key := mustRSA(f, 2048)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	loaded, err := githubauth.ParsePrivateKeyPEM(pemBytes, githubauth.DefaultLimits())
	if err != nil {
		f.Fatal(err)
	}
	f.Add("Iv1.abcdef0123456789", int64(1782218096))
	f.Fuzz(func(t *testing.T, clientID string, unix int64) {
		_, _ = loaded.SignJWT(githubauth.AppIdentity{AppID: 1, ClientID: clientID}, time.Unix(unix, 0).UTC(), githubauth.DefaultLimits())
	})
}

func mustRSA(tb testing.TB, bits int) *rsa.PrivateKey {
	tb.Helper()
	k, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		tb.Fatal(err)
	}
	return k
}

func writeKeyFile(t *testing.T, data []byte, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func decodeJSONPart(t *testing.T, part string, out any) {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		t.Fatalf("b64 decode: %v", err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("json decode: %v", err)
	}
}

func cryptoHashSHA256() crypto.Hash { return crypto.SHA256 }
