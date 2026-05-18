package type1

import (
	"encoding/hex"
	"fmt"
	"testing"
)

func TestStandardEncodingGlyphNameRepresentativeCodes(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{32, "space"},
		{33, "exclam"},
		{45, "hyphen"},
		{46, "period"},
		{48, "zero"},
		{65, "A"},
		{90, "Z"},
		{97, "a"},
		{122, "z"},
		{161, "exclamdown"},
		{174, "fi"},
		{175, "fl"},
		{251, "germandbls"},
	}

	for _, tt := range tests {
		if got := StandardEncodingGlyphName(tt.code); got != tt.want {
			t.Fatalf("StandardEncodingGlyphName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestStandardEncodingGlyphNameEmptyAndOutOfRange(t *testing.T) {
	for _, code := range []int{-1, 0, 31, 127, 128, 160, 176, 190, 192, 252, 255, 256} {
		if got := StandardEncodingGlyphName(code); got != "" {
			t.Fatalf("StandardEncodingGlyphName(%d) = %q, want empty string", code, got)
		}
	}
}

func TestStandardEncodingReturnsCopy(t *testing.T) {
	encoding := StandardEncoding()
	if len(encoding) != 256 {
		t.Fatalf("StandardEncoding length = %d, want 256", len(encoding))
	}
	if got := encoding[65]; got != "A" {
		t.Fatalf("StandardEncoding()[65] = %q, want A", got)
	}

	encoding[65] = "changed"
	if got := StandardEncodingGlyphName(65); got != "A" {
		t.Fatalf("mutating StandardEncoding copy changed lookup: got %q, want A", got)
	}
}

func TestISOLatin1EncodingGlyphNameRepresentativeCodes(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{32, "space"},
		{39, "quoteright"},
		{45, "minus"},
		{65, "A"},
		{90, "Z"},
		{97, "a"},
		{126, "asciitilde"},
		{144, "dotlessi"},
		{145, "grave"},
		{152, "dieresis"},
		{160, "space"},
		{161, "exclamdown"},
		{166, "brokenbar"},
		{169, "copyright"},
		{173, "hyphen"},
		{181, "mu"},
		{188, "onequarter"},
		{191, "questiondown"},
		{196, "Adieresis"},
		{198, "AE"},
		{215, "multiply"},
		{216, "Oslash"},
		{223, "germandbls"},
		{230, "ae"},
		{247, "divide"},
		{255, "ydieresis"},
	}

	for _, tt := range tests {
		if got := ISOLatin1EncodingGlyphName(tt.code); got != tt.want {
			t.Fatalf("ISOLatin1EncodingGlyphName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestISOLatin1EncodingGlyphNameEmptyAndOutOfRange(t *testing.T) {
	for _, code := range []int{-1, 0, 31, 127, 128, 143, 153, 156, 256} {
		if got := ISOLatin1EncodingGlyphName(code); got != "" {
			t.Fatalf("ISOLatin1EncodingGlyphName(%d) = %q, want empty string", code, got)
		}
	}
}

func TestISOLatin1EncodingReturnsCopy(t *testing.T) {
	encoding := ISOLatin1Encoding()
	if len(encoding) != 256 {
		t.Fatalf("ISOLatin1Encoding length = %d, want 256", len(encoding))
	}
	if got := encoding[45]; got != "minus" {
		t.Fatalf("ISOLatin1Encoding()[45] = %q, want minus", got)
	}

	encoding[45] = "changed"
	if got := ISOLatin1EncodingGlyphName(45); got != "minus" {
		t.Fatalf("mutating ISOLatin1Encoding copy changed lookup: got %q, want minus", got)
	}
}

func TestExpertEncodingGlyphNameRepresentativeCodes(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{32, "space"},
		{33, "exclamsmall"},
		{42, "twodotenleader"},
		{47, "fraction"},
		{48, "zerooldstyle"},
		{65, "asuperior"},
		{73, "isuperior"},
		{86, "ff"},
		{89, "ffi"},
		{91, "parenleftinferior"},
		{96, "Gravesmall"},
		{97, "Asmall"},
		{126, "Tildesmall"},
		{161, "exclamdownsmall"},
		{172, "Dotaccentsmall"},
		{175, "Macronsmall"},
		{178, "figuredash"},
		{188, "onequarter"},
		{200, "zerosuperior"},
		{224, "Agravesmall"},
		{247, "OEsmall"},
		{255, "Ydieresissmall"},
	}

	for _, tt := range tests {
		if got := ExpertEncodingGlyphName(tt.code); got != tt.want {
			t.Fatalf("ExpertEncodingGlyphName(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestExpertEncodingGlyphNameEmptyAndOutOfRange(t *testing.T) {
	for _, code := range []int{-1, 0, 31, 35, 64, 70, 72, 75, 80, 81, 85, 92, 127, 128, 159, 160, 171, 174, 177, 256} {
		if got := ExpertEncodingGlyphName(code); got != "" {
			t.Fatalf("ExpertEncodingGlyphName(%d) = %q, want empty string", code, got)
		}
	}
}

func TestExpertEncodingReturnsCopy(t *testing.T) {
	encoding := ExpertEncoding()
	if len(encoding) != 256 {
		t.Fatalf("ExpertEncoding length = %d, want 256", len(encoding))
	}
	if got := encoding[42]; got != "twodotenleader" {
		t.Fatalf("ExpertEncoding()[42] = %q, want twodotenleader", got)
	}

	encoding[42] = "changed"
	if got := ExpertEncodingGlyphName(42); got != "twodotenleader" {
		t.Fatalf("mutating ExpertEncoding copy changed lookup: got %q, want twodotenleader", got)
	}
}

func TestParseFontEncodingSelection(t *testing.T) {
	tests := []struct {
		name         string
		encodingDict string
		wantName     string
		wantGlyphs   map[int]string
	}{
		{
			name:         "default",
			encodingDict: "",
			wantName:     "StandardEncoding",
			wantGlyphs: map[int]string{
				45:  "hyphen",
				65:  "A",
				160: "",
				196: "tilde",
			},
		},
		{
			name: "custom",
			encodingDict: `/Encoding 256 array
dup 66 /B put
dup 196 /Adieresis put
readonly def
`,
			wantName: "CustomEncoding",
			wantGlyphs: map[int]string{
				45:  "",
				65:  "",
				66:  "B",
				196: "Adieresis",
			},
		},
		{
			name:         "iso latin1",
			encodingDict: "/Encoding /ISOLatin1Encoding def\n",
			wantName:     "ISOLatin1Encoding",
			wantGlyphs: map[int]string{
				45:  "minus",
				65:  "A",
				160: "space",
				196: "Adieresis",
				255: "ydieresis",
			},
		},
		{
			name:         "expert",
			encodingDict: "/Encoding /ExpertEncoding def\n",
			wantName:     "ExpertEncoding",
			wantGlyphs: map[int]string{
				32:  "space",
				42:  "twodotenleader",
				65:  "asuperior",
				160: "",
				161: "exclamdownsmall",
				255: "Ydieresissmall",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			font := parseEncodingTestFont(t, tt.encodingDict)
			if font.EncodingName != tt.wantName {
				t.Fatalf("EncodingName = %q, want %q", font.EncodingName, tt.wantName)
			}
			for code, want := range tt.wantGlyphs {
				if got := font.GlyphName(code); got != want {
					t.Fatalf("GlyphName(%d) = %q, want %q", code, got, want)
				}
			}
		})
	}
}

func parseEncodingTestFont(t *testing.T, encodingDict string) *Font {
	t.Helper()

	glyph := []byte{14}
	private := []byte(fmt.Sprintf(`
/Private 8 dict dup begin
/lenIV -1 def
/CharStrings 1 dict dup begin
/.notdef %d RD `, len(glyph)))
	private = append(private, glyph...)
	private = append(private, []byte(` ND
end
end
`)...)

	pfa := []byte(`%!PS-AdobeFont-1.0: EncodingTest 1.0
/FontName /EncodingTest def
`)
	pfa = append(pfa, []byte(encodingDict)...)
	pfa = append(pfa, []byte("currentfile eexec\n")...)
	pfa = append(pfa, []byte(hex.EncodeToString(encryptedEexecForTest(private)))...)
	pfa = append(pfa, []byte("\n0000000000000000\n")...)

	font, err := ParseFont(pfa)
	if err != nil {
		t.Fatalf("ParseFont failed: %v", err)
	}
	return font
}
