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

// CMapVariation represents a cmap that can resolve Unicode variation sequences.
type CMapVariation interface {
	LookupVariation(char rune, selector rune) (glyphID uint16, useDefault bool, ok bool)
}

// CMapTable combines a base cmap with a format 14 Unicode variation sequence map.
type CMapTable struct {
	Base      CMap
	Variation *CMapFormat14
}

func (m *CMapTable) Lookup(char rune) uint16 {
	if m == nil || m.Base == nil {
		return 0
	}
	return m.Base.Lookup(char)
}

func (m *CMapTable) LookupVariation(char rune, selector rune) (uint16, bool, bool) {
	if m == nil || m.Variation == nil {
		return 0, false, false
	}
	gid, useDefault, ok := m.Variation.LookupVariation(char, selector)
	if !ok {
		return 0, false, false
	}
	if useDefault && m.Base != nil {
		gid = m.Base.Lookup(char)
	}
	return gid, useDefault, true
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

type CMapFormat2SubHeader struct {
	FirstCode     uint16
	EntryCount    uint16
	IDDelta       int16
	IDRangeOffset uint16

	glyphIdArrayIndex int
}

type CMapFormat2 struct {
	SubHeaderKeys [256]uint16
	SubHeaders    []CMapFormat2SubHeader
	GlyphIdArray  []uint16
}

func (m *CMapFormat2) Lookup(char rune) uint16 {
	if char < 0 || char > 0xffff {
		return 0
	}

	c := uint16(char)
	var subHeaderIndex int
	var codeByte uint16
	if c <= 0xff {
		key := m.SubHeaderKeys[byte(c)]
		if key != 0 {
			return 0
		}
		codeByte = uint16(byte(c))
	} else {
		key := m.SubHeaderKeys[byte(c>>8)]
		if key == 0 {
			return 0
		}
		subHeaderIndex = int(key / 8)
		codeByte = uint16(byte(c))
	}

	if subHeaderIndex >= len(m.SubHeaders) {
		return 0
	}
	subHeader := m.SubHeaders[subHeaderIndex]
	if codeByte < subHeader.FirstCode || codeByte-subHeader.FirstCode >= subHeader.EntryCount {
		return 0
	}

	glyphIndex := subHeader.glyphIdArrayIndex + int(codeByte-subHeader.FirstCode)
	if glyphIndex < 0 || glyphIndex >= len(m.GlyphIdArray) {
		return 0
	}
	gid := m.GlyphIdArray[glyphIndex]
	if gid != 0 {
		gid += uint16(subHeader.IDDelta)
	}
	return gid
}

type CMapFormat6 struct {
	FirstCode    uint16
	GlyphIdArray []uint16
}

func (m *CMapFormat6) Lookup(char rune) uint16 {
	if char < 0 {
		return 0
	}
	c := uint32(char)
	first := uint32(m.FirstCode)
	if c < first || c-first >= uint32(len(m.GlyphIdArray)) {
		return 0
	}
	return m.GlyphIdArray[int(c-first)]
}

type CMapFormat8 struct {
	Is32   [cmapFormat8Is32Size]byte
	Groups []CMapFormat12Group
}

func (m *CMapFormat8) Lookup(char rune) uint16 {
	if char < 0 || uint32(char) > maxUnicodeScalar {
		return 0
	}
	c := uint32(char)
	if c <= 0xffff {
		if cmapFormat8Is32(&m.Is32, uint16(c)) {
			return 0
		}
	} else if !cmapFormat8Is32(&m.Is32, uint16(c>>16)) {
		return 0
	}

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
		gid := uint64(m.Groups[i].StartGlyphID) + uint64(c-m.Groups[i].StartCharCode)
		if gid > uint64(maxCMapGlyphID) {
			return 0
		}
		return uint16(gid)
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

type CMapFormat10 struct {
	StartCharCode uint32
	GlyphIdArray  []uint16
}

func (m *CMapFormat10) Lookup(char rune) uint16 {
	if char < 0 {
		return 0
	}
	c := uint32(char)
	if c < m.StartCharCode {
		return 0
	}
	index := c - m.StartCharCode
	if index >= uint32(len(m.GlyphIdArray)) {
		return 0
	}
	return m.GlyphIdArray[int(index)]
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
	if char < 0 {
		return 0
	}
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
		gid := uint64(m.Groups[i].StartGlyphID) + uint64(c-m.Groups[i].StartCharCode)
		if gid > uint64(maxCMapGlyphID) {
			return 0
		}
		return uint16(gid)
	}
	return 0
}

type CMapFormat13 struct {
	Groups []CMapFormat13Group
}

type CMapFormat13Group struct {
	StartCharCode uint32
	EndCharCode   uint32
	GlyphID       uint32
}

func (m *CMapFormat13) Lookup(char rune) uint16 {
	if char < 0 {
		return 0
	}
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
		if m.Groups[i].GlyphID > maxCMapGlyphID {
			return 0
		}
		return uint16(m.Groups[i].GlyphID)
	}
	return 0
}

type CMapFormat14 struct {
	VariationSelectors []CMapVariationSelectorRecord
}

type CMapVariationSelectorRecord struct {
	VarSelector   uint32
	DefaultUVS    []CMapDefaultUVSRange
	NonDefaultUVS []CMapNonDefaultUVSMapping
}

type CMapDefaultUVSRange struct {
	StartUnicodeValue uint32
	AdditionalCount   byte
}

type CMapNonDefaultUVSMapping struct {
	UnicodeValue uint32
	GlyphID      uint16
}

func (m *CMapFormat14) Lookup(char rune) uint16 {
	return 0
}

func (m *CMapFormat14) LookupVariation(char rune, selector rune) (uint16, bool, bool) {
	if char < 0 || selector < 0 || uint32(char) > maxUnicodeScalar || uint32(selector) > maxUnicodeScalar {
		return 0, false, false
	}
	c := uint32(char)
	sel := uint32(selector)

	i, j := 0, len(m.VariationSelectors)
	for i < j {
		h := i + (j-i)/2
		if m.VariationSelectors[h].VarSelector < sel {
			i = h + 1
		} else {
			j = h
		}
	}
	if i >= len(m.VariationSelectors) || m.VariationSelectors[i].VarSelector != sel {
		return 0, false, false
	}

	record := &m.VariationSelectors[i]
	if gid, ok := record.lookupNonDefault(c); ok {
		return gid, false, true
	}
	if record.hasDefault(c) {
		return 0, true, true
	}
	return 0, false, false
}

func (r *CMapVariationSelectorRecord) hasDefault(char uint32) bool {
	i, j := 0, len(r.DefaultUVS)
	for i < j {
		h := i + (j-i)/2
		if r.DefaultUVS[h].StartUnicodeValue+uint32(r.DefaultUVS[h].AdditionalCount) < char {
			i = h + 1
		} else {
			j = h
		}
	}
	if i >= len(r.DefaultUVS) {
		return false
	}
	uvsRange := r.DefaultUVS[i]
	return uvsRange.StartUnicodeValue <= char &&
		char <= uvsRange.StartUnicodeValue+uint32(uvsRange.AdditionalCount)
}

func (r *CMapVariationSelectorRecord) lookupNonDefault(char uint32) (uint16, bool) {
	i, j := 0, len(r.NonDefaultUVS)
	for i < j {
		h := i + (j-i)/2
		if r.NonDefaultUVS[h].UnicodeValue < char {
			i = h + 1
		} else {
			j = h
		}
	}
	if i >= len(r.NonDefaultUVS) || r.NonDefaultUVS[i].UnicodeValue != char {
		return 0, false
	}
	return r.NonDefaultUVS[i].GlyphID, true
}

