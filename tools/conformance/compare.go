package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/dh-kam/freetype-go/type1"
)

var errComparisonFailed = errors.New("conformance comparison failed")

type compareOptions struct {
	ReferencePath           string
	CandidatePath           string
	MetricTolerance         int64
	PointTolerance          int64
	AllowMissingBitmap      bool
	AllowMissingSlotMetrics bool
	AcceptExpectedGaps      bool
	FailStaleExpectedGaps   bool
	MaxDiffs                int
	ExpectedGaps            []ExpectedGap
}

type comparisonDiff struct {
	Path string
	Want string
	Got  string
	Kind string
}

func runCompareCommand(args []string, stdout, stderr io.Writer) error {
	opts, err := parseCompareOptions(args, stdout)
	if err != nil {
		if isHelp(err) {
			return nil
		}
		return err
	}

	reference, err := readDump(opts.ReferencePath)
	if err != nil {
		return fmt.Errorf("read reference: %w", err)
	}
	candidate, err := readDump(opts.CandidatePath)
	if err != nil {
		return fmt.Errorf("read candidate: %w", err)
	}

	diffs := compareDumps(reference, candidate, opts)
	gaps := expectedGapsForCompare(reference, candidate, opts)
	stale := staleExpectedGaps(expectedGapUsage(diffs, gaps))
	if len(diffs) == 0 {
		fmt.Fprintln(stdout, "conformance compare: PASS")
		printStaleExpectedGaps(stdout, stale, "", opts.MaxDiffs)
		if len(stale) > 0 && opts.FailStaleExpectedGaps {
			return errComparisonFailed
		}
		return nil
	}

	expected, unexpected := splitExpectedDiffs(diffs, gaps)
	if len(unexpected) == 0 && opts.AcceptExpectedGaps {
		fmt.Fprintf(stdout, "conformance compare: PASS with expected gaps (%d diffs)\n", len(expected))
		printExpectedGapSummary(stdout, expected, "")
		printRenderMismatchSummary(stdout, expected, "")
		printMetricDeltaSummary(stdout, expected, "")
		printOutlinePointTagSummary(stdout, expected, "")
		printDiffList(stdout, expected, opts.MaxDiffs, "")
		printStaleExpectedGaps(stdout, stale, "", opts.MaxDiffs)
		if len(stale) > 0 && opts.FailStaleExpectedGaps {
			return errComparisonFailed
		}
		return nil
	}

	printCompareFailure(stdout, "conformance compare", diffs, expected, unexpected, opts, "")
	printStaleExpectedGaps(stdout, stale, "", opts.MaxDiffs)
	return errComparisonFailed
}

func parseCompareOptions(args []string, output io.Writer) (compareOptions, error) {
	opts := compareOptions{MaxDiffs: 50}
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&opts.ReferencePath, "reference", "", "reference JSON dump path")
	fs.StringVar(&opts.CandidatePath, "candidate", "", "candidate JSON dump path")
	fs.Int64Var(&opts.MetricTolerance, "metric-tolerance", 0, "allowed absolute 26.6 metric delta")
	fs.Int64Var(&opts.PointTolerance, "point-tolerance", 0, "allowed absolute 26.6 outline point delta")
	fs.BoolVar(&opts.AllowMissingBitmap, "allow-missing-bitmap", false, "do not fail when only one dump has bitmap data")
	fs.BoolVar(&opts.AllowMissingSlotMetrics, "allow-missing-slot-metrics", false, "do not fail when only one dump has rich glyph slot metrics")
	fs.BoolVar(&opts.AcceptExpectedGaps, "accept-expected-gaps", false, "treat request/dump expected_gaps annotations as passable known gaps")
	fs.BoolVar(&opts.FailStaleExpectedGaps, "fail-stale-expected-gaps", false, "fail when an expected_gaps entry matches no current diff")
	fs.IntVar(&opts.MaxDiffs, "max-diffs", 50, "maximum diffs to print")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.ReferencePath == "" {
		return opts, errors.New("missing required -reference")
	}
	if opts.CandidatePath == "" {
		return opts, errors.New("missing required -candidate")
	}
	if opts.MetricTolerance < 0 || opts.PointTolerance < 0 {
		return opts, errors.New("tolerances must be non-negative")
	}
	return opts, nil
}

func readDump(path string) (*Dump, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var dump Dump
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&dump); err != nil {
		return nil, err
	}
	if dump.Schema != dumpSchema {
		return nil, fmt.Errorf("unsupported schema %q", dump.Schema)
	}
	return &dump, nil
}

func compareDumps(reference, candidate *Dump, opts compareOptions) []comparisonDiff {
	var diffs []comparisonDiff
	add := func(path, want, got string) {
		diffs = append(diffs, newComparisonDiff(path, want, got))
	}

	if reference.Source.SHA256 != "" && candidate.Source.SHA256 != "" && reference.Source.SHA256 != candidate.Source.SHA256 {
		add("source.sha256", reference.Source.SHA256, candidate.Source.SHA256)
	}
	if reference.Face.FaceIndex != candidate.Face.FaceIndex {
		add("face.face_index", fmt.Sprint(reference.Face.FaceIndex), fmt.Sprint(candidate.Face.FaceIndex))
	}
	if reference.Face.NumGlyphs != candidate.Face.NumGlyphs {
		add("face.num_glyphs", fmt.Sprint(reference.Face.NumGlyphs), fmt.Sprint(candidate.Face.NumGlyphs))
	}
	if reference.Face.UnitsPerEm != candidate.Face.UnitsPerEm {
		add("face.units_per_em", fmt.Sprint(reference.Face.UnitsPerEm), fmt.Sprint(candidate.Face.UnitsPerEm))
	}

	compareCharmaps(reference.Charmap, candidate.Charmap, add)

	candidateSizes := make(map[string]SizeDump, len(candidate.Sizes))
	for _, size := range candidate.Sizes {
		candidateSizes[sizeKey(size)] = size
	}
	for _, refSize := range reference.Sizes {
		path := "sizes[" + sizeKey(refSize) + "]"
		candSize, ok := candidateSizes[sizeKey(refSize)]
		if !ok {
			add(path, "present", "missing")
			continue
		}
		if refSize.Error != candSize.Error {
			add(path+".error", quoted(refSize.Error), quoted(candSize.Error))
		}
		compareGlyphs(path, refSize.Glyphs, candSize.Glyphs, opts, add)
	}

	return diffs
}

