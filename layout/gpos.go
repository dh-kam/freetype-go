package layout

import (
	"encoding/binary"
	"fmt"
	"github.com/dh-kam/freetype-go/api"
)

// GPOS represents the Glyph Positioning table.
type GPOS struct {
	VersionMajor uint16
	VersionMinor uint16
	ScriptList   *ScriptList
	FeatureList  *FeatureList
	LookupList   *LookupList
	GDEF         *GDEF
	Data         []byte
}

// ParseGPOS parses the GPOS table.
func ParseGPOS(data []byte) (*GPOS, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("GPOS table too short")
	}

	major := binary.BigEndian.Uint16(data[0:2])
	minor := binary.BigEndian.Uint16(data[2:4])
	scriptListOff := binary.BigEndian.Uint16(data[4:6])
	featureListOff := binary.BigEndian.Uint16(data[6:8])
	lookupListOff := binary.BigEndian.Uint16(data[8:10])

	gpos := &GPOS{
		VersionMajor: major,
		VersionMinor: minor,
		Data:         data,
	}

	var err error
	gpos.ScriptList, err = ParseScriptList(data, scriptListOff)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GPOS ScriptList: %v", err)
	}
	gpos.FeatureList, err = ParseFeatureList(data, featureListOff)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GPOS FeatureList: %v", err)
	}
	gpos.LookupList, err = ParseLookupList(data, lookupListOff)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GPOS LookupList: %v", err)
	}

	return gpos, nil
}

func (g *GPOS) Position(glyphs []int) []api.Vector {
	if g.LookupList == nil {
		return make([]api.Vector, len(glyphs))
	}
	// If we have features, apply them. Otherwise apply all lookups for simple cases.
	if g.FeatureList != nil && len(g.FeatureList.Features) > 0 {
		return g.PositionFeatures(glyphs, defaultFeatureIndices(g.ScriptList, g.FeatureList, g.Data))
	}

	adjustments := make([]api.Vector, len(glyphs))
	for _, lookup := range g.LookupList.Lookups {
		g.applyLookup(lookup, glyphs, adjustments)
	}
	return adjustments
}

func (g *GPOS) PositionFeatures(glyphs []int, featureIndices []int) []api.Vector {
	adjustments := make([]api.Vector, len(glyphs))
	if g.LookupList == nil || g.FeatureList == nil {
		return adjustments
	}

	activeLookups := make(map[int]bool)
	for _, fIdx := range featureIndices {
		lIndices := g.FeatureList.GetFeatureLookups(g.Data, fIdx)
		for _, lIdx := range lIndices {
			activeLookups[int(lIdx)] = true
		}
	}

	for i, lookup := range g.LookupList.Lookups {
		if activeLookups[i] {
			g.applyLookup(lookup, glyphs, adjustments)
		}
	}

	return adjustments
}

func (g *GPOS) applyLookup(lookup *LookupTable, glyphs []int, adjustments []api.Vector) {
	if lookup == nil {
		return
	}
	switch lookup.Type {
	case 1: // Single Adjustment
		g.applySinglePos(lookup, glyphs, adjustments)
	case 2: // Pair Adjustment
		g.applyPairAdj(lookup, glyphs, adjustments)
	case 3: // Cursive Attachment
		g.applyCursivePos(lookup, glyphs, adjustments)
	case 4: // Mark-to-Base
		g.applyMarkToBasePos(lookup, glyphs, adjustments)
	case 5: // Mark-to-Ligature
		g.applyMarkToLigaturePos(lookup, glyphs, adjustments)
	case 6: // Mark-to-Mark
		g.applyMarkToMarkPos(lookup, glyphs, adjustments)
	case 9: // Extension Positioning
		g.applyExtensionPos(lookup, glyphs, adjustments)
	default:
		// Unsupported
	}
}

