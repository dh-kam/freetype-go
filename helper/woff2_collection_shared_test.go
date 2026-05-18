package helper

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

type woff2CollectionSharedEntry struct {
	flag               byte
	origLength         uint32
	transformLength    uint32
	hasTransformLength bool
	payload            []byte
}

func TestWOFF2CollectionSharedReconstructsOutlinesAcrossUnorderedFaceRefs(t *testing.T) {
	transformedGlyf := woff2CollectionSharedSimpleAndEmptyGlyf()
	maxp := testMaxpTable(2)
	heads := [][]byte{
		woff2CollectionSharedHead(1000),
		woff2CollectionSharedHead(1200),
		woff2CollectionSharedHead(1400),
	}
	entries := []woff2CollectionSharedEntry{
		woff2CollectionSharedRawEntry(1, heads[0]),
		woff2CollectionSharedRawEntry(1, heads[1]),
		woff2CollectionSharedRawEntry(1, heads[2]),
		woff2CollectionSharedRawEntry(4, maxp),
		woff2CollectionSharedTransformedEntry(10, 20, transformedGlyf),
		woff2CollectionSharedTransformedEntry(11, 6, nil),
	}
	woff2Data := woff2CollectionSharedWOFF2(t, entries, [][]uint16{
		{4, 0, 5, 3}, // glyf, head, loca, maxp
		{1, 5, 3, 4}, // head, loca, maxp, glyf
		{5, 4, 3, 2}, // loca, glyf, maxp, head
	})

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	faceOffsets := woff2CollectionSharedFaceOffsets(t, outData, 3)

	woff2CollectionSharedAssertSameBounds(t, outData, faceOffsets, tagGlyf)
	woff2CollectionSharedAssertSameBounds(t, outData, faceOffsets, tagLoca)
	woff2CollectionSharedAssertSameBounds(t, outData, faceOffsets, tagMaxp)

	for i, faceOffset := range faceOffsets {
		head := sfntTableAt(t, outData, faceOffset, tagHead)
		if got := binary.BigEndian.Uint16(head[18:20]); got != uint16(1000+i*200) {
			t.Fatalf("face %d unitsPerEm = %d, want %d", i, got, 1000+i*200)
		}

		glyf := sfntTableAt(t, outData, faceOffset, tagGlyf)
		if len(glyf) != 20 {
			t.Fatalf("face %d glyf length = %d, want 20", i, len(glyf))
		}
		assertSimpleWOFF2Glyph(t, glyf)

		loca := sfntTableAt(t, outData, faceOffset, tagLoca)
		wantLoca := []uint16{0, 10, 10}
		for j, want := range wantLoca {
			if got := binary.BigEndian.Uint16(loca[j*2 : j*2+2]); got != want {
				t.Fatalf("face %d loca[%d] = %d, want %d", i, j, got, want)
			}
		}
	}
}

func TestWOFF2CollectionSharedRejectsInvalidOutlineReferenceCombinations(t *testing.T) {
	tests := []struct {
		name           string
		secondFaceRefs []uint16
	}{
		{
			name:           "shared glyf without shared loca",
			secondFaceRefs: []uint16{1, 2, 3},
		},
		{
			name:           "shared loca without shared glyf",
			secondFaceRefs: []uint16{1, 2, 4},
		},
		{
			name:           "shared glyf with raw loca",
			secondFaceRefs: []uint16{1, 2, 3, 6},
		},
		{
			name:           "raw glyf with shared loca",
			secondFaceRefs: []uint16{1, 2, 5, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			woff2Data := woff2CollectionSharedInvalidRefsWOFF2(t, tt.secondFaceRefs)

			_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
			if err == nil {
				t.Fatal("expected invalid shared outline references to fail")
			}
			if !strings.Contains(err.Error(), "WOFF2 collection font has mismatched glyf/loca tables") {
				t.Fatalf("error = %q, want collection glyf/loca mismatch", err)
			}
		})
	}
}

func TestWOFF2CollectionSharedRejectsNonZeroSharedTransformedLocaPayload(t *testing.T) {
	transformedGlyf := testWOFF2TransformedSimpleGlyf()
	head := woff2CollectionSharedHead(1000)
	maxp := testMaxpTable(1)
	entries := []woff2CollectionSharedEntry{
		woff2CollectionSharedRawEntry(1, head),
		woff2CollectionSharedRawEntry(4, maxp),
		woff2CollectionSharedTransformedEntry(10, 20, transformedGlyf),
		{
			flag:               11,
			origLength:         4,
			transformLength:    1,
			hasTransformLength: true,
			payload:            []byte{0},
		},
	}
	woff2Data := woff2CollectionSharedWOFF2(t, entries, [][]uint16{{0, 1, 2, 3}})

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected non-zero shared transformed loca payload to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 transformed loca table must have zero length") {
		t.Fatalf("error = %q, want transformed loca zero-length error", err)
	}
}

