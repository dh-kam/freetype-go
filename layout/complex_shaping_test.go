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

func TestGPOS_MarkToBaseMalformedCoverageDoesNotPanic(t *testing.T) {
	data := make([]byte, 12)

	binary.BigEndian.PutUint16(data[0:2], 1)   // format
	binary.BigEndian.PutUint16(data[2:4], 10)  // truncated mark coverage
	binary.BigEndian.PutUint16(data[4:6], 10)  // truncated base coverage
	binary.BigEndian.PutUint16(data[6:8], 1)   // mark class count
	binary.BigEndian.PutUint16(data[8:10], 0)  // mark array
	binary.BigEndian.PutUint16(data[10:12], 0) // base array

	gpos := &GPOS{Data: data}
	lookup := &LookupTable{Type: 4, SubtableOffsets: []uint16{0}}
	adjustments := make([]api.Vector, 2)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("mark-to-base panicked on malformed coverage: %v", r)
		}
	}()

	gpos.applyMarkToBasePos(lookup, []int{100, 500}, adjustments)
	if adjustments[1] != (api.Vector{}) {
		t.Fatalf("expected no adjustment for malformed table, got %+v", adjustments[1])
	}
}

func TestGPOS_MarkToMark(t *testing.T) {
	data := make([]byte, 120)

	// GPOS Header
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 20)
	binary.BigEndian.PutUint16(data[8:10], 30)

	// Empty ScriptList and FeatureList.
	binary.BigEndian.PutUint16(data[10:12], 0)
	binary.BigEndian.PutUint16(data[20:22], 0)

	// LookupList at 30.
	binary.BigEndian.PutUint16(data[30:32], 1)
	binary.BigEndian.PutUint16(data[32:34], 4)

	// LookupTable at 34.
	binary.BigEndian.PutUint16(data[34:36], 6)
	binary.BigEndian.PutUint16(data[36:38], 0)
	binary.BigEndian.PutUint16(data[38:40], 1)
	binary.BigEndian.PutUint16(data[40:42], 8)

	// MarkToMarkPos format 1 at 42.
	binary.BigEndian.PutUint16(data[42:44], 1)
	binary.BigEndian.PutUint16(data[44:46], 12) // Mark1Coverage at 54
	binary.BigEndian.PutUint16(data[46:48], 18) // Mark2Coverage at 60
	binary.BigEndian.PutUint16(data[48:50], 1)
	binary.BigEndian.PutUint16(data[50:52], 24) // Mark1Array at 66
	binary.BigEndian.PutUint16(data[52:54], 36) // Mark2Array at 78

	// Mark1Coverage at 54: glyph 501.
	binary.BigEndian.PutUint16(data[54:56], 1)
	binary.BigEndian.PutUint16(data[56:58], 1)
	binary.BigEndian.PutUint16(data[58:60], 501)

	// Mark2Coverage at 60: glyph 500.
	binary.BigEndian.PutUint16(data[60:62], 1)
	binary.BigEndian.PutUint16(data[62:64], 1)
	binary.BigEndian.PutUint16(data[64:66], 500)

	// Mark1Array at 66.
	binary.BigEndian.PutUint16(data[66:68], 1)
	binary.BigEndian.PutUint16(data[68:70], 0)
	binary.BigEndian.PutUint16(data[70:72], 6)
	binary.BigEndian.PutUint16(data[72:74], 1)
	binary.BigEndian.PutUint16(data[74:76], 10)
	binary.BigEndian.PutUint16(data[76:78], 20)

	// Mark2Array at 78.
	binary.BigEndian.PutUint16(data[78:80], 1)
	binary.BigEndian.PutUint16(data[80:82], 4)
	binary.BigEndian.PutUint16(data[82:84], 1)
	binary.BigEndian.PutUint16(data[84:86], 100)
	binary.BigEndian.PutUint16(data[86:88], 150)

	gpos, err := ParseGPOS(data)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	adjustments := make([]api.Vector, 2)
	adjustments[0] = api.Vector{X: 5, Y: 7}
	gpos.applyLookup(gpos.LookupList.Lookups[0], []int{500, 501}, adjustments)

	if adjustments[1].X != 95 {
		t.Fatalf("expected mark-to-mark X adjustment 95, got %d", adjustments[1].X)
	}
	if adjustments[1].Y != 137 {
		t.Fatalf("expected mark-to-mark Y adjustment 137, got %d", adjustments[1].Y)
	}
}

