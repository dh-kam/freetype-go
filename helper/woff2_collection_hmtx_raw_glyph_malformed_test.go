package helper

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestWOFF2CollectionHmtxRawGlyphMalformedRejectsWithoutPanicWhileDerivingBearings(t *testing.T) {
	tests := []struct {
		name string
		glyf []byte
		want string
	}{
		{
			name: "simple-glyph-truncated-point-data",
			glyf: woff2CollectionHmtxRawGlyphMalformedSimpleGlyph(),
			want: "invalid WOFF2 simple glyph point data",
		},
		{
			name: "composite-glyph-reserved-flags",
			glyf: woff2CollectionHmtxRawGlyphMalformedCompositeReservedFlags(),
			want: "invalid WOFF2 composite flags",
		},
		{
			name: "composite-glyph-nonzero-padding",
			glyf: woff2CollectionHmtxRawGlyphMalformedCompositeNonZeroPadding(),
			want: "invalid WOFF2 glyf padding",
		},
		{
			name: "glyph-length-below-minimum",
			glyf: []byte{0, 1, 0, 0, 0, 0, 0, 0},
			want: "invalid WOFF2 glyf bounds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			woff2Data := woff2CollectionHmtxRawMalformedWOFF2(
				t,
				testHeadTable(0),
				testHheaTable(1),
				testMaxpTable(1),
				4,
				[]byte{1, 0x01, 0xf4},
				tt.glyf,
				woff2CollectionHmtxRawGlyphMalformedShortLoca(t, len(tt.glyf)),
			)

			err := woff2CollectionHmtxRawMalformedDecodeErr(t, woff2Data)
			if err == nil {
				t.Fatal("expected malformed raw glyph dependency to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func woff2CollectionHmtxRawGlyphMalformedSimpleGlyph() []byte {
	return []byte{
		0, 1, // numberOfContours
		0, 0, 0, 0, 0, 0, 0, 0, // bbox
		0, 0, // endPtsOfContours[0]
		0, 0, // instructionLength; point flags are missing
	}
}

func woff2CollectionHmtxRawGlyphMalformedCompositeReservedFlags() []byte {
	return []byte{
		0xff, 0xff, // numberOfContours = -1
		0, 0, 0, 0, 0, 0, 0, 0, // bbox
		0, 0x10, // reserved composite flag bit
		0, 0, // componentGlyphIndex
	}
}

func woff2CollectionHmtxRawGlyphMalformedCompositeNonZeroPadding() []byte {
	return []byte{
		0xff, 0xff, // numberOfContours = -1
		0, 0, 0, 0, 0, 0, 0, 0, // bbox
		0, 0x02, // byte args, xy values
		0, 0, // componentGlyphIndex
		0, 0, // args
		0xff, 0, // non-zero padding
	}
}

func woff2CollectionHmtxRawGlyphMalformedShortLoca(t *testing.T, glyphLength int) []byte {
	t.Helper()
	if glyphLength < 0 || glyphLength%2 != 0 || glyphLength/2 > 0xffff {
		t.Fatalf("glyph length %d cannot be represented in short loca", glyphLength)
	}
	loca := make([]byte, 4)
	binary.BigEndian.PutUint16(loca[2:4], uint16(glyphLength/2))
	return loca
}
