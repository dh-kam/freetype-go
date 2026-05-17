package sfnt

import (
	"errors"
	"fmt"
	"github.com/dh-kam/freetype-go/api"
)

// HeadTable represents the 'head' font header table.
type HeadTable struct {
	MajorVersion       uint16
	MinorVersion       uint16
	FontRevision       int32
	CheckSumAdjustment uint32
	MagicNumber        uint32
	Flags              uint16
	UnitsPerEm         uint16
	Created            int64
	Modified           int64
	XMin               int16
	YMin               int16
	XMax               int16
	YMax               int16
	MacStyle           uint16
	LowestRecPPEM      uint16
	FontDirectionHint  int16
	IndexToLocFormat   int16 // 0 for short offsets (uint16), 1 for long (uint32)
	GlyphDataFormat    int16
}

// MaxpTable represents the 'maxp' maximum profile table.
type MaxpTable struct {
	Version               uint32
	NumGlyphs             uint16
	MaxPoints             uint16
	MaxContours           uint16
	MaxCompositePoints    uint16
	MaxCompositeContours  uint16
	MaxZones              uint16
	MaxTwilightPoints     uint16
	MaxStorage            uint16
	MaxFunctionDefs       uint16
	MaxInstructionDefs    uint16
	MaxStackElements      uint16
	MaxSizeOfInstructions uint16
	MaxComponentElements  uint16
	MaxComponentDepth     uint16
}

func parseHead(s api.Stream) (HeadTable, error) {
	var h HeadTable
	var err error

	if s.Size() < 54 {
		return h, errors.New("head table too short")
	}

	h.MajorVersion, err = readUint16(s, 0)
	if err != nil {
		return h, err
	}
	h.MinorVersion, err = readUint16(s, 2)
	if err != nil {
		return h, err
	}
	h.FontRevision, err = readInt32(s, 4)
	if err != nil {
		return h, err
	}
	h.CheckSumAdjustment, err = readUint32(s, 8)
	if err != nil {
		return h, err
	}
	h.MagicNumber, err = readUint32(s, 12)
	if err != nil {
		return h, err
	}
	h.Flags, err = readUint16(s, 16)
	if err != nil {
		return h, err
	}
	h.UnitsPerEm, err = readUint16(s, 18)
	if err != nil {
		return h, err
	}

	created, err := readUint64(s, 20)
	if err != nil {
		return h, err
	}
	h.Created = int64(created)

	modified, err := readUint64(s, 28)
	if err != nil {
		return h, err
	}
	h.Modified = int64(modified)

	h.XMin, err = readInt16(s, 36)
	if err != nil {
		return h, err
	}
	h.YMin, err = readInt16(s, 38)
	if err != nil {
		return h, err
	}
	h.XMax, err = readInt16(s, 40)
	if err != nil {
		return h, err
	}
	h.YMax, err = readInt16(s, 42)
	if err != nil {
		return h, err
	}
	h.MacStyle, err = readUint16(s, 44)
	if err != nil {
		return h, err
	}
	h.LowestRecPPEM, err = readUint16(s, 46)
	if err != nil {
		return h, err
	}
	h.FontDirectionHint, err = readInt16(s, 48)
	if err != nil {
		return h, err
	}
	h.IndexToLocFormat, err = readInt16(s, 50)
	if err != nil {
		return h, err
	}
	h.GlyphDataFormat, err = readInt16(s, 52)
	if err != nil {
		return h, err
	}

	return h, nil
}

// HheaTable represents the 'hhea' horizontal header table.
type HheaTable struct {
	Ascender            int16
	Descender           int16
	LineGap             int16
	AdvanceWidthMax     uint16
	MinLeftSideBearing  int16
	MinRightSideBearing int16
	XMaxExtent          int16
	CaretSlopeRise      int16
	CaretSlopeRun       int16
	CaretOffset         int16
	MetricDataFormat    int16
	NumberOfHMetrics    uint16
}

func parseHhea(s api.Stream) (HheaTable, error) {
	var h HheaTable
	var err error

	if s.Size() < 36 {
		return h, errors.New("hhea table too short")
	}

	h.Ascender, err = readInt16(s, 4)
	if err != nil {
		return h, err
	}
	h.Descender, err = readInt16(s, 6)
	if err != nil {
		return h, err
	}
	h.LineGap, err = readInt16(s, 8)
	if err != nil {
		return h, err
	}
	h.AdvanceWidthMax, err = readUint16(s, 10)
	if err != nil {
		return h, err
	}
	h.MinLeftSideBearing, err = readInt16(s, 12)
	if err != nil {
		return h, err
	}
	h.MinRightSideBearing, err = readInt16(s, 14)
	if err != nil {
		return h, err
	}
	h.XMaxExtent, err = readInt16(s, 16)
	if err != nil {
		return h, err
	}
	h.CaretSlopeRise, err = readInt16(s, 18)
	if err != nil {
		return h, err
	}
	h.CaretSlopeRun, err = readInt16(s, 20)
	if err != nil {
		return h, err
	}
	h.CaretOffset, err = readInt16(s, 22)
	if err != nil {
		return h, err
	}
	h.MetricDataFormat, err = readInt16(s, 32)
	if err != nil {
		return h, err
	}
	h.NumberOfHMetrics, err = readUint16(s, 34)
	if err != nil {
		return h, err
	}

	return h, nil
}

