package type1

import (
	"bytes"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

func TestType1LoaderAPINilAndEmptyStreams(t *testing.T) {
	driver := NewLoader(nil)

	if driver.Handles(nil) {
		t.Fatal("Handles(nil) = true, want false")
	}
	if _, err := driver.LoadFace(nil); err == nil {
		t.Fatal("LoadFace(nil) succeeded")
	}

	empty := core.NewMemoryStream(nil)
	if driver.Handles(empty) {
		t.Fatal("Handles(empty stream) = true, want false")
	}
	if _, err := driver.LoadFace(empty); err == nil {
		t.Fatal("LoadFace(empty stream) succeeded")
	}
}

func TestType1LoaderAPIMalformedPFB(t *testing.T) {
	driver := NewLoader(nil)
	tests := []struct {
		name        string
		data        []byte
		wantHandles bool
	}{
		{
			name:        "truncated block header",
			data:        []byte{0x80, 1},
			wantHandles: true,
		},
		{
			name:        "unsupported block type",
			data:        []byte{0x80, 9, 0, 0, 0, 0},
			wantHandles: false,
		},
		{
			name:        "declared length exceeds data",
			data:        []byte{0x80, 1, 5, 0, 0, 0, 'a'},
			wantHandles: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := core.NewMemoryStream(tt.data)
			if got := driver.Handles(stream); got != tt.wantHandles {
				t.Fatalf("Handles() = %v, want %v", got, tt.wantHandles)
			}
			if _, err := driver.LoadFace(stream); err == nil {
				t.Fatal("LoadFace() succeeded for malformed PFB")
			}
		})
	}
}

func TestType1LoaderAPIPFBBinaryFirstBlock(t *testing.T) {
	pfb := type1LoaderAPIPFB(type1LoaderAPIPFBBlock(2, testType1FacePFA()))
	driver := NewLoader(nil)

	if !driver.Handles(core.NewMemoryStream(pfb)) {
		t.Fatal("Handles(PFB with binary first block) = false")
	}
	loaded, err := driver.LoadFace(core.NewMemoryStream(pfb))
	if err != nil {
		t.Fatalf("LoadFace(PFB with binary first block) failed: %v", err)
	}
	if got := loaded.GetNumGlyphs(); got != 2 {
		t.Fatalf("num glyphs = %d, want 2", got)
	}
}

func TestType1LoaderAPIPFAHeaderVariants(t *testing.T) {
	driver := NewLoader(nil)
	tests := []struct {
		name   string
		header string
	}{
		{
			name:   "PS-AdobeFont",
			header: "%!PS-AdobeFont-1.0: LoaderAPI 1.0",
		},
		{
			name:   "FontType1",
			header: "%!FontType1: LoaderAPI 1.0",
		},
		{
			name:   "resource font",
			header: "%!PS-Adobe-3.0 Resource-Font",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := type1LoaderAPIReplaceHeader(testType1FacePFA(), tt.header)
			if !driver.Handles(core.NewMemoryStream(data)) {
				t.Fatal("Handles() = false, want true")
			}
			loaded, err := driver.LoadFace(core.NewMemoryStream(data))
			if err != nil {
				t.Fatalf("LoadFace() failed: %v", err)
			}
			gid, err := loaded.GetGlyphIndex('A')
			if err != nil {
				t.Fatalf("GetGlyphIndex('A') failed: %v", err)
			}
			if gid != 1 {
				t.Fatalf("glyph index for A = %d, want 1", gid)
			}
		})
	}
}

func TestType1LoaderAPIRejectsGenericPostScriptHeader(t *testing.T) {
	driver := NewLoader(nil)
	stream := core.NewMemoryStream([]byte("%!PS-Adobe-3.0\n"))
	if driver.Handles(stream) {
		t.Fatal("Handles(generic PostScript header) = true, want false")
	}
}

func TestType1LoaderAPIInvalidGlyphIndices(t *testing.T) {
	face := type1LoaderAPILoadFace(t)
	invalid := []int{-1, face.GetNumGlyphs()}

	for _, glyphIndex := range invalid {
		if slot, err := face.LoadGlyph(glyphIndex, api.LoadDefault); err == nil {
			t.Fatalf("LoadGlyph(%d) succeeded with slot %T", glyphIndex, slot)
		}
		if _, _, err := face.GetGlyphMetrics(glyphIndex); err == nil {
			t.Fatalf("GetGlyphMetrics(%d) succeeded", glyphIndex)
		}
	}
}

