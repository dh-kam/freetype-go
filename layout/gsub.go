package layout

import (
	"encoding/binary"
	"fmt"
)

// GSUB represents the Glyph Substitution table.
type GSUB struct {
	VersionMajor uint16
	VersionMinor uint16
	ScriptList   *ScriptList
	FeatureList  *FeatureList
	LookupList   *LookupList
	Data         []byte
}

// ParseGSUB parses the GSUB table.
func ParseGSUB(data []byte) (*GSUB, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("GSUB table too short")
	}

	major := binary.BigEndian.Uint16(data[0:2])
	minor := binary.BigEndian.Uint16(data[2:4])
	scriptListOff := binary.BigEndian.Uint16(data[4:6])
	featureListOff := binary.BigEndian.Uint16(data[6:8])
	lookupListOff := binary.BigEndian.Uint16(data[8:10])

	gsub := &GSUB{
		VersionMajor: major,
		VersionMinor: minor,
		Data:         data,
	}

	var err error
	gsub.ScriptList, err = ParseScriptList(data, scriptListOff)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GSUB ScriptList: %v", err)
	}
	gsub.FeatureList, err = ParseFeatureList(data, featureListOff)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GSUB FeatureList: %v", err)
	}
	gsub.LookupList, err = ParseLookupList(data, lookupListOff)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GSUB LookupList: %v", err)
	}

	return gsub, nil
}

func (g *GSUB) Substitute(glyphs []int) []int {
	if g.LookupList == nil {
		return glyphs
	}
	// If we have features, apply them. Otherwise apply all lookups for simple cases.
	if g.FeatureList != nil && len(g.FeatureList.Features) > 0 {
		indices := make([]int, len(g.FeatureList.Features))
		for i := range indices {
			indices[i] = i
		}
		return g.SubstituteFeatures(glyphs, indices)
	}

	out := glyphs
	for _, lookup := range g.LookupList.Lookups {
		out = g.applyLookup(lookup, out)
	}
	return out
}

func (g *GSUB) SubstituteFeatures(glyphs []int, featureIndices []int) []int {
	if g.LookupList == nil || g.FeatureList == nil {
		return glyphs
	}

	out := make([]int, len(glyphs))
	copy(out, glyphs)

	// OpenType: Lookups should be applied in the order they appear in the LookupList,
	// but only if they are part of an active feature.
	activeLookups := make(map[int]bool)
	for _, fIdx := range featureIndices {
		lIndices := g.FeatureList.GetFeatureLookups(g.Data, fIdx)
		for _, lIdx := range lIndices {
			activeLookups[int(lIdx)] = true
		}
	}

	for i, lookup := range g.LookupList.Lookups {
		if activeLookups[i] {
			out = g.applyLookup(lookup, out)
		}
	}

	return out
}

func (g *GSUB) applyLookup(lookup *LookupTable, glyphs []int) []int {
	switch lookup.Type {
	case 1: // Single Substitution
		return g.applySingleSubst(lookup, glyphs)
	case 4: // Ligature Substitution
		return g.applyLigatureSubst(lookup, glyphs)
	case 6: // Chaining Contextual Substitution
		return g.applyChainingContextSubst(lookup, glyphs)
	default:
		return glyphs
	}
}

func (g *GSUB) applyChainingContextSubst(lookup *LookupTable, glyphs []int) []int {
	for _, off := range lookup.SubtableOffsets {
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		if format != 3 {
			continue // Only support format 3 for now
		}

		backtrackCount := binary.BigEndian.Uint16(d[2:4])
		curr := 4
		backtrackCoverages := make([]CoverageTable, backtrackCount)
		for i := 0; i < int(backtrackCount); i++ {
			cOff := binary.BigEndian.Uint16(d[curr : curr+2])
			backtrackCoverages[i], _ = ParseCoverage(g.Data, off+cOff)
			curr += 2
		}

		inputCount := binary.BigEndian.Uint16(d[curr : curr+2])
		curr += 2
		inputCoverages := make([]CoverageTable, inputCount)
		for i := 0; i < int(inputCount); i++ {
			cOff := binary.BigEndian.Uint16(d[curr : curr+2])
			inputCoverages[i], _ = ParseCoverage(g.Data, off+cOff)
			curr += 2
		}

		lookaheadCount := binary.BigEndian.Uint16(d[curr : curr+2])
		curr += 2
		lookaheadCoverages := make([]CoverageTable, lookaheadCount)
		for i := 0; i < int(lookaheadCount); i++ {
			cOff := binary.BigEndian.Uint16(d[curr : curr+2])
			lookaheadCoverages[i], _ = ParseCoverage(g.Data, off+cOff)
			curr += 2
		}

		substCount := binary.BigEndian.Uint16(d[curr : curr+2])
		curr += 2
		type substRecord struct {
			index       uint16
			lookupIndex uint16
		}
		substRecords := make([]substRecord, substCount)
		for i := 0; i < int(substCount); i++ {
			substRecords[i].index = binary.BigEndian.Uint16(d[curr : curr+2])
			substRecords[i].lookupIndex = binary.BigEndian.Uint16(d[curr+2 : curr+4])
			curr += 4
		}

		// Apply to glyphs
		newGlyphs := make([]int, len(glyphs))
		copy(newGlyphs, glyphs)

		for i := 0; i < len(newGlyphs); {
			if i < int(backtrackCount) || i+int(inputCount)+int(lookaheadCount) > len(newGlyphs) {
				i++
				continue
			}

			// Check input
			match := true
			for j := 0; j < int(inputCount); j++ {
				if inputCoverages[j].GetIndex(newGlyphs[i+j]) < 0 {
					match = false
					break
				}
			}
			if !match {
				i++
				continue
			}

			// Check backtrack
			for j := 0; j < int(backtrackCount); j++ {
				if backtrackCoverages[j].GetIndex(newGlyphs[i-1-j]) < 0 {
					match = false
					break
				}
			}
			if !match {
				i++
				continue
			}

			// Check lookahead
			for j := 0; j < int(lookaheadCount); j++ {
				if lookaheadCoverages[j].GetIndex(newGlyphs[i+int(inputCount)+j]) < 0 {
					match = false
					break
				}
			}
			if !match {
				i++
				continue
			}

			// Match found, apply substitutions
			// Important: Substitutions are applied to the input sequence.
			// They are lookups from the GSUB LookupList.
			tempGlyphs := newGlyphs[i : i+int(inputCount)]
			for _, rec := range substRecords {
				if int(rec.lookupIndex) < len(g.LookupList.Lookups) {
					targetLookup := g.LookupList.Lookups[rec.lookupIndex]
					// This is slightly tricky because applying a lookup might change the number of glyphs.
					// For simple contextual substitution, it usually replaces one glyph.
					// Here we apply it to the specific index within the matched input sequence.
					if int(rec.index) < len(tempGlyphs) {
						// Apply lookup to a single glyph at rec.index
						sub := g.applyLookup(targetLookup, tempGlyphs[rec.index:rec.index+1])
						// Replace in tempGlyphs. If sub has different length, this gets complicated.
						// Usually it's 1-to-1 or ligature.
						if len(sub) == 1 {
							tempGlyphs[rec.index] = sub[0]
						} else {
							// For now, handle 1-to-1.
						}
					}
				}
			}
			copy(newGlyphs[i:], tempGlyphs)
			i += int(inputCount)
		}
		glyphs = newGlyphs
	}
	return glyphs
}

