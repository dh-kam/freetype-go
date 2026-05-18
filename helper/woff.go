package helper

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"

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

const (
	tagTTCF = 0x74746366
	tagHead = 0x68656164
	tagHhea = 0x68686561
	tagHmtx = 0x686d7478
	tagMaxp = 0x6d617870
	tagGlyf = 0x676c7966
	tagLoca = 0x6c6f6361

	woff2TransformGlyfLoca = 0
	woff2TransformHmtx     = 1
	woff2TransformNull     = 3
)

type woff2TableEntry struct {
	tag             uint32
	flags           uint8
	origLength      uint32
	transformLength uint32
}

type woff2Collection struct {
	version uint32
	fonts   []woff2CollectionFont
}

type woff2CollectionFont struct {
	flavor  uint32
	indices []uint16
}

type woffTableEntry struct {
	tag          uint32
	offset       uint32
	compLength   uint32
	origLength   uint32
	origChecksum uint32
}

type woffBlockRange struct {
	name   string
	offset uint32
	end    uint32
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

func read255UInt16(r io.Reader) (uint16, error) {
	var code [1]byte
	if _, err := io.ReadFull(r, code[:]); err != nil {
		return 0, err
	}
	switch code[0] {
	case 253:
		var buf [2]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, err
		}
		return binary.BigEndian.Uint16(buf[:]), nil
	case 254:
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		return uint16(b[0]) + 506, nil
	case 255:
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		return uint16(b[0]) + 253, nil
	default:
		return uint16(code[0]), nil
	}
}

func readWOFF2CollectionDirectory(r io.Reader, numTables int) (*woff2Collection, error) {
	var versionBuf [4]byte
	if _, err := io.ReadFull(r, versionBuf[:]); err != nil {
		return nil, err
	}
	version := binary.BigEndian.Uint32(versionBuf[:])
	if version != 0x00010000 && version != 0x00020000 {
		return nil, fmt.Errorf("unsupported WOFF2 collection version 0x%08x", version)
	}

	numFonts, err := read255UInt16(r)
	if err != nil {
		return nil, err
	}
	if numFonts == 0 {
		return nil, errors.New("WOFF2 collection has no fonts")
	}

	collection := &woff2Collection{
		version: version,
		fonts:   make([]woff2CollectionFont, int(numFonts)),
	}
	for i := range collection.fonts {
		fontNumTables, err := read255UInt16(r)
		if err != nil {
			return nil, err
		}
		if fontNumTables == 0 {
			return nil, errors.New("WOFF2 collection font has no tables")
		}
		var flavorBuf [4]byte
		if _, err := io.ReadFull(r, flavorBuf[:]); err != nil {
			return nil, err
		}
		font := woff2CollectionFont{
			flavor:  binary.BigEndian.Uint32(flavorBuf[:]),
			indices: make([]uint16, int(fontNumTables)),
		}
		seen := make(map[uint16]struct{}, int(fontNumTables))
		for j := range font.indices {
			index, err := read255UInt16(r)
			if err != nil {
				return nil, err
			}
			if int(index) >= numTables {
				return nil, errors.New("WOFF2 collection table index out of range")
			}
			if _, ok := seen[index]; ok {
				return nil, errors.New("WOFF2 collection font has duplicate table index")
			}
			seen[index] = struct{}{}
			font.indices[j] = index
		}
		collection.fonts[i] = font
	}
	return collection, nil
}

// DecodeWOFFIfNeeded unwraps WOFF/WOFF2 font streams into SFNT or TTC streams.
// Streams with any other signature are returned unchanged.
func DecodeWOFFIfNeeded(in api.Stream) (api.Stream, error) {
	if in.Size() < 4 {
		return in, nil
	}
	var sig [4]byte
	if _, err := in.ReadAt(sig[:], 0); err != nil {
		return nil, err
	}
	switch binary.BigEndian.Uint32(sig[:]) {
	case 0x774f4646: // "wOFF"
		return DecodeWOFF(in)
	case 0x774f4632: // "wOF2"
		return DecodeWOFF2(in)
	default:
		return in, nil
	}
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
	declaredLength := binary.BigEndian.Uint32(headerData[8:12])
	numTables := binary.BigEndian.Uint16(headerData[12:14])
	reserved := binary.BigEndian.Uint16(headerData[14:16])
	totalSfntSize := binary.BigEndian.Uint32(headerData[16:20])
	metaOffset := binary.BigEndian.Uint32(headerData[24:28])
	metaLength := binary.BigEndian.Uint32(headerData[28:32])
	metaOrigLength := binary.BigEndian.Uint32(headerData[32:36])
	privOffset := binary.BigEndian.Uint32(headerData[36:40])
	privLength := binary.BigEndian.Uint32(headerData[40:44])
	if reserved != 0 {
		return nil, errors.New("invalid WOFF reserved field")
	}
	if uint64(declaredLength) != uint64(in.Size()) {
		return nil, errors.New("invalid WOFF file length")
	}
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

	tables := make([]woffTableEntry, numTables)
	for i := uint16(0); i < numTables; i++ {
		offset := i * 20
		tables[i] = woffTableEntry{
			tag:          binary.BigEndian.Uint32(dirData[offset : offset+4]),
			offset:       binary.BigEndian.Uint32(dirData[offset+4 : offset+8]),
			compLength:   binary.BigEndian.Uint32(dirData[offset+8 : offset+12]),
			origLength:   binary.BigEndian.Uint32(dirData[offset+12 : offset+16]),
			origChecksum: binary.BigEndian.Uint32(dirData[offset+16 : offset+20]),
		}
	}
	tableRanges, err := validateWOFFTableRanges(tables, uint32(44+dirSize), declaredLength)
	if err != nil {
		return nil, err
	}
	if err := validateWOFFMetadataPrivateBlocks(in, tableRanges, uint32(44+dirSize), declaredLength, metaOffset, metaLength, metaOrigLength, privOffset, privLength); err != nil {
		return nil, err
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

func validateWOFFTableRanges(tables []woffTableEntry, tableDataStart, declaredLength uint32) ([]woffBlockRange, error) {
	ranges := make([]woffBlockRange, 0, len(tables))
	for _, t := range tables {
		if t.compLength > t.origLength {
			return nil, errors.New("invalid WOFF table length")
		}
		if t.offset&3 != 0 {
			return nil, errors.New("invalid WOFF table alignment")
		}
		end, err := checkedBlockEnd(t.offset, t.compLength)
		if err != nil || t.offset < tableDataStart || end > declaredLength {
			return nil, errors.New("invalid WOFF table offset")
		}
		ranges = append(ranges, woffBlockRange{
			name:   tagString(t.tag),
			offset: t.offset,
			end:    end,
		})
	}
	if err := validateNoBlockOverlap(ranges, "invalid WOFF table overlap"); err != nil {
		return nil, err
	}
	return ranges, nil
}

func validateWOFFMetadataPrivateBlocks(in api.Stream, tableRanges []woffBlockRange, tableDataStart, declaredLength, metaOffset, metaLength, metaOrigLength, privOffset, privLength uint32) error {
	lastTableEnd := tableDataStart
	for _, r := range tableRanges {
		if r.end > lastTableEnd {
			lastTableEnd = r.end
		}
	}

	nextOffset, err := align4Checked(lastTableEnd)
	if err != nil {
		return errors.New("invalid WOFF block offset")
	}
	if err := validatePaddingZeros(in, lastTableEnd, nextOffset); err != nil {
		return err
	}

	ranges := append([]woffBlockRange(nil), tableRanges...)
	hasMetadata, err := validateMetadataFields("WOFF", metaOffset, metaLength, metaOrigLength)
	if err != nil {
		return err
	}
	hasPrivate, err := validatePrivateFields("WOFF", privOffset, privLength)
	if err != nil {
		return err
	}

	if hasMetadata {
		if metaOffset&3 != 0 || metaOffset != nextOffset {
			return errors.New("invalid WOFF metadata block")
		}
		metaEnd, err := checkedBlockEnd(metaOffset, metaLength)
		if err != nil || metaEnd > declaredLength {
			return errors.New("invalid WOFF metadata block")
		}
		ranges = append(ranges, woffBlockRange{name: "metadata", offset: metaOffset, end: metaEnd})
		nextOffset = metaEnd
	} else if metaOffset != 0 || metaLength != 0 || metaOrigLength != 0 {
		return errors.New("invalid WOFF metadata block")
	}

	if hasPrivate {
		alignedNext, err := align4Checked(nextOffset)
		if err != nil {
			return errors.New("invalid WOFF private block")
		}
		if err := validatePaddingZeros(in, nextOffset, alignedNext); err != nil {
			return err
		}
		if privOffset&3 != 0 || privOffset != alignedNext {
			return errors.New("invalid WOFF private block")
		}
		privEnd, err := checkedBlockEnd(privOffset, privLength)
		if err != nil || privEnd != declaredLength {
			return errors.New("invalid WOFF private block")
		}
		ranges = append(ranges, woffBlockRange{name: "private", offset: privOffset, end: privEnd})
		nextOffset = privEnd
	} else if privOffset != 0 || privLength != 0 {
		return errors.New("invalid WOFF private block")
	}

	if !hasPrivate {
		if nextOffset != declaredLength {
			return errors.New("invalid WOFF trailing data")
		}
	}
	return validateNoBlockOverlap(ranges, "invalid WOFF block overlap")
}

func validateWOFF2MetadataPrivateBlocks(in api.Stream, declaredLength uint32, compressedOffset int64, totalCompressedSize, metaOffset, metaLength, metaOrigLength, privOffset, privLength uint32) error {
	if compressedOffset < 0 || uint64(compressedOffset) > uint64(declaredLength) {
		return errors.New("invalid WOFF2 compressed size")
	}
	compressedEnd64 := uint64(compressedOffset) + uint64(totalCompressedSize)
	if compressedEnd64 > uint64(declaredLength) || compressedEnd64 > uint64(^uint32(0)) {
		return errors.New("invalid WOFF2 compressed size")
	}
	nextOffset := uint32(compressedEnd64)

	hasMetadata, err := validateMetadataFields("WOFF2", metaOffset, metaLength, metaOrigLength)
	if err != nil {
		return err
	}
	hasPrivate, err := validatePrivateFields("WOFF2", privOffset, privLength)
	if err != nil {
		return err
	}

	if hasMetadata {
		alignedNext, err := align4Checked(nextOffset)
		if err != nil {
			return errors.New("invalid WOFF2 metadata block")
		}
		if err := validatePaddingZeros(in, nextOffset, alignedNext); err != nil {
			return err
		}
		if metaOffset&3 != 0 || metaOffset != alignedNext {
			return errors.New("invalid WOFF2 metadata block")
		}
		metaEnd, err := checkedBlockEnd(metaOffset, metaLength)
		if err != nil || metaEnd > declaredLength {
			return errors.New("invalid WOFF2 metadata block")
		}
		nextOffset = metaEnd
	} else if metaOffset != 0 || metaLength != 0 || metaOrigLength != 0 {
		return errors.New("invalid WOFF2 metadata block")
	}

	if hasPrivate {
		alignedNext, err := align4Checked(nextOffset)
		if err != nil {
			return errors.New("invalid WOFF2 private block")
		}
		if err := validatePaddingZeros(in, nextOffset, alignedNext); err != nil {
			return err
		}
		if privOffset&3 != 0 || privOffset != alignedNext {
			return errors.New("invalid WOFF2 private block")
		}
		privEnd, err := checkedBlockEnd(privOffset, privLength)
		if err != nil || privEnd != declaredLength {
			return errors.New("invalid WOFF2 private block")
		}
		nextOffset = privEnd
	} else if privOffset != 0 || privLength != 0 {
		return errors.New("invalid WOFF2 private block")
	}

	if !hasPrivate && nextOffset != declaredLength {
		return errors.New("invalid WOFF2 trailing data")
	}
	return nil
}

func validateMetadataFields(format string, offset, length, origLength uint32) (bool, error) {
	if offset == 0 && length == 0 && origLength == 0 {
		return false, nil
	}
	if offset == 0 || length == 0 || origLength == 0 {
		return false, fmt.Errorf("invalid %s metadata block", format)
	}
	if length > maxCompressedFontDataSize || origLength > maxDecodedFontSize {
		return false, fmt.Errorf("%s metadata block too large", format)
	}
	return true, nil
}

func validatePrivateFields(format string, offset, length uint32) (bool, error) {
	if offset == 0 && length == 0 {
		return false, nil
	}
	if offset == 0 || length == 0 {
		return false, fmt.Errorf("invalid %s private block", format)
	}
	return true, nil
}

func checkedBlockEnd(offset, length uint32) (uint32, error) {
	end := uint64(offset) + uint64(length)
	if end > uint64(^uint32(0)) {
		return 0, errors.New("block range overflow")
	}
	return uint32(end), nil
}

func align4Checked(offset uint32) (uint32, error) {
	aligned := align4(offset)
	if aligned < offset {
		return 0, errors.New("block alignment overflow")
	}
	return aligned, nil
}

func validateNoBlockOverlap(ranges []woffBlockRange, message string) error {
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].offset < ranges[j].offset
	})
	for i := 1; i < len(ranges); i++ {
		if ranges[i].offset < ranges[i-1].end {
			return errors.New(message)
		}
	}
	return nil
}

