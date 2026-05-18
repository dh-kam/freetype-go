package helper

import (
	"bytes"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2CollectionHmtxMixedMultiGlyphPreservesExplicitAndDerivesOmittedLSBs(t *testing.T) {
	tests := []struct {
		name            string
		transformedHmtx []byte
		wantHmtx        []byte
	}{
		{
			name: "explicit-proportional-lsbs-derived-left-side-bearing",
			transformedHmtx: []byte{
				2,
				0x01, 0xf4, // advanceWidth[0]=500
				0x02, 0x58, // advanceWidth[1]=600
				0, 4, // explicit lsb[0], not glyph 0 xMin
				0xff, 0xec, // explicit lsb[1], not glyph 1 xMin
			},
			wantHmtx: []byte{
				0x01, 0xf4, 0, 4,
				0x02, 0x58, 0xff, 0xec,
				0, 3, // derived leftSideBearing[2] from glyph 2 xMin
			},
		},
		{
			name: "derived-proportional-lsbs-explicit-left-side-bearing",
			transformedHmtx: []byte{
				1,
				0x01, 0xf4, // advanceWidth[0]=500
				0x02, 0x58, // advanceWidth[1]=600
				0xff, 0xec, // explicit leftSideBearing[2], not glyph 2 xMin
			},
			wantHmtx: []byte{
				0x01, 0xf4, 0, 10, // derived lsb[0] from glyph 0 xMin
				0x02, 0x58, 0xff, 0xf9, // derived lsb[1] from glyph 1 xMin
				0xff, 0xec,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			woff2Data := woff2CollectionHmtxMixedMultiGlyphWOFF2(t, tt.transformedHmtx)

			outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
			if err != nil {
				t.Fatalf("DecodeWOFF2 failed: %v", err)
			}
			outData := readTestStream(t, outStream)
			faceOffsets := woff2CollectionSharedFaceOffsets(t, outData, 2)

			woff2CollectionSharedAssertSameBounds(t, outData, faceOffsets, tagHmtx)

			wantGlyf := woff2CollectionHmtxMultiGlyphExpectedGlyf()
			wantLoca := [][]byte{
				woff2CollectionHmtxMultiGlyphExpectedShortLoca(),
				woff2CollectionHmtxMultiGlyphExpectedLongLoca(),
			}

			for i, faceOffset := range faceOffsets {
				if got := sfntTableAt(t, outData, faceOffset, tagGlyf); !bytes.Equal(got, wantGlyf) {
					t.Fatalf("face %d glyf = % x, want % x", i, got, wantGlyf)
				}
				if got := sfntTableAt(t, outData, faceOffset, tagLoca); !bytes.Equal(got, wantLoca[i]) {
					t.Fatalf("face %d loca = % x, want % x", i, got, wantLoca[i])
				}
				if got := sfntTableAt(t, outData, faceOffset, tagHmtx); !bytes.Equal(got, tt.wantHmtx) {
					t.Fatalf("face %d hmtx = % x, want % x", i, got, tt.wantHmtx)
				}
			}
		})
	}
}

func woff2CollectionHmtxMixedMultiGlyphWOFF2(t *testing.T, hmtx []byte) []byte {
	t.Helper()

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
