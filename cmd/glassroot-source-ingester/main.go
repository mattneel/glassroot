package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/mattneel/glassroot/internal/githubbroker"
	"github.com/mattneel/glassroot/internal/githubcontrollerstore"
	"github.com/mattneel/glassroot/internal/githubsource"
)

const version = "glassroot-source-ingester dev"

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC().Round(0) }

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: glassroot-source-ingester <version|serve>")
		return 2
	}
	switch args[0] {
	case "version":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "version accepts no arguments")
			return 2
		}
		fmt.Fprintln(stdout, version)
		return 0
	case "serve":
		return runServe(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, "unknown command")
		return 2
	}
}

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var controllerStateDir, receiverID, controllerID, sourceRoot, sourceIngesterID, brokerSocket, gitExecutable string
	var appIDText string
	fs.StringVar(&controllerStateDir, "controller-state-dir", "", "absolute controller state directory")
	fs.StringVar(&receiverID, "receiver-id", "", "receiver id")
	fs.StringVar(&controllerID, "controller-id", "", "controller id")
	fs.StringVar(&appIDText, "app-id", "", "positive GitHub App ID")
	fs.StringVar(&sourceRoot, "source-root", "", "absolute source root")
	fs.StringVar(&sourceIngesterID, "source-ingester-id", "", "source ingester id")
	fs.StringVar(&brokerSocket, "credential-broker-unix", "", "absolute credential broker Unix socket")
	fs.StringVar(&gitExecutable, "git-executable", "", "absolute Git executable")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: glassroot-source-ingester serve --controller-state-dir ABSOLUTE_DIRECTORY --receiver-id RECEIVER_ID --controller-id CONTROLLER_ID --app-id POSITIVE_INTEGER --source-root ABSOLUTE_DIRECTORY --source-ingester-id SOURCE_INGESTER_ID --credential-broker-unix ABSOLUTE_SOCKET_PATH --git-executable ABSOLUTE_GIT_PATH")
	}
	if len(args) == 1 && args[0] == "--help" {
		fs.SetOutput(stdout)
		fs.Usage()
		return 0
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		fmt.Fprintln(stderr, "unknown flag")
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "extra positional arguments rejected")
		return 2
	}
	if err := validateRequired(controllerStateDir, receiverID, controllerID, appIDText, sourceRoot, sourceIngesterID, brokerSocket, gitExecutable); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 2
	}
	appID, _ := strconv.ParseInt(appIDText, 10, 64)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := githubcontrollerstore.Open(ctx, githubcontrollerstore.Config{StateDir: controllerStateDir, ReceiverID: receiverID, ControllerID: controllerID, AppID: appID})
	if err != nil {
		fmt.Fprintln(stderr, "controller store unavailable")
		return 1
	}
	defer store.Close()
	client, err := githubbroker.Dial(ctx, brokerSocket, githubbroker.DefaultLimits())
	if err != nil {
		fmt.Fprintln(stderr, "credential broker unavailable")
		return 1
	}
	_ = sourceRoot
	_ = gitExecutable
	svc, err := githubsource.New(githubsource.Config{SourceIngesterID: sourceIngesterID, Store: store, Broker: client, Importer: githubsource.NewGitImporter(sourceRoot, gitExecutable, githubsource.DefaultLimits()), Clock: realClock{}})
	if err != nil {
		fmt.Fprintln(stderr, "source ingester configuration rejected")
		return 1
	}
	for {
		select {
		case <-ctx.Done():
			return 0
		default:
		}
		_, _, err := svc.ProcessNext(ctx)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func validateRequired(controllerStateDir, receiverID, controllerID, appIDText, sourceRoot, sourceIngesterID, brokerSocket, gitExecutable string) error {
	for _, p := range []string{controllerStateDir, sourceRoot, brokerSocket, gitExecutable} {
		if p == "" || !filepath.IsAbs(p) || filepath.Clean(p) != p || !utf8.ValidString(p) || hasControl(p) {
			return fmt.Errorf("required absolute path rejected")
		}
	}
	if !validID(receiverID) || !validID(controllerID) || !validID(sourceIngesterID) {
		return fmt.Errorf("identifier rejected")
	}
	appID, err := strconv.ParseInt(appIDText, 10, 64)
	if err != nil || appID <= 0 {
		return fmt.Errorf("app id rejected")
	}
	return nil
}
func validID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i, r := range s {
		if i == 0 && (r < 'a' || r > 'z') {
			return false
		}
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
