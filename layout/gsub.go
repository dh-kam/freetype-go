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
		return g.SubstituteFeatures(glyphs, defaultFeatureIndices(g.ScriptList, g.FeatureList, g.Data))
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
	if lookup == nil {
		return glyphs
	}
	switch lookup.Type {
	case 1: // Single Substitution
		return g.applySingleSubst(lookup, glyphs)
	case 4: // Ligature Substitution
		return g.applyLigatureSubst(lookup, glyphs)
	case 6: // Chaining Contextual Substitution
		return g.applyChainingContextSubst(lookup, glyphs)
	case 7: // Extension Substitution
		return g.applyExtensionSubst(lookup, glyphs)
	default:
		return glyphs
	}
}

type chainSubstRecord struct {
	index       uint16
	lookupIndex uint16
}

type chainingContextFormat3 struct {
	backtrackCoverages []CoverageTable
	inputCoverages     []CoverageTable
	lookaheadCoverages []CoverageTable
	substRecords       []chainSubstRecord
}

func (g *GSUB) applyChainingContextSubst(lookup *LookupTable, glyphs []int) []int {
	if g.LookupList == nil {
		return glyphs
	}

	out := make([]int, len(glyphs))
	copy(out, glyphs)

	for _, off := range lookup.SubtableOffsets {
		subtable, ok := g.parseChainingContextFormat3(off)
		if !ok {
			continue
		}

		for i := 0; i < len(out); {
			if !subtable.matches(out, i) {
				i++
				continue
			}

			inputEnd := i + len(subtable.inputCoverages)
			lengthChanged := false

			for _, rec := range subtable.substRecords {
				if lengthChanged {
					break
				}
				if int(rec.lookupIndex) >= len(g.LookupList.Lookups) {
					continue
				}
				sequenceIndex := int(rec.index)
				if sequenceIndex < 0 || sequenceIndex >= len(subtable.inputCoverages) {
					continue
				}
				targetIndex := i + sequenceIndex
				beforeLen := len(out)
				next, changed := g.applyContextLookupAt(g.LookupList.Lookups[rec.lookupIndex], out, targetIndex, inputEnd)
				if !changed {
					continue
				}
				out = next
				delta := len(out) - beforeLen
				inputEnd += delta
				lengthChanged = delta != 0
			}

			if inputEnd <= i {
				i++
			} else {
				i = inputEnd
			}
		}
	}
	return out
}

func (g *GSUB) parseChainingContextFormat3(off uint16) (*chainingContextFormat3, bool) {
	if !hasTableBytes(g.Data, off, 4) {
		return nil, false
	}
	d := g.Data[off:]
	format := binary.BigEndian.Uint16(d[0:2])
	if format != 3 {
		return nil, false
	}

	curr := 2
	readCoverages := func() ([]CoverageTable, bool) {
		if !hasBytesAt(g.Data, int(off)+curr, 2) {
			return nil, false
		}
		count := binary.BigEndian.Uint16(g.Data[int(off)+curr : int(off)+curr+2])
		curr += 2
		if !hasBytesAt(g.Data, int(off)+curr, int(count)*2) {
			return nil, false
		}

		coverages := make([]CoverageTable, int(count))
		for i := 0; i < int(count); i++ {
			coverageRel := binary.BigEndian.Uint16(g.Data[int(off)+curr : int(off)+curr+2])
			curr += 2
			coverageOffset, ok := resolveTableOffset(off, coverageRel)
			if !ok {
				return nil, false
			}
			coverage, err := ParseCoverage(g.Data, uint16(coverageOffset))
			if err != nil {
				return nil, false
			}
			coverages[i] = coverage
		}
		return coverages, true
	}

	backtrackCoverages, ok := readCoverages()
	if !ok {
		return nil, false
	}
	inputCoverages, ok := readCoverages()
	if !ok || len(inputCoverages) == 0 {
		return nil, false
	}
	lookaheadCoverages, ok := readCoverages()
	if !ok {
		return nil, false
	}

	if !hasBytesAt(g.Data, int(off)+curr, 2) {
		return nil, false
	}
	substCount := binary.BigEndian.Uint16(g.Data[int(off)+curr : int(off)+curr+2])
	curr += 2
	if !hasBytesAt(g.Data, int(off)+curr, int(substCount)*4) {
		return nil, false
	}

	substRecords := make([]chainSubstRecord, int(substCount))
	for i := 0; i < int(substCount); i++ {
		recordOffset := int(off) + curr + i*4
		substRecords[i] = chainSubstRecord{
			index:       binary.BigEndian.Uint16(g.Data[recordOffset : recordOffset+2]),
			lookupIndex: binary.BigEndian.Uint16(g.Data[recordOffset+2 : recordOffset+4]),
		}
	}

	return &chainingContextFormat3{
		backtrackCoverages: backtrackCoverages,
		inputCoverages:     inputCoverages,
		lookaheadCoverages: lookaheadCoverages,
		substRecords:       substRecords,
	}, true
}