func validatePaddingZeros(in api.Stream, from, to uint32) error {
	if to < from || to-from > 3 {
		return errors.New("invalid WOFF block padding")
	}
	if from == to {
		return nil
	}
	padding := make([]byte, int(to-from))
	if _, err := in.ReadAt(padding, int64(from)); err != nil {
		return err
	}
	for _, b := range padding {
		if b != 0 {
			return errors.New("invalid WOFF block padding")
		}
	}
	return nil
}

// DecodeWOFF2 reconstructs an SFNT or TTC font from a WOFF2 stream.
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
	declaredLength := binary.BigEndian.Uint32(headerData[8:12])
	numTables := binary.BigEndian.Uint16(headerData[12:14])
	reserved := binary.BigEndian.Uint16(headerData[14:16])
	totalSfntSize := binary.BigEndian.Uint32(headerData[16:20])
	totalCompressedSize := binary.BigEndian.Uint32(headerData[20:24])
	metaOffset := binary.BigEndian.Uint32(headerData[28:32])
	metaLength := binary.BigEndian.Uint32(headerData[32:36])
	metaOrigLength := binary.BigEndian.Uint32(headerData[36:40])
	privOffset := binary.BigEndian.Uint32(headerData[40:44])
	privLength := binary.BigEndian.Uint32(headerData[44:48])
	if reserved != 0 {
		return nil, errors.New("invalid WOFF2 reserved field")
	}
	if uint64(declaredLength) != uint64(in.Size()) {
		return nil, errors.New("invalid WOFF2 file length")
	}
	if totalSfntSize > maxDecodedFontSize {
		return nil, errors.New("WOFF2 decoded font too large")
	}
	if totalCompressedSize > maxCompressedFontDataSize {
		return nil, errors.New("WOFF2 compressed data too large")
	}

	r := io.NewSectionReader(readerAtStream{in}, 48, in.Size()-48)

	tables := make([]woff2TableEntry, numTables)
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
		switch tables[i].tag {
		case tagGlyf, tagLoca:
			switch transformVersion {
			case woff2TransformGlyfLoca:
				transformLen, err := readBase128(r)
				if err != nil {
					return nil, err
				}
				tables[i].transformLength = transformLen
			case woff2TransformNull:
				tables[i].transformLength = tables[i].origLength
			default:
				return nil, fmt.Errorf("unsupported WOFF2 %s transform version %d", tagString(tables[i].tag), transformVersion)
			}
		case tagHmtx:
			switch transformVersion {
			case 0:
				tables[i].transformLength = tables[i].origLength
			case woff2TransformHmtx:
				transformLen, err := readBase128(r)
				if err != nil {
					return nil, err
				}
				tables[i].transformLength = transformLen
			default:
				return nil, fmt.Errorf("unsupported WOFF2 %s transform version %d", tagString(tables[i].tag), transformVersion)
			}
		default:
			if transformVersion != 0 {
				return nil, fmt.Errorf("unsupported WOFF2 %s transform version %d", tagString(tables[i].tag), transformVersion)
			}
			tables[i].transformLength = tables[i].origLength
		}
		if tables[i].transformLength > maxDecodedFontSize {
			return nil, errors.New("WOFF2 decoded font too large")
		}
	}

	var collection *woff2Collection
	if flavor == tagTTCF {
		var err error
		collection, err = readWOFF2CollectionDirectory(r, len(tables))
		if err != nil {
			return nil, err
		}
		if err := validateWOFF2CollectionGlyfLocaPairs(collection, tables); err != nil {
			return nil, err
		}
	}

	expectedUncompressedSize := uint64(0)
	for _, t := range tables {
		expectedUncompressedSize += uint64(t.transformLength)
		if expectedUncompressedSize > uint64(maxDecodedFontSize) {
			return nil, errors.New("WOFF2 decoded font too large")
		}
	}

	compressedOffset, _ := r.Seek(0, io.SeekCurrent)
	compressedOffset += 48

	if uint64(compressedOffset)+uint64(totalCompressedSize) > uint64(in.Size()) {
		return nil, errors.New("invalid WOFF2 compressed size")
	}
	if err := validateWOFF2MetadataPrivateBlocks(in, declaredLength, compressedOffset, totalCompressedSize, metaOffset, metaLength, metaOrigLength, privOffset, privLength); err != nil {
		return nil, err
	}

	compData := make([]byte, totalCompressedSize)
	if _, err := in.ReadAt(compData, compressedOffset); err != nil {
		return nil, err
	}

	br := brotli.NewReader(bytes.NewReader(compData))
	uncompressedData, err := readAllLimited(br, int64(expectedUncompressedSize))
	if err != nil {
		return nil, err
	}
	if uint64(len(uncompressedData)) != expectedUncompressedSize {
		return nil, errors.New("WOFF2 decompressed size mismatch")
	}

	uncompressedOffset := uint32(0)
	tableData := make([][]byte, len(tables))
	for i, t := range tables {
		if uncompressedOffset+t.transformLength > uint32(len(uncompressedData)) {
			return nil, errors.New("WOFF2 uncompressed data too short")
		}
		tableData[i] = uncompressedData[uncompressedOffset : uncompressedOffset+t.transformLength]
		uncompressedOffset += t.transformLength
	}

	reconstructed := make([][]byte, len(tables))
	for i, t := range tables {
		if reconstructed[i] != nil {
			continue
		}

		transformVersion := t.flags >> 6
		switch {
		case t.tag == tagGlyf && transformVersion == woff2TransformGlyfLoca:
			locaIndex, err := findWOFF2TransformedLoca(tables, i, collection != nil)
			if err != nil {
				return nil, err
			}
			locaTable := tables[locaIndex]
			glyfData, locaData, err := reconstructWOFF2GlyfLoca(tableData[i], locaTable.origLength)
			if err != nil {
				return nil, err
			}
			reconstructed[i] = glyfData
			reconstructed[locaIndex] = locaData

		case t.tag == tagLoca && transformVersion == woff2TransformGlyfLoca:
			if collection != nil {
				return nil, errors.New("WOFF2 transformed loca table encountered before glyf reconstruction")
			}
			continue

		case (t.tag == tagGlyf || t.tag == tagLoca) && transformVersion == woff2TransformNull:
			if uint32(len(tableData[i])) != t.origLength {
				return nil, errors.New("WOFF2 table length mismatch")
			}
			reconstructed[i] = append([]byte(nil), tableData[i]...)

		case t.tag == tagHmtx && transformVersion == woff2TransformHmtx:
			// hmtx needs hhea/maxp plus reconstructed glyf/loca xMin data, so
			// it is handled in a second pass after the outline tables are ready.
			continue

		case transformVersion == 0:
			if uint32(len(tableData[i])) != t.origLength {
				return nil, errors.New("WOFF2 table length mismatch")
			}
			reconstructed[i] = append([]byte(nil), tableData[i]...)

		default:
			return nil, fmt.Errorf("unsupported WOFF2 %s transform version %d", tagString(t.tag), transformVersion)
		}
	}

	for i, t := range tables {
		if t.tag != tagHmtx || t.flags>>6 != woff2TransformHmtx {
			continue
		}
		hmtxData, err := reconstructWOFF2HmtxForTable(i, tableData[i], t.origLength, tables, reconstructed, collection)
		if err != nil {
			return nil, err
		}
		reconstructed[i] = hmtxData
	}

	if err := validateWOFF2ReconstructedOutlines(tables, reconstructed, collection); err != nil {
		return nil, err
	}

	if collection != nil {
		outData, err := buildTTC(collection, tables, reconstructed, int(totalSfntSize))
		if err != nil {
			return nil, err
		}
		return core.NewMemoryStream(outData), nil
	}

	outTables := make([]sfntOutputTable, 0, len(tables))
	seenTags := make(map[uint32]struct{}, len(tables))
	for i, t := range tables {
		if reconstructed[i] == nil {
			return nil, fmt.Errorf("WOFF2 table %s was not reconstructed", tagString(t.tag))
		}
		if _, ok := seenTags[t.tag]; ok {
			return nil, fmt.Errorf("duplicate WOFF2 table %s", tagString(t.tag))
		}
		seenTags[t.tag] = struct{}{}
		outTables = append(outTables, sfntOutputTable{tag: t.tag, data: reconstructed[i]})
	}

	outData, err := buildSFNT(flavor, outTables, int(totalSfntSize))
	if err != nil {
		return nil, err
	}
	return core.NewMemoryStream(outData), nil
}