func (g *GPOS) applyCursivePos(lookup *LookupTable, glyphs []int, adjustments []api.Vector) {
	for _, off := range lookup.SubtableOffsets {
		if !hasTableBytes(g.Data, off, 6) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		if format != 1 {
			continue
		}

		coverageOffset, ok := resolveTableOffset(off, binary.BigEndian.Uint16(d[2:4]))
		if !ok {
			continue
		}
		coverage, err := ParseCoverage(g.Data, uint16(coverageOffset))
		if err != nil {
			continue
		}

		entryExitCount := binary.BigEndian.Uint16(d[4:6])
		if !hasTableBytes(g.Data, off, 6+int(entryExitCount)*4) {
			continue
		}

		for i := 0; i < len(glyphs); i++ {
			if lookupIgnoresGlyph(g.GDEF, lookup, glyphs[i]) {
				continue
			}
			prevGlyphIndex := prevLookupGlyphIndex(g.GDEF, lookup, glyphs, i-1)
			if prevGlyphIndex < 0 {
				continue
			}
			prevIndex := coverage.GetIndex(glyphs[prevGlyphIndex])
			currIndex := coverage.GetIndex(glyphs[i])
			if prevIndex < 0 || prevIndex >= int(entryExitCount) || currIndex < 0 || currIndex >= int(entryExitCount) {
				continue
			}

			_, exitAnchor, ok := g.readEntryExitRecord(int(off), prevIndex)
			if !ok || exitAnchor == nil {
				continue
			}
			entryAnchor, _, ok := g.readEntryExitRecord(int(off), currIndex)
			if !ok || entryAnchor == nil {
				continue
			}

			adjustments[i].X += adjustments[prevGlyphIndex].X + int32(exitAnchor.X-entryAnchor.X)
			adjustments[i].Y += adjustments[prevGlyphIndex].Y + int32(exitAnchor.Y-entryAnchor.Y)
		}
	}
}

func (g *GPOS) readEntryExitRecord(cursiveOffset int, coverageIndex int) (*anchor, *anchor, bool) {
	recordOffset := cursiveOffset + 6 + coverageIndex*4
	if coverageIndex < 0 || !hasBytesAt(g.Data, recordOffset, 4) {
		return nil, nil, false
	}

	entryRel := binary.BigEndian.Uint16(g.Data[recordOffset : recordOffset+2])
	exitRel := binary.BigEndian.Uint16(g.Data[recordOffset+2 : recordOffset+4])
	var entryAnchor *anchor
	var exitAnchor *anchor

	if entryRel != 0 {
		entryOffset := cursiveOffset + int(entryRel)
		if !hasBytesAt(g.Data, entryOffset, 6) {
			return nil, nil, false
		}
		entry := parseAnchor(g.Data, entryOffset)
		entryAnchor = &entry
	}
	if exitRel != 0 {
		exitOffset := cursiveOffset + int(exitRel)
		if !hasBytesAt(g.Data, exitOffset, 6) {
			return nil, nil, false
		}
		exit := parseAnchor(g.Data, exitOffset)
		exitAnchor = &exit
	}

	return entryAnchor, exitAnchor, true
}