func (c *chainingContextFormat3) matches(glyphs []int, index int) bool {
	if index < len(c.backtrackCoverages) || index+len(c.inputCoverages)+len(c.lookaheadCoverages) > len(glyphs) {
		return false
	}
	for i, coverage := range c.inputCoverages {
		if coverage.GetIndex(glyphs[index+i]) < 0 {
			return false
		}
	}
	for i, coverage := range c.backtrackCoverages {
		if coverage.GetIndex(glyphs[index-1-i]) < 0 {
			return false
		}
	}
	lookaheadStart := index + len(c.inputCoverages)
	for i, coverage := range c.lookaheadCoverages {
		if coverage.GetIndex(glyphs[lookaheadStart+i]) < 0 {
			return false
		}
	}
	return true
}

func (g *GSUB) applyContextLookupAt(lookup *LookupTable, glyphs []int, index int, limit int) ([]int, bool) {
	if lookup == nil || index < 0 || index >= len(glyphs) {
		return glyphs, false
	}
	if limit > len(glyphs) {
		limit = len(glyphs)
	}
	if limit <= index {
		return glyphs, false
	}

	switch lookup.Type {
	case 1:
		substituted := g.applySingleSubst(lookup, []int{glyphs[index]})
		if len(substituted) != 1 || substituted[0] == glyphs[index] {
			return glyphs, false
		}
		glyphs[index] = substituted[0]
		return glyphs, true
	case 4:
		replacement, consumed, ok := g.firstLigatureSubstitution(lookup, glyphs[index:limit])
		if !ok {
			return glyphs, false
		}
		next := make([]int, 0, len(glyphs)-consumed+len(replacement))
		next = append(next, glyphs[:index]...)
		next = append(next, replacement...)
		next = append(next, glyphs[index+consumed:]...)
		return next, true
	case 7:
		out := glyphs
		changed := false
		for _, off := range lookup.SubtableOffsets {
			extensionLookup, ok := parseExtensionLookup(g.Data, off, lookup.Flag, 7)
			if !ok {
				continue
			}
			beforeLen := len(out)
			next, didChange := g.applyContextLookupAt(extensionLookup, out, index, limit)
			if !didChange {
				continue
			}
			out = next
			limit += len(out) - beforeLen
			changed = true
		}
		return out, changed
	default:
		return glyphs, false
	}
}