func newComparisonDiff(path, want, got string) comparisonDiff {
	return comparisonDiff{
		Path: path,
		Want: want,
		Got:  got,
		Kind: classifyComparisonDiff(path, want, got),
	}
}

func classifyComparisonDiff(path, want, got string) string {
	gotLower := strings.ToLower(got)
	switch {
	case strings.HasSuffix(path, ".render_error") && strings.Contains(gotLower, "unsupported"):
		return "go render unsupported"
	case strings.Contains(path, "/render-") && strings.Contains(path, ".bitmap.") && want == "true" && got == "false":
		return "go render bitmap missing"
	case strings.Contains(path, "/render-") && strings.Contains(path, ".bitmap.") && want != got:
		return "go render output mismatch"
	case strings.Contains(path, "/render-") && strings.HasSuffix(path, ".rendered_format") && want != got:
		return "go render output mismatch"
	case strings.Contains(path, ".slot_metrics") && (got == "missing" || got == "false" || strings.Contains(gotLower, "unavailable")):
		return "go slot metrics unavailable"
	case strings.Contains(path, ".type1_segments"):
		return "type1 segment delta"
	case isOutlineTagDeltaPath(path):
		return "outline tag delta"
	default:
		return metricDeltaKind(path)
	}
}

func printKnownGapSummary(w io.Writer, diffs []comparisonDiff, prefix string) {
	counts := make(map[string]int)
	for _, diff := range diffs {
		if diff.Kind != "" {
			counts[diff.Kind]++
		}
	}
	order := []string{
		"go render unsupported",
		"go render bitmap missing",
		"go render output mismatch",
		"go slot metrics unavailable",
	}
	printed := false
	for _, kind := range order {
		count := counts[kind]
		if count == 0 {
			continue
		}
		if !printed {
			fmt.Fprintf(w, "%sknown conformance gaps:\n", prefix)
			printed = true
		}
		fmt.Fprintf(w, "%s- %s: %d\n", prefix, kind, count)
	}
}

func printExpectedGapSummary(w io.Writer, diffs []comparisonDiff, prefix string) {
	if len(diffs) == 0 {
		return
	}
	fmt.Fprintf(w, "%sexpected known gaps:\n", prefix)
	counts := make(map[string]int)
	for _, diff := range diffs {
		kind := diff.Kind
		if kind == "" {
			kind = "path-only expected gap"
		}
		counts[kind]++
	}
	order := []string{
		"go render unsupported",
		"go render bitmap missing",
		"go render output mismatch",
		"go slot metrics unavailable",
		"glyph metric delta",
		"slot metric delta",
		"outline bbox delta",
		"outline point delta",
		"outline tag delta",
		"type1 segment delta",
		"path-only expected gap",
	}
	printed := make(map[string]bool)
	for _, kind := range order {
		count := counts[kind]
		if count == 0 {
			continue
		}
		printed[kind] = true
		fmt.Fprintf(w, "%s- %s: %d\n", prefix, kind, count)
	}
	var extras []string
	for kind := range counts {
		if !printed[kind] {
			extras = append(extras, kind)
		}
	}
	sort.Strings(extras)
	for _, kind := range extras {
		fmt.Fprintf(w, "%s- %s: %d\n", prefix, kind, counts[kind])
	}
}

func printRenderMismatchSummary(w io.Writer, diffs []comparisonDiff, prefix string) {
	counts := make(map[string]int)
	for _, diff := range diffs {
		if diff.Kind != "go render output mismatch" && diff.Kind != "go render bitmap missing" && diff.Kind != "go render unsupported" {
			continue
		}
		detail := renderMismatchDetail(diff.Path)
		if detail == "" {
			continue
		}
		counts[detail]++
	}
	if len(counts) == 0 {
		return
	}
	fmt.Fprintf(w, "%srender mismatch details:\n", prefix)
	order := []string{
		"render unsupported",
		"rendered format",
		"bitmap availability",
		"bitmap geometry",
		"bitmap pixel mode",
		"bitmap placement",
		"bitmap buffer",
		"bitmap buffer byte diff",
		"bitmap error",
	}
	printed := make(map[string]bool)
	for _, detail := range order {
		count := counts[detail]
		if count == 0 {
			continue
		}
		printed[detail] = true
		fmt.Fprintf(w, "%s- %s: %d\n", prefix, detail, count)
	}
	var extras []string
	for detail := range counts {
		if !printed[detail] {
			extras = append(extras, detail)
		}
	}
	sort.Strings(extras)
	for _, detail := range extras {
		fmt.Fprintf(w, "%s- %s: %d\n", prefix, detail, counts[detail])
	}
}

func renderMismatchDetail(path string) string {
	switch {
	case strings.HasSuffix(path, ".render_error"):
		return "render unsupported"
	case strings.HasSuffix(path, ".rendered_format"):
		return "rendered format"
	case strings.HasSuffix(path, ".bitmap.available"):
		return "bitmap availability"
	case strings.HasSuffix(path, ".bitmap.rows"), strings.HasSuffix(path, ".bitmap.width"), strings.HasSuffix(path, ".bitmap.pitch"):
		return "bitmap geometry"
	case strings.HasSuffix(path, ".bitmap.pixel_mode"), strings.HasSuffix(path, ".bitmap.pixel_mode_name"):
		return "bitmap pixel mode"
	case strings.HasSuffix(path, ".bitmap.left"), strings.HasSuffix(path, ".bitmap.top"):
		return "bitmap placement"
	case strings.HasSuffix(path, ".bitmap.buffer_size"), strings.HasSuffix(path, ".bitmap.sha256"):
		return "bitmap buffer"
	case strings.HasSuffix(path, ".bitmap.buffer_diff"):
		return "bitmap buffer byte diff"
	case strings.HasSuffix(path, ".bitmap.error"):
		return "bitmap error"
	default:
		return ""
	}
}