func (g *GPOS) applySinglePos(lookup *LookupTable, glyphs []int, adjustments []api.Vector) {
	for _, off := range lookup.SubtableOffsets {
		if int(off)+6 > len(g.Data) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		coverageOff := binary.BigEndian.Uint16(d[2:4])
		coverageOffset, ok := resolveTableOffset(off, coverageOff)
		if !ok {
			continue
		}
		coverage, err := ParseCoverage(g.Data, uint16(coverageOffset))
		if err != nil {
			continue
		}

		valueFormat := binary.BigEndian.Uint16(d[4:6])

		if format == 1 {
			valueSize := ValueRecordSize(valueFormat)
			if int(off)+6+valueSize > len(g.Data) {
				continue
			}
			vr, _ := ParseValueRecord(d[6:6+valueSize], valueFormat)
			for i, gid := range glyphs {
				if lookupIgnoresGlyph(g.GDEF, lookup, gid) {
					continue
				}
				if coverage.GetIndex(gid) >= 0 {
					adjustments[i].X += int32(vr.XPlacement)
					adjustments[i].Y += int32(vr.YPlacement)
					// XAdvance and YAdvance would affect next glyph position, but we are just returning adjustments per glyph.
					// In a full implementation, XAdvance would be added to the overall pen position.
				}
			}
		} else if format == 2 {
			if int(off)+8 > len(g.Data) {
				continue
			}
			count := binary.BigEndian.Uint16(d[6:8])
			valueSize := ValueRecordSize(valueFormat)
			valuesStart := int(off) + 8
			if valuesStart+int(count)*valueSize > len(g.Data) {
				continue
			}
			for i, gid := range glyphs {
				if lookupIgnoresGlyph(g.GDEF, lookup, gid) {
					continue
				}
				idx := coverage.GetIndex(gid)
				if idx >= 0 && idx < int(count) {
					// This is inefficient to parse every time, but for simplicity:
					// We need to find the offset of the idx-th ValueRecord.
					// Each ValueRecord has the same size determined by valueFormat.
					valueOff := valuesStart + idx*valueSize
					vr, _ := ParseValueRecord(g.Data[valueOff:valueOff+valueSize], valueFormat)
					adjustments[i].X += int32(vr.XPlacement)
					adjustments[i].Y += int32(vr.YPlacement)
				}
			}
		}
	}
}

func (g *GPOS) applyMarkToBasePos(lookup *LookupTable, glyphs []int, adjustments []api.Vector) {
	for _, off := range lookup.SubtableOffsets {
		if !hasTableBytes(g.Data, off, 12) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		if format != 1 {
			continue
		}
		markCoverageOff := binary.BigEndian.Uint16(d[2:4])
		baseCoverageOff := binary.BigEndian.Uint16(d[4:6])
		markClassCount := binary.BigEndian.Uint16(d[6:8])
		markArrayOff := binary.BigEndian.Uint16(d[8:10])
		baseArrayOff := binary.BigEndian.Uint16(d[10:12])
		if markClassCount == 0 {
			continue
		}

		markCoverageOffset, ok := resolveTableOffset(off, markCoverageOff)
		if !ok {
			continue
		}
		baseCoverageOffset, ok := resolveTableOffset(off, baseCoverageOff)
		if !ok {
			continue
		}
		markCoverage, err := ParseCoverage(g.Data, uint16(markCoverageOffset))
		if err != nil {
			continue
		}
		baseCoverage, err := ParseCoverage(g.Data, uint16(baseCoverageOffset))
		if err != nil {
			continue
		}
		markArrayOffset, ok := resolveTableOffset(off, markArrayOff)
		if !ok || !hasBytesAt(g.Data, markArrayOffset, 2) {
			continue
		}
		baseArrayOffset, ok := resolveTableOffset(off, baseArrayOff)
		if !ok || !hasBytesAt(g.Data, baseArrayOffset, 2) {
			continue
		}

		// Iterate through glyphs to find marks
		for i := 1; i < len(glyphs); i++ {
			markGid := glyphs[i]
			if lookupIgnoresGlyph(g.GDEF, lookup, markGid) {
				continue
			}
			markIdx := markCoverage.GetIndex(markGid)
			if markIdx < 0 {
				continue
			}

			// Found a mark, find its preceding base
			baseIdxInGlyphs := -1
			for j := i - 1; j >= 0; j-- {
				if lookupIgnoresGlyph(g.GDEF, lookup, glyphs[j]) {
					continue
				}
				if baseCoverage.GetIndex(glyphs[j]) >= 0 {
					baseIdxInGlyphs = j
					break
				}
			}

			if baseIdxInGlyphs == -1 {
				continue
			}

			baseGid := glyphs[baseIdxInGlyphs]
			baseIdx := baseCoverage.GetIndex(baseGid)

			// Get mark anchor
			mad := g.Data[markArrayOffset:]
			markCount := binary.BigEndian.Uint16(mad[0:2])
			if markIdx >= int(markCount) {
				continue
			}

			mRecOff := markArrayOffset + 2 + markIdx*4
			if !hasBytesAt(g.Data, mRecOff, 4) {
				continue
			}
			class := binary.BigEndian.Uint16(g.Data[mRecOff : mRecOff+2])
			if class >= markClassCount {
				continue
			}
			markAnchorOff := binary.BigEndian.Uint16(g.Data[mRecOff+2 : mRecOff+4])
			if markAnchorOff == 0 {
				continue
			}
			markAnchorOffset := markArrayOffset + int(markAnchorOff)
			if !hasBytesAt(g.Data, markAnchorOffset, 6) {
				continue
			}
			markAnchor := parseAnchor(g.Data, markAnchorOffset)

			// Get base anchor
			bad := g.Data[baseArrayOffset:]
			baseCount := binary.BigEndian.Uint16(bad[0:2])
			if baseIdx >= int(baseCount) {
				continue
			}

			bRecOff := baseArrayOffset + 2 + baseIdx*(int(markClassCount)*2)
			baseAnchorOffsetEntry := bRecOff + int(class)*2
			if !hasBytesAt(g.Data, baseAnchorOffsetEntry, 2) {
				continue
			}
			baseAnchorOff := binary.BigEndian.Uint16(g.Data[baseAnchorOffsetEntry : baseAnchorOffsetEntry+2])
			if baseAnchorOff == 0 {
				continue
			}
			baseAnchorOffset := baseArrayOffset + int(baseAnchorOff)
			if !hasBytesAt(g.Data, baseAnchorOffset, 6) {
				continue
			}
			baseAnchor := parseAnchor(g.Data, baseAnchorOffset)

			// Adjust mark position: mark anchor should coincide with base anchor
			// mark_pos + mark_anchor = base_pos + base_anchor
			// mark_pos = base_pos + base_anchor - mark_anchor
			// Since adjustments are relative to default position:
			// default_mark_pos + adj = default_base_pos + base_anchor - mark_anchor
			// For simplicity, we assume marks are normally at (0,0) relative to base or something.
			// The adjustment should be:
			adjustments[i].X += int32(baseAnchor.X - markAnchor.X)
			adjustments[i].Y += int32(baseAnchor.Y - markAnchor.Y)

			// If the base also has an adjustment, it should be added?
			// Usually Mark-to-Base positioning is absolute relative to the base's origin.
			adjustments[i].X += adjustments[baseIdxInGlyphs].X
			adjustments[i].Y += adjustments[baseIdxInGlyphs].Y
		}
	}
}

