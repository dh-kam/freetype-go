package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	t1 "github.com/dh-kam/freetype-go/type1"
)

func TestParsePPEMList(t *testing.T) {
	ppems, err := parsePPEMList("12,16x20,24")
	if err != nil {
		t.Fatalf("parsePPEMList failed: %v", err)
	}
	want := []SizeSpec{{X: 12, Y: 12}, {X: 16, Y: 20}, {X: 24, Y: 24}}
	if len(ppems) != len(want) {
		t.Fatalf("len(ppems) = %d, want %d", len(ppems), len(want))
	}
	for i := range want {
		if ppems[i] != want[i] {
			t.Fatalf("ppems[%d] = %+v, want %+v", i, ppems[i], want[i])
		}
	}
}

func TestParseGlyphList(t *testing.T) {
	glyphs, err := parseGlyphList("0,3,5-7")
	if err != nil {
		t.Fatalf("parseGlyphList failed: %v", err)
	}
	want := []int{0, 3, 5, 6, 7}
	if len(glyphs) != len(want) {
		t.Fatalf("len(glyphs) = %d, want %d", len(glyphs), len(want))
	}
	for i := range want {
		if glyphs[i] != want[i] {
			t.Fatalf("glyphs[%d] = %d, want %d", i, glyphs[i], want[i])
		}
	}
}

func TestParseLoadFlagSets(t *testing.T) {
	specs, err := parseLoadFlagSets("default,no-hinting+target-light,no-bitmap+target-mono")
	if err != nil {
		t.Fatalf("parseLoadFlagSets failed: %v", err)
	}
	if len(specs) != 3 {
		t.Fatalf("len(specs) = %d, want 3", len(specs))
	}
	if specs[0].Name != "default" || specs[0].Value != api.LoadDefault {
		t.Fatalf("default spec = %+v", specs[0])
	}
	if specs[1].Name != "no-hinting+target-light" {
		t.Fatalf("specs[1].Name = %q", specs[1].Name)
	}
	if specs[1].Value&api.LoadNoHinting == 0 || specs[1].Value&api.LoadTargetMask != api.LoadTargetLight {
		t.Fatalf("specs[1].Value = %#x", specs[1].Value)
	}
	if specs[2].Value&api.LoadNoBitmap == 0 || specs[2].Value&api.LoadTargetMask != api.LoadTargetMono {
		t.Fatalf("specs[2].Value = %#x", specs[2].Value)
	}
}

func TestParseLoadFlagCanonicalAliases(t *testing.T) {
	spec, err := parseLoadFlagSet("FT_LOAD_NO_HINTING | LOAD-NO_BITMAP | FT_LOAD_TARGET_LCD_V")
	if err != nil {
		t.Fatalf("parseLoadFlagSet failed: %v", err)
	}
	if spec.Name != "no-hinting+no-bitmap+target-lcd-v" {
		t.Fatalf("Name = %q", spec.Name)
	}
	if spec.Value&api.LoadNoHinting == 0 {
		t.Fatalf("LoadNoHinting missing from %#x", spec.Value)
	}
	if spec.Value&api.LoadNoBitmap == 0 {
		t.Fatalf("LoadNoBitmap missing from %#x", spec.Value)
	}
	if spec.Value&api.LoadTargetMask != api.LoadTargetLCDV {
		t.Fatalf("target = %#x, want %#x", spec.Value&api.LoadTargetMask, api.LoadTargetLCDV)
	}
}

func TestParseLoadFlagRejectsMultipleTargets(t *testing.T) {
	if _, err := parseLoadFlagSet("target-light+target-mono"); err == nil {
		t.Fatal("parseLoadFlagSet succeeded with multiple targets")
	}
}

func TestParseRenderModeList(t *testing.T) {
	modes, err := parseRenderModeList("none,normal,lcd-v")
	if err != nil {
		t.Fatalf("parseRenderModeList failed: %v", err)
	}
	if len(modes) != 3 {
		t.Fatalf("len(modes) = %d, want 3", len(modes))
	}
	if modes[0].Value != api.RenderModeNone || modes[1].Value != api.RenderModeNormal || modes[2].Value != api.RenderModeLCDV {
		t.Fatalf("modes = %+v", modes)
	}
}

func TestParseRenderModeCanonicalAliases(t *testing.T) {
	modes, err := parseRenderModeList("off,FT_RENDER_MODE_LCD_V,render-light,monochrome")
	if err != nil {
		t.Fatalf("parseRenderModeList failed: %v", err)
	}
	got := renderModeNames(modes)
	want := []string{"none", "lcd-v", "light", "mono"}
	if !slices.Equal(got, want) {
		t.Fatalf("modes = %v, want %v", got, want)
	}
}

func TestParseDumpOptionsRequestFile(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(requestPath, []byte(`{
		"font": "fixture.ttf",
		"face_index": 2,
		"ppem": ["9", "16x20"],
		"glyphs": [0, 3],
		"glyph_ranges": ["5-6"],
		"chars": "U+0041,U+0061",
		"load_flags": ["default", "no-hinting+target-light"],
		"render_mode": "none,normal",
		"corpus": "smoke",
		"include_bitmap_buffer": true
	}`), 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}

	opts, err := parseDumpOptions("dump", []string{"-request", requestPath, "-font", "override.ttf"}, os.Stderr)
	if err != nil {
		t.Fatalf("parseDumpOptions failed: %v", err)
	}
	if opts.FontPath != "override.ttf" {
		t.Fatalf("FontPath = %q", opts.FontPath)
	}
	if opts.FaceIndex != 2 || opts.Corpus != "smoke" {
		t.Fatalf("opts = %+v", opts)
	}
	if len(opts.PPEMs) != 2 || opts.PPEMs[1] != (SizeSpec{X: 16, Y: 20}) {
		t.Fatalf("PPEMs = %+v", opts.PPEMs)
	}
	if got, want := opts.Glyphs, []int{0, 3, 5, 6}; len(got) != len(want) || got[3] != want[3] {
		t.Fatalf("Glyphs = %+v, want %+v", got, want)
	}
	if len(opts.Chars) != 2 || opts.Chars[0].Label != "U+0041" {
		t.Fatalf("Chars = %+v", opts.Chars)
	}
	if len(opts.LoadFlags) != 2 || opts.LoadFlags[1].Name != "no-hinting+target-light" {
		t.Fatalf("LoadFlags = %+v", opts.LoadFlags)
	}
	if len(opts.RenderModes) != 2 || opts.RenderModes[1].Name != "normal" {
		t.Fatalf("RenderModes = %+v", opts.RenderModes)
	}
	if !opts.IncludeBitmapBuffer {
		t.Fatalf("IncludeBitmapBuffer = false, want true")
	}
}