type metricDeltaStats struct {
	Fields      int
	MaxAbsDelta int64
}

func printMetricDeltaSummary(w io.Writer, diffs []comparisonDiff, prefix string) {
	stats := make(map[string]metricDeltaStats)
	addMetricDeltaStats(stats, diffs)
	printMetricDeltaStats(w, "metric delta details", stats, prefix)
}

func addMetricDeltaStats(stats map[string]metricDeltaStats, diffs []comparisonDiff) {
	for _, diff := range diffs {
		detail := metricDeltaDetail(diff.Path)
		if detail == "" {
			continue
		}
		stat := stats[detail]
		stat.Fields++
		if delta, ok := numericDiffDelta(diff); ok && delta > stat.MaxAbsDelta {
			stat.MaxAbsDelta = delta
		}
		stats[detail] = stat
	}
}

func printMetricDeltaStats(w io.Writer, label string, stats map[string]metricDeltaStats, prefix string) {
	if len(stats) == 0 {
		return
	}
	fmt.Fprintf(w, "%s%s:\n", prefix, label)
	order := []string{
		"glyph metrics",
		"slot metrics",
		"outline bbox",
		"outline points",
	}
	printed := make(map[string]bool)
	for _, detail := range order {
		stat, ok := stats[detail]
		if !ok {
			continue
		}
		printed[detail] = true
		fmt.Fprintf(w, "%s- %s: %d fields, max_abs_delta=%d\n", prefix, detail, stat.Fields, stat.MaxAbsDelta)
	}
	var extras []string
	for detail := range stats {
		if !printed[detail] {
			extras = append(extras, detail)
		}
	}
	sort.Strings(extras)
	for _, detail := range extras {
		stat := stats[detail]
		fmt.Fprintf(w, "%s- %s: %d fields, max_abs_delta=%d\n", prefix, detail, stat.Fields, stat.MaxAbsDelta)
	}
}

func metricDeltaKind(path string) string {
	switch metricDeltaDetail(path) {
	case "glyph metrics":
		return "glyph metric delta"
	case "slot metrics":
		return "slot metric delta"
	case "outline bbox":
		return "outline bbox delta"
	case "outline points":
		return "outline point delta"
	default:
		return ""
	}
}

func metricDeltaDetail(path string) string {
	switch {
	case strings.HasSuffix(path, ".metrics.advance"), strings.HasSuffix(path, ".metrics.lsb"):
		return "glyph metrics"
	case strings.Contains(path, ".slot_metrics.") && hasAnySuffix(path,
		".width",
		".height",
		".hori_bearing_x",
		".hori_bearing_y",
		".hori_advance",
		".vert_bearing_x",
		".vert_bearing_y",
		".vert_advance",
	):
		return "slot metrics"
	case strings.Contains(path, ".outline.bbox.") && hasAnySuffix(path, ".x_min", ".y_min", ".x_max", ".y_max"):
		return "outline bbox"
	case strings.Contains(path, ".outline.points[") && hasAnySuffix(path, ".x", ".y"):
		return "outline points"
	default:
		return ""
	}
}

func isOutlineTagDeltaPath(path string) bool {
	_, _, field, ok := parseOutlinePointTagPath(path)
	return ok && (field == "tag" || field == "tag_length")
}

func numericDiffDelta(diff comparisonDiff) (int64, bool) {
	want, err := strconv.ParseInt(diff.Want, 10, 64)
	if err != nil {
		return 0, false
	}
	got, err := strconv.ParseInt(diff.Got, 10, 64)
	if err != nil {
		return 0, false
	}
	delta := want - got
	if delta < 0 {
		delta = -delta
	}
	return delta, true
}

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

type outlinePointTagSummary struct {
	Glyphs map[string]*outlineGlyphPointTagSummary
}

type outlineGlyphPointTagSummary struct {
	Points          map[int]*outlinePointTagPointSummary
	TagLengthDeltas int
}

type outlinePointTagPointSummary struct {
	AxisCounts       map[string]int
	CoordinateFields int
	MaxAbsDelta      int64
	TagDeltas        int
	TagTransitions   map[string]int
}

func newOutlinePointTagSummary() outlinePointTagSummary {
	return outlinePointTagSummary{Glyphs: make(map[string]*outlineGlyphPointTagSummary)}
}

func printOutlinePointTagSummary(w io.Writer, diffs []comparisonDiff, prefix string) {
	stats := newOutlinePointTagSummary()
	addOutlinePointTagStats(&stats, diffs)
	printOutlinePointTagStats(w, "outline point/tag details", stats, prefix)
}

func addOutlinePointTagStats(stats *outlinePointTagSummary, diffs []comparisonDiff) {
	if stats.Glyphs == nil {
		stats.Glyphs = make(map[string]*outlineGlyphPointTagSummary)
	}
	for _, diff := range diffs {
		glyphPath, pointIndex, field, ok := parseOutlinePointTagPath(diff.Path)
		if !ok {
			continue
		}
		glyph := stats.Glyphs[glyphPath]
		if glyph == nil {
			glyph = &outlineGlyphPointTagSummary{Points: make(map[int]*outlinePointTagPointSummary)}
			stats.Glyphs[glyphPath] = glyph
		}
		if field == "tag_length" {
			glyph.TagLengthDeltas++
			continue
		}
		point := glyph.Points[pointIndex]
		if point == nil {
			point = &outlinePointTagPointSummary{
				AxisCounts:     make(map[string]int),
				TagTransitions: make(map[string]int),
			}
			glyph.Points[pointIndex] = point
		}
		switch field {
		case "x", "y":
			point.AxisCounts[field]++
			point.CoordinateFields++
			if delta, ok := numericDiffDelta(diff); ok && delta > point.MaxAbsDelta {
				point.MaxAbsDelta = delta
			}
		case "tag":
			point.TagDeltas++
			point.TagTransitions[diff.Want+"->"+diff.Got]++
		}
	}
}

