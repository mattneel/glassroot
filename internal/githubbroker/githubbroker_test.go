package githubbroker_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubbroker"
)

func TestBrokerProtocolRejectsInvalidRequestsBeforeIssuer(t *testing.T) {
	issuer := &fakeIssuer{}
	server := newTestServer(t, issuer)
	resp, err := rawFrameRequest(server.SocketPath(), []byte(`{"schemaVersion":"wrong","purpose":"source-read","installationId":1,"repositoryId":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(resp, []byte(`"errorCode"`)) || bytes.Contains(resp, []byte("\"token\"")) {
		t.Fatalf("unexpected error response: %s", resp)
	}
	if issuer.calls.Load() != 0 {
		t.Fatalf("issuer called for invalid request")
	}
}

func TestClientRequestsSourceTokenAndOwnsLease(t *testing.T) {
	issuer := &fakeIssuer{token: []byte("opaque-token-canary"), expires: time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC)}
	server := newTestServer(t, issuer)
	client, err := githubbroker.Dial(context.Background(), server.SocketPath(), githubbroker.DefaultLimits())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	lease, err := client.RequestToken(context.Background(), githubbroker.TokenRequest{SchemaVersion: githubbroker.SchemaTokenRequestV1Alpha1, Purpose: githubbroker.PurposeSourceRead, InstallationID: 7, RepositoryID: 9})
	if err != nil {
		t.Fatalf("RequestToken: %v", err)
	}
	defer lease.Close()
	if issuer.calls.Load() != 1 {
		t.Fatalf("issuer calls = %d", issuer.calls.Load())
	}
	if lease.Metadata().Purpose != githubbroker.PurposeSourceRead || lease.Metadata().InstallationID != 7 || lease.Metadata().RepositoryID != 9 {
		t.Fatalf("metadata mismatch: %#v", lease.Metadata())
	}
	var got []byte
	if err := lease.Use(func(token []byte) error { got = append([]byte(nil), token...); return nil }); err != nil {
		t.Fatal(err)
	}
	if string(got) != "opaque-token-canary" {
		t.Fatalf("token mismatch")
	}
	formatted := fmt.Sprintf("%v %+v", lease, lease)
	if strings.Contains(formatted, "opaque-token-canary") {
		t.Fatalf("token leaked through formatting: %q", formatted)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Use(func([]byte) error { return nil }); err == nil {
		t.Fatalf("Use after Close accepted")
	}
}

func TestListenUnixValidatesParentAndRemovesOnlyOwnedSocket(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "broker.sock")
	ln, err := githubbroker.ListenUnix(path, githubbroker.DefaultLimits())
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}
	st, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 || st.Mode()&os.ModeSocket == 0 {
		t.Fatalf("bad socket mode/type: %v", st.Mode())
	}
	if err := ln.CloseAndRemove(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("socket remains or stat failed: %v", err)
	}

	badParent := filepath.Join(t.TempDir(), "bad")
	if err := os.Mkdir(badParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := githubbroker.ListenUnix(filepath.Join(badParent, "x.sock"), githubbroker.DefaultLimits()); err == nil {
		t.Fatalf("non-private parent accepted")
	}
}

func FuzzDecodeGitHubBrokerFrame(f *testing.F) {
	f.Add([]byte{0, 0, 0, 2, '{', '}'})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = githubbroker.DecodeRequestFrameForTest(b, githubbroker.DefaultLimits())
	})
}

func FuzzValidateTokenRequest(f *testing.F) {
	f.Add("source-read", int64(1), int64(2))
	f.Fuzz(func(t *testing.T, purpose string, installation, repo int64) {
		_ = githubbroker.ValidateTokenRequest(githubbroker.TokenRequest{SchemaVersion: githubbroker.SchemaTokenRequestV1Alpha1, Purpose: githubbroker.TokenPurpose(purpose), InstallationID: installation, RepositoryID: repo}, githubbroker.DefaultLimits())
	})
}

func newTestServer(t *testing.T, issuer githubbroker.Issuer) *githubbroker.Server {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "broker.sock")
	server, err := githubbroker.NewServer(githubbroker.ServerConfig{ListenUnix: path, Issuer: issuer, Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)), Limits: githubbroker.DefaultLimits()})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	go func() { _ = server.Serve() }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("socket not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return server
}

type fakeIssuer struct {
	calls   atomic.Int64
	token   []byte
	expires time.Time
}

func (f *fakeIssuer) IssueInstallationToken(ctx context.Context, req githubapi.TokenRequest) (*githubapi.TokenLease, error) {
	f.calls.Add(1)
	if len(f.token) == 0 {
		f.token = []byte("token")
	}
	if f.expires.IsZero() {
		f.expires = time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC)
	}
	return githubapi.NewTokenLeaseForTest(githubapi.TokenMetadata{Purpose: req.Purpose, InstallationID: req.InstallationID, RepositoryID: req.RepositoryID, ExpiresAt: f.expires, GrantedPermissions: []githubapi.Permission{{Name: string(req.Purpose), Access: "read"}}}, f.token), nil
}

func rawFrameRequest(socket string, payload []byte) ([]byte, error) {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	var frame bytes.Buffer
	frame.Write([]byte{byte(len(payload) >> 24), byte(len(payload) >> 16), byte(len(payload) >> 8), byte(len(payload))})
	frame.Write(payload)
	if _, err := conn.Write(frame.Bytes()); err != nil {
		return nil, err
	}
	buf := make([]byte, 65536)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	if n < 4 {
		return nil, fmt.Errorf("short frame")
	}
	ln := int(buf[0])<<24 | int(buf[1])<<16 | int(buf[2])<<8 | int(buf[3])
	return append([]byte(nil), buf[4:4+ln]...), nil
}
