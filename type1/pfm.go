package type1

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// PFMHeaderSize is the size, in bytes, of the fixed Windows PFM header.
const PFMHeaderSize = 117

const pfmExtensionExtentOffsetEnd = 10
const pfmVariablePitchBit = 0x01

// PFM contains the Windows Printer Font Metrics fields needed by Type 1
// companion metric files.
type PFM struct {
	HeaderSize int
	Version    uint16
	DFSize     uint32

	FaceOffset uint32
	FaceName   string

	FirstChar   byte
	LastChar    byte
	DefaultChar byte
	BreakChar   byte

	AvgWidth       uint16
	MaxWidth       uint16
	Weight         uint16
	Italic         bool
	Charset        byte
	PitchAndFamily byte
	WidthBytes     uint16

	ExtensionOffset   uint32
	ExtensionSize     uint16
	ExtentTableOffset uint32
	ExtentWidths      []uint16
}

// GlyphCount returns the number of encoded character slots described by the
// PFM character range.
func (p *PFM) GlyphCount() int {
	if p == nil || p.LastChar < p.FirstChar {
		return 0
	}
	return int(p.LastChar) - int(p.FirstChar) + 1
}

// CharRange returns the inclusive encoded character range described by the PFM.
func (p *PFM) CharRange() (first, last byte) {
	if p == nil {
		return 0, 0
	}
	return p.FirstChar, p.LastChar
}

// WidthByCode returns a width for an 8-bit character code.
//
// The returned bool reports whether code is inside the PFM character range. If
// an extent table is present, the width is the exact extent-table value. If the
// extent table is absent, the width is the conservative summary metric returned
// by DefaultWidth.
func (p *PFM) WidthByCode(code int) (uint16, bool) {
	if p == nil || code < int(p.FirstChar) || code > int(p.LastChar) {
		return p.DefaultWidth(), false
	}
	if width, ok := p.extentWidthByCode(code); ok {
		return width, true
	}
	return p.metricFallbackWidth(), true
}

// WidthByRune returns a width for runes that can be represented as an 8-bit
// PFM character code.
func (p *PFM) WidthByRune(r rune) (uint16, bool) {
	if r < 0 || r > 0xff {
		return p.DefaultWidth(), false
	}
	return p.WidthByCode(int(r))
}

// DefaultWidth returns the PFM default character width when the extent table is
// available, otherwise it falls back to AvgWidth and then MaxWidth.
func (p *PFM) DefaultWidth() uint16 {
	if p == nil {
		return 0
	}
	if code, ok := p.defaultCode(); ok {
		if width, ok := p.extentWidthByCode(code); ok {
			return width
		}
	}
	return p.metricFallbackWidth()
}

// IsFixedPitch reports the PFM pitch bit. In Windows PFM/FNT data the low bit
// is set for variable-pitch fonts, so a clear bit indicates fixed pitch.
func (p *PFM) IsFixedPitch() bool {
	return p != nil && p.PitchAndFamily&pfmVariablePitchBit == 0
}