func woff2CollectionSharedInvalidRefsWOFF2(t *testing.T, secondFaceRefs []uint16) []byte {
	t.Helper()
	transformedGlyf := testWOFF2TransformedSimpleGlyf()
	rawGlyf, rawLoca, err := reconstructWOFF2GlyfLoca(transformedGlyf, 4)
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}

	entries := []woff2CollectionSharedEntry{
		woff2CollectionSharedRawEntry(1, woff2CollectionSharedHead(1000)),
		woff2CollectionSharedRawEntry(1, woff2CollectionSharedHead(1200)),
		woff2CollectionSharedRawEntry(4, testMaxpTable(1)),
		woff2CollectionSharedTransformedEntry(10, uint32(len(rawGlyf)), transformedGlyf),
		woff2CollectionSharedTransformedEntry(11, uint32(len(rawLoca)), nil),
		woff2CollectionSharedRawEntry(0xc0|10, rawGlyf),
		woff2CollectionSharedRawEntry(0xc0|11, rawLoca),
	}
	return woff2CollectionSharedWOFF2(t, entries, [][]uint16{
		{0, 2, 3, 4},
		secondFaceRefs,
	})
}

func woff2CollectionSharedWOFF2(t *testing.T, entries []woff2CollectionSharedEntry, faceRefs [][]uint16) []byte {
	t.Helper()

	var directory bytes.Buffer
	var payload bytes.Buffer
	for _, entry := range entries {
		directory.WriteByte(entry.flag)
		directory.Write(testBase128(entry.origLength))
		if entry.hasTransformLength {
			directory.Write(testBase128(entry.transformLength))
		}
		payload.Write(entry.payload)
	}

	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(uint16(len(faceRefs))))
	for _, refs := range faceRefs {
		directory.Write(testUInt255(uint16(len(refs))))
		appendUint32(&directory, 0x00010000)
		for _, ref := range refs {
			directory.Write(testUInt255(ref))
		}
	}

	return testWOFF2CollectionWithDirectoryAndPayload(t, uint16(len(entries)), directory.Bytes(), payload.Bytes())
}

func woff2CollectionSharedRawEntry(flag byte, data []byte) woff2CollectionSharedEntry {
	return woff2CollectionSharedEntry{
		flag:       flag,
		origLength: uint32(len(data)),
		payload:    data,
	}
}

func woff2CollectionSharedTransformedEntry(flag byte, origLength uint32, payload []byte) woff2CollectionSharedEntry {
	return woff2CollectionSharedEntry{
		flag:               flag,
		origLength:         origLength,
		transformLength:    uint32(len(payload)),
		hasTransformLength: true,
		payload:            payload,
	}
}

func woff2CollectionSharedHead(unitsPerEm uint16) []byte {
	head := testHeadTable(0)
	binary.BigEndian.PutUint16(head[18:20], unitsPerEm)
	return head
}

func woff2CollectionSharedSimpleAndEmptyGlyf() []byte {
	return testWOFF2TransformedGlyfFromStreams(
		2,
		[]byte{0, 1, 0, 0},
		[]byte{3},
		[]byte{0, 11, 1},
		[]byte{0, 10, 20, 0},
		nil,
		[]byte{0, 0, 0, 0},
		nil,
	)
}

func woff2CollectionSharedFaceOffsets(t *testing.T, font []byte, wantNumFonts int) []int {
	t.Helper()
	if len(font) < 12 {
		t.Fatalf("decoded collection length = %d, want at least 12", len(font))
	}
	if got := binary.BigEndian.Uint32(font[0:4]); got != tagTTCF {
		t.Fatalf("output signature = 0x%08x, want ttcf", got)
	}
	numFonts := int(binary.BigEndian.Uint32(font[8:12]))
	if numFonts != wantNumFonts {
		t.Fatalf("numFonts = %d, want %d", numFonts, wantNumFonts)
	}
	if 12+numFonts*4 > len(font) {
		t.Fatalf("decoded collection offset table exceeds length %d", len(font))
	}
	offsets := make([]int, numFonts)
	for i := range offsets {
		offsets[i] = int(binary.BigEndian.Uint32(font[12+i*4 : 16+i*4]))
	}
	return offsets
}

func woff2CollectionSharedAssertSameBounds(t *testing.T, font []byte, faceOffsets []int, tag uint32) {
	t.Helper()
	wantOffset, wantLength := woff2CollectionSharedTableBounds(t, font, faceOffsets[0], tag)
	for i, faceOffset := range faceOffsets[1:] {
		gotOffset, gotLength := woff2CollectionSharedTableBounds(t, font, faceOffset, tag)
		if gotOffset != wantOffset || gotLength != wantLength {
			t.Fatalf("face %d %s bounds = (%d,%d), want (%d,%d)", i+1, tagString(tag), gotOffset, gotLength, wantOffset, wantLength)
		}
	}
}

func woff2CollectionSharedTableBounds(t *testing.T, font []byte, base int, tag uint32) (uint32, uint32) {
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
