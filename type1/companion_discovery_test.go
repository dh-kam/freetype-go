package type1

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverCompanionMetricsFilesProbesAdjacentSameStem(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "Demo.PFB")
	afmPath := filepath.Join(dir, "Demo.afm")
	pfmPath := filepath.Join(dir, "Demo.PFM")

	if err := os.WriteFile(fontPath, []byte("font"), 0o600); err != nil {
		t.Fatalf("WriteFile(font) failed: %v", err)
	}
	if err := os.WriteFile(afmPath, []byte("afm"), 0o600); err != nil {
		t.Fatalf("WriteFile(AFM) failed: %v", err)
	}
	if err := os.WriteFile(pfmPath, []byte("pfm"), 0o600); err != nil {
		t.Fatalf("WriteFile(PFM) failed: %v", err)
	}

	gotAFM, gotPFM, err := DiscoverCompanionMetricsFiles(fontPath)
	if err != nil {
		t.Fatalf("DiscoverCompanionMetricsFiles failed: %v", err)
	}
	if gotAFM != afmPath {
		t.Fatalf("AFM path = %q, want %q", gotAFM, afmPath)
	}
	if gotPFM != pfmPath {
		t.Fatalf("PFM path = %q, want %q", gotPFM, pfmPath)
	}
}

func TestReadCompanionMetricsForFontReadsDiscoveredFiles(t *testing.T) {
	pfmData, _, _ := testPFM()
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "reader.pfa")

	if err := os.WriteFile(fontPath, testType1FacePFA(), 0o600); err != nil {
		t.Fatalf("WriteFile(font) failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reader.AFM"), []byte(`
StartFontMetrics 4.1
FontName Reader-Regular
StartCharMetrics 1
C 65 ; WX 722 ; N A ; B 9 0 689 674 ;
EndCharMetrics
EndFontMetrics
`), 0o600); err != nil {
		t.Fatalf("WriteFile(AFM) failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reader.pfm"), pfmData, 0o600); err != nil {
		t.Fatalf("WriteFile(PFM) failed: %v", err)
	}

	var encoding [256]string
	encoding[65] = "A"
	encoding[67] = "C"

	metrics, err := ReadCompanionMetricsForFont(fontPath, encoding)
	if err != nil {
		t.Fatalf("ReadCompanionMetricsForFont failed: %v", err)
	}
	if metrics.AFM == nil {
		t.Fatal("AFM is nil, want discovered AFM")
	}
	if metrics.PFM == nil {
		t.Fatal("PFM is nil, want discovered PFM")
	}
	if width, ok := metrics.WidthByCode(65); !ok || width != 722 {
		t.Fatalf("WidthByCode(65) = %v, %v; want AFM width 722, true", width, ok)
	}
	if width, ok := metrics.WidthByCode(67); !ok || width != 620 {
		t.Fatalf("WidthByCode(67) = %v, %v; want PFM width 620, true", width, ok)
	}
}

func TestReadCompanionMetricsForFontMissingCompanionsIsOptional(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "missing-companions.pfb")
	if err := os.WriteFile(fontPath, []byte("font"), 0o600); err != nil {
		t.Fatalf("WriteFile(font) failed: %v", err)
	}

	var encoding [256]string
	encoding[65] = "A"

	metrics, err := ReadCompanionMetricsForFont(fontPath, encoding)
	if err != nil {
		t.Fatalf("ReadCompanionMetricsForFont failed: %v", err)
	}
	if metrics.AFM != nil {
		t.Fatalf("AFM present = %v, want false", metrics.AFM != nil)
	}
	if metrics.PFM != nil {
		t.Fatalf("PFM present = %v, want false", metrics.PFM != nil)
	}
	if name, ok := metrics.GlyphNameByCode(65); !ok || name != "A" {
		t.Fatalf("GlyphNameByCode(65) = %q, %v; want A, true", name, ok)
	}
}

func TestReadCompanionMetricsForFontWrapsDiscoveredFileErrors(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "broken.pfb")
	pfmPath := filepath.Join(dir, "broken.pfm")

	if err := os.WriteFile(fontPath, []byte("font"), 0o600); err != nil {
		t.Fatalf("WriteFile(font) failed: %v", err)
	}
	if err := os.WriteFile(pfmPath, []byte{0}, 0o600); err != nil {
		t.Fatalf("WriteFile(PFM) failed: %v", err)
	}

	if _, err := ReadCompanionMetricsForFont(fontPath, [256]string{}); err == nil {
		t.Fatal("ReadCompanionMetricsForFont succeeded for malformed discovered PFM")
	} else if !strings.Contains(err.Error(), "read PFM companion metrics") || !strings.Contains(err.Error(), pfmPath) {
		t.Fatalf("malformed PFM error = %q, want context with discovered PFM path", err)
	}
}

func TestReadCompanionMetricsForFontRejectsEmptyPath(t *testing.T) {
	if _, err := ReadCompanionMetricsForFont("", [256]string{}); err == nil {
		t.Fatal("ReadCompanionMetricsForFont succeeded for empty font path")
	} else if !strings.Contains(err.Error(), "empty Type 1 font path") {
		t.Fatalf("empty path error = %q, want empty path context", err)
	}
}

func TestType1FaceSetCompanionMetricsForFontUsesDiscoveredMetrics(t *testing.T) {
	face := companionTestFace(t)
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "MiniType1.pfb")
	afmPath := filepath.Join(dir, "MiniType1.afm")

	if err := os.WriteFile(fontPath, testType1PFB(testType1FacePFA()), 0o600); err != nil {
		t.Fatalf("WriteFile(font) failed: %v", err)
	}
	if err := os.WriteFile(afmPath, []byte(`
StartFontMetrics 4.1
StartCharMetrics 1
C 65 ; WX 740 ; N A ; B 14 0 700 700 ;
EndCharMetrics
StartKernPairs 1
KPX A A -70
EndKernPairs
EndFontMetrics
`), 0o600); err != nil {
		t.Fatalf("WriteFile(AFM) failed: %v", err)
	}

	if err := face.SetCompanionMetricsForFont(fontPath); err != nil {
		t.Fatalf("SetCompanionMetricsForFont failed: %v", err)
	}

	gid := companionGlyphIndex(t, face, 'A')
	advance, lsb, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 740*64 || lsb != 14*64 {
		t.Fatalf("metrics advance=%d lsb=%d, want advance=%d lsb=%d", advance, lsb, 740*64, 14*64)
	}

	glyphs, advances := face.Shape("AA")
	if len(glyphs) != 2 || glyphs[0] != 1 || glyphs[1] != 1 {
		t.Fatalf("glyphs = %v, want [1 1]", glyphs)
	}
	if len(advances) != 2 || advances[0].X != (740-70)*64 || advances[1].X != 740*64 {
		t.Fatalf("advances = %+v, want [%d %d]", advances, (740-70)*64, 740*64)
	}
}
