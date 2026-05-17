package helper

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"

	"github.com/andybalholm/brotli"
	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

// woff2KnownTags maps index to standard tags
var woff2KnownTags = []uint32{
	0x636d6170, 0x68656164, 0x68686561, 0x686d7478, 0x6d617870, 0x6e616d65, 0x4f532f32, 0x706f7374,
	0x63767420, 0x6670676d, 0x676c7966, 0x6c6f6361, 0x70726570, 0x43464620, 0x564f5247, 0x45424454,
	0x45424c43, 0x67617370, 0x68646d78, 0x6b65726e, 0x4c545348, 0x50434c54, 0x56444d58, 0x76686561,
	0x766d7478, 0x42415345, 0x47444546, 0x47504f53, 0x47535542, 0x45425343, 0x4a535446, 0x4d415448,
	0x43424454, 0x43424c43, 0x434f4c52, 0x4350414c, 0x53564720, 0x73626978, 0x61636e74, 0x61766172,
	0x62646174, 0x626c6f63, 0x62736c6e, 0x63766172, 0x66656174, 0x666d7478, 0x66766172, 0x67766172,
	0x68737478, 0x6a757374, 0x6c636172, 0x6d6f7274, 0x6d6f7278, 0x6f706264, 0x70726f70, 0x7472616b,
	0x5a617066, 0x53696c66, 0x476c6174, 0x476c6f63, 0x46656174, 0x53696c6c,
}

func readBase128(r io.Reader) (uint32, error) {
	var result uint32
	var b [1]byte
	for i := 0; i < 5; i++ {
		if _, err := r.Read(b[:]); err != nil {
			return 0, err
		}
		data := b[0]
		if i == 0 && data == 0x80 {
			return 0, errors.New("invalid base128")
		}
		if result&0xFE000000 != 0 {
			return 0, errors.New("base128 overflow")
		}
		result = (result << 7) | uint32(data&0x7f)
		if data&0x80 == 0 {
			return result, nil
		}
	}
	return 0, errors.New("base128 exceeds 5 bytes")
}

