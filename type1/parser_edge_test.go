package type1

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func TestParserEdgeParseFontPFBASCIIAndBinaryBlocks(t *testing.T) {
	private := parserEdgePrivateDict([]parserEdgeGlyph{
		{name: ".notdef", data: []byte{14}},
		{name: "A", data: []byte{14}},
	}, -1)
	encrypted := parserEdgeEncryptedEexec(private)

	clear := []byte(`%!PS-AdobeFont-1.0: EdgePFB 1.0
/FontName /EdgePFB def
/Encoding 256 array
dup 65 /A put
readonly def
currentfile eexec
`)
	pfb := parserEdgePFB(
		parserEdgePFBBlock(1, clear[:32]),
		parserEdgePFBBlock(1, clear[32:]),
		parserEdgePFBBlock(2, encrypted),
		parserEdgePFBBlock(1, []byte("\n0000000000000000\ncleartomark\n")),
	)

	font, err := ParseFont(pfb)
	if err != nil {
		t.Fatalf("ParseFont failed for multi-block PFB: %v", err)
	}
	if font.FontName != "EdgePFB" {
		t.Fatalf("FontName = %q, want EdgePFB", font.FontName)
	}
	if got := font.GlyphName(65); got != "A" {
		t.Fatalf("Encoding[65] = %q, want A", got)
	}
	if !bytes.Equal(font.CharStrings["A"], []byte{14}) {
		t.Fatalf("A charstring = %v, want endchar", font.CharStrings["A"])
	}
}

func TestParserEdgeExtractEexecPFAHexWithWhitespaceAndSentinel(t *testing.T) {
	private := []byte("/Private 8 dict dup begin /lenIV -1 def end")
	encrypted := parserEdgeEncryptedEexec(private)
	hexBody := hex.EncodeToString(encrypted)
	data := []byte("currentfile eexec\n")
	for i := 0; i < len(hexBody); i += 6 {
		end := i + 6
		if end > len(hexBody) {
			end = len(hexBody)
		}
		data = append(data, hexBody[i:end]...)
		data = append(data, ' ', '\r', '\n', '\t')
	}
	data = append(data, []byte("0000000000000000\ncleartomark\n")...)

	got, err := ExtractEexec(data)
	if err != nil {
		t.Fatalf("ExtractEexec failed for whitespace-heavy PFA hex: %v", err)
	}
	if !bytes.Equal(got, private) {
		t.Fatalf("private dict = %q, want %q", got, private)
	}
}

func TestParserEdgeParseFontRDSkipsExactlyOneWhitespaceByte(t *testing.T) {
	tests := []struct {
		name      string
		separator byte
	}{
		{name: "newline", separator: '\n'},
		{name: "carriage return", separator: '\r'},
		{name: "space", separator: ' '},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			private := []byte("/Private 8 dict dup begin\n/lenIV -1 def\n/CharStrings 1 dict dup begin\n/A 3 RD")
			private = append(private, tt.separator)
			private = append(private, '\r', ' ', 14)
			private = append(private, []byte(" ND\nend\nend\n")...)

			font, err := ParseFont(parserEdgePFA(private))
			if err != nil {
				t.Fatalf("ParseFont failed: %v", err)
			}
			want := []byte{'\r', ' ', 14}
			if !bytes.Equal(font.CharStrings["A"], want) {
				t.Fatalf("A charstring = %v, want %v; RD must skip exactly one whitespace byte", font.CharStrings["A"], want)
			}
		})
	}
}

func TestParserEdgeParseFontGlyphNamesPreserveCharStringOrder(t *testing.T) {
	private := parserEdgePrivateDict([]parserEdgeGlyph{
		{name: "B", data: []byte{14}},
		{name: ".notdef", data: []byte{14}},
		{name: "A", data: []byte{14}},
		{name: "space", data: []byte{14}},
	}, -1)

	font, err := ParseFont(parserEdgePFA(private))
	if err != nil {
		t.Fatalf("ParseFont failed: %v", err)
	}
	want := []string{"B", ".notdef", "A", "space"}
	if !parserEdgeStringSlicesEqual(font.GlyphNames, want) {
		t.Fatalf("GlyphNames = %v, want CharStrings declaration order %v", font.GlyphNames, want)
	}
}

