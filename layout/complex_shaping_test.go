package layout

import (
	"encoding/binary"
	"github.com/dh-kam/freetype-go/api"
	"testing"
)

func TestGPOS_MarkToBase(t *testing.T) {
	data := make([]byte, 500)

	// GPOS Header
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)  // ScriptList
	binary.BigEndian.PutUint16(data[6:8], 10)  // FeatureList
	binary.BigEndian.PutUint16(data[8:10], 10) // LookupList

	// LookupList at 10
	binary.BigEndian.PutUint16(data[10:12], 1) // LookupCount
	binary.BigEndian.PutUint16(data[12:14], 4) // Offset to LookupTable (10+4=14)

	// LookupTable at 14
	binary.BigEndian.PutUint16(data[14:16], 4) // Type 4
	binary.BigEndian.PutUint16(data[16:18], 0) // Flag
	binary.BigEndian.PutUint16(data[18:20], 1) // SubtableCount
	binary.BigEndian.PutUint16(data[20:22], 8) // SubtableOffset (14+8=22)

	// MarkToBaseSubst at 22
	off := uint16(22)
	binary.BigEndian.PutUint16(data[off:off+2], 1)      // Format 1
	binary.BigEndian.PutUint16(data[off+2:off+4], 12)   // MarkCoverage (22+12=34)
	binary.BigEndian.PutUint16(data[off+4:off+6], 20)   // BaseCoverage (22+20=42)
	binary.BigEndian.PutUint16(data[off+6:off+8], 1)    // MarkClassCount
	binary.BigEndian.PutUint16(data[off+8:off+10], 28)  // MarkArray (22+28=50)
	binary.BigEndian.PutUint16(data[off+10:off+12], 40) // BaseArray (22+40=62)

	// MarkCoverage at 34
	binary.BigEndian.PutUint16(data[34:36], 1)
	binary.BigEndian.PutUint16(data[36:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 500) // Mark Glyph ID 500

	// BaseCoverage at 42
	binary.BigEndian.PutUint16(data[42:44], 1)
	binary.BigEndian.PutUint16(data[44:46], 1)
	binary.BigEndian.PutUint16(data[46:48], 100) // Base Glyph ID 100

	// MarkArray at 50
	binary.BigEndian.PutUint16(data[50:52], 1) // MarkCount
	binary.BigEndian.PutUint16(data[52:54], 0) // Class 0
	binary.BigEndian.PutUint16(data[54:56], 6) // Anchor offset (50+6=56)
	// MarkAnchor at 56
	binary.BigEndian.PutUint16(data[56:58], 1)  // Format 1
	binary.BigEndian.PutUint16(data[58:60], 10) // X
	binary.BigEndian.PutUint16(data[60:62], 20) // Y

	// BaseArray at 62
	binary.BigEndian.PutUint16(data[62:64], 1) // BaseCount
	binary.BigEndian.PutUint16(data[64:66], 6) // BaseAnchor offset (62+6=68)
	// BaseAnchor at 68
	binary.BigEndian.PutUint16(data[68:70], 1)   // Format 1
	binary.BigEndian.PutUint16(data[70:72], 100) // X
	binary.BigEndian.PutUint16(data[72:74], 150) // Y

	gpos, err := ParseGPOS(data)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	glyphs := []int{100, 500}
	adjustments := make([]api.Vector, 2)
	gpos.applyLookup(gpos.LookupList.Lookups[0], glyphs, adjustments)

	// Mark (glyph index 1) should be adjusted
	// MarkAnchor (10, 20), BaseAnchor (100, 150)
	// Adjustment = BaseAnchor - MarkAnchor = (90, 130)
	if adjustments[1].X != 90 {
		t.Errorf("expected X adjustment 90, got %d", adjustments[1].X)
	}
	if adjustments[1].Y != 130 {
		t.Errorf("expected Y adjustment 130, got %d", adjustments[1].Y)
	}
}
