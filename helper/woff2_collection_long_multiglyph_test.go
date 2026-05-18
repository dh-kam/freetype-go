package helper

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2CollectionLongMultiGlyphSharesTransformedGlyfLocaAcrossFaces(t *testing.T) {
	transformedGlyf := woff2LongMultiGlyphTransformedGlyf()
	wantGlyf := woff2MultiGlyphExpectedGlyf()
	wantLoca := woff2LongMultiGlyphExpectedLoca()

	woff2Data := woff2CollectionLongMultiGlyphWOFF2(t, transformedGlyf, wantGlyf, wantLoca)
	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	faceOffsets := woff2CollectionSharedFaceOffsets(t, outData, 2)

	woff2CollectionSharedAssertSameBounds(t, outData, faceOffsets, tagGlyf)
	woff2CollectionSharedAssertSameBounds(t, outData, faceOffsets, tagLoca)

	for i, faceOffset := range faceOffsets {
		head := sfntTableAt(t, outData, faceOffset, tagHead)
		if got := int16(binary.BigEndian.Uint16(head[50:52])); got != 1 {
			t.Fatalf("face %d indexToLocFormat = %d, want 1", i, got)
		}

		if got := sfntTableAt(t, outData, faceOffset, tagGlyf); !bytes.Equal(got, wantGlyf) {
			t.Fatalf("face %d decoded glyf = % x, want % x", i, got, wantGlyf)
		}
		if got := sfntTableAt(t, outData, faceOffset, tagLoca); !bytes.Equal(got, wantLoca) {
			t.Fatalf("face %d decoded long loca = % x, want % x", i, got, wantLoca)
		}
	}
}

func woff2CollectionLongMultiGlyphWOFF2(t *testing.T, transformedGlyf, wantGlyf, wantLoca []byte) []byte {
	t.Helper()

	entries := []woff2CollectionSharedEntry{
		woff2CollectionSharedRawEntry(1, woff2CollectionLongLocaHead(1000)),
		woff2CollectionSharedRawEntry(1, woff2CollectionLongLocaHead(1200)),
		woff2CollectionSharedRawEntry(4, testMaxpTable(3)),
		woff2CollectionSharedTransformedEntry(10, uint32(len(wantGlyf)), transformedGlyf),
		woff2CollectionSharedTransformedEntry(11, uint32(len(wantLoca)), nil),
	}
	return woff2CollectionSharedWOFF2(t, entries, [][]uint16{
		{3, 0, 4, 2}, // glyf, head, loca, maxp
		{1, 4, 2, 3}, // head, loca, maxp, glyf
	})
}