func TestReadDumpRequestFlexibleGlyphs(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(requestPath, []byte(`{
		"description": "local request fixture",
		"font": "fixture.ttf",
		"glyphs": ["0", "3-4", "7"],
		"glyph_ranges": ["10-11"]
	}`), 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}

	request, err := readDumpRequest(requestPath)
	if err != nil {
		t.Fatalf("readDumpRequest failed: %v", err)
	}
	if got, want := []int(request.Glyphs), []int{0, 3, 4, 7}; !slices.Equal(got, want) {
		t.Fatalf("Glyphs = %v, want %v", got, want)
	}
	if got, want := []string(request.GlyphRanges), []string{"10-11"}; !slices.Equal(got, want) {
		t.Fatalf("GlyphRanges = %v, want %v", got, want)
	}
}

func TestReadDumpRequestExpectedGaps(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "request.json")
	if err := os.WriteFile(requestPath, []byte(`{
		"description": "request with known expected gaps",
		"expected_gaps": [
			{"kind": "go render unsupported", "note": "rendering not ported yet"},
			{"path": "sizes[16x16/no-hinting/render-normal]*"}
		]
	}`), 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}

	request, err := readDumpRequest(requestPath)
	if err != nil {
		t.Fatalf("readDumpRequest failed: %v", err)
	}
	if len(request.ExpectedGaps) != 2 {
		t.Fatalf("ExpectedGaps = %+v, want 2 entries", request.ExpectedGaps)
	}
	if request.ExpectedGaps[0].Kind != "go render unsupported" {
		t.Fatalf("ExpectedGaps[0] = %+v", request.ExpectedGaps[0])
	}
	if request.ExpectedGaps[1].Path == "" {
		t.Fatalf("ExpectedGaps[1] missing path: %+v", request.ExpectedGaps[1])
	}
}

func TestCheckedInConformanceRequestsParse(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "conformance", "*.json"))
	if err != nil {
		t.Fatalf("glob requests: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no checked-in conformance requests found")
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			base := filepath.Base(path)
			request, err := readDumpRequest(path)
			if err != nil {
				t.Fatalf("readDumpRequest failed: %v", err)
			}
			if !strings.HasSuffix(base, "-request.json") {
				t.Fatalf("checked-in request fixture %q must use the -request.json suffix", base)
			}
			if strings.TrimSpace(request.Description) == "" {
				t.Fatal("description is required for checked-in request fixtures")
			}
			if strings.TrimSpace(request.Corpus) == "" {
				t.Fatal("corpus is required for checked-in request fixtures")
			}
			if wantCorpus := strings.TrimSuffix(base, "-request.json"); request.Corpus != wantCorpus {
				t.Fatalf("corpus = %q, want %q from fixture filename", request.Corpus, wantCorpus)
			}
			if len(request.PPEM) == 0 {
				t.Fatal("ppem is required for checked-in request fixtures")
			}
			if len(request.Glyphs) == 0 && len(request.GlyphRanges) == 0 && len(request.Chars) == 0 {
				t.Fatal("at least one glyph, glyph range, or char selection is required")
			}
			for i, gap := range request.ExpectedGaps {
				if strings.TrimSpace(gap.Note) == "" {
					t.Fatalf("expected_gaps[%d] must explain the measured gap", i)
				}
			}
		})
	}
}

func TestDiscoverFontFiles(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	fontA := filepath.Join(dir, "A.ttf")
	fontB := filepath.Join(nested, "B.OTF")
	fontC := filepath.Join(dir, "Z.ttc")
	for _, path := range []string{fontB, fontA, fontC, filepath.Join(dir, "notes.txt")} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	got, err := discoverFontFiles([]string{fontC, filepath.Join(dir, "notes.txt")}, []string{dir, dir, filepath.Join(dir, "missing")}, 10)
	if err != nil {
		t.Fatalf("discoverFontFiles failed: %v", err)
	}
	want := []string{fontA, fontB, fontC}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("fonts = %v, want %v", got, want)
	}

	limited, err := discoverFontFiles(nil, []string{dir}, 1)
	if err != nil {
		t.Fatalf("discoverFontFiles limited failed: %v", err)
	}
	if !slices.Equal(limited, []string{fontA}) {
		t.Fatalf("limited fonts = %v, want [%s]", limited, fontA)
	}
}

func TestRunSmokeCommandSkipsWhenNoFontsFound(t *testing.T) {
	var out bytes.Buffer
	err := runSmokeCommand([]string{
		"-font-dir", t.TempDir(),
		"-out-dir", filepath.Join(t.TempDir(), "out"),
	}, &out, os.Stderr)
	if err != nil {
		t.Fatalf("runSmokeCommand failed: %v", err)
	}
	if !strings.Contains(out.String(), "SKIP (no local system fonts found") {
		t.Fatalf("output missing skip message:\n%s", out.String())
	}
}

func TestRunGoDumpCommandWritesFixtureDump(t *testing.T) {
	dir := t.TempDir()
	fontPath := writeConformanceFixtureFont(t, dir)
	outputPath := filepath.Join(dir, "dump.json")

	var out bytes.Buffer
	err := runGoDumpCommand([]string{
		"-font", fontPath,
		"-out", outputPath,
		"-ppem", "12",
		"-glyphs", "0",
		"-load-flags", "no-hinting",
		"-render-mode", "none,normal",
		"-corpus", "fixture-cli",
	}, &out, os.Stderr)
	if err != nil {
		t.Fatalf("runGoDumpCommand failed: %v\n%s", err, out.String())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty when writing -out file", out.String())
	}

	dump, err := readDump(outputPath)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	if dump.Engine.Name != "go-freetype" || dump.Request.Corpus != "fixture-cli" {
		t.Fatalf("dump identity = engine %q corpus %q", dump.Engine.Name, dump.Request.Corpus)
	}
	if dump.Source.Path != fontPath || dump.Source.Size == 0 || dump.Source.SHA256 == "" {
		t.Fatalf("source info = %+v", dump.Source)
	}
	if dump.Face.NumFaces != 1 || dump.Face.NumGlyphs != 1 || dump.Face.UnitsPerEm != 1000 {
		t.Fatalf("face info = %+v", dump.Face)
	}
	if len(dump.Sizes) != 2 {
		t.Fatalf("size count = %d, want 2", len(dump.Sizes))
	}
	loaded := dump.Sizes[0].Glyphs[0]
	if !loaded.Metrics.Available || loaded.SlotMetrics == nil || !loaded.SlotMetrics.Available {
		t.Fatalf("glyph metrics missing: %+v slot=%+v", loaded.Metrics, loaded.SlotMetrics)
	}
	if !loaded.Outline.Available || loaded.Outline.PointCount != 4 || loaded.Outline.BBox == nil {
		t.Fatalf("outline missing or incomplete: %+v", loaded.Outline)
	}

	rendered := dump.Sizes[1].Glyphs[0]
	if dump.Sizes[1].RenderMode != "normal" {
		t.Fatalf("render mode = %q, want normal", dump.Sizes[1].RenderMode)
	}
	if !rendered.Bitmap.Available || rendered.Bitmap.BufferSize == 0 || rendered.Bitmap.SHA256 == "" {
		t.Fatalf("rendered bitmap missing: %+v", rendered.Bitmap)
	}
	if rendered.RenderedFormat != "bitmap" {
		t.Fatalf("rendered format = %q, want bitmap", rendered.RenderedFormat)
	}
}

