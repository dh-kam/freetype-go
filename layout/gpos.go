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
		indices := make([]int, len(g.FeatureList.Features))
		for i := range indices {
			indices[i] = i
		}
		return g.PositionFeatures(glyphs, indices)
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
	switch lookup.Type {
	case 1: // Single Adjustment
		g.applySinglePos(lookup, glyphs, adjustments)
	case 2: // Pair Adjustment
		g.applyPairAdj(lookup, glyphs, adjustments)
	case 4: // Mark-to-Base
		g.applyMarkToBasePos(lookup, glyphs, adjustments)
	default:
		// Unsupported
	}
}

func (g *GPOS) applySinglePos(lookup *LookupTable, glyphs []int, adjustments []api.Vector) {
	for _, off := range lookup.SubtableOffsets {
		if int(off)+6 > len(g.Data) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		coverageOff := binary.BigEndian.Uint16(d[2:4])
		coverage, err := ParseCoverage(g.Data, off+coverageOff)
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
		d := g.Data[off:]
		markCoverageOff := binary.BigEndian.Uint16(d[2:4])
		baseCoverageOff := binary.BigEndian.Uint16(d[4:6])
		markClassCount := binary.BigEndian.Uint16(d[6:8])
		markArrayOff := binary.BigEndian.Uint16(d[8:10])
		baseArrayOff := binary.BigEndian.Uint16(d[10:12])

		markCoverage, _ := ParseCoverage(g.Data, off+markCoverageOff)
		baseCoverage, _ := ParseCoverage(g.Data, off+baseCoverageOff)

		// Iterate through glyphs to find marks
		for i := 1; i < len(glyphs); i++ {
			markGid := glyphs[i]
			markIdx := markCoverage.GetIndex(markGid)
			if markIdx < 0 {
				continue
			}

			// Found a mark, find its preceding base
			baseIdxInGlyphs := -1
			for j := i - 1; j >= 0; j-- {
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
			mad := g.Data[off+markArrayOff:]
			markCount := binary.BigEndian.Uint16(mad[0:2])
			if markIdx >= int(markCount) {
				continue
			}

			mRecOff := 2 + markIdx*4
			class := binary.BigEndian.Uint16(mad[mRecOff : mRecOff+2])
			markAnchorOff := binary.BigEndian.Uint16(mad[mRecOff+2 : mRecOff+4])
			markAnchor := parseAnchor(g.Data, off+markArrayOff+markAnchorOff)

			// Get base anchor
			bad := g.Data[off+baseArrayOff:]
			baseCount := binary.BigEndian.Uint16(bad[0:2])
			if baseIdx >= int(baseCount) {
				continue
			}

			bRecOff := 2 + baseIdx*(int(markClassCount)*2)
			baseAnchorOff := binary.BigEndian.Uint16(bad[bRecOff+int(class)*2 : bRecOff+int(class)*2+2])
			baseAnchor := parseAnchor(g.Data, off+baseArrayOff+baseAnchorOff)

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

type anchor struct {
	X, Y int16
}

func parseAnchor(data []byte, offset uint16) anchor {
	if int(offset)+6 > len(data) {
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
		if int(off)+8 > len(g.Data) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		coverageOff := binary.BigEndian.Uint16(d[2:4])
		coverage, err := ParseCoverage(g.Data, off+coverageOff)
		if err != nil {
			continue
		}

		valFormat1 := binary.BigEndian.Uint16(d[4:6])
		valFormat2 := binary.BigEndian.Uint16(d[6:8])

		if format == 1 { // Adjustments for Glyph Pairs
			if int(off)+10 > len(g.Data) {
				continue
			}
			pairSetCount := binary.BigEndian.Uint16(d[8:10])
			if int(off)+10+int(pairSetCount)*2 > len(g.Data) {
				continue
			}
			pairSetOffsets := make([]uint16, pairSetCount)
			for i := 0; i < int(pairSetCount); i++ {
				pairSetOffsets[i] = binary.BigEndian.Uint16(d[10+i*2 : 12+i*2])
			}

			for i := 0; i < len(glyphs)-1; i++ {
				gid1 := glyphs[i]
				gid2 := glyphs[i+1]
				covIdx := coverage.GetIndex(gid1)
				if covIdx >= 0 && covIdx < int(pairSetCount) {
					psOff := off + pairSetOffsets[covIdx]
					if int(psOff)+2 > len(g.Data) {
						continue
					}
					psd := g.Data[psOff:]
					pairValueCount := binary.BigEndian.Uint16(psd[0:2])

					curr := psOff + 2
					for j := 0; j < int(pairValueCount); j++ {
						size1 := ValueRecordSize(valFormat1)
						size2 := ValueRecordSize(valFormat2)
						if int(curr)+2+size1+size2 > len(g.Data) {
							break
						}
						secondGlyph := binary.BigEndian.Uint16(g.Data[curr : curr+2])
						if int(secondGlyph) == gid2 {
							vr1, size1 := ParseValueRecord(g.Data[curr+2:], valFormat1)
							vr2, _ := ParseValueRecord(g.Data[curr+2+uint16(size1):], valFormat2)

							adjustments[i].X += int32(vr1.XAdvance)
							adjustments[i+1].X += int32(vr2.XPlacement)
							// simplified: only applying XAdvance for first glyph and XPlacement for second
							break
						}
						// Need to calculate size of pair value record to skip
						curr += 2 + uint16(size1) + uint16(size2)
					}
				}
			}
		} else if format == 2 { // Class Pair Adjustment
			// Simplified: skip class based for now
		}
	}
}