func TestGPOS_MarkToLigature(t *testing.T) {
	data := make([]byte, 80)

	// MarkToLigaturePos format 1 at 0: mark 501 attaches to ligature 300.
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 12)
	binary.BigEndian.PutUint16(data[4:6], 18)
	binary.BigEndian.PutUint16(data[6:8], 1)
	binary.BigEndian.PutUint16(data[8:10], 24)
	binary.BigEndian.PutUint16(data[10:12], 36)
	putCoverageFormat1(data, 12, 501)
	putCoverageFormat1(data, 18, 300)

	// MarkArray at 24.
	binary.BigEndian.PutUint16(data[24:26], 1)
	binary.BigEndian.PutUint16(data[26:28], 0)
	binary.BigEndian.PutUint16(data[28:30], 6)
	binary.BigEndian.PutUint16(data[30:32], 1)
	binary.BigEndian.PutUint16(data[32:34], 10)
	binary.BigEndian.PutUint16(data[34:36], 20)

	// LigatureArray at 36. Component 0 anchor is (100, 150).
	binary.BigEndian.PutUint16(data[36:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 4)
	binary.BigEndian.PutUint16(data[40:42], 2)
	binary.BigEndian.PutUint16(data[42:44], 6)
	binary.BigEndian.PutUint16(data[44:46], 12)
	binary.BigEndian.PutUint16(data[46:48], 1)
	binary.BigEndian.PutUint16(data[48:50], 100)
	binary.BigEndian.PutUint16(data[50:52], 150)
	binary.BigEndian.PutUint16(data[52:54], 1)
	binary.BigEndian.PutUint16(data[54:56], 200)
	binary.BigEndian.PutUint16(data[56:58], 250)

	gpos := &GPOS{Data: data}
	lookup := &LookupTable{Type: 5, SubtableOffsets: []uint16{0}}
	adjustments := make([]api.Vector, 2)
	gpos.applyLookup(lookup, []int{300, 501}, adjustments)

	if adjustments[1].X != 90 {
		t.Fatalf("expected mark-to-ligature X adjustment 90, got %d", adjustments[1].X)
	}
	if adjustments[1].Y != 130 {
		t.Fatalf("expected mark-to-ligature Y adjustment 130, got %d", adjustments[1].Y)
	}
}

