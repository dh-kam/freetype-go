package type1

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestParseFontDictionariesAndCharStrings(t *testing.T) {
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
	eexec := encryptedEexecForTest(private)

	pfa := []byte(`%!PS-AdobeFont-1.0: MiniType1 1.0
/FontName /MiniType1 def
/FontMatrix [0.001 0 0 0.001 0 0] readonly def
/FontBBox [-10 -20 600 700] readonly def
/Encoding 256 array
dup 65 /A put
readonly def
currentfile eexec
`)
	pfa = append(pfa, []byte(hex.EncodeToString(eexec))...)
	pfa = append(pfa, []byte("\n0000000000000000\ncleartomark\n")...)

	font, err := ParseFont(pfa)
	if err != nil {
		t.Fatalf("ParseFont failed: %v", err)
	}
	if font.FontName != "MiniType1" {
		t.Fatalf("FontName = %q", font.FontName)
	}
	if font.FontMatrix != ([6]float64{0.001, 0, 0, 0.001, 0, 0}) {
		t.Fatalf("FontMatrix = %v", font.FontMatrix)
	}
	if font.FontBBox != ([4]float64{-10, -20, 600, 700}) {
		t.Fatalf("FontBBox = %v", font.FontBBox)
	}
	if got := font.GlyphName(65); got != "A" {
		t.Fatalf("Encoding[65] = %q, want A", got)
	}
	if len(font.Subrs) != 1 {
		t.Fatalf("Subrs length = %d, want 1", len(font.Subrs))
	}
	if _, ok := font.CharStrings[".notdef"]; !ok {
		t.Fatal("missing .notdef charstring")
	}

	sideBearing, width, seac, err := font.DecodeGlyphMetrics("A")
	if err != nil {
		t.Fatalf("DecodeGlyphMetrics failed: %v", err)
	}
	if sideBearing != (api.Vector{}) {
		t.Fatalf("side bearing = %+v", sideBearing)
	}
	if width != (api.Vector{X: 500 * 64}) {
		t.Fatalf("width = %+v", width)
	}
	if seac != nil {
		t.Fatalf("unexpected seac metadata: %+v", seac)
	}

	outline, err := font.DecodeGlyph("A")
	if err != nil {
		t.Fatalf("DecodeGlyph failed: %v", err)
	}
	wantPoints := []api.Vector{
		{X: 10 * 64, Y: 20 * 64},
		{X: 60 * 64, Y: 20 * 64},
	}
	if !vectorsEqual(outline.Outline.Points, wantPoints) {
		t.Fatalf("points = %+v, want %+v", outline.Outline.Points, wantPoints)
	}
}

func TestParseFontSupportsUnencryptedCharStrings(t *testing.T) {
	glyph := t1prog(t1nums(0, 250), t1ops(13, 14))
	private := []byte(fmt.Sprintf(`
/Private 8 dict dup begin
/lenIV -1 def
/CharStrings 1 dict dup begin
/A %d RD `, len(glyph)))
	private = append(private, glyph...)
	private = append(private, []byte(` ND
end
end
`)...)

	pfa := []byte(`%!PS-AdobeFont-1.0: PlainType1 1.0
/FontName /PlainType1 def
currentfile eexec
`)
	pfa = append(pfa, []byte(hex.EncodeToString(encryptedEexecForTest(private)))...)
	pfa = append(pfa, []byte("\n0000000000000000\n")...)

	font, err := ParseFont(pfa)
	if err != nil {
		t.Fatalf("ParseFont failed: %v", err)
	}
	_, width, _, err := font.DecodeGlyphMetrics("A")
	if err != nil {
		t.Fatalf("DecodeGlyphMetrics failed: %v", err)
	}
	if width != (api.Vector{X: 250 * 64}) {
		t.Fatalf("width = %+v", width)
	}
	if got := font.GlyphName(65); got != "A" {
		t.Fatalf("default Encoding[65] = %q, want StandardEncoding A", got)
	}
}

func TestParseFontCustomEncodingDoesNotInheritStandardEncoding(t *testing.T) {
	glyph := t1prog(t1nums(0, 250), t1ops(13, 14))
	private := []byte(fmt.Sprintf(`
/Private 8 dict dup begin
/lenIV -1 def
/CharStrings 1 dict dup begin
/B %d RD `, len(glyph)))
	private = append(private, glyph...)
	private = append(private, []byte(` ND
end
end
`)...)

	pfa := []byte(`%!PS-AdobeFont-1.0: CustomEncodingType1 1.0
/FontName /CustomEncodingType1 def
/Encoding 256 array
dup 66 /B put
readonly def
currentfile eexec
`)
	pfa = append(pfa, []byte(hex.EncodeToString(encryptedEexecForTest(private)))...)
	pfa = append(pfa, []byte("\n0000000000000000\n")...)

	font, err := ParseFont(pfa)
	if err != nil {
		t.Fatalf("ParseFont failed: %v", err)
	}
	if got := font.GlyphName(65); got != "" {
		t.Fatalf("custom Encoding[65] = %q, want empty", got)
	}
	if got := font.GlyphName(66); got != "B" {
		t.Fatalf("custom Encoding[66] = %q, want B", got)
	}
}

func TestExtractEexecIgnoresCommentedEexec(t *testing.T) {
	plain := []byte("pad!private")
	encrypted := encryptType1Bytes(plain, 55665)
	data := []byte("% eexec inside comment must be ignored\ncurrentfile eexec ")
	data = append(data, []byte(hex.EncodeToString(encrypted))...)
	data = append(data, []byte("\n0000000000000000\n")...)

	got, err := ExtractEexec(data)
	if err != nil {
		t.Fatalf("ExtractEexec failed: %v", err)
	}
	if string(got) != "private" {
		t.Fatalf("private dict = %q, want private", got)
	}
}

func TestParseFontRejectsMissingCharStrings(t *testing.T) {
	private := []byte(`/Private 8 dict dup begin /lenIV 4 def end`)
	data := []byte(`%!PS-AdobeFont-1.0: Bad 1.0
/FontName /Bad def
currentfile eexec
`)
	data = append(data, []byte(hex.EncodeToString(encryptedEexecForTest(private)))...)
	data = append(data, []byte("\n0000000000000000\n")...)

	if _, err := ParseFont(data); err == nil {
		t.Fatal("ParseFont unexpectedly succeeded")
	}
}

func encryptedTestCharString(program []byte, lenIV int) []byte {
	plain := make([]byte, 0, lenIV+len(program))
	for i := 0; i < lenIV; i++ {
		plain = append(plain, byte(i+1))
	}
	plain = append(plain, program...)
	return encryptType1Bytes(plain, 4330)
}

func encryptedEexecForTest(private []byte) []byte {
	plain := append([]byte{1, 2, 3, 4}, private...)
	return encryptType1Bytes(plain, 55665)
}