type sfntOutputTable struct {
	tag      uint32
	data     []byte
	offset   uint32
	checksum uint32
}

func findWOFF2Table(tables []woff2TableEntry, tag uint32) int {
	for i, t := range tables {
		if t.tag == tag {
			return i
		}
	}
	return -1
}

func findWOFF2TransformedLoca(tables []woff2TableEntry, glyfIndex int, requireNext bool) (int, error) {
	if glyfIndex < 0 || glyfIndex >= len(tables) || !isWOFF2TransformedGlyf(tables[glyfIndex]) {
		return -1, errors.New("WOFF2 glyf/loca transform mismatch")
	}
	if requireNext {
		locaIndex := glyfIndex + 1
		if locaIndex >= len(tables) || !isWOFF2TransformedLoca(tables[locaIndex]) {
			return -1, errors.New("WOFF2 transformed glyf table must be followed by loca table")
		}
		if tables[locaIndex].transformLength != 0 {
			return -1, errors.New("WOFF2 transformed loca table must have zero length")
		}
		return locaIndex, nil
	}

	locaIndex := -1
	for i := glyfIndex + 1; i < len(tables); i++ {
		t := tables[i]
		if t.tag != tagLoca {
			continue
		}
		if t.flags>>6 != woff2TransformGlyfLoca {
			return -1, errors.New("WOFF2 glyf/loca transform mismatch")
		}
		if t.transformLength != 0 {
			return -1, errors.New("WOFF2 transformed loca table must have zero length")
		}
		if locaIndex >= 0 {
			return -1, errors.New("duplicate WOFF2 table loca")
		}
		locaIndex = i
	}
	if locaIndex < 0 {
		return -1, errors.New("WOFF2 transformed glyf table requires transformed loca table")
	}
	return locaIndex, nil
}

func validateWOFF2CollectionGlyfLocaPairs(collection *woff2Collection, tables []woff2TableEntry) error {
	if collection == nil {
		return nil
	}

	pairedLocaByGlyf := make(map[int]int)
	pairedGlyfByLoca := make(map[int]int)
	for i, t := range tables {
		if !isWOFF2TransformedGlyf(t) {
			continue
		}
		locaIndex, err := findWOFF2TransformedLoca(tables, i, true)
		if err != nil {
			return err
		}
		pairedLocaByGlyf[i] = locaIndex
		pairedGlyfByLoca[locaIndex] = i
	}
	for i, t := range tables {
		if isWOFF2TransformedLoca(t) {
			if _, ok := pairedGlyfByLoca[i]; !ok {
				return errors.New("WOFF2 transformed loca table encountered before glyf reconstruction")
			}
		}
	}

	for _, font := range collection.fonts {
		fontIndices := make(map[int]struct{}, len(font.indices))
		for _, index := range font.indices {
			fontIndices[int(index)] = struct{}{}
		}
		for index := range fontIndices {
			if locaIndex, ok := pairedLocaByGlyf[index]; ok {
				if _, hasLoca := fontIndices[locaIndex]; !hasLoca {
					return errors.New("WOFF2 collection font has mismatched glyf/loca tables")
				}
			}
			if glyfIndex, ok := pairedGlyfByLoca[index]; ok {
				if _, hasGlyf := fontIndices[glyfIndex]; !hasGlyf {
					return errors.New("WOFF2 collection font has mismatched glyf/loca tables")
				}
			}
		}
	}
	return nil
}

func isWOFF2TransformedGlyf(t woff2TableEntry) bool {
	return t.tag == tagGlyf && t.flags>>6 == woff2TransformGlyfLoca
}

func isWOFF2TransformedLoca(t woff2TableEntry) bool {
	return t.tag == tagLoca && t.flags>>6 == woff2TransformGlyfLoca
}

func validateWOFF2ReconstructedOutlines(tables []woff2TableEntry, reconstructed [][]byte, collection *woff2Collection) error {
	if collection == nil {
		return validateWOFF2FontOutlines(tables, reconstructed)
	}
	for _, font := range collection.fonts {
		fontTables, fontReconstructed, err := collectionFontTableSlices(font, tables, reconstructed, -1)
		if err != nil {
			return err
		}
		if err := validateWOFF2FontOutlines(fontTables, fontReconstructed); err != nil {
			return err
		}
	}
	return nil
}

func validateWOFF2FontOutlines(tables []woff2TableEntry, reconstructed [][]byte) error {
	glyfIndex := findWOFF2Table(tables, tagGlyf)
	locaIndex := findWOFF2Table(tables, tagLoca)
	if glyfIndex < 0 && locaIndex < 0 {
		return nil
	}
	if glyfIndex < 0 || locaIndex < 0 || glyfIndex >= len(reconstructed) || locaIndex >= len(reconstructed) ||
		reconstructed[glyfIndex] == nil || reconstructed[locaIndex] == nil {
		return errors.New("WOFF2 font has incomplete glyf/loca tables")
	}

	headIndex := findWOFF2Table(tables, tagHead)
	maxpIndex := findWOFF2Table(tables, tagMaxp)
	if headIndex < 0 || maxpIndex < 0 {
		return nil
	}
	if headIndex >= len(reconstructed) || maxpIndex >= len(reconstructed) ||
		reconstructed[headIndex] == nil || reconstructed[maxpIndex] == nil {
		return nil
	}

	head := reconstructed[headIndex]
	maxp := reconstructed[maxpIndex]
	if len(head) < 54 || len(maxp) < 6 {
		return errors.New("WOFF2 glyf/loca metadata is incomplete")
	}
	indexFormat := int16(binary.BigEndian.Uint16(head[50:52]))
	if indexFormat != 0 && indexFormat != 1 {
		return errors.New("invalid WOFF2 glyf/loca index format")
	}
	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:6]))
	expectedLocaLength := (numGlyphs + 1) * (2 + 2*int(indexFormat))
	if len(reconstructed[locaIndex]) != expectedLocaLength {
		return errors.New("WOFF2 glyf/loca metadata mismatch")
	}
	glyf := reconstructed[glyfIndex]
	loca := reconstructed[locaIndex]
	if err := validateWOFF2LocaOffsets(glyf, loca, uint16(indexFormat), numGlyphs); err != nil {
		return err
	}
	return validateWOFF2GlyfCompositeReferences(glyf, loca, uint16(indexFormat), numGlyphs)
}

