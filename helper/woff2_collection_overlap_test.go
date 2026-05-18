package helper

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2CollectionOverlapBitmapSurvivesSharedTransformedGlyfLoca(t *testing.T) {
	transformedGlyf := woff2CollectionOverlapTransformedGlyf()
	wantGlyf := woff2CollectionOverlapExpectedGlyf()
	wantLoca := woff2CollectionOverlapExpectedShortLoca()
	entries := []woff2CollectionSharedEntry{
		woff2CollectionSharedRawEntry(1, woff2CollectionSharedHead(1000)),
		woff2CollectionSharedRawEntry(1, woff2CollectionSharedHead(1200)),
		woff2CollectionSharedRawEntry(4, testMaxpTable(3)),
		woff2CollectionSharedTransformedEntry(10, uint32(len(wantGlyf)), transformedGlyf),
		woff2CollectionSharedTransformedEntry(11, uint32(len(wantLoca)), nil),
	}
	woff2Data := woff2CollectionSharedWOFF2(t, entries, [][]uint16{
		{0, 2, 3, 4},
		{1, 4, 2, 3},
	})

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	faceOffsets := woff2CollectionSharedFaceOffsets(t, outData, 2)

	woff2CollectionSharedAssertSameBounds(t, outData, faceOffsets, tagGlyf)
	woff2CollectionSharedAssertSameBounds(t, outData, faceOffsets, tagLoca)

	for i, faceOffset := range faceOffsets {
		glyf := sfntTableAt(t, outData, faceOffset, tagGlyf)
		if !bytes.Equal(glyf, wantGlyf) {
			t.Fatalf("face %d glyf = % x, want % x", i, glyf, wantGlyf)
		}
		woff2CollectionOverlapAssertPointFlags(t, glyf, i)

		loca := sfntTableAt(t, outData, faceOffset, tagLoca)
		if !bytes.Equal(loca, wantLoca) {
			t.Fatalf("face %d loca = % x, want % x", i, loca, wantLoca)
		}
	}
}

func woff2CollectionOverlapTransformedGlyf() []byte {
	transformed := testWOFF2TransformedGlyfFromStreams(
		3,
		[]byte{0, 1, 0, 1, 0, 0},
		[]byte{1, 1},
		[]byte{11, 11},
		[]byte{10, 0, 20, 0},
		nil,
		[]byte{0, 0, 0, 0},
		nil,
	)
	binary.BigEndian.PutUint16(transformed[2:4], 1)
	return append(transformed, 0x40, 0, 0, 0)
}

func woff2CollectionOverlapExpectedGlyf() []byte {
	return []byte{
		0, 1, // glyph 0: numberOfContours
		0, 10, 0, 0, 0, 10, 0, 0, // computed bbox
		0, 0, // endPtsOfContours[0]
		0, 0, // instructionLength
		0x33, 10, // point flag without OVERLAP_SIMPLE, x delta

		0, 1, // glyph 1: numberOfContours
		0, 20, 0, 0, 0, 20, 0, 0, // computed bbox
		0, 0, // endPtsOfContours[0]
		0, 0, // instructionLength
		0x73, 20, // point flag with OVERLAP_SIMPLE, x delta
	}
}

func woff2CollectionOverlapExpectedShortLoca() []byte {
	return []byte{
		0, 0,
		0, 8,
		0, 16,
		0, 16,
	}
}

func woff2CollectionOverlapAssertPointFlags(t *testing.T, glyf []byte, face int) {
	t.Helper()

	const overlapSimple = 1 << 6
	if len(glyf) <= 30 {
		t.Fatalf("face %d glyf length = %d, want overlap flag position available", face, len(glyf))
	}
	if glyf[14]&overlapSimple != 0 {
		t.Fatalf("face %d glyph 0 point flag = 0x%02x, want OVERLAP_SIMPLE clear", face, glyf[14])
	}
	if glyf[30]&overlapSimple == 0 {
		t.Fatalf("face %d glyph 1 point flag = 0x%02x, want OVERLAP_SIMPLE set", face, glyf[30])
	}
}