func (g *GSUB) applySingleSubst(lookup *LookupTable, glyphs []int) []int {
	newGlyphs := make([]int, len(glyphs))
	copy(newGlyphs, glyphs)

	for _, off := range lookup.SubtableOffsets {
		if !hasTableBytes(g.Data, off, 6) {
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

		if format == 1 {
			delta := int16(binary.BigEndian.Uint16(d[4:6]))
			for i, gid := range newGlyphs {
				idx := coverage.GetIndex(gid)
				if idx >= 0 {
					newGlyphs[i] = int(uint16(int32(gid) + int32(delta)))
				}
			}
		} else if format == 2 {
			if !hasTableBytes(g.Data, off, 6) {
				continue
			}
			count := binary.BigEndian.Uint16(d[4:6])
			if !hasTableBytes(g.Data, off, 6+int(count)*2) {
				continue
			}
			substitutes := make([]uint16, int(count))
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

	newGlyphs := make([]int, 0, len(glyphs))
	for i := 0; i < len(glyphs); {
		replacement, consumed, ok := g.firstLigatureSubstitution(lookup, glyphs[i:])
		if ok {
			newGlyphs = append(newGlyphs, replacement...)
			i += consumed
			continue
		}
		newGlyphs = append(newGlyphs, glyphs[i])
		i++
	}
	return newGlyphs
}

func (g *GSUB) firstLigatureSubstitution(lookup *LookupTable, glyphs []int) ([]int, int, bool) {
	if lookup == nil || len(glyphs) == 0 {
		return nil, 0, false
	}
	for _, off := range lookup.SubtableOffsets {
		if !hasTableBytes(g.Data, off, 6) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		if format != 1 {
			continue
		}
		coverageOff := binary.BigEndian.Uint16(d[2:4])
		coverageOffset, ok := resolveTableOffset(off, coverageOff)
		if !ok {
			continue
		}
		coverage, err := ParseCoverage(g.Data, uint16(coverageOffset))
		if err != nil {
			continue
		}

		ligSetCount := binary.BigEndian.Uint16(d[4:6])
		if !hasTableBytes(g.Data, off, 6+int(ligSetCount)*2) {
			continue
		}
		covIdx := coverage.GetIndex(glyphs[0])
		if covIdx < 0 || covIdx >= int(ligSetCount) {
			continue
		}

		ligSetRel := binary.BigEndian.Uint16(d[6+covIdx*2 : 8+covIdx*2])
		ligSetOffset, ok := resolveTableOffset(off, ligSetRel)
		if !ok || !hasBytesAt(g.Data, ligSetOffset, 2) {
			continue
		}
		ligSetData := g.Data[ligSetOffset:]
		ligCount := binary.BigEndian.Uint16(ligSetData[0:2])
		if !hasBytesAt(g.Data, ligSetOffset, 2+int(ligCount)*2) {
			continue
		}

		for i := 0; i < int(ligCount); i++ {
			ligRel := binary.BigEndian.Uint16(ligSetData[2+i*2 : 4+i*2])
			ligOffset := ligSetOffset + int(ligRel)
			if !hasBytesAt(g.Data, ligOffset, 4) {
				continue
			}
			ligData := g.Data[ligOffset:]
			ligGlyph := binary.BigEndian.Uint16(ligData[0:2])
			compCount := binary.BigEndian.Uint16(ligData[2:4])
			if compCount == 0 || int(compCount) > len(glyphs) {
				continue
			}
			if !hasBytesAt(g.Data, ligOffset, 4+(int(compCount)-1)*2) {
				continue
			}

			match := true
			for j := 1; j < int(compCount); j++ {
				componentGlyph := binary.BigEndian.Uint16(ligData[4+(j-1)*2 : 6+(j-1)*2])
				if glyphs[j] != int(componentGlyph) {
					match = false
					break
				}
			}
			if match {
				return []int{int(ligGlyph)}, int(compCount), true
			}
		}
	}
	return nil, 0, false
}

func (g *GSUB) applyExtensionSubst(lookup *LookupTable, glyphs []int) []int {
	out := glyphs
	for _, off := range lookup.SubtableOffsets {
		extensionLookup, ok := parseExtensionLookup(g.Data, off, lookup.Flag, 7)
		if !ok {
			continue
		}
		out = g.applyLookup(extensionLookup, out)
	}
	return out
}