func (g *GPOS) applyMarkToLigaturePos(lookup *LookupTable, glyphs []int, adjustments []api.Vector) {
	for _, off := range lookup.SubtableOffsets {
		if !hasTableBytes(g.Data, off, 12) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		if format != 1 {
			continue
		}

		markCoverageOff := binary.BigEndian.Uint16(d[2:4])
		ligatureCoverageOff := binary.BigEndian.Uint16(d[4:6])
		markClassCount := binary.BigEndian.Uint16(d[6:8])
		markArrayOff := binary.BigEndian.Uint16(d[8:10])
		ligatureArrayOff := binary.BigEndian.Uint16(d[10:12])
		if markClassCount == 0 {
			continue
		}

		markCoverageOffset, ok := resolveTableOffset(off, markCoverageOff)
		if !ok {
			continue
		}
		ligatureCoverageOffset, ok := resolveTableOffset(off, ligatureCoverageOff)
		if !ok {
			continue
		}
		markCoverage, err := ParseCoverage(g.Data, uint16(markCoverageOffset))
		if err != nil {
			continue
		}
		ligatureCoverage, err := ParseCoverage(g.Data, uint16(ligatureCoverageOffset))
		if err != nil {
			continue
		}
		markArrayOffset, ok := resolveTableOffset(off, markArrayOff)
		if !ok || !hasBytesAt(g.Data, markArrayOffset, 2) {
			continue
		}
		ligatureArrayOffset, ok := resolveTableOffset(off, ligatureArrayOff)
		if !ok || !hasBytesAt(g.Data, ligatureArrayOffset, 2) {
			continue
		}

		for i := 1; i < len(glyphs); i++ {
			if lookupIgnoresGlyph(g.GDEF, lookup, glyphs[i]) {
				continue
			}
			markIndex := markCoverage.GetIndex(glyphs[i])
			if markIndex < 0 {
				continue
			}

			ligatureGlyphIndex := -1
			ligatureCoverageIndex := -1
			for j := i - 1; j >= 0; j-- {
				if lookupIgnoresGlyph(g.GDEF, lookup, glyphs[j]) {
					continue
				}
				if coverageIndex := ligatureCoverage.GetIndex(glyphs[j]); coverageIndex >= 0 {
					ligatureGlyphIndex = j
					ligatureCoverageIndex = coverageIndex
					break
				}
			}
			if ligatureGlyphIndex < 0 {
				continue
			}

			class, markAnchor, ok := g.readMarkArrayRecord(markArrayOffset, markIndex, markClassCount)
			if !ok {
				continue
			}
			componentIndex := i - ligatureGlyphIndex - 1
			if g.GDEF != nil {
				componentIndex = countNonMarkGlyphs(g.GDEF, lookup, glyphs, ligatureGlyphIndex+1, i)
			}
			ligatureAnchor, ok := g.readLigatureArrayAnchor(ligatureArrayOffset, ligatureCoverageIndex, componentIndex, int(class), markClassCount)
			if !ok {
				continue
			}

			adjustments[i].X += int32(ligatureAnchor.X-markAnchor.X) + adjustments[ligatureGlyphIndex].X
			adjustments[i].Y += int32(ligatureAnchor.Y-markAnchor.Y) + adjustments[ligatureGlyphIndex].Y
		}
	}
}