const (
	maxCMapGlyphID        = uint32(0xffff)
	maxUnicodeScalar      = uint32(0x10ffff)
	cmapFormat2HeaderSize = 6 + 256*2
	cmapFormat2MinSize    = cmapFormat2HeaderSize + 8
	cmapFormat8Is32Size   = 8192
	cmapFormat8HeaderSize = 12 + cmapFormat8Is32Size + 4
)

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
	var bestVariation *CMapFormat14
	bestVariationScore := -1

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

		cmap, err := parseCMapSubtable(s, int64(offset))
		if err != nil {
			continue
		}
		if score < 0 {
			if _, ok := cmap.(*CMapFormat2); ok && isCMapFormat2Encoding(platformID, encodingID) {
				score = 0
			}
		}
		if variation, ok := cmap.(*CMapFormat14); ok {
			if score > bestVariationScore {
				bestVariation = variation
				bestVariationScore = score
			}
			continue
		}
		if score > bestScore {
			bestCMap = cmap
			bestScore = score
		}
	}

	if bestCMap == nil {
		if bestVariation != nil {
			return bestVariation, nil
		}
		return nil, errors.New("no supported cmap subtable found")
	}
	if bestVariation != nil {
		return &CMapTable{Base: bestCMap, Variation: bestVariation}, nil
	}

	return bestCMap, nil
}

func isCMapFormat2Encoding(platformID uint16, encodingID uint16) bool {
	return platformID == 1 || (platformID == 3 && encodingID >= 2 && encodingID <= 6)
}

