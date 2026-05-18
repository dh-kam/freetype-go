package helper

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2CollectionHmtxMultiGlyphDerivesSharedBearingsFromShortAndLongOutlines(t *testing.T) {
	woff2Data := woff2CollectionHmtxMultiGlyphWOFF2(t)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	faceOffsets := woff2CollectionSharedFaceOffsets(t, outData, 2)

	woff2CollectionSharedAssertSameBounds(t, outData, faceOffsets, tagHmtx)

	wantGlyf := woff2CollectionHmtxMultiGlyphExpectedGlyf()
	wantHmtx := []byte{
		0x01, 0xf4, 0, 10, // advanceWidth[0]=500, lsb[0]=glyph 0 xMin
		0x02, 0x58, 0xff, 0xf9, // advanceWidth[1]=600, lsb[1]=glyph 1 xMin
		0, 3, // leftSideBearing[2]=glyph 2 xMin
	}
	wantLoca := [][]byte{
		woff2CollectionHmtxMultiGlyphExpectedShortLoca(),
		woff2CollectionHmtxMultiGlyphExpectedLongLoca(),
	}
	wantIndexFormat := []int16{0, 1}

	for i, faceOffset := range faceOffsets {
		head := sfntTableAt(t, outData, faceOffset, tagHead)
		if got := int16(binary.BigEndian.Uint16(head[50:52])); got != wantIndexFormat[i] {
			t.Fatalf("face %d indexToLocFormat = %d, want %d", i, got, wantIndexFormat[i])
		}

		if got := sfntTableAt(t, outData, faceOffset, tagGlyf); !bytes.Equal(got, wantGlyf) {
			t.Fatalf("face %d glyf = % x, want % x", i, got, wantGlyf)
		}
		if got := sfntTableAt(t, outData, faceOffset, tagLoca); !bytes.Equal(got, wantLoca[i]) {
			t.Fatalf("face %d loca = % x, want % x", i, got, wantLoca[i])
		}
		if got := sfntTableAt(t, outData, faceOffset, tagHmtx); !bytes.Equal(got, wantHmtx) {
			t.Fatalf("face %d hmtx = % x, want % x", i, got, wantHmtx)
		}
	}
}

func woff2CollectionHmtxMultiGlyphWOFF2(t *testing.T) []byte {
	t.Helper()

	hmtx := []byte{3, 0x01, 0xf4, 0x02, 0x58} // omit all LSB values, keep two advance widths
	wantGlyf := woff2CollectionHmtxMultiGlyphExpectedGlyf()
	entries := []woff2CollectionSharedEntry{
		woff2CollectionSharedRawEntry(1, testHeadTable(0)),
		woff2CollectionSharedRawEntry(1, testHeadTable(1)),
		woff2CollectionSharedRawEntry(2, testHheaTable(2)),
		woff2CollectionSharedTransformedEntry(0x40|3, 10, hmtx),
		woff2CollectionSharedRawEntry(4, testMaxpTable(3)),
		woff2CollectionSharedTransformedEntry(10, uint32(len(wantGlyf)), woff2CollectionHmtxMultiGlyphTransformedGlyf(0)),
		woff2CollectionSharedTransformedEntry(11, uint32(len(woff2CollectionHmtxMultiGlyphExpectedShortLoca())), nil),
		woff2CollectionSharedTransformedEntry(10, uint32(len(wantGlyf)), woff2CollectionHmtxMultiGlyphTransformedGlyf(1)),
		woff2CollectionSharedTransformedEntry(11, uint32(len(woff2CollectionHmtxMultiGlyphExpectedLongLoca())), nil),
	}

	return woff2CollectionSharedWOFF2(t, entries, [][]uint16{
		{5, 0, 6, 3, 4, 2}, // short glyf/loca face
		{1, 2, 4, 3, 7, 8}, // long glyf/loca face
	})
}

func woff2CollectionHmtxMultiGlyphTransformedGlyf(indexFormat int16) []byte {
	transformed := testWOFF2TransformedGlyfFromStreams(
		3,
		[]byte{0, 1, 0, 1, 0, 1},
		[]byte{1, 1, 1},
		[]byte{11, 10, 11},
		[]byte{10, 0, 7, 0, 3, 0},
		nil,
		[]byte{0, 0, 0, 0},
		nil,
	)
	binary.BigEndian.PutUint16(transformed[6:8], uint16(indexFormat))
	return transformed
}

func woff2CollectionHmtxMultiGlyphExpectedGlyf() []byte {
	glyf := append([]byte(nil), testWOFF2OnePointGlyphAtX10()...)
	glyf = append(glyf, woff2CollectionHmtxMultiGlyphOnePointGlyph(-7)...)
	glyf = append(glyf, woff2CollectionHmtxMultiGlyphOnePointGlyph(3)...)
	return glyf
}

func woff2CollectionHmtxMultiGlyphOnePointGlyph(x int16) []byte {
	var glyph bytes.Buffer
	appendUint16(&glyph, 1)
	appendInt16(&glyph, x)
	appendUint16(&glyph, 0)
	appendInt16(&glyph, x)
	appendUint16(&glyph, 0)
	appendUint16(&glyph, 0)
	appendUint16(&glyph, 0)

	flag := byte(0x33)
	if x < 0 {
		flag = 0x23
		x = -x
	}
	glyph.WriteByte(flag)
	glyph.WriteByte(byte(x))
	return glyph.Bytes()
}

func woff2CollectionHmtxMultiGlyphExpectedShortLoca() []byte {
	return []byte{
		0, 0,
		0, 8,
		0, 16,
		0, 24,
	}
}

func woff2CollectionHmtxMultiGlyphExpectedLongLoca() []byte {
	return []byte{
		0, 0, 0, 0,
		0, 0, 0, 16,
		0, 0, 0, 32,
		0, 0, 0, 48,
	}
}