func (g *GPOS) applyMarkToMarkPos(lookup *LookupTable, glyphs []int, adjustments []api.Vector) {
	for _, off := range lookup.SubtableOffsets {
		if !hasTableBytes(g.Data, off, 12) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		if format != 1 {
			continue
		}

		mark1CoverageOff := binary.BigEndian.Uint16(d[2:4])
		mark2CoverageOff := binary.BigEndian.Uint16(d[4:6])
		markClassCount := binary.BigEndian.Uint16(d[6:8])
		mark1ArrayOff := binary.BigEndian.Uint16(d[8:10])
		mark2ArrayOff := binary.BigEndian.Uint16(d[10:12])
		if markClassCount == 0 {
			continue
		}

		mark1CoverageOffset, ok := resolveTableOffset(off, mark1CoverageOff)
		if !ok {
			continue
		}
		mark2CoverageOffset, ok := resolveTableOffset(off, mark2CoverageOff)
		if !ok {
			continue
		}
		mark1Coverage, err := ParseCoverage(g.Data, uint16(mark1CoverageOffset))
		if err != nil {
			continue
		}
		mark2Coverage, err := ParseCoverage(g.Data, uint16(mark2CoverageOffset))
		if err != nil {
			continue
		}
		mark1ArrayOffset, ok := resolveTableOffset(off, mark1ArrayOff)
		if !ok || !hasBytesAt(g.Data, mark1ArrayOffset, 2) {
			continue
		}
		mark2ArrayOffset, ok := resolveTableOffset(off, mark2ArrayOff)
		if !ok || !hasBytesAt(g.Data, mark2ArrayOffset, 2) {
			continue
		}

		for i := 1; i < len(glyphs); i++ {
			if lookupIgnoresGlyph(g.GDEF, lookup, glyphs[i]) {
				continue
			}
			mark1Index := mark1Coverage.GetIndex(glyphs[i])
			if mark1Index < 0 {
				continue
			}

			mark2GlyphIndex := -1
			mark2CoverageIndex := -1
			for j := i - 1; j >= 0; j-- {
				if lookupIgnoresGlyph(g.GDEF, lookup, glyphs[j]) {
					continue
				}
				if coverageIndex := mark2Coverage.GetIndex(glyphs[j]); coverageIndex >= 0 {
					mark2GlyphIndex = j
					mark2CoverageIndex = coverageIndex
					break
				}
			}
			if mark2GlyphIndex < 0 {
				continue
			}

			class, mark1Anchor, ok := g.readMarkArrayRecord(mark1ArrayOffset, mark1Index, markClassCount)
			if !ok {
				continue
			}
			mark2Anchor, ok := g.readMark2ArrayAnchor(mark2ArrayOffset, mark2CoverageIndex, int(class), markClassCount)
			if !ok {
				continue
			}

			adjustments[i].X += int32(mark2Anchor.X-mark1Anchor.X) + adjustments[mark2GlyphIndex].X
			adjustments[i].Y += int32(mark2Anchor.Y-mark1Anchor.Y) + adjustments[mark2GlyphIndex].Y
		}
	}
}

