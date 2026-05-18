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

func TestGSUB_Substitute_Multiple(t *testing.T) {
	data := make([]byte, 32)

	// MultipleSubst format 1 at 0: glyph 20 -> [21 22].
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 10)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint16(data[6:8], 16)
	putCoverageFormat1(data, 10, 20)
	binary.BigEndian.PutUint16(data[16:18], 2)
	binary.BigEndian.PutUint16(data[18:20], 21)
	binary.BigEndian.PutUint16(data[20:22], 22)

	gsub := &GSUB{Data: data}
	lookup := &LookupTable{Type: 2, SubtableOffsets: []uint16{0}}
	got := gsub.applyLookup(lookup, []int{10, 20, 30})
	assertGlyphs(t, got, []int{10, 21, 22, 30})
}

func TestGSUB_Substitute_AlternateUsesFirstAlternate(t *testing.T) {
	data := make([]byte, 32)

	// AlternateSubst format 1 at 0: glyph 20 -> first alternate 120.
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 10)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint16(data[6:8], 16)
	putCoverageFormat1(data, 10, 20)
	binary.BigEndian.PutUint16(data[16:18], 2)
	binary.BigEndian.PutUint16(data[18:20], 120)
	binary.BigEndian.PutUint16(data[20:22], 121)

	gsub := &GSUB{Data: data}
	lookup := &LookupTable{Type: 3, SubtableOffsets: []uint16{0}}
	got := gsub.applyLookup(lookup, []int{10, 20, 30})
	assertGlyphs(t, got, []int{10, 120, 30})
}

func TestGSUB_LookupFlagIgnoreMarksSkipsSingleSubstTargets(t *testing.T) {
	data := make([]byte, 18)

	// SingleSubst format 1 at 0 covers a base and a mark.
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 6)
	binary.BigEndian.PutUint16(data[4:6], 100)
	putCoverageFormat1(data, 6, 20, 500)

	gsub := &GSUB{Data: data, GDEF: makeTestGDEFWithMarks(500)}
	lookup := &LookupTable{
		Type:            1,
		Flag:            lookupFlagIgnoreMarks,
		SubtableOffsets: []uint16{0},
	}

	got := gsub.applyLookup(lookup, []int{20, 500})
	assertGlyphs(t, got, []int{120, 500})
}

func TestGSUB_MarkAttachmentTypeFiltersMarks(t *testing.T) {
	data := make([]byte, 18)

	// SingleSubst format 1 at 0 covers both marks.
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 6)
	binary.BigEndian.PutUint16(data[4:6], 100)
	putCoverageFormat1(data, 6, 500, 501)

	gsub := &GSUB{Data: data, GDEF: makeTestGDEFWithMarkAttachment(map[uint16]uint16{
		500: 1,
		501: 2,
	})}
	lookup := &LookupTable{
		Type:            1,
		Flag:            1 << 8,
		SubtableOffsets: []uint16{0},
	}

	got := gsub.applyLookup(lookup, []int{500, 501})
	assertGlyphs(t, got, []int{600, 501})
}

func TestGSUB_MarkFilteringSetFiltersMarks(t *testing.T) {
	data := make([]byte, 18)

	// SingleSubst format 1 at 0 covers both marks.
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 6)
	binary.BigEndian.PutUint16(data[4:6], 100)
	putCoverageFormat1(data, 6, 500, 501)

	gdef := makeTestGDEFWithMarks(500, 501)
	gdef.MarkGlyphSets = []CoverageTable{&CoverageFormat1{Glyphs: []uint16{501}}}
	gsub := &GSUB{Data: data, GDEF: gdef}
	lookup := &LookupTable{
		Type:             1,
		Flag:             lookupFlagUseMarkFilteringSet,
		SubtableOffsets:  []uint16{0},
		MarkFilteringSet: 0,
	}

	got := gsub.applyLookup(lookup, []int{500, 501})
	assertGlyphs(t, got, []int{500, 601})
}