func TestRunBatchCommandWritesFixtureDump(t *testing.T) {
	dir := t.TempDir()
	fontPath := writeConformanceFixtureFont(t, dir)
	requestPath := filepath.Join(dir, "fixture-request.json")
	if err := os.WriteFile(requestPath, []byte(fmt.Sprintf(`{
		"font": %q,
		"corpus": "fixture-batch",
		"ppem": "10",
		"glyphs": "0",
		"load_flags": "no-hinting",
		"render_mode": "none"
	}`, fontPath)), 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}

	outputDir := filepath.Join(dir, "out")
	var out bytes.Buffer
	err := runBatchCommand([]string{
		"-request", requestPath,
		"-out-dir", outputDir,
	}, &out, os.Stderr)
	if err != nil {
		t.Fatalf("runBatchCommand failed: %v\n%s", err, out.String())
	}
	text := out.String()
	if !strings.Contains(text, "PASS") {
		t.Fatalf("batch output missing PASS:\n%s", text)
	}

	dump, err := readDump(filepath.Join(outputDir, "fixture-request.go.json"))
	if err != nil {
		t.Fatalf("read batch dump: %v", err)
	}
	if dump.Request.RequestPath != requestPath || dump.Request.Corpus != "fixture-batch" {
		t.Fatalf("request info = %+v", dump.Request)
	}
	if len(dump.Sizes) != 1 || len(dump.Sizes[0].Glyphs) != 1 || !dump.Sizes[0].Glyphs[0].Outline.Available {
		t.Fatalf("batch dump glyphs = %+v", dump.Sizes)
	}
}

func TestExpandRequestPaths(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.json")
	second := filepath.Join(dir, "b.json")
	for _, path := range []string{second, first} {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write request %s: %v", path, err)
		}
	}

	got, err := expandRequestPaths([]string{filepath.Join(dir, "*.json"), first})
	if err != nil {
		t.Fatalf("expandRequestPaths failed: %v", err)
	}
	want := []string{first, second}
	if !slices.Equal(got, want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
}

func TestCompareDumpsPointTolerance(t *testing.T) {
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate := minimalDumpWithPoint(Point{X: 101, Y: 198})

	noTolerance := compareDumps(reference, candidate, compareOptions{})
	if len(noTolerance) != 2 {
		t.Fatalf("diff count without tolerance = %d, want 2", len(noTolerance))
	}

	withTolerance := compareDumps(reference, candidate, compareOptions{PointTolerance: 2})
	if len(withTolerance) != 0 {
		t.Fatalf("diff count with tolerance = %d, want 0: %+v", len(withTolerance), withTolerance)
	}
}