func (g *GPOS) readMarkArrayRecord(markArrayOffset int, markIndex int, markClassCount uint16) (uint16, anchor, bool) {
	if markIndex < 0 || !hasBytesAt(g.Data, markArrayOffset, 2) {
		return 0, anchor{}, false
	}
	markCount := binary.BigEndian.Uint16(g.Data[markArrayOffset : markArrayOffset+2])
	if markIndex >= int(markCount) {
		return 0, anchor{}, false
	}

	recordOffset := markArrayOffset + 2 + markIndex*4
	if !hasBytesAt(g.Data, recordOffset, 4) {
		return 0, anchor{}, false
	}
	class := binary.BigEndian.Uint16(g.Data[recordOffset : recordOffset+2])
	if class >= markClassCount {
		return 0, anchor{}, false
	}

	anchorOffset := binary.BigEndian.Uint16(g.Data[recordOffset+2 : recordOffset+4])
	if anchorOffset == 0 {
		return 0, anchor{}, false
	}
	anchorAbsoluteOffset := markArrayOffset + int(anchorOffset)
	if !hasBytesAt(g.Data, anchorAbsoluteOffset, 6) {
		return 0, anchor{}, false
	}
	return class, parseAnchor(g.Data, anchorAbsoluteOffset), true
}

func (g *GPOS) readMark2ArrayAnchor(mark2ArrayOffset int, mark2Index int, class int, markClassCount uint16) (anchor, bool) {
	if mark2Index < 0 || class < 0 || class >= int(markClassCount) || !hasBytesAt(g.Data, mark2ArrayOffset, 2) {
		return anchor{}, false
	}
	mark2Count := binary.BigEndian.Uint16(g.Data[mark2ArrayOffset : mark2ArrayOffset+2])
	if mark2Index >= int(mark2Count) {
		return anchor{}, false
	}

	recordOffset := mark2ArrayOffset + 2 + mark2Index*int(markClassCount)*2
	anchorOffsetEntry := recordOffset + class*2
	if !hasBytesAt(g.Data, anchorOffsetEntry, 2) {
		return anchor{}, false
	}
	anchorOffset := binary.BigEndian.Uint16(g.Data[anchorOffsetEntry : anchorOffsetEntry+2])
	if anchorOffset == 0 {
		return anchor{}, false
	}

	anchorAbsoluteOffset := mark2ArrayOffset + int(anchorOffset)
	if !hasBytesAt(g.Data, anchorAbsoluteOffset, 6) {
		return anchor{}, false
	}
	return parseAnchor(g.Data, anchorAbsoluteOffset), true
}

