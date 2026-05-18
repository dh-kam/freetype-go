package type1

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestType1FaceAFMCompanionOverridesMetrics(t *testing.T) {
	face := companionTestFace(t)
	face.SetAFM(&AFM{
		CharMetrics: []AFMCharMetric{
			{Name: "A", WidthX: 700, BBox: [4]float64{12, 0, 650, 700}},
		},
	})
	gid := companionGlyphIndex(t, face, 'A')

	advance, lsb, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 700*64 || lsb != 12*64 {
		t.Fatalf("metrics advance=%d lsb=%d, want advance=%d lsb=%d", advance, lsb, 700*64, 12*64)
	}

	noScaleSlot, err := face.LoadGlyph(gid, api.LoadNoScale)
	if err != nil {
		t.Fatalf("LoadGlyph(no scale) failed: %v", err)
	}
	noScaleMetrics := companionSlotMetrics(t, noScaleSlot)
	if noScaleMetrics.HoriAdvance != 700*64 || noScaleMetrics.HoriBearingX != 12*64 {
		t.Fatalf("no-scale slot metrics advance=%d lsb=%d, want advance=%d lsb=%d", noScaleMetrics.HoriAdvance, noScaleMetrics.HoriBearingX, 700*64, 12*64)
	}

	if err := face.SetPixelSizes(500, 500); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	scaledSlot, err := face.LoadGlyph(gid, api.LoadDefault)
	if err != nil {
		t.Fatalf("LoadGlyph(default) failed: %v", err)
	}
	scaledMetrics := companionSlotMetrics(t, scaledSlot)
	if scaledMetrics.HoriAdvance != 350*64 || scaledMetrics.HoriBearingX != 6*64 {
		t.Fatalf("scaled slot metrics advance=%d lsb=%d, want advance=%d lsb=%d", scaledMetrics.HoriAdvance, scaledMetrics.HoriBearingX, 350*64, 6*64)
	}
}

func TestType1FaceAFMCompanionZeroLSBOverridesSlotBearing(t *testing.T) {
	face := companionTestFace(t)
	face.SetAFM(&AFM{
		CharMetrics: []AFMCharMetric{
			{Name: "A", WidthX: 700, BBox: [4]float64{0, 0, 650, 700}},
		},
	})
	gid := companionGlyphIndex(t, face, 'A')

	slot, err := face.LoadGlyph(gid, api.LoadNoScale)
	if err != nil {
		t.Fatalf("LoadGlyph(no scale) failed: %v", err)
	}
	metrics := companionSlotMetrics(t, slot)
	if metrics.HoriBearingX != 0 {
		t.Fatalf("no-scale slot lsb = %d, want explicit AFM lsb 0", metrics.HoriBearingX)
	}
}

func TestType1FacePFMCompanionFallbackMetrics(t *testing.T) {
	face := companionTestFace(t)
	face.SetAFM(&AFM{
		CharMetrics: []AFMCharMetric{
			{Name: "B", WidthX: 900, BBox: [4]float64{30, 0, 800, 700}},
		},
	})
	face.SetPFM(&PFM{
		FirstChar:    'A',
		LastChar:     'A',
		AvgWidth:     500,
		MaxWidth:     700,
		ExtentWidths: []uint16{620},
	})
	gid := companionGlyphIndex(t, face, 'A')

	advance, lsb, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 620*64 || lsb != 0 {
		t.Fatalf("metrics advance=%d lsb=%d, want advance=%d lsb=0", advance, lsb, 620*64)
	}

	if err := face.SetPixelSizes(500, 500); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	slot, err := face.LoadGlyph(gid, api.LoadDefault)
	if err != nil {
		t.Fatalf("LoadGlyph(default) failed: %v", err)
	}
	metrics := companionSlotMetrics(t, slot)
	if metrics.HoriAdvance != 310*64 {
		t.Fatalf("scaled slot advance = %d, want %d", metrics.HoriAdvance, 310*64)
	}
}