func tagString(tag uint32) string {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], tag)
	return string(b[:])
}

func align4(n uint32) uint32 {
	return (n + 3) &^ 3
}

func buildSFNT(flavor uint32, tables []sfntOutputTable, capacityHint int) ([]byte, error) {
	sort.Slice(tables, func(i, j int) bool {
		return tables[i].tag < tables[j].tag
	})

	numTables := uint16(len(tables))
	currentOffset := uint32(12) + uint32(numTables)*16
	headOffset := -1
	for i := range tables {
		if uint64(len(tables[i].data)) > uint64(maxDecodedFontSize) {
			return nil, errors.New("WOFF2 decoded font too large")
		}
		if tables[i].tag == tagHead {
			if len(tables[i].data) < 12 {
				return nil, errors.New("invalid WOFF2 head table")
			}
			tables[i].data = append([]byte(nil), tables[i].data...)
			binary.BigEndian.PutUint32(tables[i].data[8:12], 0)
			headOffset = i
		}
		tables[i].offset = currentOffset
		length := uint32(len(tables[i].data))
		paddedLength := align4(length)
		if paddedLength < length || uint64(currentOffset)+uint64(paddedLength) > uint64(maxDecodedFontSize) {
			return nil, errors.New("WOFF2 decoded font too large")
		}
		tables[i].checksum = sfntChecksum(tables[i].data)
		currentOffset += paddedLength
	}

	searchRange, entrySelector, rangeShift := sfntSearchParams(numTables)
	outCapacity := int(currentOffset)
	if capacityHint > outCapacity {
		outCapacity = capacityHint
	}
	outBuf := bytes.NewBuffer(make([]byte, 0, outCapacity))

	sfntHeader := make([]byte, 12)
	binary.BigEndian.PutUint32(sfntHeader[0:4], flavor)
	binary.BigEndian.PutUint16(sfntHeader[4:6], numTables)
	binary.BigEndian.PutUint16(sfntHeader[6:8], searchRange)
	binary.BigEndian.PutUint16(sfntHeader[8:10], entrySelector)
	binary.BigEndian.PutUint16(sfntHeader[10:12], rangeShift)
	outBuf.Write(sfntHeader)

	sfntDir := make([]byte, int(numTables)*16)
	for i, t := range tables {
		dirOff := i * 16
		binary.BigEndian.PutUint32(sfntDir[dirOff:dirOff+4], t.tag)
		binary.BigEndian.PutUint32(sfntDir[dirOff+4:dirOff+8], t.checksum)
		binary.BigEndian.PutUint32(sfntDir[dirOff+8:dirOff+12], t.offset)
		binary.BigEndian.PutUint32(sfntDir[dirOff+12:dirOff+16], uint32(len(t.data)))
	}
	outBuf.Write(sfntDir)

	for _, t := range tables {
		outBuf.Write(t.data)
		padding := int(align4(uint32(len(t.data))) - uint32(len(t.data)))
		if padding > 0 {
			outBuf.Write(make([]byte, padding))
		}
	}

	out := outBuf.Bytes()
	if headOffset >= 0 {
		adjustmentOffset := int(tables[headOffset].offset) + 8
		if adjustmentOffset+4 > len(out) {
			return nil, errors.New("invalid WOFF2 head table offset")
		}
		binary.BigEndian.PutUint32(out[adjustmentOffset:adjustmentOffset+4], 0xb1b0afba-sfntChecksum(out))
	}
	return out, nil
}

func buildTTC(collection *woff2Collection, tables []woff2TableEntry, reconstructed [][]byte, capacityHint int) ([]byte, error) {
	if collection == nil || len(collection.fonts) == 0 {
		return nil, errors.New("invalid WOFF2 collection")
	}
	if collection.version != 0x00010000 && collection.version != 0x00020000 {
		return nil, errors.New("invalid WOFF2 collection version")
	}

	used := make([]bool, len(tables))
	for _, font := range collection.fonts {
		if len(font.indices) > 0xffff {
			return nil, errors.New("WOFF2 collection font has too many tables")
		}
		seenTags := make(map[uint32]struct{}, len(font.indices))
		for _, index := range font.indices {
			if int(index) >= len(tables) || reconstructed[index] == nil {
				return nil, errors.New("WOFF2 collection references missing table")
			}
			tag := tables[index].tag
			if _, ok := seenTags[tag]; ok {
				return nil, fmt.Errorf("WOFF2 collection font has duplicate %s table", tagString(tag))
			}
			seenTags[tag] = struct{}{}
			if uint64(len(reconstructed[index])) > uint64(maxDecodedFontSize) {
				return nil, errors.New("WOFF2 decoded font too large")
			}
			used[index] = true
		}
	}

	outTableData := make([][]byte, len(reconstructed))
	for i := range tables {
		if !used[i] {
			continue
		}
		data := reconstructed[i]
		if tables[i].tag == tagHead {
			if len(data) < 12 {
				return nil, errors.New("invalid WOFF2 collection head table")
			}
			data = append([]byte(nil), data...)
			binary.BigEndian.PutUint32(data[8:12], 0)
		}
		outTableData[i] = data
	}

	numFonts := len(collection.fonts)
	headerSize := uint32(12 + 4*numFonts)
	if collection.version == 0x00020000 {
		headerSize += 12
	}
	currentOffset := headerSize
	fontOffsets := make([]uint32, numFonts)
	for i, font := range collection.fonts {
		fontOffsets[i] = currentOffset
		fontDirSize := uint32(12 + len(font.indices)*16)
		if uint64(currentOffset)+uint64(fontDirSize) > uint64(maxDecodedFontSize) {
			return nil, errors.New("WOFF2 decoded font too large")
		}
		currentOffset += fontDirSize
	}

	tableOffsets := make([]uint32, len(tables))
	tableChecksums := make([]uint32, len(tables))
	for i := range tables {
		if !used[i] {
			continue
		}
		data := outTableData[i]
		tableOffsets[i] = currentOffset
		tableChecksums[i] = sfntChecksum(data)
		length := uint32(len(data))
		paddedLength := align4(length)
		if paddedLength < length || uint64(currentOffset)+uint64(paddedLength) > uint64(maxDecodedFontSize) {
			return nil, errors.New("WOFF2 decoded font too large")
		}
		currentOffset += paddedLength
	}

	outCapacity := int(currentOffset)
	if capacityHint > outCapacity {
		outCapacity = capacityHint
	}
	out := bytes.NewBuffer(make([]byte, 0, outCapacity))
	appendUint32(out, tagTTCF)
	appendUint32(out, collection.version)
	appendUint32(out, uint32(numFonts))
	for _, offset := range fontOffsets {
		appendUint32(out, offset)
	}
	if collection.version == 0x00020000 {
		appendUint32(out, 0)
		appendUint32(out, 0)
		appendUint32(out, 0)
	}

	for fontIndex, font := range collection.fonts {
		if uint32(out.Len()) != fontOffsets[fontIndex] {
			return nil, errors.New("invalid WOFF2 collection offset layout")
		}
		fontTables := make([]uint16, len(font.indices))
		copy(fontTables, font.indices)
		sort.Slice(fontTables, func(i, j int) bool {
			return tables[fontTables[i]].tag < tables[fontTables[j]].tag
		})
		searchRange, entrySelector, rangeShift := sfntSearchParams(uint16(len(fontTables)))
		appendUint32(out, font.flavor)
		appendUint16(out, uint16(len(fontTables)))
		appendUint16(out, searchRange)
		appendUint16(out, entrySelector)
		appendUint16(out, rangeShift)
		for _, tableIndex := range fontTables {
			t := tables[tableIndex]
			data := outTableData[tableIndex]
			appendUint32(out, t.tag)
			appendUint32(out, tableChecksums[tableIndex])
			appendUint32(out, tableOffsets[tableIndex])
			appendUint32(out, uint32(len(data)))
		}
	}

	for i := range tables {
		if !used[i] {
			continue
		}
		if uint32(out.Len()) != tableOffsets[i] {
			return nil, errors.New("invalid WOFF2 collection table layout")
		}
		data := outTableData[i]
		out.Write(data)
		padding := int(align4(uint32(len(data))) - uint32(len(data)))
		if padding > 0 {
			out.Write(make([]byte, padding))
		}
	}
	return out.Bytes(), nil
}

