package sfnt

import (
	"encoding/binary"
	"errors"

	"github.com/dh-kam/freetype-go/api"
)

const maxEmbeddedBitmapImageSize = 64 << 20

func streamHasRange(s api.Stream, offset int64, length int64) bool {
	if offset < 0 || length < 0 {
		return false
	}
	size := s.Size()
	if offset > size {
		return false
	}
	return length <= size-offset
}

func validateBitmapRead(s api.Stream, offset int64, length int64) error {
	if length <= 0 {
		return errors.New("invalid image length")
	}
	if length > maxEmbeddedBitmapImageSize {
		return errors.New("image data too large")
	}
	if !streamHasRange(s, offset, length) {
		return errors.New("image data out of bounds")
	}
	return nil
}

func bitmapDataOffset(base uint32, index uint32, size uint32) (int64, error) {
	offset := uint64(base) + uint64(index)*uint64(size)
	if offset > uint64(^uint64(0)>>1) {
		return 0, errors.New("image data offset too large")
	}
	return int64(offset), nil
}

type SbixTable struct {
	Stream        api.Stream
	NumStrikes    uint32
	StrikeOffsets []uint32
}

func parseSbix(s api.Stream) (SbixTable, error) {
	var t SbixTable
	var err error
	if s.Size() < 8 {
		return t, errors.New("sbix table too short")
	}
	t.Stream = s
	t.NumStrikes, err = readUint32(s, 4)
	if err != nil {
		return t, err
	}
	if !streamHasRange(s, 8, int64(t.NumStrikes)*4) {
		return t, errors.New("sbix table too short for strike offsets")
	}

	t.StrikeOffsets = make([]uint32, int(t.NumStrikes))
	for i := uint32(0); i < t.NumStrikes; i++ {
		t.StrikeOffsets[i], err = readUint32(s, 8+int64(i)*4)
		if err != nil {
			return t, err
		}
	}
	return t, nil
}

func (t *SbixTable) GetImage(glyphIndex int, ppem uint16) ([]byte, error) {
	if t.Stream == nil {
		return nil, errors.New("no sbix stream")
	}
	if glyphIndex < 0 {
		return nil, errors.New("invalid glyph index")
	}

	var bestStrikeOffset uint32
	var bestPpem uint16 = 0

	for _, off := range t.StrikeOffsets {
		strikePpem, err := readUint16(t.Stream, int64(off))
		if err != nil {
			continue
		}
		if strikePpem <= ppem && strikePpem > bestPpem {
			bestPpem = strikePpem
			bestStrikeOffset = off
		} else if bestStrikeOffset == 0 {
			bestPpem = strikePpem
			bestStrikeOffset = off
		}
	}

	if bestStrikeOffset == 0 {
		if len(t.StrikeOffsets) == 0 {
			return nil, errors.New("no strikes")
		}
		bestStrikeOffset = t.StrikeOffsets[0]
	}

	offset := int64(bestStrikeOffset) + 4 + int64(glyphIndex)*4
	if !streamHasRange(t.Stream, offset, 8) {
		return nil, errors.New("sbix glyph offset out of bounds")
	}
	startOffset, err := readUint32(t.Stream, offset)
	if err != nil {
		return nil, err
	}
	endOffset, err := readUint32(t.Stream, offset+4)
	if err != nil {
		return nil, err
	}

	if startOffset == endOffset {
		return nil, nil // no image
	}

	glyphDataOffset := int64(bestStrikeOffset) + int64(startOffset)
	if uint64(endOffset) <= uint64(startOffset)+8 {
		return nil, errors.New("invalid image length")
	}
	dataLen := int64(uint64(endOffset) - uint64(startOffset) - 8)
	if err := validateBitmapRead(t.Stream, glyphDataOffset+8, dataLen); err != nil {
		return nil, err
	}

	payload := make([]byte, int(dataLen))
	if err := readExactAt(t.Stream, payload, glyphDataOffset+8); err != nil {
		return nil, err
	}

	return payload, nil
}