func TestBatchCompareReportsKnownGaps(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "render-gap.json")
	if err := os.WriteFile(requestPath, []byte(`{"description":"batch compare key"}`), 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}

	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].RenderMode = "normal"
	reference.Sizes[0].Glyphs[0].RenderedFormat = "bitmap"
	reference.Sizes[0].Glyphs[0].Bitmap = BitmapRecord{Available: true, Rows: 1, Width: 1, Pitch: 1, PixelModeName: "gray", BufferSize: 1, SHA256: "ref"}
	reference.Sizes[0].Glyphs[0].SlotMetrics = &SlotMetricsRecord{Available: true, Width: 20}

	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].RenderMode = "normal"
	candidate.Sizes[0].Glyphs[0].RenderedFormat = "outline"
	candidate.Sizes[0].Glyphs[0].RenderError = "render mode normal unsupported by Go conformance dumper"
	candidate.Sizes[0].Glyphs[0].SlotMetrics = &SlotMetricsRecord{Available: false, Error: "glyph slot metrics unavailable"}

	if err := writeDump(reference, filepath.Join(dir, "render-gap.freetype.json"), ioDiscard{}); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := writeDump(candidate, filepath.Join(dir, "render-gap.go.json"), ioDiscard{}); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	var out bytes.Buffer
	err := runBatchCompareCommand([]string{
		"-request", requestPath,
		"-reference-dir", dir,
		"-candidate-dir", dir,
		"-max-diffs", "10",
	}, &out, os.Stderr)
	if !errors.Is(err, errComparisonFailed) {
		t.Fatalf("err = %v, want errComparisonFailed", err)
	}
	text := out.String()
	for _, want := range []string{"known conformance gaps", "go render unsupported", "go render bitmap missing", "go slot metrics unavailable"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestBatchCompareAcceptsExpectedGaps(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "render-gap.json")
	if err := os.WriteFile(requestPath, []byte(`{
		"description": "batch compare key",
		"expected_gaps": [
			{"kind": "go render unsupported"},
			{"kind": "go render bitmap missing"},
			{"kind": "go render output mismatch"},
			{"kind": "go slot metrics unavailable"}
		]
	}`), 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}

	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].RenderMode = "normal"
	reference.Sizes[0].Glyphs[0].RenderedFormat = "bitmap"
	reference.Sizes[0].Glyphs[0].Bitmap = BitmapRecord{Available: true, Rows: 1, Width: 1, Pitch: 1, PixelModeName: "gray", BufferSize: 1, SHA256: "ref"}
	reference.Sizes[0].Glyphs[0].SlotMetrics = &SlotMetricsRecord{Available: true, Width: 20}

	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].RenderMode = "normal"
	candidate.Sizes[0].Glyphs[0].RenderedFormat = "outline"
	candidate.Sizes[0].Glyphs[0].RenderError = "render mode normal unsupported by Go conformance dumper"
	candidate.Sizes[0].Glyphs[0].SlotMetrics = &SlotMetricsRecord{Available: false, Error: "glyph slot metrics unavailable"}

	if err := writeDump(reference, filepath.Join(dir, "render-gap.freetype.json"), ioDiscard{}); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := writeDump(candidate, filepath.Join(dir, "render-gap.go.json"), ioDiscard{}); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	var out bytes.Buffer
	err := runBatchCompareCommand([]string{
		"-request", requestPath,
		"-reference-dir", dir,
		"-candidate-dir", dir,
		"-accept-expected-gaps",
	}, &out, os.Stderr)
	if err != nil {
		t.Fatalf("runBatchCompareCommand failed: %v\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{"PASS with expected gaps", "expected known gaps", "conformance batch summary", "conformance batch compare: PASS"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestBatchCompareSummaryReportsMetricDeltaDetails(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "metric-delta.json")
	if err := os.WriteFile(requestPath, []byte(`{"description":"metric delta summary"}`), 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}

	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].Glyphs[0].Metrics.Advance = 64
	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].Glyphs[0].Metrics.Advance = 68

	if err := writeDump(reference, filepath.Join(dir, "metric-delta.freetype.json"), ioDiscard{}); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := writeDump(candidate, filepath.Join(dir, "metric-delta.go.json"), ioDiscard{}); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	var out bytes.Buffer
	err := runBatchCompareCommand([]string{
		"-request", requestPath,
		"-reference-dir", dir,
		"-candidate-dir", dir,
	}, &out, os.Stderr)
	if !errors.Is(err, errComparisonFailed) {
		t.Fatalf("err = %v, want errComparisonFailed\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{
		"conformance batch summary",
		"metric delta details",
		"glyph metrics: 1 fields, max_abs_delta=4",
		"unexpected diff kinds",
		"glyph metric delta",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestBatchCompareSummaryReportsUnexpectedLoadFlagBuckets(t *testing.T) {
	dir := t.TempDir()
	requestPath := filepath.Join(dir, "outline-buckets.json")
	if err := os.WriteFile(requestPath, []byte(`{"description":"outline bucket summary"}`), 0o600); err != nil {
		t.Fatalf("write request: %v", err)
	}

	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes = []SizeDump{
		{
			PPEMX:     12,
			PPEMY:     12,
			LoadFlags: "default",
			Glyphs:    []GlyphRecord{reference.Sizes[0].Glyphs[0]},
		},
		{
			PPEMX:     12,
			PPEMY:     12,
			LoadFlags: "no-hinting",
			Glyphs:    []GlyphRecord{reference.Sizes[0].Glyphs[0]},
		},
		{
			PPEMX:      12,
			PPEMY:      12,
			LoadFlags:  "no-hinting",
			RenderMode: "normal",
			Glyphs:     []GlyphRecord{reference.Sizes[0].Glyphs[0]},
		},
	}

	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes = []SizeDump{
		{
			PPEMX:     12,
			PPEMY:     12,
			LoadFlags: "default",
			Glyphs:    []GlyphRecord{candidate.Sizes[0].Glyphs[0]},
		},
		{
			PPEMX:     12,
			PPEMY:     12,
			LoadFlags: "no-hinting",
			Glyphs:    []GlyphRecord{candidate.Sizes[0].Glyphs[0]},
		},
		{
			PPEMX:      12,
			PPEMY:      12,
			LoadFlags:  "no-hinting",
			RenderMode: "normal",
			Glyphs:     []GlyphRecord{candidate.Sizes[0].Glyphs[0]},
		},
	}
	candidate.Sizes[0].Glyphs[0].Metrics.Advance = 12
	candidate.Sizes[1].Glyphs[0].Metrics.Advance = 13
	candidate.Sizes[2].Glyphs[0].Metrics.Advance = 14

	if err := writeDump(reference, filepath.Join(dir, "outline-buckets.freetype.json"), ioDiscard{}); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := writeDump(candidate, filepath.Join(dir, "outline-buckets.go.json"), ioDiscard{}); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	var out bytes.Buffer
	err := runBatchCompareCommand([]string{
		"-request", requestPath,
		"-reference-dir", dir,
		"-candidate-dir", dir,
	}, &out, os.Stderr)
	if !errors.Is(err, errComparisonFailed) {
		t.Fatalf("err = %v, want errComparisonFailed\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{
		"unexpected diff size/load/render keys",
		"12x12/default: 1",
		"12x12/no-hinting: 1",
		"12x12/no-hinting/render-normal: 1",
		"unexpected diff load flags",
		"no-hinting: 2",
		"default: 1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestCompareReportsStaleExpectedGaps(t *testing.T) {
	dir := t.TempDir()
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Request.ExpectedGaps = []ExpectedGap{{Kind: "go render output mismatch", Note: "should be removed after parity lands"}}

	refPath := filepath.Join(dir, "ref.json")
	candPath := filepath.Join(dir, "cand.json")
	if err := writeDump(reference, refPath, ioDiscard{}); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := writeDump(candidate, candPath, ioDiscard{}); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	var out bytes.Buffer
	err := runCompareCommand([]string{
		"-reference", refPath,
		"-candidate", candPath,
		"-fail-stale-expected-gaps",
	}, &out, os.Stderr)
	if !errors.Is(err, errComparisonFailed) {
		t.Fatalf("err = %v, want errComparisonFailed\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "stale expected gaps") {
		t.Fatalf("output missing stale expected gaps:\n%s", out.String())
	}
}

func TestCompareReportsRenderBitmapPlacementMismatch(t *testing.T) {
	dir := t.TempDir()
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].RenderMode = "normal"
	reference.Sizes[0].Glyphs[0].Bitmap = BitmapRecord{
		Available:     true,
		Rows:          2,
		Width:         2,
		Pitch:         2,
		PixelModeName: "gray",
		Left:          1,
		Top:           7,
		BufferSize:    4,
		SHA256:        "same",
	}

	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].RenderMode = "normal"
	candidate.Sizes[0].Glyphs[0].Bitmap = reference.Sizes[0].Glyphs[0].Bitmap
	candidate.Sizes[0].Glyphs[0].Bitmap.Left = 2
	candidate.Sizes[0].Glyphs[0].Bitmap.Top = 6

	refPath := filepath.Join(dir, "ref.json")
	candPath := filepath.Join(dir, "cand.json")
	if err := writeDump(reference, refPath, ioDiscard{}); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := writeDump(candidate, candPath, ioDiscard{}); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	var out bytes.Buffer
	err := runCompareCommand([]string{"-reference", refPath, "-candidate", candPath}, &out, os.Stderr)
	if !errors.Is(err, errComparisonFailed) {
		t.Fatalf("err = %v, want errComparisonFailed\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{"go render output mismatch", "render mismatch details", "bitmap placement"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestCompareReportsBitmapBufferByteDiff(t *testing.T) {
	dir := t.TempDir()
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].RenderMode = "normal"
	reference.Sizes[0].Glyphs[0].Bitmap = BitmapRecord{
		Available:     true,
		Rows:          2,
		Width:         3,
		Pitch:         3,
		PixelModeName: "gray",
		BufferSize:    6,
		SHA256:        "ref",
		BufferHex:     "010203040506",
	}

	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].RenderMode = "normal"
	candidate.Sizes[0].Glyphs[0].Bitmap = reference.Sizes[0].Glyphs[0].Bitmap
	candidate.Sizes[0].Glyphs[0].Bitmap.SHA256 = "cand"
	candidate.Sizes[0].Glyphs[0].Bitmap.BufferHex = "010209040809"

	refPath := filepath.Join(dir, "ref.json")
	candPath := filepath.Join(dir, "cand.json")
	if err := writeDump(reference, refPath, ioDiscard{}); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := writeDump(candidate, candPath, ioDiscard{}); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	var out bytes.Buffer
	err := runCompareCommand([]string{"-reference", refPath, "-candidate", candPath}, &out, os.Stderr)
	if !errors.Is(err, errComparisonFailed) {
		t.Fatalf("err = %v, want errComparisonFailed\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{
		"bitmap buffer byte diff",
		"first_diff_offset=2",
		"byte_diff_count=3",
		"row_diff_rows=2",
		"row=0 count=1 first_col=2",
		"row=1 count=2 first_col=1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestCompareDumpsSlotMetricsAndBBox(t *testing.T) {
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].Glyphs[0].SlotMetrics = &SlotMetricsRecord{Available: true, Width: 20, HoriAdvance: 64}
	reference.Sizes[0].Glyphs[0].Outline.BBox = &BBox{XMin: 100, YMin: 200, XMax: 100, YMax: 200}

	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].Glyphs[0].SlotMetrics = &SlotMetricsRecord{Available: true, Width: 21, HoriAdvance: 64}
	candidate.Sizes[0].Glyphs[0].Outline.BBox = &BBox{XMin: 101, YMin: 200, XMax: 100, YMax: 200}

	diffs := compareDumps(reference, candidate, compareOptions{})
	if len(diffs) != 2 {
		t.Fatalf("diff count = %d, want 2: %+v", len(diffs), diffs)
	}
}

func TestCompareDumpsType1SegmentsEqual(t *testing.T) {
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].Glyphs[0].Type1Segments = conformanceType1SegmentRecords()
	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].Glyphs[0].Type1Segments = conformanceType1SegmentRecords()

	if diffs := compareDumps(reference, candidate, compareOptions{}); len(diffs) != 0 {
		t.Fatalf("unexpected diffs: %+v", diffs)
	}
}

func TestCompareDumpsType1SegmentsMissingCandidate(t *testing.T) {
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].Glyphs[0].Type1Segments = conformanceType1SegmentRecords()
	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})

	diffs := compareDumps(reference, candidate, compareOptions{})
	if len(diffs) != 1 {
		t.Fatalf("diff count = %d, want 1: %+v", len(diffs), diffs)
	}
	if got, want := diffs[0].Path, "sizes[12x12/no-hinting].glyphs[0].type1_segments"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if diffs[0].Want != "present" || diffs[0].Got != "missing" {
		t.Fatalf("diff = %+v, want present/missing", diffs[0])
	}
	if got, want := diffs[0].Kind, "type1 segment delta"; got != want {
		t.Fatalf("kind = %q, want %q", got, want)
	}
}

