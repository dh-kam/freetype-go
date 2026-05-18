package helper

import (
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2CollectionHmtxRejectsMalformedRawOutlineDependencies(t *testing.T) {
	tests := []struct {
		name           string
		head           []byte
		hhea           []byte
		maxp           []byte
		hmtxOrigLength uint32
		hmtx           []byte
		glyf           []byte
		loca           []byte
		want           string
	}{
		{
			name:           "short-loca-length-mismatches-maxp",
			head:           testHeadTable(0),
			hhea:           testHheaTable(1),
			maxp:           testMaxpTable(2),
			hmtxOrigLength: 6,
			hmtx:           []byte{3, 0x01, 0xf4},
			glyf:           testWOFF2OnePointGlyphAtX10(),
			loca:           []byte{0, 0, 0, 8},
			want:           "WOFF2 glyf/loca metadata mismatch",
		},
		{
			name:           "short-loca-offset-beyond-glyf",
			head:           testHeadTable(0),
			hhea:           testHheaTable(1),
			maxp:           testMaxpTable(1),
			hmtxOrigLength: 4,
			hmtx:           []byte{1, 0x01, 0xf4},
			glyf:           testWOFF2OnePointGlyphAtX10(),
			loca:           []byte{0, 0, 0, 100},
			want:           "invalid WOFF2 glyf bounds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			woff2Data := woff2CollectionHmtxRawMalformedWOFF2(
				t,
				tt.head,
				tt.hhea,
				tt.maxp,
				tt.hmtxOrigLength,
				tt.hmtx,
				tt.glyf,
				tt.loca,
			)

			err := woff2CollectionHmtxRawMalformedDecodeErr(t, woff2Data)
			if err == nil {
				t.Fatal("expected malformed raw hmtx outline dependency to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func woff2CollectionHmtxRawMalformedWOFF2(t *testing.T, head, hhea, maxp []byte, hmtxOrigLength uint32, hmtx, glyf, loca []byte) []byte {
	t.Helper()

	entries := []woff2CollectionSharedEntry{
		woff2CollectionSharedRawEntry(1, head),
		woff2CollectionSharedRawEntry(1, head),
		woff2CollectionSharedRawEntry(2, hhea),
		woff2CollectionSharedTransformedEntry(0x40|3, hmtxOrigLength, hmtx),
		woff2CollectionSharedRawEntry(4, maxp),
		woff2CollectionSharedRawEntry(0xc0|10, glyf),
		woff2CollectionSharedRawEntry(0xc0|11, loca),
	}
	return woff2CollectionSharedWOFF2(t, entries, [][]uint16{
		{0, 2, 3, 4, 5, 6},
		{1, 2, 3, 4, 5, 6},
	})
}

func woff2CollectionHmtxRawMalformedDecodeErr(t *testing.T, woff2Data []byte) (err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DecodeWOFF2 panicked: %v", r)
		}
	}()
	_, err = DecodeWOFF2(core.NewMemoryStream(woff2Data))
	return err
}