func sfntSearchParams(numTables uint16) (searchRange, entrySelector, rangeShift uint16) {
	var maxPower2 uint16 = 1
	for maxPower2*2 <= numTables {
		maxPower2 *= 2
		entrySelector++
	}
	searchRange = maxPower2 * 16
	rangeShift = numTables*16 - searchRange
	return searchRange, entrySelector, rangeShift
}

func sfntChecksum(data []byte) uint32 {
	var sum uint32
	for i := 0; i < len(data); i += 4 {
		var word uint32
		for j := 0; j < 4; j++ {
			word <<= 8
			if i+j < len(data) {
				word |= uint32(data[i+j])
			}
		}
		sum += word
	}
	return sum
}

type woff2ByteReader struct {
	data []byte
	off  int
}

func newWOFF2ByteReader(data []byte) *woff2ByteReader {
	return &woff2ByteReader{data: data}
}

func (r *woff2ByteReader) remaining() int {
	return len(r.data) - r.off
}

func (r *woff2ByteReader) consumed() int {
	return r.off
}

func (r *woff2ByteReader) readU8() (byte, error) {
	if r.remaining() < 1 {
		return 0, io.ErrUnexpectedEOF
	}
	v := r.data[r.off]
	r.off++
	return v, nil
}

func (r *woff2ByteReader) readU16() (uint16, error) {
	if r.remaining() < 2 {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.BigEndian.Uint16(r.data[r.off : r.off+2])
	r.off += 2
	return v, nil
}

func (r *woff2ByteReader) readBytes(n int) ([]byte, error) {
	if n < 0 || r.remaining() < n {
		return nil, io.ErrUnexpectedEOF
	}
	v := r.data[r.off : r.off+n]
	r.off += n
	return v, nil
}

func (r *woff2ByteReader) read255UInt16() (uint16, error) {
	code, err := r.readU8()
	if err != nil {
		return 0, err
	}
	switch code {
	case 253:
		return r.readU16()
	case 254:
		b, err := r.readU8()
		if err != nil {
			return 0, err
		}
		return uint16(b) + 506, nil
	case 255:
		b, err := r.readU8()
		if err != nil {
			return 0, err
		}
		return uint16(b) + 253, nil
	default:
		return uint16(code), nil
	}
}

type woff2Point struct {
	x, y    int
	onCurve bool
}

func reconstructWOFF2GlyfLoca(data []byte, locaOrigLength uint32) ([]byte, []byte, error) {
	if len(data) < 36 {
		return nil, nil, errors.New("invalid WOFF2 transformed glyf table")
	}

	reserved := binary.BigEndian.Uint16(data[0:2])
	optionFlags := binary.BigEndian.Uint16(data[2:4])
	numGlyphs := binary.BigEndian.Uint16(data[4:6])
	indexFormat := binary.BigEndian.Uint16(data[6:8])
	if reserved != 0 {
		return nil, nil, errors.New("invalid WOFF2 glyf reserved field")
	}
	if optionFlags&^uint16(1) != 0 {
		return nil, nil, errors.New("invalid WOFF2 glyf option flags")
	}
	if indexFormat > 1 {
		return nil, nil, errors.New("invalid WOFF2 glyf index format")
	}

	var streamSizes [7]uint32
	streamOffset := uint32(36)
	for i := range streamSizes {
		sizeOff := 8 + i*4
		streamSizes[i] = binary.BigEndian.Uint32(data[sizeOff : sizeOff+4])
		if uint64(streamOffset)+uint64(streamSizes[i]) > uint64(len(data)) {
			return nil, nil, errors.New("invalid WOFF2 glyf substream size")
		}
		streamOffset += streamSizes[i]
	}

	bitmapLength := int(((uint32(numGlyphs) + 31) >> 5) << 2)
	if uint64(locaOrigLength) != (uint64(numGlyphs)+1)*uint64(2+2*indexFormat) {
		return nil, nil, errors.New("invalid WOFF2 transformed loca length")
	}
	if optionFlags&1 != 0 && len(data) < int(streamOffset)+bitmapLength {
		return nil, nil, errors.New("invalid WOFF2 glyf overlap bitmap")
	}

	substreams := make([][]byte, len(streamSizes))
	offset := uint32(36)
	for i, size := range streamSizes {
		substreams[i] = data[offset : offset+size]
		offset += size
	}

	overlapBitmap := []byte(nil)
	if optionFlags&1 != 0 {
		overlapBitmap = data[offset : offset+uint32(bitmapLength)]
		offset += uint32(bitmapLength)
	}
	if offset != uint32(len(data)) {
		return nil, nil, errors.New("invalid WOFF2 glyf trailing data")
	}

	nContourStream := newWOFF2ByteReader(substreams[0])
	nPointsStream := newWOFF2ByteReader(substreams[1])
	flagStream := newWOFF2ByteReader(substreams[2])
	glyphStream := newWOFF2ByteReader(substreams[3])
	compositeStream := newWOFF2ByteReader(substreams[4])
	bboxStream := newWOFF2ByteReader(substreams[5])
	instructionStream := newWOFF2ByteReader(substreams[6])
	if bboxStream.remaining() < bitmapLength {
		return nil, nil, errors.New("invalid WOFF2 bbox stream")
	}
	bboxBitmap, err := bboxStream.readBytes(bitmapLength)
	if err != nil {
		return nil, nil, err
	}

	locaValues := make([]uint32, int(numGlyphs)+1)
	glyf := bytes.NewBuffer(nil)
	for glyphIndex := 0; glyphIndex < int(numGlyphs); glyphIndex++ {
		locaValues[glyphIndex] = uint32(glyf.Len())
		nContoursRaw, err := nContourStream.readU16()
		if err != nil {
			return nil, nil, err
		}
		hasBBox := bitmapBit(bboxBitmap, glyphIndex)
		hasOverlap := len(overlapBitmap) > 0 && bitmapBit(overlapBitmap, glyphIndex)

		switch {
		case nContoursRaw == 0:
			if hasBBox {
				return nil, nil, errors.New("WOFF2 empty glyph has explicit bbox")
			}

		case nContoursRaw == 0xffff:
			if !hasBBox {
				return nil, nil, errors.New("WOFF2 composite glyph missing bbox")
			}
			glyphData, err := reconstructWOFF2CompositeGlyph(compositeStream, glyphStream, instructionStream, int(numGlyphs))
			if err != nil {
				return nil, nil, err
			}
			if err := applyWOFF2ExplicitBBox(glyphData, bboxStream); err != nil {
				return nil, nil, err
			}
			glyf.Write(glyphData)

		case nContoursRaw&0x8000 != 0:
			return nil, nil, errors.New("invalid WOFF2 glyph contour count")

		default:
			glyphData, err := reconstructWOFF2SimpleGlyph(int(nContoursRaw), nPointsStream, flagStream, glyphStream, instructionStream, hasOverlap)
			if err != nil {
				return nil, nil, err
			}
			if hasBBox {
				if err := applyWOFF2ExplicitBBox(glyphData, bboxStream); err != nil {
					return nil, nil, err
				}
			}
			glyf.Write(glyphData)
		}

		if glyf.Len() > maxDecodedFontSize {
			return nil, nil, errors.New("WOFF2 decoded font too large")
		}
		for glyf.Len()%4 != 0 {
			glyf.WriteByte(0)
		}
	}
	locaValues[len(locaValues)-1] = uint32(glyf.Len())

	if nContourStream.remaining() != 0 || nPointsStream.remaining() != 0 || flagStream.remaining() != 0 ||
		glyphStream.remaining() != 0 || compositeStream.remaining() != 0 || bboxStream.remaining() != 0 ||
		instructionStream.remaining() != 0 {
		return nil, nil, errors.New("WOFF2 glyf substream length mismatch")
	}

	loca, err := buildWOFF2Loca(locaValues, indexFormat, locaOrigLength)
	if err != nil {
		return nil, nil, err
	}
	return glyf.Bytes(), loca, nil
}

func reconstructWOFF2SimpleGlyph(nContours int, nPointsStream, flagStream, glyphStream, instructionStream *woff2ByteReader, overlap bool) ([]byte, error) {
	endPts := make([]uint16, nContours)
	totalPoints := 0
	for i := 0; i < nContours; i++ {
		nPoints, err := nPointsStream.read255UInt16()
		if err != nil {
			return nil, err
		}
		if nPoints == 0 {
			return nil, errors.New("invalid WOFF2 simple glyph contour")
		}
		totalPoints += int(nPoints)
		if totalPoints > 65535 {
			return nil, errors.New("WOFF2 simple glyph has too many points")
		}
		endPts[i] = uint16(totalPoints - 1)
	}

	flags, err := flagStream.readBytes(totalPoints)
	if err != nil {
		return nil, err
	}
	points, err := decodeWOFF2Triplets(flags, glyphStream)
	if err != nil {
		return nil, err
	}
	instructionLength, err := glyphStream.read255UInt16()
	if err != nil {
		return nil, err
	}
	if int(instructionLength) > instructionStream.remaining() {
		return nil, errors.New("invalid WOFF2 simple glyph instructions")
	}
	instructions, err := instructionStream.readBytes(int(instructionLength))
	if err != nil {
		return nil, err
	}

	glyph := bytes.NewBuffer(make([]byte, 0, 10+2*nContours+2+len(instructions)+len(points)*3))
	appendInt16(glyph, int16(nContours))
	appendGlyphBBox(glyph, points)
	for _, endPt := range endPts {
		appendUint16(glyph, endPt)
	}
	appendUint16(glyph, instructionLength)
	glyph.Write(instructions)
	if err := appendTrueTypePointData(glyph, points, overlap); err != nil {
		return nil, err
	}
	return glyph.Bytes(), nil
}

func reconstructWOFF2CompositeGlyph(compositeStream, glyphStream, instructionStream *woff2ByteReader, numGlyphs int) ([]byte, error) {
	const (
		flagArgWords                = 1 << 0
		flagMoreComponents          = 1 << 5
		flagHaveScale               = 1 << 3
		flagHaveXYScale             = 1 << 6
		flagHaveTwoByTwo            = 1 << 7
		flagHaveInstructions        = 1 << 8
		flagScaledComponentOffset   = 1 << 11
		flagUnscaledComponentOffset = 1 << 12
		flagReservedComposite       = 0xe010
		compositeGlyphHeader        = 10
		compositeBBoxPosition       = 2
	)

	glyph := bytes.NewBuffer(make([]byte, 0, compositeGlyphHeader+16))
	appendUint16(glyph, 0xffff)
	glyph.Write(make([]byte, 8))

	haveInstructions := false
	for {
		flagsBytes, err := compositeStream.readBytes(2)
		if err != nil {
			return nil, err
		}
		flags := binary.BigEndian.Uint16(flagsBytes)
		haveInstructions = haveInstructions || flags&flagHaveInstructions != 0
		glyph.Write(flagsBytes)

		if flags&flagReservedComposite != 0 {
			return nil, errors.New("invalid WOFF2 composite flags")
		}
		if flags&flagScaledComponentOffset != 0 && flags&flagUnscaledComponentOffset != 0 {
			return nil, errors.New("invalid WOFF2 composite offset flags")
		}
		transformCount := 0
		if flags&flagHaveScale != 0 {
			transformCount++
		}
		if flags&flagHaveXYScale != 0 {
			transformCount++
		}
		if flags&flagHaveTwoByTwo != 0 {
			transformCount++
		}
		if transformCount > 1 {
			return nil, errors.New("invalid WOFF2 composite transform flags")
		}

		componentGlyphIndex, err := compositeStream.readU16()
		if err != nil {
			return nil, err
		}
		if numGlyphs < 0 || int(componentGlyphIndex) >= numGlyphs {
			return nil, errors.New("invalid WOFF2 composite glyph index")
		}
		appendUint16(glyph, componentGlyphIndex)

		argSize := 2
		if flags&flagArgWords != 0 {
			argSize = 4
		}
		if flags&flagHaveScale != 0 {
			argSize += 2
		} else if flags&flagHaveXYScale != 0 {
			argSize += 4
		} else if flags&flagHaveTwoByTwo != 0 {
			argSize += 8
		}
		args, err := compositeStream.readBytes(argSize)
		if err != nil {
			return nil, err
		}
		glyph.Write(args)
		if flags&flagMoreComponents == 0 {
			break
		}
	}

	if haveInstructions {
		instructionLength, err := glyphStream.read255UInt16()
		if err != nil {
			return nil, err
		}
		appendUint16(glyph, instructionLength)
		instructions, err := instructionStream.readBytes(int(instructionLength))
		if err != nil {
			return nil, err
		}
		glyph.Write(instructions)
	}

	out := glyph.Bytes()
	if len(out) < compositeBBoxPosition+8 {
		return nil, errors.New("invalid WOFF2 composite glyph")
	}
	return out, nil
}

func decodeWOFF2Triplets(flags []byte, glyphStream *woff2ByteReader) ([]woff2Point, error) {
	points := make([]woff2Point, 0, len(flags))
	x, y := 0, 0
	for _, rawFlag := range flags {
		onCurve := rawFlag>>7 == 0
		flag := rawFlag & 0x7f
		var dataBytes int
		switch {
		case flag < 84:
			dataBytes = 1
		case flag < 120:
			dataBytes = 2
		case flag < 124:
			dataBytes = 3
		default:
			dataBytes = 4
		}
		data, err := glyphStream.readBytes(dataBytes)
		if err != nil {
			return nil, err
		}

		dx, dy := 0, 0
		switch {
		case flag < 10:
			dy = withWOFF2Sign(int(flag), (int(flag&14)<<7)+int(data[0]))
		case flag < 20:
			dx = withWOFF2Sign(int(flag), (int((flag-10)&14)<<7)+int(data[0]))
		case flag < 84:
			b0 := int(flag - 20)
			b1 := int(data[0])
			dx = withWOFF2Sign(int(flag), 1+(b0&0x30)+(b1>>4))
			dy = withWOFF2Sign(int(flag>>1), 1+((b0&0x0c)<<2)+(b1&0x0f))
		case flag < 120:
			b0 := int(flag - 84)
			dx = withWOFF2Sign(int(flag), 1+((b0/12)<<8)+int(data[0]))
			dy = withWOFF2Sign(int(flag>>1), 1+(((b0%12)>>2)<<8)+int(data[1]))
		case flag < 124:
			b2 := int(data[1])
			dx = withWOFF2Sign(int(flag), (int(data[0])<<4)+(b2>>4))
			dy = withWOFF2Sign(int(flag>>1), ((b2&0x0f)<<8)+int(data[2]))
		default:
			dx = withWOFF2Sign(int(flag), (int(data[0])<<8)+int(data[1]))
			dy = withWOFF2Sign(int(flag>>1), (int(data[2])<<8)+int(data[3]))
		}

		x += dx
		y += dy
		if x < -32768 || x > 32767 || y < -32768 || y > 32767 {
			return nil, errors.New("WOFF2 glyph coordinate out of range")
		}
		points = append(points, woff2Point{x: x, y: y, onCurve: onCurve})
	}
	return points, nil
}

func withWOFF2Sign(flag, value int) int {
	if flag&1 != 0 {
		return value
	}
	return -value
}

func appendTrueTypePointData(buf *bytes.Buffer, points []woff2Point, overlap bool) error {
	const (
		flagOnCurve       = 1 << 0
		flagXShort        = 1 << 1
		flagYShort        = 1 << 2
		flagThisXIsSame   = 1 << 4
		flagThisYIsSame   = 1 << 5
		flagOverlapSimple = 1 << 6
	)

	flags := make([]byte, len(points))
	xBytes := bytes.NewBuffer(nil)
	yBytes := bytes.NewBuffer(nil)
	lastX, lastY := 0, 0
	for i, p := range points {
		flag := byte(0)
		if p.onCurve {
			flag |= flagOnCurve
		}
		if overlap && i == 0 {
			flag |= flagOverlapSimple
		}
		dx := p.x - lastX
		dy := p.y - lastY
		if dx < -32768 || dx > 32767 || dy < -32768 || dy > 32767 {
			return errors.New("WOFF2 glyph coordinate delta out of range")
		}
		if dx == 0 {
			flag |= flagThisXIsSame
		} else if dx > -256 && dx < 256 {
			flag |= flagXShort
			if dx > 0 {
				flag |= flagThisXIsSame
			}
			xBytes.WriteByte(byte(absInt(dx)))
		} else {
			appendInt16(xBytes, int16(dx))
		}
		if dy == 0 {
			flag |= flagThisYIsSame
		} else if dy > -256 && dy < 256 {
			flag |= flagYShort
			if dy > 0 {
				flag |= flagThisYIsSame
			}
			yBytes.WriteByte(byte(absInt(dy)))
		} else {
			appendInt16(yBytes, int16(dy))
		}
		flags[i] = flag
		lastX, lastY = p.x, p.y
	}
	buf.Write(flags)
	buf.Write(xBytes.Bytes())
	buf.Write(yBytes.Bytes())
	return nil
}

func appendGlyphBBox(buf *bytes.Buffer, points []woff2Point) {
	xMin, yMin, xMax, yMax := 0, 0, 0, 0
	for i, p := range points {
		if i == 0 || p.x < xMin {
			xMin = p.x
		}
		if i == 0 || p.x > xMax {
			xMax = p.x
		}
		if i == 0 || p.y < yMin {
			yMin = p.y
		}
		if i == 0 || p.y > yMax {
			yMax = p.y
		}
	}
	appendInt16(buf, int16(xMin))
	appendInt16(buf, int16(yMin))
	appendInt16(buf, int16(xMax))
	appendInt16(buf, int16(yMax))
}

func applyWOFF2ExplicitBBox(glyphData []byte, bboxStream *woff2ByteReader) error {
	if len(glyphData) < 10 {
		return errors.New("invalid WOFF2 glyph bbox target")
	}
	bbox, err := bboxStream.readBytes(8)
	if err != nil {
		return err
	}
	if err := validateWOFF2BBox(bbox); err != nil {
		return err
	}
	copy(glyphData[2:10], bbox)
	return nil
}

func validateWOFF2BBox(bbox []byte) error {
	if len(bbox) < 8 {
		return errors.New("invalid WOFF2 glyph bbox")
	}
	xMin := int16(binary.BigEndian.Uint16(bbox[0:2]))
	yMin := int16(binary.BigEndian.Uint16(bbox[2:4]))
	xMax := int16(binary.BigEndian.Uint16(bbox[4:6]))
	yMax := int16(binary.BigEndian.Uint16(bbox[6:8]))
	if xMin > xMax || yMin > yMax {
		return errors.New("invalid WOFF2 glyph bbox")
	}
	return nil
}

func buildWOFF2Loca(locaValues []uint32, indexFormat uint16, locaOrigLength uint32) ([]byte, error) {
	loca := make([]byte, locaOrigLength)
	if indexFormat == 0 {
		if len(locaValues)*2 != len(loca) {
			return nil, errors.New("invalid WOFF2 loca length")
		}
		for i, value := range locaValues {
			if value&1 != 0 || value > 0x1fffe {
				return nil, errors.New("WOFF2 loca short offset out of range")
			}
			binary.BigEndian.PutUint16(loca[i*2:i*2+2], uint16(value/2))
		}
		return loca, nil
	}
	if len(locaValues)*4 != len(loca) {
		return nil, errors.New("invalid WOFF2 loca length")
	}
	for i, value := range locaValues {
		binary.BigEndian.PutUint32(loca[i*4:i*4+4], value)
	}
	return loca, nil
}

func reconstructWOFF2Hmtx(data []byte, origLength uint32, tables []woff2TableEntry, reconstructed [][]byte) ([]byte, error) {
	if len(data) < 1 {
		return nil, errors.New("invalid WOFF2 transformed hmtx table")
	}
	flags := data[0]
	if flags == 0 || flags&^byte(3) != 0 {
		return nil, errors.New("invalid WOFF2 hmtx transform flags")
	}

	head, err := requiredWOFF2Table(tables, reconstructed, tagHead)
	if err != nil {
		return nil, err
	}
	hhea, err := requiredWOFF2Table(tables, reconstructed, tagHhea)
	if err != nil {
		return nil, err
	}
	maxp, err := requiredWOFF2Table(tables, reconstructed, tagMaxp)
	if err != nil {
		return nil, err
	}
	glyf, err := requiredWOFF2Table(tables, reconstructed, tagGlyf)
	if err != nil {
		return nil, err
	}
	loca, err := requiredWOFF2Table(tables, reconstructed, tagLoca)
	if err != nil {
		return nil, err
	}
	if len(head) < 54 || len(hhea) < 36 || len(maxp) < 6 {
		return nil, errors.New("WOFF2 hmtx transform missing required metrics metadata")
	}
	if err := validateWOFF2FontOutlines(tables, reconstructed); err != nil {
		return nil, err
	}

	indexFormat := int16(binary.BigEndian.Uint16(head[50:52]))
	if indexFormat != 0 && indexFormat != 1 {
		return nil, errors.New("invalid WOFF2 hmtx loca index format")
	}
	numHMetrics := int(binary.BigEndian.Uint16(hhea[34:36]))
	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:6]))
	if numHMetrics < 0 || numHMetrics > numGlyphs || (numGlyphs > 0 && numHMetrics == 0) {
		return nil, errors.New("invalid WOFF2 hmtx metrics count")
	}

	expectedLength := numHMetrics*4 + (numGlyphs-numHMetrics)*2
	if uint32(expectedLength) != origLength {
		return nil, errors.New("invalid WOFF2 hmtx original length")
	}

	r := newWOFF2ByteReader(data[1:])
	advanceWidths := make([]uint16, numHMetrics)
	for i := range advanceWidths {
		advanceWidth, err := r.readU16()
		if err != nil {
			return nil, err
		}
		advanceWidths[i] = advanceWidth
	}

	lsbs := make([]int16, numHMetrics)
	if flags&1 == 0 {
		for i := range lsbs {
			value, err := r.readU16()
			if err != nil {
				return nil, err
			}
			lsbs[i] = int16(value)
		}
	} else {
		for i := range lsbs {
			xMin, err := woff2GlyphXMin(glyf, loca, uint16(indexFormat), i)
			if err != nil {
				return nil, err
			}
			lsbs[i] = xMin
		}
	}

	leftSideBearings := make([]int16, numGlyphs-numHMetrics)
	if flags&2 == 0 {
		for i := range leftSideBearings {
			value, err := r.readU16()
			if err != nil {
				return nil, err
			}
			leftSideBearings[i] = int16(value)
		}
	} else {
		for i := range leftSideBearings {
			xMin, err := woff2GlyphXMin(glyf, loca, uint16(indexFormat), numHMetrics+i)
			if err != nil {
				return nil, err
			}
			leftSideBearings[i] = xMin
		}
	}
	if r.remaining() != 0 {
		return nil, errors.New("WOFF2 hmtx transform length mismatch")
	}

	out := bytes.NewBuffer(make([]byte, 0, expectedLength))
	for i, advanceWidth := range advanceWidths {
		appendUint16(out, advanceWidth)
		appendInt16(out, lsbs[i])
	}
	for _, lsb := range leftSideBearings {
		appendInt16(out, lsb)
	}
	return out.Bytes(), nil
}

