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

	FontPath            string
	Corpus              string
	FaceIndex           int
	PPEMList            string
	GlyphList           string
	CharList            string
	LoadFlagList        string
	RenderModeList      string
	IncludeBitmapBuffer bool
	visited             map[string]bool
}

type batchCompareOptions struct {
	Requests        []string
	ReferenceDir    string
	CandidateDir    string
	ReferenceSuffix string
	CandidateSuffix string
	Compare         compareOptions
}

type batchCompareSummary struct {
	Requests            int
	Clean               int
	ExpectedGap         int
	Mismatched          int
	ReadErrors          int
	Diffs               int
	ExpectedDiffs       int
	UnexpectedDiffs     int
	StaleExpectedGaps   int
	ExpectedKinds       map[string]int
	UnexpectedKinds     map[string]int
	UnexpectedSizeKeys  map[string]int
	UnexpectedLoadFlags map[string]int
	RenderDetails       map[string]int
	MetricDetails       map[string]metricDeltaStats
	OutlinePointTags    outlinePointTagSummary
	StaleKinds          map[string]int
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

	summary := newBatchCompareSummary(len(opts.Requests))
	for i, requestPath := range opts.Requests {
		refPath := batchOutputPath(opts.ReferenceDir, requestPath, opts.ReferenceSuffix)
		candPath := batchOutputPath(opts.CandidateDir, requestPath, opts.CandidateSuffix)
		fmt.Fprintf(stdout, "[%d/%d] %s\n", i+1, len(opts.Requests), requestPath)

		request, err := readDumpRequest(requestPath)
		if err != nil {
			summary.ReadErrors++
			fmt.Fprintf(stdout, "  FAIL: read request %s: %v\n", requestPath, err)
			continue
		}
		compareOpts := opts.Compare
		compareOpts.ExpectedGaps = append(append([]ExpectedGap(nil), opts.Compare.ExpectedGaps...), request.ExpectedGaps...)

		reference, err := readDump(refPath)
		if err != nil {
			summary.ReadErrors++
			fmt.Fprintf(stdout, "  FAIL: read reference %s: %v\n", refPath, err)
			continue
		}
		candidate, err := readDump(candPath)
		if err != nil {
			summary.ReadErrors++
			fmt.Fprintf(stdout, "  FAIL: read candidate %s: %v\n", candPath, err)
			continue
		}

		diffs := compareDumps(reference, candidate, compareOpts)
		gaps := expectedGapsForCompare(reference, candidate, compareOpts)
		stale := staleExpectedGaps(expectedGapUsage(diffs, gaps))
		summary.StaleExpectedGaps += len(stale)
		addBatchStaleKinds(summary, stale)
		if len(diffs) == 0 {
			summary.Clean++
			fmt.Fprintln(stdout, "  PASS")
			printStaleExpectedGaps(stdout, stale, "  ", compareOpts.MaxDiffs)
			continue
		}
		expected, unexpected := splitExpectedDiffs(diffs, gaps)
		addBatchDiffSummary(summary, diffs, expected, unexpected)
		if len(unexpected) == 0 && compareOpts.AcceptExpectedGaps {
			summary.ExpectedGap++
			fmt.Fprintf(stdout, "  PASS with expected gaps (%d diffs)\n", len(expected))
			printExpectedGapSummary(stdout, expected, "  ")
			printRenderMismatchSummary(stdout, expected, "  ")
			printMetricDeltaSummary(stdout, expected, "  ")
			printOutlinePointTagSummary(stdout, expected, "  ")
			printDiffList(stdout, expected, compareOpts.MaxDiffs, "  ")
			printStaleExpectedGaps(stdout, stale, "  ", compareOpts.MaxDiffs)
			continue
		}
		summary.Mismatched++
		printCompareFailure(stdout, "", diffs, expected, unexpected, compareOpts, "  ")
		printStaleExpectedGaps(stdout, stale, "  ", compareOpts.MaxDiffs)
	}

	printBatchCompareSummary(stdout, summary)

	switch {
	case summary.ReadErrors > 0:
		fmt.Fprintf(stdout, "conformance batch compare: FAIL (%d requests: %d passed, %d expected-gap, %d mismatched, %d read errors)\n", len(opts.Requests), summary.Clean, summary.ExpectedGap, summary.Mismatched, summary.ReadErrors)
		return fmt.Errorf("conformance batch compare failed: %d read errors, %d mismatched requests", summary.ReadErrors, summary.Mismatched)
	case summary.Mismatched > 0:
		fmt.Fprintf(stdout, "conformance batch compare: FAIL (%d requests: %d passed, %d expected-gap, %d mismatched)\n", len(opts.Requests), summary.Clean, summary.ExpectedGap, summary.Mismatched)
		return errComparisonFailed
	case summary.StaleExpectedGaps > 0 && opts.Compare.FailStaleExpectedGaps:
		fmt.Fprintf(stdout, "conformance batch compare: FAIL (%d requests: %d stale expected gaps)\n", len(opts.Requests), summary.StaleExpectedGaps)
		return errComparisonFailed
	default:
		if summary.ExpectedGap > 0 || summary.StaleExpectedGaps > 0 {
			fmt.Fprintf(stdout, "conformance batch compare: PASS (%d requests: %d clean, %d expected-gap, %d stale expected gaps)\n", len(opts.Requests), summary.Clean, summary.ExpectedGap, summary.StaleExpectedGaps)
			return nil
		}
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
	fs.BoolVar(&opts.IncludeBitmapBuffer, "include-bitmap-buffer", false, "include raw bitmap buffers as hex for byte-level comparison diagnostics")

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
	fs.BoolVar(&opts.Compare.AcceptExpectedGaps, "accept-expected-gaps", false, "treat request/dump expected_gaps annotations as passable known gaps")
	fs.BoolVar(&opts.Compare.FailStaleExpectedGaps, "fail-stale-expected-gaps", false, "fail when an expected_gaps entry matches no current diff")
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

func newBatchCompareSummary(requests int) *batchCompareSummary {
	return &batchCompareSummary{
		Requests:            requests,
		ExpectedKinds:       make(map[string]int),
		UnexpectedKinds:     make(map[string]int),
		UnexpectedSizeKeys:  make(map[string]int),
		UnexpectedLoadFlags: make(map[string]int),
		RenderDetails:       make(map[string]int),
		MetricDetails:       make(map[string]metricDeltaStats),
		OutlinePointTags:    newOutlinePointTagSummary(),
		StaleKinds:          make(map[string]int),
	}
}

func addBatchDiffSummary(summary *batchCompareSummary, diffs, expected, unexpected []comparisonDiff) {
	summary.Diffs += len(diffs)
	summary.ExpectedDiffs += len(expected)
	summary.UnexpectedDiffs += len(unexpected)
	addDiffKindCounts(summary.ExpectedKinds, expected)
	addDiffKindCounts(summary.UnexpectedKinds, unexpected)
	addDiffSizeKeyCounts(summary.UnexpectedSizeKeys, unexpected)
	addDiffLoadFlagCounts(summary.UnexpectedLoadFlags, unexpected)
	for _, diff := range diffs {
		detail := renderMismatchDetail(diff.Path)
		if detail != "" && diff.Kind != "" {
			summary.RenderDetails[detail]++
		}
	}
	addMetricDeltaStats(summary.MetricDetails, diffs)
	addOutlinePointTagStats(&summary.OutlinePointTags, diffs)
}

func addDiffKindCounts(counts map[string]int, diffs []comparisonDiff) {
	for _, diff := range diffs {
		kind := diff.Kind
		if kind == "" {
			kind = "unclassified"
		}
		counts[kind]++
	}
}

func addDiffSizeKeyCounts(counts map[string]int, diffs []comparisonDiff) {
	for _, diff := range diffs {
		key := diffSizeKey(diff.Path)
		if key == "" {
			continue
		}
		counts[key]++
	}
}

func addDiffLoadFlagCounts(counts map[string]int, diffs []comparisonDiff) {
	for _, diff := range diffs {
		loadFlags := diffLoadFlags(diff.Path)
		if loadFlags == "" {
			continue
		}
		counts[loadFlags]++
	}
}

func addBatchStaleKinds(summary *batchCompareSummary, gaps []ExpectedGap) {
	for _, gap := range gaps {
		kind := strings.TrimSpace(gap.Kind)
		if kind == "" {
			kind = "path-only expected gap"
		}
		summary.StaleKinds[kind]++
	}
}

func printBatchCompareSummary(w io.Writer, summary *batchCompareSummary) {
	fmt.Fprintf(w, "conformance batch summary: %d requests, %d clean, %d expected-gap, %d mismatched, %d read errors, %d diffs (%d expected, %d unexpected), %d stale expected gaps\n",
		summary.Requests,
		summary.Clean,
		summary.ExpectedGap,
		summary.Mismatched,
		summary.ReadErrors,
		summary.Diffs,
		summary.ExpectedDiffs,
		summary.UnexpectedDiffs,
		summary.StaleExpectedGaps,
	)
	printCountMap(w, "  expected gap diff kinds", summary.ExpectedKinds)
	printCountMap(w, "  unexpected diff kinds", summary.UnexpectedKinds)
	printCountMapByCount(w, "  unexpected diff size/load/render keys", summary.UnexpectedSizeKeys)
	printCountMapByCount(w, "  unexpected diff load flags", summary.UnexpectedLoadFlags)
	printCountMap(w, "  render mismatch details", summary.RenderDetails)
	printMetricDeltaStats(w, "metric delta details", summary.MetricDetails, "  ")
	printOutlinePointTagStats(w, "outline point/tag details", summary.OutlinePointTags, "  ")
	printCountMap(w, "  stale expected gap kinds", summary.StaleKinds)
}

func printCountMap(w io.Writer, label string, counts map[string]int) {
	if len(counts) == 0 {
		return
	}
	fmt.Fprintf(w, "%s:\n", label)
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(w, "  - %s: %d\n", key, counts[key])
	}
}

func printCountMapByCount(w io.Writer, label string, counts map[string]int) {
	if len(counts) == 0 {
		return
	}
	fmt.Fprintf(w, "%s:\n", label)
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] == counts[keys[j]] {
			return keys[i] < keys[j]
		}
		return counts[keys[i]] > counts[keys[j]]
	})
	for _, key := range keys {
		fmt.Fprintf(w, "  - %s: %d\n", key, counts[key])
	}
}

func diffSizeKey(path string) string {
	const prefix = "sizes["
	start := strings.Index(path, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.IndexByte(path[start:], ']')
	if end < 0 {
		return ""
	}
	return path[start : start+end]
}

func diffLoadFlags(path string) string {
	key := diffSizeKey(path)
	if key == "" {
		return ""
	}
	parts := strings.Split(key, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
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
	if opts.visited["include-bitmap-buffer"] {
		dumpArgs = append(dumpArgs, fmt.Sprintf("-include-bitmap-buffer=%t", opts.IncludeBitmapBuffer))
	}

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
