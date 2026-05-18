package type1

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

func TestLoadFaceAutoAttachesCompanionMetricsFromPathStream(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "AutoAttach.pfb")
	afmPath := filepath.Join(dir, "AutoAttach.afm")
	pfb := testType1PFB(testType1FacePFA())

	if err := os.WriteFile(fontPath, pfb, 0o600); err != nil {
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

	face := loadCompanionLoaderFace(t, companionLoaderTestStream{
		data: pfb,
		path: fontPath,
	})

	gid := companionLoaderGlyphIndex(t, face, 'A')
	advance, lsb, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 740*64 || lsb != 14*64 {
		t.Fatalf("metrics advance=%d lsb=%d, want advance=%d lsb=%d", advance, lsb, 740*64, 14*64)
	}

	glyphs, advances := face.Shape("AA")
	if len(glyphs) != 2 || glyphs[0] != gid || glyphs[1] != gid {
		t.Fatalf("glyphs = %v, want [%d %d]", glyphs, gid, gid)
	}
	if len(advances) != 2 || advances[0].X != (740-70)*64 || advances[1].X != 740*64 {
		t.Fatalf("advances = %+v, want [%d %d]", advances, (740-70)*64, 740*64)
	}
}

func TestLoadFaceMemoryStreamDoesNotAutoAttachCompanionMetrics(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "MemoryOnly.pfb")
	afmPath := filepath.Join(dir, "MemoryOnly.afm")
	pfb := testType1PFB(testType1FacePFA())

	if err := os.WriteFile(fontPath, pfb, 0o600); err != nil {
		t.Fatalf("WriteFile(font) failed: %v", err)
	}
	if err := os.WriteFile(afmPath, []byte(`
StartFontMetrics 4.1
StartCharMetrics 1
C 65 ; WX 740 ; N A ; B 14 0 700 700 ;
EndCharMetrics
EndFontMetrics
`), 0o600); err != nil {
		t.Fatalf("WriteFile(AFM) failed: %v", err)
	}

	face := loadCompanionLoaderFace(t, core.NewMemoryStream(pfb))

	gid := companionLoaderGlyphIndex(t, face, 'A')
	advance, _, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 500*64 {
		t.Fatalf("advance = %d, want fallback charstring advance %d", advance, 500*64)
	}
}

func TestLoadFacePathStreamMissingCompanionsIsOptional(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "NoCompanions.pfa")
	pfa := testType1FacePFA()

	if err := os.WriteFile(fontPath, pfa, 0o600); err != nil {
		t.Fatalf("WriteFile(font) failed: %v", err)
	}

	face := loadCompanionLoaderFace(t, companionLoaderTestStream{
		data: pfa,
		path: fontPath,
	})

	gid := companionLoaderGlyphIndex(t, face, 'A')
	advance, _, err := face.GetGlyphMetrics(gid)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 500*64 {
		t.Fatalf("advance = %d, want fallback charstring advance %d", advance, 500*64)
	}
}

func TestLoadFacePathStreamPropagatesCompanionErrors(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "BrokenCompanion.pfb")
	pfmPath := filepath.Join(dir, "BrokenCompanion.pfm")
	pfb := testType1PFB(testType1FacePFA())

	if err := os.WriteFile(fontPath, pfb, 0o600); err != nil {
		t.Fatalf("WriteFile(font) failed: %v", err)
	}
	if err := os.WriteFile(pfmPath, []byte{0}, 0o600); err != nil {
		t.Fatalf("WriteFile(PFM) failed: %v", err)
	}

	_, err := NewLoader(nil).LoadFace(companionLoaderTestStream{
		data: pfb,
		path: fontPath,
	})
	if err == nil {
		t.Fatal("LoadFace succeeded for malformed discovered PFM")
	}
	if !strings.Contains(err.Error(), "read PFM companion metrics") || !strings.Contains(err.Error(), pfmPath) {
		t.Fatalf("LoadFace error = %q, want context with discovered PFM path", err)
	}
}

func loadCompanionLoaderFace(t *testing.T, stream api.Stream) *Face {
	t.Helper()
	loaded, err := NewLoader(nil).LoadFace(stream)
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	face, ok := loaded.(*Face)
	if !ok {
		t.Fatalf("LoadFace returned %T, want *Face", loaded)
	}
	return face
}

func companionLoaderGlyphIndex(t *testing.T, face *Face, r rune) int {
	t.Helper()
	gid, err := face.GetGlyphIndex(r)
	if err != nil {
		t.Fatalf("GetGlyphIndex(%q) failed: %v", r, err)
	}
	if gid == 0 {
		t.Fatalf("GetGlyphIndex(%q) = 0, want non-zero glyph", r)
	}
	return gid
}

type companionLoaderTestStream struct {
	data []byte
	path string
}

func (s companionLoaderTestStream) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(s.data)) {
		return 0, io.EOF
	}
	n := copy(p, s.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (s companionLoaderTestStream) Size() int64 {
	return int64(len(s.data))
}

func (s companionLoaderTestStream) Type1FontPath() string {
	return s.path
}

var _ api.Stream = companionLoaderTestStream{}
var _ FontPathStream = companionLoaderTestStream{}