func (g *GSUB) applySingleSubst(lookup *LookupTable, glyphs []int) []int {
	newGlyphs := make([]int, len(glyphs))
	copy(newGlyphs, glyphs)

	for _, off := range lookup.SubtableOffsets {
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		coverageOff := binary.BigEndian.Uint16(d[2:4])
		coverage, err := ParseCoverage(g.Data, off+coverageOff)
		if err != nil {
			continue
		}

		if format == 1 {
			delta := int16(binary.BigEndian.Uint16(d[4:6]))
			for i, gid := range newGlyphs {
				idx := coverage.GetIndex(gid)
				if idx >= 0 {
					newGlyphs[i] = int(uint16(int32(gid) + int32(delta)))
				}
			}
		} else if format == 2 {
			count := binary.BigEndian.Uint16(d[4:6])
			substitutes := make([]uint16, count)
			for i := 0; i < int(count); i++ {
				substitutes[i] = binary.BigEndian.Uint16(d[6+i*2 : 8+i*2])
			}
			for i, gid := range newGlyphs {
				idx := coverage.GetIndex(gid)
				if idx >= 0 && idx < int(count) {
					newGlyphs[i] = int(substitutes[idx])
				}
			}
		}
	}
	return newGlyphs
}

func (g *GSUB) applyLigatureSubst(lookup *LookupTable, glyphs []int) []int {
	if len(glyphs) == 0 {
		return glyphs
	}

	for _, off := range lookup.SubtableOffsets {
		d := g.Data[off:]
		coverageOff := binary.BigEndian.Uint16(d[2:4])
		coverage, err := ParseCoverage(g.Data, off+coverageOff)
		if err != nil {
			continue
		}

		ligSetCount := binary.BigEndian.Uint16(d[4:6])
		ligSetOffsets := make([]uint16, ligSetCount)
		for i := 0; i < int(ligSetCount); i++ {
			ligSetOffsets[i] = binary.BigEndian.Uint16(d[6+i*2 : 8+i*2])
		}

		newGlyphs := make([]int, 0, len(glyphs))
		for i := 0; i < len(glyphs); {
			gid := glyphs[i]
			covIdx := coverage.GetIndex(gid)
			if covIdx >= 0 && covIdx < int(ligSetCount) {
				// Found a start of a potential ligature
				ligSetOff := off + ligSetOffsets[covIdx]
				lsd := g.Data[ligSetOff:]
				ligCount := binary.BigEndian.Uint16(lsd[0:2])
				matched := false
				for j := 0; j < int(ligCount); j++ {
					ligOff := ligSetOff + binary.BigEndian.Uint16(lsd[2+j*2:4+j*2])
					ld := g.Data[ligOff:]
					ligGlyph := binary.BigEndian.Uint16(ld[0:2])
					compCount := binary.BigEndian.Uint16(ld[2:4])

					if i+int(compCount) <= len(glyphs) {
						matchComp := true
						for k := 1; k < int(compCount); k++ {
							if glyphs[i+k] != int(binary.BigEndian.Uint16(ld[4+(k-1)*2:6+(k-1)*2])) {
								matchComp = false
								break
							}
						}
						if matchComp {
							newGlyphs = append(newGlyphs, int(ligGlyph))
							i += int(compCount)
							matched = true
							break
						}
					}
				}
				if !matched {
					newGlyphs = append(newGlyphs, gid)
					i++
				}
			} else {
				newGlyphs = append(newGlyphs, gid)
				i++
			}
		}
		glyphs = newGlyphs
	}
	return glyphs
}
