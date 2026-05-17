package sfnt

import (
	"errors"
	"github.com/dh-kam/freetype-go/api"
)

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

	t.StrikeOffsets = make([]uint32, t.NumStrikes)
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
	dataLen := endOffset - startOffset - 8
	if dataLen <= 0 {
		return nil, errors.New("invalid image length")
	}

	payload := make([]byte, dataLen)
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
	t.NumSizes = numSizes
	t.Stream = s
	return t, nil
}

func parseCBDT(s api.Stream) (CBDTTable, error) {
	return CBDTTable{Stream: s}, nil
}

func GetCBLCImage(cblc CBLCTable, cbdt CBDTTable, glyphIndex int) ([]byte, error) {
	if cblc.Stream == nil || cbdt.Stream == nil {
		return nil, errors.New("missing CBLC or CBDT")
	}

	var foundSizeOffset int64 = 0
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
			foundSizeOffset = offset
			break
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

	var subTableOffset int64 = 0
	for i := uint32(0); i < numberOfIndexSubTables; i++ {
		offset := int64(indexSubTableArrayOffset) + int64(i*8)
		firstGlyph, err := readUint16(cblc.Stream, offset)
		if err != nil {
			return nil, err
		}
		lastGlyph, err := readUint16(cblc.Stream, offset+2)
		if err != nil {
			return nil, err
		}

		if glyphIndex >= int(firstGlyph) && glyphIndex <= int(lastGlyph) {
			addOffset, err := readUint32(cblc.Stream, offset+4)
			if err != nil {
				return nil, err
			}
			subTableOffset = int64(indexSubTableArrayOffset) + int64(addOffset)
			break
		}
	}

	if subTableOffset == 0 {
		return nil, nil
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

	// We need the firstGlyphIndex of the found subtable. We can re-read it.
	// Actually, wait, it's easier to just pass firstGlyph along. Let's re-find it.

	// Re-find firstGlyph
	var firstGlyph uint16
	for i := uint32(0); i < numberOfIndexSubTables; i++ {
		offset := int64(indexSubTableArrayOffset) + int64(i*8)
		fg, _ := readUint16(cblc.Stream, offset)
		lg, _ := readUint16(cblc.Stream, offset+2)
		if glyphIndex >= int(fg) && glyphIndex <= int(lg) {
			firstGlyph = fg
			break
		}
	}

	var dataOffset int64
	var dataLength int64

	switch indexFormat {
	case 1:
		off1, err := readUint32(cblc.Stream, subTableOffset+8+int64(glyphIndex-int(firstGlyph))*4)
		if err != nil {
			return nil, err
		}
		off2, err := readUint32(cblc.Stream, subTableOffset+8+int64(glyphIndex-int(firstGlyph)+1)*4)
		if err != nil {
			return nil, err
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
		off1, err := readUint16(cblc.Stream, subTableOffset+8+int64(glyphIndex-int(firstGlyph))*2)
		if err != nil {
			return nil, err
		}
		off2, err := readUint16(cblc.Stream, subTableOffset+8+int64(glyphIndex-int(firstGlyph)+1)*2)
		if err != nil {
			return nil, err
		}
		dataOffset = int64(imageDataOffset) + int64(off1)
		dataLength = int64(off2 - off1)
	default:
		return nil, errors.New("unsupported index format")
	}

	if dataLength <= 0 {
		return nil, nil
	}

	rawData := make([]byte, dataLength)
	if err := readExactAt(cbdt.Stream, rawData, dataOffset); err != nil {
		return nil, err
	}

	switch imageFormat {
	case 17:
		return rawData, nil
	case 18:
		if len(rawData) > 5 {
			return rawData[5:], nil
		}
	case 19:
		if len(rawData) > 8 {
			return rawData[8:], nil
		}
	default:
		// Unsupported image format (maybe not PNG), return as is for now
		return rawData, nil
	}

	return rawData, nil
}