func TestCompareAcceptsType1SegmentExpectedGapByPathAndKind(t *testing.T) {
	dir := t.TempDir()
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].Glyphs[0].Type1Segments = conformanceType1SegmentRecords()
	reference.Request.ExpectedGaps = []ExpectedGap{{
		Kind: "type1 segment delta",
		Path: "sizes[12x12/no-hinting].glyphs[0].type1_segments",
	}}
	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})

	refPath := filepath.Join(dir, "ref.json")
	candPath := filepath.Join(dir, "cand.json")
	if err := writeDump(reference, refPath, ioDiscard{}); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := writeDump(candidate, candPath, ioDiscard{}); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	var out bytes.Buffer
	err := runCompareCommand([]string{"-reference", refPath, "-candidate", candPath, "-accept-expected-gaps"}, &out, os.Stderr)
	if err != nil {
		t.Fatalf("runCompareCommand failed: %v\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{"PASS with expected gaps", "type1 segment delta"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestCompareDumpsType1SegmentsKindUnitAndPointMismatch(t *testing.T) {
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].Glyphs[0].Type1Segments = conformanceType1SegmentRecords()
	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].Glyphs[0].Type1Segments = conformanceType1SegmentRecords()
	candidate.Sizes[0].Glyphs[0].Type1Segments[0].Kind = "line"
	candidate.Sizes[0].Glyphs[0].Type1Segments[0].Units = "raw design units"
	candidate.Sizes[0].Glyphs[0].Type1Segments[0].Points[0].X = 641

	diffs := compareDumps(reference, candidate, compareOptions{})
	if len(diffs) != 3 {
		t.Fatalf("diff count = %d, want 3: %+v", len(diffs), diffs)
	}
	want := map[string]comparisonDiff{
		`sizes[12x12/no-hinting].glyphs[0].type1_segments[0].kind`: {
			Want: `"move"`,
			Got:  `"line"`,
		},
		`sizes[12x12/no-hinting].glyphs[0].type1_segments[0].units`: {
			Want: `"26.6 design units"`,
			Got:  `"raw design units"`,
		},
		`sizes[12x12/no-hinting].glyphs[0].type1_segments[0].points[0].x`: {
			Want: "640",
			Got:  "641",
		},
	}
	for _, diff := range diffs {
		wantDiff, ok := want[diff.Path]
		if !ok {
			t.Fatalf("unexpected diff path %q in %+v", diff.Path, diffs)
		}
		if diff.Want != wantDiff.Want || diff.Got != wantDiff.Got {
			t.Fatalf("diff for %s = want %s got %s, want %s/%s", diff.Path, diff.Want, diff.Got, wantDiff.Want, wantDiff.Got)
		}
		delete(want, diff.Path)
	}
	if len(want) != 0 {
		t.Fatalf("missing expected diffs: %+v", want)
	}
}

func TestExpectedGapMatchesType1SegmentPathAndKindTogether(t *testing.T) {
	diff := newComparisonDiff("sizes[12x12/no-hinting].glyphs[0].type1_segments[0].points[0].x", "640", "641")
	if got, want := diff.Kind, "type1 segment delta"; got != want {
		t.Fatalf("kind = %q, want %q", got, want)
	}

	gap := ExpectedGap{
		Kind: "type1 segment delta",
		Path: "sizes[12x12/no-hinting].glyphs[0].type1_segments*",
	}
	if !expectedGapMatches(gap, diff) {
		t.Fatalf("expected gap did not match diff %+v", diff)
	}

	wrongKind := gap
	wrongKind.Kind = "glyph metric delta"
	if expectedGapMatches(wrongKind, diff) {
		t.Fatalf("expected gap with wrong kind matched diff %+v", diff)
	}

	wrongPath := gap
	wrongPath.Path = "sizes[12x12/no-hinting].glyphs[1].type1_segments*"
	if expectedGapMatches(wrongPath, diff) {
		t.Fatalf("expected gap with wrong path matched diff %+v", diff)
	}
}

