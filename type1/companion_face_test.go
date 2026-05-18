package type1

import (
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
