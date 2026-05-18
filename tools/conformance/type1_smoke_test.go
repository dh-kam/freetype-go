package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
	t1 "github.com/dh-kam/freetype-go/type1"
)

func TestType1SmokeDumpGlyphFromMemoryPFA(t *testing.T) {
	pfa := type1SmokePFA()
	loaded, err := t1.NewLoader(core.NewSystem()).LoadFace(core.NewMemoryStream(pfa))
	if err != nil {
		t.Fatalf("load in-memory Type 1 PFA: %v", err)
	}

	opts := dumpOptions{
		Corpus:    "type1-smoke-memory",
		PPEMs:     []SizeSpec{{X: 20, Y: 20}},
		Chars:     []CharSpec{{Label: "U+0041", Rune: 'A'}},
		LoadFlags: []loadFlagSpec{{Name: "no-hinting", Value: api.LoadNoHinting}},
		RenderModes: []renderModeSpec{
			{Name: "none", Value: api.RenderModeNone},
		},
	}
	selections, charmap := resolveGoSelections(loaded, opts)
	sum := sha256.Sum256(pfa)
	dump := newDump("go-freetype", "", SourceInfo{
		Path:   "memory:type1-smoke.pfa",
		Size:   int64(len(pfa)),
		SHA256: fmt.Sprintf("%x", sum),
	}, opts, FaceInfo{
		FaceIndex:  0,
		NumFaces:   1,
		NumGlyphs:  loaded.GetNumGlyphs(),
		UnitsPerEm: loaded.GetUnitsPerEm(),
	}, charmap)

	for _, loadFlags := range opts.LoadFlags {
		for _, renderMode := range opts.RenderModes {
			for _, ppem := range opts.PPEMs {
				sizeDump := SizeDump{
					PPEMX:      ppem.X,
					PPEMY:      ppem.Y,
					LoadFlags:  loadFlags.Name,
					RenderMode: renderMode.Name,
				}
				if err := loaded.SetPixelSizes(ppem.X, ppem.Y); err != nil {
					t.Fatalf("set Type 1 ppem: %v", err)
				}
				for _, sel := range selections {
					sizeDump.Glyphs = append(sizeDump.Glyphs, dumpGoGlyph(loaded, sel, loadFlags, renderMode, false))
				}
				dump.Sizes = append(dump.Sizes, sizeDump)
			}
		}
	}

	if dump.Face.NumFaces != 1 || dump.Face.NumGlyphs != 2 || dump.Face.UnitsPerEm != 1000 {
		t.Fatalf("face info = %+v, want 1 face, 2 glyphs, 1000 upem", dump.Face)
	}
	if len(dump.Charmap) != 1 || dump.Charmap[0].Char != "U+0041" || dump.Charmap[0].GlyphIndex != 1 {
		t.Fatalf("charmap = %+v, want U+0041 -> glyph 1", dump.Charmap)
	}
	if len(dump.Sizes) != 1 || len(dump.Sizes[0].Glyphs) != 1 {
		t.Fatalf("dump sizes = %+v, want one size with one glyph", dump.Sizes)
	}

	glyph := dump.Sizes[0].Glyphs[0]
	if glyph.LoadError != "" {
		t.Fatalf("Type 1 glyph load error: %s", glyph.LoadError)
	}
	if glyph.GlyphIndex != 1 || len(glyph.Chars) != 1 || glyph.Chars[0] != "U+0041" {
		t.Fatalf("glyph selection = index %d chars %v, want glyph 1 for U+0041", glyph.GlyphIndex, glyph.Chars)
	}
	if !glyph.Metrics.Available || glyph.Metrics.Advance <= 0 {
		t.Fatalf("metrics missing or empty: %+v", glyph.Metrics)
	}
	if glyph.SlotMetrics == nil || !glyph.SlotMetrics.Available || glyph.SlotMetrics.HoriAdvance <= 0 {
		t.Fatalf("slot metrics missing or empty: %+v", glyph.SlotMetrics)
	}
	if !glyph.Outline.Available || glyph.Outline.PointCount != 4 || glyph.Outline.ContourCount != 1 || glyph.Outline.BBox == nil {
		t.Fatalf("outline missing or incomplete: %+v", glyph.Outline)
	}
	if glyph.Format != "outline" || glyph.RenderedFormat != "outline" {
		t.Fatalf("formats = %q/%q, want outline/outline", glyph.Format, glyph.RenderedFormat)
	}

	var encoded bytes.Buffer
	if err := writeDump(dump, "-", &encoded); err != nil {
		t.Fatalf("write Type 1 dump JSON: %v", err)
	}
	var roundTrip Dump
	if err := json.NewDecoder(&encoded).Decode(&roundTrip); err != nil {
		t.Fatalf("decode Type 1 dump JSON: %v", err)
	}
	if roundTrip.Request.Corpus != "type1-smoke-memory" || len(roundTrip.Sizes) != 1 {
		t.Fatalf("round-tripped dump request=%+v sizes=%d", roundTrip.Request, len(roundTrip.Sizes))
	}
}

