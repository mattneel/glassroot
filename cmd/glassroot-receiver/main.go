package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mattneel/glassroot/internal/githubreceiver"
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
		fmt.Fprintf(stdout, "glassroot-receiver %s\ncommit: %s\nbuilt: %s\n", version, commit, built)
		return 0
	}
	if args[0] != "serve" {
		printUsage(stderr)
		return 2
	}
	parsed, err := parseServe(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-receiver serve: %s\n", err.Error())
		printServeUsage(stderr)
		return 2
	}
	if parsed.help {
		printServeUsage(stdout)
		return 0
	}
	return serve(parsed, stderr)
}

type serveConfig struct {
	listenUnix, stateDir, receiverID, currentSecretFile, previousSecretFile string
	help                                                                    bool
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
		case "--state-dir":
			cfg.stateDir = v
		case "--receiver-id":
			cfg.receiverID = v
		case "--current-secret-file":
			cfg.currentSecretFile = v
		case "--previous-secret-file":
			cfg.previousSecretFile = v
		default:
			return cfg, fmt.Errorf("unknown flag %s", a)
		}
	}
	if cfg.listenUnix == "" || cfg.stateDir == "" || cfg.receiverID == "" || cfg.currentSecretFile == "" {
		return cfg, fmt.Errorf("missing required flag")
	}
	return cfg, nil
}

func serve(cfg serveConfig, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{}))
	srv, err := githubreceiver.NewServer(ctx, githubreceiver.ServeConfig{ListenUnix: cfg.listenUnix, StateDir: cfg.stateDir, ReceiverID: cfg.receiverID, CurrentSecretFile: cfg.currentSecretFile, PreviousSecretFile: cfg.previousSecretFile, Limits: githubreceiver.DefaultLimits(), Clock: realClock{}, Logger: logger})
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-receiver serve: %s\n", githubreceiver.Diagnostic(err))
		return 3
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), githubreceiver.DefaultLimits().ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(stderr, "glassroot-receiver serve: %s\n", githubreceiver.Diagnostic(err))
			return 3
		}
		if err := <-done; err != nil {
			fmt.Fprintf(stderr, "glassroot-receiver serve: %s\n", githubreceiver.Diagnostic(err))
			return 3
		}
		return 0
	case err := <-done:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), githubreceiver.DefaultLimits().ShutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		if err != nil {
			fmt.Fprintf(stderr, "glassroot-receiver serve: %s\n", githubreceiver.Diagnostic(err))
			return 3
		}
		return 0
	}
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC().Round(0) }

func printUsage(w io.Writer) {
	fmt.Fprint(w, "usage: glassroot-receiver <command>\n\ncommands:\n  version\n  serve\n")
}
func printServeUsage(w io.Writer) {
	fmt.Fprint(w, "usage: glassroot-receiver serve --listen-unix ABSOLUTE_SOCKET_PATH --state-dir ABSOLUTE_STATE_DIRECTORY --receiver-id RECEIVER_ID --current-secret-file ABSOLUTE_SECRET_FILE [--previous-secret-file ABSOLUTE_SECRET_FILE]\n")
}