func parseCMapSubtable(s api.Stream, offset int64) (CMap, error) {
	format, err := readUint16(s, offset)
	if err != nil {
		return nil, err
	}

	switch format {
	case 0:
		length, err := readUint16(s, offset+2)
		if err != nil {
			return nil, err
		}
		if err := checkCMapSubtableLength(s, offset, uint32(length), 262, "cmap format 0"); err != nil {
			return nil, err
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

	case 2:
		length, err := readUint16(s, offset+2)
		if err != nil {
			return nil, err
		}
		if err := checkCMapSubtableLength(s, offset, uint32(length), cmapFormat2MinSize, "cmap format 2"); err != nil {
			return nil, err
		}

		var subHeaderKeys [256]uint16
		var maxSubHeaderKey uint16
		for i := 0; i < 256; i++ {
			key, err := readUint16(s, offset+6+int64(i*2))
			if err != nil {
				return nil, err
			}
			if key%8 != 0 {
				return nil, errors.New("cmap format 2 subHeaderKey is not a subHeader offset")
			}
			subHeaderKeys[i] = key
			if key > maxSubHeaderKey {
				maxSubHeaderKey = key
			}
		}

		subHeaderCount := uint32(maxSubHeaderKey/8) + 1
		if !cmapItemsFit(uint32(length), cmapFormat2HeaderSize, 8, subHeaderCount) {
			return nil, errors.New("cmap format 2 too short for subHeaders")
		}
		subHeaderCountInt, err := cmapCountToInt(subHeaderCount)
		if err != nil {
			return nil, err
		}
		glyphArrayOffset := uint32(cmapFormat2HeaderSize) + subHeaderCount*8
		if (uint32(length)-glyphArrayOffset)%2 != 0 {
			return nil, errors.New("cmap format 2 glyph array has odd byte length")
		}

		m := &CMapFormat2{
			SubHeaderKeys: subHeaderKeys,
			SubHeaders:    make([]CMapFormat2SubHeader, subHeaderCountInt),
			GlyphIdArray:  make([]uint16, int((uint32(length)-glyphArrayOffset)/2)),
		}
		for i := 0; i < subHeaderCountInt; i++ {
			subHeaderOffset := offset + cmapFormat2HeaderSize + int64(i*8)
			subHeader := &m.SubHeaders[i]
			subHeader.FirstCode, err = readUint16(s, subHeaderOffset)
			if err != nil {
				return nil, err
			}
			subHeader.EntryCount, err = readUint16(s, subHeaderOffset+2)
			if err != nil {
				return nil, err
			}
			subHeader.IDDelta, err = readInt16(s, subHeaderOffset+4)
			if err != nil {
				return nil, err
			}
			subHeader.IDRangeOffset, err = readUint16(s, subHeaderOffset+6)
			if err != nil {
				return nil, err
			}
			if subHeader.EntryCount == 0 {
				continue
			}
			if uint32(subHeader.FirstCode)+uint32(subHeader.EntryCount) > 0x100 {
				return nil, errors.New("cmap format 2 subHeader byte range overflows")
			}
			if subHeader.IDRangeOffset%2 != 0 {
				return nil, errors.New("cmap format 2 idRangeOffset is not word aligned")
			}

			idRangeOffsetWordOffset := uint32(cmapFormat2HeaderSize + i*8 + 6)
			glyphStartOffset := idRangeOffsetWordOffset + uint32(subHeader.IDRangeOffset)
			if glyphStartOffset < glyphArrayOffset ||
				glyphStartOffset > uint32(length) ||
				uint32(length)-glyphStartOffset < uint32(subHeader.EntryCount)*2 {
				return nil, errors.New("cmap format 2 subHeader glyph range exceeds glyph array")
			}
			subHeader.glyphIdArrayIndex = int((glyphStartOffset - glyphArrayOffset) / 2)
		}
		for i := range m.GlyphIdArray {
			m.GlyphIdArray[i], err = readUint16(s, offset+int64(glyphArrayOffset)+int64(i*2))
			if err != nil {
				return nil, err
			}
		}
		return m, nil

	case 6:
		length, err := readUint16(s, offset+2)
		if err != nil {
			return nil, err
		}
		if err := checkCMapSubtableLength(s, offset, uint32(length), 10, "cmap format 6"); err != nil {
			return nil, err
		}
		firstCode, err := readUint16(s, offset+6)
		if err != nil {
			return nil, err
		}
		entryCount, err := readUint16(s, offset+8)
		if err != nil {
			return nil, err
		}
		if uint32(firstCode)+uint32(entryCount) > 0x10000 {
			return nil, errors.New("cmap format 6 code range overflows BMP")
		}
		if !cmapItemsFit(uint32(length), 10, 2, uint32(entryCount)) {
			return nil, errors.New("cmap format 6 too short for glyph array")
		}

		m := &CMapFormat6{
			FirstCode:    firstCode,
			GlyphIdArray: make([]uint16, int(entryCount)),
		}
		for i := 0; i < int(entryCount); i++ {
			m.GlyphIdArray[i], err = readUint16(s, offset+10+int64(i*2))
			if err != nil {
				return nil, err
			}
		}
		return m, nil

	case 4:
		length, err := readUint16(s, offset+2)
		if err != nil {
			return nil, err
		}
		if err := checkCMapSubtableLength(s, offset, uint32(length), 16, "cmap format 4"); err != nil {
			return nil, err
		}
		segCountX2, err := readUint16(s, offset+6)
		if err != nil {
			return nil, err
		}
		if segCountX2 == 0 || segCountX2%2 != 0 {
			return nil, fmt.Errorf("invalid cmap format 4 segment count: %d", segCountX2)
		}
		segCount := int(segCountX2 / 2)
		if !cmapItemsFit(uint32(length), 16, 8, uint32(segCount)) {
			return nil, errors.New("cmap format 4 too short for segment arrays")
		}

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

	case 8:
		length, err := readUint32(s, offset+4)
		if err != nil {
			return nil, err
		}
		if err := checkCMapSubtableLength(s, offset, length, cmapFormat8HeaderSize, "cmap format 8"); err != nil {
			return nil, err
		}
		var is32 [cmapFormat8Is32Size]byte
		if err := readExactAt(s, is32[:], offset+12); err != nil {
			return nil, err
		}
		numGroups, err := readUint32(s, offset+12+cmapFormat8Is32Size)
		if err != nil {
			return nil, err
		}
		if !cmapItemsFit(length, cmapFormat8HeaderSize, 12, numGroups) {
			return nil, errors.New("cmap format 8 too short for groups")
		}
		groupCount, err := cmapCountToInt(numGroups)
		if err != nil {
			return nil, err
		}

		m := &CMapFormat8{
			Is32:   is32,
			Groups: make([]CMapFormat12Group, groupCount),
		}
		for i := 0; i < groupCount; i++ {
			groupOffset := offset + cmapFormat8HeaderSize + int64(i*12)
			m.Groups[i].StartCharCode, err = readUint32(s, groupOffset)
			if err != nil {
				return nil, err
			}
			m.Groups[i].EndCharCode, err = readUint32(s, groupOffset+4)
			if err != nil {
				return nil, err
			}
			m.Groups[i].StartGlyphID, err = readUint32(s, groupOffset+8)
			if err != nil {
				return nil, err
			}
			if m.Groups[i].StartCharCode > m.Groups[i].EndCharCode {
				return nil, errors.New("cmap format 8 group has descending character range")
			}
			if i > 0 && m.Groups[i].StartCharCode <= m.Groups[i-1].EndCharCode {
				return nil, errors.New("cmap format 8 groups overlap or are unsorted")
			}
			if m.Groups[i].EndCharCode > maxUnicodeScalar {
				return nil, errors.New("cmap format 8 group exceeds Unicode range")
			}
			if err := validateCMapFormat8Group(&is32, m.Groups[i].StartCharCode, m.Groups[i].EndCharCode); err != nil {
				return nil, err
			}
			glyphIDMax := uint64(m.Groups[i].StartGlyphID) + uint64(m.Groups[i].EndCharCode-m.Groups[i].StartCharCode)
			if glyphIDMax > uint64(maxCMapGlyphID) {
				return nil, errors.New("cmap format 8 glyph range exceeds uint16 glyph IDs")
			}
		}
		return m, nil

	case 10:
		length, err := readUint32(s, offset+4)
		if err != nil {
			return nil, err
		}
		if err := checkCMapSubtableLength(s, offset, length, 20, "cmap format 10"); err != nil {
			return nil, err
		}
		startCharCode, err := readUint32(s, offset+12)
		if err != nil {
			return nil, err
		}
		numChars, err := readUint32(s, offset+16)
		if err != nil {
			return nil, err
		}
		if numChars > 0 && startCharCode > ^uint32(0)-numChars+1 {
			return nil, errors.New("cmap format 10 code range overflows uint32")
		}
		if !cmapItemsFit(length, 20, 2, numChars) {
			return nil, errors.New("cmap format 10 too short for glyph array")
		}
		count, err := cmapCountToInt(numChars)
		if err != nil {
			return nil, err
		}

		m := &CMapFormat10{
			StartCharCode: startCharCode,
			GlyphIdArray:  make([]uint16, count),
		}
		for i := 0; i < count; i++ {
			m.GlyphIdArray[i], err = readUint16(s, offset+20+int64(i*2))
			if err != nil {
				return nil, err
			}
		}
		return m, nil

	case 12:
		length, err := readUint32(s, offset+4)
		if err != nil {
			return nil, err
		}
		if err := checkCMapSubtableLength(s, offset, length, 16, "cmap format 12"); err != nil {
			return nil, err
		}
		numGroups, err := readUint32(s, offset+12)
		if err != nil {
			return nil, err
		}
		if !cmapItemsFit(length, 16, 12, numGroups) {
			return nil, errors.New("cmap format 12 too short for groups")
		}
		groupCount, err := cmapCountToInt(numGroups)
		if err != nil {
			return nil, err
		}

		m := &CMapFormat12{
			Groups: make([]CMapFormat12Group, groupCount),
		}
		for i := 0; i < groupCount; i++ {
			groupOffset := offset + 16 + int64(i*12)
			m.Groups[i].StartCharCode, err = readUint32(s, groupOffset)
			if err != nil {
				return nil, err
			}
			m.Groups[i].EndCharCode, err = readUint32(s, groupOffset+4)
			if err != nil {
				return nil, err
			}
			m.Groups[i].StartGlyphID, err = readUint32(s, groupOffset+8)
			if err != nil {
				return nil, err
			}
			if m.Groups[i].StartCharCode > m.Groups[i].EndCharCode {
				return nil, errors.New("cmap format 12 group has descending character range")
			}
			if i > 0 && m.Groups[i].StartCharCode <= m.Groups[i-1].EndCharCode {
				return nil, errors.New("cmap format 12 groups overlap or are unsorted")
			}
			glyphIDMax := uint64(m.Groups[i].StartGlyphID) + uint64(m.Groups[i].EndCharCode-m.Groups[i].StartCharCode)
			if glyphIDMax > uint64(maxCMapGlyphID) {
				return nil, errors.New("cmap format 12 glyph range exceeds uint16 glyph IDs")
			}
		}
		return m, nil

	case 13:
		length, err := readUint32(s, offset+4)
		if err != nil {
			return nil, err
		}
		if err := checkCMapSubtableLength(s, offset, length, 16, "cmap format 13"); err != nil {
			return nil, err
		}
		numGroups, err := readUint32(s, offset+12)
		if err != nil {
			return nil, err
		}
		if !cmapItemsFit(length, 16, 12, numGroups) {
			return nil, errors.New("cmap format 13 too short for groups")
		}
		groupCount, err := cmapCountToInt(numGroups)
		if err != nil {
			return nil, err
		}

		m := &CMapFormat13{
			Groups: make([]CMapFormat13Group, groupCount),
		}
		for i := 0; i < groupCount; i++ {
			groupOffset := offset + 16 + int64(i*12)
			m.Groups[i].StartCharCode, err = readUint32(s, groupOffset)
			if err != nil {
				return nil, err
			}
			m.Groups[i].EndCharCode, err = readUint32(s, groupOffset+4)
			if err != nil {
				return nil, err
			}
			m.Groups[i].GlyphID, err = readUint32(s, groupOffset+8)
			if err != nil {
				return nil, err
			}
			if m.Groups[i].StartCharCode > m.Groups[i].EndCharCode {
				return nil, errors.New("cmap format 13 group has descending character range")
			}
			if i > 0 && m.Groups[i].StartCharCode <= m.Groups[i-1].EndCharCode {
				return nil, errors.New("cmap format 13 groups overlap or are unsorted")
			}
			if m.Groups[i].GlyphID > maxCMapGlyphID {
				return nil, errors.New("cmap format 13 glyph ID exceeds uint16")
			}
		}
		return m, nil

	case 14:
		length, err := readUint32(s, offset+2)
		if err != nil {
			return nil, err
		}
		if err := checkCMapSubtableLength(s, offset, length, 10, "cmap format 14"); err != nil {
			return nil, err
		}
		numRecords, err := readUint32(s, offset+6)
		if err != nil {
			return nil, err
		}
		if !cmapItemsFit(length, 10, 11, numRecords) {
			return nil, errors.New("cmap format 14 too short for variation selector records")
		}
		recordCount, err := cmapCountToInt(numRecords)
		if err != nil {
			return nil, err
		}

		m := &CMapFormat14{
			VariationSelectors: make([]CMapVariationSelectorRecord, recordCount),
		}
		var previousSelector uint32
		for i := 0; i < recordCount; i++ {
			recordOffset := offset + 10 + int64(i*11)
			selector, err := readUint24(s, recordOffset)
			if err != nil {
				return nil, err
			}
			if selector > maxUnicodeScalar {
				return nil, errors.New("cmap format 14 variation selector exceeds Unicode range")
			}
			if i > 0 && selector <= previousSelector {
				return nil, errors.New("cmap format 14 variation selectors overlap or are unsorted")
			}
			previousSelector = selector

			defaultOffset, err := readUint32(s, recordOffset+3)
			if err != nil {
				return nil, err
			}
			nonDefaultOffset, err := readUint32(s, recordOffset+7)
			if err != nil {
				return nil, err
			}

			record := &m.VariationSelectors[i]
			record.VarSelector = selector
			if defaultOffset != 0 {
				record.DefaultUVS, err = parseCMap14DefaultUVS(s, offset, length, defaultOffset)
				if err != nil {
					return nil, err
				}
			}
			if nonDefaultOffset != 0 {
				record.NonDefaultUVS, err = parseCMap14NonDefaultUVS(s, offset, length, nonDefaultOffset)
				if err != nil {
					return nil, err
				}
			}
		}
		return m, nil
	}

	return nil, fmt.Errorf("unsupported cmap format: %d", format)
}

func cmapFormat8Is32(is32 *[cmapFormat8Is32Size]byte, word uint16) bool {
	return is32[int(word)/8]&byte(1<<(7-(word%8))) != 0
}

func validateCMapFormat8Group(is32 *[cmapFormat8Is32Size]byte, start uint32, end uint32) error {
	if start <= 0xffff {
		if end > 0xffff {
			return errors.New("cmap format 8 group mixes 16-bit and 32-bit character codes")
		}
		for c := start; c <= end; c++ {
			if cmapFormat8Is32(is32, uint16(c)) {
				return errors.New("cmap format 8 maps 16-bit code marked as 32-bit lead")
			}
		}
		return nil
	}

	startHigh := start >> 16
	endHigh := end >> 16
	for high := startHigh; high <= endHigh; high++ {
		if !cmapFormat8Is32(is32, uint16(high)) {
			return errors.New("cmap format 8 missing 32-bit lead marker")
		}
	}
	return nil
}

func parseCMap14DefaultUVS(s api.Stream, cmapOffset int64, cmapLength uint32, tableOffset uint32) ([]CMapDefaultUVSRange, error) {
	offset, available, err := checkedCMap14SubtableOffset(cmapOffset, cmapLength, tableOffset, 4, "cmap format 14 default UVS table")
	if err != nil {
		return nil, err
	}
	numRanges, err := readUint32(s, offset)
	if err != nil {
		return nil, err
	}
	if !cmapItemsFit(available, 4, 4, numRanges) {
		return nil, errors.New("cmap format 14 default UVS table too short for ranges")
	}
	rangeCount, err := cmapCountToInt(numRanges)
	if err != nil {
		return nil, err
	}

	ranges := make([]CMapDefaultUVSRange, rangeCount)
	var previousEnd uint32
	for i := 0; i < rangeCount; i++ {
		rangeOffset := offset + 4 + int64(i*4)
		start, err := readUint24(s, rangeOffset)
		if err != nil {
			return nil, err
		}
		additionalCount, err := readByte(s, rangeOffset+3)
		if err != nil {
			return nil, err
		}
		end := start + uint32(additionalCount)
		if start > maxUnicodeScalar || end > maxUnicodeScalar {
			return nil, errors.New("cmap format 14 default UVS range exceeds Unicode range")
		}
		if i > 0 && start <= previousEnd {
			return nil, errors.New("cmap format 14 default UVS ranges overlap or are unsorted")
		}
		previousEnd = end
		ranges[i] = CMapDefaultUVSRange{
			StartUnicodeValue: start,
			AdditionalCount:   additionalCount,
		}
	}
	return ranges, nil
}

func parseCMap14NonDefaultUVS(s api.Stream, cmapOffset int64, cmapLength uint32, tableOffset uint32) ([]CMapNonDefaultUVSMapping, error) {
	offset, available, err := checkedCMap14SubtableOffset(cmapOffset, cmapLength, tableOffset, 4, "cmap format 14 non-default UVS table")
	if err != nil {
		return nil, err
	}
	numMappings, err := readUint32(s, offset)
	if err != nil {
		return nil, err
	}
	if !cmapItemsFit(available, 4, 5, numMappings) {
		return nil, errors.New("cmap format 14 non-default UVS table too short for mappings")
	}
	mappingCount, err := cmapCountToInt(numMappings)
	if err != nil {
		return nil, err
	}

	mappings := make([]CMapNonDefaultUVSMapping, mappingCount)
	var previousUnicode uint32
	for i := 0; i < mappingCount; i++ {
		mappingOffset := offset + 4 + int64(i*5)
		unicodeValue, err := readUint24(s, mappingOffset)
		if err != nil {
			return nil, err
		}
		if unicodeValue > maxUnicodeScalar {
			return nil, errors.New("cmap format 14 non-default UVS mapping exceeds Unicode range")
		}
		if i > 0 && unicodeValue <= previousUnicode {
			return nil, errors.New("cmap format 14 non-default UVS mappings overlap or are unsorted")
		}
		previousUnicode = unicodeValue

		glyphID, err := readUint16(s, mappingOffset+3)
		if err != nil {
			return nil, err
		}
		mappings[i] = CMapNonDefaultUVSMapping{
			UnicodeValue: unicodeValue,
			GlyphID:      glyphID,
		}
	}
	return mappings, nil
}

func checkedCMap14SubtableOffset(cmapOffset int64, cmapLength uint32, tableOffset uint32, minLength uint32, name string) (int64, uint32, error) {
	if tableOffset > cmapLength || cmapLength-tableOffset < minLength {
		return 0, 0, fmt.Errorf("%s too short", name)
	}
	return cmapOffset + int64(tableOffset), cmapLength - tableOffset, nil
}

func checkCMapSubtableLength(s api.Stream, offset int64, length uint32, minLength uint32, name string) error {
	if length < minLength {
		return fmt.Errorf("%s too short", name)
	}
	end := offset + int64(length)
	if offset < 0 || end < offset || s.Size() < end {
		return fmt.Errorf("%s too short", name)
	}
	return nil
}

func checkTableRange(s api.Stream, offset int64, length int64, name string) error {
	if offset < 0 || length < 0 || offset > s.Size() || s.Size()-offset < length {
		return fmt.Errorf("%s too short", name)
	}
	return nil
}

func cmapItemsFit(length uint32, headerSize uint32, itemSize uint32, count uint32) bool {
	if length < headerSize {
		return false
	}
	return uint64(count)*uint64(itemSize) <= uint64(length-headerSize)
}

func cmapCountToInt(count uint32) (int, error) {
	if uint64(count) > uint64(int(^uint(0)>>1)) {
		return 0, errors.New("cmap subtable item count too large")
	}
	return int(count), nil
}

func readUint24(s api.Stream, off int64) (uint32, error) {
	var buf [3]byte
	if err := readExactAt(s, buf[:], off); err != nil {
		return 0, err
	}
	return uint32(buf[0])<<16 | uint32(buf[1])<<8 | uint32(buf[2]), nil
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
	AdvanceHeightMax     uint16
	MinTopSideBearing    int16
	MinBottomSideBearing int16
	YMaxExtent           int16
	CaretSlopeRise       int16
	CaretSlopeRun        int16
	CaretOffset          int16
	Reserved             [4]int16
	MetricDataFormat     int16
	NumOfLongVerMetrics  uint16

	advanceHeightMax     int16
	minTopSideBearing    int16
	minBottomSideBearing int16
	yMaxExtent           int16
	caretSlopeRise       int16
	caretSlopeRun        int16
	caretOffset          int16
	metricDataFormat     int16
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
	h.AdvanceHeightMax, err = readUint16(s, 10)
	if err != nil {
		return h, err
	}
	h.advanceHeightMax = int16(h.AdvanceHeightMax)
	h.MinTopSideBearing, err = readInt16(s, 12)
	if err != nil {
		return h, err
	}
	h.minTopSideBearing = h.MinTopSideBearing
	h.MinBottomSideBearing, err = readInt16(s, 14)
	if err != nil {
		return h, err
	}
	h.minBottomSideBearing = h.MinBottomSideBearing
	h.YMaxExtent, err = readInt16(s, 16)
	if err != nil {
		return h, err
	}
	h.yMaxExtent = h.YMaxExtent
	h.CaretSlopeRise, err = readInt16(s, 18)
	if err != nil {
		return h, err
	}
	h.caretSlopeRise = h.CaretSlopeRise
	h.CaretSlopeRun, err = readInt16(s, 20)
	if err != nil {
		return h, err
	}
	h.caretSlopeRun = h.CaretSlopeRun
	h.CaretOffset, err = readInt16(s, 22)
	if err != nil {
		return h, err
	}
	h.caretOffset = h.CaretOffset
	for i := range h.Reserved {
		h.Reserved[i], err = readInt16(s, 24+int64(i*2))
		if err != nil {
			return h, err
		}
	}
	h.MetricDataFormat, err = readInt16(s, 32)
	if err != nil {
		return h, err
	}
	h.metricDataFormat = h.MetricDataFormat
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
	MajorVersion       uint16
	MinorVersion       uint16
	HorizAxisOffset    uint16
	VertAxisOffset     uint16
	ItemVarStoreOffset uint32
	HorizAxis          *BASEAxis
	VertAxis           *BASEAxis
}

type BASEAxis struct {
	BaseTagListOffset    uint16
	BaseScriptListOffset uint16
	BaseTags             []uint32
	BaseScriptRecords    []BASEScriptRecord
}

type BASEScriptRecord struct {
	BaseScriptTag    uint32
	BaseScriptOffset uint16
	BaseScript       BASEScript
}

type BASEScript struct {
	BaseValuesOffset    uint16
	DefaultMinMaxOffset uint16
	BaseLangSysCount    uint16
	BaseLangSysRecords  []BASELangSysRecord
	BaseValues          *BASEValues
	DefaultMinMax       *BASEMinMax
}

type BASELangSysRecord struct {
	BaseLangSysTag uint32
	MinMaxOffset   uint16
	MinMax         *BASEMinMax
}

type BASEValues struct {
	DefaultBaselineIndex uint16
	BaseCoordCount       uint16
	BaseCoordOffsets     []uint16
	BaseCoords           []*BASECoord
}

type BASECoord struct {
	Format         uint16
	Coordinate     int16
	ReferenceGlyph uint16
	BaseCoordPoint uint16
	DeviceOffset   uint16
	Device         *BASEDevice
}

type BASEDevice struct {
	StartSize          uint16
	EndSize            uint16
	DeltaFormat        uint16
	DeltaValues        []int16
	DeltaSetOuterIndex uint16
	DeltaSetInnerIndex uint16
}

type BASEMinMax struct {
	MinCoordOffset    uint16
	MaxCoordOffset    uint16
	FeatMinMaxCount   uint16
	MinCoord          *BASECoord
	MaxCoord          *BASECoord
	FeatMinMaxRecords []BASEFeatMinMaxRecord
}

type BASEFeatMinMaxRecord struct {
	FeatureTag     uint32
	MinCoordOffset uint16
	MaxCoordOffset uint16
	MinCoord       *BASECoord
	MaxCoord       *BASECoord
}

func ParseBASE(s api.Stream) (BASETable, error) {
	var t BASETable
	var err error
	if s.Size() < 8 {
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
	t.HorizAxisOffset, err = readUint16(s, 4)
	if err != nil {
		return t, err
	}
	t.VertAxisOffset, err = readUint16(s, 6)
	if err != nil {
		return t, err
	}
	if t.MinorVersion >= 1 {
		if s.Size() < 12 {
			return t, errors.New("BASE version 1.1 table too short")
		}
		t.ItemVarStoreOffset, err = readUint32(s, 8)
		if err != nil {
			return t, err
		}
	}
	if t.HorizAxisOffset != 0 {
		t.HorizAxis, err = parseBASEAxis(s, int64(t.HorizAxisOffset))
		if err != nil {
			return t, err
		}
	}
	if t.VertAxisOffset != 0 {
		t.VertAxis, err = parseBASEAxis(s, int64(t.VertAxisOffset))
		if err != nil {
			return t, err
		}
	}
	return t, nil
}

func parseBASEAxis(s api.Stream, offset int64) (*BASEAxis, error) {
	if err := checkTableRange(s, offset, 4, "BASE Axis table"); err != nil {
		return nil, err
	}
	axis := &BASEAxis{}
	var err error
	axis.BaseTagListOffset, err = readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	axis.BaseScriptListOffset, err = readUint16(s, offset+2)
	if err != nil {
		return nil, err
	}
	if axis.BaseTagListOffset != 0 {
		axis.BaseTags, err = parseBASETagList(s, offset+int64(axis.BaseTagListOffset))
		if err != nil {
			return nil, err
		}
	}
	if axis.BaseScriptListOffset == 0 {
		return nil, errors.New("BASE Axis missing BaseScriptList")
	}
	axis.BaseScriptRecords, err = parseBASEScriptList(s, offset+int64(axis.BaseScriptListOffset))
	if err != nil {
		return nil, err
	}
	return axis, nil
}

func parseBASETagList(s api.Stream, offset int64) ([]uint32, error) {
	if err := checkTableRange(s, offset, 2, "BASE BaseTagList table"); err != nil {
		return nil, err
	}
	count, err := readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	if err := checkTableRange(s, offset, 2+int64(count)*4, "BASE BaseTagList table"); err != nil {
		return nil, err
	}
	tags := make([]uint32, int(count))
	for i := 0; i < int(count); i++ {
		tags[i], err = readUint32(s, offset+2+int64(i*4))
		if err != nil {
			return nil, err
		}
	}
	return tags, nil
}

func parseBASEScriptList(s api.Stream, offset int64) ([]BASEScriptRecord, error) {
	if err := checkTableRange(s, offset, 2, "BASE BaseScriptList table"); err != nil {
		return nil, err
	}
	count, err := readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	if err := checkTableRange(s, offset, 2+int64(count)*6, "BASE BaseScriptList table"); err != nil {
		return nil, err
	}
	records := make([]BASEScriptRecord, int(count))
	for i := 0; i < int(count); i++ {
		recordOffset := offset + 2 + int64(i*6)
		records[i].BaseScriptTag, err = readUint32(s, recordOffset)
		if err != nil {
			return nil, err
		}
		records[i].BaseScriptOffset, err = readUint16(s, recordOffset+4)
		if err != nil {
			return nil, err
		}
		if records[i].BaseScriptOffset == 0 {
			return nil, errors.New("BASE BaseScriptRecord has null BaseScript offset")
		}
		records[i].BaseScript, err = parseBASEScript(s, offset+int64(records[i].BaseScriptOffset))
		if err != nil {
			return nil, err
		}
	}
	return records, nil
}

func parseBASEScript(s api.Stream, offset int64) (BASEScript, error) {
	var script BASEScript
	if err := checkTableRange(s, offset, 6, "BASE BaseScript table"); err != nil {
		return script, err
	}
	var err error
	script.BaseValuesOffset, err = readUint16(s, offset)
	if err != nil {
		return script, err
	}
	script.DefaultMinMaxOffset, err = readUint16(s, offset+2)
	if err != nil {
		return script, err
	}
	script.BaseLangSysCount, err = readUint16(s, offset+4)
	if err != nil {
		return script, err
	}
	if err := checkTableRange(s, offset, 6+int64(script.BaseLangSysCount)*6, "BASE BaseScript table"); err != nil {
		return script, err
	}
	if script.BaseValuesOffset != 0 {
		values, err := parseBASEValues(s, offset+int64(script.BaseValuesOffset))
		if err != nil {
			return script, err
		}
		script.BaseValues = values
	}
	if script.DefaultMinMaxOffset != 0 {
		minMax, err := parseBASEMinMax(s, offset+int64(script.DefaultMinMaxOffset))
		if err != nil {
			return script, err
		}
		script.DefaultMinMax = minMax
	}
	script.BaseLangSysRecords = make([]BASELangSysRecord, int(script.BaseLangSysCount))
	for i := 0; i < int(script.BaseLangSysCount); i++ {
		recordOffset := offset + 6 + int64(i*6)
		script.BaseLangSysRecords[i].BaseLangSysTag, err = readUint32(s, recordOffset)
		if err != nil {
			return script, err
		}
		script.BaseLangSysRecords[i].MinMaxOffset, err = readUint16(s, recordOffset+4)
		if err != nil {
			return script, err
		}
		if script.BaseLangSysRecords[i].MinMaxOffset == 0 {
			return script, errors.New("BASE BaseLangSysRecord has null MinMax offset")
		}
		minMax, err := parseBASEMinMax(s, offset+int64(script.BaseLangSysRecords[i].MinMaxOffset))
		if err != nil {
			return script, err
		}
		script.BaseLangSysRecords[i].MinMax = minMax
	}
	return script, nil
}

func parseBASEValues(s api.Stream, offset int64) (*BASEValues, error) {
	if err := checkTableRange(s, offset, 4, "BASE BaseValues table"); err != nil {
		return nil, err
	}
	values := &BASEValues{}
	var err error
	values.DefaultBaselineIndex, err = readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	values.BaseCoordCount, err = readUint16(s, offset+2)
	if err != nil {
		return nil, err
	}
	if values.BaseCoordCount > 0 && values.DefaultBaselineIndex >= values.BaseCoordCount {
		return nil, errors.New("BASE BaseValues default baseline index out of range")
	}
	if err := checkTableRange(s, offset, 4+int64(values.BaseCoordCount)*2, "BASE BaseValues table"); err != nil {
		return nil, err
	}
	values.BaseCoordOffsets = make([]uint16, int(values.BaseCoordCount))
	values.BaseCoords = make([]*BASECoord, int(values.BaseCoordCount))
	for i := 0; i < int(values.BaseCoordCount); i++ {
		values.BaseCoordOffsets[i], err = readUint16(s, offset+4+int64(i*2))
		if err != nil {
			return nil, err
		}
		if values.BaseCoordOffsets[i] == 0 {
			continue
		}
		coord, err := parseBASECoord(s, offset+int64(values.BaseCoordOffsets[i]))
		if err != nil {
			return nil, err
		}
		values.BaseCoords[i] = coord
	}
	return values, nil
}

func parseBASECoord(s api.Stream, offset int64) (*BASECoord, error) {
	if err := checkTableRange(s, offset, 4, "BASE BaseCoord table"); err != nil {
		return nil, err
	}
	coord := &BASECoord{}
	var err error
	coord.Format, err = readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	coord.Coordinate, err = readInt16(s, offset+2)
	if err != nil {
		return nil, err
	}
	switch coord.Format {
	case 1:
		return coord, nil
	case 2:
		if err := checkTableRange(s, offset, 8, "BASE BaseCoord format 2 table"); err != nil {
			return nil, err
		}
		coord.ReferenceGlyph, err = readUint16(s, offset+4)
		if err != nil {
			return nil, err
		}
		coord.BaseCoordPoint, err = readUint16(s, offset+6)
		if err != nil {
			return nil, err
		}
		return coord, nil
	case 3:
		if err := checkTableRange(s, offset, 6, "BASE BaseCoord format 3 table"); err != nil {
			return nil, err
		}
		coord.DeviceOffset, err = readUint16(s, offset+4)
		if err != nil {
			return nil, err
		}
		if coord.DeviceOffset != 0 {
			coord.Device, err = parseBASEDevice(s, offset+int64(coord.DeviceOffset))
			if err != nil {
				return nil, err
			}
		}
		return coord, nil
	default:
		return nil, fmt.Errorf("unsupported BASE BaseCoord format: %d", coord.Format)
	}
}

func parseBASEDevice(s api.Stream, offset int64) (*BASEDevice, error) {
	if err := checkTableRange(s, offset, 6, "BASE Device table"); err != nil {
		return nil, err
	}
	device := &BASEDevice{}
	var err error
	device.StartSize, err = readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	device.EndSize, err = readUint16(s, offset+2)
	if err != nil {
		return nil, err
	}
	device.DeltaFormat, err = readUint16(s, offset+4)
	if err != nil {
		return nil, err
	}

	if device.DeltaFormat == 0x8000 {
		device.DeltaSetOuterIndex = device.StartSize
		device.DeltaSetInnerIndex = device.EndSize
		return device, nil
	}
	if device.EndSize < device.StartSize {
		return nil, errors.New("BASE Device table has descending size range")
	}

	bitsPerDelta := 0
	switch device.DeltaFormat {
	case 1:
		bitsPerDelta = 2
	case 2:
		bitsPerDelta = 4
	case 3:
		bitsPerDelta = 8
	default:
		return nil, fmt.Errorf("unsupported BASE Device delta format: %d", device.DeltaFormat)
	}

	count := int(device.EndSize-device.StartSize) + 1
	words := (count*bitsPerDelta + 15) / 16
	if err := checkTableRange(s, offset+6, int64(words*2), "BASE Device delta values"); err != nil {
		return nil, err
	}
	device.DeltaValues = make([]int16, count)
	for i := 0; i < count; i++ {
		bitOffset := i * bitsPerDelta
		word, err := readUint16(s, offset+6+int64(bitOffset/16)*2)
		if err != nil {
			return nil, err
		}
		shift := 16 - bitsPerDelta - bitOffset%16
		raw := int16((word >> shift) & uint16((1<<bitsPerDelta)-1))
		signBit := int16(1 << (bitsPerDelta - 1))
		if raw&signBit != 0 {
			raw -= int16(1 << bitsPerDelta)
		}
		device.DeltaValues[i] = raw
	}
	return device, nil
}

func parseBASEMinMax(s api.Stream, offset int64) (*BASEMinMax, error) {
	if err := checkTableRange(s, offset, 6, "BASE MinMax table"); err != nil {
		return nil, err
	}
	minMax := &BASEMinMax{}
	var err error
	minMax.MinCoordOffset, err = readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	minMax.MaxCoordOffset, err = readUint16(s, offset+2)
	if err != nil {
		return nil, err
	}
	minMax.FeatMinMaxCount, err = readUint16(s, offset+4)
	if err != nil {
		return nil, err
	}
	if err := checkTableRange(s, offset, 6+int64(minMax.FeatMinMaxCount)*8, "BASE MinMax table"); err != nil {
		return nil, err
	}
	if minMax.MinCoordOffset != 0 {
		minMax.MinCoord, err = parseBASECoord(s, offset+int64(minMax.MinCoordOffset))
		if err != nil {
			return nil, err
		}
	}
	if minMax.MaxCoordOffset != 0 {
		minMax.MaxCoord, err = parseBASECoord(s, offset+int64(minMax.MaxCoordOffset))
		if err != nil {
			return nil, err
		}
	}
	minMax.FeatMinMaxRecords = make([]BASEFeatMinMaxRecord, int(minMax.FeatMinMaxCount))
	for i := 0; i < int(minMax.FeatMinMaxCount); i++ {
		recordOffset := offset + 6 + int64(i*8)
		minMax.FeatMinMaxRecords[i].FeatureTag, err = readUint32(s, recordOffset)
		if err != nil {
			return nil, err
		}
		minMax.FeatMinMaxRecords[i].MinCoordOffset, err = readUint16(s, recordOffset+4)
		if err != nil {
			return nil, err
		}
		minMax.FeatMinMaxRecords[i].MaxCoordOffset, err = readUint16(s, recordOffset+6)
		if err != nil {
			return nil, err
		}
		if minMax.FeatMinMaxRecords[i].MinCoordOffset != 0 {
			minMax.FeatMinMaxRecords[i].MinCoord, err = parseBASECoord(s, offset+int64(minMax.FeatMinMaxRecords[i].MinCoordOffset))
			if err != nil {
				return nil, err
			}
		}
		if minMax.FeatMinMaxRecords[i].MaxCoordOffset != 0 {
			minMax.FeatMinMaxRecords[i].MaxCoord, err = parseBASECoord(s, offset+int64(minMax.FeatMinMaxRecords[i].MaxCoordOffset))
			if err != nil {
				return nil, err
			}
		}
	}
	return minMax, nil
}

type JstfTable struct {
	MajorVersion    uint16
	MinorVersion    uint16
	JstfScriptCount uint16
	JstfScripts     []JstfScriptRecord
}

type JstfScriptRecord struct {
	JstfScriptTag    uint32
	JstfScriptOffset uint16
	JstfScript       *JstfScript
}

type JstfScript struct {
	ExtenderGlyphOffset  uint16
	DefJstfLangSysOffset uint16
	JstfLangSysCount     uint16
	ExtenderGlyphs       []uint16
	DefJstfLangSys       *JstfLangSys
	JstfLangSysRecords   []JstfLangSysRecord
}

type JstfLangSysRecord struct {
	JstfLangSysTag    uint32
	JstfLangSysOffset uint16
	JstfLangSys       *JstfLangSys
}

type JstfLangSys struct {
	JstfPriorityCount   uint16
	JstfPriorityOffsets []uint16
	JstfPriorities      []*JstfPriority
}

type JstfPriority struct {
	GsubShrinkageEnableOffset  uint16
	GsubShrinkageDisableOffset uint16
	GposShrinkageEnableOffset  uint16
	GposShrinkageDisableOffset uint16
	ShrinkageJstfMaxOffset     uint16
	GsubExtensionEnableOffset  uint16
	GsubExtensionDisableOffset uint16
	GposExtensionEnableOffset  uint16
	GposExtensionDisableOffset uint16
	ExtensionJstfMaxOffset     uint16
	GsubShrinkageEnable        *JstfModList
	GsubShrinkageDisable       *JstfModList
	GposShrinkageEnable        *JstfModList
	GposShrinkageDisable       *JstfModList
	ShrinkageJstfMax           *JstfMax
	GsubExtensionEnable        *JstfModList
	GsubExtensionDisable       *JstfModList
	GposExtensionEnable        *JstfModList
	GposExtensionDisable       *JstfModList
	ExtensionJstfMax           *JstfMax
}

type JstfModList struct {
	LookupCount   uint16
	LookupIndices []uint16
}

type JstfMax struct {
	LookupCount   uint16
	LookupOffsets []uint16
}

func ParseJstf(s api.Stream) (JstfTable, error) {
	var t JstfTable
	var err error
	if s.Size() < 6 {
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
	t.JstfScriptCount, err = readUint16(s, 4)
	if err != nil {
		return t, err
	}
	if err := checkTableRange(s, 0, 6+int64(t.JstfScriptCount)*6, "Jstf table"); err != nil {
		return t, err
	}

	t.JstfScripts = make([]JstfScriptRecord, int(t.JstfScriptCount))
	for i := 0; i < int(t.JstfScriptCount); i++ {
		recordOffset := int64(6 + i*6)
		t.JstfScripts[i].JstfScriptTag, err = readUint32(s, recordOffset)
		if err != nil {
			return t, err
		}
		t.JstfScripts[i].JstfScriptOffset, err = readUint16(s, recordOffset+4)
		if err != nil {
			return t, err
		}
		if t.JstfScripts[i].JstfScriptOffset == 0 {
			return t, errors.New("JstfScriptRecord has null script offset")
		}
		script, err := parseJstfScript(s, int64(t.JstfScripts[i].JstfScriptOffset))
		if err != nil {
			return t, err
		}
		t.JstfScripts[i].JstfScript = script
	}
	return t, nil
}

func parseJstfScript(s api.Stream, offset int64) (*JstfScript, error) {
	if err := checkTableRange(s, offset, 6, "JstfScript table"); err != nil {
		return nil, err
	}
	script := &JstfScript{}
	var err error
	script.ExtenderGlyphOffset, err = readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	script.DefJstfLangSysOffset, err = readUint16(s, offset+2)
	if err != nil {
		return nil, err
	}
	script.JstfLangSysCount, err = readUint16(s, offset+4)
	if err != nil {
		return nil, err
	}
	if err := checkTableRange(s, offset, 6+int64(script.JstfLangSysCount)*6, "JstfScript table"); err != nil {
		return nil, err
	}
	if script.ExtenderGlyphOffset != 0 {
		script.ExtenderGlyphs, err = parseJstfExtenderGlyphs(s, offset+int64(script.ExtenderGlyphOffset))
		if err != nil {
			return nil, err
		}
	}
	if script.DefJstfLangSysOffset != 0 {
		script.DefJstfLangSys, err = parseJstfLangSys(s, offset+int64(script.DefJstfLangSysOffset))
		if err != nil {
			return nil, err
		}
	}

	script.JstfLangSysRecords = make([]JstfLangSysRecord, int(script.JstfLangSysCount))
	for i := 0; i < int(script.JstfLangSysCount); i++ {
		recordOffset := offset + 6 + int64(i*6)
		record := &script.JstfLangSysRecords[i]
		record.JstfLangSysTag, err = readUint32(s, recordOffset)
		if err != nil {
			return nil, err
		}
		record.JstfLangSysOffset, err = readUint16(s, recordOffset+4)
		if err != nil {
			return nil, err
		}
		if record.JstfLangSysOffset == 0 {
			return nil, errors.New("JstfLangSysRecord has null language-system offset")
		}
		record.JstfLangSys, err = parseJstfLangSys(s, offset+int64(record.JstfLangSysOffset))
		if err != nil {
			return nil, err
		}
	}
	return script, nil
}

func parseJstfExtenderGlyphs(s api.Stream, offset int64) ([]uint16, error) {
	if err := checkTableRange(s, offset, 2, "Jstf ExtenderGlyph table"); err != nil {
		return nil, err
	}
	count, err := readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	if err := checkTableRange(s, offset, 2+int64(count)*2, "Jstf ExtenderGlyph table"); err != nil {
		return nil, err
	}
	glyphs := make([]uint16, int(count))
	for i := range glyphs {
		glyphs[i], err = readUint16(s, offset+2+int64(i*2))
		if err != nil {
			return nil, err
		}
	}
	return glyphs, nil
}

func parseJstfLangSys(s api.Stream, offset int64) (*JstfLangSys, error) {
	if err := checkTableRange(s, offset, 2, "JstfLangSys table"); err != nil {
		return nil, err
	}
	langSys := &JstfLangSys{}
	var err error
	langSys.JstfPriorityCount, err = readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	if err := checkTableRange(s, offset, 2+int64(langSys.JstfPriorityCount)*2, "JstfLangSys table"); err != nil {
		return nil, err
	}
	langSys.JstfPriorityOffsets = make([]uint16, int(langSys.JstfPriorityCount))
	langSys.JstfPriorities = make([]*JstfPriority, int(langSys.JstfPriorityCount))
	for i := 0; i < int(langSys.JstfPriorityCount); i++ {
		langSys.JstfPriorityOffsets[i], err = readUint16(s, offset+2+int64(i*2))
		if err != nil {
			return nil, err
		}
		if langSys.JstfPriorityOffsets[i] == 0 {
			continue
		}
		priority, err := parseJstfPriority(s, offset+int64(langSys.JstfPriorityOffsets[i]))
		if err != nil {
			return nil, err
		}
		langSys.JstfPriorities[i] = priority
	}
	return langSys, nil
}

func parseJstfPriority(s api.Stream, offset int64) (*JstfPriority, error) {
	if err := checkTableRange(s, offset, 20, "JstfPriority table"); err != nil {
		return nil, err
	}
	priority := &JstfPriority{}
	offsets := []*uint16{
		&priority.GsubShrinkageEnableOffset,
		&priority.GsubShrinkageDisableOffset,
		&priority.GposShrinkageEnableOffset,
		&priority.GposShrinkageDisableOffset,
		&priority.ShrinkageJstfMaxOffset,
		&priority.GsubExtensionEnableOffset,
		&priority.GsubExtensionDisableOffset,
		&priority.GposExtensionEnableOffset,
		&priority.GposExtensionDisableOffset,
		&priority.ExtensionJstfMaxOffset,
	}
	for i, out := range offsets {
		v, err := readUint16(s, offset+int64(i*2))
		if err != nil {
			return nil, err
		}
		*out = v
	}

	var err error
	if priority.GsubShrinkageEnable, err = parseOptionalJstfModList(s, offset, priority.GsubShrinkageEnableOffset); err != nil {
		return nil, err
	}
	if priority.GsubShrinkageDisable, err = parseOptionalJstfModList(s, offset, priority.GsubShrinkageDisableOffset); err != nil {
		return nil, err
	}
	if priority.GposShrinkageEnable, err = parseOptionalJstfModList(s, offset, priority.GposShrinkageEnableOffset); err != nil {
		return nil, err
	}
	if priority.GposShrinkageDisable, err = parseOptionalJstfModList(s, offset, priority.GposShrinkageDisableOffset); err != nil {
		return nil, err
	}
	if priority.ShrinkageJstfMax, err = parseOptionalJstfMax(s, offset, priority.ShrinkageJstfMaxOffset); err != nil {
		return nil, err
	}
	if priority.GsubExtensionEnable, err = parseOptionalJstfModList(s, offset, priority.GsubExtensionEnableOffset); err != nil {
		return nil, err
	}
	if priority.GsubExtensionDisable, err = parseOptionalJstfModList(s, offset, priority.GsubExtensionDisableOffset); err != nil {
		return nil, err
	}
	if priority.GposExtensionEnable, err = parseOptionalJstfModList(s, offset, priority.GposExtensionEnableOffset); err != nil {
		return nil, err
	}
	if priority.GposExtensionDisable, err = parseOptionalJstfModList(s, offset, priority.GposExtensionDisableOffset); err != nil {
		return nil, err
	}
	if priority.ExtensionJstfMax, err = parseOptionalJstfMax(s, offset, priority.ExtensionJstfMaxOffset); err != nil {
		return nil, err
	}
	return priority, nil
}

func parseOptionalJstfModList(s api.Stream, parentOffset int64, relativeOffset uint16) (*JstfModList, error) {
	if relativeOffset == 0 {
		return nil, nil
	}
	return parseJstfModList(s, parentOffset+int64(relativeOffset))
}

func parseJstfModList(s api.Stream, offset int64) (*JstfModList, error) {
	if err := checkTableRange(s, offset, 2, "JstfModList table"); err != nil {
		return nil, err
	}
	modList := &JstfModList{}
	var err error
	modList.LookupCount, err = readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	if err := checkTableRange(s, offset, 2+int64(modList.LookupCount)*2, "JstfModList table"); err != nil {
		return nil, err
	}
	modList.LookupIndices = make([]uint16, int(modList.LookupCount))
	for i := range modList.LookupIndices {
		modList.LookupIndices[i], err = readUint16(s, offset+2+int64(i*2))
		if err != nil {
			return nil, err
		}
	}
	return modList, nil
}

func parseOptionalJstfMax(s api.Stream, parentOffset int64, relativeOffset uint16) (*JstfMax, error) {
	if relativeOffset == 0 {
		return nil, nil
	}
	return parseJstfMax(s, parentOffset+int64(relativeOffset))
}

func parseJstfMax(s api.Stream, offset int64) (*JstfMax, error) {
	if err := checkTableRange(s, offset, 2, "JstfMax table"); err != nil {
		return nil, err
	}
	max := &JstfMax{}
	var err error
	max.LookupCount, err = readUint16(s, offset)
	if err != nil {
		return nil, err
	}
	if err := checkTableRange(s, offset, 2+int64(max.LookupCount)*2, "JstfMax table"); err != nil {
		return nil, err
	}
	max.LookupOffsets = make([]uint16, int(max.LookupCount))
	for i := range max.LookupOffsets {
		max.LookupOffsets[i], err = readUint16(s, offset+2+int64(i*2))
		if err != nil {
			return nil, err
		}
		if max.LookupOffsets[i] != 0 {
			if err := checkTableRange(s, offset+int64(max.LookupOffsets[i]), 6, "JstfMax Lookup table"); err != nil {
				return nil, err
			}
		}
	}
	return max, nil
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
	RatioRanges   []VDMXRatioRange
	GroupOffsets  []uint16
	Groups        []VDMXGroup
}

type VDMXRatioRange struct {
	CharSet     byte
	XRatio      byte
	YStartRatio byte
	YEndRatio   byte
}

type VDMXGroup struct {
	StartSize byte
	EndSize   byte
	Records   []VDMXRecord
}

type VDMXRecord struct {
	YPelHeight uint16
	YMax       int16
	YMin       int16
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
	t.NumVDMXGroups, err = readUint16(s, 2)
	if err != nil {
		return t, err
	}
	t.NumRatios, err = readUint16(s, 4)
	if err != nil {
		return t, err
	}

	ratioCount := int(t.NumRatios)
	ratioRangesOffset := int64(6)
	groupOffsetsOffset := ratioRangesOffset + int64(ratioCount*4)
	groupDataMinOffset := groupOffsetsOffset + int64(ratioCount*2)
	if s.Size() < groupDataMinOffset {
		return t, errors.New("VDMX table too short for ratio ranges")
	}

	t.RatioRanges = make([]VDMXRatioRange, ratioCount)
	for i := 0; i < ratioCount; i++ {
		rangeOffset := ratioRangesOffset + int64(i*4)
		t.RatioRanges[i].CharSet, err = readByte(s, rangeOffset)
		if err != nil {
			return t, err
		}
		t.RatioRanges[i].XRatio, err = readByte(s, rangeOffset+1)
		if err != nil {
			return t, err
		}
		t.RatioRanges[i].YStartRatio, err = readByte(s, rangeOffset+2)
		if err != nil {
			return t, err
		}
		t.RatioRanges[i].YEndRatio, err = readByte(s, rangeOffset+3)
		if err != nil {
			return t, err
		}
	}

	t.GroupOffsets = make([]uint16, ratioCount)
	seenGroupOffsets := make(map[uint16]bool, ratioCount)
	for i := 0; i < ratioCount; i++ {
		groupOffset, err := readUint16(s, groupOffsetsOffset+int64(i*2))
		if err != nil {
			return t, err
		}
		t.GroupOffsets[i] = groupOffset
		if seenGroupOffsets[groupOffset] {
			continue
		}
		seenGroupOffsets[groupOffset] = true
		if int64(groupOffset) < groupDataMinOffset {
			return t, errors.New("VDMX group offset points into header")
		}
		if len(t.Groups) >= int(t.NumVDMXGroups) {
			return t, errors.New("VDMX table has more group offsets than declared groups")
		}
		group, err := parseVDMXGroup(s, int64(groupOffset))
		if err != nil {
			return t, err
		}
		t.Groups = append(t.Groups, group)
	}

	return t, nil
}

func parseVDMXGroup(s api.Stream, offset int64) (VDMXGroup, error) {
	var g VDMXGroup
	if offset < 0 || s.Size() < offset+4 {
		return g, errors.New("VDMX group too short")
	}
	numRecords, err := readUint16(s, offset)
	if err != nil {
		return g, err
	}
	g.StartSize, err = readByte(s, offset+2)
	if err != nil {
		return g, err
	}
	g.EndSize, err = readByte(s, offset+3)
	if err != nil {
		return g, err
	}
	if s.Size() < offset+4+int64(numRecords)*6 {
		return g, errors.New("VDMX group too short for records")
	}

	g.Records = make([]VDMXRecord, int(numRecords))
	for i := 0; i < int(numRecords); i++ {
		recordOffset := offset + 4 + int64(i*6)
		g.Records[i].YPelHeight, err = readUint16(s, recordOffset)
		if err != nil {
			return g, err
		}
		g.Records[i].YMax, err = readInt16(s, recordOffset+2)
		if err != nil {
			return g, err
		}
		g.Records[i].YMin, err = readInt16(s, recordOffset+4)
		if err != nil {
			return g, err
		}
	}
	return g, nil
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

func (t HdmxTable) RecordForPixelSize(ppem uint8) (HdmxDeviceRecord, bool) {
	for _, record := range t.Records {
		if record.PixelSize == ppem {
			return record, true
		}
	}
	return HdmxDeviceRecord{}, false
}

func (t HdmxTable) Width(glyphIndex int, ppem uint8) (uint8, bool) {
	if glyphIndex < 0 {
		return 0, false
	}
	record, ok := t.RecordForPixelSize(ppem)
	if !ok || glyphIndex >= len(record.Widths) {
		return 0, false
	}
	return record.Widths[glyphIndex], true
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
