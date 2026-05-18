package type1

import (
	"strings"
	"testing"
)

func TestParseAFMBasicMetrics(t *testing.T) {
	data := []byte(`
StartFontMetrics 4.1
FontName Test-Regular
FullName Test Regular
FamilyName Test Family
Weight Medium
ItalicAngle -12.5
IsFixedPitch false
FontBBox -10 -20 600 700
StartCharMetrics 3
C 32 ; WX 250 ; N space ; B 0 0 0 0 ;
C 65 ; WX 722 ; N A ; B 9 0 689 674 ;
C -1 ; W0X 556 ; N fi ; B 50 -10 500 700 ;
EndCharMetrics
EndFontMetrics
`)

	afm, err := ParseAFM(data)
	if err != nil {
		t.Fatalf("ParseAFM failed: %v", err)
	}
	if afm.FontName != "Test-Regular" {
		t.Fatalf("FontName = %q, want Test-Regular", afm.FontName)
	}
	if afm.FullName != "Test Regular" {
		t.Fatalf("FullName = %q, want Test Regular", afm.FullName)
	}
	if afm.FamilyName != "Test Family" {
		t.Fatalf("FamilyName = %q, want Test Family", afm.FamilyName)
	}
	if afm.Weight != "Medium" {
		t.Fatalf("Weight = %q, want Medium", afm.Weight)
	}
	if afm.ItalicAngle != -12.5 {
		t.Fatalf("ItalicAngle = %v, want -12.5", afm.ItalicAngle)
	}
	if afm.IsFixedPitch {
		t.Fatalf("IsFixedPitch = true, want false")
	}
	if afm.FontBBox != ([4]float64{-10, -20, 600, 700}) {
		t.Fatalf("FontBBox = %v", afm.FontBBox)
	}

	if len(afm.CharMetrics) != 3 {
		t.Fatalf("CharMetrics len = %d, want 3", len(afm.CharMetrics))
	}
	if got := afm.CharMetrics[0]; got.Code != 32 || got.Name != "space" || got.WidthX != 250 || got.BBox != ([4]float64{0, 0, 0, 0}) {
		t.Fatalf("space metric = %#v", got)
	}
	if got := afm.CharMetrics[1]; got.Code != 65 || got.Name != "A" || got.WidthX != 722 || got.BBox != ([4]float64{9, 0, 689, 674}) {
		t.Fatalf("A metric = %#v", got)
	}
	if got := afm.CharMetrics[2]; got.Code != -1 || got.Name != "fi" || got.WidthX != 556 || got.BBox != ([4]float64{50, -10, 500, 700}) {
		t.Fatalf("fi metric = %#v", got)
	}
}

func TestParseAFMKernPairs(t *testing.T) {
	data := []byte(`
StartFontMetrics 4.1
StartKernPairs 2
KPX A V -80
KPX T o -40.5
EndKernPairs
EndFontMetrics
`)

	afm, err := ParseAFM(data)
	if err != nil {
		t.Fatalf("ParseAFM failed: %v", err)
	}
	if len(afm.KernPairs) != 2 {
		t.Fatalf("KernPairs len = %d, want 2", len(afm.KernPairs))
	}
	if got := afm.KernPairs[0]; got != (AFMKernPair{Left: "A", Right: "V", X: -80}) {
		t.Fatalf("KernPairs[0] = %#v", got)
	}
	if got := afm.KernPairs[1]; got != (AFMKernPair{Left: "T", Right: "o", X: -40.5}) {
		t.Fatalf("KernPairs[1] = %#v", got)
	}
}

func TestAFMLookupsPreferFirstMetricDeclaration(t *testing.T) {
	data := []byte(`
StartFontMetrics 4.1
StartCharMetrics 3
C 65 ; WX 700 ; N A ; B 0 0 700 700 ;
C 66 ; WX 610 ; N A ; B 0 0 610 700 ;
C 65 ; WX 710 ; N A.alt ; B 0 0 710 700 ;
EndCharMetrics
EndFontMetrics
`)

	afm, err := ParseAFM(data)
	if err != nil {
		t.Fatalf("ParseAFM failed: %v", err)
	}
	metric, ok := afm.GlyphMetricByName("A")
	if !ok {
		t.Fatalf("GlyphMetricByName(A) missing")
	}
	if metric.Code != 65 || metric.WidthX != 700 {
		t.Fatalf("GlyphMetricByName(A) = %#v, want first declaration", metric)
	}
	metric, ok = afm.GlyphMetricByCode(65)
	if !ok {
		t.Fatalf("GlyphMetricByCode(65) missing")
	}
	if metric.Name != "A" || metric.WidthX != 700 {
		t.Fatalf("GlyphMetricByCode(65) = %#v, want first declaration", metric)
	}
	if width, ok := afm.WidthXByName("A"); !ok || width != 700 {
		t.Fatalf("WidthXByName(A) = %v, %v; want 700, true", width, ok)
	}
	if width, ok := afm.WidthXByCode(65); !ok || width != 700 {
		t.Fatalf("WidthXByCode(65) = %v, %v; want 700, true", width, ok)
	}
}