// HMetric represents a single horizontal metric.
type HMetric struct {
	AdvanceWidth    uint16
	LeftSideBearing int16
}

// HmtxTable represents the 'hmtx' horizontal metrics table.
type HmtxTable struct {
	HMetrics         []HMetric
	LeftSideBearings []int16
}

func parseHmtx(s api.Stream, numGlyphs int, numberOfHMetrics int) (HmtxTable, error) {
	var h HmtxTable
	var err error

	if int(s.Size()) < numberOfHMetrics*4+(numGlyphs-numberOfHMetrics)*2 {
		return h, errors.New("hmtx table too short")
	}

	h.HMetrics = make([]HMetric, numberOfHMetrics)
	for i := 0; i < numberOfHMetrics; i++ {
		h.HMetrics[i].AdvanceWidth, err = readUint16(s, int64(i*4))
		if err != nil {
			return h, err
		}
		h.HMetrics[i].LeftSideBearing, err = readInt16(s, int64(i*4+2))
		if err != nil {
			return h, err
		}
	}

	numLSB := numGlyphs - numberOfHMetrics
	if numLSB > 0 {
		h.LeftSideBearings = make([]int16, numLSB)
		offset := int64(numberOfHMetrics * 4)
		for i := 0; i < numLSB; i++ {
			h.LeftSideBearings[i], err = readInt16(s, offset+int64(i*2))
			if err != nil {
				return h, err
			}
		}
	}

	return h, nil
}

// CMap represents a character map.
type CMap interface {
	Lookup(char rune) uint16
}

type CMapFormat0 struct {
	GlyphIdArray [256]byte
}

func (m *CMapFormat0) Lookup(char rune) uint16 {
	if char >= 0 && char < 256 {
		return uint16(m.GlyphIdArray[char])
	}
	return 0
}

type CMapFormat4 struct {
	segCount      int
	endCount      []uint16
	startCount    []uint16
	idDelta       []int16
	idRangeOffset []uint16
	glyphIdArray  []uint16
}

func (m *CMapFormat4) Lookup(char rune) uint16 {
	c := uint16(char)
	if rune(c) != char {
		return 0
	}

	// Binary search for the first endCount >= c
	i, j := 0, m.segCount
	for i < j {
		h := i + (j-i)/2
		if m.endCount[h] < c {
			i = h + 1
		} else {
			j = h
		}
	}

	if i >= m.segCount || m.startCount[i] > c {
		return 0
	}

	if m.idRangeOffset[i] == 0 {
		return (c + uint16(m.idDelta[i])) & 0xFFFF
	}

	// idRangeOffset[i] is the offset in bytes from the current idRangeOffset[i]
	// to the glyphIndex.
	// In Format 4, the glyphIdArray follows the idRangeOffset array.
	// index = idRangeOffset[i]/2 + (c - startCount[i]) + (i - segCount)
	// Wait, the standard formula is a bit tricky due to pointer arithmetic in C.
	// glyphIndex = *(idRangeOffset[i]/2 + (c - startCount[i]) + &idRangeOffset[i])

	// Let's use a simpler way: the index into glyphIdArray is:
	// index = (c - m.startCount[i]) + (m.idRangeOffset[i] / 2) - (m.segCount - i)
	index := int(c-m.startCount[i]) + int(m.idRangeOffset[i]/2) - (m.segCount - i)
	if index < 0 || index >= len(m.glyphIdArray) {
		return 0
	}
	gid := m.glyphIdArray[index]
	if gid != 0 {
		gid = (gid + uint16(m.idDelta[i])) & 0xFFFF
	}
	return gid
}

type CMapFormat12 struct {
	Groups []CMapFormat12Group
}

type CMapFormat12Group struct {
	StartCharCode uint32
	EndCharCode   uint32
	StartGlyphID  uint32
}