func TestGSUB_LigatureSubstSkipsIgnoredMarksAndPreservesThem(t *testing.T) {
	data := make([]byte, 32)

	// LigatureSubst format 1 at 0: glyphs 10,11 -> 100.
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 22)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint16(data[6:8], 8)
	binary.BigEndian.PutUint16(data[8:10], 1)
	binary.BigEndian.PutUint16(data[10:12], 4)
	binary.BigEndian.PutUint16(data[12:14], 100)
	binary.BigEndian.PutUint16(data[14:16], 2)
	binary.BigEndian.PutUint16(data[16:18], 11)
	putCoverageFormat1(data, 22, 10)

	gsub := &GSUB{Data: data, GDEF: makeTestGDEFWithMarks(500)}
	lookup := &LookupTable{
		Type:            4,
		Flag:            lookupFlagIgnoreMarks,
		SubtableOffsets: []uint16{0},
	}

	got := gsub.applyLookup(lookup, []int{10, 500, 11, 12})
	assertGlyphs(t, got, []int{100, 500, 12})
}

func TestGSUB_ContextMatchingSkipsIgnoredMarks(t *testing.T) {
	data := make([]byte, 48)

	// ContextSubst format 3 at 0: match 10,11 and apply lookup 1 to sequence index 1.
	binary.BigEndian.PutUint16(data[0:2], 3)
	binary.BigEndian.PutUint16(data[2:4], 2)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint16(data[6:8], 18)
	binary.BigEndian.PutUint16(data[8:10], 24)
	binary.BigEndian.PutUint16(data[10:12], 1)
	binary.BigEndian.PutUint16(data[12:14], 1)
	putCoverageFormat1(data, 18, 10)
	putCoverageFormat1(data, 24, 11)

	// Nested SingleSubst format 1 at 30: glyph 11 -> 111.
	binary.BigEndian.PutUint16(data[30:32], 1)
	binary.BigEndian.PutUint16(data[32:34], 6)
	binary.BigEndian.PutUint16(data[34:36], 100)
	putCoverageFormat1(data, 36, 11)

	contextLookup := &LookupTable{
		Type:            5,
		Flag:            lookupFlagIgnoreMarks,
		SubtableOffsets: []uint16{0},
	}
	gsub := &GSUB{
		Data: data,
		GDEF: makeTestGDEFWithMarks(500),
		LookupList: &LookupList{Lookups: []*LookupTable{
			contextLookup,
			{Type: 1, Flag: lookupFlagIgnoreMarks, SubtableOffsets: []uint16{30}},
		}},
	}

	got := gsub.applyLookup(contextLookup, []int{10, 500, 11})
	assertGlyphs(t, got, []int{10, 500, 111})
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

func TestGPOS_Position_PairFormat2ClassPair(t *testing.T) {
	data := make([]byte, 180)

	// GPOS Header
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 20)
	binary.BigEndian.PutUint16(data[8:10], 30)

	// Empty ScriptList and FeatureList so Position applies all lookups.
	binary.BigEndian.PutUint16(data[10:12], 0)
	binary.BigEndian.PutUint16(data[20:22], 0)

	// LookupList at 30
	binary.BigEndian.PutUint16(data[30:32], 1)
	binary.BigEndian.PutUint16(data[32:34], 4)

	// LookupTable at 34
	binary.BigEndian.PutUint16(data[34:36], 2)
	binary.BigEndian.PutUint16(data[36:38], 0)
	binary.BigEndian.PutUint16(data[38:40], 1)
	binary.BigEndian.PutUint16(data[40:42], 8)

	// PairPos format 2 at 42.
	binary.BigEndian.PutUint16(data[42:44], 2)
	binary.BigEndian.PutUint16(data[44:46], 40)     // Coverage at 82
	binary.BigEndian.PutUint16(data[46:48], 0x0004) // ValueFormat1: XAdvance
	binary.BigEndian.PutUint16(data[48:50], 0x0001) // ValueFormat2: XPlacement
	binary.BigEndian.PutUint16(data[50:52], 46)     // ClassDef1 at 88
	binary.BigEndian.PutUint16(data[52:54], 54)     // ClassDef2 at 96
	binary.BigEndian.PutUint16(data[54:56], 2)      // Class1Count
	binary.BigEndian.PutUint16(data[56:58], 3)      // Class2Count

	// Class pair matrix at 58. Record size is 4 bytes.
	// Class1=1, Class2=1 adjusts first glyph advance by -80 and second placement by +15.
	binary.BigEndian.PutUint16(data[74:76], uint16(0xffb0))
	binary.BigEndian.PutUint16(data[76:78], 15)
	// Class1=1, Class2=2 adjusts first glyph advance by -30.
	binary.BigEndian.PutUint16(data[78:80], uint16(0xffe2))

	// Coverage at 82: only glyph 20 can start a pair adjustment.
	binary.BigEndian.PutUint16(data[82:84], 1)
	binary.BigEndian.PutUint16(data[84:86], 1)
	binary.BigEndian.PutUint16(data[86:88], 20)

	// ClassDef1 format 1 at 88: glyph 20 -> class 1.
	binary.BigEndian.PutUint16(data[88:90], 1)
	binary.BigEndian.PutUint16(data[90:92], 20)
	binary.BigEndian.PutUint16(data[92:94], 1)
	binary.BigEndian.PutUint16(data[94:96], 1)

	// ClassDef2 format 2 at 96: glyph 21 -> class 1, glyph 22 -> class 2.
	binary.BigEndian.PutUint16(data[96:98], 2)
	binary.BigEndian.PutUint16(data[98:100], 2)
	binary.BigEndian.PutUint16(data[100:102], 21)
	binary.BigEndian.PutUint16(data[102:104], 21)
	binary.BigEndian.PutUint16(data[104:106], 1)
	binary.BigEndian.PutUint16(data[106:108], 22)
	binary.BigEndian.PutUint16(data[108:110], 22)
	binary.BigEndian.PutUint16(data[110:112], 2)

	gpos, err := ParseGPOS(data)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	adj := gpos.Position([]int{20, 21, 20, 22, 99, 21})
	if adj[0].X != -80 {
		t.Fatalf("expected first pair X adjustment -80, got %d", adj[0].X)
	}
	if adj[1].X != 15 {
		t.Fatalf("expected second glyph X placement 15, got %d", adj[1].X)
	}
	if adj[2].X != -30 {
		t.Fatalf("expected class2 pair X adjustment -30, got %d", adj[2].X)
	}
	if adj[4] != (api.Vector{}) || adj[5] != (api.Vector{}) {
		t.Fatalf("expected uncovered first glyph pair to remain unchanged, got %+v %+v", adj[4], adj[5])
	}
}