func TestType1SmokeEndToEndRequestLoadsStandaloneType1(t *testing.T) {
	pfa := type1SmokePFA()
	tests := []struct {
		name string
		ext  string
		font []byte
	}{
		{name: "PFA", ext: ".pfa", font: pfa},
		{name: "PFB", ext: ".pfb", font: type1SmokePFB(pfa)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			fontPath := filepath.Join(dir, "type1-smoke"+tt.ext)
			if err := os.WriteFile(fontPath, tt.font, 0o600); err != nil {
				t.Fatalf("write Type 1 smoke font: %v", err)
			}
			requestPath := filepath.Join(dir, "type1-smoke-request.json")
			if err := os.WriteFile(requestPath, []byte(fmt.Sprintf(`{
				"description": "Generated minimal Type 1 smoke fixture.",
				"font": %q,
				"corpus": "type1-smoke",
				"ppem": "20",
				"chars": "U+0041",
				"load_flags": "no-hinting",
				"render_mode": "none"
			}`, fontPath)), 0o600); err != nil {
				t.Fatalf("write Type 1 smoke request: %v", err)
			}

			outputPath := filepath.Join(dir, "type1-smoke.go.json")
			var out bytes.Buffer
			err := runGoDumpCommand([]string{
				"-request", requestPath,
				"-out", outputPath,
			}, &out, &out)
			if err != nil {
				t.Fatalf("run Type 1 smoke dump: %v\n%s", err, out.String())
			}

			data, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("read Type 1 smoke dump: %v", err)
			}
			var dump Dump
			if err := json.Unmarshal(data, &dump); err != nil {
				t.Fatalf("decode Type 1 smoke dump: %v", err)
			}
			if dump.Source.Path != fontPath || dump.Source.Size != int64(len(tt.font)) || dump.Source.SHA256 == "" {
				t.Fatalf("source info = %+v, want path %q size %d and hash", dump.Source, fontPath, len(tt.font))
			}
			if dump.Face.NumFaces != 1 || dump.Face.NumGlyphs != 2 || dump.Face.UnitsPerEm != 1000 {
				t.Fatalf("face info = %+v, want 1 face, 2 glyphs, 1000 upem", dump.Face)
			}
			if len(dump.Charmap) != 1 || dump.Charmap[0].Char != "U+0041" || dump.Charmap[0].GlyphIndex != 1 {
				t.Fatalf("charmap = %+v, want U+0041 -> glyph 1", dump.Charmap)
			}
			if len(dump.Sizes) != 1 || dump.Sizes[0].PPEMX != 20 || dump.Sizes[0].PPEMY != 20 {
				t.Fatalf("sizes = %+v, want one 20ppem dump", dump.Sizes)
			}

			var glyph *GlyphRecord
			for i := range dump.Sizes[0].Glyphs {
				if dump.Sizes[0].Glyphs[i].GlyphIndex == 1 {
					glyph = &dump.Sizes[0].Glyphs[i]
					break
				}
			}
			if glyph == nil {
				t.Fatalf("glyphs = %+v, want glyph 1 from requested char", dump.Sizes[0].Glyphs)
			}
			if glyph.LoadError != "" || !glyph.Metrics.Available || !glyph.Outline.Available {
				t.Fatalf("glyph record = %+v, want loaded outline with metrics", *glyph)
			}
			type1SmokeAssertSegments(t, *glyph)
		})
	}
}