func TestCompareDumpsIgnoresCandidateType1SegmentsWhenReferenceOmitsThem(t *testing.T) {
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].Glyphs[0].Type1Segments = conformanceType1SegmentRecords()

	if diffs := compareDumps(reference, candidate, compareOptions{}); len(diffs) != 0 {
		t.Fatalf("unexpected diffs: %+v", diffs)
	}
}

func TestComparePassesWhenReferenceJSONOmitsType1Segments(t *testing.T) {
	dir := t.TempDir()
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].Glyphs[0].Type1Segments = conformanceType1SegmentRecords()

	refPath := filepath.Join(dir, "ref.json")
	candPath := filepath.Join(dir, "cand.json")
	if err := writeDump(reference, refPath, ioDiscard{}); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := writeDump(candidate, candPath, ioDiscard{}); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	var out bytes.Buffer
	err := runCompareCommand([]string{"-reference", refPath, "-candidate", candPath}, &out, os.Stderr)
	if err != nil {
		t.Fatalf("runCompareCommand failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "conformance compare: PASS") {
		t.Fatalf("output missing PASS:\n%s", out.String())
	}
}

func TestCompareReportsMetricDeltaDetails(t *testing.T) {
	dir := t.TempDir()
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].Glyphs[0].Metrics.Advance = 64
	reference.Sizes[0].Glyphs[0].Metrics.LSB = -2
	reference.Sizes[0].Glyphs[0].SlotMetrics = &SlotMetricsRecord{Available: true, Width: 20, HoriAdvance: 64}
	reference.Sizes[0].Glyphs[0].Outline.BBox = &BBox{XMin: 100, YMin: 200, XMax: 160, YMax: 260}

	candidate := minimalDumpWithPoint(Point{X: 103, Y: 196})
	candidate.Sizes[0].Glyphs[0].Metrics.Advance = 66
	candidate.Sizes[0].Glyphs[0].Metrics.LSB = -5
	candidate.Sizes[0].Glyphs[0].SlotMetrics = &SlotMetricsRecord{Available: true, Width: 25, HoriAdvance: 64}
	candidate.Sizes[0].Glyphs[0].Outline.BBox = &BBox{XMin: 99, YMin: 200, XMax: 160, YMax: 260}

	refPath := filepath.Join(dir, "ref.json")
	candPath := filepath.Join(dir, "cand.json")
	if err := writeDump(reference, refPath, ioDiscard{}); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := writeDump(candidate, candPath, ioDiscard{}); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	var out bytes.Buffer
	err := runCompareCommand([]string{"-reference", refPath, "-candidate", candPath}, &out, os.Stderr)
	if !errors.Is(err, errComparisonFailed) {
		t.Fatalf("err = %v, want errComparisonFailed\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{
		"metric delta details",
		"glyph metrics: 2 fields, max_abs_delta=3",
		"slot metrics: 1 fields, max_abs_delta=5",
		"outline bbox: 1 fields, max_abs_delta=1",
		"outline points: 2 fields, max_abs_delta=4",
		"glyph metric delta",
		"slot metric delta",
		"outline bbox delta",
		"outline point delta",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestCompareReportsOutlinePointTagDetails(t *testing.T) {
	dir := t.TempDir()
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].Glyphs[0].Outline = OutlineRecord{
		Available:    true,
		PointCount:   3,
		ContourCount: 1,
		Points: []Point{
			{X: 100, Y: 200},
			{X: 300, Y: 400},
			{X: 500, Y: 600},
		},
		Tags:     []int{1, 0, 1},
		Contours: []int{2},
	}

	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].Glyphs[0].Outline = OutlineRecord{
		Available:    true,
		PointCount:   3,
		ContourCount: 1,
		Points: []Point{
			{X: 103, Y: 196},
			{X: 300, Y: 400},
			{X: 505, Y: 600},
		},
		Tags:     []int{1, 1, 0},
		Contours: []int{2},
	}

	refPath := filepath.Join(dir, "ref.json")
	candPath := filepath.Join(dir, "cand.json")
	if err := writeDump(reference, refPath, ioDiscard{}); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := writeDump(candidate, candPath, ioDiscard{}); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	var out bytes.Buffer
	err := runCompareCommand([]string{"-reference", refPath, "-candidate", candPath}, &out, os.Stderr)
	if !errors.Is(err, errComparisonFailed) {
		t.Fatalf("err = %v, want errComparisonFailed\n%s", err, out.String())
	}
	text := out.String()
	for _, want := range []string{
		"outline point/tag details",
		"sizes[12x12/no-hinting].glyphs[0]: coordinate-only_points=1, coordinate+tag_points=1, tag-only_points=1, coordinate_fields=3, max_abs_delta=5, tag_deltas=2, tag_transitions=[0->1,1->0]",
		"point[0]: coordinates axes=x,y fields=2 max_abs_delta=4",
		"point[1]: tag-only tag 0->1",
		"point[2]: coordinate+tag axes=x fields=1 max_abs_delta=5 tag 1->0",
		"outline point delta",
		"outline tag delta",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

func TestLoadFlagsForRenderModeOverridesTarget(t *testing.T) {
	flags := loadFlagsForRenderMode(api.LoadNoHinting|api.LoadTargetMono, api.RenderModeLCDV)
	if flags&api.LoadRender == 0 {
		t.Fatalf("LoadRender missing from %#x", flags)
	}
	if flags&api.LoadNoHinting == 0 {
		t.Fatalf("LoadNoHinting was cleared from %#x", flags)
	}
	if got := flags & api.LoadTargetMask; got != api.LoadTargetLCDV {
		t.Fatalf("target = %#x, want %#x", got, api.LoadTargetLCDV)
	}
}

func TestDumpGoGlyphRecordsMetricsOutlineBitmapAndImage(t *testing.T) {
	face := &conformanceFakeFace{
		slot: &conformanceFakeSlot{
			outline: conformanceFakeOutline{
				points:   []api.Vector{{X: 1, Y: 2}, {X: 5, Y: 9}, {X: 11, Y: 3}, {X: 90, Y: 91}, {X: 92, Y: 93}},
				tags:     []byte{1, 1, 1, 0, 0},
				contours: []int{2},
			},
			bitmap:  conformanceFakeBitmap{rows: 2, width: 3, pitch: 3, pixelMode: api.MODE_GRAY, buffer: []byte{1, 2, 3, 4, 5, 6}, left: -1, top: 7},
			image:   &api.Image{Width: 2, Height: 1, Pixels: []byte{9, 8, 7, 6}},
			metrics: api.GlyphMetrics{Width: 30, Height: 20, HoriBearingX: -2, HoriBearingY: 18, HoriAdvance: 64},
			has:     true,
		},
		advance: 70,
		lsb:     -3,
	}
	record := dumpGoGlyph(face, glyphSelection{GlyphIndex: 4, Chars: []string{"U+0041"}}, loadFlagSpec{Name: "no-hinting", Value: api.LoadNoHinting}, renderModeSpec{Name: "lcd-v", Value: api.RenderModeLCDV}, true)

	wantFlags := api.LoadRender | api.LoadNoHinting | api.LoadTargetLCDV
	if face.loadedGlyph != 4 || face.loadedFlags != wantFlags {
		t.Fatalf("LoadGlyph called glyph=%d flags=%#x, want glyph=4 flags=%#x", face.loadedGlyph, face.loadedFlags, wantFlags)
	}
	if !record.Metrics.Available || record.Metrics.Advance != 70 || record.Metrics.LSB != -3 {
		t.Fatalf("metrics = %+v", record.Metrics)
	}
	if record.SlotMetrics == nil || !record.SlotMetrics.Available || record.SlotMetrics.HoriAdvance != 64 {
		t.Fatalf("slot metrics = %+v", record.SlotMetrics)
	}
	if !record.Outline.Available || record.Outline.PointCount != 3 || record.Outline.PhantomPointCount != 2 || record.Outline.BBox.XMax != 11 {
		t.Fatalf("outline = %+v", record.Outline)
	}
	bitmapHash := sha256.Sum256([]byte{1, 2, 3, 4, 5, 6})
	if !record.Bitmap.Available || record.Bitmap.Left != -1 || record.Bitmap.Top != 7 || record.Bitmap.SHA256 != fmt.Sprintf("%x", bitmapHash) || record.Bitmap.BufferHex != "010203040506" {
		t.Fatalf("bitmap = %+v", record.Bitmap)
	}
	imageHash := sha256.Sum256([]byte{9, 8, 7, 6})
	if record.Image == nil || !record.Image.Available || record.Image.SHA256 != fmt.Sprintf("%x", imageHash) {
		t.Fatalf("image = %+v", record.Image)
	}
	if record.Format != "outline" || record.RenderedFormat != "bitmap" || record.RenderError != "" {
		t.Fatalf("formats = format %q rendered %q render error %q", record.Format, record.RenderedFormat, record.RenderError)
	}
}

func TestDumpGoGlyphReportsLoadAndRenderFailures(t *testing.T) {
	loadErr := errors.New("fixture load failed")
	record := dumpGoGlyph(&conformanceFakeFace{loadErr: loadErr}, glyphSelection{GlyphIndex: 9}, loadFlagSpec{Name: "default"}, renderModeSpec{Name: "none", Value: api.RenderModeNone}, false)
	if record.LoadError != loadErr.Error() || record.Metrics.Error != loadErr.Error() || record.Outline.Error != loadErr.Error() || record.Bitmap.Error != loadErr.Error() {
		t.Fatalf("load failure record = %+v", record)
	}

	record = dumpGoGlyph(&conformanceFakeFace{slot: &conformanceFakeSlot{}}, glyphSelection{GlyphIndex: 1}, loadFlagSpec{Name: "default"}, renderModeSpec{Name: "normal", Value: api.RenderModeNormal}, false)
	if record.RenderError == "" || record.Bitmap.Error != record.RenderError || record.RenderedFormat != "none" {
		t.Fatalf("render failure record = %+v", record)
	}
}

func TestCompareBitmapsTreatsPixelModeNameAsCanonical(t *testing.T) {
	reference := minimalDumpWithPoint(Point{X: 100, Y: 200})
	reference.Sizes[0].Glyphs[0].Bitmap = BitmapRecord{
		Available:     true,
		Rows:          2,
		Width:         6,
		Pitch:         6,
		PixelMode:     5,
		PixelModeName: "lcd",
		BufferSize:    12,
		SHA256:        "same",
	}

	candidate := minimalDumpWithPoint(Point{X: 100, Y: 200})
	candidate.Sizes[0].Glyphs[0].Bitmap = BitmapRecord{
		Available:     true,
		Rows:          2,
		Width:         6,
		Pitch:         6,
		PixelMode:     api.MODE_LCD,
		PixelModeName: "lcd",
		BufferSize:    12,
		SHA256:        "same",
	}

	if diffs := compareDumps(reference, candidate, compareOptions{}); len(diffs) != 0 {
		t.Fatalf("unexpected diffs: %+v", diffs)
	}
}

func TestGoRenderedGlyphFormatPrefersBitmapWhenRendered(t *testing.T) {
	record := GlyphRecord{
		Format:  "outline",
		Bitmap:  BitmapRecord{Available: true},
		Outline: OutlineRecord{Available: true},
	}
	if got := goRenderedGlyphFormat(record, api.RenderModeNormal); got != "bitmap" {
		t.Fatalf("rendered format = %q, want bitmap", got)
	}
	if got := goRenderedGlyphFormat(record, api.RenderModeNone); got != "outline" {
		t.Fatalf("non-rendered format = %q, want outline", got)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

func minimalDumpWithPoint(point Point) *Dump {
	return &Dump{
		Schema: dumpSchema,
		Source: SourceInfo{SHA256: "same"},
		Face:   FaceInfo{FaceIndex: 0, NumGlyphs: 1, UnitsPerEm: 1000},
		Sizes: []SizeDump{
			{
				PPEMX:     12,
				PPEMY:     12,
				LoadFlags: "no-hinting",
				Glyphs: []GlyphRecord{
					{
						GlyphIndex: 0,
						Metrics:    MetricsRecord{Available: true, Advance: 10, LSB: 1},
						Outline: OutlineRecord{
							Available:    true,
							PointCount:   1,
							ContourCount: 1,
							Points:       []Point{point},
							Tags:         []int{1},
							Contours:     []int{0},
						},
						Bitmap: BitmapRecord{Available: false, Error: "bitmap unavailable"},
					},
				},
			},
		},
	}
}

func conformanceType1SegmentRecords() []t1.SegmentRecord {
	return []t1.SegmentRecord{
		{
			Kind:  "move",
			Units: t1.CharStringSegmentUnits,
			Points: []t1.SegmentPoint{
				{X: 640, Y: 640},
			},
		},
		{
			Kind:  "line",
			Units: t1.CharStringSegmentUnits,
			Points: []t1.SegmentPoint{
				{X: 7040, Y: 640},
			},
		},
		{
			Kind:  "close",
			Units: t1.CharStringSegmentUnits,
		},
	}
}

type conformanceTestTable struct {
	tag  string
	data []byte
}

func writeConformanceFixtureFont(t *testing.T, dir string) string {
	t.Helper()

	glyph := conformanceSimpleRectGlyphData(0, 0, 10, 10)
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000)
	binary.BigEndian.PutUint16(head[50:52], 1)

	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:36], 1)

	hmtx := make([]byte, 4)
	binary.BigEndian.PutUint16(hmtx[0:2], 12)
	binary.BigEndian.PutUint16(hmtx[2:4], 0)

	loca := make([]byte, 8)
	binary.BigEndian.PutUint32(loca[4:8], uint32(len(glyph)))

	maxp := make([]byte, 32)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], 1)

	path := filepath.Join(dir, "fixture.ttf")
	data := buildConformanceSFNTTestData(t, []conformanceTestTable{
		{tag: "glyf", data: glyph},
		{tag: "head", data: head},
		{tag: "hhea", data: hhea},
		{tag: "hmtx", data: hmtx},
		{tag: "loca", data: loca},
		{tag: "maxp", data: maxp},
	})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture font: %v", err)
	}
	return path
}