func TestGPOS_PairPosSkipsIgnoredMarksBetweenPairGlyphs(t *testing.T) {
	data := make([]byte, 32)

	// PairPos format 1 at 0: glyph 20 followed by glyph 21 adjusts both glyphs.
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 24)
	binary.BigEndian.PutUint16(data[4:6], 0x0004)
	binary.BigEndian.PutUint16(data[6:8], 0x0001)
	binary.BigEndian.PutUint16(data[8:10], 1)
	binary.BigEndian.PutUint16(data[10:12], 12)
	binary.BigEndian.PutUint16(data[12:14], 1)
	binary.BigEndian.PutUint16(data[14:16], 21)
	binary.BigEndian.PutUint16(data[16:18], uint16(0xffce))
	binary.BigEndian.PutUint16(data[18:20], 15)
	putCoverageFormat1(data, 24, 20)

	gpos := &GPOS{Data: data, GDEF: makeTestGDEFWithMarks(500)}
	lookup := &LookupTable{
		Type:            2,
		Flag:            lookupFlagIgnoreMarks,
		SubtableOffsets: []uint16{0},
	}
	adjustments := make([]api.Vector, 3)

	gpos.applyLookup(lookup, []int{20, 500, 21}, adjustments)

	if adjustments[0].X != -50 {
		t.Fatalf("expected first pair glyph X adjustment -50, got %d", adjustments[0].X)
	}
	if adjustments[1] != (api.Vector{}) {
		t.Fatalf("expected ignored mark adjustment to remain zero, got %+v", adjustments[1])
	}
	if adjustments[2].X != 15 {
		t.Fatalf("expected second pair glyph X adjustment 15, got %d", adjustments[2].X)
	}
}