func printOutlinePointTagStats(w io.Writer, label string, stats outlinePointTagSummary, prefix string) {
	if len(stats.Glyphs) == 0 {
		return
	}
	fmt.Fprintf(w, "%s%s:\n", prefix, label)
	glyphPaths := make([]string, 0, len(stats.Glyphs))
	for glyphPath := range stats.Glyphs {
		glyphPaths = append(glyphPaths, glyphPath)
	}
	sort.Strings(glyphPaths)
	for _, glyphPath := range glyphPaths {
		glyph := stats.Glyphs[glyphPath]
		coordinateOnly, coordinateAndTag, tagOnly, coordinateFields, tagDeltas, maxAbsDelta := outlinePointTagGlyphCounts(glyph)
		parts := []string{
			fmt.Sprintf("coordinate-only_points=%d", coordinateOnly),
			fmt.Sprintf("coordinate+tag_points=%d", coordinateAndTag),
			fmt.Sprintf("tag-only_points=%d", tagOnly),
		}
		if coordinateFields > 0 {
			parts = append(parts,
				fmt.Sprintf("coordinate_fields=%d", coordinateFields),
				fmt.Sprintf("max_abs_delta=%d", maxAbsDelta),
			)
		}
		if tagDeltas > 0 {
			parts = append(parts, fmt.Sprintf("tag_deltas=%d", tagDeltas))
			if transitions := outlineGlyphTagTransitions(glyph); transitions != "" {
				parts = append(parts, "tag_transitions=["+transitions+"]")
			}
		}
		if glyph.TagLengthDeltas > 0 {
			parts = append(parts, fmt.Sprintf("tag_length_deltas=%d", glyph.TagLengthDeltas))
		}
		fmt.Fprintf(w, "%s- %s: %s\n", prefix, glyphPath, strings.Join(parts, ", "))
		pointIndexes := make([]int, 0, len(glyph.Points))
		for pointIndex := range glyph.Points {
			pointIndexes = append(pointIndexes, pointIndex)
		}
		sort.Ints(pointIndexes)
		for _, pointIndex := range pointIndexes {
			fmt.Fprintf(w, "%s  - point[%d]: %s\n", prefix, pointIndex, outlinePointTagPointDetail(glyph.Points[pointIndex]))
		}
	}
}

func outlinePointTagGlyphCounts(glyph *outlineGlyphPointTagSummary) (coordinateOnly, coordinateAndTag, tagOnly, coordinateFields, tagDeltas int, maxAbsDelta int64) {
	for _, point := range glyph.Points {
		hasCoordinates := point.CoordinateFields > 0
		hasTags := point.TagDeltas > 0
		switch {
		case hasCoordinates && hasTags:
			coordinateAndTag++
		case hasCoordinates:
			coordinateOnly++
		case hasTags:
			tagOnly++
		}
		coordinateFields += point.CoordinateFields
		tagDeltas += point.TagDeltas
		if point.MaxAbsDelta > maxAbsDelta {
			maxAbsDelta = point.MaxAbsDelta
		}
	}
	return coordinateOnly, coordinateAndTag, tagOnly, coordinateFields, tagDeltas, maxAbsDelta
}

func outlinePointTagPointDetail(point *outlinePointTagPointSummary) string {
	hasCoordinates := point.CoordinateFields > 0
	hasTags := point.TagDeltas > 0
	var parts []string
	switch {
	case hasCoordinates && hasTags:
		parts = append(parts, "coordinate+tag")
	case hasCoordinates:
		parts = append(parts, "coordinates")
	case hasTags:
		parts = append(parts, "tag-only")
	}
	if hasCoordinates {
		parts = append(parts,
			"axes="+formatCountMap(point.AxisCounts, []string{"x", "y"}),
			fmt.Sprintf("fields=%d", point.CoordinateFields),
			fmt.Sprintf("max_abs_delta=%d", point.MaxAbsDelta),
		)
	}
	if hasTags {
		parts = append(parts, "tag "+formatCountMap(point.TagTransitions, nil))
	}
	return strings.Join(parts, " ")
}

func outlineGlyphTagTransitions(glyph *outlineGlyphPointTagSummary) string {
	counts := make(map[string]int)
	for _, point := range glyph.Points {
		for transition, count := range point.TagTransitions {
			counts[transition] += count
		}
	}
	return formatCountMap(counts, nil)
}

func formatCountMap(counts map[string]int, preferred []string) string {
	if len(counts) == 0 {
		return ""
	}
	var keys []string
	printed := make(map[string]bool)
	for _, key := range preferred {
		if _, ok := counts[key]; !ok {
			continue
		}
		keys = append(keys, key)
		printed[key] = true
	}
	var extras []string
	for key := range counts {
		if !printed[key] {
			extras = append(extras, key)
		}
	}
	sort.Strings(extras)
	keys = append(keys, extras...)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		count := counts[key]
		if count == 1 {
			parts = append(parts, key)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%d", key, count))
	}
	return strings.Join(parts, ",")
}

func parseOutlinePointTagPath(path string) (glyphPath string, pointIndex int, field string, ok bool) {
	const outlineSegment = ".outline."
	outlineIndex := strings.Index(path, outlineSegment)
	if outlineIndex < 0 {
		return "", 0, "", false
	}
	glyphPath = path[:outlineIndex]
	tail := path[outlineIndex+len(outlineSegment):]
	switch {
	case strings.HasPrefix(tail, "points["):
		index, rest, ok := parseIndexedPath(tail, "points")
		if !ok || (rest != ".x" && rest != ".y") {
			return "", 0, "", false
		}
		return glyphPath, index, strings.TrimPrefix(rest, "."), true
	case strings.HasPrefix(tail, "tags["):
		index, rest, ok := parseIndexedPath(tail, "tags")
		if !ok || rest != "" {
			return "", 0, "", false
		}
		return glyphPath, index, "tag", true
	case tail == "tags.length":
		return glyphPath, -1, "tag_length", true
	default:
		return "", 0, "", false
	}
}

func parseIndexedPath(path, prefix string) (index int, rest string, ok bool) {
	open := prefix + "["
	if !strings.HasPrefix(path, open) {
		return 0, "", false
	}
	indexStart := len(open)
	indexEnd := strings.IndexByte(path[indexStart:], ']')
	if indexEnd < 0 {
		return 0, "", false
	}
	indexEnd += indexStart
	index, err := strconv.Atoi(path[indexStart:indexEnd])
	if err != nil {
		return 0, "", false
	}
	return index, path[indexEnd+1:], true
}