func (m *CMapFormat12) Lookup(char rune) uint16 {
	c := uint32(char)
	i, j := 0, len(m.Groups)
	for i < j {
		h := i + (j-i)/2
		if m.Groups[h].EndCharCode < c {
			i = h + 1
		} else {
			j = h
		}
	}

	if i < len(m.Groups) && m.Groups[i].StartCharCode <= c {
		return uint16(m.Groups[i].StartGlyphID + (c - m.Groups[i].StartCharCode))
	}
	return 0
}

func parseCMap(s api.Stream) (CMap, error) {
	if s.Size() < 4 {
		return nil, errors.New("cmap table too short")
	}

	numTables, err := readUint16(s, 2)
	if err != nil {
		return nil, err
	}

	var bestCMap CMap
	bestScore := -1

	for i := 0; i < int(numTables); i++ {
		off := int64(4 + i*8)
		platformID, err := readUint16(s, off)
		if err != nil {
			return nil, err
		}
		encodingID, err := readUint16(s, off+2)
		if err != nil {
			return nil, err
		}
		offset, err := readUint32(s, off+4)
		if err != nil {
			return nil, err
		}

		score := -1
		if platformID == 3 && encodingID == 10 { // Windows Unicode full repertoire
			score = 10
		} else if platformID == 3 && encodingID == 1 { // Windows Unicode BMP
			score = 5
		} else if platformID == 0 { // Unicode
			score = 1
		}

		if score > bestScore {
			cmap, err := parseCMapSubtable(s, int64(offset))
			if err == nil {
				bestCMap = cmap
				bestScore = score
			}
		}
	}

	if bestCMap == nil {
		return nil, errors.New("no supported cmap subtable found")
	}

	return bestCMap, nil
}

func parseCMapSubtable(s api.Stream, offset int64) (CMap, error) {
	format, err := readUint16(s, offset)
	if err != nil {
		return nil, err
	}

	switch format {
	case 0:
		if s.Size() < offset+262 {
			return nil, errors.New("cmap format 0 too short")
		}
		var m CMapFormat0
		for i := 0; i < 256; i++ {
			b, err := readByte(s, offset+6+int64(i))
			if err != nil {
				return nil, err
			}
			m.GlyphIdArray[i] = b
		}
		return &m, nil

	case 4:
		length, err := readUint16(s, offset+2)
		if err != nil {
			return nil, err
		}
		if s.Size() < offset+int64(length) {
			return nil, errors.New("cmap format 4 too short")
		}
		segCountX2, err := readUint16(s, offset+6)
		if err != nil {
			return nil, err
		}
		segCount := int(segCountX2 / 2)

		m := &CMapFormat4{
			segCount:      segCount,
			endCount:      make([]uint16, segCount),
			startCount:    make([]uint16, segCount),
			idDelta:       make([]int16, segCount),
			idRangeOffset: make([]uint16, segCount),
		}

		for i := 0; i < segCount; i++ {
			m.endCount[i], err = readUint16(s, offset+14+int64(i*2))
			if err != nil {
				return nil, err
			}
		}
		// skip reservedPad
		for i := 0; i < segCount; i++ {
			m.startCount[i], err = readUint16(s, offset+16+int64(segCount*2)+int64(i*2))
			if err != nil {
				return nil, err
			}
		}
		for i := 0; i < segCount; i++ {
			m.idDelta[i], err = readInt16(s, offset+16+int64(segCount*4)+int64(i*2))
			if err != nil {
				return nil, err
			}
		}
		for i := 0; i < segCount; i++ {
			m.idRangeOffset[i], err = readUint16(s, offset+16+int64(segCount*6)+int64(i*2))
			if err != nil {
				return nil, err
			}
		}

		glyphIdArraySize := (int(length) - (16 + segCount*8)) / 2
		if glyphIdArraySize > 0 {
			m.glyphIdArray = make([]uint16, glyphIdArraySize)
			for i := 0; i < glyphIdArraySize; i++ {
				m.glyphIdArray[i], err = readUint16(s, offset+16+int64(segCount*8)+int64(i*2))
				if err != nil {
					return nil, err
				}
			}
		}
		return m, nil

	case 12:
		length, err := readUint32(s, offset+4)
		if err != nil {
			return nil, err
		}
		if s.Size() < offset+int64(length) {
			return nil, errors.New("cmap format 12 too short")
		}
		numGroups, err := readUint32(s, offset+12)
		if err != nil {
			return nil, err
		}

		m := &CMapFormat12{
			Groups: make([]CMapFormat12Group, numGroups),
		}
		for i := uint32(0); i < numGroups; i++ {
			m.Groups[i].StartCharCode, err = readUint32(s, offset+16+int64(i*12))
			if err != nil {
				return nil, err
			}
			m.Groups[i].EndCharCode, err = readUint32(s, offset+16+int64(i*12+4))
			if err != nil {
				return nil, err
			}
			m.Groups[i].StartGlyphID, err = readUint32(s, offset+16+int64(i*12+8))
			if err != nil {
				return nil, err
			}
		}
		return m, nil
	}

	return nil, fmt.Errorf("unsupported cmap format: %d", format)
}