func TestType1SmokeFaceIndexOutOfRangeReportsType1Context(t *testing.T) {
	pfa := type1SmokePFA()
	tests := []struct {
		name string
		ext  string
		font []byte
	}{
		{name: "PFA", ext: ".pfa", font: pfa},
		{name: "PFB", ext: ".pfb", font: type1SmokePFB(pfa)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			fontPath := filepath.Join(dir, "type1-smoke"+tt.ext)
			if err := os.WriteFile(fontPath, tt.font, 0o600); err != nil {
				t.Fatalf("write Type 1 smoke font: %v", err)
			}
			requestPath := filepath.Join(dir, "type1-smoke-request.json")
			if err := os.WriteFile(requestPath, []byte(fmt.Sprintf(`{
				"font": %q,
				"face_index": 1,
				"ppem": "20",
				"chars": "U+0041",
				"load_flags": "no-hinting",
				"render_mode": "none"
			}`, fontPath)), 0o600); err != nil {
				t.Fatalf("write Type 1 smoke request: %v", err)
			}

			var out bytes.Buffer
			err := runGoDumpCommand([]string{
				"-request", requestPath,
				"-out", filepath.Join(dir, "type1-smoke.go.json"),
			}, &out, &out)
			if err == nil {
				t.Fatal("run Type 1 smoke dump succeeded with face_index 1")
			}
			message := err.Error()
			for _, want := range []string{"standalone Type 1", "face index 1", "out of range"} {
				if !strings.Contains(message, want) {
					t.Fatalf("error %q missing %q", message, want)
				}
			}
		})
	}
}

func TestType1SmokeRenderModesFromStandaloneFallback(t *testing.T) {
	pfa := type1SmokePFA()
	tests := []struct {
		name string
		ext  string
		font []byte
	}{
		{name: "PFA", ext: ".pfa", font: pfa},
		{name: "PFB", ext: ".pfb", font: type1SmokePFB(pfa)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			fontPath := filepath.Join(dir, "type1-smoke"+tt.ext)
			if err := os.WriteFile(fontPath, tt.font, 0o600); err != nil {
				t.Fatalf("write Type 1 smoke font: %v", err)
			}

			outputPath := filepath.Join(dir, "type1-smoke-render.go.json")
			var out bytes.Buffer
			err := runGoDumpCommand([]string{
				"-font", fontPath,
				"-out", outputPath,
				"-ppem", "20",
				"-glyphs", "",
				"-chars", "U+0041",
				"-load-flags", "no-hinting",
				"-render-mode", "normal,mono",
			}, &out, &out)
			if err != nil {
				t.Fatalf("run Type 1 render smoke dump: %v\n%s", err, out.String())
			}

			dump, err := readDump(outputPath)
			if err != nil {
				t.Fatalf("read Type 1 render smoke dump: %v", err)
			}
			if dump.Request.RenderMode != "normal,mono" || len(dump.Request.RenderModes) != 2 {
				t.Fatalf("render request = mode %q modes %v", dump.Request.RenderMode, dump.Request.RenderModes)
			}
			if len(dump.Sizes) != 2 {
				t.Fatalf("size count = %d, want two render-mode dumps", len(dump.Sizes))
			}

			wantPixelModes := map[string]string{
				"normal": "gray",
				"mono":   "mono",
			}
			seen := make(map[string]bool)
			for _, size := range dump.Sizes {
				wantPixelMode, ok := wantPixelModes[size.RenderMode]
				if !ok {
					t.Fatalf("unexpected render mode %q in size dump", size.RenderMode)
				}
				seen[size.RenderMode] = true
				glyph := type1SmokeFindGlyph(t, size.Glyphs, 1)
				if glyph.LoadError != "" || glyph.RenderError != "" {
					t.Fatalf("%s glyph errors: load=%q render=%q", size.RenderMode, glyph.LoadError, glyph.RenderError)
				}
				if glyph.RenderedFormat != "bitmap" {
					t.Fatalf("%s rendered format = %q, want bitmap", size.RenderMode, glyph.RenderedFormat)
				}
				if !glyph.Bitmap.Available || glyph.Bitmap.BufferSize == 0 || glyph.Bitmap.SHA256 == "" {
					t.Fatalf("%s bitmap missing or empty: %+v", size.RenderMode, glyph.Bitmap)
				}
				if glyph.Bitmap.PixelModeName != wantPixelMode {
					t.Fatalf("%s pixel mode = %q, want %q", size.RenderMode, glyph.Bitmap.PixelModeName, wantPixelMode)
				}
				type1SmokeAssertSegments(t, glyph)
			}
			for mode := range wantPixelModes {
				if !seen[mode] {
					t.Fatalf("missing render mode %q in dump", mode)
				}
			}
		})
	}
}

