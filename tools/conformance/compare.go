package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

var errComparisonFailed = errors.New("conformance comparison failed")

type compareOptions struct {
	ReferencePath           string
	CandidatePath           string
	MetricTolerance         int64
	PointTolerance          int64
	AllowMissingBitmap      bool
	AllowMissingSlotMetrics bool
	MaxDiffs                int
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
	if len(diffs) == 0 {
		fmt.Fprintln(stdout, "conformance compare: PASS")
		return nil
	}

	fmt.Fprintf(stdout, "conformance compare: FAIL (%d diffs)\n", len(diffs))
	printKnownGapSummary(stdout, diffs, "")
	printDiffList(stdout, diffs, opts.MaxDiffs, "")
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
	case strings.Contains(path, "/render-") && strings.HasSuffix(path, ".rendered_format") && want != got:
		return "go render output mismatch"
	case strings.Contains(path, ".slot_metrics") && (got == "missing" || got == "false" || strings.Contains(gotLower, "unavailable")):
		return "go slot metrics unavailable"
	default:
		return ""
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
	if !equalInts(reference.Tags, candidate.Tags) {
		add(path+".tags", fmt.Sprint(reference.Tags), fmt.Sprint(candidate.Tags))
	}
	for i := 0; i < len(reference.Points) && i < len(candidate.Points); i++ {
		if absDelta(reference.Points[i].X, candidate.Points[i].X) > tolerance {
			add(fmt.Sprintf("%s.points[%d].x", path, i), fmt.Sprint(reference.Points[i].X), fmt.Sprint(candidate.Points[i].X))
		}
		if absDelta(reference.Points[i].Y, candidate.Points[i].Y) > tolerance {
			add(fmt.Sprintf("%s.points[%d].y", path, i), fmt.Sprint(reference.Points[i].Y), fmt.Sprint(candidate.Points[i].Y))
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
	if reference.PixelMode != candidate.PixelMode {
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