func parseMaxp(s api.Stream) (MaxpTable, error) {
	var m MaxpTable
	var err error

	if s.Size() < 6 {
		return m, errors.New("maxp table too short")
	}

	m.Version, err = readUint32(s, 0)
	if err != nil {
		return m, err
	}
	m.NumGlyphs, err = readUint16(s, 4)
	if err != nil {
		return m, err
	}

	if m.Version == 0x00010000 {
		if s.Size() < 32 {
			return m, errors.New("maxp table version 1.0 too short")
		}
		m.MaxPoints, _ = readUint16(s, 6)
		m.MaxContours, _ = readUint16(s, 8)
		m.MaxCompositePoints, _ = readUint16(s, 10)
		m.MaxCompositeContours, _ = readUint16(s, 12)
		m.MaxZones, _ = readUint16(s, 14)
		m.MaxTwilightPoints, _ = readUint16(s, 16)
		m.MaxStorage, _ = readUint16(s, 18)
		m.MaxFunctionDefs, _ = readUint16(s, 20)
		m.MaxInstructionDefs, _ = readUint16(s, 22)
		m.MaxStackElements, _ = readUint16(s, 24)
		m.MaxSizeOfInstructions, _ = readUint16(s, 26)
		m.MaxComponentElements, _ = readUint16(s, 28)
		m.MaxComponentDepth, _ = readUint16(s, 30)
	}

	return m, nil
}

func parseFpgm(s api.Stream) ([]byte, error) {
	return readStreamData(s)
}

func parsePrep(s api.Stream) ([]byte, error) {
	return readStreamData(s)
}

func parseCvt(s api.Stream) ([]int32, error) {
	n := s.Size() / 2
	cvt := make([]int32, n)
	for i := int64(0); i < n; i++ {
		val, err := readInt16(s, i*2)
		if err != nil {
			return nil, err
		}
		// CVT values are initially in FUnits.
		// They will be scaled by the interpreter if needed,
		// but the table itself stores them as-is.
		cvt[i] = int32(val)
	}
	return cvt, nil
}

// OS2Table represents the 'OS/2' OS/2 and Windows metrics table.
type OS2Table struct {
	Version             uint16
	XAvgCharWidth       int16
	UsWeightClass       uint16
	UsWidthClass        uint16
	FsType              uint16
	YSubscriptXSize     int16
	YSubscriptYSize     int16
	YSubscriptXOffset   int16
	YSubscriptYOffset   int16
	YSuperscriptXSize   int16
	YSuperscriptYSize   int16
	YSuperscriptXOffset int16
	YSuperscriptYOffset int16
	YStrikeoutSize      int16
	YStrikeoutPosition  int16
	SFamilyClass        int16
	Panose              [10]byte
	UlUnicodeRange1     uint32
	UlUnicodeRange2     uint32
	UlUnicodeRange3     uint32
	UlUnicodeRange4     uint32
	AchVendID           [4]byte
	FsSelection         uint16
	UsFirstCharIndex    uint16
	UsLastCharIndex     uint16
	STypoAscender       int16
	STypoDescender      int16
	STypoLineGap        int16
	UsWinAscent         uint16
	UsWinDescent        uint16
	UlCodePageRange1    uint32
	UlCodePageRange2    uint32
	SxHeight            int16
	SCapHeight          int16
	UsDefaultChar       uint16
	UsBreakChar         uint16
	UsMaxContext        uint16
}

