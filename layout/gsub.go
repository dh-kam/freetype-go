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
	GDEF         *GDEF
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
	case 2: // Multiple Substitution
		return g.applyMultipleSubst(lookup, glyphs)
	case 3: // Alternate Substitution
		return g.applyAlternateSubst(lookup, glyphs)
	case 4: // Ligature Substitution
		return g.applyLigatureSubst(lookup, glyphs)
	case 5: // Contextual Substitution
		return g.applyContextSubst(lookup, glyphs)
	case 6: // Chaining Contextual Substitution
		return g.applyChainingContextSubst(lookup, glyphs)
	case 7: // Extension Substitution
		return g.applyExtensionSubst(lookup, glyphs)
	case 8: // Reverse Chaining Contextual Single Substitution
		return g.applyReverseChainingContextSingleSubst(lookup, glyphs)
	default:
		return glyphs
	}
}

type chainSubstRecord struct {
	index       uint16
	lookupIndex uint16
}

type glyphMatcher interface {
	matchesGlyph(glyphID int) bool
}

type exactGlyphMatcher uint16

func (m exactGlyphMatcher) matchesGlyph(glyphID int) bool {
	return glyphID == int(m)
}

type coverageGlyphMatcher struct {
	coverage CoverageTable
}

func (m coverageGlyphMatcher) matchesGlyph(glyphID int) bool {
	return m.coverage != nil && m.coverage.GetIndex(glyphID) >= 0
}

type coverageIndexGlyphMatcher struct {
	coverage CoverageTable
	index    int
}

func (m coverageIndexGlyphMatcher) matchesGlyph(glyphID int) bool {
	return m.coverage != nil && m.coverage.GetIndex(glyphID) == m.index
}

type classGlyphMatcher struct {
	classDef ClassDefTable
	class    uint16
}

func (m classGlyphMatcher) matchesGlyph(glyphID int) bool {
	return m.classDef != nil && m.classDef.GetClass(glyphID) == int(m.class)
}

type coverageClassGlyphMatcher struct {
	coverage CoverageTable
	classDef ClassDefTable
	class    uint16
}

func (m coverageClassGlyphMatcher) matchesGlyph(glyphID int) bool {
	return m.coverage != nil && m.coverage.GetIndex(glyphID) >= 0 && m.classDef != nil && m.classDef.GetClass(glyphID) == int(m.class)
}

type contextSubstRule struct {
	backtrack    []glyphMatcher
	input        []glyphMatcher
	lookahead    []glyphMatcher
	substRecords []chainSubstRecord
}

func (r *contextSubstRule) matches(glyphs []int, index int) bool {
	_, ok := r.matchIndexes(nil, nil, glyphs, index)
	return ok
}

func (r *contextSubstRule) matchIndexes(gdef *GDEF, lookup *LookupTable, glyphs []int, index int) ([]int, bool) {
	if r == nil || len(r.input) == 0 || index < 0 || index >= len(glyphs) {
		return nil, false
	}
	if !lookupGlyphApplies(gdef, lookup, glyphs, index) {
		return nil, false
	}
	inputIndexes := make([]int, len(r.input))
	curr := index
	for i, matcher := range r.input {
		if i > 0 {
			curr = nextLookupGlyphIndex(gdef, lookup, glyphs, curr+1)
			if curr < 0 {
				return nil, false
			}
		}
		if matcher == nil || !matcher.matchesGlyph(glyphs[curr]) {
			return nil, false
		}
		inputIndexes[i] = curr
	}
	curr = index
	for _, matcher := range r.backtrack {
		curr = prevLookupGlyphIndex(gdef, lookup, glyphs, curr-1)
		if curr < 0 || matcher == nil || !matcher.matchesGlyph(glyphs[curr]) {
			return nil, false
		}
	}
	curr = inputIndexes[len(inputIndexes)-1]
	for _, matcher := range r.lookahead {
		curr = nextLookupGlyphIndex(gdef, lookup, glyphs, curr+1)
		if curr < 0 || matcher == nil || !matcher.matchesGlyph(glyphs[curr]) {
			return nil, false
		}
	}
	return inputIndexes, true
}

func (g *GSUB) applyContextSubst(lookup *LookupTable, glyphs []int) []int {
	return g.applyContextualSubst(lookup, glyphs, g.parseContextSubstRules)
}

func (g *GSUB) applyChainingContextSubst(lookup *LookupTable, glyphs []int) []int {
	return g.applyContextualSubst(lookup, glyphs, g.parseChainingContextSubstRules)
}