func TestParserEdgeParseFontRejectsCharStringShorterThanLenIV(t *testing.T) {
	private := []byte("/Private 8 dict dup begin\n/lenIV 4 def\n/CharStrings 1 dict dup begin\n/A 3 RD abc ND\nend\nend\n")

	_, err := ParseFont(parserEdgePFBForPrivate(private))
	if err == nil {
		t.Fatal("ParseFont unexpectedly accepted a charstring shorter than lenIV")
	}
	if !strings.Contains(err.Error(), "charstring data shorter than lenIV") {
		t.Fatalf("ParseFont error = %v, want lenIV shortage error", err)
	}
}

type parserEdgeGlyph struct {
	name string
	data []byte
}

func parserEdgePrivateDict(glyphs []parserEdgeGlyph, lenIV int) []byte {
	var out []byte
	out = append(out, []byte("/Private 8 dict dup begin\n")...)
	out = append(out, []byte("/lenIV ")...)
	out = append(out, []byte(parserEdgeItoa(lenIV))...)
	out = append(out, []byte(" def\n")...)
	out = append(out, []byte("/CharStrings ")...)
	out = append(out, []byte(parserEdgeItoa(len(glyphs)))...)
	out = append(out, []byte(" dict dup begin\n")...)
	for _, glyph := range glyphs {
		out = append(out, '/')
		out = append(out, glyph.name...)
		out = append(out, ' ')
		out = append(out, []byte(parserEdgeItoa(len(glyph.data)))...)
		out = append(out, []byte(" RD ")...)
		out = append(out, glyph.data...)
		out = append(out, []byte(" ND\n")...)
	}
	out = append(out, []byte("end\nend\n")...)
	return out
}

func parserEdgePFA(private []byte) []byte {
	out := []byte(`%!PS-AdobeFont-1.0: EdgePFA 1.0
/FontName /EdgePFA def
currentfile eexec
`)
	out = append(out, []byte(hex.EncodeToString(parserEdgeEncryptedEexec(private)))...)
	out = append(out, []byte("\n0000000000000000\ncleartomark\n")...)
	return out
}

func parserEdgePFBForPrivate(private []byte) []byte {
	clear := []byte(`%!PS-AdobeFont-1.0: EdgePFB 1.0
/FontName /EdgePFB def
currentfile eexec
`)
	return parserEdgePFB(
		parserEdgePFBBlock(1, clear),
		parserEdgePFBBlock(2, parserEdgeEncryptedEexec(private)),
		parserEdgePFBBlock(1, []byte("\n0000000000000000\ncleartomark\n")),
	)
}

func parserEdgePFB(blocks ...[]byte) []byte {
	var out []byte
	for _, block := range blocks {
		out = append(out, block...)
	}
	out = append(out, 0x80, 3)
	return out
}

func parserEdgePFBBlock(blockType byte, data []byte) []byte {
	out := []byte{0x80, blockType, byte(len(data)), byte(len(data) >> 8), byte(len(data) >> 16), byte(len(data) >> 24)}
	out = append(out, data...)
	return out
}

func parserEdgeEncryptedEexec(private []byte) []byte {
	plain := append([]byte{1, 2, 3, 4}, private...)
	return parserEdgeEncryptType1Bytes(plain, 55665)
}

func parserEdgeEncryptType1Bytes(data []byte, seed uint16) []byte {
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

func parserEdgeItoa(v int) string {
	if v == 0 {
		return "0"
	}
	negative := v < 0
	if negative {
		v = -v
	}
	var digits [20]byte
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
	}
	if negative {
		i--
		digits[i] = '-'
	}
	return string(digits[i:])
}

func parserEdgeStringSlicesEqual(a, b []string) bool {
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