func printCompareFailure(w io.Writer, label string, diffs, expected, unexpected []comparisonDiff, opts compareOptions, prefix string) {
	header := "FAIL"
	if label != "" {
		header = label + ": FAIL"
	}
	if len(expected) > 0 {
		fmt.Fprintf(w, "%s%s (%d diffs: %d unexpected, %d expected)\n", prefix, header, len(diffs), len(unexpected), len(expected))
		printExpectedGapSummary(w, expected, prefix)
	} else {
		fmt.Fprintf(w, "%s%s (%d diffs)\n", prefix, header, len(diffs))
	}
	displayDiffs := unexpected
	if len(displayDiffs) == 0 {
		displayDiffs = diffs
	}
	printKnownGapSummary(w, displayDiffs, prefix)
	printRenderMismatchSummary(w, displayDiffs, prefix)
	printMetricDeltaSummary(w, displayDiffs, prefix)
	printOutlinePointTagSummary(w, displayDiffs, prefix)
	printDiffList(w, displayDiffs, opts.MaxDiffs, prefix)
}

func printDiffList(w io.Writer, diffs []comparisonDiff, maxDiffs int, prefix string) {
	limit := maxDiffs
	if limit <= 0 || limit > len(diffs) {
		limit = len(diffs)
	}
	for i := 0; i < limit; i++ {
		diff := diffs[i]
		if diff.Kind != "" {
			fmt.Fprintf(w, "%s- %s [%s]: want %s, got %s\n", prefix, diff.Path, diff.Kind, diff.Want, diff.Got)
			continue
		}
		fmt.Fprintf(w, "%s- %s: want %s, got %s\n", prefix, diff.Path, diff.Want, diff.Got)
	}
	if limit < len(diffs) {
		fmt.Fprintf(w, "%s... %d additional diffs omitted\n", prefix, len(diffs)-limit)
	}
}

func expectedGapsForCompare(reference, candidate *Dump, opts compareOptions) []ExpectedGap {
	gaps := make([]ExpectedGap, 0, len(opts.ExpectedGaps)+len(reference.Request.ExpectedGaps)+len(candidate.Request.ExpectedGaps))
	gaps = append(gaps, opts.ExpectedGaps...)
	gaps = append(gaps, reference.Request.ExpectedGaps...)
	gaps = append(gaps, candidate.Request.ExpectedGaps...)
	return dedupeExpectedGaps(gaps)
}

func splitExpectedDiffs(diffs []comparisonDiff, gaps []ExpectedGap) ([]comparisonDiff, []comparisonDiff) {
	if len(gaps) == 0 {
		return nil, append([]comparisonDiff(nil), diffs...)
	}
	var expected []comparisonDiff
	var unexpected []comparisonDiff
	for _, diff := range diffs {
		if expectedGapMatchesAny(gaps, diff) {
			expected = append(expected, diff)
			continue
		}
		unexpected = append(unexpected, diff)
	}
	return expected, unexpected
}

type expectedGapUse struct {
	Gap     ExpectedGap
	Matches int
}

func expectedGapUsage(diffs []comparisonDiff, gaps []ExpectedGap) []expectedGapUse {
	uses := make([]expectedGapUse, 0, len(gaps))
	for _, gap := range dedupeExpectedGaps(gaps) {
		use := expectedGapUse{Gap: gap}
		for _, diff := range diffs {
			if expectedGapMatches(gap, diff) {
				use.Matches++
			}
		}
		uses = append(uses, use)
	}
	return uses
}

func staleExpectedGaps(uses []expectedGapUse) []ExpectedGap {
	var stale []ExpectedGap
	for _, use := range uses {
		if use.Matches == 0 {
			stale = append(stale, use.Gap)
		}
	}
	return stale
}

func printStaleExpectedGaps(w io.Writer, gaps []ExpectedGap, prefix string, maxGaps int) {
	if len(gaps) == 0 {
		return
	}
	fmt.Fprintf(w, "%sstale expected gaps (%d):\n", prefix, len(gaps))
	limit := maxGaps
	if limit <= 0 || limit > len(gaps) {
		limit = len(gaps)
	}
	for i := 0; i < limit; i++ {
		fmt.Fprintf(w, "%s- %s\n", prefix, formatExpectedGap(gaps[i]))
	}
	if limit < len(gaps) {
		fmt.Fprintf(w, "%s... %d additional stale expected gaps omitted\n", prefix, len(gaps)-limit)
	}
}

func formatExpectedGap(gap ExpectedGap) string {
	var parts []string
	if strings.TrimSpace(gap.Kind) != "" {
		parts = append(parts, "kind="+strconv.Quote(gap.Kind))
	}
	if strings.TrimSpace(gap.Path) != "" {
		parts = append(parts, "path="+strconv.Quote(gap.Path))
	}
	if strings.TrimSpace(gap.Note) != "" {
		parts = append(parts, "note="+strconv.Quote(gap.Note))
	}
	return strings.Join(parts, " ")
}

func expectedGapMatchesAny(gaps []ExpectedGap, diff comparisonDiff) bool {
	for _, gap := range gaps {
		if expectedGapMatches(gap, diff) {
			return true
		}
	}
	return false
}

func expectedGapMatches(gap ExpectedGap, diff comparisonDiff) bool {
	kind := strings.TrimSpace(gap.Kind)
	path := strings.TrimSpace(gap.Path)
	if kind == "" && path == "" {
		return false
	}
	if kind != "" && kind != diff.Kind {
		return false
	}
	if path == "" {
		return true
	}
	if path == diff.Path {
		return true
	}
	if strings.HasSuffix(path, "*") {
		return strings.HasPrefix(diff.Path, strings.TrimSuffix(path, "*"))
	}
	return false
}

