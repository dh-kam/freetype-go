package sfnt

import (
	"encoding/binary"
	"github.com/dh-kam/freetype-go/core"
	"testing"
)

func TestHintingIntegration(t *testing.T) {
	// Create a dummy SFNT with head, maxp, loca, glyf, fpgm, prep, and cvt
	data := make([]byte, 2000)

	// Offset Table
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], 7) // numTables

	// Table Directory
	tables := []struct {
		tag    string
		offset uint32
		length uint32
	}{
		{"fpgm", 150, 10},
		{"glyf", 200, 100},
		{"head", 350, 54},
		{"loca", 450, 20},
		{"maxp", 550, 6},
		{"prep", 600, 10},
		{"cvt ", 650, 4},
	}

	for i, tbl := range tables {
		off := 12 + i*16
		copy(data[off:off+4], tbl.tag)
		binary.BigEndian.PutUint32(data[off+8:off+12], tbl.offset)
		binary.BigEndian.PutUint32(data[off+12:off+16], tbl.length)
	}

	// head table data
	binary.BigEndian.PutUint16(data[350+50:350+52], 0) // indexToLocFormat = 0

	// maxp table data
	binary.BigEndian.PutUint16(data[550+4:550+6], 2) // numGlyphs = 2

	// loca table data
	binary.BigEndian.PutUint16(data[450:452], 0)
	binary.BigEndian.PutUint16(data[452:454], 0)  // Glyph 0 empty
	binary.BigEndian.PutUint16(data[454:456], 50) // Glyph 1 length 100

	// fpgm: empty
	fpgm := []byte{}
	copy(data[150:150+len(fpgm)], fpgm)

	// prep: empty
	prep := []byte{}
	copy(data[600:600+len(prep)], prep)

	// glyf table data for Glyph 1
	glyphOffset := 200
	binary.BigEndian.PutUint16(data[glyphOffset:glyphOffset+2], 1)     // 1 contour
	binary.BigEndian.PutUint16(data[glyphOffset+10:glyphOffset+12], 0) // endPtOfContours[0] = 0 (1 point)

	// Instructions:
	// PUSHB[1] 0 (index)
	// PUSHW[1] 6400 (value)
	// WCVTP (pops val 6400, then idx 0)
	// SVTCA[x] (0x01)
	// PUSHB[1] 0 (Point index)
	// PUSHB[1] 0 (CVT index)
	// MIAP[no-round] (0x3E) (pops cvtIdx 0, then pIdx 0)
	instr := []byte{
		0xB0, 0, // PUSHB 0 (index)
		0xB8, 25, 0, // PUSHW 6400 (value)
		0x44,    // WCVTP
		0x01,    // SVTCA[x]
		0xB0, 0, // PUSHB 0 (point index)
		0xB0, 0, // PUSHB 0 (cvt index)
		0x3E, // MIAP[0]
	}
	binary.BigEndian.PutUint16(data[glyphOffset+12:glyphOffset+14], uint16(len(instr)))
	copy(data[glyphOffset+14:glyphOffset+14+len(instr)], instr)

	// Flags (1 point)
	flagsOffset := glyphOffset + 14 + len(instr)
	data[flagsOffset] = 0x01 // onCurve, X is 16-bit, Y is 16-bit

	// X coordinate (int16)
	binary.BigEndian.PutUint16(data[flagsOffset+1:flagsOffset+3], 10)
	// Y coordinate (int16)
	binary.BigEndian.PutUint16(data[flagsOffset+3:flagsOffset+5], 20)

	stream := &mockStream{data: data}
	sys := core.NewSystem()
	loader := NewLoader(sys)
	f, err := loader.LoadFace(stream)
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}

	slot, err := f.LoadGlyph(1, 0)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}

	outline := slot.GetOutline()
	points := outline.GetPoints()

	if points[0].X != 6400 {
		t.Errorf("expected X=6400 after hinting, got %d", points[0].X)
	}
	if points[0].Y != 20<<6 {
		t.Errorf("expected Y=1280, got %d", points[0].Y)
	}
}

func TestCFFLoading(t *testing.T) {
	data := make([]byte, 1000)

	// Offset Table
	binary.BigEndian.PutUint32(data[0:4], 0x4F54544F) // 'OTTO' for CFF
	binary.BigEndian.PutUint16(data[4:6], 3)          // numTables

	// Table Directory: head, maxp, CFF
	tables := []struct {
		tag    string
		offset uint32
		length uint32
	}{
		{"CFF ", 100, 200},
		{"head", 350, 54},
		{"maxp", 550, 6},
	}

	for i, tbl := range tables {
		off := 12 + i*16
		copy(data[off:off+4], tbl.tag)
		binary.BigEndian.PutUint32(data[off+8:off+12], tbl.offset)
		binary.BigEndian.PutUint32(data[off+12:off+16], tbl.length)
	}

	// head
	binary.BigEndian.PutUint16(data[350+18:350+20], 1000) // unitsPerEm

	// maxp
	binary.BigEndian.PutUint16(data[550+4:550+6], 1) // numGlyphs

	// CFF table (at 100)
	cffBase := 100
	data[cffBase+0] = 1 // major
	data[cffBase+1] = 0 // minor
	data[cffBase+2] = 4 // hdrSize
	data[cffBase+3] = 1 // offSize

	// Name INDEX (at 104)
	binary.BigEndian.PutUint16(data[cffBase+4:], 0) // count=0

	// Top DICT INDEX (at 106)
	binary.BigEndian.PutUint16(data[cffBase+6:], 1) // count=1
	data[cffBase+8] = 1                             // offSize
	data[cffBase+9] = 1                             // offset 1
	data[cffBase+10] = 5                            // offset 5 (length 4)
	// Data (at 111): 28, 0, 50, 17 (CharStrings at relative offset 50)
	copy(data[cffBase+11:], []byte{28, 0, 50, 17})

	// String INDEX (at 115)
	binary.BigEndian.PutUint16(data[cffBase+15:], 0)

	// Global Subr INDEX (at 117)
	binary.BigEndian.PutUint16(data[cffBase+17:], 0)

	// CharStrings INDEX (at 150 = 100 + 50)
	csBase := cffBase + 50
	binary.BigEndian.PutUint16(data[csBase:], 1) // count=1
	data[csBase+2] = 1                           // offSize
	data[csBase+3] = 1                           // offset 1
	data[csBase+4] = 5                           // offset 5 (length 4)
	// CharString data (at csBase + 5): rmoveto 10, 20 + endchar
	copy(data[csBase+5:], []byte{149, 159, 21, 14})

	stream := &mockStream{data: data}
	sys := core.NewSystem()
	loader := NewLoader(sys)
	f, err := loader.LoadFace(stream)
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}

	if f.GetNumGlyphs() != 1 {
		t.Errorf("expected 1 glyph, got %d", f.GetNumGlyphs())
	}

	slot, err := f.LoadGlyph(0, 0)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}

	outline := slot.GetOutline()
	points := outline.GetPoints()
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	if points[0].X != 10<<6 || points[0].Y != 20<<6 {
		t.Errorf("expected (640, 1280), got (%d, %d)", points[0].X, points[0].Y)
	}
}