func TestGPOS_MarkToLigatureUsesNonMarkComponentHeuristic(t *testing.T) {
	data := make([]byte, 90)

	// MarkToLigaturePos format 1 at 0: marks 501 and 502 attach to ligature 300.
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 12)
	binary.BigEndian.PutUint16(data[4:6], 20)
	binary.BigEndian.PutUint16(data[6:8], 1)
	binary.BigEndian.PutUint16(data[8:10], 26)
	binary.BigEndian.PutUint16(data[10:12], 50)
	putCoverageFormat1(data, 12, 501, 502)
	putCoverageFormat1(data, 20, 300)

	// MarkArray at 26. Both marks are class 0 with the same anchor.
	binary.BigEndian.PutUint16(data[26:28], 2)
	binary.BigEndian.PutUint16(data[28:30], 0)
	binary.BigEndian.PutUint16(data[30:32], 10)
	binary.BigEndian.PutUint16(data[32:34], 0)
	binary.BigEndian.PutUint16(data[34:36], 16)
	binary.BigEndian.PutUint16(data[36:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint16(data[40:42], 20)
	binary.BigEndian.PutUint16(data[42:44], 1)
	binary.BigEndian.PutUint16(data[44:46], 10)
	binary.BigEndian.PutUint16(data[46:48], 20)

	// LigatureArray at 50. Component 0 anchor is (100,150);
	// component 1 anchor is intentionally different.
	binary.BigEndian.PutUint16(data[50:52], 1)
	binary.BigEndian.PutUint16(data[52:54], 4)
	binary.BigEndian.PutUint16(data[54:56], 2)
	binary.BigEndian.PutUint16(data[56:58], 6)
	binary.BigEndian.PutUint16(data[58:60], 12)
	binary.BigEndian.PutUint16(data[60:62], 1)
	binary.BigEndian.PutUint16(data[62:64], 100)
	binary.BigEndian.PutUint16(data[64:66], 150)
	binary.BigEndian.PutUint16(data[66:68], 1)
	binary.BigEndian.PutUint16(data[68:70], 200)
	binary.BigEndian.PutUint16(data[70:72], 250)

	gpos := &GPOS{Data: data, GDEF: makeTestGDEFWithMarks(501, 502)}
	lookup := &LookupTable{Type: 5, SubtableOffsets: []uint16{0}}
	adjustments := make([]api.Vector, 3)
	gpos.applyLookup(lookup, []int{300, 501, 502}, adjustments)

	if adjustments[1].X != 90 || adjustments[1].Y != 130 {
		t.Fatalf("expected first mark to use component 0 anchor, got %+v", adjustments[1])
	}
	if adjustments[2].X != 90 || adjustments[2].Y != 130 {
		t.Fatalf("expected consecutive mark to reuse component 0 anchor, got %+v", adjustments[2])
	}
}

func TestGPOS_MarkFilteringSetFiltersMarks(t *testing.T) {
	data := make([]byte, 80)

	// MarkToBasePos format 1 at 0. Both marks are covered by the lookup.
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 12)
	binary.BigEndian.PutUint16(data[4:6], 20)
	binary.BigEndian.PutUint16(data[6:8], 1)
	binary.BigEndian.PutUint16(data[8:10], 26)
	binary.BigEndian.PutUint16(data[10:12], 48)
	putCoverageFormat1(data, 12, 500, 501)
	putCoverageFormat1(data, 20, 100)

	// MarkArray at 26.
	binary.BigEndian.PutUint16(data[26:28], 2)
	binary.BigEndian.PutUint16(data[28:30], 0)
	binary.BigEndian.PutUint16(data[30:32], 10)
	binary.BigEndian.PutUint16(data[32:34], 0)
	binary.BigEndian.PutUint16(data[34:36], 16)
	binary.BigEndian.PutUint16(data[36:38], 1)
	binary.BigEndian.PutUint16(data[38:40], 10)
	binary.BigEndian.PutUint16(data[40:42], 20)
	binary.BigEndian.PutUint16(data[42:44], 1)
	binary.BigEndian.PutUint16(data[44:46], 30)
	binary.BigEndian.PutUint16(data[46:48], 40)

	// BaseArray at 48.
	binary.BigEndian.PutUint16(data[48:50], 1)
	binary.BigEndian.PutUint16(data[50:52], 4)
	binary.BigEndian.PutUint16(data[52:54], 1)
	binary.BigEndian.PutUint16(data[54:56], 100)
	binary.BigEndian.PutUint16(data[56:58], 100)

	gdefData := make([]byte, 64)
	binary.BigEndian.PutUint32(gdefData[0:4], 0x00010002)
	binary.BigEndian.PutUint16(gdefData[4:6], 20)
	binary.BigEndian.PutUint16(gdefData[12:14], 40)
	binary.BigEndian.PutUint16(gdefData[20:22], 2)
	binary.BigEndian.PutUint16(gdefData[22:24], 2)
	binary.BigEndian.PutUint16(gdefData[24:26], 100)
	binary.BigEndian.PutUint16(gdefData[26:28], 100)
	binary.BigEndian.PutUint16(gdefData[28:30], gdefGlyphClassBase)
	binary.BigEndian.PutUint16(gdefData[30:32], 500)
	binary.BigEndian.PutUint16(gdefData[32:34], 501)
	binary.BigEndian.PutUint16(gdefData[34:36], gdefGlyphClassMark)
	binary.BigEndian.PutUint16(gdefData[40:42], 1)
	binary.BigEndian.PutUint16(gdefData[42:44], 1)
	binary.BigEndian.PutUint32(gdefData[44:48], 8)
	putCoverageFormat1(gdefData, 48, 501)

	gdef, err := ParseGDEF(gdefData)
	if err != nil {
		t.Fatalf("ParseGDEF failed: %v", err)
	}

	gpos := &GPOS{Data: data, GDEF: gdef}
	lookup := &LookupTable{
		Type:             4,
		Flag:             lookupFlagUseMarkFilteringSet,
		SubtableOffsets:  []uint16{0},
		MarkFilteringSet: 0,
	}
	adjustments := make([]api.Vector, 3)
	gpos.applyLookup(lookup, []int{100, 500, 501}, adjustments)

	if adjustments[1] != (api.Vector{}) {
		t.Fatalf("expected filtered mark to remain unadjusted, got %+v", adjustments[1])
	}
	if adjustments[2].X != 70 {
		t.Fatalf("expected allowed mark X adjustment 70, got %d", adjustments[2].X)
	}
	if adjustments[2].Y != 60 {
		t.Fatalf("expected allowed mark Y adjustment 60, got %d", adjustments[2].Y)
	}
}

