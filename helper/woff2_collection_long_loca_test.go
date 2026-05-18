package helper

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2CollectionLongLocaSharesTransformedGlyfLocaAcrossFaces(t *testing.T) {
	woff2Data := woff2CollectionLongLocaWOFF2(t, false)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	faceOffsets := woff2CollectionLongLocaFaceOffsets(t, outData)

	woff2CollectionLongLocaAssertSameBounds(t, outData, faceOffsets, tagGlyf)
	woff2CollectionLongLocaAssertSameBounds(t, outData, faceOffsets, tagLoca)

	for i, faceOffset := range faceOffsets {
		head := sfntTableAt(t, outData, faceOffset, tagHead)
		if got := int16(binary.BigEndian.Uint16(head[50:52])); got != 1 {
			t.Fatalf("face %d indexToLocFormat = %d, want 1", i, got)
		}

		assertSimpleWOFF2Glyph(t, sfntTableAt(t, outData, faceOffset, tagGlyf))

		loca := sfntTableAt(t, outData, faceOffset, tagLoca)
		if len(loca) != 8 {
			t.Fatalf("face %d loca length = %d, want 8", i, len(loca))
		}
		if got0, got1 := binary.BigEndian.Uint32(loca[0:4]), binary.BigEndian.Uint32(loca[4:8]); got0 != 0 || got1 != 20 {
			t.Fatalf("face %d long loca offsets = [%d %d], want [0 20]", i, got0, got1)
		}
	}
}

func TestWOFF2CollectionLongLocaReconstructsSharedTransformedHmtx(t *testing.T) {
	woff2Data := woff2CollectionLongLocaWOFF2(t, true)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	faceOffsets := woff2CollectionLongLocaFaceOffsets(t, outData)

	woff2CollectionLongLocaAssertSameBounds(t, outData, faceOffsets, tagGlyf)
	woff2CollectionLongLocaAssertSameBounds(t, outData, faceOffsets, tagLoca)
	woff2CollectionLongLocaAssertSameBounds(t, outData, faceOffsets, tagHmtx)

	for i, faceOffset := range faceOffsets {
		hmtx := sfntTableAt(t, outData, faceOffset, tagHmtx)
		if want := []byte{0x01, 0xf4, 0, 0}; !bytes.Equal(hmtx, want) {
			t.Fatalf("face %d hmtx = % x, want % x", i, hmtx, want)
		}

		loca := sfntTableAt(t, outData, faceOffset, tagLoca)
		if got0, got1 := binary.BigEndian.Uint32(loca[0:4]), binary.BigEndian.Uint32(loca[4:8]); got0 != 0 || got1 != 20 {
			t.Fatalf("face %d long loca offsets = [%d %d], want [0 20]", i, got0, got1)
		}
	}
}

func woff2CollectionLongLocaWOFF2(t *testing.T, includeHmtx bool) []byte {
	t.Helper()

	head0 := woff2CollectionLongLocaHead(1000)
	head1 := woff2CollectionLongLocaHead(1200)
	maxp := testMaxpTable(1)
	transformedGlyf := woff2CollectionLongLocaTransformedSimpleGlyf()

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head for face 0
	directory.Write(testBase128(uint32(len(head0))))
	directory.WriteByte(byte(1)) // head for face 1
	directory.Write(testBase128(uint32(len(head1))))

	hheaIndex := uint16(0)
	hmtxIndex := uint16(0)
	maxpIndex := uint16(2)
	glyfIndex := uint16(3)
	locaIndex := uint16(4)
	numTables := uint16(5)
	var payload bytes.Buffer
	payload.Write(head0)
	payload.Write(head1)

	if includeHmtx {
		hhea := testHheaTable(1)
		hmtx := []byte{1, 0x01, 0xf4} // omit proportional lsb[], advanceWidth[0]=500
		hheaIndex = 2
		hmtxIndex = 3
		maxpIndex = 4
		glyfIndex = 5
		locaIndex = 6
		numTables = 7

		directory.WriteByte(byte(2)) // shared hhea
		directory.Write(testBase128(uint32(len(hhea))))
		directory.WriteByte(byte(0x40 | 3)) // shared transformed hmtx
		directory.Write(testBase128(4))
		directory.Write(testBase128(uint32(len(hmtx))))

		payload.Write(hhea)
		payload.Write(hmtx)
	}

	directory.WriteByte(byte(4)) // shared maxp
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(10)) // shared transformed glyf
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // shared transformed loca
	directory.Write(testBase128(8))
	directory.Write(testBase128(0))

	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(2)) // numFonts
	if includeHmtx {
		directory.Write(testUInt255(6))
		appendUint32(&directory, 0x00010000)
		directory.Write(testUInt255(glyfIndex))
		directory.Write(testUInt255(0))
		directory.Write(testUInt255(locaIndex))
		directory.Write(testUInt255(hmtxIndex))
		directory.Write(testUInt255(maxpIndex))
		directory.Write(testUInt255(hheaIndex))

		directory.Write(testUInt255(6))
		appendUint32(&directory, 0x00010000)
		directory.Write(testUInt255(1))
		directory.Write(testUInt255(hheaIndex))
		directory.Write(testUInt255(maxpIndex))
		directory.Write(testUInt255(hmtxIndex))
		directory.Write(testUInt255(locaIndex))
		directory.Write(testUInt255(glyfIndex))
	} else {
		directory.Write(testUInt255(4))
		appendUint32(&directory, 0x00010000)
		directory.Write(testUInt255(glyfIndex))
		directory.Write(testUInt255(0))
		directory.Write(testUInt255(locaIndex))
		directory.Write(testUInt255(maxpIndex))

		directory.Write(testUInt255(4))
		appendUint32(&directory, 0x00010000)
		directory.Write(testUInt255(1))
		directory.Write(testUInt255(locaIndex))
		directory.Write(testUInt255(maxpIndex))
		directory.Write(testUInt255(glyfIndex))
	}

	payload.Write(maxp)
	payload.Write(transformedGlyf)

	return testWOFF2CollectionWithDirectoryAndPayload(t, numTables, directory.Bytes(), payload.Bytes())
}