func TestType1FaceSetCompanionMetricsAttachesParsedAFMAndPFM(t *testing.T) {
	face := companionTestFace(t)
	pfmData, _, _ := testPFM()
	var encoding [256]string
	encoding[65] = "A"

	metrics, err := ReadCompanionMetrics(strings.NewReader(`
StartFontMetrics 4.1
StartKernPairs 1
KPX A A -80
EndKernPairs
EndFontMetrics
`), bytes.NewReader(pfmData), encoding)
	if err != nil {
		t.Fatalf("ReadCompanionMetrics failed: %v", err)
	}
	face.SetCompanionMetrics(metrics)

	gid := companionGlyphIndex(t, face, 'A')
	advance, lsb, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 600*64 || lsb != 0 {
		t.Fatalf("metrics advance=%d lsb=%d, want advance=%d lsb=0", advance, lsb, 600*64)
	}

	glyphs, advances := face.Shape("AA")
	if !intsEqual(glyphs, []int{1, 1}) {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	wantAdvances := []api.Vector{
		{X: (600 - 80) * 64},
		{X: 600 * 64},
	}
	if !vectorsEqual(advances, wantAdvances) {
		t.Fatalf("advances = %+v, want %+v", advances, wantAdvances)
	}
}

func TestType1FaceSetCompanionMetricsFilesAttachesExplicitFiles(t *testing.T) {
	face := companionTestFace(t)
	pfmData, _, _ := testPFM()
	dir := t.TempDir()
	afmPath := filepath.Join(dir, "explicit.afm")
	pfmPath := filepath.Join(dir, "explicit.pfm")

	if err := os.WriteFile(afmPath, []byte(`
StartFontMetrics 4.1
StartKernPairs 1
KPX A A -80
EndKernPairs
EndFontMetrics
`), 0o600); err != nil {
		t.Fatalf("WriteFile(AFM) failed: %v", err)
	}
	if err := os.WriteFile(pfmPath, pfmData, 0o600); err != nil {
		t.Fatalf("WriteFile(PFM) failed: %v", err)
	}

	var encoding [256]string
	encoding[65] = "A"

	if err := face.SetCompanionMetricsFiles(afmPath, pfmPath, encoding); err != nil {
		t.Fatalf("SetCompanionMetricsFiles failed: %v", err)
	}

	gid := companionGlyphIndex(t, face, 'A')
	advance, lsb, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 600*64 || lsb != 0 {
		t.Fatalf("metrics advance=%d lsb=%d, want advance=%d lsb=0", advance, lsb, 600*64)
	}

	glyphs, advances := face.Shape("AA")
	if !intsEqual(glyphs, []int{1, 1}) {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	wantAdvances := []api.Vector{
		{X: (600 - 80) * 64},
		{X: 600 * 64},
	}
	if !vectorsEqual(advances, wantAdvances) {
		t.Fatalf("advances = %+v, want %+v", advances, wantAdvances)
	}
}

func TestType1FaceSetCompanionMetricsFilesAFMOnlyUsesWidthsAndKerning(t *testing.T) {
	face := companionTestFace(t)
	dir := t.TempDir()
	afmPath := filepath.Join(dir, "metrics.afm")

	if err := os.WriteFile(afmPath, []byte(`
StartFontMetrics 4.1
StartCharMetrics 1
C 65 ; WX 720 ; N A ; B 12 0 690 700 ;
EndCharMetrics
StartKernPairs 1
KPX A A -80
EndKernPairs
EndFontMetrics
`), 0o600); err != nil {
		t.Fatalf("WriteFile(AFM) failed: %v", err)
	}

	var encoding [256]string
	encoding[65] = "A"

	if err := face.SetCompanionMetricsFiles(afmPath, "", encoding); err != nil {
		t.Fatalf("SetCompanionMetricsFiles failed: %v", err)
	}

	gid := companionGlyphIndex(t, face, 'A')
	advance, lsb, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 720*64 || lsb != 12*64 {
		t.Fatalf("metrics advance=%d lsb=%d, want advance=%d lsb=%d", advance, lsb, 720*64, 12*64)
	}

	glyphs, advances := face.Shape("AA")
	if !intsEqual(glyphs, []int{1, 1}) {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	wantAdvances := []api.Vector{
		{X: (720 - 80) * 64},
		{X: 720 * 64},
	}
	if !vectorsEqual(advances, wantAdvances) {
		t.Fatalf("advances = %+v, want %+v", advances, wantAdvances)
	}
}

func TestType1FaceSetCompanionMetricsFilesPFMOnlyUsesFallbackWidths(t *testing.T) {
	face := companionTestFace(t)
	pfmData, _, _ := testPFM()
	dir := t.TempDir()
	pfmPath := filepath.Join(dir, "metrics.pfm")

	if err := os.WriteFile(pfmPath, pfmData, 0o600); err != nil {
		t.Fatalf("WriteFile(PFM) failed: %v", err)
	}

	var encoding [256]string
	encoding[65] = "A"

	if err := face.SetCompanionMetricsFiles("", pfmPath, encoding); err != nil {
		t.Fatalf("SetCompanionMetricsFiles failed: %v", err)
	}

	gid := companionGlyphIndex(t, face, 'A')
	advance, lsb, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 600*64 || lsb != 0 {
		t.Fatalf("metrics advance=%d lsb=%d, want advance=%d lsb=0", advance, lsb, 600*64)
	}

	glyphs, advances := face.Shape("AA")
	if !intsEqual(glyphs, []int{1, 1}) {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	wantAdvances := []api.Vector{
		{X: 600 * 64},
		{X: 600 * 64},
	}
	if !vectorsEqual(advances, wantAdvances) {
		t.Fatalf("advances = %+v, want %+v", advances, wantAdvances)
	}
}

func TestType1FaceSetCompanionMetricsFilesFailedSecondPathKeepsExistingMetrics(t *testing.T) {
	face := companionTestFace(t)
	face.SetCompanionMetrics(&CompanionMetrics{
		AFM: &AFM{
			KernPairs: []AFMKernPair{
				{Left: "A", Right: "A", X: -120},
			},
		},
		PFM: &PFM{
			FirstChar:    'A',
			LastChar:     'A',
			AvgWidth:     930,
			MaxWidth:     930,
			ExtentWidths: []uint16{930},
		},
	})

	dir := t.TempDir()
	afmPath := filepath.Join(dir, "replacement.afm")
	missingPFM := filepath.Join(dir, "missing.pfm")
	if err := os.WriteFile(afmPath, []byte(`
StartFontMetrics 4.1
StartCharMetrics 1
C 65 ; WX 720 ; N A ; B 12 0 690 700 ;
EndCharMetrics
StartKernPairs 1
KPX A A -80
EndKernPairs
EndFontMetrics
`), 0o600); err != nil {
		t.Fatalf("WriteFile(AFM) failed: %v", err)
	}

	var encoding [256]string
	encoding[65] = "A"

	err := face.SetCompanionMetricsFiles(afmPath, missingPFM, encoding)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing PFM error = %v, want os.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), "open PFM companion metrics") || !strings.Contains(err.Error(), missingPFM) {
		t.Fatalf("missing PFM error = %q, want context with PFM path", err)
	}

	gid := companionGlyphIndex(t, face, 'A')
	advance, lsb, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 930*64 || lsb != 0 {
		t.Fatalf("metrics advance=%d lsb=%d, want existing advance=%d lsb=0", advance, lsb, 930*64)
	}

	glyphs, advances := face.Shape("AA")
	if !intsEqual(glyphs, []int{1, 1}) {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	wantAdvances := []api.Vector{
		{X: (930 - 120) * 64},
		{X: 930 * 64},
	}
	if !vectorsEqual(advances, wantAdvances) {
		t.Fatalf("advances = %+v, want existing %+v", advances, wantAdvances)
	}
}

func TestType1FaceSetCompanionMetricsFilesEmptyPathsClearMetricsAndWrapErrors(t *testing.T) {
	var encoding [256]string
	encoding[65] = "A"

	t.Run("empty paths clear optional companions", func(t *testing.T) {
		face := companionTestFace(t)
		face.SetCompanionMetrics(&CompanionMetrics{
			AFM: &AFM{
				CharMetrics: []AFMCharMetric{
					{Name: "A", WidthX: 910, BBox: [4]float64{20, 0, 800, 700}},
				},
			},
			PFM: &PFM{
				FirstChar:    'A',
				LastChar:     'A',
				AvgWidth:     910,
				MaxWidth:     910,
				ExtentWidths: []uint16{910},
			},
		})

		if err := face.SetCompanionMetricsFiles("", "", encoding); err != nil {
			t.Fatalf("SetCompanionMetricsFiles failed: %v", err)
		}

		gid := companionGlyphIndex(t, face, 'A')
		advance, _, err := face.GetGlyphMetrics(gid)
		if err != nil {
			t.Fatalf("GetGlyphMetrics failed: %v", err)
		}
		if advance != 500*64 {
			t.Fatalf("advance = %d, want fallback charstring advance %d", advance, 500*64)
		}
	})

	t.Run("missing file error remains wrapped", func(t *testing.T) {
		face := companionTestFace(t)
		missingAFM := filepath.Join(t.TempDir(), "missing.afm")

		err := face.SetCompanionMetricsFiles(missingAFM, "", encoding)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing AFM error = %v, want os.ErrNotExist", err)
		}
		if !strings.Contains(err.Error(), "open AFM companion metrics") || !strings.Contains(err.Error(), missingAFM) {
			t.Fatalf("missing AFM error = %q, want context with AFM path", err)
		}
	})
}

func TestType1FaceSetCompanionMetricsAcceptsNilOptionalComponents(t *testing.T) {
	pfmData, _, _ := testPFM()
	var encoding [256]string
	encoding[65] = "A"

	tests := []struct {
		name        string
		metrics     func(t *testing.T) *CompanionMetrics
		wantAdvance int32
		wantShape   []api.Vector
	}{
		{
			name: "nil metrics clears attachments",
			metrics: func(t *testing.T) *CompanionMetrics {
				return nil
			},
			wantAdvance: 500 * 64,
			wantShape: []api.Vector{
				{X: 500 * 64},
				{X: 500 * 64},
			},
		},
		{
			name: "no companions clears attachments",
			metrics: func(t *testing.T) *CompanionMetrics {
				t.Helper()
				metrics, err := ReadCompanionMetrics(nil, nil, encoding)
				if err != nil {
					t.Fatalf("ReadCompanionMetrics failed: %v", err)
				}
				return metrics
			},
			wantAdvance: 500 * 64,
		},
		{
			name: "nil AFM keeps PFM widths",
			metrics: func(t *testing.T) *CompanionMetrics {
				t.Helper()
				metrics, err := ReadCompanionMetrics(nil, bytes.NewReader(pfmData), encoding)
				if err != nil {
					t.Fatalf("ReadCompanionMetrics failed: %v", err)
				}
				return metrics
			},
			wantAdvance: 600 * 64,
		},
		{
			name: "nil PFM keeps AFM kerning",
			metrics: func(t *testing.T) *CompanionMetrics {
				t.Helper()
				metrics, err := ReadCompanionMetrics(strings.NewReader(`
StartFontMetrics 4.1
StartKernPairs 1
KPX A A -80
EndKernPairs
EndFontMetrics
`), nil, encoding)
				if err != nil {
					t.Fatalf("ReadCompanionMetrics failed: %v", err)
				}
				return metrics
			},
			wantAdvance: 500 * 64,
			wantShape: []api.Vector{
				{X: (500 - 80) * 64},
				{X: 500 * 64},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			face := companionTestFace(t)
			face.SetCompanionMetrics(&CompanionMetrics{
				AFM: &AFM{
					CharMetrics: []AFMCharMetric{
						{Name: "A", WidthX: 910, BBox: [4]float64{20, 0, 800, 700}},
					},
					KernPairs: []AFMKernPair{
						{Left: "A", Right: "A", X: -120},
					},
				},
				PFM: &PFM{
					FirstChar:    'A',
					LastChar:     'A',
					AvgWidth:     910,
					MaxWidth:     910,
					ExtentWidths: []uint16{910},
				},
			})

			face.SetCompanionMetrics(tt.metrics(t))

			gid := companionGlyphIndex(t, face, 'A')
			advance, _, err := face.GetGlyphMetrics(gid)
			if err != nil {
				t.Fatalf("GetGlyphMetrics failed: %v", err)
			}
			if advance != tt.wantAdvance {
				t.Fatalf("advance = %d, want %d", advance, tt.wantAdvance)
			}

			if tt.wantShape == nil {
				return
			}
			glyphs, advances := face.Shape("AA")
			if !intsEqual(glyphs, []int{1, 1}) {
				t.Fatalf("glyphs = %v, want [1 1]", glyphs)
			}
			if !vectorsEqual(advances, tt.wantShape) {
				t.Fatalf("advances = %+v, want %+v", advances, tt.wantShape)
			}
		})
	}
}

func TestType1FaceCompanionMetricsFallbackToCharString(t *testing.T) {
	face := companionTestFace(t)
	gid := companionGlyphIndex(t, face, 'A')

	advance, lsb, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 500*64 || lsb != 0 {
		t.Fatalf("metrics advance=%d lsb=%d, want advance=%d lsb=0", advance, lsb, 500*64)
	}

	slot, err := face.LoadGlyph(gid, api.LoadNoScale)
	if err != nil {
		t.Fatalf("LoadGlyph(no scale) failed: %v", err)
	}
	metrics := companionSlotMetrics(t, slot)
	if metrics.HoriAdvance != 500*64 {
		t.Fatalf("no-scale slot advance = %d, want %d", metrics.HoriAdvance, 500*64)
	}
}

func TestType1FaceCompanionMetricsShapeAdvance(t *testing.T) {
	face := companionTestFace(t)
	face.SetAFM(&AFM{
		CharMetrics: []AFMCharMetric{
			{Name: "A", WidthX: 720, BBox: [4]float64{0, 0, 650, 700}},
		},
	})
	if err := face.SetPixelSizes(500, 500); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}

	glyphs, advances := face.Shape("AA")
	if !intsEqual(glyphs, []int{1, 1}) {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	wantAdvances := []api.Vector{
		{X: 360 * 64},
		{X: 360 * 64},
	}
	if !vectorsEqual(advances, wantAdvances) {
		t.Fatalf("advances = %+v, want %+v", advances, wantAdvances)
	}
}

func TestType1FaceAFMCompanionShapeKerning(t *testing.T) {
	face := companionTestFace(t)
	face.SetAFM(&AFM{
		KernPairs: []AFMKernPair{
			{Left: "A", Right: "A", X: -80},
		},
	})

	glyphs, advances := face.Shape("AA")
	if !intsEqual(glyphs, []int{1, 1}) {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	wantAdvances := []api.Vector{
		{X: (500 - 80) * 64},
		{X: 500 * 64},
	}
	if !vectorsEqual(advances, wantAdvances) {
		t.Fatalf("advances = %+v, want %+v", advances, wantAdvances)
	}
}

func TestType1FaceAFMCompanionShapeNoAFMFallback(t *testing.T) {
	face := companionTestFace(t)

	glyphs, advances := face.Shape("AA")
	if !intsEqual(glyphs, []int{1, 1}) {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	wantAdvances := []api.Vector{
		{X: 500 * 64},
		{X: 500 * 64},
	}
	if !vectorsEqual(advances, wantAdvances) {
		t.Fatalf("advances = %+v, want %+v", advances, wantAdvances)
	}
}

func TestType1FaceAFMCompanionShapeMissingKernPair(t *testing.T) {
	face := companionTestFace(t)
	face.SetAFM(&AFM{
		KernPairs: []AFMKernPair{
			{Left: "A", Right: "V", X: -80},
		},
	})

	glyphs, advances := face.Shape("AA")
	if !intsEqual(glyphs, []int{1, 1}) {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	wantAdvances := []api.Vector{
		{X: 500 * 64},
		{X: 500 * 64},
	}
	if !vectorsEqual(advances, wantAdvances) {
		t.Fatalf("advances = %+v, want %+v", advances, wantAdvances)
	}
}

func TestType1FaceAFMCompanionShapeKerningScalesAfterSetPixelSizes(t *testing.T) {
	face := companionTestFace(t)
	face.SetAFM(&AFM{
		KernPairs: []AFMKernPair{
			{Left: "A", Right: "A", X: -80},
		},
	})
	if err := face.SetPixelSizes(500, 500); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}

	glyphs, advances := face.Shape("AA")
	if !intsEqual(glyphs, []int{1, 1}) {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	wantAdvances := []api.Vector{
		{X: (250 - 40) * 64},
		{X: 250 * 64},
	}
	if !vectorsEqual(advances, wantAdvances) {
		t.Fatalf("advances = %+v, want %+v", advances, wantAdvances)
	}
}

func companionTestFace(t *testing.T) *Face {
	t.Helper()
	loaded, err := NewLoader(nil).LoadFace(type1TestStream{data: testType1FacePFA()})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	face, ok := loaded.(*Face)
	if !ok {
		t.Fatalf("LoadFace returned %T, want *Face", loaded)
	}
	return face
}

func companionGlyphIndex(t *testing.T, face *Face, r rune) int {
	t.Helper()
	gid, err := face.GetGlyphIndex(r)
	if err != nil {
		t.Fatalf("GetGlyphIndex(%q) failed: %v", r, err)
	}
	if gid == 0 {
		t.Fatalf("GetGlyphIndex(%q) = 0, want mapped glyph", r)
	}
	return gid
}

func companionSlotMetrics(t *testing.T, slot api.GlyphSlot) api.GlyphMetrics {
	t.Helper()
	metrics, ok := api.GetGlyphSlotMetrics(slot)
	if !ok {
		t.Fatal("slot did not expose metrics")
	}
	return metrics
}
