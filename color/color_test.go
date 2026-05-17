package color

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// mockStream implements api.Stream for testing
type mockStream struct {
	data []byte
}

func (m *mockStream) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 || int(off) >= len(m.data) {
		return 0, nil
	}
	n = copy(p, m.data[off:])
	return n, nil
}

func (m *mockStream) Size() int64 {
	return int64(len(m.data))
}

func TestParseCOLR_V1_PaintLinearGradient(t *testing.T) {
	var buf bytes.Buffer

	// COLR Header (14 bytes)
	binary.Write(&buf, binary.BigEndian, uint16(1)) // Version
	binary.Write(&buf, binary.BigEndian, uint16(0)) // NumBaseGlyphRecords
	binary.Write(&buf, binary.BigEndian, uint32(0)) // BaseGlyphRecordsOffset
	binary.Write(&buf, binary.BigEndian, uint32(0)) // LayerRecordsOffset
	binary.Write(&buf, binary.BigEndian, uint16(0)) // NumLayerRecords

	// COLR v1 Extensions (20 bytes)
	baseGlyphListOffset := uint32(34)
	binary.Write(&buf, binary.BigEndian, baseGlyphListOffset)
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))

	// BaseGlyphList Table (starts at 34)
	binary.Write(&buf, binary.BigEndian, uint32(1)) // numRecords

	// BaseGlyphPaintRecord[0]
	binary.Write(&buf, binary.BigEndian, uint16(42)) // GlyphID
	binary.Write(&buf, binary.BigEndian, uint32(10)) // PaintOffset (from start of BaseGlyphList = 34)

	// PaintLinearGradient (Format 4)
	buf.WriteByte(4) // format

	// colorLineOffset (24-bit), relative to this paint table (offset 44)
	// PaintLinearGradient table header is 1 + 3 + 12 = 16 bytes.
	// We'll put ColorLine exactly after it, so offset is 16.
	buf.Write([]byte{0x00, 0x00, 0x10})

	// x0, y0, x1, y1, x2, y2 (int16)
	binary.Write(&buf, binary.BigEndian, int16(100))
	binary.Write(&buf, binary.BigEndian, int16(200))
	binary.Write(&buf, binary.BigEndian, int16(300))
	binary.Write(&buf, binary.BigEndian, int16(400))
	binary.Write(&buf, binary.BigEndian, int16(500))
	binary.Write(&buf, binary.BigEndian, int16(600))

	// ColorLine (starts at offset 16 from paint table)
	buf.WriteByte(1)                                // extend = 1 (pad)
	binary.Write(&buf, binary.BigEndian, uint16(2)) // numStops = 2

	// Stop 1
	binary.Write(&buf, binary.BigEndian, int16(0))     // stopOffset (F2Dot14) = 0.0
	binary.Write(&buf, binary.BigEndian, uint16(0))    // paletteIndex
	binary.Write(&buf, binary.BigEndian, int16(16384)) // alpha (F2Dot14) = 1.0

	// Stop 2
	binary.Write(&buf, binary.BigEndian, int16(16384)) // stopOffset = 1.0
	binary.Write(&buf, binary.BigEndian, uint16(1))    // paletteIndex
	binary.Write(&buf, binary.BigEndian, int16(16384)) // alpha = 1.0

	stream := &mockStream{data: buf.Bytes()}

	colr, err := ParseCOLR(stream)
	if err != nil {
		t.Fatalf("ParseCOLR failed: %v", err)
	}

	record, ok := colr.BaseGlyphV1Records[42]
	if !ok {
		t.Fatalf("BaseGlyphV1Record for glyph 42 not found")
	}

	paint, ok := record.Paint.(*PaintLinearGradient)
	if !ok {
		t.Fatalf("Expected PaintLinearGradient, got %T", record.Paint)
	}

	if paint.X0 != 100 || paint.Y0 != 200 {
		t.Errorf("Unexpected x0/y0: %d/%d", paint.X0, paint.Y0)
	}

	if paint.ColorLine.Extend != 1 {
		t.Errorf("Unexpected extend: %d", paint.ColorLine.Extend)
	}

	if len(paint.ColorLine.ColorStops) != 2 {
		t.Fatalf("Expected 2 color stops, got %d", len(paint.ColorLine.ColorStops))
	}
}
