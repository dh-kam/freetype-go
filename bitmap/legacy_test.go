package bitmap

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestParsePCF_Basic(t *testing.T) {
	buf := new(bytes.Buffer)

	// Magic
	buf.Write([]byte{0x01, 'p', 'c', 'f'})

	// Table count
	binary.Write(buf, binary.LittleEndian, uint32(2))

	// Table 1: Metrics
	binary.Write(buf, binary.LittleEndian, uint32(PCF_METRICS))
	binary.Write(buf, binary.LittleEndian, uint32(PCF_DEFAULT_FORMAT)) // Format
	binary.Write(buf, binary.LittleEndian, uint32(12+4))               // Size
	binary.Write(buf, binary.LittleEndian, uint32(4+4+2*16))           // Offset (after header and 2 table entries)

	// Table 2: Bitmaps
	binary.Write(buf, binary.LittleEndian, uint32(PCF_BITMAPS))
	binary.Write(buf, binary.LittleEndian, uint32(PCF_DEFAULT_FORMAT)) // Format
	binary.Write(buf, binary.LittleEndian, uint32(100))                // Size
	binary.Write(buf, binary.LittleEndian, uint32(200))                // Offset

	// Metrics data
	binary.Write(buf, binary.LittleEndian, uint32(PCF_DEFAULT_FORMAT)) // Format
	binary.Write(buf, binary.LittleEndian, uint32(0))                  // Count = 0

	pcf, err := ParsePCF(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to parse PCF TOC: %v", err)
	}

	if len(pcf.Tables) != 2 {
		t.Errorf("Expected 2 tables, got %d", len(pcf.Tables))
	}

	if _, ok := pcf.Tables[PCF_METRICS]; !ok {
		t.Errorf("Metrics table not found")
	}
}

func TestParsePCF_WithMetrics(t *testing.T) {
	buf := new(bytes.Buffer)

	// Magic
	buf.Write([]byte{0x01, 'p', 'c', 'f'})

	// Table count
	binary.Write(buf, binary.LittleEndian, uint32(1))

	// Table 1: Metrics (Compressed)
	binary.Write(buf, binary.LittleEndian, uint32(PCF_METRICS))
	binary.Write(buf, binary.LittleEndian, uint32(PCF_DEFAULT_FORMAT|PCF_COMPRESSED_METRICS)) // Format
	binary.Write(buf, binary.LittleEndian, uint32(2+1*5))                                     // Size (count + 1 metric)
	binary.Write(buf, binary.LittleEndian, uint32(4+4+1*16))                                  // Offset

	// Metrics data
	// Format
	binary.Write(buf, binary.LittleEndian, uint32(PCF_DEFAULT_FORMAT|PCF_COMPRESSED_METRICS))
	// Count
	binary.Write(buf, binary.LittleEndian, uint16(1))
	// 1 Compressed metric: Left, Right, Width, Ascent, Descent
	// Each byte is value + 0x80. So for value 0, it's 0x80.
	buf.Write([]byte{0x80 - 2, 0x80 + 6, 0x80 + 8, 0x80 + 10, 0x80 + 2})

	pcf, err := ParsePCF(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to parse PCF: %v", err)
	}

	if len(pcf.Metrics) != 1 {
		t.Errorf("Expected 1 metric, got %d", len(pcf.Metrics))
	}

	m := pcf.Metrics[0]
	if m.Left != -2 || m.Right != 6 || m.Width != 8 || m.Ascent != 10 || m.Desent != 2 {
		t.Errorf("Unexpected metrics: %+v", m)
	}
}

func TestParseFNT_Basic(t *testing.T) {
	header := FNTHeader{
		Version:   0x300,
		FirstChar: 32,
		LastChar:  33, // 2 chars: 32 and 33
		PixHeight: 16,
	}

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, header)

	// Char table entries for 32 and 33
	binary.Write(buf, binary.LittleEndian, uint16(8))   // Width char 32
	binary.Write(buf, binary.LittleEndian, uint32(100)) // Offset char 32
	binary.Write(buf, binary.LittleEndian, uint16(10))  // Width char 33
	binary.Write(buf, binary.LittleEndian, uint32(200)) // Offset char 33

	fnt, err := ParseFNT(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Failed to parse FNT: %v", err)
	}

	if fnt.Header.Version != 0x300 {
		t.Errorf("Expected version 0x300, got 0x%x", fnt.Header.Version)
	}

	if len(fnt.Chars) != 2 {
		t.Errorf("Expected 2 char entries, got %d", len(fnt.Chars))
	}

	if fnt.Chars[0].Width != 8 {
		t.Errorf("Expected char 32 width 8, got %d", fnt.Chars[0].Width)
	}

	bm, err := fnt.GetBitmap(32)
	if err != nil {
		t.Errorf("Failed to get bitmap for char 32: %v", err)
	}
	if bm.GetWidth() != 8 || bm.GetRows() != 16 {
		t.Errorf("Expected 8x16 bitmap, got %dx%d", bm.GetWidth(), bm.GetRows())
	}
}