type CBLCTable struct {
	Stream   api.Stream
	NumSizes uint32
}

type CBDTTable struct {
	Stream api.Stream
}

func parseCBLC(s api.Stream) (CBLCTable, error) {
	var t CBLCTable
	if s.Size() < 8 {
		return t, errors.New("CBLC too short")
	}
	numSizes, err := readUint32(s, 4)
	if err != nil {
		return t, err
	}
	if !streamHasRange(s, 8, int64(numSizes)*48) {
		return t, errors.New("CBLC too short for bitmap size records")
	}
	t.NumSizes = numSizes
	t.Stream = s
	return t, nil
}

func parseCBDT(s api.Stream) (CBDTTable, error) {
	return CBDTTable{Stream: s}, nil
}

func GetCBLCImage(cblc CBLCTable, cbdt CBDTTable, glyphIndex int) ([]byte, error) {
	return GetCBLCImageAtPPEM(cblc, cbdt, glyphIndex, 0)
}

func GetCBLCImageAtPPEM(cblc CBLCTable, cbdt CBDTTable, glyphIndex int, ppem uint16) ([]byte, error) {
	if cblc.Stream == nil || cbdt.Stream == nil {
		return nil, errors.New("missing CBLC or CBDT")
	}
	if glyphIndex < 0 {
		return nil, errors.New("invalid glyph index")
	}

	var foundSizeOffset int64 = 0
	var foundPPEM uint16
	for i := uint32(0); i < cblc.NumSizes; i++ {
		offset := int64(8 + i*48)
		startGlyph, err := readUint16(cblc.Stream, offset+40)
		if err != nil {
			return nil, err
		}
		endGlyph, err := readUint16(cblc.Stream, offset+42)
		if err != nil {
			return nil, err
		}

		if glyphIndex >= int(startGlyph) && glyphIndex <= int(endGlyph) {
			sizePPEM, err := cblcSizeRecordPPEM(cblc.Stream, offset)
			if err != nil {
				return nil, err
			}
			if foundSizeOffset == 0 || betterCBLCStrike(ppem, sizePPEM, foundPPEM) {
				foundSizeOffset = offset
				foundPPEM = sizePPEM
				if ppem != 0 && sizePPEM == ppem {
					break
				}
			}
		}
	}

	if foundSizeOffset == 0 {
		return nil, nil // no image for this glyph
	}

	indexSubTableArrayOffset, err := readUint32(cblc.Stream, foundSizeOffset)
	if err != nil {
		return nil, err
	}
	numberOfIndexSubTables, err := readUint32(cblc.Stream, foundSizeOffset+8)
	if err != nil {
		return nil, err
	}
	if !streamHasRange(cblc.Stream, int64(indexSubTableArrayOffset), int64(numberOfIndexSubTables)*8) {
		return nil, errors.New("CBLC index subtable array out of bounds")
	}

	var subTableOffset int64 = 0
	var firstGlyph uint16
	for i := uint32(0); i < numberOfIndexSubTables; i++ {
		offset := int64(indexSubTableArrayOffset) + int64(i*8)
		currentFirstGlyph, err := readUint16(cblc.Stream, offset)
		if err != nil {
			return nil, err
		}
		lastGlyph, err := readUint16(cblc.Stream, offset+2)
		if err != nil {
			return nil, err
		}

		if glyphIndex >= int(currentFirstGlyph) && glyphIndex <= int(lastGlyph) {
			addOffset, err := readUint32(cblc.Stream, offset+4)
			if err != nil {
				return nil, err
			}
			subTableOffset = int64(indexSubTableArrayOffset) + int64(addOffset)
			firstGlyph = currentFirstGlyph
			break
		}
	}

	if subTableOffset == 0 {
		return nil, nil
	}
	if !streamHasRange(cblc.Stream, subTableOffset, 8) {
		return nil, errors.New("CBLC index subtable out of bounds")
	}

	indexFormat, err := readUint16(cblc.Stream, subTableOffset)
	if err != nil {
		return nil, err
	}
	imageFormat, err := readUint16(cblc.Stream, subTableOffset+2)
	if err != nil {
		return nil, err
	}
	imageDataOffset, err := readUint32(cblc.Stream, subTableOffset+4)
	if err != nil {
		return nil, err
	}

	var dataOffset int64
	var dataLength int64

	switch indexFormat {
	case 1:
		offsetArrayEntry := subTableOffset + 8 + int64(glyphIndex-int(firstGlyph))*4
		if !streamHasRange(cblc.Stream, offsetArrayEntry, 8) {
			return nil, errors.New("CBLC offset array out of bounds")
		}
		off1, err := readUint32(cblc.Stream, offsetArrayEntry)
		if err != nil {
			return nil, err
		}
		off2, err := readUint32(cblc.Stream, offsetArrayEntry+4)
		if err != nil {
			return nil, err
		}
		if off2 < off1 {
			return nil, errors.New("invalid CBLC image offsets")
		}
		dataOffset = int64(imageDataOffset) + int64(off1)
		dataLength = int64(off2 - off1)
	case 2:
		imageSize, err := readUint32(cblc.Stream, subTableOffset+8)
		if err != nil {
			return nil, err
		}
		dataOffset = int64(imageDataOffset) + int64(glyphIndex-int(firstGlyph))*int64(imageSize)
		dataLength = int64(imageSize)
	case 3:
		offsetArrayEntry := subTableOffset + 8 + int64(glyphIndex-int(firstGlyph))*2
		if !streamHasRange(cblc.Stream, offsetArrayEntry, 4) {
			return nil, errors.New("CBLC offset array out of bounds")
		}
		off1, err := readUint16(cblc.Stream, offsetArrayEntry)
		if err != nil {
			return nil, err
		}
		off2, err := readUint16(cblc.Stream, offsetArrayEntry+2)
		if err != nil {
			return nil, err
		}
		if off2 < off1 {
			return nil, errors.New("invalid CBLC image offsets")
		}
		dataOffset = int64(imageDataOffset) + int64(off1)
		dataLength = int64(off2 - off1)
	case 4:
		numGlyphs, err := readUint32(cblc.Stream, subTableOffset+8)
		if err != nil {
			return nil, err
		}
		pairsOffset := subTableOffset + 12
		if !streamHasRange(cblc.Stream, pairsOffset, (int64(numGlyphs)+1)*4) {
			return nil, errors.New("CBLC glyph offset pairs out of bounds")
		}

		var off1, off2 uint16
		found := false
		for i := uint32(0); i < numGlyphs; i++ {
			pairOffset := pairsOffset + int64(i)*4
			gid, err := readUint16(cblc.Stream, pairOffset)
			if err != nil {
				return nil, err
			}
			if int(gid) != glyphIndex {
				continue
			}
			off1, err = readUint16(cblc.Stream, pairOffset+2)
			if err != nil {
				return nil, err
			}
			off2, err = readUint16(cblc.Stream, pairOffset+6)
			if err != nil {
				return nil, err
			}
			found = true
			break
		}
		if !found {
			return nil, nil
		}
		if off2 < off1 {
			return nil, errors.New("invalid CBLC image offsets")
		}
		dataOffset = int64(imageDataOffset) + int64(off1)
		dataLength = int64(off2 - off1)
	case 5:
		imageSize, err := readUint32(cblc.Stream, subTableOffset+8)
		if err != nil {
			return nil, err
		}
		numGlyphs, err := readUint32(cblc.Stream, subTableOffset+20)
		if err != nil {
			return nil, err
		}
		glyphArrayOffset := subTableOffset + 24
		if !streamHasRange(cblc.Stream, glyphArrayOffset, int64(numGlyphs)*2) {
			return nil, errors.New("CBLC glyph array out of bounds")
		}

		var foundIndex uint32
		found := false
		for i := uint32(0); i < numGlyphs; i++ {
			gid, err := readUint16(cblc.Stream, glyphArrayOffset+int64(i)*2)
			if err != nil {
				return nil, err
			}
			if int(gid) == glyphIndex {
				foundIndex = i
				found = true
				break
			}
		}
		if !found {
			return nil, nil
		}
		dataOffset, err = bitmapDataOffset(imageDataOffset, foundIndex, imageSize)
		if err != nil {
			return nil, err
		}
		dataLength = int64(imageSize)
	default:
		return nil, errors.New("unsupported index format")
	}

	if dataLength <= 0 {
		return nil, nil
	}
	if err := validateBitmapRead(cbdt.Stream, dataOffset, dataLength); err != nil {
		return nil, err
	}

	rawData := make([]byte, int(dataLength))
	if err := readExactAt(cbdt.Stream, rawData, dataOffset); err != nil {
		return nil, err
	}

	return extractEmbeddedBitmapPayload(imageFormat, rawData)
}

