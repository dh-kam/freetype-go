package type1

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestType1LoadGlyphComposesSEACNoScale(t *testing.T) {
	face := loadType1SEACFace(t)
	if got := face.font.GlyphName(65); got == "A" {
		t.Fatalf("test fixture must not resolve code 65 through font encoding; got %q", got)
	}

	gid := type1SEACGlyphIndex(t, face, "Aacute")
	slot, err := face.LoadGlyph(gid, api.LoadNoScale)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}

	outline := slot.GetOutline()
	wantPoints := []api.Vector{
		{X: 20 * 64, Y: 0},
		{X: 120 * 64, Y: 0},
		{X: 120 * 64, Y: 100 * 64},
		{X: 20 * 64, Y: 100 * 64},
		{X: 80 * 64, Y: 120 * 64},
		{X: 100 * 64, Y: 140 * 64},
	}
	if !vectorsEqual(outline.GetPoints(), wantPoints) {
		t.Fatalf("points = %+v, want %+v", outline.GetPoints(), wantPoints)
	}
	if got, want := outline.GetContours(), []int{3, 5}; !intsEqual(got, want) {
		t.Fatalf("contours = %v, want %v", got, want)
	}

	metrics, ok := api.GetGlyphSlotMetrics(slot)
	if !ok {
		t.Fatal("slot did not expose metrics")
	}
	if metrics.HoriAdvance != 500*64 || metrics.HoriBearingX != 20*64 || metrics.HoriBearingY != 140*64 {
		t.Fatalf("metrics = %+v, want advance=%d bearingX=%d bearingY=%d", metrics, 500*64, 20*64, 140*64)
	}
	if metrics.Width != 100*64 || metrics.Height != 140*64 {
		t.Fatalf("bounds metrics = width %d height %d, want width %d height %d", metrics.Width, metrics.Height, 100*64, 140*64)
	}
}

func TestType1LoadGlyphComposesSEACScaled(t *testing.T) {
	face := loadType1SEACFace(t)
	if err := face.SetPixelSizes(500, 500); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}

	gid := type1SEACGlyphIndex(t, face, "Aacute")
	slot, err := face.LoadGlyph(gid, api.LoadDefault)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}

	wantPoints := []api.Vector{
		{X: 10 * 64, Y: 0},
		{X: 60 * 64, Y: 0},
		{X: 60 * 64, Y: 50 * 64},
		{X: 10 * 64, Y: 50 * 64},
		{X: 40 * 64, Y: 60 * 64},
		{X: 50 * 64, Y: 70 * 64},
	}
	if points := slot.GetOutline().GetPoints(); !vectorsEqual(points, wantPoints) {
		t.Fatalf("scaled points = %+v, want %+v", points, wantPoints)
	}

	metrics, ok := api.GetGlyphSlotMetrics(slot)
	if !ok {
		t.Fatal("slot did not expose metrics")
	}
	if metrics.HoriAdvance != 250*64 || metrics.HoriBearingX != 10*64 || metrics.HoriBearingY != 70*64 {
		t.Fatalf("scaled metrics = %+v, want advance=%d bearingX=%d bearingY=%d", metrics, 250*64, 10*64, 70*64)
	}
	if metrics.Width != 50*64 || metrics.Height != 70*64 {
		t.Fatalf("scaled bounds metrics = width %d height %d, want width %d height %d", metrics.Width, metrics.Height, 50*64, 70*64)
	}
}

func loadType1SEACFace(t *testing.T) *Face {
	t.Helper()
	loaded, err := NewLoader(nil).LoadFace(type1TestStream{data: testType1SEACFacePFA()})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	face, ok := loaded.(*Face)
	if !ok {
		t.Fatalf("LoadFace returned %T, want *Face", loaded)
	}
	return face
}

func type1SEACGlyphIndex(t *testing.T, face *Face, name string) int {
	t.Helper()
	gid, ok := face.glyphIndex[name]
	if !ok {
		t.Fatalf("missing glyph index for %q", name)
	}
	return gid
}

func testType1SEACFacePFA() []byte {
	notdef := encryptedTestCharString(t1prog(t1nums(0, 500), t1ops(13, 14)), 4)
	base := encryptedTestCharString(
		t1prog(
			t1nums(20, 500), t1ops(13),
			t1nums(0, 0), t1ops(21),
			t1nums(100, 0, 0, 100, -100, 0), t1ops(5),
			t1ops(9, 14),
		),
		4,
	)
	accent := encryptedTestCharString(
		t1prog(
			t1nums(15, 200), t1ops(13),
			t1nums(0, 0), t1ops(21),
			t1nums(20, 20), t1ops(5),
			t1ops(14),
		),
		4,
	)
	composite := encryptedTestCharString(
		t1prog(
			t1nums(20, 500), t1ops(13),
			t1nums(15, 60, 120, 65, 194), t1ops(12, 6),
			t1ops(14),
		),
		4,
	)

	private := []byte(`/Private 8 dict dup begin
/lenIV 4 def
/CharStrings 4 dict dup begin
`)
	private = appendType1SEACCharString(private, ".notdef", notdef)
	private = appendType1SEACCharString(private, "A", base)
	private = appendType1SEACCharString(private, "acute", accent)
	private = appendType1SEACCharString(private, "Aacute", composite)
	private = append(private, []byte(`end
end
`)...)

	pfa := []byte(`%!PS-AdobeFont-1.0: SEACType1 1.0
/FontName /SEACType1 def
/FontMatrix [0.001 0 0 0.001 0 0] readonly def
/FontBBox [0 0 200 200] readonly def
/Encoding 256 array
dup 65 /wrongbase put
dup 194 /wrongaccent put
readonly def
currentfile eexec
`)
	pfa = append(pfa, []byte(hex.EncodeToString(encryptedEexecForTest(private)))...)
	pfa = append(pfa, []byte("\n0000000000000000\ncleartomark\n")...)
	return pfa
}

func appendType1SEACCharString(dst []byte, name string, data []byte) []byte {
	dst = append(dst, []byte(fmt.Sprintf("/%s %d RD ", name, len(data)))...)
	dst = append(dst, data...)
	return append(dst, []byte(" ND\n")...)
}