func TestType1SmokeFallbackPreservesSFNTFaceIndexError(t *testing.T) {
	dir := t.TempDir()
	fontPath := writeConformanceFixtureFont(t, dir)

	var out bytes.Buffer
	err := runGoDumpCommand([]string{
		"-font", fontPath,
		"-face", "1",
		"-out", filepath.Join(dir, "sfnt.go.json"),
		"-ppem", "12",
		"-glyphs", "0",
		"-load-flags", "no-hinting",
		"-render-mode", "none",
	}, &out, &out)
	if err == nil {
		t.Fatal("run SFNT dump succeeded with out-of-range face index")
	}

	message := err.Error()
	for _, want := range []string{"load face 1", "face index out of range"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q missing %q", message, want)
		}
	}
	for _, unwanted := range []string{"Type 1 fallback failed", "standalone Type 1"} {
		if strings.Contains(message, unwanted) {
			t.Fatalf("SFNT error %q unexpectedly contains %q", message, unwanted)
		}
	}
}

func type1SmokeFindGlyph(t *testing.T, glyphs []GlyphRecord, glyphIndex int) GlyphRecord {
	t.Helper()

	for _, glyph := range glyphs {
		if glyph.GlyphIndex == glyphIndex {
			return glyph
		}
	}
	t.Fatalf("glyphs = %+v, want glyph %d", glyphs, glyphIndex)
	return GlyphRecord{}
}

func type1SmokeAssertSegments(t *testing.T, glyph GlyphRecord) {
	t.Helper()

	wantKinds := []string{"move", "line", "line", "line", "close"}
	wantPoints := [][]t1.SegmentPoint{
		{{X: 10 * 64, Y: 10 * 64}},
		{{X: 110 * 64, Y: 10 * 64}},
		{{X: 110 * 64, Y: 110 * 64}},
		{{X: 10 * 64, Y: 110 * 64}},
		nil,
	}
	if len(glyph.Type1Segments) != len(wantKinds) {
		t.Fatalf("type1 segments = %+v, want %d native segments", glyph.Type1Segments, len(wantKinds))
	}
	for i, segment := range glyph.Type1Segments {
		if segment.Kind != wantKinds[i] {
			t.Fatalf("segment %d kind = %q, want %q; segments=%+v", i, segment.Kind, wantKinds[i], glyph.Type1Segments)
		}
		if segment.Units != t1.CharStringSegmentUnits {
			t.Fatalf("segment %d units = %q, want %q", i, segment.Units, t1.CharStringSegmentUnits)
		}
		if len(segment.Points) != len(wantPoints[i]) {
			t.Fatalf("segment %d points = %+v, want %+v", i, segment.Points, wantPoints[i])
		}
		for j, point := range segment.Points {
			if point != wantPoints[i][j] {
				t.Fatalf("segment %d point %d = %+v, want %+v", i, j, point, wantPoints[i][j])
			}
		}
	}
}