func cblcSizeRecordPPEM(s api.Stream, offset int64) (uint16, error) {
	ppemY, err := readByteAt(s, offset+45)
	if err != nil {
		return 0, err
	}
	if ppemY != 0 {
		return uint16(ppemY), nil
	}
	ppemX, err := readByteAt(s, offset+44)
	if err != nil {
		return 0, err
	}
	return uint16(ppemX), nil
}

func readByteAt(s api.Stream, offset int64) (byte, error) {
	var buf [1]byte
	if err := readExactAt(s, buf[:], offset); err != nil {
		return 0, err
	}
	return buf[0], nil
}

func betterCBLCStrike(requested, candidate, current uint16) bool {
	if requested == 0 {
		return false
	}
	if current == 0 {
		return true
	}
	if candidate == requested {
		return true
	}
	if current == requested {
		return false
	}
	if candidate <= requested && current > requested {
		return true
	}
	if candidate <= requested && current <= requested {
		return candidate > current
	}
	if candidate > requested && current > requested {
		return candidate < current
	}
	return false
}

func extractEmbeddedBitmapPayload(imageFormat uint16, rawData []byte) ([]byte, error) {
	switch imageFormat {
	case 1, 2:
		return stripEmbeddedBitmapHeader(rawData, 5)
	case 5:
		return rawData, nil
	case 6, 7:
		return stripEmbeddedBitmapHeader(rawData, 8)
	case 17:
		return extractLengthPrefixedEmbeddedBitmap(rawData, 5)
	case 18:
		return extractLengthPrefixedEmbeddedBitmap(rawData, 8)
	case 19:
		return extractLengthPrefixedEmbeddedBitmap(rawData, 0)
	default:
		// Composite and unknown bitmap formats are not decoded here yet.
		return rawData, nil
	}
}

func stripEmbeddedBitmapHeader(rawData []byte, headerLen int) ([]byte, error) {
	if len(rawData) < headerLen {
		return nil, errors.New("embedded bitmap payload too short")
	}
	return rawData[headerLen:], nil
}

func extractLengthPrefixedEmbeddedBitmap(rawData []byte, metricsLen int) ([]byte, error) {
	lengthOffset := metricsLen
	if len(rawData) < lengthOffset+4 {
		return nil, errors.New("embedded bitmap PNG payload too short")
	}
	dataLen := binary.BigEndian.Uint32(rawData[lengthOffset : lengthOffset+4])
	if dataLen > uint32(len(rawData)-(lengthOffset+4)) {
		return nil, errors.New("embedded bitmap PNG length out of bounds")
	}
	start := lengthOffset + 4
	return rawData[start : start+int(dataLen)], nil
}