func TestType1LoaderAPIUnmappedChars(t *testing.T) {
	face := type1LoaderAPILoadFace(t)
	tests := []rune{'B', rune(256), rune(-1)}

	for _, r := range tests {
		gid, err := face.GetGlyphIndex(r)
		if err != nil {
			t.Fatalf("GetGlyphIndex(%U) failed: %v", r, err)
		}
		if gid != 0 {
			t.Fatalf("GetGlyphIndex(%U) = %d, want 0", r, gid)
		}
	}
}

func TestType1LoaderAPISetPixelSizesZeroAndNegative(t *testing.T) {
	tests := []struct {
		name        string
		width       int
		height      int
		wantErr     bool
		wantAdvance int32
	}{
		{
			name:    "negative width",
			width:   -1,
			height:  12,
			wantErr: true,
		},
		{
			name:    "negative height",
			width:   12,
			height:  -1,
			wantErr: true,
		},
		{
			name:    "both zero",
			width:   0,
			height:  0,
			wantErr: true,
		},
		{
			name:        "zero width copies height",
			width:       0,
			height:      500,
			wantAdvance: 250 * 64,
		},
		{
			name:        "zero height copies width",
			width:       250,
			height:      0,
			wantAdvance: 125 * 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			face := type1LoaderAPILoadFace(t)
			err := face.SetPixelSizes(tt.width, tt.height)
			if tt.wantErr {
				if err == nil {
					t.Fatal("SetPixelSizes() succeeded")
				}
				return
			}
			if err != nil {
				t.Fatalf("SetPixelSizes() failed: %v", err)
			}
			advance, _, err := face.GetGlyphMetrics(1)
			if err != nil {
				t.Fatalf("GetGlyphMetrics(1) failed: %v", err)
			}
			if advance != tt.wantAdvance {
				t.Fatalf("advance = %d, want %d", advance, tt.wantAdvance)
			}
		})
	}
}

func TestType1LoaderAPIRenderFallbacks(t *testing.T) {
	tests := []struct {
		name string
		sys  api.FreetypeSystem
	}{
		{name: "nil system", sys: nil},
		{name: "system without rasterizer", sys: core.NewSystem()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			face, err := NewLoader(tt.sys).LoadFace(core.NewMemoryStream(testType1RectFacePFA()))
			if err != nil {
				t.Fatalf("LoadFace() failed: %v", err)
			}
			if err := face.SetPixelSizes(100, 100); err != nil {
				t.Fatalf("SetPixelSizes() failed: %v", err)
			}
			gid, err := face.GetGlyphIndex('A')
			if err != nil {
				t.Fatalf("GetGlyphIndex('A') failed: %v", err)
			}
			slot, err := face.LoadGlyph(gid, api.LoadRender|api.LoadNoHinting)
			if err != nil {
				t.Fatalf("LoadGlyph(render) failed: %v", err)
			}
			bitmap := slot.GetBitmap()
			if bitmap == nil {
				t.Fatal("rendered bitmap is nil")
			}
			if bitmap.GetWidth() == 0 || bitmap.GetRows() == 0 || bitmap.GetPitch() == 0 {
				t.Fatalf("empty bitmap geometry: width=%d rows=%d pitch=%d", bitmap.GetWidth(), bitmap.GetRows(), bitmap.GetPitch())
			}
			if !type1HasNonZeroByte(bitmap.GetBuffer()) {
				t.Fatal("rendered bitmap buffer is empty")
			}
		})
	}
}

func type1LoaderAPILoadFace(t *testing.T) api.Face {
	t.Helper()
	face, err := NewLoader(nil).LoadFace(core.NewMemoryStream(testType1FacePFA()))
	if err != nil {
		t.Fatalf("LoadFace() failed: %v", err)
	}
	return face
}

func type1LoaderAPIReplaceHeader(pfa []byte, header string) []byte {
	lineEnd := bytes.IndexByte(pfa, '\n')
	if lineEnd < 0 {
		panic("test Type 1 PFA missing header line")
	}
	out := make([]byte, 0, len(header)+1+len(pfa)-lineEnd-1)
	out = append(out, header...)
	out = append(out, '\n')
	out = append(out, pfa[lineEnd+1:]...)
	return out
}

func type1LoaderAPIPFB(blocks ...[]byte) []byte {
	var out []byte
	for _, block := range blocks {
		out = append(out, block...)
	}
	return append(out, 0x80, 3)
}

func type1LoaderAPIPFBBlock(blockType byte, payload []byte) []byte {
	size := len(payload)
	block := []byte{
		0x80,
		blockType,
		byte(size),
		byte(size >> 8),
		byte(size >> 16),
		byte(size >> 24),
	}
	return append(block, payload...)
}
