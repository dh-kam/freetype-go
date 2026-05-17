package bitmap

import (
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestParseBDF(t *testing.T) {
	mockBDF := `STARTFONT 2.1
FONT -test-font
SIZE 10 75 75
FONTBOUNDINGBOX 8 10 0 0
STARTPROPERTIES 1
COPYRIGHT "Mock"
ENDPROPERTIES
CHARS 1
STARTCHAR charA
ENCODING 65
SWIDTH 1000 0
DWIDTH 8 0
BBX 8 8 0 0
BITMAP
FF
81
81
81
81
81
81
FF
ENDCHAR
ENDFONT`

	bdf, err := ParseBDF(strings.NewReader(mockBDF))
	if err != nil {
		t.Fatalf("Failed to parse BDF: %v", err)
	}

	if bdf.Name != "-test-font" {
		t.Errorf("Expected font name -test-font, got %s", bdf.Name)
	}

	glyph, ok := bdf.Glyphs[65]
	if !ok {
		t.Fatalf("Glyph for encoding 65 (A) not found")
	}

	if glyph.Name != "charA" {
		t.Errorf("Expected glyph name charA, got %s", glyph.Name)
	}

	bitmap := glyph.Bitmap
	if bitmap.GetWidth() != 8 || bitmap.GetRows() != 8 {
		t.Errorf("Expected 8x8 bitmap, got %dx%d", bitmap.GetWidth(), bitmap.GetRows())
	}

	if bitmap.GetPixelMode() != api.MODE_MONO {
		t.Errorf("Expected pixel mode MONO, got %v", bitmap.GetPixelMode())
	}

	buffer := bitmap.GetBuffer()
	// Check first row (FF -> all bits set)
	for i := 0; i < 8; i++ {
		if buffer[i] != 255 {
			t.Errorf("Row 0, pixel %d should be 255, got %d", i, buffer[i])
		}
	}

	// Check second row (81 -> 10000001)
	row1 := buffer[8:16]
	if row1[0] != 255 || row1[7] != 255 {
		t.Errorf("Row 1 pixels 0 and 7 should be 255")
	}
	for i := 1; i < 7; i++ {
		if row1[i] != 0 {
			t.Errorf("Row 1 pixel %d should be 0", i)
		}
	}
}
