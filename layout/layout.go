package layout

import (
	"encoding/binary"
	"fmt"
)

func hasBytesAt(data []byte, offset int, size int) bool {
	if offset < 0 || size < 0 || offset > len(data) {
		return false
	}
	return size <= len(data)-offset
}

func hasTableBytes(data []byte, offset uint16, size int) bool {
	return hasBytesAt(data, int(offset), size)
}

func resolveTableOffset(base, rel uint16) (int, bool) {
	offset := int(base) + int(rel)
	if offset > 0xffff {
		return 0, false
	}
	return offset, true
}

func resolveTableOffset32(base uint16, rel uint32) (int, bool) {
	offset := uint64(base) + uint64(rel)
	maxInt := uint64(^uint(0) >> 1)
	if offset > maxInt {
		return 0, false
	}
	return int(offset), true
}

const (
	tagDefaultScriptUpper uint32 = 0x44464c54 // "DFLT"
	tagDefaultScriptLower uint32 = 0x64666c74 // "dflt"
	noRequiredFeature     uint16 = 0xffff
)

// ScriptList represents the ScriptList table in GSUB/GPOS.
type ScriptList struct {
	Scripts []ScriptRecord
}

type ScriptRecord struct {
	Tag    uint32
	Offset uint16
}

func (sl *ScriptList) selectFeatureIndices(data []byte, scriptTag, languageTag uint32, featureCount int) ([]int, bool) {
	if sl == nil || len(sl.Scripts) == 0 || featureCount <= 0 {
		return nil, false
	}

	script, ok := sl.selectScript(scriptTag)
	if !ok {
		return nil, false
	}

	langSysOffset, ok := selectLangSysOffset(data, script.Offset, languageTag)
	if !ok {
		return nil, false
	}

	return parseLangSysFeatureIndices(data, langSysOffset, featureCount)
}

func (sl *ScriptList) selectScript(scriptTag uint32) (ScriptRecord, bool) {
	if scriptTag != 0 {
		for _, script := range sl.Scripts {
			if script.Tag == scriptTag {
				return script, true
			}
		}
	}

	for _, fallbackTag := range []uint32{tagDefaultScriptUpper, tagDefaultScriptLower} {
		for _, script := range sl.Scripts {
			if script.Tag == fallbackTag {
				return script, true
			}
		}
	}

	if scriptTag == 0 {
		return sl.Scripts[0], true
	}
	return ScriptRecord{}, false
}

func selectLangSysOffset(data []byte, scriptOffset uint16, languageTag uint32) (uint16, bool) {
	if !hasTableBytes(data, scriptOffset, 4) {
		return 0, false
	}
	d := data[scriptOffset:]
	defaultLangSysOffset := binary.BigEndian.Uint16(d[0:2])
	langSysCount := binary.BigEndian.Uint16(d[2:4])
	if !hasTableBytes(data, scriptOffset, 4+int(langSysCount)*6) {
		return 0, false
	}

	var firstLangSysRel uint16
	for i := 0; i < int(langSysCount); i++ {
		rec := 4 + i*6
		tag := binary.BigEndian.Uint32(d[rec : rec+4])
		rel := binary.BigEndian.Uint16(d[rec+4 : rec+6])
		if i == 0 {
			firstLangSysRel = rel
		}
		if languageTag != 0 && tag == languageTag {
			return resolveLangSysOffset(scriptOffset, rel)
		}
	}

	if defaultLangSysOffset != 0 {
		return resolveLangSysOffset(scriptOffset, defaultLangSysOffset)
	}
	if languageTag == 0 && langSysCount > 0 {
		return resolveLangSysOffset(scriptOffset, firstLangSysRel)
	}
	return 0, false
}

func resolveLangSysOffset(scriptOffset, rel uint16) (uint16, bool) {
	offset, ok := resolveTableOffset(scriptOffset, rel)
	if !ok {
		return 0, false
	}
	return uint16(offset), true
}

func parseLangSysFeatureIndices(data []byte, langSysOffset uint16, featureCount int) ([]int, bool) {
	if !hasTableBytes(data, langSysOffset, 6) {
		return nil, false
	}
	d := data[langSysOffset:]
	requiredFeatureIndex := binary.BigEndian.Uint16(d[2:4])
	featureIndexCount := binary.BigEndian.Uint16(d[4:6])
	if !hasTableBytes(data, langSysOffset, 6+int(featureIndexCount)*2) {
		return nil, false
	}

	seen := make(map[int]bool)
	var indices []int
	add := func(index uint16) {
		i := int(index)
		if i >= 0 && i < featureCount && !seen[i] {
			indices = append(indices, i)
			seen[i] = true
		}
	}

	if requiredFeatureIndex != noRequiredFeature {
		add(requiredFeatureIndex)
	}
	for i := 0; i < int(featureIndexCount); i++ {
		add(binary.BigEndian.Uint16(d[6+i*2 : 8+i*2]))
	}

	return indices, true
}