func dedupeExpectedGaps(gaps []ExpectedGap) []ExpectedGap {
	seen := make(map[string]bool)
	out := make([]ExpectedGap, 0, len(gaps))
	for _, gap := range gaps {
		gap.Kind = strings.TrimSpace(gap.Kind)
		gap.Path = strings.TrimSpace(gap.Path)
		gap.Note = strings.TrimSpace(gap.Note)
		if gap.Kind == "" && gap.Path == "" {
			continue
		}
		key := gap.Kind + "\x00" + gap.Path + "\x00" + gap.Note
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, gap)
	}
	return out
}

func compareCharmaps(reference, candidate []CharMapRecord, add func(path, want, got string)) {
	candidateByChar := make(map[string]CharMapRecord, len(candidate))
	for _, record := range candidate {
		candidateByChar[record.Char] = record
	}
	for _, refRecord := range reference {
		path := "charmap[" + refRecord.Char + "]"
		candRecord, ok := candidateByChar[refRecord.Char]
		if !ok {
			add(path, "present", "missing")
			continue
		}
		if refRecord.Error != candRecord.Error {
			add(path+".error", quoted(refRecord.Error), quoted(candRecord.Error))
		}
		if refRecord.GlyphIndex != candRecord.GlyphIndex {
			add(path+".glyph_index", fmt.Sprint(refRecord.GlyphIndex), fmt.Sprint(candRecord.GlyphIndex))
		}
	}
}

func compareGlyphs(path string, reference, candidate []GlyphRecord, opts compareOptions, add func(path, want, got string)) {
	candidateByGlyph := make(map[int]GlyphRecord, len(candidate))
	for _, glyph := range candidate {
		candidateByGlyph[glyph.GlyphIndex] = glyph
	}

	for _, refGlyph := range reference {
		glyphPath := fmt.Sprintf("%s.glyphs[%d]", path, refGlyph.GlyphIndex)
		candGlyph, ok := candidateByGlyph[refGlyph.GlyphIndex]
		if !ok {
			add(glyphPath, "present", "missing")
			continue
		}
		if refGlyph.LoadError != candGlyph.LoadError {
			add(glyphPath+".load_error", quoted(refGlyph.LoadError), quoted(candGlyph.LoadError))
		}
		if refGlyph.RenderError != candGlyph.RenderError {
			add(glyphPath+".render_error", quoted(refGlyph.RenderError), quoted(candGlyph.RenderError))
		}
		if refGlyph.GlyphName != "" && refGlyph.GlyphName != candGlyph.GlyphName {
			add(glyphPath+".glyph_name", quoted(refGlyph.GlyphName), quoted(candGlyph.GlyphName))
		}
		if refGlyph.Format != "" && refGlyph.Format != candGlyph.Format {
			add(glyphPath+".format", quoted(refGlyph.Format), quoted(candGlyph.Format))
		}
		if refGlyph.RenderedFormat != "" && refGlyph.RenderedFormat != candGlyph.RenderedFormat {
			add(glyphPath+".rendered_format", quoted(refGlyph.RenderedFormat), quoted(candGlyph.RenderedFormat))
		}
		compareMetrics(glyphPath+".metrics", refGlyph.Metrics, candGlyph.Metrics, opts.MetricTolerance, add)
		compareSlotMetrics(glyphPath+".slot_metrics", refGlyph.SlotMetrics, candGlyph.SlotMetrics, opts, add)
		compareOutlines(glyphPath+".outline", refGlyph.Outline, candGlyph.Outline, opts.PointTolerance, add)
		compareBitmaps(glyphPath+".bitmap", refGlyph.Bitmap, candGlyph.Bitmap, opts.AllowMissingBitmap, add)
		compareType1Segments(glyphPath+".type1_segments", refGlyph.Type1Segments, candGlyph.Type1Segments, add)
	}
}

func compareType1Segments(path string, reference, candidate []type1.SegmentRecord, add func(path, want, got string)) {
	if len(reference) == 0 {
		return
	}
	if len(candidate) == 0 {
		add(path, "present", "missing")
		return
	}
	if len(reference) != len(candidate) {
		add(path+".length", fmt.Sprint(len(reference)), fmt.Sprint(len(candidate)))
	}
	limit := len(reference)
	if len(candidate) < limit {
		limit = len(candidate)
	}
	for i := 0; i < limit; i++ {
		segmentPath := fmt.Sprintf("%s[%d]", path, i)
		if reference[i].Kind != candidate[i].Kind {
			add(segmentPath+".kind", quoted(reference[i].Kind), quoted(candidate[i].Kind))
		}
		if reference[i].Units != candidate[i].Units {
			add(segmentPath+".units", quoted(reference[i].Units), quoted(candidate[i].Units))
		}
		compareType1SegmentPoints(segmentPath+".points", reference[i].Points, candidate[i].Points, add)
	}
}

func compareType1SegmentPoints(path string, reference, candidate []type1.SegmentPoint, add func(path, want, got string)) {
	if len(reference) != len(candidate) {
		add(path+".length", fmt.Sprint(len(reference)), fmt.Sprint(len(candidate)))
	}
	limit := len(reference)
	if len(candidate) < limit {
		limit = len(candidate)
	}
	for i := 0; i < limit; i++ {
		pointPath := fmt.Sprintf("%s[%d]", path, i)
		if reference[i].X != candidate[i].X {
			add(pointPath+".x", fmt.Sprint(reference[i].X), fmt.Sprint(candidate[i].X))
		}
		if reference[i].Y != candidate[i].Y {
			add(pointPath+".y", fmt.Sprint(reference[i].Y), fmt.Sprint(candidate[i].Y))
		}
	}
}

func compareMetrics(path string, reference, candidate MetricsRecord, tolerance int64, add func(path, want, got string)) {
	if reference.Available != candidate.Available {
		add(path+".available", fmt.Sprint(reference.Available), fmt.Sprint(candidate.Available))
		return
	}
	if reference.Error != candidate.Error {
		add(path+".error", quoted(reference.Error), quoted(candidate.Error))
	}
	if !reference.Available {
		return
	}
	if absDelta(reference.Advance, candidate.Advance) > tolerance {
		add(path+".advance", fmt.Sprint(reference.Advance), fmt.Sprint(candidate.Advance))
	}
	if absDelta(reference.LSB, candidate.LSB) > tolerance {
		add(path+".lsb", fmt.Sprint(reference.LSB), fmt.Sprint(candidate.LSB))
	}
}