func conformanceSimpleRectGlyphData(xMin, yMin, xMax, yMax int16) []byte {
	data := make([]byte, 34)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], uint16(xMin))
	binary.BigEndian.PutUint16(data[4:6], uint16(yMin))
	binary.BigEndian.PutUint16(data[6:8], uint16(xMax))
	binary.BigEndian.PutUint16(data[8:10], uint16(yMax))
	binary.BigEndian.PutUint16(data[10:12], 3)
	binary.BigEndian.PutUint16(data[12:14], 0)
	for i := 0; i < 4; i++ {
		data[14+i] = 0x01
	}
	offset := 18
	for _, dx := range []int16{xMin, xMax - xMin, 0, xMin - xMax} {
		binary.BigEndian.PutUint16(data[offset:offset+2], uint16(dx))
		offset += 2
	}
	for _, dy := range []int16{yMin, 0, yMax - yMin, 0} {
		binary.BigEndian.PutUint16(data[offset:offset+2], uint16(dy))
		offset += 2
	}
	return data
}

func buildConformanceSFNTTestData(t *testing.T, tables []conformanceTestTable) []byte {
	t.Helper()

	headerLen := align4ConformanceTest(uint32(12 + len(tables)*16))
	data := make([]byte, headerLen)
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], uint16(len(tables)))

	offset := headerLen
	for i, table := range tables {
		if len(table.tag) != 4 {
			t.Fatalf("table tag %q is not 4 bytes", table.tag)
		}
		offset = align4ConformanceTest(offset)
		for uint32(len(data)) < offset {
			data = append(data, 0)
		}
		tableOffset := offset
		data = append(data, table.data...)

		entry := 12 + i*16
		copy(data[entry:entry+4], table.tag)
		binary.BigEndian.PutUint32(data[entry+8:entry+12], tableOffset)
		binary.BigEndian.PutUint32(data[entry+12:entry+16], uint32(len(table.data)))
		offset = tableOffset + uint32(len(table.data))
	}
	return data
}

