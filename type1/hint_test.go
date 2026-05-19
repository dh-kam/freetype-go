package type1

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestBuildHintContextScalesStemsAndPrivateHints(t *testing.T) {
	font := &Font{
		StdHW:            []float64{12},
		StdVW:            []float64{10},
		StemSnapH:        []float64{12, 14},
		StemSnapV:        []float64{10, 20},
		BlueValues:       []float64{-20, 0, 480, 500},
		OtherBlues:       []float64{-200, -180},
		BlueScale:        0.039625,
		HasBlueScale:     true,
		BlueShift:        7,
		HasBlueShift:     true,
		BlueFuzz:         1,
		HasBlueFuzz:      true,
		ForceBold:        true,
		HasForceBold:     true,
		LanguageGroup:    1,
		HasLanguageGroup: true,
	}
	result := &CharStringResult{
		Hints: []CharStringHint{
			{
				Kind:     CharStringHintStem,
				Operator: "hstem",
				Stems: []CharStringStemHint{
					{Orientation: CharStringStemHorizontal, Position: 20, Width: 12},
				},
			},
			{
				Kind:     CharStringHintStem,
				Operator: "vstem",
				Stems: []CharStringStemHint{
					{Orientation: CharStringStemVertical, Position: 11, Width: 10},
				},
			},
		},
	}

	context := BuildHintContext(font, result, 32<<16, 64<<16)
	if got, want := len(context.Stems), 2; got != want {
		t.Fatalf("stem count = %d, want %d", got, want)
	}
	hstem := context.Stems[0]
	if hstem.DesignMin != 20*64 || hstem.DesignMax != 32*64 || hstem.ScaledMin != 20*64 || hstem.ScaledMax != 32*64 {
		t.Fatalf("hstem edges = %+v, want design/scaled 20..32 font units", hstem)
	}
	if !hstem.PrivateWidthMatch {
		t.Fatalf("hstem PrivateWidthMatch = false, want true")
	}
	vstem := context.Stems[1]
	if vstem.DesignMin != 11*64 || vstem.DesignMax != 21*64 || vstem.ScaledMin != 11*32 || vstem.ScaledMax != 21*32 {
		t.Fatalf("vstem edges = %+v, want design 11..21 and scaled half-size", vstem)
	}
	if vstem.ScaledWidth != 10*32 || !vstem.PrivateWidthMatch {
		t.Fatalf("vstem width/match = %d/%v, want %d/true", vstem.ScaledWidth, vstem.PrivateWidthMatch, 10*32)
	}
	if got, want := len(context.Private.StdVW), 1; got != want {
		t.Fatalf("StdVW count = %d, want %d", got, want)
	}
	if context.Private.StdVW[0] != (ScaledHintMetric{Design: 10, Scaled: 10 * 32}) {
		t.Fatalf("StdVW[0] = %+v, want design 10 scaled %d", context.Private.StdVW[0], 10*32)
	}
	if got, want := len(context.Private.BlueValues), 2; got != want {
		t.Fatalf("BlueValues zones = %d, want %d", got, want)
	}
	if context.Private.BlueValues[0] != (ScaledHintZone{DesignMin: -20, DesignMax: 0, ScaledMin: -20 * 64, ScaledMax: 0}) {
		t.Fatalf("BlueValues[0] = %+v, want -20..0", context.Private.BlueValues[0])
	}
	if !context.Private.HasBlueScale || !context.Private.HasBlueShift || !context.Private.HasBlueFuzz || !context.Private.HasForceBold || !context.Private.HasLanguageGroup {
		t.Fatalf("private scalar hint flags not preserved: %+v", context.Private)
	}
}

func TestType1HintingSnapsExactStemEdgesOnlyWhenHintedAndScaled(t *testing.T) {
	loaded, err := NewLoader(nil).LoadFace(type1TestStream{data: testType1HintedStemPFA()})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	if err := loaded.SetPixelSizes(500, 500); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	gid, err := loaded.GetGlyphIndex('A')
	if err != nil {
		t.Fatalf("GetGlyphIndex failed: %v", err)
	}

	noHintSlot, err := loaded.LoadGlyph(gid, api.LoadNoHinting)
	if err != nil {
		t.Fatalf("LoadGlyph no-hinting failed: %v", err)
	}
	wantNoHint := []api.Vector{
		{X: 11 * 32, Y: 0},
		{X: 21 * 32, Y: 0},
		{X: 21 * 32, Y: 50 * 32},
		{X: 11 * 32, Y: 50 * 32},
	}
	if points := noHintSlot.GetOutline().GetPoints(); !vectorsEqual(points, wantNoHint) {
		t.Fatalf("LoadNoHinting points = %+v, want %+v", points, wantNoHint)
	}

	noScaleSlot, err := loaded.LoadGlyph(gid, api.LoadNoScale)
	if err != nil {
		t.Fatalf("LoadGlyph no-scale failed: %v", err)
	}
	wantNoScale := []api.Vector{
		{X: 11 * 64, Y: 0},
		{X: 21 * 64, Y: 0},
		{X: 21 * 64, Y: 50 * 64},
		{X: 11 * 64, Y: 50 * 64},
	}
	if points := noScaleSlot.GetOutline().GetPoints(); !vectorsEqual(points, wantNoScale) {
		t.Fatalf("LoadNoScale points = %+v, want %+v", points, wantNoScale)
	}

	hintedSlot, err := loaded.LoadGlyph(gid, api.LoadDefault)
	if err != nil {
		t.Fatalf("LoadGlyph hinted failed: %v", err)
	}
	wantHinted := []api.Vector{
		{X: 6 * 64, Y: 0},
		{X: 11 * 64, Y: 0},
		{X: 11 * 64, Y: 50 * 32},
		{X: 6 * 64, Y: 50 * 32},
	}
	if points := hintedSlot.GetOutline().GetPoints(); !vectorsEqual(points, wantHinted) {
		t.Fatalf("hinted points = %+v, want %+v", points, wantHinted)
	}
}

func testType1HintedStemPFA() []byte {
	notdef := encryptedTestCharString(t1prog(t1nums(0, 500), t1ops(13, 14)), 4)
	glyph := encryptedTestCharString(
		t1prog(
			t1nums(0, 500), t1ops(13),
			t1nums(11, 10), t1ops(3),
			t1nums(11, 0), t1ops(21),
			t1nums(10, 0, 0, 50, -10, 0), t1ops(5),
			t1ops(9, 14),
		),
		4,
	)

	private := []byte(`/Private 10 dict dup begin
/lenIV 4 def
/StdVW [10] def
/StemSnapV [10] def
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

	pfa := []byte(`%!PS-AdobeFont-1.0: HintedStem 1.0
/FontName /HintedStem def
/FontMatrix [0.001 0 0 0.001 0 0] readonly def
/FontBBox [0 0 100 100] readonly def
/Encoding 256 array
dup 65 /A put
readonly def
currentfile eexec
`)
	pfa = append(pfa, []byte(hex.EncodeToString(encryptedEexecForTest(private)))...)
	pfa = append(pfa, []byte("\n0000000000000000\ncleartomark\n")...)
	return pfa
}
