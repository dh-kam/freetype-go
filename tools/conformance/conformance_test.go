package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/api"
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
		"corpus": "smoke"
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