// ParsePFM parses a Windows Printer Font Metrics file far enough to expose
// Type 1 companion metrics and optional unscaled extent-table widths.
func ParsePFM(data []byte) (*PFM, error) {
	if len(data) < PFMHeaderSize {
		return nil, errors.New("pfm: truncated header")
	}

	size := binary.LittleEndian.Uint32(data[2:6])
	if size < PFMHeaderSize {
		return nil, fmt.Errorf("pfm: dfSize %d is smaller than header", size)
	}
	if uint64(size) > uint64(len(data)) {
		return nil, fmt.Errorf("pfm: dfSize %d exceeds data length %d", size, len(data))
	}
	limit := int(size)

	firstChar := data[95]
	lastChar := data[96]
	if lastChar < firstChar {
		return nil, fmt.Errorf("pfm: last character %d precedes first character %d", lastChar, firstChar)
	}

	faceOffset := binary.LittleEndian.Uint32(data[105:109])
	faceName, err := pfmCString(data, limit, faceOffset, "face name")
	if err != nil {
		return nil, err
	}

	pfm := &PFM{
		HeaderSize: PFMHeaderSize,
		Version:    binary.LittleEndian.Uint16(data[0:2]),
		DFSize:     size,

		FaceOffset: faceOffset,
		FaceName:   faceName,

		FirstChar:   firstChar,
		LastChar:    lastChar,
		DefaultChar: data[97],
		BreakChar:   data[98],

		AvgWidth:       binary.LittleEndian.Uint16(data[91:93]),
		MaxWidth:       binary.LittleEndian.Uint16(data[93:95]),
		Weight:         binary.LittleEndian.Uint16(data[83:85]),
		Italic:         data[80] != 0,
		Charset:        data[85],
		PitchAndFamily: data[90],
		WidthBytes:     binary.LittleEndian.Uint16(data[99:101]),
	}

	extensionOffset := PFMHeaderSize + int(pfm.WidthBytes)
	if extensionOffset > limit {
		return nil, fmt.Errorf("pfm: width table ends at %d beyond dfSize %d", extensionOffset, limit)
	}
	if extensionOffset == limit {
		return pfm, nil
	}

	if extensionOffset+2 > limit {
		return nil, errors.New("pfm: truncated extension size")
	}
	pfm.ExtensionOffset = uint32(extensionOffset)
	pfm.ExtensionSize = binary.LittleEndian.Uint16(data[extensionOffset : extensionOffset+2])
	if pfm.ExtensionSize == 0 {
		return pfm, nil
	}
	if uint64(extensionOffset)+uint64(pfm.ExtensionSize) > uint64(limit) {
		return nil, fmt.Errorf("pfm: extension size %d at offset %d exceeds dfSize %d", pfm.ExtensionSize, extensionOffset, limit)
	}

	if pfm.ExtensionSize < pfmExtensionExtentOffsetEnd {
		return pfm, nil
	}
	pfm.ExtentTableOffset = binary.LittleEndian.Uint32(data[extensionOffset+6 : extensionOffset+10])
	if pfm.ExtentTableOffset == 0 {
		return pfm, nil
	}

	widths, err := pfmExtentWidths(data, limit, pfm.ExtentTableOffset, firstChar, lastChar)
	if err != nil {
		return nil, err
	}
	pfm.ExtentWidths = widths
	return pfm, nil
}

func pfmCString(data []byte, limit int, offset uint32, field string) (string, error) {
	if offset == 0 {
		return "", fmt.Errorf("pfm: missing %s offset", field)
	}
	start, ok := pfmRange(limit, offset, 1)
	if !ok {
		return "", fmt.Errorf("pfm: %s offset %d out of bounds", field, offset)
	}
	end := start
	for end < limit && data[end] != 0 {
		end++
	}
	if end == limit {
		return "", fmt.Errorf("pfm: unterminated %s", field)
	}
	if end == start {
		return "", fmt.Errorf("pfm: empty %s", field)
	}
	return string(data[start:end]), nil
}

func pfmExtentWidths(data []byte, limit int, offset uint32, firstChar, lastChar byte) ([]uint16, error) {
	count := int(lastChar) - int(firstChar) + 1
	start, ok := pfmRange(limit, offset, count*2)
	if !ok {
		return nil, fmt.Errorf("pfm: extent table offset %d with %d widths out of bounds", offset, count)
	}
	widths := make([]uint16, count)
	for i := range widths {
		widths[i] = binary.LittleEndian.Uint16(data[start+i*2 : start+i*2+2])
	}
	return widths, nil
}

func (p *PFM) extentWidthByCode(code int) (uint16, bool) {
	if p == nil || len(p.ExtentWidths) == 0 {
		return 0, false
	}
	index := code - int(p.FirstChar)
	if index < 0 || index >= len(p.ExtentWidths) {
		return 0, false
	}
	return p.ExtentWidths[index], true
}

func (p *PFM) defaultCode() (int, bool) {
	if p == nil || p.LastChar < p.FirstChar {
		return 0, false
	}

	// PFM stores dfDefaultChar relative to dfFirstChar. Accept an absolute
	// in-range value as a compatibility fallback for hand-built fixtures.
	code := int(p.FirstChar) + int(p.DefaultChar)
	if code <= int(p.LastChar) {
		return code, true
	}
	if int(p.DefaultChar) >= int(p.FirstChar) && int(p.DefaultChar) <= int(p.LastChar) {
		return int(p.DefaultChar), true
	}
	return 0, false
}

func (p *PFM) metricFallbackWidth() uint16 {
	if p == nil {
		return 0
	}
	if p.AvgWidth != 0 {
		return p.AvgWidth
	}
	return p.MaxWidth
}

func pfmRange(limit int, offset uint32, length int) (int, bool) {
	if length < 0 {
		return 0, false
	}
	if uint64(offset) > uint64(limit) {
		return 0, false
	}
	if uint64(length) > uint64(limit)-uint64(offset) {
		return 0, false
	}
	return int(offset), true
}