func woff2CollectionLongLocaHead(unitsPerEm uint16) []byte {
	head := testHeadTable(1)
	binary.BigEndian.PutUint16(head[18:20], unitsPerEm)
	return head
}

func woff2CollectionLongLocaTransformedSimpleGlyf() []byte {
	transformed := append([]byte(nil), testWOFF2TransformedSimpleGlyf()...)
	binary.BigEndian.PutUint16(transformed[6:8], 1)
	return transformed
}

func woff2CollectionLongLocaFaceOffsets(t *testing.T, font []byte) []int {
	t.Helper()
	if len(font) < 20 {
		t.Fatalf("decoded collection length = %d, want at least 20", len(font))
	}
	if got := binary.BigEndian.Uint32(font[0:4]); got != tagTTCF {
		t.Fatalf("output signature = 0x%08x, want ttcf", got)
	}
	if got := binary.BigEndian.Uint32(font[8:12]); got != 2 {
		t.Fatalf("numFonts = %d, want 2", got)
	}
	return []int{
		int(binary.BigEndian.Uint32(font[12:16])),
		int(binary.BigEndian.Uint32(font[16:20])),
	}
}

func woff2CollectionLongLocaAssertSameBounds(t *testing.T, font []byte, faceOffsets []int, tag uint32) {
	t.Helper()
	wantOffset, wantLength := woff2CollectionLongLocaTableBounds(t, font, faceOffsets[0], tag)
	gotOffset, gotLength := woff2CollectionLongLocaTableBounds(t, font, faceOffsets[1], tag)
	if gotOffset != wantOffset || gotLength != wantLength {
		t.Fatalf("%s bounds differ: first=(%d,%d) second=(%d,%d)", tagString(tag), wantOffset, wantLength, gotOffset, gotLength)
	}
}

func woff2CollectionLongLocaTableBounds(t *testing.T, font []byte, base int, tag uint32) (uint32, uint32) {
	t.Helper()
	if base < 0 || base+12 > len(font) {
		t.Fatalf("invalid SFNT base offset %d", base)
	}
	numTables := int(binary.BigEndian.Uint16(font[base+4 : base+6]))
	for i := 0; i < numTables; i++ {
		entry := base + 12 + i*16
		if entry+16 > len(font) {
			t.Fatalf("invalid SFNT table directory bounds for %s", tagString(tag))
		}
		if binary.BigEndian.Uint32(font[entry:entry+4]) != tag {
			continue
		}
		offset := binary.BigEndian.Uint32(font[entry+8 : entry+12])
		length := binary.BigEndian.Uint32(font[entry+12 : entry+16])
		if uint64(offset)+uint64(length) > uint64(len(font)) {
			t.Fatalf("invalid SFNT table bounds for %s", tagString(tag))
		}
		return offset, length
	}
	t.Fatalf("SFNT table %s not found", tagString(tag))
	return 0, 0
}