func TestAFMLookupsCodeMinusOneGlyphNames(t *testing.T) {
	data := []byte(`
StartFontMetrics 4.1
StartCharMetrics 2
C -1 ; WX 556 ; N fi ; B 50 -10 500 700 ;
C -1 ; WX 333 ; N dotlessi ; B 20 0 310 690 ;
EndCharMetrics
EndFontMetrics
`)

	afm, err := ParseAFM(data)
	if err != nil {
		t.Fatalf("ParseAFM failed: %v", err)
	}
	metric, ok := afm.GlyphMetricByName("dotlessi")
	if !ok {
		t.Fatalf("GlyphMetricByName(dotlessi) missing")
	}
	if metric.Code != -1 || metric.WidthX != 333 {
		t.Fatalf("GlyphMetricByName(dotlessi) = %#v", metric)
	}
	if width, ok := afm.WidthXByName("fi"); !ok || width != 556 {
		t.Fatalf("WidthXByName(fi) = %v, %v; want 556, true", width, ok)
	}
	metric, ok = afm.GlyphMetricByCode(-1)
	if !ok {
		t.Fatalf("GlyphMetricByCode(-1) missing")
	}
	if metric.Name != "fi" || metric.WidthX != 556 {
		t.Fatalf("GlyphMetricByCode(-1) = %#v, want first declaration", metric)
	}
}

func TestAFMLookupsAbsentMetrics(t *testing.T) {
	data := []byte(`
StartFontMetrics 4.1
StartCharMetrics 1
C 65 ; WX 700 ; N A ; B 0 0 700 700 ;
EndCharMetrics
StartKernPairs 1
KPX A V -80
EndKernPairs
EndFontMetrics
`)

	afm, err := ParseAFM(data)
	if err != nil {
		t.Fatalf("ParseAFM failed: %v", err)
	}
	if metric, ok := afm.GlyphMetricByName("B"); ok {
		t.Fatalf("GlyphMetricByName(B) = %#v, true; want missing", metric)
	}
	if metric, ok := afm.GlyphMetricByCode(66); ok {
		t.Fatalf("GlyphMetricByCode(66) = %#v, true; want missing", metric)
	}
	if width, ok := afm.WidthXByName("B"); ok {
		t.Fatalf("WidthXByName(B) = %v, true; want missing", width)
	}
	if width, ok := afm.WidthXByCode(66); ok {
		t.Fatalf("WidthXByCode(66) = %v, true; want missing", width)
	}
	if x, ok := afm.KernX("A", "W"); ok {
		t.Fatalf("KernX(A, W) = %v, true; want missing", x)
	}
}

func TestAFMKernLookup(t *testing.T) {
	data := []byte(`
StartFontMetrics 4.1
StartKernPairs 3
KPX A V -80
KPX T o -40.5
KPX A V -70
EndKernPairs
EndFontMetrics
`)

	afm, err := ParseAFM(data)
	if err != nil {
		t.Fatalf("ParseAFM failed: %v", err)
	}
	if x, ok := afm.KernX("A", "V"); !ok || x != -80 {
		t.Fatalf("KernX(A, V) = %v, %v; want -80, true", x, ok)
	}
	if x, ok := afm.KernX("T", "o"); !ok || x != -40.5 {
		t.Fatalf("KernX(T, o) = %v, %v; want -40.5, true", x, ok)
	}
}

func TestParseAFMIgnoresUnknownRecords(t *testing.T) {
	data := []byte(`
StartFontMetrics 4.1
Notice ignored top-level text
StartCharMetrics 1
C 65 ; WX 700 ; N A ; B 0 0 700 700 ; L Aacute acute ;
EndCharMetrics
StartKernData
StartTrackKern 0
EndTrackKern
EndKernData
EndFontMetrics
`)

	afm, err := ParseAFM(data)
	if err != nil {
		t.Fatalf("ParseAFM failed: %v", err)
	}
	if len(afm.CharMetrics) != 1 {
		t.Fatalf("CharMetrics len = %d, want 1", len(afm.CharMetrics))
	}
	if got := afm.CharMetrics[0]; got.Name != "A" || got.WidthX != 700 {
		t.Fatalf("CharMetrics[0] = %#v", got)
	}
}

func TestParseAFMMalformedValues(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "italic angle",
			data: "ItalicAngle nope\n",
			want: "ItalicAngle",
		},
		{
			name: "font bbox",
			data: "FontBBox 0 nope 1 2\n",
			want: "FontBBox",
		},
		{
			name: "char metric count",
			data: "StartCharMetrics many\n",
			want: "StartCharMetrics",
		},
		{
			name: "char code",
			data: "StartCharMetrics 1\nC nope ; WX 1 ; N A ; B 0 0 1 1 ;\n",
			want: "C",
		},
		{
			name: "width",
			data: "StartCharMetrics 1\nC 65 ; WX nope ; N A ; B 0 0 1 1 ;\n",
			want: "WX",
		},
		{
			name: "glyph bbox",
			data: "StartCharMetrics 1\nC 65 ; WX 1 ; N A ; B 0 nope 1 1 ;\n",
			want: "B",
		},
		{
			name: "kern pair count",
			data: "StartKernPairs many\n",
			want: "StartKernPairs",
		},
		{
			name: "kern pair value",
			data: "StartKernPairs 1\nKPX A V nope\n",
			want: "KPX",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAFM([]byte(tt.data))
			if err == nil {
				t.Fatalf("ParseAFM succeeded")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to mention %q", err.Error(), tt.want)
			}
		})
	}
}