func parseOS2(s api.Stream) (OS2Table, error) {
	var t OS2Table
	var err error

	if s.Size() < 78 {
		return t, errors.New("OS/2 table too short")
	}

	t.Version, err = readUint16(s, 0)
	if err != nil {
		return t, err
	}
	t.XAvgCharWidth, err = readInt16(s, 2)
	if err != nil {
		return t, err
	}
	t.UsWeightClass, err = readUint16(s, 4)
	if err != nil {
		return t, err
	}
	t.UsWidthClass, err = readUint16(s, 6)
	if err != nil {
		return t, err
	}
	t.FsType, err = readUint16(s, 8)
	if err != nil {
		return t, err
	}
	t.YSubscriptXSize, err = readInt16(s, 10)
	if err != nil {
		return t, err
	}
	t.YSubscriptYSize, err = readInt16(s, 12)
	if err != nil {
		return t, err
	}
	t.YSubscriptXOffset, err = readInt16(s, 14)
	if err != nil {
		return t, err
	}
	t.YSubscriptYOffset, err = readInt16(s, 16)
	if err != nil {
		return t, err
	}
	t.YSuperscriptXSize, err = readInt16(s, 18)
	if err != nil {
		return t, err
	}
	t.YSuperscriptYSize, err = readInt16(s, 20)
	if err != nil {
		return t, err
	}
	t.YSuperscriptXOffset, err = readInt16(s, 22)
	if err != nil {
		return t, err
	}
	t.YSuperscriptYOffset, err = readInt16(s, 24)
	if err != nil {
		return t, err
	}
	t.YStrikeoutSize, err = readInt16(s, 26)
	if err != nil {
		return t, err
	}
	t.YStrikeoutPosition, err = readInt16(s, 28)
	if err != nil {
		return t, err
	}
	t.SFamilyClass, err = readInt16(s, 30)
	if err != nil {
		return t, err
	}
	if err := readExactAt(s, t.Panose[:], 32); err != nil {
		return t, err
	}
	t.UlUnicodeRange1, err = readUint32(s, 42)
	if err != nil {
		return t, err
	}
	t.UlUnicodeRange2, err = readUint32(s, 46)
	if err != nil {
		return t, err
	}
	t.UlUnicodeRange3, err = readUint32(s, 50)
	if err != nil {
		return t, err
	}
	t.UlUnicodeRange4, err = readUint32(s, 54)
	if err != nil {
		return t, err
	}
	if err := readExactAt(s, t.AchVendID[:], 58); err != nil {
		return t, err
	}
	t.FsSelection, err = readUint16(s, 62)
	if err != nil {
		return t, err
	}
	t.UsFirstCharIndex, err = readUint16(s, 64)
	if err != nil {
		return t, err
	}
	t.UsLastCharIndex, err = readUint16(s, 66)
	if err != nil {
		return t, err
	}
	t.STypoAscender, err = readInt16(s, 68)
	if err != nil {
		return t, err
	}
	t.STypoDescender, err = readInt16(s, 70)
	if err != nil {
		return t, err
	}
	t.STypoLineGap, err = readInt16(s, 72)
	if err != nil {
		return t, err
	}
	t.UsWinAscent, err = readUint16(s, 74)
	if err != nil {
		return t, err
	}
	t.UsWinDescent, err = readUint16(s, 76)
	if err != nil {
		return t, err
	}

	if t.Version >= 1 && s.Size() >= 86 {
		t.UlCodePageRange1, err = readUint32(s, 78)
		if err != nil {
			return t, err
		}
		t.UlCodePageRange2, err = readUint32(s, 82)
		if err != nil {
			return t, err
		}
	}

	if t.Version >= 2 && s.Size() >= 96 {
		t.SxHeight, err = readInt16(s, 86)
		if err != nil {
			return t, err
		}
		t.SCapHeight, err = readInt16(s, 88)
		if err != nil {
			return t, err
		}
		t.UsDefaultChar, err = readUint16(s, 90)
		if err != nil {
			return t, err
		}
		t.UsBreakChar, err = readUint16(s, 92)
		if err != nil {
			return t, err
		}
		t.UsMaxContext, err = readUint16(s, 94)
		if err != nil {
			return t, err
		}
	}

	return t, nil
}

// PostTable represents the 'post' PostScript information table.
type PostTable struct {
	Version            uint32
	ItalicAngle        int32
	UnderlinePosition  int16
	UnderlineThickness int16
	IsFixedPitch       uint32
	MinMemType42       uint32
	MaxMemType42       uint32
	MinMemType1        uint32
	MaxMemType1        uint32
	// Version 2.0 fields
	NumGlyphs   uint16
	GlyphNameID []uint16
	Names       []string
}

