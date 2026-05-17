package sfnt

import (
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestLoadFace(t *testing.T) {
	// Create a dummy SFNT
	data := make([]byte, 200)

	// Offset Table
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], 2) // numTables

	// Table Directory
	// head
	copy(data[12:16], "head")
	binary.BigEndian.PutUint32(data[20:24], 44) // offset
	binary.BigEndian.PutUint32(data[24:28], 54) // length

	// maxp
	copy(data[28:32], "maxp")
	binary.BigEndian.PutUint32(data[36:40], 98) // offset
	binary.BigEndian.PutUint32(data[40:44], 6)  // length

	// head table data (at 44)
	binary.BigEndian.PutUint16(data[44+50:44+52], 1) // indexToLocFormat = 1 (long)

	// maxp table data (at 98)
	binary.BigEndian.PutUint16(data[98+4:98+6], 42) // numGlyphs = 42

	stream := &mockStream{data: data}
	sys := &mockSystem{}
	loader := NewLoader(sys)

	f, err := loader.LoadFace(stream)
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}

	if f.GetNumGlyphs() != 42 {
		t.Errorf("expected 42 glyphs, got %d", f.GetNumGlyphs())
	}

	// Verify indexToLocFormat
	sfntFace := f.(*Face)
	if sfntFace.head.IndexToLocFormat != 1 {
		t.Errorf("expected indexToLocFormat 1, got %d", sfntFace.head.IndexToLocFormat)
	}
}

