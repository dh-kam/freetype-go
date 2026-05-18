package helper

import (
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2CollectionHmtxRejectsInconsistentSharedDependencies(t *testing.T) {
	hmtx := []byte{1, 0x01, 0xf4}
	rawGlyf := testWOFF2OnePointGlyphAtX10()
	rawLoca := []byte{0, 0, 0, 8}
	entries := []woff2CollectionSharedEntry{
		woff2CollectionSharedRawEntry(1, woff2CollectionSharedHead(1000)),
		woff2CollectionSharedRawEntry(1, woff2CollectionSharedHead(1200)),
		woff2CollectionSharedRawEntry(2, testHheaTable(1)),
		woff2CollectionSharedTransformedEntry(0x40|3, 4, hmtx),
		woff2CollectionSharedRawEntry(4, testMaxpTable(1)),
		woff2CollectionSharedTransformedEntry(10, 20, testWOFF2TransformedSimpleGlyf()),
		woff2CollectionSharedTransformedEntry(11, 4, nil),
		woff2CollectionSharedRawEntry(0xc0|10, rawGlyf),
		woff2CollectionSharedRawEntry(0xc0|11, rawLoca),
	}
	woff2Data := woff2CollectionSharedWOFF2(t, entries, [][]uint16{
		{0, 2, 3, 4, 5, 6},
		{1, 2, 3, 4, 7, 8},
	})

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected inconsistent shared hmtx dependencies to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 shared hmtx transform has inconsistent collection dependencies") {
		t.Fatalf("error = %q, want inconsistent shared hmtx dependencies", err)
	}
}

func TestWOFF2CollectionHmtxRejectsMissingOutlineReferences(t *testing.T) {
	hmtx := []byte{1, 0x01, 0xf4}
	rawGlyf := testWOFF2OnePointGlyphAtX10()
	rawLoca := []byte{0, 0, 0, 8}
	entries := []woff2CollectionSharedEntry{
		woff2CollectionSharedRawEntry(1, woff2CollectionSharedHead(1000)),
		woff2CollectionSharedRawEntry(2, testHheaTable(1)),
		woff2CollectionSharedTransformedEntry(0x40|3, 4, hmtx),
		woff2CollectionSharedRawEntry(4, testMaxpTable(1)),
		woff2CollectionSharedRawEntry(0xc0|10, rawGlyf),
		woff2CollectionSharedRawEntry(0xc0|11, rawLoca),
	}
	tests := []struct {
		name string
		refs []uint16
		want string
	}{
		{
			name: "missing glyf",
			refs: []uint16{0, 1, 2, 3, 5},
			want: "WOFF2 hmtx transform requires glyf table",
		},
		{
			name: "missing loca",
			refs: []uint16{0, 1, 2, 3, 4},
			want: "WOFF2 hmtx transform requires loca table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			woff2Data := woff2CollectionSharedWOFF2(t, entries, [][]uint16{tt.refs})

			_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
			if err == nil {
				t.Fatal("expected missing hmtx outline dependency to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestWOFF2CollectionHmtxRejectsUnreferencedTransformedTable(t *testing.T) {
	hmtx := []byte{1, 0x01, 0xf4}
	entries := []woff2CollectionSharedEntry{
		woff2CollectionSharedRawEntry(1, woff2CollectionSharedHead(1000)),
		woff2CollectionSharedRawEntry(2, testHheaTable(1)),
		woff2CollectionSharedTransformedEntry(0x40|3, 4, hmtx),
		woff2CollectionSharedRawEntry(4, testMaxpTable(1)),
		woff2CollectionSharedTransformedEntry(10, 20, testWOFF2TransformedSimpleGlyf()),
		woff2CollectionSharedTransformedEntry(11, 4, nil),
	}
	woff2Data := woff2CollectionSharedWOFF2(t, entries, [][]uint16{
		{0, 1, 3, 4, 5},
	})

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected unreferenced transformed hmtx table to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 collection hmtx table is not referenced") {
		t.Fatalf("error = %q, want unreferenced collection hmtx error", err)
	}
}
