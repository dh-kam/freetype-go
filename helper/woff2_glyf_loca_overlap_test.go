package helper

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2GlyfLocaOverlapBitmapAppliesPerGlyphAcrossDecode(t *testing.T) {
	transformed := woff2OverlapTransformedGlyf()
	wantGlyf := woff2OverlapExpectedGlyf()
	wantLoca := woff2OverlapExpectedShortLoca()

	glyf, loca, err := reconstructWOFF2GlyfLoca(transformed, uint32(len(wantLoca)))
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	if !bytes.Equal(glyf, wantGlyf) {
		t.Fatalf("glyf = % x, want % x", glyf, wantGlyf)
	}
	if !bytes.Equal(loca, wantLoca) {
		t.Fatalf("loca = % x, want % x", loca, wantLoca)
	}
	woff2OverlapAssertPointFlags(t, glyf)

	woff2Data := woff2OverlapWOFF2(t, transformed)
	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	if got := sfntTable(t, outData, tagGlyf); !bytes.Equal(got, wantGlyf) {
		t.Fatalf("decoded glyf = % x, want % x", got, wantGlyf)
	} else {
		woff2OverlapAssertPointFlags(t, got)
	}
	if got := sfntTable(t, outData, tagLoca); !bytes.Equal(got, wantLoca) {
		t.Fatalf("decoded loca = % x, want % x", got, wantLoca)
	}
}

func woff2OverlapTransformedGlyf() []byte {
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

func woff2OverlapWOFF2(t *testing.T, transformedGlyf []byte) []byte {
	t.Helper()

	head := testHeadTable(0)
	maxp := testMaxpTable(3)
	glyf := woff2OverlapExpectedGlyf()
	loca := woff2OverlapExpectedShortLoca()

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(4)) // maxp
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(10)) // transformed glyf
	directory.Write(testBase128(uint32(len(glyf))))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // transformed loca
	directory.Write(testBase128(uint32(len(loca))))
	directory.Write(testBase128(0))

	var payload bytes.Buffer
	payload.Write(head)
	payload.Write(maxp)
	payload.Write(transformedGlyf)
	return testWOFF2WithDirectoryAndPayload(t, 4, directory.Bytes(), payload.Bytes())
}

func woff2OverlapExpectedGlyf() []byte {
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

func woff2OverlapExpectedShortLoca() []byte {
	return []byte{
		0, 0,
		0, 8,
		0, 16,
		0, 16,
	}
}

func woff2OverlapAssertPointFlags(t *testing.T, glyf []byte) {
	t.Helper()

	const overlapSimple = 1 << 6
	if len(glyf) <= 30 {
		t.Fatalf("glyf length = %d, want overlap flag position available", len(glyf))
	}
	if glyf[14]&overlapSimple != 0 {
		t.Fatalf("glyph 0 point flag = 0x%02x, want OVERLAP_SIMPLE clear", glyf[14])
	}
	if glyf[30]&overlapSimple == 0 {
		t.Fatalf("glyph 1 point flag = 0x%02x, want OVERLAP_SIMPLE set", glyf[30])
	}
}