func reconstructWOFF2HmtxForTable(tableIndex int, data []byte, origLength uint32, tables []woff2TableEntry, reconstructed [][]byte, collection *woff2Collection) ([]byte, error) {
	if collection == nil {
		return reconstructWOFF2Hmtx(data, origLength, tables, reconstructed)
	}
	var result []byte
	for _, font := range collection.fonts {
		if !collectionFontReferences(font, tableIndex) {
			continue
		}
		fontTables, fontReconstructed, err := collectionFontTableSlices(font, tables, reconstructed, tableIndex)
		if err != nil {
			return nil, err
		}
		hmtx, err := reconstructWOFF2Hmtx(data, origLength, fontTables, fontReconstructed)
		if err != nil {
			return nil, err
		}
		if result == nil {
			result = hmtx
			continue
		}
		if !bytes.Equal(result, hmtx) {
			return nil, errors.New("WOFF2 shared hmtx transform has inconsistent collection dependencies")
		}
	}
	if result == nil {
		return nil, errors.New("WOFF2 collection hmtx table is not referenced")
	}
	return result, nil
}

func collectionFontReferences(font woff2CollectionFont, tableIndex int) bool {
	if tableIndex < 0 || tableIndex > 0xffff {
		return false
	}
	for _, index := range font.indices {
		if int(index) == tableIndex {
			return true
		}
	}
	return false
}

