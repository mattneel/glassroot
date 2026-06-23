package main

import (
	"fmt"
	"io"
	"os"
)

var (
	version = "dev"
	commit  = "unknown"
	built   = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		printVersion(stdout)
		return 0
	}

	fmt.Fprintln(stderr, "usage: glassroot version")
	return 2
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "glassroot %s\n", version)
	fmt.Fprintf(w, "commit: %s\n", commit)
	fmt.Fprintf(w, "built: %s\n", built)
}