func align4ConformanceTest(v uint32) uint32 {
	return (v + 3) &^ 3
}

type conformanceFakeFace struct {
	slot        api.GlyphSlot
	loadErr     error
	advance     int32
	lsb         int32
	metricErr   error
	loadedGlyph int
	loadedFlags int
}

func (f *conformanceFakeFace) GetNumGlyphs() int { return 1 }

func (f *conformanceFakeFace) SetPixelSizes(width, height int) error { return nil }

func (f *conformanceFakeFace) LoadGlyph(glyphIndex int, loadFlags int) (api.GlyphSlot, error) {
	f.loadedGlyph = glyphIndex
	f.loadedFlags = loadFlags
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return f.slot, nil
}

func (f *conformanceFakeFace) GetGlyphSlot() api.GlyphSlot { return f.slot }

func (f *conformanceFakeFace) GetUnitsPerEm() uint16 { return 1000 }

func (f *conformanceFakeFace) GetGlyphIndex(char rune) (int, error) { return 0, nil }

func (f *conformanceFakeFace) GetGlyphMetrics(glyphIndex int) (int32, int32, error) {
	if f.metricErr != nil {
		return 0, 0, f.metricErr
	}
	return f.advance, f.lsb, nil
}

func (f *conformanceFakeFace) Shape(text string) ([]int, []api.Vector) { return nil, nil }

type conformanceFakeSlot struct {
	outline api.Outline
	bitmap  api.Bitmap
	image   *api.Image
	metrics api.GlyphMetrics
	has     bool
}

func (s *conformanceFakeSlot) GetOutline() api.Outline { return s.outline }

func (s *conformanceFakeSlot) SetOutline(outline api.Outline) { s.outline = outline }

func (s *conformanceFakeSlot) GetBitmap() api.Bitmap { return s.bitmap }

func (s *conformanceFakeSlot) GetImage() *api.Image { return s.image }

func (s *conformanceFakeSlot) GetMetrics() (api.GlyphMetrics, bool) {
	return s.metrics, s.has
}

type conformanceFakeOutline struct {
	points   []api.Vector
	tags     []byte
	contours []int
}

func (o conformanceFakeOutline) GetPoints() []api.Vector { return o.points }

func (o conformanceFakeOutline) GetTags() []byte { return o.tags }

func (o conformanceFakeOutline) GetContours() []int { return o.contours }

func (o conformanceFakeOutline) Scale(xScale, yScale int32) {}

func (o conformanceFakeOutline) Translate(x, y int32) {}

func (o conformanceFakeOutline) Transform(matrix *api.Matrix) {}

type conformanceFakeBitmap struct {
	rows      int
	width     int
	pitch     int
	pixelMode uint8
	buffer    []byte
	left      int
	top       int
}

func (b conformanceFakeBitmap) GetRows() int { return b.rows }

func (b conformanceFakeBitmap) GetWidth() int { return b.width }

func (b conformanceFakeBitmap) GetPitch() int { return b.pitch }

func (b conformanceFakeBitmap) GetBuffer() []byte { return b.buffer }

func (b conformanceFakeBitmap) GetPixelMode() uint8 { return b.pixelMode }

func (b conformanceFakeBitmap) SetPixelMode(mode uint8) {}

func (b conformanceFakeBitmap) GetLeft() int { return b.left }

func (b conformanceFakeBitmap) GetTop() int { return b.top }