func parsePost(s api.Stream) (PostTable, error) {
	var t PostTable
	var err error

	if s.Size() < 32 {
		return t, errors.New("post table too short")
	}

	t.Version, err = readUint32(s, 0)
	if err != nil {
		return t, err
	}
	t.ItalicAngle, err = readInt32(s, 4)
	if err != nil {
		return t, err
	}
	t.UnderlinePosition, err = readInt16(s, 8)
	if err != nil {
		return t, err
	}
	t.UnderlineThickness, err = readInt16(s, 10)
	if err != nil {
		return t, err
	}
	t.IsFixedPitch, err = readUint32(s, 12)
	if err != nil {
		return t, err
	}
	t.MinMemType42, err = readUint32(s, 16)
	if err != nil {
		return t, err
	}
	t.MaxMemType42, err = readUint32(s, 20)
	if err != nil {
		return t, err
	}
	t.MinMemType1, err = readUint32(s, 24)
	if err != nil {
		return t, err
	}
	t.MaxMemType1, err = readUint32(s, 28)
	if err != nil {
		return t, err
	}

	if t.Version == 0x00020000 && s.Size() >= 34 {
		t.NumGlyphs, err = readUint16(s, 32)
		if err != nil {
			return t, err
		}
		t.GlyphNameID = make([]uint16, t.NumGlyphs)
		for i := 0; i < int(t.NumGlyphs); i++ {
			t.GlyphNameID[i], err = readUint16(s, 34+int64(i*2))
			if err != nil {
				return t, err
			}
		}

		// Read Pascal strings
		offset := 34 + int64(t.NumGlyphs*2)
		for offset < s.Size() {
			length, err := readByte(s, offset)
			if err != nil {
				break
			}
			offset++
			if length == 0 {
				t.Names = append(t.Names, "")
				continue
			}
			name := make([]byte, length)
			if err := readExactAt(s, name, offset); err != nil {
				break
			}
			t.Names = append(t.Names, string(name))
			offset += int64(length)
		}
	}

	return t, nil
}

// VMetric represents a single vertical metric.
type VMetric struct {
	AdvanceHeight  uint16
	TopSideBearing int16
}

// VmtxTable represents the 'vmtx' vertical metrics table.
type VmtxTable struct {
	VMetrics        []VMetric
	TopSideBearings []int16
}

func parseVmtx(s api.Stream, numGlyphs int, numberOfVMetrics int) (VmtxTable, error) {
	var v VmtxTable
	var err error

	if int(s.Size()) < numberOfVMetrics*4+(numGlyphs-numberOfVMetrics)*2 {
		return v, errors.New("vmtx table too short")
	}

	v.VMetrics = make([]VMetric, numberOfVMetrics)
	for i := 0; i < numberOfVMetrics; i++ {
		v.VMetrics[i].AdvanceHeight, err = readUint16(s, int64(i*4))
		if err != nil {
			return v, err
		}
		v.VMetrics[i].TopSideBearing, err = readInt16(s, int64(i*4+2))
		if err != nil {
			return v, err
		}
	}

	numTSB := numGlyphs - numberOfVMetrics
	if numTSB > 0 {
		v.TopSideBearings = make([]int16, numTSB)
		offset := int64(numberOfVMetrics * 4)
		for i := 0; i < numTSB; i++ {
			v.TopSideBearings[i], err = readInt16(s, offset+int64(i*2))
			if err != nil {
				return v, err
			}
		}
	}

	return v, nil
}

// VheaTable represents the 'vhea' vertical header table.
type VheaTable struct {
	Version              uint32
	Ascent               int16
	Descent              int16
	LineGap              int16
	advanceHeightMax     int16
	minTopSideBearing    int16
	minBottomSideBearing int16
	yMaxExtent           int16
	caretSlopeRise       int16
	caretSlopeRun        int16
	caretOffset          int16
	metricDataFormat     int16
	NumOfLongVerMetrics  uint16
}

func parseVhea(s api.Stream) (VheaTable, error) {
	var h VheaTable
	var err error

	if s.Size() < 36 {
		return h, errors.New("vhea table too short")
	}

	h.Version, err = readUint32(s, 0)
	if err != nil {
		return h, err
	}
	h.Ascent, err = readInt16(s, 4)
	if err != nil {
		return h, err
	}
	h.Descent, err = readInt16(s, 6)
	if err != nil {
		return h, err
	}
	h.LineGap, err = readInt16(s, 8)
	if err != nil {
		return h, err
	}
	h.advanceHeightMax, err = readInt16(s, 10)
	if err != nil {
		return h, err
	}
	h.minTopSideBearing, err = readInt16(s, 12)
	if err != nil {
		return h, err
	}
	h.minBottomSideBearing, err = readInt16(s, 14)
	if err != nil {
		return h, err
	}
	h.yMaxExtent, err = readInt16(s, 16)
	if err != nil {
		return h, err
	}
	h.caretSlopeRise, err = readInt16(s, 18)
	if err != nil {
		return h, err
	}
	h.caretSlopeRun, err = readInt16(s, 20)
	if err != nil {
		return h, err
	}
	h.caretOffset, err = readInt16(s, 22)
	if err != nil {
		return h, err
	}
	h.metricDataFormat, err = readInt16(s, 32)
	if err != nil {
		return h, err
	}
	h.NumOfLongVerMetrics, err = readUint16(s, 34)
	if err != nil {
		return h, err
	}

	return h, nil
}

// VertOriginYMetric represents a single vertical origin Y metric.
type VertOriginYMetric struct {
	GlyphIndex  uint16
	VertOriginY int16
}

