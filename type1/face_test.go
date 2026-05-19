package type1

import (
	"encoding/hex"
	"fmt"
	"io"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

func TestType1LoaderAndFace(t *testing.T) {
	data := testType1FacePFA()
	driver := NewLoader(nil)
	if !driver.Handles(type1TestStream{data: data}) {
		t.Fatal("Type1 loader did not handle PFA stream")
	}

	loaded, err := driver.LoadFace(type1TestStream{data: data})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	face, ok := loaded.(*Face)
	if !ok {
		t.Fatalf("LoadFace returned %T, want *Face", loaded)
	}
	if face.GetUnitsPerEm() != 1000 {
		t.Fatalf("unitsPerEm = %d, want 1000", face.GetUnitsPerEm())
	}
	if face.GetNumGlyphs() != 2 {
		t.Fatalf("num glyphs = %d, want 2", face.GetNumGlyphs())
	}
	if name, ok := face.GetGlyphName(1); !ok || name != "A" {
		t.Fatalf("GetGlyphName(1) = %q, %v; want A, true", name, ok)
	}
	gid, err := face.GetGlyphIndex('A')
	if err != nil {
		t.Fatalf("GetGlyphIndex failed: %v", err)
	}
	if gid != 1 {
		t.Fatalf("glyph index for A = %d, want 1", gid)
	}
	nameGID, ok := face.GetGlyphIndexByName("A")
	if !ok {
		t.Fatal("GetGlyphIndexByName did not find A")
	}
	if nameGID != gid {
		t.Fatalf("glyph index by name = %d, want %d", nameGID, gid)
	}
	if _, ok := face.GetGlyphIndexByName("MissingGlyph"); ok {
		t.Fatal("GetGlyphIndexByName unexpectedly found MissingGlyph")
	}

	slot, err := face.LoadGlyph(gid, api.LoadNoScale)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	points := slot.GetOutline().GetPoints()
	wantPoints := []api.Vector{
		{X: 10 * 64, Y: 20 * 64},
		{X: 60 * 64, Y: 20 * 64},
	}
	if !vectorsEqual(points, wantPoints) {
		t.Fatalf("points = %+v, want %+v", points, wantPoints)
	}

	advance, lsb, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 500*64 || lsb != 0 {
		t.Fatalf("metrics advance=%d lsb=%d, want advance=%d lsb=0", advance, lsb, 500*64)
	}

	if err := face.SetPixelSizes(500, 500); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	scaledSlot, err := face.LoadGlyph(gid, api.LoadDefault)
	if err != nil {
		t.Fatalf("scaled LoadGlyph failed: %v", err)
	}
	scaled := scaledSlot.GetOutline().GetPoints()
	wantScaled := []api.Vector{
		{X: 5 * 64, Y: 10 * 64},
		{X: 30 * 64, Y: 10 * 64},
	}
	if !vectorsEqual(scaled, wantScaled) {
		t.Fatalf("scaled points = %+v, want %+v", scaled, wantScaled)
	}

	metrics, ok := api.GetGlyphSlotMetrics(scaledSlot)
	if !ok {
		t.Fatal("scaled slot did not expose metrics")
	}
	if metrics.HoriAdvance != 250*64 {
		t.Fatalf("slot HoriAdvance = %d, want %d", metrics.HoriAdvance, 250*64)
	}
}

func TestType1PFBLoaderHandle(t *testing.T) {
	pfa := testType1FacePFA()
	pfb := testType1PFB(pfa)

	driver := NewLoader(nil)
	if !driver.Handles(type1TestStream{data: pfb}) {
		t.Fatal("Type1 loader did not handle PFB stream")
	}
	if _, err := driver.LoadFace(type1TestStream{data: pfb}); err != nil {
		t.Fatalf("LoadFace failed for PFB: %v", err)
	}
}

func TestType1LibraryLoadsPFAAndPFB(t *testing.T) {
	pfa := testType1FacePFA()
	tests := []struct {
		name string
		data []byte
	}{
		{name: "PFA", data: pfa},
		{name: "PFB", data: testType1PFB(pfa)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := core.NewLibrary()
			lib.AddDriver(NewLoader(core.NewSystem()))

			loaded, err := lib.LoadFace(core.NewMemoryStream(tt.data))
			if err != nil {
				t.Fatalf("LoadFace failed: %v", err)
			}
			if loaded.GetUnitsPerEm() != 1000 {
				t.Fatalf("unitsPerEm = %d, want 1000", loaded.GetUnitsPerEm())
			}
			if loaded.GetNumGlyphs() != 2 {
				t.Fatalf("num glyphs = %d, want 2", loaded.GetNumGlyphs())
			}
			gid, err := loaded.GetGlyphIndex('A')
			if err != nil {
				t.Fatalf("GetGlyphIndex failed: %v", err)
			}
			if gid != 1 {
				t.Fatalf("glyph index for A = %d, want 1", gid)
			}
		})
	}
}