func type1SmokePFA() []byte {
	notdef := type1SmokeCharString(type1SmokeNumbers(0, 500), type1SmokeOps(13, 14))
	glyph := type1SmokeCharString(
		type1SmokeNumbers(0, 500), type1SmokeOps(13),
		type1SmokeNumbers(10, 10), type1SmokeOps(21),
		type1SmokeNumbers(100, 0, 0, 100, -100, 0), type1SmokeOps(5),
		type1SmokeOps(9, 14),
	)

	private := []byte(`/Private 8 dict dup begin
/lenIV -1 def
/CharStrings 2 dict dup begin
/.notdef `)
	private = append(private, []byte(fmt.Sprintf("%d RD ", len(notdef)))...)
	private = append(private, notdef...)
	private = append(private, []byte(` ND
/A `)...)
	private = append(private, []byte(fmt.Sprintf("%d RD ", len(glyph)))...)
	private = append(private, glyph...)
	private = append(private, []byte(` ND
end
end
`)...)

	pfa := []byte(`%!PS-AdobeFont-1.0: ConformanceType1Smoke 1.0
/FontName /ConformanceType1Smoke def
/FontMatrix [0.001 0 0 0.001 0 0] readonly def
/FontBBox [0 0 120 120] readonly def
/Encoding 256 array
dup 65 /A put
readonly def
currentfile eexec
`)
	pfa = append(pfa, []byte(hex.EncodeToString(type1SmokeEncryptedEexec(private)))...)
	pfa = append(pfa, []byte("\n0000000000000000\ncleartomark\n")...)
	return pfa
}

func type1SmokeCharString(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func type1SmokeNumbers(vals ...int) []byte {
	var out []byte
	for _, v := range vals {
		out = append(out, type1SmokeNumber(v)...)
	}
	return out
}

func type1SmokeOps(ops ...int) []byte {
	out := make([]byte, len(ops))
	for i, op := range ops {
		out[i] = byte(op)
	}
	return out
}

func type1SmokeNumber(v int) []byte {
	switch {
	case v >= -107 && v <= 107:
		return []byte{byte(v + 139)}
	case v >= 108 && v <= 1131:
		x := v - 108
		return []byte{byte(x/256 + 247), byte(x % 256)}
	case v <= -108 && v >= -1131:
		x := -v - 108
		return []byte{byte(x/256 + 251), byte(x % 256)}
	default:
		return []byte{255, byte(uint32(v) >> 24), byte(uint32(v) >> 16), byte(uint32(v) >> 8), byte(uint32(v))}
	}
}

func type1SmokeEncryptedEexec(private []byte) []byte {
	plain := append([]byte{1, 2, 3, 4}, private...)
	return type1SmokeEncrypt(plain, 55665)
}

func type1SmokePFB(pfa []byte) []byte {
	size := len(pfa)
	pfb := []byte{
		0x80,
		1,
		byte(size),
		byte(size >> 8),
		byte(size >> 16),
		byte(size >> 24),
	}
	pfb = append(pfb, pfa...)
	return append(pfb, 0x80, 3)
}

func type1SmokeEncrypt(data []byte, seed uint16) []byte {
	const c1 = 52845
	const c2 = 22719

	out := make([]byte, len(data))
	r := seed
	for i, plain := range data {
		cipher := plain ^ byte(r>>8)
		out[i] = cipher
		r = (uint16(cipher)+r)*c1 + c2
	}
	return out
}
