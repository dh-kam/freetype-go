package layout

import (
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestGSUB_Substitute_Single(t *testing.T) {
	// GSUB header (10 bytes)
	// ScriptList (10 bytes)
	// FeatureList (10 bytes)
	// LookupList (4 bytes)
	// LookupTable (8 bytes)
	// SingleSubst (10 bytes)
	// Coverage (6 bytes)
	data := make([]byte, 100)

	// GSUB Header
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 20)
	binary.BigEndian.PutUint16(data[8:10], 30)

	// ScriptList (emptyish)
	binary.BigEndian.PutUint16(data[10:12], 0)

	// FeatureList (emptyish)
	binary.BigEndian.PutUint16(data[20:22], 0)

	// LookupList
	binary.BigEndian.PutUint16(data[30:32], 1)
	binary.BigEndian.PutUint16(data[32:34], 4) // Offset to LookupTable from LookupList start (30+4=34)

	// LookupTable at 34
	binary.BigEndian.PutUint16(data[34:36], 1) // Type 1
	binary.BigEndian.PutUint16(data[36:38], 0) // Flag
	binary.BigEndian.PutUint16(data[38:40], 1) // Subtable count
	binary.BigEndian.PutUint16(data[40:42], 8) // Subtable offset from LookupTable start (34+8=42)

	// SingleSubst at 42 (Format 1)
	binary.BigEndian.PutUint16(data[42:44], 1)  // Format 1
	binary.BigEndian.PutUint16(data[44:46], 6)  // Coverage offset from SingleSubst start (42+6=48)
	binary.BigEndian.PutUint16(data[46:48], 10) // DeltaGlyphID

	// Coverage at 48
	binary.BigEndian.PutUint16(data[48:50], 1) // Format 1
	binary.BigEndian.PutUint16(data[50:52], 1) // Count
	binary.BigEndian.PutUint16(data[52:54], 1) // GlyphID 1

	gsub, err := ParseGSUB(data)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	glyphs := []int{1, 2, 3}
	result := gsub.Substitute(glyphs)

	if result[0] != 11 {
		t.Errorf("expected glyph 11, got %d", result[0])
	}
	if result[1] != 2 {
		t.Errorf("expected glyph 2, got %d", result[1])
	}
}

func TestGSUB_Substitute_Ligature(t *testing.T) {
	data := make([]byte, 200)

	// GSUB Header
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 20)
	binary.BigEndian.PutUint16(data[8:10], 30)

	// ScriptList (emptyish) at 10
	binary.BigEndian.PutUint16(data[10:12], 0)

	// FeatureList (emptyish) at 20
	binary.BigEndian.PutUint16(data[20:22], 0)

	// LookupList at 30
	binary.BigEndian.PutUint16(data[30:32], 1)
	binary.BigEndian.PutUint16(data[32:34], 4) // Offset to LookupTable from LookupList start (30+4=34)

	// LookupTable at 34
	binary.BigEndian.PutUint16(data[34:36], 4) // Type 4 (Ligature Substitution)
	binary.BigEndian.PutUint16(data[36:38], 0) // Flag
	binary.BigEndian.PutUint16(data[38:40], 1) // Subtable count
	binary.BigEndian.PutUint16(data[40:42], 8) // Subtable offset from LookupTable start (34+8=42)

	// LigatureSubst at 42 (Format 1)
	binary.BigEndian.PutUint16(data[42:44], 1)
	binary.BigEndian.PutUint16(data[44:46], 22) // Coverage offset from LigatureSubst start (42+22=64)
	binary.BigEndian.PutUint16(data[46:48], 1)  // LigSet count
	binary.BigEndian.PutUint16(data[48:50], 8)  // LigSet offset from LigatureSubst start (42+8=50)

	// LigSet at 50
	binary.BigEndian.PutUint16(data[50:52], 1) // LigCount
	binary.BigEndian.PutUint16(data[52:54], 4) // Lig offset from LigSet start (50+4=54)

	// Ligature at 54
	binary.BigEndian.PutUint16(data[54:56], 100) // LigGlyph (fi)
	binary.BigEndian.PutUint16(data[56:58], 2)   // CompCount
	binary.BigEndian.PutUint16(data[58:60], 11)  // Comp 2 (i)

	// Coverage at 64
	binary.BigEndian.PutUint16(data[64:66], 1)  // Format 1
	binary.BigEndian.PutUint16(data[66:68], 1)  // Count
	binary.BigEndian.PutUint16(data[68:70], 10) // GlyphID 10 (f)

	gsub, err := ParseGSUB(data)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	glyphs := []int{10, 11, 12}
	result := gsub.Substitute(glyphs)

	if len(result) != 2 {
		t.Errorf("expected 2 glyphs, got %d", len(result))
	}
	if result[0] != 100 {
		t.Errorf("expected glyph 100, got %d", result[0])
	}
	if result[1] != 12 {
		t.Errorf("expected glyph 12, got %d", result[1])
	}
}