// FeatureList represents the FeatureList table in GSUB/GPOS.
type FeatureList struct {
	Features []FeatureRecord
}

type FeatureRecord struct {
	Tag    uint32
	Offset uint16
}

// LookupList represents the LookupList table in GSUB/GPOS.
type LookupList struct {
	Lookups []*LookupTable
}

type LookupTable struct {
	Type             uint16
	Flag             uint16
	SubtableOffsets  []uint16 // Absolute offsets from table start
	MarkFilteringSet uint16
}

func parseExtensionLookup(data []byte, offset uint16, parentFlag uint16, parentMarkFilteringSet uint16, disallowedType uint16) (*LookupTable, bool) {
	if !hasTableBytes(data, offset, 8) {
		return nil, false
	}
	d := data[offset:]
	format := binary.BigEndian.Uint16(d[0:2])
	if format != 1 {
		return nil, false
	}

	lookupType := binary.BigEndian.Uint16(d[2:4])
	if lookupType == 0 || lookupType == disallowedType {
		return nil, false
	}

	extensionOffset := binary.BigEndian.Uint32(d[4:8])
	subtableOffset, ok := resolveTableOffset32(offset, extensionOffset)
	if !ok || subtableOffset > 0xffff || !hasBytesAt(data, subtableOffset, 2) {
		return nil, false
	}

	return &LookupTable{
		Type:             lookupType,
		Flag:             parentFlag,
		SubtableOffsets:  []uint16{uint16(subtableOffset)},
		MarkFilteringSet: parentMarkFilteringSet,
	}, true
}

type CoverageTable interface {
	GetIndex(glyphID int) int
}

type CoverageFormat1 struct {
	Glyphs []uint16
}

func (c *CoverageFormat1) GetIndex(glyphID int) int {
	for i, g := range c.Glyphs {
		if int(g) == glyphID {
			return i
		}
	}
	return -1
}

type CoverageFormat2 struct {
	Ranges []RangeRecord
}

type RangeRecord struct {
	Start              uint16
	End                uint16
	StartCoverageIndex uint16
}

func (c *CoverageFormat2) GetIndex(glyphID int) int {
	for _, r := range c.Ranges {
		if glyphID >= int(r.Start) && glyphID <= int(r.End) {
			return int(r.StartCoverageIndex) + (glyphID - int(r.Start))
		}
	}
	return -1
}

type ClassDefTable interface {
	GetClass(glyphID int) int
}

type ClassDefFormat1 struct {
	StartGlyph uint16
	Classes    []uint16
}

func (c *ClassDefFormat1) GetClass(glyphID int) int {
	index := glyphID - int(c.StartGlyph)
	if index < 0 || index >= len(c.Classes) {
		return 0
	}
	return int(c.Classes[index])
}

type ClassDefFormat2 struct {
	Ranges []ClassRangeRecord
}

type ClassRangeRecord struct {
	Start uint16
	End   uint16
	Class uint16
}

func (c *ClassDefFormat2) GetClass(glyphID int) int {
	for _, r := range c.Ranges {
		if glyphID >= int(r.Start) && glyphID <= int(r.End) {
			return int(r.Class)
		}
	}
	return 0
}

func ParseScriptList(data []byte, offset uint16) (*ScriptList, error) {
	if !hasTableBytes(data, offset, 2) {
		return nil, fmt.Errorf("script list offset out of bounds")
	}
	d := data[offset:]
	count := binary.BigEndian.Uint16(d[0:2])
	if !hasTableBytes(data, offset, 2+int(count)*6) {
		return nil, fmt.Errorf("script list too short")
	}
	scripts := make([]ScriptRecord, int(count))
	for i := 0; i < int(count); i++ {
		scripts[i].Tag = binary.BigEndian.Uint32(d[2+i*6 : 6+i*6])
		scripts[i].Offset = uint16(int(offset) + int(binary.BigEndian.Uint16(d[6+i*6:8+i*6])))
	}
	return &ScriptList{Scripts: scripts}, nil
}

func ParseFeatureList(data []byte, offset uint16) (*FeatureList, error) {
	if !hasTableBytes(data, offset, 2) {
		return nil, fmt.Errorf("feature list offset out of bounds")
	}
	d := data[offset:]
	count := binary.BigEndian.Uint16(d[0:2])
	if !hasTableBytes(data, offset, 2+int(count)*6) {
		return nil, fmt.Errorf("feature list too short")
	}
	features := make([]FeatureRecord, int(count))
	for i := 0; i < int(count); i++ {
		features[i].Tag = binary.BigEndian.Uint32(d[2+i*6 : 6+i*6])
		features[i].Offset = uint16(int(offset) + int(binary.BigEndian.Uint16(d[6+i*6:8+i*6])))
	}
	return &FeatureList{Features: features}, nil
}

