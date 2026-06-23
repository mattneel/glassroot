package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mattneel/glassroot/internal/config"
)

var (
	version = "dev"
	commit  = "unknown"
	built   = "unknown"
)

var readConfigFile = readBoundedRegularFile

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		printVersion(stdout)
		return 0
	}
	if len(args) >= 1 && args[0] == "validate" {
		return runValidate(args[1:], stdout, stderr)
	}

	printUsage(stderr)
	return 2
}

func runValidate(args []string, stdout, stderr io.Writer) int {
	path := ".glassroot/pipeline.yaml"
	switch len(args) {
	case 0:
	case 2:
		if args[0] != "--file" || args[1] == "" {
			printUsage(stderr)
			return 2
		}
		path = args[1]
	default:
		printUsage(stderr)
		return 2
	}

	data, err := readConfigFile(path)
	if err != nil {
		if errors.Is(err, errMissingConfig) || errors.Is(err, errInvalidConfigFile) {
			fmt.Fprintf(stderr, "%s: %s\n", err, path)
			return 2
		}
		fmt.Fprintf(stderr, "unexpected I/O while reading %s: %v\n", path, err)
		return 3
	}
	if _, err := config.ParseAndValidate(data); err != nil {
		writeDiagnostics(stderr, path, err)
		return 2
	}
	fmt.Fprintf(stdout, "valid: %s\n", path)
	return 0
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "glassroot %s\n", version)
	fmt.Fprintf(w, "commit: %s\n", commit)
	fmt.Fprintf(w, "built: %s\n", built)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: glassroot version")
	fmt.Fprintln(w, "       glassroot validate [--file PATH]")
}

func writeDiagnostics(w io.Writer, file string, err error) {
	var diags config.Diagnostics
	if errors.As(err, &diags) {
		for _, diag := range diags {
			fmt.Fprintf(w, "%s", file)
			if diag.Line > 0 {
				fmt.Fprintf(w, ":%d", diag.Line)
				if diag.Column > 0 {
					fmt.Fprintf(w, ":%d", diag.Column)
				}
			}
			fmt.Fprintf(w, ": %s", diag.Code)
			if diag.Path != "" {
				fmt.Fprintf(w, ": %s", diag.Path)
			}
			if diag.Message != "" {
				fmt.Fprintf(w, ": %s", diag.Message)
			}
			fmt.Fprintln(w)
		}
		return
	}
	fmt.Fprintf(w, "%s: %v\n", file, err)
}

type configFileError string

func (e configFileError) Error() string { return string(e) }

const (
	errMissingConfig     configFileError = "missing configuration"
	errInvalidConfigFile configFileError = "invalid configuration file"
)

func readBoundedRegularFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errMissingConfig
		}
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > config.MaxPipelineBytes {
		return nil, errInvalidConfigFile
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, config.MaxPipelineBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > config.MaxPipelineBytes {
		return nil, errInvalidConfigFile
	}
	return data, nil
}