func TestGSUB_SubstituteUsesDefaultLangSysFeatures(t *testing.T) {
	data := make([]byte, 180)

	// GSUB Header
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 50)
	binary.BigEndian.PutUint16(data[8:10], 90)

	// ScriptList at 10: DFLT script with a DefaultLangSys selecting feature 0 only.
	binary.BigEndian.PutUint16(data[10:12], 1)
	binary.BigEndian.PutUint32(data[12:16], 0x44464c54)
	binary.BigEndian.PutUint16(data[16:18], 8)
	binary.BigEndian.PutUint16(data[18:20], 4) // DefaultLangSys at 22
	binary.BigEndian.PutUint16(data[20:22], 0)
	binary.BigEndian.PutUint16(data[22:24], 0)
	binary.BigEndian.PutUint16(data[24:26], 0xffff)
	binary.BigEndian.PutUint16(data[26:28], 1)
	binary.BigEndian.PutUint16(data[28:30], 0)

	// FeatureList at 50: feature 0 activates lookup 0; feature 1 activates lookup 1.
	binary.BigEndian.PutUint16(data[50:52], 2)
	binary.BigEndian.PutUint32(data[52:56], 0x74657374)
	binary.BigEndian.PutUint16(data[56:58], 14)
	binary.BigEndian.PutUint32(data[58:62], 0x73616c74)
	binary.BigEndian.PutUint16(data[62:64], 20)
	binary.BigEndian.PutUint16(data[64:66], 0)
	binary.BigEndian.PutUint16(data[66:68], 1)
	binary.BigEndian.PutUint16(data[68:70], 0)
	binary.BigEndian.PutUint16(data[70:72], 0)
	binary.BigEndian.PutUint16(data[72:74], 1)
	binary.BigEndian.PutUint16(data[74:76], 1)

	// LookupList at 90.
	binary.BigEndian.PutUint16(data[90:92], 2)
	binary.BigEndian.PutUint16(data[92:94], 6)  // Lookup 0 at 96
	binary.BigEndian.PutUint16(data[94:96], 40) // Lookup 1 at 130

	// Lookup 0 substitutes glyph 10 -> 11.
	binary.BigEndian.PutUint16(data[96:98], 1)
	binary.BigEndian.PutUint16(data[98:100], 0)
	binary.BigEndian.PutUint16(data[100:102], 1)
	binary.BigEndian.PutUint16(data[102:104], 8)
	binary.BigEndian.PutUint16(data[104:106], 1)
	binary.BigEndian.PutUint16(data[106:108], 6)
	binary.BigEndian.PutUint16(data[108:110], 1)
	binary.BigEndian.PutUint16(data[110:112], 1)
	binary.BigEndian.PutUint16(data[112:114], 1)
	binary.BigEndian.PutUint16(data[114:116], 10)

	// Lookup 1 would substitute glyph 11 -> 111 if all features were applied.
	binary.BigEndian.PutUint16(data[130:132], 1)
	binary.BigEndian.PutUint16(data[132:134], 0)
	binary.BigEndian.PutUint16(data[134:136], 1)
	binary.BigEndian.PutUint16(data[136:138], 8)
	binary.BigEndian.PutUint16(data[138:140], 1)
	binary.BigEndian.PutUint16(data[140:142], 6)
	binary.BigEndian.PutUint16(data[142:144], 100)
	binary.BigEndian.PutUint16(data[144:146], 1)
	binary.BigEndian.PutUint16(data[146:148], 1)
	binary.BigEndian.PutUint16(data[148:150], 11)

	gsub, err := ParseGSUB(data)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	got := gsub.Substitute([]int{10})
	if len(got) != 1 || got[0] != 11 {
		t.Fatalf("expected default LangSys to apply only feature 0 and produce [11], got %v", got)
	}
	if gotAll := gsub.SubstituteFeatures([]int{10}, []int{0, 1}); len(gotAll) != 1 || gotAll[0] != 111 {
		t.Fatalf("expected explicit all-feature substitution to produce [111], got %v", gotAll)
	}
}

func TestGSUB_ExtensionSubstSingle(t *testing.T) {
	data := make([]byte, 100)

	// GSUB Header
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 20)
	binary.BigEndian.PutUint16(data[8:10], 30)

	binary.BigEndian.PutUint16(data[10:12], 0)
	binary.BigEndian.PutUint16(data[20:22], 0)

	// LookupList at 30.
	binary.BigEndian.PutUint16(data[30:32], 1)
	binary.BigEndian.PutUint16(data[32:34], 4)

	// LookupTable at 34: ExtensionSubst.
	binary.BigEndian.PutUint16(data[34:36], 7)
	binary.BigEndian.PutUint16(data[36:38], 0)
	binary.BigEndian.PutUint16(data[38:40], 1)
	binary.BigEndian.PutUint16(data[40:42], 8)

	// ExtensionSubst at 42 wraps a SingleSubst subtable at 50.
	binary.BigEndian.PutUint16(data[42:44], 1)
	binary.BigEndian.PutUint16(data[44:46], 1)
	binary.BigEndian.PutUint32(data[46:50], 8)

	// SingleSubst format 1 at 50.
	binary.BigEndian.PutUint16(data[50:52], 1)
	binary.BigEndian.PutUint16(data[52:54], 6)
	binary.BigEndian.PutUint16(data[54:56], 5)

	// Coverage at 56.
	binary.BigEndian.PutUint16(data[56:58], 1)
	binary.BigEndian.PutUint16(data[58:60], 1)
	binary.BigEndian.PutUint16(data[60:62], 10)

	gsub, err := ParseGSUB(data)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	got := gsub.Substitute([]int{10, 12})
	if len(got) != 2 || got[0] != 15 || got[1] != 12 {
		t.Fatalf("expected extension substitution [15 12], got %v", got)
	}
}

