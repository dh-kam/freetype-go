package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}

	switch args[0] {
	case "-h", "--help", "help":
		printUsage(stdout)
		return nil
	case "dump":
		return runGoDumpCommand(args[1:], stdout, stderr)
	case "ftdump":
		return runFreeTypeDumpCommand(args[1:], stdout, stderr)
	case "compare":
		return runCompareCommand(args[1:], stdout, stderr)
	case "batch":
		return runBatchCommand(args[1:], stdout, stderr)
	case "batch-compare":
		return runBatchCompareCommand(args[1:], stdout, stderr)
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "FreeType-Go conformance harness")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  go run ./tools/conformance dump -font font.ttf [flags]")
	fmt.Fprintln(w, "  CGO_ENABLED=1 go run -tags freetype_conformance ./tools/conformance ftdump -font font.ttf [flags]")
	fmt.Fprintln(w, "  go run ./tools/conformance compare -reference freetype.json -candidate go.json [flags]")
	fmt.Fprintln(w, "  go run ./tools/conformance dump -request request.json")
	fmt.Fprintln(w, "  go run ./tools/conformance batch -requests 'testdata/conformance/*.json' -font font.ttf")
	fmt.Fprintln(w, "  go run ./tools/conformance batch-compare -requests 'testdata/conformance/*.json'")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  dump     Write a JSON metrics/outline/bitmap dump from the Go engine.")
	fmt.Fprintln(w, "  ftdump   Write the same JSON dump from the C FreeType library when built with cgo.")
	fmt.Fprintln(w, "  compare  Compare two JSON dumps field-by-field.")
	fmt.Fprintln(w, "  batch    Run dump or ftdump over multiple request JSON files.")
	fmt.Fprintln(w, "  batch-compare")
	fmt.Fprintln(w, "           Compare per-request reference and candidate dump files.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Common dump flags:")
	fmt.Fprintln(w, "  -load-flags  comma-separated sets such as default,no-hinting+target-light")
	fmt.Fprintln(w, "  -render-mode comma-separated modes: none, normal, light, mono, lcd, lcd-v")
	fmt.Fprintln(w, "  -request     JSON request file; explicit CLI flags override request fields")
}

func isHelp(err error) bool {
	return errors.Is(err, flag.ErrHelp)
}