func collectionFontTableSlices(font woff2CollectionFont, tables []woff2TableEntry, reconstructed [][]byte, missingOKIndex int) ([]woff2TableEntry, [][]byte, error) {
	fontTables := make([]woff2TableEntry, len(font.indices))
	fontReconstructed := make([][]byte, len(font.indices))
	for i, index := range font.indices {
		if int(index) >= len(tables) || int(index) >= len(reconstructed) {
			return nil, nil, errors.New("WOFF2 collection references missing table")
		}
		if reconstructed[index] == nil && int(index) != missingOKIndex {
			return nil, nil, errors.New("WOFF2 collection references missing table")
		}
		fontTables[i] = tables[index]
		fontReconstructed[i] = reconstructed[index]
	}
	return fontTables, fontReconstructed, nil
}

func requiredWOFF2Table(tables []woff2TableEntry, reconstructed [][]byte, tag uint32) ([]byte, error) {
	index := findWOFF2Table(tables, tag)
	if index < 0 || index >= len(reconstructed) || reconstructed[index] == nil {
		return nil, fmt.Errorf("WOFF2 hmtx transform requires %s table", tagString(tag))
	}
	return reconstructed[index], nil
}

func validateWOFF2LocaOffsets(glyf, loca []byte, indexFormat uint16, numGlyphs int) error {
	var prev uint32
	for i := 0; i <= numGlyphs; i++ {
		offset, err := woff2LocaOffset(loca, indexFormat, i)
		if err != nil {
			return err
		}
		if offset < prev {
			return errors.New("invalid WOFF2 loca offsets")
		}
		if offset > uint32(len(glyf)) {
			return errors.New("invalid WOFF2 glyf bounds")
		}
		if i > 0 {
			length := offset - prev
			if length > 0 && length < 10 {
				return errors.New("invalid WOFF2 glyf bounds")
			}
		}
		prev = offset
	}
	if prev != uint32(len(glyf)) {
		return errors.New("WOFF2 loca/glyf length mismatch")
	}
	return nil
}

func woff2LocaOffset(loca []byte, indexFormat uint16, glyphIndex int) (uint32, error) {
	if glyphIndex < 0 {
		return 0, errors.New("invalid WOFF2 glyph index")
	}
	if indexFormat == 0 {
		offset := glyphIndex * 2
		if offset+2 > len(loca) {
			return 0, errors.New("invalid WOFF2 loca bounds")
		}
		return uint32(binary.BigEndian.Uint16(loca[offset:offset+2])) * 2, nil
	}
	if indexFormat == 1 {
		offset := glyphIndex * 4
		if offset+4 > len(loca) {
			return 0, errors.New("invalid WOFF2 loca bounds")
		}
		return binary.BigEndian.Uint32(loca[offset : offset+4]), nil
	}
	return 0, errors.New("invalid WOFF2 loca index format")
}

