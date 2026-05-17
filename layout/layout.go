package layout

import (
	"encoding/binary"
	"fmt"
)

// ScriptList represents the ScriptList table in GSUB/GPOS.
type ScriptList struct {
	Scripts []ScriptRecord
}

type ScriptRecord struct {
	Tag    uint32
	Offset uint16
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
	Type            uint16
	Flag            uint16
	SubtableOffsets []uint16 // Absolute offsets from table start
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

func ParseScriptList(data []byte, offset uint16) (*ScriptList, error) {
	if int(offset)+2 > len(data) {
		return nil, fmt.Errorf("script list offset out of bounds")
	}
	d := data[offset:]
	count := binary.BigEndian.Uint16(d[0:2])
	scripts := make([]ScriptRecord, count)
	for i := 0; i < int(count); i++ {
		scripts[i].Tag = binary.BigEndian.Uint32(d[2+i*6 : 6+i*6])
		scripts[i].Offset = uint16(int(offset) + int(binary.BigEndian.Uint16(d[6+i*6:8+i*6])))
	}
	return &ScriptList{Scripts: scripts}, nil
}

func ParseFeatureList(data []byte, offset uint16) (*FeatureList, error) {
	if int(offset)+2 > len(data) {
		return nil, fmt.Errorf("feature list offset out of bounds")
	}
	d := data[offset:]
	count := binary.BigEndian.Uint16(d[0:2])
	features := make([]FeatureRecord, count)
	for i := 0; i < int(count); i++ {
		features[i].Tag = binary.BigEndian.Uint32(d[2+i*6 : 6+i*6])
		features[i].Offset = uint16(int(offset) + int(binary.BigEndian.Uint16(d[6+i*6:8+i*6])))
	}
	return &FeatureList{Features: features}, nil
}

func (fl *FeatureList) GetFeatureLookups(data []byte, index int) []uint16 {
	if index < 0 || index >= len(fl.Features) {
		return nil
	}
	off := fl.Features[index].Offset
	if int(off)+4 > len(data) {
		return nil
	}
	d := data[off:]
	// uint16 featureParamsOffset (reserved)
	count := binary.BigEndian.Uint16(d[2:4])
	if int(off)+4+int(count)*2 > len(data) {
		return nil
	}
	indices := make([]uint16, count)
	for i := 0; i < int(count); i++ {
		indices[i] = binary.BigEndian.Uint16(d[4+i*2 : 6+i*2])
	}
	return indices
}

func ParseLookupList(data []byte, offset uint16) (*LookupList, error) {
	if int(offset)+2 > len(data) {
		return nil, fmt.Errorf("lookup list offset out of bounds")
	}
	d := data[offset:]
	count := binary.BigEndian.Uint16(d[0:2])
	lookups := make([]*LookupTable, count)
	for i := 0; i < int(count); i++ {
		off := binary.BigEndian.Uint16(d[2+i*2 : 4+i*2])
		lookup, err := ParseLookupTable(data, uint16(int(offset)+int(off)))
		if err != nil {
			return nil, err
		}
		lookups[i] = lookup
	}
	return &LookupList{Lookups: lookups}, nil
}

func ParseLookupTable(data []byte, offset uint16) (*LookupTable, error) {
	if int(offset)+6 > len(data) {
		return nil, fmt.Errorf("lookup table too short")
	}
	d := data[offset:]
	lt := &LookupTable{
		Type: binary.BigEndian.Uint16(d[0:2]),
		Flag: binary.BigEndian.Uint16(d[2:4]),
	}
	count := binary.BigEndian.Uint16(d[4:6])
	lt.SubtableOffsets = make([]uint16, count)
	for i := 0; i < int(count); i++ {
		lt.SubtableOffsets[i] = uint16(int(offset) + int(binary.BigEndian.Uint16(d[6+i*2:8+i*2])))
	}
	return lt, nil
}

func ParseCoverage(data []byte, offset uint16) (CoverageTable, error) {
	if int(offset)+2 > len(data) {
		return nil, fmt.Errorf("coverage table too short")
	}
	d := data[offset:]
	format := binary.BigEndian.Uint16(d[0:2])
	if format == 1 {
		count := binary.BigEndian.Uint16(d[2:4])
		glyphs := make([]uint16, count)
		for i := 0; i < int(count); i++ {
			glyphs[i] = binary.BigEndian.Uint16(d[4+i*2 : 6+i*2])
		}
		return &CoverageFormat1{Glyphs: glyphs}, nil
	} else if format == 2 {
		count := binary.BigEndian.Uint16(d[2:4])
		ranges := make([]RangeRecord, count)
		for i := 0; i < int(count); i++ {
			ranges[i].Start = binary.BigEndian.Uint16(d[4+i*6 : 6+i*6])
			ranges[i].End = binary.BigEndian.Uint16(d[6+i*6 : 8+i*6])
			ranges[i].StartCoverageIndex = binary.BigEndian.Uint16(d[8+i*6 : 10+i*6])
		}
		return &CoverageFormat2{Ranges: ranges}, nil
	}
	return nil, fmt.Errorf("unsupported coverage format: %d", format)
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