func TestGPOS_ExtensionPosSingle(t *testing.T) {
	data := make([]byte, 100)

	// GPOS Header
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 20)
	binary.BigEndian.PutUint16(data[8:10], 30)

	binary.BigEndian.PutUint16(data[10:12], 0)
	binary.BigEndian.PutUint16(data[20:22], 0)

	// LookupList at 30.
	binary.BigEndian.PutUint16(data[30:32], 1)
	binary.BigEndian.PutUint16(data[32:34], 4)

	// LookupTable at 34: ExtensionPos.
	binary.BigEndian.PutUint16(data[34:36], 9)
	binary.BigEndian.PutUint16(data[36:38], 0)
	binary.BigEndian.PutUint16(data[38:40], 1)
	binary.BigEndian.PutUint16(data[40:42], 8)

	// ExtensionPos at 42 wraps a SinglePos subtable at 50.
	binary.BigEndian.PutUint16(data[42:44], 1)
	binary.BigEndian.PutUint16(data[44:46], 1)
	binary.BigEndian.PutUint32(data[46:50], 8)

	// SinglePos format 1 at 50.
	binary.BigEndian.PutUint16(data[50:52], 1)
	binary.BigEndian.PutUint16(data[52:54], 8)
	binary.BigEndian.PutUint16(data[54:56], 0x0001)
	binary.BigEndian.PutUint16(data[56:58], 25)

	// Coverage at 58.
	binary.BigEndian.PutUint16(data[58:60], 1)
	binary.BigEndian.PutUint16(data[60:62], 1)
	binary.BigEndian.PutUint16(data[62:64], 30)

	gpos, err := ParseGPOS(data)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	adjustments := gpos.Position([]int{30, 31})
	if adjustments[0].X != 25 || adjustments[1] != (api.Vector{}) {
		t.Fatalf("expected extension positioning [{25 0} {0 0}], got %+v", adjustments)
	}
}

func TestGSUB_ChainedContextFormat3AppliesLigatureAtSequenceIndex(t *testing.T) {
	data := make([]byte, 150)

	// GSUB Header
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 20)
	binary.BigEndian.PutUint16(data[8:10], 30)

	binary.BigEndian.PutUint16(data[10:12], 0)
	binary.BigEndian.PutUint16(data[20:22], 0)

	// LookupList at 30: chained context lookup plus ligature lookup.
	binary.BigEndian.PutUint16(data[30:32], 2)
	binary.BigEndian.PutUint16(data[32:34], 6)  // Lookup 0 at 36
	binary.BigEndian.PutUint16(data[34:36], 60) // Lookup 1 at 90

	// Lookup 0 at 36: ChainedContextSubst.
	binary.BigEndian.PutUint16(data[36:38], 6)
	binary.BigEndian.PutUint16(data[38:40], 0)
	binary.BigEndian.PutUint16(data[40:42], 1)
	binary.BigEndian.PutUint16(data[42:44], 8)

	// ChainedContextSubst format 3 at 44.
	binary.BigEndian.PutUint16(data[44:46], 3)
	binary.BigEndian.PutUint16(data[46:48], 0)  // BacktrackGlyphCount
	binary.BigEndian.PutUint16(data[48:50], 2)  // InputGlyphCount
	binary.BigEndian.PutUint16(data[50:52], 26) // InputCoverage[0] at 70
	binary.BigEndian.PutUint16(data[52:54], 32) // InputCoverage[1] at 76
	binary.BigEndian.PutUint16(data[54:56], 0)  // LookaheadGlyphCount
	binary.BigEndian.PutUint16(data[56:58], 1)  // SubstCount
	binary.BigEndian.PutUint16(data[58:60], 0)  // SequenceIndex
	binary.BigEndian.PutUint16(data[60:62], 1)  // LookupListIndex

	// Input coverages at 70 and 76.
	binary.BigEndian.PutUint16(data[70:72], 1)
	binary.BigEndian.PutUint16(data[72:74], 1)
	binary.BigEndian.PutUint16(data[74:76], 10)
	binary.BigEndian.PutUint16(data[76:78], 1)
	binary.BigEndian.PutUint16(data[78:80], 1)
	binary.BigEndian.PutUint16(data[80:82], 11)

	// Lookup 1 at 90: LigatureSubst.
	binary.BigEndian.PutUint16(data[90:92], 4)
	binary.BigEndian.PutUint16(data[92:94], 0)
	binary.BigEndian.PutUint16(data[94:96], 1)
	binary.BigEndian.PutUint16(data[96:98], 8)

	// LigatureSubst at 98.
	binary.BigEndian.PutUint16(data[98:100], 1)
	binary.BigEndian.PutUint16(data[100:102], 30) // Coverage at 128
	binary.BigEndian.PutUint16(data[102:104], 1)
	binary.BigEndian.PutUint16(data[104:106], 8) // LigSet at 106

	// LigSet and Ligature.
	binary.BigEndian.PutUint16(data[106:108], 1)
	binary.BigEndian.PutUint16(data[108:110], 4)
	binary.BigEndian.PutUint16(data[110:112], 100)
	binary.BigEndian.PutUint16(data[112:114], 2)
	binary.BigEndian.PutUint16(data[114:116], 11)

	// Coverage at 128.
	binary.BigEndian.PutUint16(data[128:130], 1)
	binary.BigEndian.PutUint16(data[130:132], 1)
	binary.BigEndian.PutUint16(data[132:134], 10)

	gsub, err := ParseGSUB(data)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	got := gsub.Substitute([]int{10, 11, 12})
	if len(got) != 2 || got[0] != 100 || got[1] != 12 {
		t.Fatalf("expected chained context ligature [100 12], got %v", got)
	}
}