func woff2GlyphXMin(glyf, loca []byte, indexFormat uint16, glyphIndex int) (int16, error) {
	start, end, err := woff2GlyphBounds(loca, indexFormat, glyphIndex)
	if err != nil {
		return 0, err
	}
	if start == end {
		return 0, nil
	}
	if end < start || end > uint32(len(glyf)) || end-start < 10 {
		return 0, errors.New("invalid WOFF2 glyf bounds")
	}
	return int16(binary.BigEndian.Uint16(glyf[start+2 : start+4])), nil
}

func woff2GlyphBounds(loca []byte, indexFormat uint16, glyphIndex int) (uint32, uint32, error) {
	if glyphIndex < 0 {
		return 0, 0, errors.New("invalid WOFF2 glyph index")
	}
	if indexFormat == 0 {
		offset := glyphIndex * 2
		if offset+4 > len(loca) {
			return 0, 0, errors.New("invalid WOFF2 loca bounds")
		}
		start := uint32(binary.BigEndian.Uint16(loca[offset:offset+2])) * 2
		end := uint32(binary.BigEndian.Uint16(loca[offset+2:offset+4])) * 2
		return start, end, nil
	}
	if indexFormat == 1 {
		offset := glyphIndex * 4
		if offset+8 > len(loca) {
			return 0, 0, errors.New("invalid WOFF2 loca bounds")
		}
		start := binary.BigEndian.Uint32(loca[offset : offset+4])
		end := binary.BigEndian.Uint32(loca[offset+4 : offset+8])
		return start, end, nil
	}
	return 0, 0, errors.New("invalid WOFF2 loca index format")
}

func validateWOFF2GlyfCompositeReferences(glyf, loca []byte, indexFormat uint16, numGlyphs int) error {
	for glyphIndex := 0; glyphIndex < numGlyphs; glyphIndex++ {
		start, end, err := woff2GlyphBounds(loca, indexFormat, glyphIndex)
		if err != nil {
			return err
		}
		if start == end {
			continue
		}
		if end < start || end > uint32(len(glyf)) || end-start < 10 {
			return errors.New("invalid WOFF2 glyf bounds")
		}

		glyph := glyf[start:end]
		if err := validateWOFF2BBox(glyph[2:10]); err != nil {
			return err
		}
		numberOfContours := int16(binary.BigEndian.Uint16(glyph[0:2]))
		switch {
		case numberOfContours >= 0:
			if err := validateWOFF2SimpleGlyph(glyph, int(numberOfContours)); err != nil {
				return err
			}
		case numberOfContours == -1:
			if err := validateWOFF2CompositeGlyphReferences(glyph, numGlyphs); err != nil {
				return err
			}
		default:
			return errors.New("invalid WOFF2 glyph contour count")
		}
	}
	return nil
}

func validateWOFF2SimpleGlyph(glyph []byte, nContours int) error {
	const (
		simpleGlyphHeader = 10
		flagXShort        = 1 << 1
		flagYShort        = 1 << 2
		flagRepeat        = 1 << 3
		flagThisXIsSame   = 1 << 4
		flagThisYIsSame   = 1 << 5
	)

	offset := simpleGlyphHeader
	if nContours < 0 || offset+2*nContours+2 > len(glyph) {
		return errors.New("invalid WOFF2 simple glyph")
	}

	totalPoints := 0
	prevEndPt := -1
	for i := 0; i < nContours; i++ {
		endPt := int(binary.BigEndian.Uint16(glyph[offset : offset+2]))
		offset += 2
		if endPt <= prevEndPt {
			return errors.New("invalid WOFF2 simple glyph contour endpoints")
		}
		prevEndPt = endPt
	}
	if nContours > 0 {
		totalPoints = prevEndPt + 1
	}

	instructionLength := int(binary.BigEndian.Uint16(glyph[offset : offset+2]))
	offset += 2
	if instructionLength > len(glyph)-offset {
		return errors.New("invalid WOFF2 simple glyph instructions")
	}
	offset += instructionLength

	flags := make([]byte, 0, totalPoints)
	for len(flags) < totalPoints {
		if offset >= len(glyph) {
			return errors.New("invalid WOFF2 simple glyph point data")
		}
		flag := glyph[offset]
		offset++
		repeat := 1
		if flag&flagRepeat != 0 {
			if offset >= len(glyph) {
				return errors.New("invalid WOFF2 simple glyph point data")
			}
			repeat += int(glyph[offset])
			offset++
		}
		if len(flags)+repeat > totalPoints {
			return errors.New("invalid WOFF2 simple glyph point data")
		}
		for i := 0; i < repeat; i++ {
			flags = append(flags, flag)
		}
	}

	coordinateLength := 0
	for _, flag := range flags {
		if flag&flagXShort != 0 {
			coordinateLength++
		} else if flag&flagThisXIsSame == 0 {
			coordinateLength += 2
		}
		if flag&flagYShort != 0 {
			coordinateLength++
		} else if flag&flagThisYIsSame == 0 {
			coordinateLength += 2
		}
	}
	if coordinateLength > len(glyph)-offset {
		return errors.New("invalid WOFF2 simple glyph point data")
	}
	offset += coordinateLength
	return validateWOFF2GlyphPadding(glyph[offset:])
}

func validateWOFF2CompositeGlyphReferences(glyph []byte, numGlyphs int) error {
	const (
		flagArgWords                = 1 << 0
		flagMoreComponents          = 1 << 5
		flagHaveScale               = 1 << 3
		flagHaveXYScale             = 1 << 6
		flagHaveTwoByTwo            = 1 << 7
		flagHaveInstructions        = 1 << 8
		flagScaledComponentOffset   = 1 << 11
		flagUnscaledComponentOffset = 1 << 12
		flagReservedComposite       = 0xe010
		compositeGlyphHeader        = 10
	)

	offset := compositeGlyphHeader
	haveInstructions := false
	for {
		if offset+4 > len(glyph) {
			return errors.New("invalid WOFF2 composite glyph")
		}
		flags := binary.BigEndian.Uint16(glyph[offset : offset+2])
		componentGlyphIndex := binary.BigEndian.Uint16(glyph[offset+2 : offset+4])
		offset += 4

		haveInstructions = haveInstructions || flags&flagHaveInstructions != 0
		if int(componentGlyphIndex) >= numGlyphs {
			return errors.New("invalid WOFF2 composite glyph index")
		}
		if flags&flagReservedComposite != 0 {
			return errors.New("invalid WOFF2 composite flags")
		}
		if flags&flagScaledComponentOffset != 0 && flags&flagUnscaledComponentOffset != 0 {
			return errors.New("invalid WOFF2 composite offset flags")
		}

		transformCount := 0
		if flags&flagHaveScale != 0 {
			transformCount++
		}
		if flags&flagHaveXYScale != 0 {
			transformCount++
		}
		if flags&flagHaveTwoByTwo != 0 {
			transformCount++
		}
		if transformCount > 1 {
			return errors.New("invalid WOFF2 composite transform flags")
		}

		componentLength := 2
		if flags&flagArgWords != 0 {
			componentLength = 4
		}
		if flags&flagHaveScale != 0 {
			componentLength += 2
		} else if flags&flagHaveXYScale != 0 {
			componentLength += 4
		} else if flags&flagHaveTwoByTwo != 0 {
			componentLength += 8
		}
		if offset+componentLength > len(glyph) {
			return errors.New("invalid WOFF2 composite glyph")
		}
		offset += componentLength

		if flags&flagMoreComponents == 0 {
			break
		}
	}

	if haveInstructions {
		if offset+2 > len(glyph) {
			return errors.New("invalid WOFF2 composite glyph")
		}
		instructionLength := int(binary.BigEndian.Uint16(glyph[offset : offset+2]))
		offset += 2
		if offset+instructionLength > len(glyph) {
			return errors.New("invalid WOFF2 composite glyph")
		}
		offset += instructionLength
	}
	if err := validateWOFF2GlyphPadding(glyph[offset:]); err != nil {
		return err
	}
	return nil
}

func validateWOFF2GlyphPadding(padding []byte) error {
	if len(padding) > 3 {
		return errors.New("invalid WOFF2 glyf padding")
	}
	for _, b := range padding {
		if b != 0 {
			return errors.New("invalid WOFF2 glyf padding")
		}
	}
	return nil
}

func bitmapBit(bitmap []byte, index int) bool {
	if index < 0 || index/8 >= len(bitmap) {
		return false
	}
	return bitmap[index>>3]&(0x80>>uint(index&7)) != 0
}

func appendUint16(buf *bytes.Buffer, value uint16) {
	var tmp [2]byte
	binary.BigEndian.PutUint16(tmp[:], value)
	buf.Write(tmp[:])
}

func appendUint32(buf *bytes.Buffer, value uint32) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], value)
	buf.Write(tmp[:])
}

func appendInt16(buf *bytes.Buffer, value int16) {
	appendUint16(buf, uint16(value))
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
