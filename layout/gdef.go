package layout

import (
	"encoding/binary"
	"fmt"
)

const (
	lookupFlagIgnoreBaseGlyphs       uint16 = 0x0002
	lookupFlagIgnoreLigatures        uint16 = 0x0004
	lookupFlagIgnoreMarks            uint16 = 0x0008
	lookupFlagUseMarkFilteringSet    uint16 = 0x0010
	lookupFlagMarkAttachmentTypeMask uint16 = 0xff00

	gdefGlyphClassBase      = 1
	gdefGlyphClassLigature  = 2
	gdefGlyphClassMark      = 3
	gdefGlyphClassComponent = 4
)

// GDEF represents the parts of the Glyph Definition table needed by lookup flags.
type GDEF struct {
	Version            uint32
	GlyphClassDef      ClassDefTable
	MarkAttachClassDef ClassDefTable
	MarkGlyphSets      []CoverageTable
}

// ParseGDEF parses GDEF glyph class and mark filtering data.
func ParseGDEF(data []byte) (*GDEF, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("GDEF table too short")
	}

	gdef := &GDEF{
		Version: binary.BigEndian.Uint32(data[0:4]),
	}

	if offset := binary.BigEndian.Uint16(data[4:6]); offset != 0 {
		classDef, err := ParseClassDef(data, offset)
		if err != nil {
			return nil, fmt.Errorf("failed to parse GDEF GlyphClassDef: %v", err)
		}
		gdef.GlyphClassDef = classDef
	}

	if offset := binary.BigEndian.Uint16(data[10:12]); offset != 0 {
		classDef, err := ParseClassDef(data, offset)
		if err != nil {
			return nil, fmt.Errorf("failed to parse GDEF MarkAttachClassDef: %v", err)
		}
		gdef.MarkAttachClassDef = classDef
	}

	if gdef.Version >= 0x00010002 && len(data) >= 14 {
		if offset := binary.BigEndian.Uint16(data[12:14]); offset != 0 {
			sets, err := parseMarkGlyphSets(data, int(offset))
			if err != nil {
				return nil, err
			}
			gdef.MarkGlyphSets = sets
		}
	}

	return gdef, nil
}

func parseMarkGlyphSets(data []byte, offset int) ([]CoverageTable, error) {
	if !hasBytesAt(data, offset, 4) {
		return nil, fmt.Errorf("GDEF MarkGlyphSets table too short")
	}

	format := binary.BigEndian.Uint16(data[offset : offset+2])
	if format != 1 {
		return nil, fmt.Errorf("unsupported GDEF MarkGlyphSets format: %d", format)
	}

	count := binary.BigEndian.Uint16(data[offset+2 : offset+4])
	if !hasBytesAt(data, offset, 4+int(count)*4) {
		return nil, fmt.Errorf("GDEF MarkGlyphSets offsets too short")
	}

	sets := make([]CoverageTable, int(count))
	for i := 0; i < int(count); i++ {
		rel := binary.BigEndian.Uint32(data[offset+4+i*4 : offset+8+i*4])
		coverageOffset64 := uint64(offset) + uint64(rel)
		maxInt := uint64(^uint(0) >> 1)
		if coverageOffset64 > maxInt {
			return nil, fmt.Errorf("GDEF MarkGlyphSets coverage offset overflow")
		}
		coverage, err := parseCoverage(data, int(coverageOffset64))
		if err != nil {
			return nil, fmt.Errorf("failed to parse GDEF MarkGlyphSets coverage: %v", err)
		}
		sets[i] = coverage
	}

	return sets, nil
}

func lookupIgnoresGlyph(gdef *GDEF, lookup *LookupTable, glyphID int) bool {
	if gdef == nil || lookup == nil {
		return false
	}

	switch gdef.glyphClass(glyphID) {
	case gdefGlyphClassBase:
		return lookup.Flag&lookupFlagIgnoreBaseGlyphs != 0
	case gdefGlyphClassLigature:
		return lookup.Flag&lookupFlagIgnoreLigatures != 0
	case gdefGlyphClassMark:
		if lookup.Flag&lookupFlagIgnoreMarks != 0 {
			return true
		}
		markAttachmentType := int((lookup.Flag & lookupFlagMarkAttachmentTypeMask) >> 8)
		if markAttachmentType != 0 {
			if gdef.MarkAttachClassDef == nil || gdef.MarkAttachClassDef.GetClass(glyphID) != markAttachmentType {
				return true
			}
		}
		if lookup.Flag&lookupFlagUseMarkFilteringSet != 0 {
			setIndex := int(lookup.MarkFilteringSet)
			if setIndex < 0 || setIndex >= len(gdef.MarkGlyphSets) || gdef.MarkGlyphSets[setIndex] == nil {
				return true
			}
			return gdef.MarkGlyphSets[setIndex].GetIndex(glyphID) < 0
		}
	}

	return false
}

func (gdef *GDEF) glyphClass(glyphID int) int {
	if gdef == nil || gdef.GlyphClassDef == nil {
		return 0
	}
	return gdef.GlyphClassDef.GetClass(glyphID)
}

func lookupGlyphApplies(gdef *GDEF, lookup *LookupTable, glyphs []int, index int) bool {
	return index >= 0 && index < len(glyphs) && !lookupIgnoresGlyph(gdef, lookup, glyphs[index])
}

func nextLookupGlyphIndex(gdef *GDEF, lookup *LookupTable, glyphs []int, start int) int {
	for i := start; i < len(glyphs); i++ {
		if lookupGlyphApplies(gdef, lookup, glyphs, i) {
			return i
		}
	}
	return -1
}

func prevLookupGlyphIndex(gdef *GDEF, lookup *LookupTable, glyphs []int, start int) int {
	if start >= len(glyphs) {
		start = len(glyphs) - 1
	}
	for i := start; i >= 0; i-- {
		if lookupGlyphApplies(gdef, lookup, glyphs, i) {
			return i
		}
	}
	return -1
}

func countNonMarkGlyphs(gdef *GDEF, lookup *LookupTable, glyphs []int, start int, end int) int {
	if start < 0 {
		start = 0
	}
	if end > len(glyphs) {
		end = len(glyphs)
	}
	count := 0
	for i := start; i < end; i++ {
		if lookupIgnoresGlyph(gdef, lookup, glyphs[i]) {
			continue
		}
		if gdef != nil && gdef.glyphClass(glyphs[i]) == gdefGlyphClassMark {
			continue
		}
		count++
	}
	return count
}