func TestGSUB_ContextFormat1SubRuleAppliesAtSequenceIndex(t *testing.T) {
	data := makeGSUBContextTestData(180)

	putLookupList(data, 40, 46, 120)
	putLookupTable(data, 46, 5, 54)

	// ContextSubst format 1 at 54: match 10,11,12 and substitute sequence index 2.
	binary.BigEndian.PutUint16(data[54:56], 1)
	binary.BigEndian.PutUint16(data[56:58], 36) // Coverage at 90.
	binary.BigEndian.PutUint16(data[58:60], 1)
	binary.BigEndian.PutUint16(data[60:62], 8) // SubRuleSet at 62.
	binary.BigEndian.PutUint16(data[62:64], 1)
	binary.BigEndian.PutUint16(data[64:66], 4) // SubRule at 66.
	binary.BigEndian.PutUint16(data[66:68], 3)
	binary.BigEndian.PutUint16(data[68:70], 1)
	binary.BigEndian.PutUint16(data[70:72], 11)
	binary.BigEndian.PutUint16(data[72:74], 12)
	binary.BigEndian.PutUint16(data[74:76], 2)
	binary.BigEndian.PutUint16(data[76:78], 1)
	putCoverageFormat1(data, 90, 10)

	putSingleSubstDeltaLookup(data, 120, 128, 12, 100)

	gsub, err := ParseGSUB(data)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	got := gsub.Substitute([]int{10, 11, 12, 10, 11, 13, 12})
	assertGlyphs(t, got, []int{10, 11, 112, 10, 11, 13, 12})
}

func TestGSUB_ContextFormat2ClassRuleAppliesAtSequenceIndex(t *testing.T) {
	data := makeGSUBContextTestData(220)

	putLookupList(data, 40, 46, 150)
	putLookupTable(data, 46, 5, 54)

	// ContextSubst format 2 at 54: first glyph class 1, then classes 2 and 3.
	binary.BigEndian.PutUint16(data[54:56], 2)
	binary.BigEndian.PutUint16(data[56:58], 36) // Coverage at 90.
	binary.BigEndian.PutUint16(data[58:60], 44) // ClassDef at 98.
	binary.BigEndian.PutUint16(data[60:62], 2)
	binary.BigEndian.PutUint16(data[62:64], 0)
	binary.BigEndian.PutUint16(data[64:66], 12) // SubClassSet for class 1 at 66.
	binary.BigEndian.PutUint16(data[66:68], 1)
	binary.BigEndian.PutUint16(data[68:70], 4) // SubClassRule at 70.
	binary.BigEndian.PutUint16(data[70:72], 3)
	binary.BigEndian.PutUint16(data[72:74], 1)
	binary.BigEndian.PutUint16(data[74:76], 2)
	binary.BigEndian.PutUint16(data[76:78], 3)
	binary.BigEndian.PutUint16(data[78:80], 2)
	binary.BigEndian.PutUint16(data[80:82], 1)
	putCoverageFormat1(data, 90, 20)
	putClassDefFormat1(data, 98, 20, []uint16{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 3})

	putSingleSubstDeltaLookup(data, 150, 158, 31, 100)

	gsub, err := ParseGSUB(data)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	got := gsub.Substitute([]int{20, 30, 31, 20, 30, 32, 31})
	assertGlyphs(t, got, []int{20, 30, 131, 20, 30, 32, 31})
}

