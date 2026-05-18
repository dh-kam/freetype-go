package helper

import (
	"strings"
	"testing"
)

func TestWOFF2CollectionHmtxRejectsMalformedRawLongLocaDependencies(t *testing.T) {
	tests := []struct {
		name           string
		maxp           []byte
		hmtxOrigLength uint32
		hmtx           []byte
		loca           []byte
		want           string
	}{
		{
			name:           "long-loca-length-mismatches-maxp",
			maxp:           testMaxpTable(2),
			hmtxOrigLength: 6,
			hmtx:           []byte{3, 0x01, 0xf4},
			loca: []byte{
				0, 0, 0, 0,
				0, 0, 0, 20,
			},
			want: "WOFF2 glyf/loca metadata mismatch",
		},
		{
			name:           "long-loca-final-offset-mismatches-glyf-length",
			maxp:           testMaxpTable(1),
			hmtxOrigLength: 4,
			hmtx:           []byte{1, 0x01, 0xf4},
			loca: []byte{
				0, 0, 0, 0,
				0, 0, 0, 12,
			},
			want: "WOFF2 loca/glyf length mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			woff2Data := woff2CollectionHmtxRawMalformedWOFF2(
				t,
				testHeadTable(1),
				testHheaTable(1),
				tt.maxp,
				tt.hmtxOrigLength,
				tt.hmtx,
				testWOFF2OnePointGlyphAtX10(),
				tt.loca,
			)

			err := woff2CollectionHmtxRawMalformedDecodeErr(t, woff2Data)
			if err == nil {
				t.Fatal("expected malformed raw long-loca hmtx dependency to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err, tt.want)
			}
		})
	}
}
