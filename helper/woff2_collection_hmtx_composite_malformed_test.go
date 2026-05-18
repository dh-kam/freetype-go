package helper

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2CollectionHmtxCompositeMalformedReferencesRejectWithoutPanic(t *testing.T) {
	tests := []struct {
		name     string
		faceRefs [][]uint16
	}{
		{
			name: "shared hmtx dependency validation",
			faceRefs: [][]uint16{
				{0, 2, 3, 4, 5, 6},
				{1, 2, 3, 4, 7, 8},
			},
		},
		{
			name: "final outline validation for non-hmtx face",
			faceRefs: [][]uint16{
				{0, 2, 3, 4, 5, 6},
				{1, 4, 7, 8},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			woff2Data := woff2CollectionHmtxCompositeMalformedWOFF2(t, tt.faceRefs)

			err := woff2CollectionHmtxCompositeMalformedDecodeErr(t, woff2Data)
			if err == nil {
				t.Fatal("expected malformed composite glyph reference to fail")
			}
			if !strings.Contains(err.Error(), "invalid WOFF2 composite glyph index") {
				t.Fatalf("error = %q, want composite glyph index error", err)
			}
		})
	}
}

func woff2CollectionHmtxCompositeMalformedDecodeErr(t *testing.T, woff2Data []byte) (err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("DecodeWOFF2 panicked: %v", recovered)
		}
	}()

	_, err = DecodeWOFF2(core.NewMemoryStream(woff2Data))
	return err
}

func woff2CollectionHmtxCompositeMalformedWOFF2(t *testing.T, faceRefs [][]uint16) []byte {
	t.Helper()

	hmtx := []byte{1, 0x01, 0xf4, 0x02, 0x58}
	validGlyf := woff2CollectionCompositeExpectedGlyf()
	badGlyf := woff2CollectionHmtxCompositeMalformedGlyf(t)
	loca := woff2CollectionHmtxCompositeShortLoca()
	entries := []woff2CollectionSharedEntry{
		woff2CollectionSharedRawEntry(1, testHeadTable(0)),
		woff2CollectionSharedRawEntry(1, testHeadTable(0)),
		woff2CollectionSharedRawEntry(2, testHheaTable(2)),
		woff2CollectionSharedTransformedEntry(0x40|3, 8, hmtx),
		woff2CollectionSharedRawEntry(4, testMaxpTable(2)),
		woff2CollectionSharedRawEntry(0xc0|10, validGlyf),
		woff2CollectionSharedRawEntry(0xc0|11, loca),
		woff2CollectionSharedRawEntry(0xc0|10, badGlyf),
		woff2CollectionSharedRawEntry(0xc0|11, loca),
	}

	return woff2CollectionSharedWOFF2(t, entries, faceRefs)
}

func woff2CollectionHmtxCompositeMalformedGlyf(t *testing.T) []byte {
	t.Helper()

	glyf := append([]byte(nil), woff2CollectionCompositeExpectedGlyf()...)
	loca := woff2CollectionHmtxCompositeShortLoca()
	compositeStart := int(binary.BigEndian.Uint16(loca[2:4])) * 2
	componentIndexOffset := compositeStart + 12
	if componentIndexOffset+2 > len(glyf) {
		t.Fatalf("composite component index offset %d exceeds glyf length %d", componentIndexOffset, len(glyf))
	}
	binary.BigEndian.PutUint16(glyf[componentIndexOffset:componentIndexOffset+2], 2)
	return glyf
}

func woff2CollectionHmtxCompositeShortLoca() []byte {
	return []byte{
		0, 0,
		0, 10,
		0, 26,
	}
}