func TestReadUint16RejectsShortRead(t *testing.T) {
	_, err := readUint16(&mockStream{data: []byte{0x12}}, 0)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("got %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

func TestSetPixelSizes(t *testing.T) {
	f := &Face{}
	if err := f.SetPixelSizes(0, 19); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	if f.xPPEM != 19 || f.yPPEM != 19 || f.pointSize != 19 {
		t.Fatalf("unexpected size state: xPPEM=%d yPPEM=%d pointSize=%d", f.xPPEM, f.yPPEM, f.pointSize)
	}
	if err := f.SetPixelSizes(12, 34); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	if f.xPPEM != 12 || f.yPPEM != 34 || f.pointSize != 34 {
		t.Fatalf("unexpected size state: xPPEM=%d yPPEM=%d pointSize=%d", f.xPPEM, f.yPPEM, f.pointSize)
	}
	if err := f.SetPixelSizes(0, 0); err == nil {
		t.Fatal("expected zero pixel size to fail")
	}
	if err := f.SetPixelSizes(-1, 12); err == nil {
		t.Fatal("expected negative pixel size to fail")
	}
}

func TestGetGlyphMetricsRejectsMissingOrShortHmtx(t *testing.T) {
	f := &Face{
		maxp: MaxpTable{NumGlyphs: 2},
		hhea: HheaTable{NumberOfHMetrics: 2},
	}
	if _, _, err := f.GetGlyphMetrics(0); err == nil {
		t.Fatal("expected missing hmtx to fail")
	}

	f.hmtx = HmtxTable{
		HMetrics: []HMetric{{AdvanceWidth: 1000, LeftSideBearing: 20}},
	}
	if _, _, err := f.GetGlyphMetrics(1); err == nil {
		t.Fatal("expected short hmtx to fail")
	}
}

func TestLoadGlyphSimple(t *testing.T) {
	// Create a dummy SFNT with head, maxp, loca, and glyf
	data := make([]byte, 1000)

	// Offset Table
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], 4) // numTables

	// Table Directory entries (sorted by tag: glyf, head, loca, maxp)
	// glyf
	copy(data[12:16], "glyf")
	binary.BigEndian.PutUint32(data[20:24], 200)
	binary.BigEndian.PutUint32(data[24:28], 100)
	// head
	copy(data[28:32], "head")
	binary.BigEndian.PutUint32(data[36:40], 300)
	binary.BigEndian.PutUint32(data[40:44], 54)
	// loca
	copy(data[44:48], "loca")
	binary.BigEndian.PutUint32(data[52:56], 400)
	binary.BigEndian.PutUint32(data[56:60], 20)
	// maxp
	copy(data[60:64], "maxp")
	binary.BigEndian.PutUint32(data[68:72], 500)
	binary.BigEndian.PutUint32(data[72:76], 6)

	// head table data (indexToLocFormat = 0, short)
	binary.BigEndian.PutUint16(data[300+50:300+52], 0)

	// maxp table data (numGlyphs = 2)
	binary.BigEndian.PutUint16(data[500+4:500+6], 2)

	// loca table data (offsets for glyph 0, 1, and end of 1)
	// Glyph 0: offset 0, Glyph 1: offset 10, end: offset 50
	binary.BigEndian.PutUint16(data[400:402], 0)
	binary.BigEndian.PutUint16(data[402:404], 5)  // 10 / 2
	binary.BigEndian.PutUint16(data[404:406], 25) // 50 / 2

	// glyf table data for Glyph 1 (starts at glyfOffset + 10)
	glyphOffset := 200 + 10
	// numberOfContours = 1
	binary.BigEndian.PutUint16(data[glyphOffset:glyphOffset+2], 1)
	// Bounding box (xMin, yMin, xMax, yMax)
	binary.BigEndian.PutUint16(data[glyphOffset+2:glyphOffset+4], 0)
	binary.BigEndian.PutUint16(data[glyphOffset+4:glyphOffset+6], 0)
	binary.BigEndian.PutUint16(data[glyphOffset+6:glyphOffset+8], 100)
	binary.BigEndian.PutUint16(data[glyphOffset+8:glyphOffset+10], 100)

	// endPtsOfContours[0] = 2 (3 points: 0, 1, 2)
	binary.BigEndian.PutUint16(data[glyphOffset+10:glyphOffset+12], 2)
	// instructionLength = 0
	binary.BigEndian.PutUint16(data[glyphOffset+12:glyphOffset+14], 0)

	// Flags (3 points)
	data[glyphOffset+14] = 0x01 | 0x02 | 0x04 | 0x10 | 0x20 // 0x37
	data[glyphOffset+15] = 0x01 | 0x02 | 0x04 | 0x00 | 0x20 // 0x25
	data[glyphOffset+16] = 0x01 | 0x02 | 0x04 | 0x10 | 0x00 // 0x17

	// X coordinates (3 bytes)
	data[glyphOffset+17] = 10 // x0 = 10
	data[glyphOffset+18] = 5  // x1 = 10 - 5 = 5
	data[glyphOffset+19] = 20 // x2 = 5 + 20 = 25

	// Y coordinates (3 bytes)
	data[glyphOffset+20] = 30 // y0 = 30
	data[glyphOffset+21] = 10 // y1 = 30 + 10 = 40
	data[glyphOffset+22] = 15 // y2 = 40 - 15 = 25

	stream := &mockStream{data: data}
	sys := &mockSystem{}
	loader := NewLoader(sys)
	f, _ := loader.LoadFace(stream)

	slot, err := f.LoadGlyph(1, 0)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	if got := f.GetGlyphSlot(); got != slot {
		t.Fatalf("GetGlyphSlot returned %T, want loaded slot %T", got, slot)
	}

	outline := slot.GetOutline()
	if outline == nil {
		t.Fatal("Outline is nil")
	}

	points := outline.GetPoints()
	if len(points) != 7 {
		t.Errorf("expected 7 points, got %d", len(points))
	}

	// Point 0: (10, 30) -> (640, 1920) in 26.6
	if points[0].X != 640 || points[0].Y != 1920 {
		t.Errorf("point 0 mismatch: got (%d, %d)", points[0].X, points[0].Y)
	}
	// Point 1: (5, 40) -> (320, 2560)
	if points[1].X != 320 || points[1].Y != 2560 {
		t.Errorf("point 1 mismatch: got (%d, %d)", points[1].X, points[1].Y)
	}
	// Point 2: (25, 25) -> (1600, 1600)
	if points[2].X != 1600 || points[2].Y != 1600 {
		t.Errorf("point 2 mismatch: got (%d, %d)", points[2].X, points[2].Y)
	}
}
