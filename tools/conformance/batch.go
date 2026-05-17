package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type batchOptions struct {
	Engine       string
	Requests     []string
	OutputDir    string
	OutputSuffix string

	FontPath       string
	Corpus         string
	FaceIndex      int
	PPEMList       string
	GlyphList      string
	CharList       string
	LoadFlagList   string
	RenderModeList string
	visited        map[string]bool
}

type batchCompareOptions struct {
	Requests        []string
	ReferenceDir    string
	CandidateDir    string
	ReferenceSuffix string
	CandidateSuffix string
	Compare         compareOptions
}

type stringValues []string

func (v *stringValues) String() string {
	return strings.Join(*v, ",")
}

func (v *stringValues) Set(raw string) error {
	for _, part := range splitRequestList(raw) {
		*v = append(*v, part)
	}
	return nil
}

func runBatchCommand(args []string, stdout, stderr io.Writer) error {
	opts, err := parseBatchOptions(args, stdout)
	if err != nil {
		if isHelp(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return err
	}

	failures := 0
	for i, requestPath := range opts.Requests {
		outputPath := batchOutputPath(opts.OutputDir, requestPath, opts.OutputSuffix)
		fmt.Fprintf(stdout, "[%d/%d] %s -> %s\n", i+1, len(opts.Requests), requestPath, outputPath)

		dump, err := buildBatchDump(opts, requestPath, outputPath, stderr)
		if err == nil {
			err = writeDump(dump, outputPath, stdout)
		}
		if err != nil {
			failures++
			fmt.Fprintf(stdout, "  FAIL: %v\n", err)
			continue
		}
		fmt.Fprintln(stdout, "  PASS")
	}

	if failures > 0 {
		return fmt.Errorf("conformance batch failed: %d of %d requests", failures, len(opts.Requests))
	}
	return nil
}

func runBatchCompareCommand(args []string, stdout, stderr io.Writer) error {
	opts, err := parseBatchCompareOptions(args, stdout)
	if err != nil {
		if isHelp(err) {
			return nil
		}
		return err
	}

	diffFailures := 0
	readFailures := 0
	for i, requestPath := range opts.Requests {
		refPath := batchOutputPath(opts.ReferenceDir, requestPath, opts.ReferenceSuffix)
		candPath := batchOutputPath(opts.CandidateDir, requestPath, opts.CandidateSuffix)
		fmt.Fprintf(stdout, "[%d/%d] %s\n", i+1, len(opts.Requests), requestPath)

		reference, err := readDump(refPath)
		if err != nil {
			readFailures++
			fmt.Fprintf(stdout, "  FAIL: read reference %s: %v\n", refPath, err)
			continue
		}
		candidate, err := readDump(candPath)
		if err != nil {
			readFailures++
			fmt.Fprintf(stdout, "  FAIL: read candidate %s: %v\n", candPath, err)
			continue
		}

		diffs := compareDumps(reference, candidate, opts.Compare)
		if len(diffs) == 0 {
			fmt.Fprintln(stdout, "  PASS")
			continue
		}
		diffFailures++
		fmt.Fprintf(stdout, "  FAIL (%d diffs)\n", len(diffs))
		printKnownGapSummary(stdout, diffs, "  ")
		printDiffList(stdout, diffs, opts.Compare.MaxDiffs, "  ")
	}

	switch {
	case readFailures > 0:
		return fmt.Errorf("conformance batch compare failed: %d read errors, %d mismatched requests", readFailures, diffFailures)
	case diffFailures > 0:
		return errComparisonFailed
	default:
		fmt.Fprintf(stdout, "conformance batch compare: PASS (%d requests)\n", len(opts.Requests))
		return nil
	}
}

func parseBatchOptions(args []string, output io.Writer) (batchOptions, error) {
	var requestInputs stringValues
	opts := batchOptions{
		Engine:    "go",
		OutputDir: "conformance-out",
	}

	fs := flag.NewFlagSet("batch", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Var(&requestInputs, "request", "request JSON path, glob, or comma-separated list; repeatable")
	fs.Var(&requestInputs, "requests", "request JSON path, glob, or comma-separated list; repeatable")
	fs.StringVar(&opts.Engine, "engine", opts.Engine, "dump engine: go, freetype, or ftdump")
	fs.StringVar(&opts.OutputDir, "out-dir", opts.OutputDir, "directory for per-request JSON dumps")
	fs.StringVar(&opts.OutputSuffix, "suffix", "", "output filename suffix; defaults to .go.json or .freetype.json")
	fs.StringVar(&opts.FontPath, "font", "", "optional font override for every request")
	fs.StringVar(&opts.Corpus, "corpus", "", "optional corpus label override for every request")
	fs.IntVar(&opts.FaceIndex, "face", 0, "optional face index override for every request")
	fs.StringVar(&opts.PPEMList, "ppem", "", "optional ppem override for every request")
	fs.StringVar(&opts.GlyphList, "glyphs", "", "optional glyph override for every request")
	fs.StringVar(&opts.CharList, "chars", "", "optional char override for every request")
	fs.StringVar(&opts.LoadFlagList, "load-flags", "", "optional load flag override for every request")
	fs.StringVar(&opts.RenderModeList, "render-mode", "", "optional render mode override for every request")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	opts.visited = visitedFlags(fs)

	engine, err := canonicalBatchEngine(opts.Engine)
	if err != nil {
		return opts, err
	}
	opts.Engine = engine
	if opts.OutputSuffix == "" {
		opts.OutputSuffix = batchEngineSuffix(opts.Engine)
	}
	if opts.OutputDir == "" {
		return opts, errors.New("missing required -out-dir")
	}

	requests, err := expandRequestPaths([]string(requestInputs))
	if err != nil {
		return opts, err
	}
	if len(requests) == 0 {
		return opts, errors.New("missing required -request or -requests")
	}
	opts.Requests = requests
	return opts, nil
}

func parseBatchCompareOptions(args []string, output io.Writer) (batchCompareOptions, error) {
	var requestInputs stringValues
	opts := batchCompareOptions{
		ReferenceDir:    "conformance-out",
		CandidateDir:    "conformance-out",
		ReferenceSuffix: ".freetype.json",
		CandidateSuffix: ".go.json",
		Compare:         compareOptions{MaxDiffs: 50},
	}

	fs := flag.NewFlagSet("batch-compare", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Var(&requestInputs, "request", "request JSON path, glob, or comma-separated list; repeatable")
	fs.Var(&requestInputs, "requests", "request JSON path, glob, or comma-separated list; repeatable")
	fs.StringVar(&opts.ReferenceDir, "reference-dir", opts.ReferenceDir, "directory containing reference dumps")
	fs.StringVar(&opts.CandidateDir, "candidate-dir", opts.CandidateDir, "directory containing candidate dumps")
	fs.StringVar(&opts.ReferenceSuffix, "reference-suffix", opts.ReferenceSuffix, "reference dump filename suffix")
	fs.StringVar(&opts.CandidateSuffix, "candidate-suffix", opts.CandidateSuffix, "candidate dump filename suffix")
	fs.Int64Var(&opts.Compare.MetricTolerance, "metric-tolerance", 0, "allowed absolute 26.6 metric delta")
	fs.Int64Var(&opts.Compare.PointTolerance, "point-tolerance", 0, "allowed absolute 26.6 outline point delta")
	fs.BoolVar(&opts.Compare.AllowMissingBitmap, "allow-missing-bitmap", false, "do not fail when only one dump has bitmap data")
	fs.BoolVar(&opts.Compare.AllowMissingSlotMetrics, "allow-missing-slot-metrics", false, "do not fail when only one dump has rich glyph slot metrics")
	fs.IntVar(&opts.Compare.MaxDiffs, "max-diffs", 50, "maximum diffs to print per request")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.ReferenceDir == "" {
		return opts, errors.New("missing required -reference-dir")
	}
	if opts.CandidateDir == "" {
		return opts, errors.New("missing required -candidate-dir")
	}
	if opts.ReferenceSuffix == "" || opts.CandidateSuffix == "" {
		return opts, errors.New("dump suffixes must be non-empty")
	}
	if opts.Compare.MetricTolerance < 0 || opts.Compare.PointTolerance < 0 {
		return opts, errors.New("tolerances must be non-negative")
	}

	requests, err := expandRequestPaths([]string(requestInputs))
	if err != nil {
		return opts, err
	}
	if len(requests) == 0 {
		return opts, errors.New("missing required -request or -requests")
	}
	opts.Requests = requests
	return opts, nil
}

func buildBatchDump(opts batchOptions, requestPath, outputPath string, output io.Writer) (*Dump, error) {
	dumpArgs := []string{"-request", requestPath, "-out", outputPath}
	dumpArgs = appendBatchDumpOverride(dumpArgs, opts, "font", opts.FontPath)
	dumpArgs = appendBatchDumpOverride(dumpArgs, opts, "corpus", opts.Corpus)
	if opts.visited["face"] {
		dumpArgs = append(dumpArgs, "-face", fmt.Sprint(opts.FaceIndex))
	}
	dumpArgs = appendBatchDumpOverride(dumpArgs, opts, "ppem", opts.PPEMList)
	dumpArgs = appendBatchDumpOverride(dumpArgs, opts, "glyphs", opts.GlyphList)
	dumpArgs = appendBatchDumpOverride(dumpArgs, opts, "chars", opts.CharList)
	dumpArgs = appendBatchDumpOverride(dumpArgs, opts, "load-flags", opts.LoadFlagList)
	dumpArgs = appendBatchDumpOverride(dumpArgs, opts, "render-mode", opts.RenderModeList)

	parsed, err := parseDumpOptions(opts.Engine, dumpArgs, output)
	if err != nil {
		return nil, err
	}
	switch opts.Engine {
	case "go":
		return buildGoDump(parsed)
	case "freetype":
		return buildFreeTypeDump(parsed)
	default:
		return nil, fmt.Errorf("unsupported batch engine %q", opts.Engine)
	}
}

func appendBatchDumpOverride(args []string, opts batchOptions, name, value string) []string {
	if !opts.visited[name] {
		return args
	}
	return append(args, "-"+name, value)
}

func canonicalBatchEngine(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "go":
		return "go", nil
	case "ft", "ftdump", "freetype":
		return "freetype", nil
	default:
		return "", fmt.Errorf("unsupported batch engine %q", raw)
	}
}

func batchEngineSuffix(engine string) string {
	if engine == "freetype" {
		return ".freetype.json"
	}
	return ".go.json"
}

func expandRequestPaths(inputs []string) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	for _, input := range inputs {
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		matches := []string{input}
		if hasGlobMeta(input) {
			globMatches, err := filepath.Glob(input)
			if err != nil {
				return nil, err
			}
			if len(globMatches) == 0 {
				return nil, fmt.Errorf("request glob %q matched no files", input)
			}
			sort.Strings(globMatches)
			matches = globMatches
		}
		for _, match := range matches {
			if seen[match] {
				continue
			}
			seen[match] = true
			out = append(out, match)
		}
	}
	return out, nil
}

func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func batchOutputPath(outputDir, requestPath, suffix string) string {
	base := filepath.Base(requestPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return filepath.Join(outputDir, name+suffix)
}
