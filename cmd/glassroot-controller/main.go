package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mattneel/glassroot/internal/githubapi"
	"github.com/mattneel/glassroot/internal/githubbroker"
	"github.com/mattneel/glassroot/internal/githubcontroller"
	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
	"github.com/mattneel/glassroot/internal/githubinbox"
)

var version = "dev"
var commit = "unknown"
var built = "unknown"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

type serveConfig struct {
	inboxStateDir, receiverID, controllerStateDir, controllerID, brokerUnix string
	appID                                                                   int64
	help                                                                    bool
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && args[0] == "--help") {
		printUsage(stdout)
		return 0
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(stdout, "glassroot-controller %s\ncommit: %s\nbuilt: %s\n", version, commit, built)
		return 0
	}
	if args[0] != "serve" {
		printUsage(stderr)
		return 2
	}
	cfg, err := parseServe(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-controller serve: %s\n", err)
		printServeUsage(stderr)
		return 2
	}
	if cfg.help {
		printServeUsage(stdout)
		return 0
	}
	return serve(cfg, stderr)
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
		case "--inbox-state-dir":
			cfg.inboxStateDir = v
		case "--receiver-id":
			cfg.receiverID = v
		case "--controller-state-dir":
			cfg.controllerStateDir = v
		case "--controller-id":
			cfg.controllerID = v
		case "--credential-broker-unix":
			cfg.brokerUnix = v
		case "--app-id":
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil || id <= 0 {
				return cfg, fmt.Errorf("invalid app id")
			}
			cfg.appID = id
		default:
			return cfg, fmt.Errorf("unknown flag %s", a)
		}
	}
	if cfg.inboxStateDir == "" || cfg.receiverID == "" || cfg.controllerStateDir == "" || cfg.controllerID == "" || cfg.brokerUnix == "" || cfg.appID <= 0 {
		return cfg, fmt.Errorf("missing required flag")
	}
	if err := validateConfiguredPath(cfg.inboxStateDir); err != nil {
		return cfg, fmt.Errorf("invalid inbox state directory")
	}
	if err := validateConfiguredPath(cfg.controllerStateDir); err != nil {
		return cfg, fmt.Errorf("invalid controller state directory")
	}
	if err := validateConfiguredPath(cfg.brokerUnix); err != nil {
		return cfg, fmt.Errorf("invalid broker socket path")
	}
	if pathsOverlap(cfg.inboxStateDir, cfg.controllerStateDir) || pathsOverlap(cfg.inboxStateDir, cfg.brokerUnix) || pathsOverlap(cfg.controllerStateDir, cfg.brokerUnix) {
		return cfg, fmt.Errorf("configured paths must not overlap")
	}
	return cfg, nil
}

func validateConfiguredPath(path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("path rejected")
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("path rejected")
		}
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if a == b {
		return true
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	if a == string(filepath.Separator) {
		return true
	}
	return strings.HasPrefix(b, a+string(filepath.Separator))
}

func serve(cfg serveConfig, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{}))
	inbox, err := githubinbox.Open(ctx, githubinbox.Config{StateDir: cfg.inboxStateDir, ReceiverID: cfg.receiverID})
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-controller serve: %s\n", githubinbox.Diagnostic(err))
		return 3
	}
	defer inbox.Close()
	store, err := githubcontrollerstore.Open(ctx, githubcontrollerstore.Config{StateDir: cfg.controllerStateDir, ControllerID: cfg.controllerID, ReceiverID: cfg.receiverID, AppID: cfg.appID})
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-controller serve: %s\n", githubcontrollerstore.Diagnostic(err))
		return 3
	}
	defer store.Close()
	broker, err := githubbroker.Dial(ctx, cfg.brokerUnix, githubbroker.DefaultLimits())
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-controller serve: %s\n", githubbroker.Diagnostic(err))
		return 3
	}
	apiClient, err := githubapi.NewInstallationClient(githubapi.InstallationClientConfig{Limits: githubapi.DefaultLimits()})
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-controller serve: %s\n", githubapi.Diagnostic(err))
		return 3
	}
	defer apiClient.CloseIdleConnections()
	ctrl, err := githubcontroller.New(githubcontroller.Config{ControllerID: cfg.controllerID, Store: store, Broker: broker, PullRequests: apiClient, Clock: realClock{}, AppID: cfg.appID})
	if err != nil {
		fmt.Fprintf(stderr, "glassroot-controller serve: %s\n", githubcontroller.Diagnostic(err))
		return 3
	}
	for {
		select {
		case <-ctx.Done():
			return 0
		default:
		}
		processed, res, err := ctrl.ProcessNext(ctx, inbox)
		if err != nil {
			logger.Error("controller", "component", "github-controller", "operation", "process", "errorCode", githubcontroller.Diagnostic(err), "decision", res.Decision)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if !processed {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC().Round(0) }
func printUsage(w io.Writer) {
	fmt.Fprint(w, "usage: glassroot-controller <command>\n\ncommands:\n  version\n  serve\n")
}
func printServeUsage(w io.Writer) {
	fmt.Fprint(w, "usage: glassroot-controller serve --inbox-state-dir ABSOLUTE_DIRECTORY --receiver-id RECEIVER_ID --controller-state-dir ABSOLUTE_DIRECTORY --controller-id CONTROLLER_ID --credential-broker-unix ABSOLUTE_SOCKET_PATH --app-id POSITIVE_INTEGER\n")
}
