package sfnt

import (
	"encoding/binary"
	"testing"
)

func TestCMapFormat4Internal(t *testing.T) {
	// Create a dummy cmap format 4 table
	// segCount = 2
	// Seg 0: 0x0041-0x0041 (A) -> GID 1
	// Seg 1: 0xFFFF-0xFFFF -> GID 0 (required)

	segCount := 2
	length := 16 + segCount*8
	data := make([]byte, length)

	binary.BigEndian.PutUint16(data[0:2], 4)                  // format
	binary.BigEndian.PutUint16(data[2:4], uint16(length))     // length
	binary.BigEndian.PutUint16(data[4:6], 0)                  // language
	binary.BigEndian.PutUint16(data[6:8], uint16(segCount*2)) // segCountX2
	binary.BigEndian.PutUint16(data[8:10], 2)                 // searchRange
	binary.BigEndian.PutUint16(data[10:12], 1)                // entrySelector
	binary.BigEndian.PutUint16(data[12:14], 2)                // rangeShift

	// endCount
	binary.BigEndian.PutUint16(data[14:16], 0x0041)
	binary.BigEndian.PutUint16(data[16:18], 0xFFFF)

	// reservedPad
	binary.BigEndian.PutUint16(data[18:20], 0)

	// startCount
	binary.BigEndian.PutUint16(data[20:22], 0x0041)
	binary.BigEndian.PutUint16(data[22:24], 0xFFFF)

	// idDelta
	binary.BigEndian.PutUint16(data[24:26], 0xFFC0) // idDelta[0] = -64
	binary.BigEndian.PutUint16(data[26:28], 0)

	// idRangeOffset
	binary.BigEndian.PutUint16(data[28:30], 0)
	binary.BigEndian.PutUint16(data[30:32], 0)

	stream := &mockStream{data: data}
	cmap, err := parseCMapSubtable(stream, 0)
	if err != nil {
		t.Fatalf("parseCMapSubtable failed: %v", err)
	}

	gid := cmap.Lookup('A')
	if gid != 1 {
		t.Errorf("expected GID 1 for 'A', got %d", gid)
	}

	gid = cmap.Lookup('B')
	if gid != 0 {
		t.Errorf("expected GID 0 for 'B', got %d", gid)
	}
}

func TestHmtxInternal(t *testing.T) {
	// 2 glyphs, 1 hMetric
	// hMetric 0: advance 1000, lsb 100
	// Glyph 1: lsb 200 (uses advance of hMetric 0)

	hmtxData := make([]byte, 4+2)
	binary.BigEndian.PutUint16(hmtxData[0:2], 1000) // advance
	binary.BigEndian.PutUint16(hmtxData[2:4], 100)  // lsb
	binary.BigEndian.PutUint16(hmtxData[4:6], 200)  // lsb for glyph 1

	stream := &mockStream{data: hmtxData}
	hmtx, err := parseHmtx(stream, 2, 1)
	if err != nil {
		t.Fatalf("parseHmtx failed: %v", err)
	}

	if len(hmtx.HMetrics) != 1 || hmtx.HMetrics[0].AdvanceWidth != 1000 || hmtx.HMetrics[0].LeftSideBearing != 100 {
		t.Errorf("hmtx.HMetrics mismatch: %+v", hmtx.HMetrics)
	}
	if len(hmtx.LeftSideBearings) != 1 || hmtx.LeftSideBearings[0] != 200 {
		t.Errorf("hmtx.LeftSideBearings mismatch: %+v", hmtx.LeftSideBearings)
	}
}

func TestFaceCMapAndHmtx(t *testing.T) {
	data := make([]byte, 2000)

	// Offset Table
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], 5) // numTables

	// Table Directory
	tables := []struct {
		tag    string
		offset uint32
		length uint32
	}{
		{"cmap", 100, 100},
		{"head", 350, 54},
		{"hhea", 450, 36},
		{"hmtx", 550, 20},
		{"maxp", 650, 6},
	}

	for i, tbl := range tables {
		off := 12 + i*16
		copy(data[off:off+4], tbl.tag)
		binary.BigEndian.PutUint32(data[off+8:off+12], tbl.offset)
		binary.BigEndian.PutUint32(data[off+12:off+16], tbl.length)
	}

	// cmap table (at 100)
	// Format 4 subtable at 100 + 12 = 112
	binary.BigEndian.PutUint16(data[100:102], 0)  // version
	binary.BigEndian.PutUint16(data[102:104], 1)  // numTables
	binary.BigEndian.PutUint16(data[104:106], 3)  // platformID Windows
	binary.BigEndian.PutUint16(data[106:108], 1)  // encodingID Unicode BMP
	binary.BigEndian.PutUint32(data[108:112], 12) // offset from cmap start

	sub := data[112:]
	binary.BigEndian.PutUint16(sub[0:2], 4)        // format
	binary.BigEndian.PutUint16(sub[2:4], 32)       // length
	binary.BigEndian.PutUint16(sub[6:8], 4)        // segCountX2
	binary.BigEndian.PutUint16(sub[14:16], 0x0041) // endCount[0]
	binary.BigEndian.PutUint16(sub[16:18], 0xFFFF) // endCount[1]
	binary.BigEndian.PutUint16(sub[20:22], 0x0041) // startCount[0]
	binary.BigEndian.PutUint16(sub[22:24], 0xFFFF) // startCount[1]
	binary.BigEndian.PutUint16(sub[24:26], 0xFFC0) // idDelta[0] = -64

	// head
	binary.BigEndian.PutUint16(data[350+18:350+20], 1000) // unitsPerEm

	// hhea
	binary.BigEndian.PutUint16(data[450+34:450+36], 1) // numberOfHMetrics

	// hmtx
	binary.BigEndian.PutUint16(data[550:552], 1000)    // advance
	binary.BigEndian.PutUint16(data[550+2:552+2], 100) // lsb
	binary.BigEndian.PutUint16(data[550+4:550+6], 200) // lsb for glyph 1

	// maxp
	binary.BigEndian.PutUint16(data[650+4:650+6], 2) // numGlyphs

	stream := &mockStream{data: data}
	sys := &mockSystem{}
	loader := NewLoader(sys)
	f, err := loader.LoadFace(stream)
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}

	gid, err := f.GetGlyphIndex('A')
	if err != nil {
		t.Fatalf("GetGlyphIndex failed: %v", err)
	}
	if gid != 1 {
		t.Errorf("expected GID 1, got %d", gid)
	}

	advance, lsb, err := f.GetGlyphMetrics(1)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 1000 || lsb != 200 {
		t.Errorf("expected (1000, 200), got (%d, %d)", advance, lsb)
	}
}