func TestGPOS_CursivePositioning(t *testing.T) {
	data := make([]byte, 100)

	// GPOS Header
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 20)
	binary.BigEndian.PutUint16(data[8:10], 30)

	binary.BigEndian.PutUint16(data[10:12], 0)
	binary.BigEndian.PutUint16(data[20:22], 0)

	putLookupList(data, 30, 34)
	putLookupTable(data, 34, 3, 42)

	// CursivePos format 1 at 42. Glyph 200 has an exit anchor; glyph 201 has
	// an entry anchor, so glyph 201 is moved by exit-entry.
	binary.BigEndian.PutUint16(data[42:44], 1)
	binary.BigEndian.PutUint16(data[44:46], 14) // Coverage at 56.
	binary.BigEndian.PutUint16(data[46:48], 2)
	binary.BigEndian.PutUint16(data[48:50], 0)
	binary.BigEndian.PutUint16(data[50:52], 22) // Exit anchor at 64.
	binary.BigEndian.PutUint16(data[52:54], 28) // Entry anchor at 70.
	binary.BigEndian.PutUint16(data[54:56], 0)
	putCoverageFormat1(data, 56, 200, 201)

	binary.BigEndian.PutUint16(data[64:66], 1)
	binary.BigEndian.PutUint16(data[66:68], 300)
	binary.BigEndian.PutUint16(data[68:70], 40)
	binary.BigEndian.PutUint16(data[70:72], 1)
	binary.BigEndian.PutUint16(data[72:74], 30)
	binary.BigEndian.PutUint16(data[74:76], 10)

	gpos, err := ParseGPOS(data)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	adjustments := gpos.Position([]int{200, 201, 202})
	if adjustments[1].X != 270 {
		t.Fatalf("expected cursive X adjustment 270, got %d", adjustments[1].X)
	}
	if adjustments[1].Y != 30 {
		t.Fatalf("expected cursive Y adjustment 30, got %d", adjustments[1].Y)
	}
	if adjustments[2] != (api.Vector{}) {
		t.Fatalf("expected uncovered glyph adjustment to remain zero, got %+v", adjustments[2])
	}
}