func (g *GPOS) readLigatureArrayAnchor(ligatureArrayOffset int, ligatureIndex int, componentIndex int, class int, markClassCount uint16) (anchor, bool) {
	if ligatureIndex < 0 || class < 0 || class >= int(markClassCount) || !hasBytesAt(g.Data, ligatureArrayOffset, 2) {
		return anchor{}, false
	}
	ligatureCount := binary.BigEndian.Uint16(g.Data[ligatureArrayOffset : ligatureArrayOffset+2])
	if ligatureIndex >= int(ligatureCount) {
		return anchor{}, false
	}

	ligatureAttachOffsetEntry := ligatureArrayOffset + 2 + ligatureIndex*2
	if !hasBytesAt(g.Data, ligatureAttachOffsetEntry, 2) {
		return anchor{}, false
	}
	ligatureAttachRel := binary.BigEndian.Uint16(g.Data[ligatureAttachOffsetEntry : ligatureAttachOffsetEntry+2])
	if ligatureAttachRel == 0 {
		return anchor{}, false
	}
	ligatureAttachOffset := ligatureArrayOffset + int(ligatureAttachRel)
	if !hasBytesAt(g.Data, ligatureAttachOffset, 2) {
		return anchor{}, false
	}
	componentCount := binary.BigEndian.Uint16(g.Data[ligatureAttachOffset : ligatureAttachOffset+2])
	if componentCount == 0 {
		return anchor{}, false
	}
	if componentIndex < 0 {
		componentIndex = 0
	}
	if componentIndex >= int(componentCount) {
		componentIndex = int(componentCount) - 1
	}

	recordOffset := ligatureAttachOffset + 2 + componentIndex*int(markClassCount)*2
	anchorOffsetEntry := recordOffset + class*2
	if !hasBytesAt(g.Data, anchorOffsetEntry, 2) {
		return anchor{}, false
	}
	anchorOffset := binary.BigEndian.Uint16(g.Data[anchorOffsetEntry : anchorOffsetEntry+2])
	if anchorOffset == 0 {
		return anchor{}, false
	}

	anchorAbsoluteOffset := ligatureAttachOffset + int(anchorOffset)
	if !hasBytesAt(g.Data, anchorAbsoluteOffset, 6) {
		return anchor{}, false
	}
	return parseAnchor(g.Data, anchorAbsoluteOffset), true
}

type anchor struct {
	X, Y int16
}

func parseAnchor(data []byte, offset int) anchor {
	if !hasBytesAt(data, offset, 6) {
		return anchor{}
	}
	d := data[offset:]
	// format := binary.BigEndian.Uint16(d[0:2])
	x := int16(binary.BigEndian.Uint16(d[2:4]))
	y := int16(binary.BigEndian.Uint16(d[4:6]))
	return anchor{X: x, Y: y}
}