func defaultFeatureIndices(scriptList *ScriptList, featureList *FeatureList, data []byte) []int {
	if featureList == nil || len(featureList.Features) == 0 {
		return nil
	}
	if indices, ok := scriptList.selectFeatureIndices(data, 0, 0, len(featureList.Features)); ok {
		return indices
	}

	indices := make([]int, len(featureList.Features))
	for i := range indices {
		indices[i] = i
	}
	return indices
}

func (fl *FeatureList) GetFeatureLookups(data []byte, index int) []uint16 {
	if index < 0 || index >= len(fl.Features) {
		return nil
	}
	off := fl.Features[index].Offset
	if !hasTableBytes(data, off, 4) {
		return nil
	}
	d := data[off:]
	// uint16 featureParamsOffset (reserved)
	count := binary.BigEndian.Uint16(d[2:4])
	if !hasTableBytes(data, off, 4+int(count)*2) {
		return nil
	}
	indices := make([]uint16, int(count))
	for i := 0; i < int(count); i++ {
		indices[i] = binary.BigEndian.Uint16(d[4+i*2 : 6+i*2])
	}
	return indices
}

func ParseLookupList(data []byte, offset uint16) (*LookupList, error) {
	if !hasTableBytes(data, offset, 2) {
		return nil, fmt.Errorf("lookup list offset out of bounds")
	}
	d := data[offset:]
	count := binary.BigEndian.Uint16(d[0:2])
	if !hasTableBytes(data, offset, 2+int(count)*2) {
		return nil, fmt.Errorf("lookup list too short")
	}
	lookups := make([]*LookupTable, int(count))
	for i := 0; i < int(count); i++ {
		off := binary.BigEndian.Uint16(d[2+i*2 : 4+i*2])
		lookupOffset, ok := resolveTableOffset(offset, off)
		if !ok {
			return nil, fmt.Errorf("lookup table offset overflow")
		}
		lookup, err := ParseLookupTable(data, uint16(lookupOffset))
		if err != nil {
			return nil, err
		}
		lookups[i] = lookup
	}
	return &LookupList{Lookups: lookups}, nil
}

func ParseLookupTable(data []byte, offset uint16) (*LookupTable, error) {
	if !hasTableBytes(data, offset, 6) {
		return nil, fmt.Errorf("lookup table too short")
	}
	d := data[offset:]
	lt := &LookupTable{
		Type: binary.BigEndian.Uint16(d[0:2]),
		Flag: binary.BigEndian.Uint16(d[2:4]),
	}
	count := binary.BigEndian.Uint16(d[4:6])
	if !hasTableBytes(data, offset, 6+int(count)*2) {
		return nil, fmt.Errorf("lookup table too short for subtables")
	}
	lt.SubtableOffsets = make([]uint16, int(count))
	for i := 0; i < int(count); i++ {
		subtableOffset := binary.BigEndian.Uint16(d[6+i*2 : 8+i*2])
		resolvedOffset, ok := resolveTableOffset(offset, subtableOffset)
		if !ok {
			return nil, fmt.Errorf("lookup subtable offset overflow")
		}
		lt.SubtableOffsets[i] = uint16(resolvedOffset)
	}
	if lt.Flag&lookupFlagUseMarkFilteringSet != 0 {
		markFilteringSetOffset := 6 + int(count)*2
		if !hasTableBytes(data, offset, markFilteringSetOffset+2) {
			return nil, fmt.Errorf("lookup table too short for mark filtering set")
		}
		lt.MarkFilteringSet = binary.BigEndian.Uint16(d[markFilteringSetOffset : markFilteringSetOffset+2])
	}
	return lt, nil
}

func ParseCoverage(data []byte, offset uint16) (CoverageTable, error) {
	return parseCoverage(data, int(offset))
}