func compareOutlines(path string, reference, candidate OutlineRecord, tolerance int64, add func(path, want, got string)) {
	if reference.Available != candidate.Available {
		add(path+".available", fmt.Sprint(reference.Available), fmt.Sprint(candidate.Available))
		return
	}
	if reference.Error != candidate.Error {
		add(path+".error", quoted(reference.Error), quoted(candidate.Error))
	}
	if !reference.Available {
		return
	}
	if reference.PointCount != candidate.PointCount {
		add(path+".point_count", fmt.Sprint(reference.PointCount), fmt.Sprint(candidate.PointCount))
	}
	if reference.ContourCount != candidate.ContourCount {
		add(path+".contour_count", fmt.Sprint(reference.ContourCount), fmt.Sprint(candidate.ContourCount))
	}
	if reference.RawPointCount != candidate.RawPointCount {
		add(path+".raw_point_count", fmt.Sprint(reference.RawPointCount), fmt.Sprint(candidate.RawPointCount))
	}
	if reference.PhantomPointCount != candidate.PhantomPointCount {
		add(path+".phantom_point_count", fmt.Sprint(reference.PhantomPointCount), fmt.Sprint(candidate.PhantomPointCount))
	}
	compareBBox(path+".bbox", reference.BBox, candidate.BBox, tolerance, add)
	if !equalInts(reference.Contours, candidate.Contours) {
		add(path+".contours", fmt.Sprint(reference.Contours), fmt.Sprint(candidate.Contours))
	}
	compareOutlineTags(path+".tags", reference.Tags, candidate.Tags, add)
	for i := 0; i < len(reference.Points) && i < len(candidate.Points); i++ {
		if absDelta(reference.Points[i].X, candidate.Points[i].X) > tolerance {
			add(fmt.Sprintf("%s.points[%d].x", path, i), fmt.Sprint(reference.Points[i].X), fmt.Sprint(candidate.Points[i].X))
		}
		if absDelta(reference.Points[i].Y, candidate.Points[i].Y) > tolerance {
			add(fmt.Sprintf("%s.points[%d].y", path, i), fmt.Sprint(reference.Points[i].Y), fmt.Sprint(candidate.Points[i].Y))
		}
	}
}

func compareOutlineTags(path string, reference, candidate []int, add func(path, want, got string)) {
	if len(reference) != len(candidate) {
		add(path+".length", fmt.Sprint(len(reference)), fmt.Sprint(len(candidate)))
	}
	limit := len(reference)
	if len(candidate) < limit {
		limit = len(candidate)
	}
	for i := 0; i < limit; i++ {
		if reference[i] != candidate[i] {
			add(fmt.Sprintf("%s[%d]", path, i), fmt.Sprint(reference[i]), fmt.Sprint(candidate[i]))
		}
	}
}

func compareSlotMetrics(path string, reference, candidate *SlotMetricsRecord, opts compareOptions, add func(path, want, got string)) {
	if reference == nil && candidate == nil {
		return
	}
	if reference == nil {
		return
	}
	if reference == nil || candidate == nil {
		if opts.AllowMissingSlotMetrics {
			return
		}
		add(path, present(reference != nil), present(candidate != nil))
		return
	}
	if reference.Available != candidate.Available {
		if !opts.AllowMissingSlotMetrics {
			add(path+".available", fmt.Sprint(reference.Available), fmt.Sprint(candidate.Available))
		}
		return
	}
	if reference.Error != candidate.Error {
		add(path+".error", quoted(reference.Error), quoted(candidate.Error))
	}
	if !reference.Available {
		return
	}
	compareMetricField(path+".width", reference.Width, candidate.Width, opts.MetricTolerance, add)
	compareMetricField(path+".height", reference.Height, candidate.Height, opts.MetricTolerance, add)
	compareMetricField(path+".hori_bearing_x", reference.HoriBearingX, candidate.HoriBearingX, opts.MetricTolerance, add)
	compareMetricField(path+".hori_bearing_y", reference.HoriBearingY, candidate.HoriBearingY, opts.MetricTolerance, add)
	compareMetricField(path+".hori_advance", reference.HoriAdvance, candidate.HoriAdvance, opts.MetricTolerance, add)
	compareMetricField(path+".vert_bearing_x", reference.VertBearingX, candidate.VertBearingX, opts.MetricTolerance, add)
	compareMetricField(path+".vert_bearing_y", reference.VertBearingY, candidate.VertBearingY, opts.MetricTolerance, add)
	compareMetricField(path+".vert_advance", reference.VertAdvance, candidate.VertAdvance, opts.MetricTolerance, add)
}

func compareMetricField(path string, reference, candidate int32, tolerance int64, add func(path, want, got string)) {
	if absDelta(reference, candidate) > tolerance {
		add(path, fmt.Sprint(reference), fmt.Sprint(candidate))
	}
}

func compareBBox(path string, reference, candidate *BBox, tolerance int64, add func(path, want, got string)) {
	if reference == nil && candidate == nil {
		return
	}
	if reference == nil || candidate == nil {
		add(path, present(reference != nil), present(candidate != nil))
		return
	}
	compareMetricField(path+".x_min", reference.XMin, candidate.XMin, tolerance, add)
	compareMetricField(path+".y_min", reference.YMin, candidate.YMin, tolerance, add)
	compareMetricField(path+".x_max", reference.XMax, candidate.XMax, tolerance, add)
	compareMetricField(path+".y_max", reference.YMax, candidate.YMax, tolerance, add)
}