func TestType1LoadRenderCreatesBitmap(t *testing.T) {
	loaded, err := NewLoader(core.NewSystem()).LoadFace(type1TestStream{data: testType1RectFacePFA()})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	if err := loaded.SetPixelSizes(100, 100); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	gid, err := loaded.GetGlyphIndex('A')
	if err != nil {
		t.Fatalf("GetGlyphIndex failed: %v", err)
	}
	slot, err := loaded.LoadGlyph(gid, api.LoadRender|api.LoadNoHinting)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	bitmap := slot.GetBitmap()
	if bitmap == nil {
		t.Fatal("rendered bitmap is nil")
	}
	if bitmap.GetPixelMode() != api.MODE_GRAY {
		t.Fatalf("bitmap pixel mode = %d, want MODE_GRAY", bitmap.GetPixelMode())
	}
	if bitmap.GetWidth() == 0 || bitmap.GetRows() == 0 || bitmap.GetPitch() == 0 {
		t.Fatalf("empty bitmap geometry: width=%d rows=%d pitch=%d", bitmap.GetWidth(), bitmap.GetRows(), bitmap.GetPitch())
	}
	if !type1HasNonZeroByte(bitmap.GetBuffer()) {
		t.Fatal("rendered bitmap buffer is empty")
	}
}

func TestType1ShapeReturnsGlyphsAndAdvances(t *testing.T) {
	loaded, err := NewLoader(nil).LoadFace(type1TestStream{data: testType1FacePFA()})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	if err := loaded.SetPixelSizes(500, 500); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}

	glyphs, advances := loaded.Shape("AA")
	if !intsEqual(glyphs, []int{1, 1}) {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	wantAdvances := []api.Vector{
		{X: 250 * 64},
		{X: 250 * 64},
	}
	if !vectorsEqual(advances, wantAdvances) {
		t.Fatalf("advances = %+v, want %+v", advances, wantAdvances)
	}
}

func testType1FacePFA() []byte {
	notdef := encryptedTestCharString(t1prog(t1nums(0, 500), t1ops(13, 14)), 4)
	subr := encryptedTestCharString(t1prog(t1nums(50), t1ops(6, 11)), 4)
	glyph := encryptedTestCharString(
		t1prog(
			t1nums(0, 500), t1ops(13),
			t1nums(10, 20), t1ops(21),
			t1nums(0), t1ops(10),
			t1ops(14),
		),
		4,
	)

	private := []byte(fmt.Sprintf(`
/Private 8 dict dup begin
/lenIV 4 def
/Subrs 1 array
dup 0 %d RD `, len(subr)))
	private = append(private, subr...)
	private = append(private, []byte(` NP
/CharStrings 2 dict dup begin
/.notdef `)...)
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

	pfa := []byte(`%!PS-AdobeFont-1.0: MiniType1 1.0
/FontName /MiniType1 def
/FontMatrix [0.001 0 0 0.001 0 0] readonly def
/FontBBox [-10 -20 600 700] readonly def
/Encoding 256 array
dup 65 /A put
readonly def
currentfile eexec
`)
	pfa = append(pfa, []byte(hex.EncodeToString(encryptedEexecForTest(private)))...)
	pfa = append(pfa, []byte("\n0000000000000000\ncleartomark\n")...)
	return pfa
}

func testType1RectFacePFA() []byte {
	notdef := encryptedTestCharString(t1prog(t1nums(0, 500), t1ops(13, 14)), 4)
	glyph := encryptedTestCharString(
		t1prog(
			t1nums(0, 500), t1ops(13),
			t1nums(10, 10), t1ops(21),
			t1nums(100, 0, 0, 100, -100, 0), t1ops(5),
			t1ops(9, 14),
		),
		4,
	)

	private := []byte(`/Private 8 dict dup begin
/lenIV 4 def
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

	pfa := []byte(`%!PS-AdobeFont-1.0: MiniType1Rect 1.0
/FontName /MiniType1Rect def
/FontMatrix [0.001 0 0 0.001 0 0] readonly def
/FontBBox [0 0 200 200] readonly def
/Encoding 256 array
dup 65 /A put
readonly def
currentfile eexec
`)
	pfa = append(pfa, []byte(hex.EncodeToString(encryptedEexecForTest(private)))...)
	pfa = append(pfa, []byte("\n0000000000000000\ncleartomark\n")...)
	return pfa
}

func testType1PFB(pfa []byte) []byte {
	pfb := append([]byte{0x80, 1, byte(len(pfa)), byte(len(pfa) >> 8), byte(len(pfa) >> 16), byte(len(pfa) >> 24)}, pfa...)
	return append(pfb, 0x80, 3)
}

func type1HasNonZeroByte(buf []byte) bool {
	for _, b := range buf {
		if b != 0 {
			return true
		}
	}
	return false
}

type type1TestStream struct {
	data []byte
}

func (s type1TestStream) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(s.data)) {
		return 0, io.EOF
	}
	n := copy(p, s.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (s type1TestStream) Size() int64 {
	return int64(len(s.data))
}
