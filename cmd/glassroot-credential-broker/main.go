package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubauth"
	"github.com/mattneel/glassroot/internal/githubbroker"
)

var (
	version = "dev"
	commit  = "unknown"
	built   = "unknown"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && args[0] == "--help") {
		printUsage(stdout)
		return 0
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(stdout, "glassroot-credential-broker %s\ncommit: %s\nbuilt: %s\n", version, commit, built)
		return 0
	}
	if args[0] != "serve" {
		printUsage(stderr)
		return 2
	}
	cfg, err := parseServe(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-credential-broker serve: %s\n", err.Error())
		printServeUsage(stderr)
		return 2
	}
	if cfg.help {
		printServeUsage(stdout)
		return 0
	}
	return serve(cfg, stderr)
}

type serveConfig struct {
	listenUnix, privateKeyFile, appClientID string
	appID                                   int64
	help                                    bool
}

func parseServe(args []string) (serveConfig, error) {
	var cfg serveConfig
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--help" {
			if len(args) != 1 {
				return cfg, fmt.Errorf("--help cannot be combined")
			}
			cfg.help = true
			return cfg, nil
		}
		if !strings.HasPrefix(a, "--") {
			return cfg, fmt.Errorf("unexpected positional argument")
		}
		if seen[a] {
			return cfg, fmt.Errorf("duplicate flag %s", a)
		}
		seen[a] = true
		if i+1 >= len(args) {
			return cfg, fmt.Errorf("missing value for %s", a)
		}
		v := args[i+1]
		i++
		switch a {
		case "--listen-unix":
			cfg.listenUnix = v
		case "--private-key-file":
			cfg.privateKeyFile = v
		case "--app-id":
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil || id <= 0 {
				return cfg, fmt.Errorf("invalid app id")
			}
			cfg.appID = id
		case "--app-client-id":
			cfg.appClientID = v
		default:
			return cfg, fmt.Errorf("unknown flag %s", a)
		}
	}
	if cfg.listenUnix == "" || cfg.privateKeyFile == "" || cfg.appID <= 0 || cfg.appClientID == "" {
		return cfg, fmt.Errorf("missing required flag")
	}
	if err := githubauth.ValidateAppIdentity(githubauth.AppIdentity{AppID: cfg.appID, ClientID: cfg.appClientID}, githubauth.DefaultLimits()); err != nil {
		return cfg, fmt.Errorf("invalid app identity")
	}
	return cfg, nil
}

func serve(cfg serveConfig, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{}))
	key, err := githubauth.LoadPrivateKey(cfg.privateKeyFile, githubauth.DefaultLimits())
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-credential-broker serve: %s\n", githubauth.Diagnostic(err))
		return 3
	}
	defer key.Close()
	apiClient, err := githubapi.NewClient(githubapi.Config{Identity: githubauth.AppIdentity{AppID: cfg.appID, ClientID: cfg.appClientID}, Signer: key, Clock: realClock{}, Limits: githubapi.DefaultLimits(), AuthLimits: githubauth.DefaultLimits()})
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-credential-broker serve: %s\n", githubapi.Diagnostic(err))
		return 3
	}
	defer apiClient.CloseIdleConnections()
	if err := apiClient.VerifyApp(ctx); err != nil {
		fmt.Fprintf(stderr, "glassroot-credential-broker serve: %s\n", githubapi.Diagnostic(err))
		return 3
	}
	srv, err := githubbroker.NewServer(githubbroker.ServerConfig{ListenUnix: cfg.listenUnix, Issuer: apiClient, Logger: logger, Limits: githubbroker.DefaultLimits()})
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-credential-broker serve: %s\n", githubbroker.Diagnostic(err))
		return 3
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(stderr, "glassroot-credential-broker serve: %s\n", githubbroker.Diagnostic(err))
			return 3
		}
		if err := <-done; err != nil {
			fmt.Fprintf(stderr, "glassroot-credential-broker serve: %s\n", githubbroker.Diagnostic(err))
			return 3
		}
		return 0
	case err := <-done:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		if err != nil {
			fmt.Fprintf(stderr, "glassroot-credential-broker serve: %s\n", githubbroker.Diagnostic(err))
			return 3
		}
		return 0
	}
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC().Round(0) }

func printUsage(w io.Writer) {
	fmt.Fprint(w, "usage: glassroot-credential-broker <command>\n\ncommands:\n  version\n  serve\n")
}
func printServeUsage(w io.Writer) {
	fmt.Fprint(w, "usage: glassroot-credential-broker serve --listen-unix ABSOLUTE_SOCKET_PATH --private-key-file ABSOLUTE_PEM_PATH --app-id POSITIVE_INTEGER --app-client-id CLIENT_ID\n")
}