func TestGPOS_Position_Pair(t *testing.T) {
	data := make([]byte, 200)

	// GPOS Header
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 20)
	binary.BigEndian.PutUint16(data[8:10], 30)

	// LookupList
	binary.BigEndian.PutUint16(data[30:32], 1)
	binary.BigEndian.PutUint16(data[32:34], 4)

	// LookupTable at 34
	binary.BigEndian.PutUint16(data[34:36], 2) // Type 2
	binary.BigEndian.PutUint16(data[36:38], 0)
	binary.BigEndian.PutUint16(data[38:40], 1)
	binary.BigEndian.PutUint16(data[40:42], 8)

	// PairAdj at 42
	binary.BigEndian.PutUint16(data[42:44], 1)      // Format 1
	binary.BigEndian.PutUint16(data[44:46], 10)     // Coverage offset (52)
	binary.BigEndian.PutUint16(data[46:48], 0x0004) // ValueFormat1 (XAdvance)
	binary.BigEndian.PutUint16(data[48:50], 0x0000) // ValueFormat2
	binary.BigEndian.PutUint16(data[50:52], 1)      // PairSet count
	binary.BigEndian.PutUint16(data[52:54], 14)     // PairSet offset (42+14=56)

	// Coverage at 52 (Re-fixed)
	binary.BigEndian.PutUint16(data[44:46], 20) // Coverage offset (42+20=62)
	binary.BigEndian.PutUint16(data[62:64], 1)
	binary.BigEndian.PutUint16(data[64:66], 1)  // Count
	binary.BigEndian.PutUint16(data[66:68], 20) // GlyphID 20

	// PairSet at 56
	binary.BigEndian.PutUint16(data[56:58], 1)      // PairValueCount
	binary.BigEndian.PutUint16(data[58:60], 21)     // SecondGlyph 21
	binary.BigEndian.PutUint16(data[60:62], 0xFFCE) // ValueRecord1 (XAdvance = -50)

	gpos, err := ParseGPOS(data)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	glyphs := []int{20, 21, 22}
	adj := gpos.Position(glyphs)

	if adj[0].X != -50 {
		t.Errorf("expected X adjustment -50, got %d", adj[0].X)
	}
}

func TestGPOS_SinglePosFormat2MalformedValueRecordsDoesNotPanic(t *testing.T) {
	data := make([]byte, 14)

	// SinglePos format 2 at offset 0.
	binary.BigEndian.PutUint16(data[0:2], 2)
	binary.BigEndian.PutUint16(data[2:4], 8)      // Coverage offset.
	binary.BigEndian.PutUint16(data[4:6], 0x0001) // ValueFormat: XPlacement.
	binary.BigEndian.PutUint16(data[6:8], 100)    // ValueCount larger than available data.

	// Coverage at offset 8 covers glyph 5.
	binary.BigEndian.PutUint16(data[8:10], 1)
	binary.BigEndian.PutUint16(data[10:12], 1)
	binary.BigEndian.PutUint16(data[12:14], 5)

	gpos := &GPOS{Data: data}
	lookup := &LookupTable{Type: 1, SubtableOffsets: []uint16{0}}
	adjustments := make([]api.Vector, 1)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("applySinglePos panicked on malformed format 2 table: %v", r)
		}
	}()

	gpos.applySinglePos(lookup, []int{5}, adjustments)
	if adjustments[0] != (api.Vector{}) {
		t.Fatalf("expected no adjustment for malformed table, got %+v", adjustments[0])
	}
}
