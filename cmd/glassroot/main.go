package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mattneel/glassroot/internal/config"
	"github.com/mattneel/glassroot/internal/demo"
	"github.com/mattneel/glassroot/internal/inspect"
	"github.com/mattneel/glassroot/internal/model"
	"github.com/mattneel/glassroot/internal/report"
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
	return runWithContext(context.Background(), args, stdout, stderr)
}

func runWithContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		printVersion(stdout)
		return 0
	}
	if len(args) >= 1 && args[0] == "validate" {
		return runValidate(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "inspect" {
		return runInspect(ctx, args[1:], stdout, stderr)
	}
	if len(args) >= 2 && args[0] == "demo" && args[1] == "fake" {
		return runDemoFake(ctx, args[2:], stdout, stderr)
	}

	printUsage(stderr)
	return 2
}

func runDemoFake(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	parsed, err := demo.ParseCLIArguments(args)
	if err != nil {
		writeDemoDiagnostic(stderr, err, true)
		return 2
	}
	if parsed.Help {
		printDemoFakeUsage(stdout)
		return 0
	}
	d, err := demo.New(demo.DefaultLimits())
	if err != nil {
		writeDemoDiagnostic(stderr, err, false)
		return 3
	}
	result, err := d.Create(ctx, parsed.Request)
	if err != nil {
		writeDemoDiagnostic(stderr, err, demo.IsUsageError(err))
		if demo.IsUsageError(err) {
			return 2
		}
		return 3
	}
	exitCode, err := inspectDispositionExitCode(result.EffectiveDisposition)
	if err != nil {
		writeDemoDiagnostic(stderr, err, false)
		return 3
	}
	out, err := renderDemoOutput(ctx, result.Report, parsed.Format)
	if err != nil {
		writeDemoDiagnostic(stderr, err, false)
		return 3
	}
	if err := writeAll(stdout, out); err != nil {
		writeDemoDiagnostic(stderr, &demo.Error{Code: demo.CodeOutputFailed, Stage: "output", Message: "stdout write failed", Err: err}, false)
		return 3
	}
	return exitCode
}

func renderDemoOutput(ctx context.Context, fr *report.FrozenReport, format string) ([]byte, error) {
	if fr == nil {
		return nil, &demo.Error{Code: demo.CodeReportRenderFailed, Stage: "render", Message: "missing report"}
	}
	switch format {
	case "terminal":
		out, err := report.RenderTerminal(ctx, fr, report.DefaultRenderLimits())
		if err != nil {
			return nil, &demo.Error{Code: demo.CodeReportRenderFailed, Stage: "render", Message: "terminal rendering failed", Err: err}
		}
		return append([]byte(nil), out.Bytes...), nil
	case "markdown":
		out, err := report.RenderMarkdown(ctx, fr, report.DefaultRenderLimits())
		if err != nil {
			return nil, &demo.Error{Code: demo.CodeReportRenderFailed, Stage: "render", Message: "markdown rendering failed", Err: err}
		}
		return append([]byte(nil), out.Bytes...), nil
	case "json":
		return fr.JSON(), nil
	default:
		return nil, &demo.Error{Code: demo.CodeInvalidRequest, Stage: "render", Message: "invalid output format", Usage: true}
	}
}

func writeDemoDiagnostic(w io.Writer, err error, includeUsage bool) {
	_, _ = fmt.Fprintf(w, "glassroot demo fake: %s\n", demo.Diagnostic(err))
	if includeUsage {
		printDemoFakeUsage(w)
	}
}

func runInspect(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	parsed, err := inspect.ParseCLIArguments(args)
	if err != nil {
		writeInspectDiagnostic(stderr, err, true)
		return 2
	}
	if parsed.Help {
		printInspectUsage(stdout)
		return 0
	}
	inspector, err := inspect.New(inspect.DefaultLimits())
	if err != nil {
		writeInspectDiagnostic(stderr, err, false)
		return 3
	}
	result, err := inspector.Inspect(ctx, parsed.Request)
	if err != nil {
		writeInspectDiagnostic(stderr, err, false)
		if inspect.IsUsageError(err) {
			return 2
		}
		return 3
	}
	exitCode, err := inspectDispositionExitCode(result.OverallDisposition)
	if err != nil {
		writeInspectDiagnostic(stderr, err, false)
		return 3
	}
	out, err := renderInspectOutput(ctx, result, parsed.Format)
	if err != nil {
		writeInspectDiagnostic(stderr, err, false)
		return 3
	}
	if err := writeAll(stdout, out); err != nil {
		writeInspectDiagnostic(stderr, &inspect.Error{Code: inspect.CodeOutputFailed, Stage: "output", Message: "stdout write failed", Err: err}, false)
		return 3
	}
	return exitCode
}

func inspectDispositionExitCode(disposition model.Disposition) (int, error) {
	switch disposition {
	case model.DispositionPassed:
		return 0, nil
	case model.DispositionRequiresReview:
		return 4, nil
	case model.DispositionFailed:
		return 5, nil
	default:
		return 3, &inspect.Error{Code: inspect.CodeRenderFailed, Stage: "output", Message: "unknown effective disposition"}
	}
}

func renderInspectOutput(ctx context.Context, result *inspect.Result, format string) ([]byte, error) {
	if result == nil || result.Report == nil {
		return nil, &inspect.Error{Code: inspect.CodeRenderFailed, Stage: "render", Message: "missing report"}
	}
	switch format {
	case "terminal":
		out, err := report.RenderTerminal(ctx, result.Report, report.DefaultRenderLimits())
		if err != nil {
			return nil, &inspect.Error{Code: inspect.CodeRenderFailed, Stage: "render", Message: "terminal rendering failed", Err: err}
		}
		return append([]byte(nil), out.Bytes...), nil
	case "markdown":
		out, err := report.RenderMarkdown(ctx, result.Report, report.DefaultRenderLimits())
		if err != nil {
			return nil, &inspect.Error{Code: inspect.CodeRenderFailed, Stage: "render", Message: "markdown rendering failed", Err: err}
		}
		return append([]byte(nil), out.Bytes...), nil
	case "json":
		return result.Report.JSON(), nil
	default:
		return nil, &inspect.Error{Code: inspect.CodeInvalidRequest, Stage: "render", Message: "invalid output format", Usage: true}
	}
}

func writeAll(w io.Writer, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	n, err := w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func writeInspectDiagnostic(w io.Writer, err error, includeUsage bool) {
	_, _ = fmt.Fprintf(w, "glassroot inspect: %s\n", inspect.Diagnostic(err))
	if includeUsage {
		printInspectUsage(w)
	}
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
	fmt.Fprintln(w, "       glassroot inspect [flags] <absolute-evidence-directory>")
	fmt.Fprintln(w, "       glassroot demo fake [flags] <absolute-new-output-directory>")
}

func printInspectUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: glassroot inspect [flags] <absolute-evidence-directory>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "required flags:")
	fmt.Fprintln(w, "  --git-dir ABSOLUTE_PATH")
	fmt.Fprintln(w, "  --base-commit FULL_OBJECT_ID")
	fmt.Fprintln(w, "  --head-commit FULL_OBJECT_ID")
	fmt.Fprintln(w, "  --evaluated-at YYYY-MM-DDTHH:MM:SSZ")
	fmt.Fprintln(w, "  exactly one of:")
	fmt.Fprintln(w, "    --expected-manifest-digest sha256:<64-lowercase-hex>")
	fmt.Fprintln(w, "    --allow-internal-consistency-only")
	fmt.Fprintln(w, "optional flags:")
	fmt.Fprintln(w, "  --format terminal|markdown|json   default: terminal")
}

func printDemoFakeUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: glassroot demo fake [flags] <absolute-new-output-directory>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "optional flags:")
	fmt.Fprintln(w, "  --fixture behavior-change|control   default: behavior-change")
	fmt.Fprintln(w, "  --format terminal|markdown|json      default: terminal")
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
