package githubreceiver

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnixSocketServerPersistsDeliveryAndCleansSocket(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state")
	sockdir := filepath.Join(root, "sock")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sockdir, 0o700); err != nil {
		t.Fatal(err)
	}
	current := writeSecretFile(t, root, "current.secret", testCurrentSecret, 0o600)
	sock := filepath.Join(sockdir, "github.sock")
	ctx := context.Background()
	srv, err := NewServer(ctx, ServeConfig{ListenUnix: sock, StateDir: state, ReceiverID: "receiver-1", CurrentSecretFile: current, Limits: testServerLimits(), Clock: fixedClock{t: time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) { return net.Dial("unix", sock) }}}
	req := signedRequest(t, "pull_request", validPullRequestPayload("opened"), testCurrentSecret)
	req.URL.Scheme = "http"
	req.URL.Host = "unix"
	req.RequestURI = ""
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	leased, err := srv.Store.ClaimOutbox(ctx, "controller-1", time.Date(2026, 6, 23, 12, 1, 0, 0, time.UTC), time.Minute, 10)
	if err != nil || len(leased) != 1 {
		t.Fatalf("leased=%d err=%v", len(leased), err)
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("serve: %v", err)
	}
	if _, err := os.Lstat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket still exists or stat err=%v", err)
	}
}

func TestServerShutdownZeroizesLoadedSecrets(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state")
	sockdir := filepath.Join(root, "sock")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sockdir, 0o700); err != nil {
		t.Fatal(err)
	}
	current := writeSecretFile(t, root, "current.secret", testCurrentSecret, 0o600)
	previous := writeSecretFile(t, root, "previous.secret", testPreviousSecret, 0o600)
	srv, err := NewServer(context.Background(), ServeConfig{
		ListenUnix:         filepath.Join(sockdir, "github.sock"),
		StateDir:           state,
		ReceiverID:         "receiver-1",
		CurrentSecretFile:  current,
		PreviousSecretFile: previous,
		Limits:             testServerLimits(),
		Clock:              fixedClock{t: time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if len(srv.Secrets.Current) == 0 || srv.Secrets.Current[0] == 0 || len(srv.handler.secrets.Current) == 0 || srv.handler.secrets.Current[0] == 0 {
		t.Fatalf("server did not retain owned secret buffers before shutdown")
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	for _, b := range srv.Secrets.Current {
		if b != 0 {
			t.Fatalf("loaded current secret not zeroized")
		}
	}
	for _, b := range srv.Secrets.Previous {
		if b != 0 {
			t.Fatalf("loaded previous secret not zeroized")
		}
	}
	for _, b := range srv.handler.secrets.Current {
		if b != 0 {
			t.Fatalf("handler current secret not zeroized")
		}
	}
	for _, b := range srv.handler.secrets.Previous {
		if b != 0 {
			t.Fatalf("handler previous secret not zeroized")
		}
	}
}

func TestListenerDoesNotRemoveReplacedSocketPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "github.sock")
	ln, err := ListenUnix(sock, "", DefaultLimits())
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}
	if err := os.Remove(sock); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sock, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ln.CloseAndRemove(); err != nil {
		t.Fatalf("close/remove: %v", err)
	}
	b, err := os.ReadFile(sock)
	if err != nil || string(b) != "replacement" {
		t.Fatalf("replacement removed or changed: %q err=%v", b, err)
	}
}

func testServerLimits() Limits {
	l := DefaultLimits()
	l.GitHub.MinWebhookSecretBytes = 1
	l.ReadHeaderTimeout = time.Second
	l.ReadTimeout = 2 * time.Second
	l.PerRequestIntakeTimeout = 2 * time.Second
	l.WriteTimeout = time.Second
	l.ShutdownTimeout = time.Second
	return l
}