// DecodeWOFF reconstructs an SFNT font from a WOFF stream.
func DecodeWOFF(in api.Stream) (api.Stream, error) {
	if in.Size() < 44 {
		return nil, errors.New("invalid WOFF file: too small")
	}

	headerData := make([]byte, 44)
	if _, err := in.ReadAt(headerData, 0); err != nil {
		return nil, err
	}

	sig := binary.BigEndian.Uint32(headerData[0:4])
	if sig != 0x774F4646 { // "wOFF"
		return nil, errors.New("not a WOFF file")
	}

	flavor := binary.BigEndian.Uint32(headerData[4:8])
	numTables := binary.BigEndian.Uint16(headerData[12:14])
	totalSfntSize := binary.BigEndian.Uint32(headerData[16:20])
	if totalSfntSize > maxDecodedFontSize {
		return nil, errors.New("WOFF decoded font too large")
	}

	dirSize := int64(numTables) * 20
	if in.Size() < 44+dirSize {
		return nil, errors.New("invalid WOFF file: directory too small")
	}

	dirData := make([]byte, dirSize)
	if dirSize > 0 {
		if _, err := in.ReadAt(dirData, 44); err != nil {
			return nil, err
		}
	}

	outBuf := bytes.NewBuffer(make([]byte, 0, totalSfntSize))

	var entrySelector uint16
	var maxPower2 uint16 = 1
	for maxPower2*2 <= numTables {
		maxPower2 *= 2
		entrySelector++
	}
	searchRange := maxPower2 * 16
	rangeShift := numTables*16 - searchRange

	sfntHeader := make([]byte, 12)
	binary.BigEndian.PutUint32(sfntHeader[0:4], flavor)
	binary.BigEndian.PutUint16(sfntHeader[4:6], numTables)
	binary.BigEndian.PutUint16(sfntHeader[6:8], searchRange)
	binary.BigEndian.PutUint16(sfntHeader[8:10], entrySelector)
	binary.BigEndian.PutUint16(sfntHeader[10:12], rangeShift)
	outBuf.Write(sfntHeader)

	type tableEntry struct {
		tag          uint32
		offset       uint32
		compLength   uint32
		origLength   uint32
		origChecksum uint32
	}

	tables := make([]tableEntry, numTables)
	for i := uint16(0); i < numTables; i++ {
		offset := i * 20
		tables[i] = tableEntry{
			tag:          binary.BigEndian.Uint32(dirData[offset : offset+4]),
			offset:       binary.BigEndian.Uint32(dirData[offset+4 : offset+8]),
			compLength:   binary.BigEndian.Uint32(dirData[offset+8 : offset+12]),
			origLength:   binary.BigEndian.Uint32(dirData[offset+12 : offset+16]),
			origChecksum: binary.BigEndian.Uint32(dirData[offset+16 : offset+20]),
		}
	}

	currentOffset := uint32(12) + uint32(numTables)*16
	sfntDir := make([]byte, int(numTables)*16)
	for i, t := range tables {
		dirOff := i * 16
		binary.BigEndian.PutUint32(sfntDir[dirOff:dirOff+4], t.tag)
		binary.BigEndian.PutUint32(sfntDir[dirOff+4:dirOff+8], t.origChecksum)
		binary.BigEndian.PutUint32(sfntDir[dirOff+8:dirOff+12], currentOffset)
		binary.BigEndian.PutUint32(sfntDir[dirOff+12:dirOff+16], t.origLength)

		paddedLength := (t.origLength + 3) &^ 3
		if t.origLength > maxDecodedFontSize || paddedLength < t.origLength || uint64(currentOffset)+uint64(paddedLength) > uint64(maxDecodedFontSize) {
			return nil, errors.New("WOFF decoded font too large")
		}
		currentOffset += paddedLength
	}
	outBuf.Write(sfntDir)

	for _, t := range tables {
		if uint64(t.offset)+uint64(t.compLength) > uint64(in.Size()) {
			return nil, errors.New("invalid WOFF table offset")
		}
		if t.compLength > maxCompressedFontDataSize {
			return nil, errors.New("WOFF compressed table too large")
		}

		compData := make([]byte, t.compLength)
		if _, err := in.ReadAt(compData, int64(t.offset)); err != nil {
			return nil, err
		}

		var origData []byte
		if t.compLength < t.origLength {
			if t.origLength > maxDecodedFontSize {
				return nil, errors.New("WOFF decoded font too large")
			}
			r, err := zlib.NewReader(bytes.NewReader(compData))
			if err != nil {
				return nil, err
			}
			origData, err = readAllLimited(r, int64(t.origLength))
			r.Close()
			if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
				return nil, err
			}
			if uint32(len(origData)) != t.origLength {
				return nil, errors.New("WOFF table decompression size mismatch")
			}
		} else {
			origData = compData
		}

		outBuf.Write(origData)

		padding := (4 - (t.origLength & 3)) & 3
		if padding > 0 {
			outBuf.Write(make([]byte, padding))
		}
	}

	return core.NewMemoryStream(outBuf.Bytes()), nil
}

