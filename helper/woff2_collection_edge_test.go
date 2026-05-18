package helper

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2CollectionEdgeRejectsExtraTransformedSimpleGlyphSubstreamBytes(t *testing.T) {
	transformed := testWOFF2TransformedGlyfFromStreams(
		1,
		[]byte{0, 1},
		[]byte{3},
		[]byte{0, 11, 1},
		[]byte{0, 10, 20, 0, 0xff},
		nil,
		[]byte{0, 0, 0, 0},
		nil,
	)
	woff2Data := testWOFF2WithTransformedGlyfLoca(t, transformed)

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected extra transformed glyph substream bytes to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 glyf substream length mismatch") {
		t.Fatalf("error = %q, want glyf substream length mismatch", err)
	}
}

func TestWOFF2CollectionEdgeSharesReconstructedGlyfLocaTables(t *testing.T) {
	woff2Data := woff2CollectionEdgeSharedOutlineWOFF2(t, testMaxpTable(1))

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	if binary.BigEndian.Uint32(outData[0:4]) != tagTTCF {
		t.Fatalf("output signature = 0x%08x, want ttcf", binary.BigEndian.Uint32(outData[0:4]))
	}
	if binary.BigEndian.Uint32(outData[8:12]) != 2 {
		t.Fatalf("numFonts = %d, want 2", binary.BigEndian.Uint32(outData[8:12]))
	}

	firstFaceOffset := int(binary.BigEndian.Uint32(outData[12:16]))
	secondFaceOffset := int(binary.BigEndian.Uint32(outData[16:20]))
	firstGlyfOffset, firstGlyfLength := woff2CollectionEdgeTableBounds(t, outData, firstFaceOffset, tagGlyf)
	secondGlyfOffset, secondGlyfLength := woff2CollectionEdgeTableBounds(t, outData, secondFaceOffset, tagGlyf)
	if firstGlyfOffset != secondGlyfOffset || firstGlyfLength != secondGlyfLength {
		t.Fatalf("shared glyf bounds differ: first=(%d,%d) second=(%d,%d)", firstGlyfOffset, firstGlyfLength, secondGlyfOffset, secondGlyfLength)
	}
	firstLocaOffset, firstLocaLength := woff2CollectionEdgeTableBounds(t, outData, firstFaceOffset, tagLoca)
	secondLocaOffset, secondLocaLength := woff2CollectionEdgeTableBounds(t, outData, secondFaceOffset, tagLoca)
	if firstLocaOffset != secondLocaOffset || firstLocaLength != secondLocaLength {
		t.Fatalf("shared loca bounds differ: first=(%d,%d) second=(%d,%d)", firstLocaOffset, firstLocaLength, secondLocaOffset, secondLocaLength)
	}

	assertSimpleWOFF2Glyph(t, sfntTableAt(t, outData, firstFaceOffset, tagGlyf))
	assertSimpleWOFF2Glyph(t, sfntTableAt(t, outData, secondFaceOffset, tagGlyf))
	loca := sfntTableAt(t, outData, secondFaceOffset, tagLoca)
	if binary.BigEndian.Uint16(loca[0:2]) != 0 || binary.BigEndian.Uint16(loca[2:4]) != 10 {
		t.Fatalf("unexpected shared short loca offsets: % x", loca)
	}
}

func TestWOFF2CollectionEdgeRejectsSharedGlyfLocaMetadataMismatch(t *testing.T) {
	woff2Data := woff2CollectionEdgeSharedOutlineWOFF2(t, testMaxpTable(2))

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected shared glyf/loca metadata mismatch to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 glyf/loca metadata mismatch") {
		t.Fatalf("error = %q, want glyf/loca metadata mismatch", err)
	}
}

func woff2CollectionEdgeSharedOutlineWOFF2(t *testing.T, secondMaxp []byte) []byte {
	t.Helper()

	head0 := testHeadTable(0)
	head1 := testHeadTable(0)
	maxp0 := testMaxpTable(1)
	transformedGlyf := testWOFF2TransformedSimpleGlyf()

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head for font 0
	directory.Write(testBase128(uint32(len(head0))))
	directory.WriteByte(byte(1)) // head for font 1
	directory.Write(testBase128(uint32(len(head1))))
	directory.WriteByte(byte(4)) // maxp for font 0
	directory.Write(testBase128(uint32(len(maxp0))))
	directory.WriteByte(byte(4)) // maxp for font 1
	directory.Write(testBase128(uint32(len(secondMaxp))))
	directory.WriteByte(byte(10)) // shared transformed glyf
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // shared transformed loca
	directory.Write(testBase128(4))
	directory.Write(testBase128(0))

	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(2)) // numFonts
	directory.Write(testUInt255(4)) // font 0 tables
	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(0)) // head0
	directory.Write(testUInt255(2)) // maxp0
	directory.Write(testUInt255(4)) // glyf
	directory.Write(testUInt255(5)) // loca
	directory.Write(testUInt255(4)) // font 1 tables
	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(1)) // head1
	directory.Write(testUInt255(3)) // secondMaxp
	directory.Write(testUInt255(4)) // glyf
	directory.Write(testUInt255(5)) // loca

	var payload bytes.Buffer
	payload.Write(head0)
	payload.Write(head1)
	payload.Write(maxp0)
	payload.Write(secondMaxp)
	payload.Write(transformedGlyf)

	return testWOFF2CollectionWithDirectoryAndPayload(t, 6, directory.Bytes(), payload.Bytes())
}

func woff2CollectionEdgeTableBounds(t *testing.T, font []byte, base int, tag uint32) (uint32, uint32) {
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
