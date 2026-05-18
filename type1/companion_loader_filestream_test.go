package type1

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestLoadFaceFromCoreFileStreamAutoAttachesSameStemCompanions(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "CoreFileStream.pfb")
	afmPath := filepath.Join(dir, "CoreFileStream.afm")
	pfmPath := filepath.Join(dir, "CoreFileStream.pfm")
	pfb := testType1PFB(testType1FacePFA())
	pfmData, _, _ := testPFM()

	if err := os.WriteFile(fontPath, pfb, 0o600); err != nil {
		t.Fatalf("WriteFile(font) failed: %v", err)
	}
	if err := os.WriteFile(afmPath, []byte(`
StartFontMetrics 4.1
StartKernPairs 1
KPX A A -70
EndKernPairs
EndFontMetrics
`), 0o600); err != nil {
		t.Fatalf("WriteFile(AFM) failed: %v", err)
	}
	if err := os.WriteFile(pfmPath, pfmData, 0o600); err != nil {
		t.Fatalf("WriteFile(PFM) failed: %v", err)
	}

	file, err := os.Open(fontPath)
	if err != nil {
		t.Fatalf("Open(font) failed: %v", err)
	}
	defer file.Close()

	stream, err := core.NewFileStream(file)
	if err != nil {
		t.Fatalf("NewFileStream failed: %v", err)
	}

	face := loadCompanionLoaderFace(t, stream)
	gid := companionLoaderGlyphIndex(t, face, 'A')
	advance, _, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 600*64 {
		t.Fatalf("advance = %d, want PFM companion width %d", advance, 600*64)
	}

	glyphs, advances := face.Shape("AA")
	if len(glyphs) != 2 || glyphs[0] != gid || glyphs[1] != gid {
		t.Fatalf("glyphs = %v, want [%d %d]", glyphs, gid, gid)
	}
	if len(advances) != 2 || advances[0].X != (600-70)*64 || advances[1].X != 600*64 {
		t.Fatalf("advances = %+v, want [%d %d]", advances, (600-70)*64, 600*64)
	}
}