func (g *GSUB) applyContextualSubst(lookup *LookupTable, glyphs []int, parseRules func(uint16) ([]contextSubstRule, bool)) []int {
	if g.LookupList == nil {
		return glyphs
	}

	out := make([]int, len(glyphs))
	copy(out, glyphs)

	for _, off := range lookup.SubtableOffsets {
		rules, ok := parseRules(off)
		if !ok {
			continue
		}

		for i := 0; i < len(out); {
			matched := false
			for ruleIndex := range rules {
				rule := &rules[ruleIndex]
				inputIndexes, ok := rule.matchIndexes(g.GDEF, lookup, out, i)
				if !ok {
					continue
				}

				matched = true
				inputEnd := inputIndexes[len(inputIndexes)-1] + 1
				lengthChanged := false

				for _, rec := range rule.substRecords {
					if lengthChanged {
						break
					}
					if int(rec.lookupIndex) >= len(g.LookupList.Lookups) {
						continue
					}
					sequenceIndex := int(rec.index)
					if sequenceIndex < 0 || sequenceIndex >= len(rule.input) {
						continue
					}
					targetIndex := inputIndexes[sequenceIndex]
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
				break
			}
			if !matched {
				i++
			}
		}
	}
	return out
}

func (g *GSUB) parseContextSubstRules(off uint16) ([]contextSubstRule, bool) {
	if !hasTableBytes(g.Data, off, 2) {
		return nil, false
	}
	d := g.Data[off:]
	format := binary.BigEndian.Uint16(d[0:2])
	switch format {
	case 1:
		return g.parseContextSubstFormat1(off)
	case 2:
		return g.parseContextSubstFormat2(off)
	case 3:
		return g.parseContextSubstFormat3(off)
	default:
		return nil, false
	}
}

func (g *GSUB) parseChainingContextSubstRules(off uint16) ([]contextSubstRule, bool) {
	if !hasTableBytes(g.Data, off, 2) {
		return nil, false
	}
	d := g.Data[off:]
	format := binary.BigEndian.Uint16(d[0:2])
	switch format {
	case 1:
		return g.parseChainingContextSubstFormat1(off)
	case 2:
		return g.parseChainingContextSubstFormat2(off)
	case 3:
		return g.parseChainingContextSubstFormat3(off)
	default:
		return nil, false
	}
}

func (g *GSUB) parseContextSubstFormat1(off uint16) ([]contextSubstRule, bool) {
	if !hasTableBytes(g.Data, off, 6) {
		return nil, false
	}
	d := g.Data[off:]
	coverage, ok := g.parseCoverageAt(off, binary.BigEndian.Uint16(d[2:4]))
	if !ok {
		return nil, false
	}
	subRuleSetCount := binary.BigEndian.Uint16(d[4:6])
	if !hasTableBytes(g.Data, off, 6+int(subRuleSetCount)*2) {
		return nil, false
	}

	var rules []contextSubstRule
	for setIndex := 0; setIndex < int(subRuleSetCount); setIndex++ {
		setRel := binary.BigEndian.Uint16(d[6+setIndex*2 : 8+setIndex*2])
		if setRel == 0 {
			continue
		}
		setOffset := int(off) + int(setRel)
		if !hasBytesAt(g.Data, setOffset, 2) {
			continue
		}
		subRuleCount := binary.BigEndian.Uint16(g.Data[setOffset : setOffset+2])
		if !hasBytesAt(g.Data, setOffset, 2+int(subRuleCount)*2) {
			continue
		}
		for i := 0; i < int(subRuleCount); i++ {
			ruleRel := binary.BigEndian.Uint16(g.Data[setOffset+2+i*2 : setOffset+4+i*2])
			rule, ok := g.parseContextSubRule(setOffset+int(ruleRel), coverageIndexGlyphMatcher{
				coverage: coverage,
				index:    setIndex,
			})
			if !ok {
				continue
			}
			rules = append(rules, rule)
		}
	}
	return rules, len(rules) > 0
}

func (g *GSUB) parseContextSubstFormat2(off uint16) ([]contextSubstRule, bool) {
	if !hasTableBytes(g.Data, off, 8) {
		return nil, false
	}
	d := g.Data[off:]
	coverage, ok := g.parseCoverageAt(off, binary.BigEndian.Uint16(d[2:4]))
	if !ok {
		return nil, false
	}
	classDef, ok := g.parseClassDefAt(off, binary.BigEndian.Uint16(d[4:6]))
	if !ok {
		return nil, false
	}
	subClassSetCount := binary.BigEndian.Uint16(d[6:8])
	if !hasTableBytes(g.Data, off, 8+int(subClassSetCount)*2) {
		return nil, false
	}

	var rules []contextSubstRule
	for setIndex := 0; setIndex < int(subClassSetCount); setIndex++ {
		setRel := binary.BigEndian.Uint16(d[8+setIndex*2 : 10+setIndex*2])
		if setRel == 0 {
			continue
		}
		setOffset := int(off) + int(setRel)
		if !hasBytesAt(g.Data, setOffset, 2) {
			continue
		}
		subClassRuleCount := binary.BigEndian.Uint16(g.Data[setOffset : setOffset+2])
		if !hasBytesAt(g.Data, setOffset, 2+int(subClassRuleCount)*2) {
			continue
		}
		for i := 0; i < int(subClassRuleCount); i++ {
			ruleRel := binary.BigEndian.Uint16(g.Data[setOffset+2+i*2 : setOffset+4+i*2])
			rule, ok := g.parseContextSubClassRule(setOffset+int(ruleRel), classDef, coverageClassGlyphMatcher{
				coverage: coverage,
				classDef: classDef,
				class:    uint16(setIndex),
			})
			if !ok {
				continue
			}
			rules = append(rules, rule)
		}
	}
	return rules, len(rules) > 0
}

func (g *GSUB) parseContextSubstFormat3(off uint16) ([]contextSubstRule, bool) {
	if !hasTableBytes(g.Data, off, 6) {
		return nil, false
	}
	d := g.Data[off:]
	glyphCount := binary.BigEndian.Uint16(d[2:4])
	substCount := binary.BigEndian.Uint16(d[4:6])
	if glyphCount == 0 || !hasTableBytes(g.Data, off, 6+int(glyphCount)*2+int(substCount)*4) {
		return nil, false
	}

	input := make([]glyphMatcher, int(glyphCount))
	for i := 0; i < int(glyphCount); i++ {
		coverage, ok := g.parseCoverageAt(off, binary.BigEndian.Uint16(d[6+i*2:8+i*2]))
		if !ok {
			return nil, false
		}
		input[i] = coverageGlyphMatcher{coverage: coverage}
	}
	records, ok := readSequenceLookupRecords(g.Data, int(off)+6+int(glyphCount)*2, substCount)
	if !ok {
		return nil, false
	}

	return []contextSubstRule{{input: input, substRecords: records}}, true
}

func (g *GSUB) parseChainingContextSubstFormat1(off uint16) ([]contextSubstRule, bool) {
	if !hasTableBytes(g.Data, off, 6) {
		return nil, false
	}
	d := g.Data[off:]
	coverage, ok := g.parseCoverageAt(off, binary.BigEndian.Uint16(d[2:4]))
	if !ok {
		return nil, false
	}
	chainSubRuleSetCount := binary.BigEndian.Uint16(d[4:6])
	if !hasTableBytes(g.Data, off, 6+int(chainSubRuleSetCount)*2) {
		return nil, false
	}

	var rules []contextSubstRule
	for setIndex := 0; setIndex < int(chainSubRuleSetCount); setIndex++ {
		setRel := binary.BigEndian.Uint16(d[6+setIndex*2 : 8+setIndex*2])
		if setRel == 0 {
			continue
		}
		setOffset := int(off) + int(setRel)
		if !hasBytesAt(g.Data, setOffset, 2) {
			continue
		}
		chainSubRuleCount := binary.BigEndian.Uint16(g.Data[setOffset : setOffset+2])
		if !hasBytesAt(g.Data, setOffset, 2+int(chainSubRuleCount)*2) {
			continue
		}
		for i := 0; i < int(chainSubRuleCount); i++ {
			ruleRel := binary.BigEndian.Uint16(g.Data[setOffset+2+i*2 : setOffset+4+i*2])
			rule, ok := g.parseChainSubRule(setOffset+int(ruleRel), coverageIndexGlyphMatcher{
				coverage: coverage,
				index:    setIndex,
			})
			if !ok {
				continue
			}
			rules = append(rules, rule)
		}
	}
	return rules, len(rules) > 0
}

func (g *GSUB) parseChainingContextSubstFormat2(off uint16) ([]contextSubstRule, bool) {
	if !hasTableBytes(g.Data, off, 12) {
		return nil, false
	}
	d := g.Data[off:]
	coverage, ok := g.parseCoverageAt(off, binary.BigEndian.Uint16(d[2:4]))
	if !ok {
		return nil, false
	}
	backtrackClassDef, ok := g.parseClassDefAt(off, binary.BigEndian.Uint16(d[4:6]))
	if !ok {
		return nil, false
	}
	inputClassDef, ok := g.parseClassDefAt(off, binary.BigEndian.Uint16(d[6:8]))
	if !ok {
		return nil, false
	}
	lookaheadClassDef, ok := g.parseClassDefAt(off, binary.BigEndian.Uint16(d[8:10]))
	if !ok {
		return nil, false
	}
	chainSubClassSetCount := binary.BigEndian.Uint16(d[10:12])
	if !hasTableBytes(g.Data, off, 12+int(chainSubClassSetCount)*2) {
		return nil, false
	}

	var rules []contextSubstRule
	for setIndex := 0; setIndex < int(chainSubClassSetCount); setIndex++ {
		setRel := binary.BigEndian.Uint16(d[12+setIndex*2 : 14+setIndex*2])
		if setRel == 0 {
			continue
		}
		setOffset := int(off) + int(setRel)
		if !hasBytesAt(g.Data, setOffset, 2) {
			continue
		}
		chainSubClassRuleCount := binary.BigEndian.Uint16(g.Data[setOffset : setOffset+2])
		if !hasBytesAt(g.Data, setOffset, 2+int(chainSubClassRuleCount)*2) {
			continue
		}
		for i := 0; i < int(chainSubClassRuleCount); i++ {
			ruleRel := binary.BigEndian.Uint16(g.Data[setOffset+2+i*2 : setOffset+4+i*2])
			rule, ok := g.parseChainSubClassRule(setOffset+int(ruleRel), backtrackClassDef, inputClassDef, lookaheadClassDef, coverageClassGlyphMatcher{
				coverage: coverage,
				classDef: inputClassDef,
				class:    uint16(setIndex),
			})
			if !ok {
				continue
			}
			rules = append(rules, rule)
		}
	}
	return rules, len(rules) > 0
}

func (g *GSUB) parseChainingContextSubstFormat3(off uint16) ([]contextSubstRule, bool) {
	if !hasTableBytes(g.Data, off, 4) {
		return nil, false
	}

	curr := 2
	readCoverages := func() ([]glyphMatcher, bool) {
		if !hasBytesAt(g.Data, int(off)+curr, 2) {
			return nil, false
		}
		count := binary.BigEndian.Uint16(g.Data[int(off)+curr : int(off)+curr+2])
		curr += 2
		if !hasBytesAt(g.Data, int(off)+curr, int(count)*2) {
			return nil, false
		}

		coverages := make([]glyphMatcher, int(count))
		for i := 0; i < int(count); i++ {
			coverageRel := binary.BigEndian.Uint16(g.Data[int(off)+curr : int(off)+curr+2])
			curr += 2
			coverage, ok := g.parseCoverageAt(off, coverageRel)
			if !ok {
				return nil, false
			}
			coverages[i] = coverageGlyphMatcher{coverage: coverage}
		}
		return coverages, true
	}

	backtrack, ok := readCoverages()
	if !ok {
		return nil, false
	}
	input, ok := readCoverages()
	if !ok || len(input) == 0 {
		return nil, false
	}
	lookahead, ok := readCoverages()
	if !ok {
		return nil, false
	}

	if !hasBytesAt(g.Data, int(off)+curr, 2) {
		return nil, false
	}
	substCount := binary.BigEndian.Uint16(g.Data[int(off)+curr : int(off)+curr+2])
	curr += 2
	records, ok := readSequenceLookupRecords(g.Data, int(off)+curr, substCount)
	if !ok {
		return nil, false
	}

	return []contextSubstRule{{
		backtrack:    backtrack,
		input:        input,
		lookahead:    lookahead,
		substRecords: records,
	}}, true
}

func (g *GSUB) parseContextSubRule(ruleOffset int, first glyphMatcher) (contextSubstRule, bool) {
	if !hasBytesAt(g.Data, ruleOffset, 4) {
		return contextSubstRule{}, false
	}
	glyphCount := binary.BigEndian.Uint16(g.Data[ruleOffset : ruleOffset+2])
	substCount := binary.BigEndian.Uint16(g.Data[ruleOffset+2 : ruleOffset+4])
	if glyphCount == 0 {
		return contextSubstRule{}, false
	}
	sequenceCount := int(glyphCount) - 1
	if !hasBytesAt(g.Data, ruleOffset, 4+sequenceCount*2+int(substCount)*4) {
		return contextSubstRule{}, false
	}

	input := make([]glyphMatcher, int(glyphCount))
	input[0] = first
	for i := 0; i < sequenceCount; i++ {
		input[i+1] = exactGlyphMatcher(binary.BigEndian.Uint16(g.Data[ruleOffset+4+i*2 : ruleOffset+6+i*2]))
	}
	records, ok := readSequenceLookupRecords(g.Data, ruleOffset+4+sequenceCount*2, substCount)
	if !ok {
		return contextSubstRule{}, false
	}
	return contextSubstRule{input: input, substRecords: records}, true
}

func (g *GSUB) parseContextSubClassRule(ruleOffset int, classDef ClassDefTable, first glyphMatcher) (contextSubstRule, bool) {
	if !hasBytesAt(g.Data, ruleOffset, 4) {
		return contextSubstRule{}, false
	}
	glyphCount := binary.BigEndian.Uint16(g.Data[ruleOffset : ruleOffset+2])
	substCount := binary.BigEndian.Uint16(g.Data[ruleOffset+2 : ruleOffset+4])
	if glyphCount == 0 {
		return contextSubstRule{}, false
	}
	sequenceCount := int(glyphCount) - 1
	if !hasBytesAt(g.Data, ruleOffset, 4+sequenceCount*2+int(substCount)*4) {
		return contextSubstRule{}, false
	}

	input := make([]glyphMatcher, int(glyphCount))
	input[0] = first
	for i := 0; i < sequenceCount; i++ {
		input[i+1] = classGlyphMatcher{
			classDef: classDef,
			class:    binary.BigEndian.Uint16(g.Data[ruleOffset+4+i*2 : ruleOffset+6+i*2]),
		}
	}
	records, ok := readSequenceLookupRecords(g.Data, ruleOffset+4+sequenceCount*2, substCount)
	if !ok {
		return contextSubstRule{}, false
	}
	return contextSubstRule{input: input, substRecords: records}, true
}

func (g *GSUB) parseChainSubRule(ruleOffset int, first glyphMatcher) (contextSubstRule, bool) {
	curr := ruleOffset
	backtrack, ok := g.readExactGlyphMatchers(&curr)
	if !ok {
		return contextSubstRule{}, false
	}
	input, ok := g.readChainInputExactGlyphMatchers(&curr, first)
	if !ok {
		return contextSubstRule{}, false
	}
	lookahead, ok := g.readExactGlyphMatchers(&curr)
	if !ok {
		return contextSubstRule{}, false
	}
	if !hasBytesAt(g.Data, curr, 2) {
		return contextSubstRule{}, false
	}
	substCount := binary.BigEndian.Uint16(g.Data[curr : curr+2])
	curr += 2
	records, ok := readSequenceLookupRecords(g.Data, curr, substCount)
	if !ok {
		return contextSubstRule{}, false
	}
	return contextSubstRule{backtrack: backtrack, input: input, lookahead: lookahead, substRecords: records}, true
}

func (g *GSUB) parseChainSubClassRule(ruleOffset int, backtrackClassDef, inputClassDef, lookaheadClassDef ClassDefTable, first glyphMatcher) (contextSubstRule, bool) {
	curr := ruleOffset
	backtrack, ok := g.readClassGlyphMatchers(&curr, backtrackClassDef)
	if !ok {
		return contextSubstRule{}, false
	}
	input, ok := g.readChainInputClassGlyphMatchers(&curr, inputClassDef, first)
	if !ok {
		return contextSubstRule{}, false
	}
	lookahead, ok := g.readClassGlyphMatchers(&curr, lookaheadClassDef)
	if !ok {
		return contextSubstRule{}, false
	}
	if !hasBytesAt(g.Data, curr, 2) {
		return contextSubstRule{}, false
	}
	substCount := binary.BigEndian.Uint16(g.Data[curr : curr+2])
	curr += 2
	records, ok := readSequenceLookupRecords(g.Data, curr, substCount)
	if !ok {
		return contextSubstRule{}, false
	}
	return contextSubstRule{backtrack: backtrack, input: input, lookahead: lookahead, substRecords: records}, true
}

func (g *GSUB) readExactGlyphMatchers(curr *int) ([]glyphMatcher, bool) {
	if !hasBytesAt(g.Data, *curr, 2) {
		return nil, false
	}
	count := binary.BigEndian.Uint16(g.Data[*curr : *curr+2])
	*curr += 2
	if !hasBytesAt(g.Data, *curr, int(count)*2) {
		return nil, false
	}
	matchers := make([]glyphMatcher, int(count))
	for i := 0; i < int(count); i++ {
		matchers[i] = exactGlyphMatcher(binary.BigEndian.Uint16(g.Data[*curr+i*2 : *curr+i*2+2]))
	}
	*curr += int(count) * 2
	return matchers, true
}

func (g *GSUB) readChainInputExactGlyphMatchers(curr *int, first glyphMatcher) ([]glyphMatcher, bool) {
	if !hasBytesAt(g.Data, *curr, 2) {
		return nil, false
	}
	count := binary.BigEndian.Uint16(g.Data[*curr : *curr+2])
	*curr += 2
	if count == 0 {
		return nil, false
	}
	suffixCount := int(count) - 1
	if !hasBytesAt(g.Data, *curr, suffixCount*2) {
		return nil, false
	}
	matchers := make([]glyphMatcher, int(count))
	matchers[0] = first
	for i := 0; i < suffixCount; i++ {
		matchers[i+1] = exactGlyphMatcher(binary.BigEndian.Uint16(g.Data[*curr+i*2 : *curr+i*2+2]))
	}
	*curr += suffixCount * 2
	return matchers, true
}

func (g *GSUB) readClassGlyphMatchers(curr *int, classDef ClassDefTable) ([]glyphMatcher, bool) {
	if !hasBytesAt(g.Data, *curr, 2) {
		return nil, false
	}
	count := binary.BigEndian.Uint16(g.Data[*curr : *curr+2])
	*curr += 2
	if !hasBytesAt(g.Data, *curr, int(count)*2) {
		return nil, false
	}
	matchers := make([]glyphMatcher, int(count))
	for i := 0; i < int(count); i++ {
		matchers[i] = classGlyphMatcher{
			classDef: classDef,
			class:    binary.BigEndian.Uint16(g.Data[*curr+i*2 : *curr+i*2+2]),
		}
	}
	*curr += int(count) * 2
	return matchers, true
}

func (g *GSUB) readChainInputClassGlyphMatchers(curr *int, classDef ClassDefTable, first glyphMatcher) ([]glyphMatcher, bool) {
	if !hasBytesAt(g.Data, *curr, 2) {
		return nil, false
	}
	count := binary.BigEndian.Uint16(g.Data[*curr : *curr+2])
	*curr += 2
	if count == 0 {
		return nil, false
	}
	suffixCount := int(count) - 1
	if !hasBytesAt(g.Data, *curr, suffixCount*2) {
		return nil, false
	}
	matchers := make([]glyphMatcher, int(count))
	matchers[0] = first
	for i := 0; i < suffixCount; i++ {
		matchers[i+1] = classGlyphMatcher{
			classDef: classDef,
			class:    binary.BigEndian.Uint16(g.Data[*curr+i*2 : *curr+i*2+2]),
		}
	}
	*curr += suffixCount * 2
	return matchers, true
}

func (g *GSUB) parseCoverageAt(base uint16, rel uint16) (CoverageTable, bool) {
	offset, ok := resolveTableOffset(base, rel)
	if !ok {
		return nil, false
	}
	coverage, err := ParseCoverage(g.Data, uint16(offset))
	return coverage, err == nil
}

func (g *GSUB) parseClassDefAt(base uint16, rel uint16) (ClassDefTable, bool) {
	offset, ok := resolveTableOffset(base, rel)
	if !ok {
		return nil, false
	}
	classDef, err := ParseClassDef(g.Data, uint16(offset))
	return classDef, err == nil
}

func readSequenceLookupRecords(data []byte, offset int, count uint16) ([]chainSubstRecord, bool) {
	if !hasBytesAt(data, offset, int(count)*4) {
		return nil, false
	}
	records := make([]chainSubstRecord, int(count))
	for i := 0; i < int(count); i++ {
		recordOffset := offset + i*4
		records[i] = chainSubstRecord{
			index:       binary.BigEndian.Uint16(data[recordOffset : recordOffset+2]),
			lookupIndex: binary.BigEndian.Uint16(data[recordOffset+2 : recordOffset+4]),
		}
	}
	return records, true
}

func (g *GSUB) applyContextLookupAt(lookup *LookupTable, glyphs []int, index int, limit int) ([]int, bool) {
	if lookup == nil || index < 0 || index >= len(glyphs) {
		return glyphs, false
	}
	if lookupIgnoresGlyph(g.GDEF, lookup, glyphs[index]) {
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
	case 2:
		replacement, ok := g.firstMultipleSubstitution(lookup, glyphs[index])
		if !ok {
			return glyphs, false
		}
		next := make([]int, 0, len(glyphs)-1+len(replacement))
		next = append(next, glyphs[:index]...)
		next = append(next, replacement...)
		next = append(next, glyphs[index+1:]...)
		return next, true
	case 3:
		replacement, ok := g.firstAlternateSubstitution(lookup, glyphs[index])
		if !ok || replacement == glyphs[index] {
			return glyphs, false
		}
		glyphs[index] = replacement
		return glyphs, true
	case 4:
		match, ok := g.firstLigatureSubstitution(lookup, glyphs, index, limit)
		if !ok {
			return glyphs, false
		}
		next := make([]int, 0, len(glyphs)-len(match.componentIndexes)+1)
		next = append(next, glyphs[:index]...)
		next = append(next, match.replacement...)
		next = appendSkippedLigatureGlyphs(next, glyphs, match.componentIndexes)
		next = append(next, glyphs[match.componentIndexes[len(match.componentIndexes)-1]+1:]...)
		return next, true
	case 7:
		out := glyphs
		changed := false
		for _, off := range lookup.SubtableOffsets {
			extensionLookup, ok := parseExtensionLookup(g.Data, off, lookup.Flag, lookup.MarkFilteringSet, 7)
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

func (g *GSUB) applyMultipleSubst(lookup *LookupTable, glyphs []int) []int {
	out := make([]int, 0, len(glyphs))
	for _, gid := range glyphs {
		if lookupIgnoresGlyph(g.GDEF, lookup, gid) {
			out = append(out, gid)
			continue
		}
		replacement, ok := g.firstMultipleSubstitution(lookup, gid)
		if ok {
			out = append(out, replacement...)
			continue
		}
		out = append(out, gid)
	}
	return out
}

func (g *GSUB) firstMultipleSubstitution(lookup *LookupTable, glyphID int) ([]int, bool) {
	for _, off := range lookup.SubtableOffsets {
		if !hasTableBytes(g.Data, off, 6) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		if format != 1 {
			continue
		}
		coverage, ok := g.parseCoverageAt(off, binary.BigEndian.Uint16(d[2:4]))
		if !ok {
			continue
		}
		sequenceCount := binary.BigEndian.Uint16(d[4:6])
		if !hasTableBytes(g.Data, off, 6+int(sequenceCount)*2) {
			continue
		}
		coverageIndex := coverage.GetIndex(glyphID)
		if coverageIndex < 0 || coverageIndex >= int(sequenceCount) {
			continue
		}
		sequenceRel := binary.BigEndian.Uint16(d[6+coverageIndex*2 : 8+coverageIndex*2])
		sequenceOffset, ok := resolveTableOffset(off, sequenceRel)
		if !ok || !hasBytesAt(g.Data, sequenceOffset, 2) {
			continue
		}
		glyphCount := binary.BigEndian.Uint16(g.Data[sequenceOffset : sequenceOffset+2])
		if !hasBytesAt(g.Data, sequenceOffset, 2+int(glyphCount)*2) {
			continue
		}
		replacement := make([]int, int(glyphCount))
		for i := 0; i < int(glyphCount); i++ {
			replacement[i] = int(binary.BigEndian.Uint16(g.Data[sequenceOffset+2+i*2 : sequenceOffset+4+i*2]))
		}
		return replacement, true
	}
	return nil, false
}

func (g *GSUB) applyAlternateSubst(lookup *LookupTable, glyphs []int) []int {
	out := make([]int, len(glyphs))
	copy(out, glyphs)
	for i, gid := range out {
		if lookupIgnoresGlyph(g.GDEF, lookup, gid) {
			continue
		}
		if replacement, ok := g.firstAlternateSubstitution(lookup, gid); ok {
			out[i] = replacement
		}
	}
	return out
}

func (g *GSUB) firstAlternateSubstitution(lookup *LookupTable, glyphID int) (int, bool) {
	for _, off := range lookup.SubtableOffsets {
		if !hasTableBytes(g.Data, off, 6) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		if format != 1 {
			continue
		}
		coverage, ok := g.parseCoverageAt(off, binary.BigEndian.Uint16(d[2:4]))
		if !ok {
			continue
		}
		alternateSetCount := binary.BigEndian.Uint16(d[4:6])
		if !hasTableBytes(g.Data, off, 6+int(alternateSetCount)*2) {
			continue
		}
		coverageIndex := coverage.GetIndex(glyphID)
		if coverageIndex < 0 || coverageIndex >= int(alternateSetCount) {
			continue
		}
		alternateSetRel := binary.BigEndian.Uint16(d[6+coverageIndex*2 : 8+coverageIndex*2])
		alternateSetOffset, ok := resolveTableOffset(off, alternateSetRel)
		if !ok || !hasBytesAt(g.Data, alternateSetOffset, 4) {
			continue
		}
		glyphCount := binary.BigEndian.Uint16(g.Data[alternateSetOffset : alternateSetOffset+2])
		if glyphCount == 0 || !hasBytesAt(g.Data, alternateSetOffset, 2+int(glyphCount)*2) {
			continue
		}
		return int(binary.BigEndian.Uint16(g.Data[alternateSetOffset+2 : alternateSetOffset+4])), true
	}
	return 0, false
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
				if lookupIgnoresGlyph(g.GDEF, lookup, gid) {
					continue
				}
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
				if lookupIgnoresGlyph(g.GDEF, lookup, gid) {
					continue
				}
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
		match, ok := g.firstLigatureSubstitution(lookup, glyphs, i, len(glyphs))
		if ok {
			newGlyphs = append(newGlyphs, match.replacement...)
			newGlyphs = appendSkippedLigatureGlyphs(newGlyphs, glyphs, match.componentIndexes)
			i = match.componentIndexes[len(match.componentIndexes)-1] + 1
			continue
		}
		newGlyphs = append(newGlyphs, glyphs[i])
		i++
	}
	return newGlyphs
}

type ligatureSubstitutionMatch struct {
	replacement      []int
	componentIndexes []int
}

func appendSkippedLigatureGlyphs(out []int, glyphs []int, componentIndexes []int) []int {
	if len(componentIndexes) == 0 {
		return out
	}
	componentSet := make(map[int]bool, len(componentIndexes))
	for _, index := range componentIndexes {
		componentSet[index] = true
	}
	for i := componentIndexes[0] + 1; i < componentIndexes[len(componentIndexes)-1]; i++ {
		if !componentSet[i] {
			out = append(out, glyphs[i])
		}
	}
	return out
}

func (g *GSUB) firstLigatureSubstitution(lookup *LookupTable, glyphs []int, start int, limit int) (ligatureSubstitutionMatch, bool) {
	if lookup == nil || start < 0 || start >= len(glyphs) || lookupIgnoresGlyph(g.GDEF, lookup, glyphs[start]) {
		return ligatureSubstitutionMatch{}, false
	}
	if limit > len(glyphs) {
		limit = len(glyphs)
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
		covIdx := coverage.GetIndex(glyphs[start])
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
			if compCount == 0 {
				continue
			}
			if !hasBytesAt(g.Data, ligOffset, 4+(int(compCount)-1)*2) {
				continue
			}

			match := true
			componentIndexes := make([]int, int(compCount))
			componentIndexes[0] = start
			componentIndex := start
			for j := 1; j < int(compCount); j++ {
				componentGlyph := binary.BigEndian.Uint16(ligData[4+(j-1)*2 : 6+(j-1)*2])
				componentIndex = nextLookupGlyphIndex(g.GDEF, lookup, glyphs, componentIndex+1)
				if componentIndex < 0 || componentIndex >= limit || glyphs[componentIndex] != int(componentGlyph) {
					match = false
					break
				}
				componentIndexes[j] = componentIndex
			}
			if match {
				return ligatureSubstitutionMatch{
					replacement:      []int{int(ligGlyph)},
					componentIndexes: componentIndexes,
				}, true
			}
		}
	}
	return ligatureSubstitutionMatch{}, false
}

func (g *GSUB) applyReverseChainingContextSingleSubst(lookup *LookupTable, glyphs []int) []int {
	out := make([]int, len(glyphs))
	copy(out, glyphs)

	for _, off := range lookup.SubtableOffsets {
		if !hasTableBytes(g.Data, off, 6) {
			continue
		}
		d := g.Data[off:]
		format := binary.BigEndian.Uint16(d[0:2])
		if format != 1 {
			continue
		}
		coverage, ok := g.parseCoverageAt(off, binary.BigEndian.Uint16(d[2:4]))
		if !ok {
			continue
		}

		curr := int(off) + 4
		backtrack, ok := g.readCoverageMatchers(off, &curr)
		if !ok {
			continue
		}
		lookahead, ok := g.readCoverageMatchers(off, &curr)
		if !ok || !hasBytesAt(g.Data, curr, 2) {
			continue
		}
		substituteCount := binary.BigEndian.Uint16(g.Data[curr : curr+2])
		curr += 2
		if !hasBytesAt(g.Data, curr, int(substituteCount)*2) {
			continue
		}

		substitutes := make([]uint16, int(substituteCount))
		for i := 0; i < int(substituteCount); i++ {
			substitutes[i] = binary.BigEndian.Uint16(g.Data[curr+i*2 : curr+i*2+2])
		}

		for i := len(out) - 1; i >= 0; i-- {
			if lookupIgnoresGlyph(g.GDEF, lookup, out[i]) {
				continue
			}
			coverageIndex := coverage.GetIndex(out[i])
			if coverageIndex < 0 || coverageIndex >= len(substitutes) {
				continue
			}
			rule := contextSubstRule{
				backtrack: backtrack,
				input:     []glyphMatcher{coverageIndexGlyphMatcher{coverage: coverage, index: coverageIndex}},
				lookahead: lookahead,
			}
			if _, ok := rule.matchIndexes(g.GDEF, lookup, out, i); ok {
				out[i] = int(substitutes[coverageIndex])
			}
		}
	}

	return out
}

func (g *GSUB) readCoverageMatchers(base uint16, curr *int) ([]glyphMatcher, bool) {
	if !hasBytesAt(g.Data, *curr, 2) {
		return nil, false
	}
	count := binary.BigEndian.Uint16(g.Data[*curr : *curr+2])
	*curr += 2
	if !hasBytesAt(g.Data, *curr, int(count)*2) {
		return nil, false
	}
	matchers := make([]glyphMatcher, int(count))
	for i := 0; i < int(count); i++ {
		coverageRel := binary.BigEndian.Uint16(g.Data[*curr+i*2 : *curr+i*2+2])
		coverage, ok := g.parseCoverageAt(base, coverageRel)
		if !ok {
			return nil, false
		}
		matchers[i] = coverageGlyphMatcher{coverage: coverage}
	}
	*curr += int(count) * 2
	return matchers, true
}

func (g *GSUB) applyExtensionSubst(lookup *LookupTable, glyphs []int) []int {
	out := glyphs
	for _, off := range lookup.SubtableOffsets {
		extensionLookup, ok := parseExtensionLookup(g.Data, off, lookup.Flag, lookup.MarkFilteringSet, 7)
		if !ok {
			continue
		}
		out = g.applyLookup(extensionLookup, out)
	}
	return out
}