func parseCoverage(data []byte, offset int) (CoverageTable, error) {
	if !hasBytesAt(data, offset, 2) {
		return nil, fmt.Errorf("coverage table too short")
	}
	d := data[offset:]
	format := binary.BigEndian.Uint16(d[0:2])
	if format == 1 {
		if !hasBytesAt(data, offset, 4) {
			return nil, fmt.Errorf("coverage table too short")
		}
		count := binary.BigEndian.Uint16(d[2:4])
		if !hasBytesAt(data, offset, 4+int(count)*2) {
			return nil, fmt.Errorf("coverage format 1 too short")
		}
		glyphs := make([]uint16, int(count))
		for i := 0; i < int(count); i++ {
			glyphs[i] = binary.BigEndian.Uint16(d[4+i*2 : 6+i*2])
		}
		return &CoverageFormat1{Glyphs: glyphs}, nil
	} else if format == 2 {
		if !hasBytesAt(data, offset, 4) {
			return nil, fmt.Errorf("coverage table too short")
		}
		count := binary.BigEndian.Uint16(d[2:4])
		if !hasBytesAt(data, offset, 4+int(count)*6) {
			return nil, fmt.Errorf("coverage format 2 too short")
		}
		ranges := make([]RangeRecord, int(count))
		for i := 0; i < int(count); i++ {
			ranges[i].Start = binary.BigEndian.Uint16(d[4+i*6 : 6+i*6])
			ranges[i].End = binary.BigEndian.Uint16(d[6+i*6 : 8+i*6])
			ranges[i].StartCoverageIndex = binary.BigEndian.Uint16(d[8+i*6 : 10+i*6])
		}
		return &CoverageFormat2{Ranges: ranges}, nil
	}
	return nil, fmt.Errorf("unsupported coverage format: %d", format)
}

func ParseClassDef(data []byte, offset uint16) (ClassDefTable, error) {
	if !hasTableBytes(data, offset, 2) {
		return nil, fmt.Errorf("class definition table too short")
	}
	d := data[offset:]
	format := binary.BigEndian.Uint16(d[0:2])

	switch format {
	case 1:
		if !hasTableBytes(data, offset, 6) {
			return nil, fmt.Errorf("class definition format 1 too short")
		}
		startGlyph := binary.BigEndian.Uint16(d[2:4])
		glyphCount := binary.BigEndian.Uint16(d[4:6])
		if !hasTableBytes(data, offset, 6+int(glyphCount)*2) {
			return nil, fmt.Errorf("class definition format 1 class array too short")
		}
		classes := make([]uint16, int(glyphCount))
		for i := 0; i < int(glyphCount); i++ {
			classes[i] = binary.BigEndian.Uint16(d[6+i*2 : 8+i*2])
		}
		return &ClassDefFormat1{StartGlyph: startGlyph, Classes: classes}, nil

	case 2:
		if !hasTableBytes(data, offset, 4) {
			return nil, fmt.Errorf("class definition format 2 too short")
		}
		classRangeCount := binary.BigEndian.Uint16(d[2:4])
		if !hasTableBytes(data, offset, 4+int(classRangeCount)*6) {
			return nil, fmt.Errorf("class definition format 2 ranges too short")
		}
		ranges := make([]ClassRangeRecord, int(classRangeCount))
		for i := 0; i < int(classRangeCount); i++ {
			ranges[i].Start = binary.BigEndian.Uint16(d[4+i*6 : 6+i*6])
			ranges[i].End = binary.BigEndian.Uint16(d[6+i*6 : 8+i*6])
			ranges[i].Class = binary.BigEndian.Uint16(d[8+i*6 : 10+i*6])
		}
		return &ClassDefFormat2{Ranges: ranges}, nil
	}

	return nil, fmt.Errorf("unsupported class definition format: %d", format)
}

// GPOS ValueRecord
type ValueRecord struct {
	XPlacement int16
	YPlacement int16
	XAdvance   int16
	YAdvance   int16
	// Ignoring device tables for now
}

func ParseValueRecord(data []byte, format uint16) (ValueRecord, int) {
	var vr ValueRecord
	offset := 0
	if format&0x0001 != 0 {
		if offset+2 > len(data) {
			return vr, ValueRecordSize(format)
		}
		vr.XPlacement = int16(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
	}
	if format&0x0002 != 0 {
		if offset+2 > len(data) {
			return vr, ValueRecordSize(format)
		}
		vr.YPlacement = int16(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
	}
	if format&0x0004 != 0 {
		if offset+2 > len(data) {
			return vr, ValueRecordSize(format)
		}
		vr.XAdvance = int16(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
	}
	if format&0x0008 != 0 {
		if offset+2 > len(data) {
			return vr, ValueRecordSize(format)
		}
		vr.YAdvance = int16(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
	}
	// Skip device tables bits (0x0010, 0x0020, 0x0040, 0x0080)
	if format&0x0010 != 0 {
		offset += 2
	}
	if format&0x0020 != 0 {
		offset += 2
	}
	if format&0x0040 != 0 {
		offset += 2
	}
	if format&0x0080 != 0 {
		offset += 2
	}
	return vr, offset
}

func ValueRecordSize(format uint16) int {
	size := 0
	for bit := uint16(0x0001); bit <= 0x0080; bit <<= 1 {
		if format&bit != 0 {
			size += 2
		}
	}
	return size
}