func TestGSUB_ChainedContextFormat2ClassRuleUsesBacktrackAndLookahead(t *testing.T) {
	data := makeGSUBContextTestData(320)

	putLookupList(data, 40, 46, 260)
	putLookupTable(data, 46, 6, 54)

	// ChainedContextSubst format 2 at 54: backtrack class 1, input class 2,
	// lookahead class 3, then substitute the input glyph.
	binary.BigEndian.PutUint16(data[54:56], 2)
	binary.BigEndian.PutUint16(data[56:58], 106) // Coverage at 160.
	binary.BigEndian.PutUint16(data[58:60], 116) // Backtrack ClassDef at 170.
	binary.BigEndian.PutUint16(data[60:62], 130) // Input ClassDef at 184.
	binary.BigEndian.PutUint16(data[62:64], 144) // Lookahead ClassDef at 198.
	binary.BigEndian.PutUint16(data[64:66], 3)
	binary.BigEndian.PutUint16(data[66:68], 0)
	binary.BigEndian.PutUint16(data[68:70], 0)
	binary.BigEndian.PutUint16(data[70:72], 26) // ClassSet for input class 2 at 80.

	binary.BigEndian.PutUint16(data[80:82], 1)
	binary.BigEndian.PutUint16(data[82:84], 4) // ChainSubClassRule at 84.
	binary.BigEndian.PutUint16(data[84:86], 1)
	binary.BigEndian.PutUint16(data[86:88], 1)
	binary.BigEndian.PutUint16(data[88:90], 1)
	binary.BigEndian.PutUint16(data[90:92], 1)
	binary.BigEndian.PutUint16(data[92:94], 3)
	binary.BigEndian.PutUint16(data[94:96], 1)
	binary.BigEndian.PutUint16(data[96:98], 0)
	binary.BigEndian.PutUint16(data[98:100], 1)

	putCoverageFormat1(data, 160, 40)
	putClassDefFormat1(data, 170, 10, []uint16{1})
	putClassDefFormat1(data, 184, 40, []uint16{2})
	putClassDefFormat1(data, 198, 60, []uint16{3, 0})
	putSingleSubstDeltaLookup(data, 260, 268, 40, 100)

	gsub, err := ParseGSUB(data)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	got := gsub.Substitute([]int{10, 40, 60, 40, 60, 10, 40, 61})
	assertGlyphs(t, got, []int{10, 140, 60, 40, 60, 10, 40, 61})
}

func TestGSUB_ReverseChainingContextSingleSubst(t *testing.T) {
	data := make([]byte, 48)

	// ReverseChainSingleSubst format 1 at 0:
	// replace glyph 50 with 200 only between backtrack 40 and lookahead 60.
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 20)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint16(data[6:8], 26)
	binary.BigEndian.PutUint16(data[8:10], 1)
	binary.BigEndian.PutUint16(data[10:12], 32)
	binary.BigEndian.PutUint16(data[12:14], 1)
	binary.BigEndian.PutUint16(data[14:16], 200)
	putCoverageFormat1(data, 20, 50)
	putCoverageFormat1(data, 26, 40)
	putCoverageFormat1(data, 32, 60)

	gsub := &GSUB{Data: data}
	lookup := &LookupTable{Type: 8, SubtableOffsets: []uint16{0}}
	got := gsub.applyLookup(lookup, []int{40, 50, 60, 41, 50, 60})
	assertGlyphs(t, got, []int{40, 200, 60, 41, 50, 60})
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

func makeGSUBContextTestData(size int) []byte {
	data := make([]byte, size)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 20)
	binary.BigEndian.PutUint16(data[8:10], 40)
	binary.BigEndian.PutUint16(data[10:12], 0)
	putFeatureListWithLookup0(data, 20)
	return data
}