func compareBitmaps(path string, reference, candidate BitmapRecord, allowMissing bool, add func(path, want, got string)) {
	if reference.Available != candidate.Available {
		if !allowMissing {
			add(path+".available", fmt.Sprint(reference.Available), fmt.Sprint(candidate.Available))
		}
		return
	}
	if reference.Error != candidate.Error {
		add(path+".error", quoted(reference.Error), quoted(candidate.Error))
	}
	if !reference.Available {
		return
	}
	if reference.Rows != candidate.Rows {
		add(path+".rows", fmt.Sprint(reference.Rows), fmt.Sprint(candidate.Rows))
	}
	if reference.Width != candidate.Width {
		add(path+".width", fmt.Sprint(reference.Width), fmt.Sprint(candidate.Width))
	}
	if reference.Pitch != candidate.Pitch {
		add(path+".pitch", fmt.Sprint(reference.Pitch), fmt.Sprint(candidate.Pitch))
	}
	samePixelModeName := reference.PixelModeName != "" && reference.PixelModeName == candidate.PixelModeName
	if reference.PixelMode != candidate.PixelMode && !samePixelModeName {
		add(path+".pixel_mode", fmt.Sprint(reference.PixelMode), fmt.Sprint(candidate.PixelMode))
	}
	if reference.PixelModeName != "" && reference.PixelModeName != candidate.PixelModeName {
		add(path+".pixel_mode_name", quoted(reference.PixelModeName), quoted(candidate.PixelModeName))
	}
	if reference.PixelModeName != "" {
		if reference.Left != candidate.Left {
			add(path+".left", fmt.Sprint(reference.Left), fmt.Sprint(candidate.Left))
		}
		if reference.Top != candidate.Top {
			add(path+".top", fmt.Sprint(reference.Top), fmt.Sprint(candidate.Top))
		}
	}
	if reference.BufferSize != candidate.BufferSize {
		add(path+".buffer_size", fmt.Sprint(reference.BufferSize), fmt.Sprint(candidate.BufferSize))
	}
	if reference.SHA256 != candidate.SHA256 {
		add(path+".sha256", reference.SHA256, candidate.SHA256)
	}
	if summary, ok := bitmapBufferDiffSummary(reference, candidate); ok {
		add(path+".buffer_diff", "matching buffer", summary)
	}
}

func bitmapBufferDiffSummary(reference, candidate BitmapRecord) (string, bool) {
	if reference.BufferHex == "" || candidate.BufferHex == "" {
		return "", false
	}
	referenceBuffer, err := hex.DecodeString(reference.BufferHex)
	if err != nil {
		return "", false
	}
	candidateBuffer, err := hex.DecodeString(candidate.BufferHex)
	if err != nil {
		return "", false
	}
	if bytes.Equal(referenceBuffer, candidateBuffer) {
		return "", false
	}

	byteDiffCount, firstDiffOffset := byteDiffStats(referenceBuffer, candidateBuffer)
	parts := []string{
		fmt.Sprintf("first_diff_offset=%d", firstDiffOffset),
		fmt.Sprintf("byte_diff_count=%d", byteDiffCount),
	}
	if rowDiffs, rowDiffRows, omitted := bitmapRowDiffSummaries(reference, candidate, referenceBuffer, candidateBuffer); rowDiffRows > 0 {
		parts = append(parts, fmt.Sprintf("row_diff_rows=%d", rowDiffRows))
		parts = append(parts, "row_byte_diff_counts=["+strings.Join(rowDiffs, "; ")+"]")
		if omitted > 0 {
			parts = append(parts, fmt.Sprintf("row_byte_diff_counts_omitted=%d", omitted))
		}
	}
	return strings.Join(parts, ", "), true
}

func byteDiffStats(reference, candidate []byte) (count int, first int) {
	first = -1
	limit := len(reference)
	if len(candidate) > limit {
		limit = len(candidate)
	}
	for i := 0; i < limit; i++ {
		referenceByte, referenceOK := byteAt(reference, i)
		candidateByte, candidateOK := byteAt(candidate, i)
		if referenceOK == candidateOK && referenceByte == candidateByte {
			continue
		}
		count++
		if first < 0 {
			first = i
		}
	}
	return count, first
}

func bitmapRowDiffSummaries(reference, candidate BitmapRecord, referenceBuffer, candidateBuffer []byte) ([]string, int, int) {
	const maxRows = 16
	if reference.Rows <= 0 || reference.Rows != candidate.Rows {
		return nil, 0, 0
	}
	referenceStride := absInt(reference.Pitch)
	candidateStride := absInt(candidate.Pitch)
	if referenceStride <= 0 || referenceStride != candidateStride {
		return nil, 0, 0
	}

	var rows []string
	rowDiffRows := 0
	for row := 0; row < reference.Rows; row++ {
		start := row * referenceStride
		count, firstCol := byteDiffStatsInRange(referenceBuffer, candidateBuffer, start, referenceStride)
		if count == 0 {
			continue
		}
		rowDiffRows++
		if len(rows) < maxRows {
			rows = append(rows, fmt.Sprintf("row=%d count=%d first_col=%d", row, count, firstCol))
		}
	}
	return rows, rowDiffRows, rowDiffRows - len(rows)
}

func byteDiffStatsInRange(reference, candidate []byte, start, length int) (count int, first int) {
	first = -1
	for col := 0; col < length; col++ {
		offset := start + col
		referenceByte, referenceOK := byteAt(reference, offset)
		candidateByte, candidateOK := byteAt(candidate, offset)
		if referenceOK == candidateOK && referenceByte == candidateByte {
			continue
		}
		count++
		if first < 0 {
			first = col
		}
	}
	return count, first
}

func byteAt(buffer []byte, offset int) (byte, bool) {
	if offset < 0 || offset >= len(buffer) {
		return 0, false
	}
	return buffer[offset], true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sizeKey(size SizeDump) string {
	if size.RenderMode == "" || size.RenderMode == "none" {
		return fmt.Sprintf("%dx%d/%s", size.PPEMX, size.PPEMY, size.LoadFlags)
	}
	return fmt.Sprintf("%dx%d/%s/render-%s", size.PPEMX, size.PPEMY, size.LoadFlags, size.RenderMode)
}

func quoted(s string) string {
	if s == "" {
		return "<empty>"
	}
	return fmt.Sprintf("%q", s)
}

func present(ok bool) string {
	if ok {
		return "present"
	}
	return "missing"
}

func absDelta(a, b int32) int64 {
	d := int64(a) - int64(b)
	if d < 0 {
		return -d
	}
	return d
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