// VORGTable represents the 'VORG' vertical origin table.
type VORGTable struct {
	MajorVersion          uint16
	MinorVersion          uint16
	DefaultVertOriginY    int16
	NumVertOriginYMetrics uint16
	VertOriginYMetrics    []VertOriginYMetric
}

func parseVORG(s api.Stream) (VORGTable, error) {
	var t VORGTable
	var err error

	if s.Size() < 8 {
		return t, errors.New("VORG table too short")
	}

	t.MajorVersion, err = readUint16(s, 0)
	if err != nil {
		return t, err
	}
	t.MinorVersion, err = readUint16(s, 2)
	if err != nil {
		return t, err
	}
	t.DefaultVertOriginY, err = readInt16(s, 4)
	if err != nil {
		return t, err
	}
	t.NumVertOriginYMetrics, err = readUint16(s, 6)
	if err != nil {
		return t, err
	}

	if int(s.Size()) < 8+int(t.NumVertOriginYMetrics)*4 {
		return t, errors.New("VORG table too short for metrics")
	}

	t.VertOriginYMetrics = make([]VertOriginYMetric, t.NumVertOriginYMetrics)
	for i := 0; i < int(t.NumVertOriginYMetrics); i++ {
		t.VertOriginYMetrics[i].GlyphIndex, err = readUint16(s, int64(8+i*4))
		if err != nil {
			return t, err
		}
		t.VertOriginYMetrics[i].VertOriginY, err = readInt16(s, int64(8+i*4+2))
		if err != nil {
			return t, err
		}
	}

	return t, nil
}

type BASETable struct {
	MajorVersion    uint16
	MinorVersion    uint16
	HorizAxisOffset uint16
	VertAxisOffset  uint16
}

func ParseBASE(s api.Stream) (BASETable, error) {
	var t BASETable
	var err error
	if s.Size() < 4 {
		return t, errors.New("BASE table too short")
	}
	t.MajorVersion, err = readUint16(s, 0)
	if err != nil {
		return t, err
	}
	t.MinorVersion, err = readUint16(s, 2)
	if err != nil {
		return t, err
	}
	if s.Size() >= 8 {
		t.HorizAxisOffset, err = readUint16(s, 4)
		if err != nil {
			return t, err
		}
		t.VertAxisOffset, err = readUint16(s, 6)
		if err != nil {
			return t, err
		}
	}
	return t, nil
}

type JstfTable struct {
	MajorVersion    uint16
	MinorVersion    uint16
	JstfScriptCount uint16
}

func ParseJstf(s api.Stream) (JstfTable, error) {
	var t JstfTable
	var err error
	if s.Size() < 4 {
		return t, errors.New("Jstf table too short")
	}
	t.MajorVersion, err = readUint16(s, 0)
	if err != nil {
		return t, err
	}
	t.MinorVersion, err = readUint16(s, 2)
	if err != nil {
		return t, err
	}
	if s.Size() >= 6 {
		t.JstfScriptCount, err = readUint16(s, 4)
		if err != nil {
			return t, err
		}
	}
	return t, nil
}

type GaspRange struct {
	RangeMaxPPEM      uint16
	RangeGaspBehavior uint16
}

type GaspTable struct {
	Version    uint16
	NumRanges  uint16
	GaspRanges []GaspRange
}

func parseGasp(s api.Stream) (GaspTable, error) {
	var t GaspTable
	var err error
	if s.Size() < 4 {
		return t, errors.New("gasp table too short")
	}
	t.Version, err = readUint16(s, 0)
	if err != nil {
		return t, err
	}
	t.NumRanges, err = readUint16(s, 2)
	if err != nil {
		return t, err
	}

	if s.Size() < 4+int64(t.NumRanges)*4 {
		return t, errors.New("gasp table too short for ranges")
	}
	t.GaspRanges = make([]GaspRange, t.NumRanges)
	for i := 0; i < int(t.NumRanges); i++ {
		t.GaspRanges[i].RangeMaxPPEM, err = readUint16(s, 4+int64(i*4))
		if err != nil {
			return t, err
		}
		t.GaspRanges[i].RangeGaspBehavior, err = readUint16(s, 4+int64(i*4)+2)
		if err != nil {
			return t, err
		}
	}
	return t, nil
}

type VDMXTable struct {
	Version       uint16
	NumRatios     uint16
	NumVDMXGroups uint16
}