func putFeatureListWithLookup0(data []byte, offset int) {
	binary.BigEndian.PutUint16(data[offset:offset+2], 1)
	binary.BigEndian.PutUint32(data[offset+2:offset+6], 0x74657374)
	binary.BigEndian.PutUint16(data[offset+6:offset+8], 8)
	featureOffset := offset + 8
	binary.BigEndian.PutUint16(data[featureOffset:featureOffset+2], 0)
	binary.BigEndian.PutUint16(data[featureOffset+2:featureOffset+4], 1)
	binary.BigEndian.PutUint16(data[featureOffset+4:featureOffset+6], 0)
}

func putLookupList(data []byte, offset int, lookupOffsets ...int) {
	binary.BigEndian.PutUint16(data[offset:offset+2], uint16(len(lookupOffsets)))
	for i, lookupOffset := range lookupOffsets {
		binary.BigEndian.PutUint16(data[offset+2+i*2:offset+4+i*2], uint16(lookupOffset-offset))
	}
}

func putLookupTable(data []byte, offset int, lookupType uint16, subtableOffset int) {
	binary.BigEndian.PutUint16(data[offset:offset+2], lookupType)
	binary.BigEndian.PutUint16(data[offset+2:offset+4], 0)
	binary.BigEndian.PutUint16(data[offset+4:offset+6], 1)
	binary.BigEndian.PutUint16(data[offset+6:offset+8], uint16(subtableOffset-offset))
}

func putSingleSubstDeltaLookup(data []byte, lookupOffset int, subtableOffset int, glyph uint16, delta uint16) {
	putLookupTable(data, lookupOffset, 1, subtableOffset)
	binary.BigEndian.PutUint16(data[subtableOffset:subtableOffset+2], 1)
	binary.BigEndian.PutUint16(data[subtableOffset+2:subtableOffset+4], 6)
	binary.BigEndian.PutUint16(data[subtableOffset+4:subtableOffset+6], delta)
	putCoverageFormat1(data, subtableOffset+6, glyph)
}

func putCoverageFormat1(data []byte, offset int, glyphs ...uint16) {
	binary.BigEndian.PutUint16(data[offset:offset+2], 1)
	binary.BigEndian.PutUint16(data[offset+2:offset+4], uint16(len(glyphs)))
	for i, glyph := range glyphs {
		binary.BigEndian.PutUint16(data[offset+4+i*2:offset+6+i*2], glyph)
	}
}

func putClassDefFormat1(data []byte, offset int, startGlyph uint16, classes []uint16) {
	binary.BigEndian.PutUint16(data[offset:offset+2], 1)
	binary.BigEndian.PutUint16(data[offset+2:offset+4], startGlyph)
	binary.BigEndian.PutUint16(data[offset+4:offset+6], uint16(len(classes)))
	for i, class := range classes {
		binary.BigEndian.PutUint16(data[offset+6+i*2:offset+8+i*2], class)
	}
}

func assertGlyphs(t *testing.T, got []int, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected glyphs %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected glyphs %v, got %v", want, got)
		}
	}
}

func makeTestGDEFWithMarks(glyphs ...uint16) *GDEF {
	ranges := make([]ClassRangeRecord, len(glyphs))
	for i, glyph := range glyphs {
		ranges[i] = ClassRangeRecord{
			Start: glyph,
			End:   glyph,
			Class: gdefGlyphClassMark,
		}
	}
	return &GDEF{
		GlyphClassDef: &ClassDefFormat2{Ranges: ranges},
	}
}

func makeTestGDEFWithMarkAttachment(classes map[uint16]uint16) *GDEF {
	glyphRanges := make([]ClassRangeRecord, 0, len(classes))
	attachRanges := make([]ClassRangeRecord, 0, len(classes))
	for glyph, class := range classes {
		glyphRanges = append(glyphRanges, ClassRangeRecord{
			Start: glyph,
			End:   glyph,
			Class: gdefGlyphClassMark,
		})
		attachRanges = append(attachRanges, ClassRangeRecord{
			Start: glyph,
			End:   glyph,
			Class: class,
		})
	}
	return &GDEF{
		GlyphClassDef:      &ClassDefFormat2{Ranges: glyphRanges},
		MarkAttachClassDef: &ClassDefFormat2{Ranges: attachRanges},
	}
}