// DecodeWOFF2 reconstructs an SFNT font from a WOFF2 stream.
func DecodeWOFF2(in api.Stream) (api.Stream, error) {
	if in.Size() < 48 {
		return nil, errors.New("invalid WOFF2 file: too small")
	}

	headerData := make([]byte, 48)
	if _, err := in.ReadAt(headerData, 0); err != nil {
		return nil, err
	}

	sig := binary.BigEndian.Uint32(headerData[0:4])
	if sig != 0x774F4632 { // "wOF2"
		return nil, errors.New("not a WOFF2 file")
	}

	flavor := binary.BigEndian.Uint32(headerData[4:8])
	numTables := binary.BigEndian.Uint16(headerData[12:14])
	totalSfntSize := binary.BigEndian.Uint32(headerData[16:20])
	totalCompressedSize := binary.BigEndian.Uint32(headerData[20:24])
	if totalSfntSize > maxDecodedFontSize {
		return nil, errors.New("WOFF2 decoded font too large")
	}
	if totalCompressedSize > maxCompressedFontDataSize {
		return nil, errors.New("WOFF2 compressed data too large")
	}

	r := io.NewSectionReader(readerAtStream{in}, 48, in.Size()-48)

	type tableEntry2 struct {
		tag             uint32
		flags           uint8
		origLength      uint32
		transformLength uint32
	}

	tables := make([]tableEntry2, numTables)
	for i := uint16(0); i < numTables; i++ {
		var flag [1]byte
		if _, err := r.Read(flag[:]); err != nil {
			return nil, err
		}
		tables[i].flags = flag[0]

		tagIdx := flag[0] & 0x3f
		if tagIdx == 0x3f {
			var tagBuf [4]byte
			if _, err := r.Read(tagBuf[:]); err != nil {
				return nil, err
			}
			tables[i].tag = binary.BigEndian.Uint32(tagBuf[:])
		} else {
			if int(tagIdx) >= len(woff2KnownTags) {
				return nil, errors.New("invalid WOFF2 tag index")
			}
			tables[i].tag = woff2KnownTags[tagIdx]
		}

		origLen, err := readBase128(r)
		if err != nil {
			return nil, err
		}
		tables[i].origLength = origLen
		if origLen > maxDecodedFontSize {
			return nil, errors.New("WOFF2 decoded font too large")
		}

		transformVersion := flag[0] >> 6
		if tables[i].tag == 0x676c7966 || tables[i].tag == 0x6c6f6361 {
			if transformVersion == 0 {
				transformLen, err := readBase128(r)
				if err != nil {
					return nil, err
				}
				tables[i].transformLength = transformLen
			} else {
				tables[i].transformLength = tables[i].origLength
			}
		} else if transformVersion != 0 {
			transformLen, err := readBase128(r)
			if err != nil {
				return nil, err
			}
			tables[i].transformLength = transformLen
		} else {
			tables[i].transformLength = tables[i].origLength
		}
		if tables[i].transformLength > maxDecodedFontSize {
			return nil, errors.New("WOFF2 decoded font too large")
		}
	}

	compressedOffset, _ := r.Seek(0, io.SeekCurrent)
	compressedOffset += 48

	if uint64(compressedOffset)+uint64(totalCompressedSize) > uint64(in.Size()) {
		return nil, errors.New("invalid WOFF2 compressed size")
	}

	compData := make([]byte, totalCompressedSize)
	if _, err := in.ReadAt(compData, compressedOffset); err != nil {
		return nil, err
	}

	br := brotli.NewReader(bytes.NewReader(compData))
	uncompressedData, err := readAllLimited(br, maxDecodedFontSize)
	if err != nil {
		return nil, err
	}

	outBuf := bytes.NewBuffer(make([]byte, 0, totalSfntSize))

	var entrySelector uint16
	var maxPower2 uint16 = 1
	for maxPower2*2 <= numTables {
		maxPower2 *= 2
		entrySelector++
	}
	searchRange := maxPower2 * 16
	rangeShift := numTables*16 - searchRange

	sfntHeader := make([]byte, 12)
	binary.BigEndian.PutUint32(sfntHeader[0:4], flavor)
	binary.BigEndian.PutUint16(sfntHeader[4:6], numTables)
	binary.BigEndian.PutUint16(sfntHeader[6:8], searchRange)
	binary.BigEndian.PutUint16(sfntHeader[8:10], entrySelector)
	binary.BigEndian.PutUint16(sfntHeader[10:12], rangeShift)
	outBuf.Write(sfntHeader)

	currentOffset := uint32(12) + uint32(numTables)*16
	sfntDir := make([]byte, int(numTables)*16)
	for i, t := range tables {
		dirOff := i * 16
		binary.BigEndian.PutUint32(sfntDir[dirOff:dirOff+4], t.tag)
		binary.BigEndian.PutUint32(sfntDir[dirOff+4:dirOff+8], 0)
		binary.BigEndian.PutUint32(sfntDir[dirOff+8:dirOff+12], currentOffset)
		binary.BigEndian.PutUint32(sfntDir[dirOff+12:dirOff+16], t.origLength)

		paddedLength := (t.origLength + 3) &^ 3
		if paddedLength < t.origLength || uint64(currentOffset)+uint64(paddedLength) > uint64(maxDecodedFontSize) {
			return nil, errors.New("WOFF2 decoded font too large")
		}
		currentOffset += paddedLength
	}
	outBuf.Write(sfntDir)

	uncompressedOffset := uint32(0)
	for _, t := range tables {
		if uncompressedOffset+t.transformLength > uint32(len(uncompressedData)) {
			return nil, errors.New("WOFF2 uncompressed data too short")
		}

		tableData := uncompressedData[uncompressedOffset : uncompressedOffset+t.transformLength]
		uncompressedOffset += t.transformLength

		if (t.tag == 0x676c7966 || t.tag == 0x6c6f6361) && (t.flags>>6) == 0 {
			return nil, errors.New("WOFF2 glyf/loca reconstruction not implemented")
		}

		outBuf.Write(tableData)

		padding := (4 - (t.origLength & 3)) & 3
		if padding > 0 {
			outBuf.Write(make([]byte, padding))
		}
	}

	return core.NewMemoryStream(outBuf.Bytes()), nil
}