func (g *GPOS) applyPairAdj(lookup *LookupTable, glyphs []int, adjustments []api.Vector) {
	for _, off := range lookup.SubtableOffsets {
		if !hasTableBytes(g.Data, off, 8) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		coverageOff := binary.BigEndian.Uint16(d[2:4])
		coverageOffset, ok := resolveTableOffset(off, coverageOff)
		if !ok {
			continue
		}
		coverage, err := ParseCoverage(g.Data, uint16(coverageOffset))
		if err != nil {
			continue
		}

		valFormat1 := binary.BigEndian.Uint16(d[4:6])
		valFormat2 := binary.BigEndian.Uint16(d[6:8])

		if format == 1 { // Adjustments for Glyph Pairs
			if !hasTableBytes(g.Data, off, 10) {
				continue
			}
			pairSetCount := binary.BigEndian.Uint16(d[8:10])
			if !hasTableBytes(g.Data, off, 10+int(pairSetCount)*2) {
				continue
			}

			for i := 0; i < len(glyphs); i++ {
				if lookupIgnoresGlyph(g.GDEF, lookup, glyphs[i]) {
					continue
				}
				secondIndex := nextLookupGlyphIndex(g.GDEF, lookup, glyphs, i+1)
				if secondIndex < 0 {
					continue
				}
				gid1 := glyphs[i]
				gid2 := glyphs[secondIndex]
				covIdx := coverage.GetIndex(gid1)
				if covIdx >= 0 && covIdx < int(pairSetCount) {
					pairSetRel := binary.BigEndian.Uint16(d[10+covIdx*2 : 12+covIdx*2])
					psOff := int(off) + int(pairSetRel)
					if !hasBytesAt(g.Data, psOff, 2) {
						continue
					}
					psd := g.Data[psOff:]
					pairValueCount := binary.BigEndian.Uint16(psd[0:2])

					curr := psOff + 2
					for j := 0; j < int(pairValueCount); j++ {
						size1 := ValueRecordSize(valFormat1)
						size2 := ValueRecordSize(valFormat2)
						if !hasBytesAt(g.Data, curr, 2+size1+size2) {
							break
						}
						secondGlyph := binary.BigEndian.Uint16(g.Data[curr : curr+2])
						if int(secondGlyph) == gid2 {
							vr1, size1 := ParseValueRecord(g.Data[curr+2:], valFormat1)
							vr2, _ := ParseValueRecord(g.Data[curr+2+size1:], valFormat2)

							applyPairValueRecord(adjustments, i, vr1)
							applyPairValueRecord(adjustments, secondIndex, vr2)
							break
						}
						// Need to calculate size of pair value record to skip
						curr += 2 + size1 + size2
					}
				}
			}
		} else if format == 2 { // Class Pair Adjustment
			if !hasTableBytes(g.Data, off, 16) {
				continue
			}
			classDef1Off := binary.BigEndian.Uint16(d[8:10])
			classDef2Off := binary.BigEndian.Uint16(d[10:12])
			class1Count := binary.BigEndian.Uint16(d[12:14])
			class2Count := binary.BigEndian.Uint16(d[14:16])
			if class1Count == 0 || class2Count == 0 {
				continue
			}

			classDef1Offset, ok := resolveTableOffset(off, classDef1Off)
			if !ok {
				continue
			}
			classDef2Offset, ok := resolveTableOffset(off, classDef2Off)
			if !ok {
				continue
			}
			classDef1, err := ParseClassDef(g.Data, uint16(classDef1Offset))
			if err != nil {
				continue
			}
			classDef2, err := ParseClassDef(g.Data, uint16(classDef2Offset))
			if err != nil {
				continue
			}

			valueSize1 := ValueRecordSize(valFormat1)
			valueSize2 := ValueRecordSize(valFormat2)
			class2RecordSize := valueSize1 + valueSize2
			if class2RecordSize == 0 {
				continue
			}

			classRecordsOffset := int(off) + 16
			for i := 0; i < len(glyphs); i++ {
				if lookupIgnoresGlyph(g.GDEF, lookup, glyphs[i]) {
					continue
				}
				secondIndex := nextLookupGlyphIndex(g.GDEF, lookup, glyphs, i+1)
				if secondIndex < 0 {
					continue
				}
				if coverage.GetIndex(glyphs[i]) < 0 {
					continue
				}

				class1 := classDef1.GetClass(glyphs[i])
				class2 := classDef2.GetClass(glyphs[secondIndex])
				if class1 < 0 || class1 >= int(class1Count) || class2 < 0 || class2 >= int(class2Count) {
					continue
				}

				pairIndex := class1*int(class2Count) + class2
				recordOffset := classRecordsOffset + pairIndex*class2RecordSize
				if recordOffset < classRecordsOffset || !hasBytesAt(g.Data, recordOffset, class2RecordSize) {
					continue
				}

				vr1, _ := ParseValueRecord(g.Data[recordOffset:recordOffset+valueSize1], valFormat1)
				vr2, _ := ParseValueRecord(g.Data[recordOffset+valueSize1:recordOffset+class2RecordSize], valFormat2)
				applyPairValueRecord(adjustments, i, vr1)
				applyPairValueRecord(adjustments, secondIndex, vr2)
			}
		}
	}
}

func applyPairValueRecord(adjustments []api.Vector, index int, vr ValueRecord) {
	if index < 0 || index >= len(adjustments) {
		return
	}
	adjustments[index].X += int32(vr.XPlacement) + int32(vr.XAdvance)
	adjustments[index].Y += int32(vr.YPlacement) + int32(vr.YAdvance)
}

func (g *GPOS) applyExtensionPos(lookup *LookupTable, glyphs []int, adjustments []api.Vector) {
	for _, off := range lookup.SubtableOffsets {
		extensionLookup, ok := parseExtensionLookup(g.Data, off, lookup.Flag, lookup.MarkFilteringSet, 9)
		if !ok {
			continue
		}
		g.applyLookup(extensionLookup, glyphs, adjustments)
	}
}