func parseVDMX(s api.Stream) (VDMXTable, error) {
	var t VDMXTable
	var err error
	if s.Size() < 6 {
		return t, errors.New("VDMX table too short")
	}
	t.Version, err = readUint16(s, 0)
	if err != nil {
		return t, err
	}
	t.NumRatios, err = readUint16(s, 2)
	if err != nil {
		return t, err
	}
	t.NumVDMXGroups, err = readUint16(s, 4)
	if err != nil {
		return t, err
	}
	return t, nil
}

type HdmxDeviceRecord struct {
	PixelSize byte
	MaxWidth  byte
	Widths    []byte
}

type HdmxTable struct {
	Version          uint16
	NumRecords       int16
	SizeDeviceRecord int32
	Records          []HdmxDeviceRecord
}

func parseHdmx(s api.Stream, numGlyphs int) (HdmxTable, error) {
	var t HdmxTable
	var err error
	if s.Size() < 8 {
		return t, errors.New("hdmx table too short")
	}
	t.Version, err = readUint16(s, 0)
	if err != nil {
		return t, err
	}

	numRecs, err := readInt16(s, 2)
	if err != nil {
		return t, err
	}
	t.NumRecords = numRecs

	t.SizeDeviceRecord, err = readInt32(s, 4)
	if err != nil {
		return t, err
	}

	if numGlyphs < 0 {
		return t, errors.New("negative glyph count")
	}
	if t.NumRecords < 0 {
		return t, fmt.Errorf("invalid hdmx record count: %d", t.NumRecords)
	}
	if t.SizeDeviceRecord < int32(2+numGlyphs) {
		return t, fmt.Errorf("invalid hdmx device record size: %d", t.SizeDeviceRecord)
	}
	recordSize := int64(t.SizeDeviceRecord)
	if s.Size() < 8+int64(t.NumRecords)*recordSize {
		return t, errors.New("hdmx table too short for declared records")
	}

	t.Records = make([]HdmxDeviceRecord, int(t.NumRecords))
	offset := int64(8)
	for i := 0; i < int(t.NumRecords); i++ {
		t.Records[i].PixelSize, err = readByte(s, offset)
		if err != nil {
			return t, err
		}
		t.Records[i].MaxWidth, err = readByte(s, offset+1)
		if err != nil {
			return t, err
		}

		t.Records[i].Widths = make([]byte, numGlyphs)
		if err := readExactAt(s, t.Records[i].Widths, offset+2); err != nil {
			return t, err
		}
		offset += recordSize
	}

	return t, nil
}

type LTSHTable struct {
	Version   uint16
	NumGlyphs uint16
	Ypels     []byte
}

func parseLTSH(s api.Stream) (LTSHTable, error) {
	var t LTSHTable
	var err error
	if s.Size() < 4 {
		return t, errors.New("LTSH table too short")
	}
	t.Version, err = readUint16(s, 0)
	if err != nil {
		return t, err
	}
	t.NumGlyphs, err = readUint16(s, 2)
	if err != nil {
		return t, err
	}

	if s.Size() < 4+int64(t.NumGlyphs) {
		return t, errors.New("LTSH table too short for declared glyph count")
	}

	t.Ypels = make([]byte, t.NumGlyphs)
	if err := readExactAt(s, t.Ypels, 4); err != nil {
		return t, err
	}
	return t, nil
}

type STATTable struct {
	MajorVersion         uint16
	MinorVersion         uint16
	DesignAxisSize       uint16
	DesignAxisCount      uint16
	DesignAxesOffset     uint32
	AxisValueCount       uint16
	AxisValueOffset      uint32
	ElidedFallbackNameID uint16
}

func parseSTAT(s api.Stream) (STATTable, error) {
	var t STATTable
	var err error
	if s.Size() < 12 {
		return t, errors.New("STAT table too short")
	}
	t.MajorVersion, err = readUint16(s, 0)
	if err != nil {
		return t, err
	}
	t.MinorVersion, err = readUint16(s, 2)
	if err != nil {
		return t, err
	}
	t.DesignAxisSize, err = readUint16(s, 4)
	if err != nil {
		return t, err
	}
	t.DesignAxisCount, err = readUint16(s, 6)
	if err != nil {
		return t, err
	}
	t.DesignAxesOffset, err = readUint32(s, 8)
	if err != nil {
		return t, err
	}

	if s.Size() >= 18 {
		t.AxisValueCount, err = readUint16(s, 12)
		if err != nil {
			return t, err
		}
		t.AxisValueOffset, err = readUint32(s, 14)
		if err != nil {
			return t, err
		}
		if t.MajorVersion > 1 || (t.MajorVersion == 1 && t.MinorVersion > 1) {
			if s.Size() >= 20 {
				t.ElidedFallbackNameID, err = readUint16(s, 18)
				if err != nil {
					return t, err
				}
			}
		}
	}
	return t, nil
}
