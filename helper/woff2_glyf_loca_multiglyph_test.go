package helper

import (
	"bytes"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2GlyfLocaMultiGlyphReconstructsEmptyAndSimpleGlyphs(t *testing.T) {
	transformed := woff2MultiGlyphTransformedGlyf()

	glyf, loca, err := reconstructWOFF2GlyfLoca(transformed, 8)
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	if want := woff2MultiGlyphExpectedGlyf(); !bytes.Equal(glyf, want) {
		t.Fatalf("glyf = % x, want % x", glyf, want)
	}
	if want := woff2MultiGlyphExpectedShortLoca(); !bytes.Equal(loca, want) {
		t.Fatalf("loca = % x, want % x", loca, want)
	}
}

func TestWOFF2GlyfLocaMultiGlyphDecodeReconstructsTables(t *testing.T) {
	transformed := woff2MultiGlyphTransformedGlyf()
	woff2Data := woff2MultiGlyphWOFF2(t, transformed)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)

	if got, want := sfntTable(t, outData, tagGlyf), woff2MultiGlyphExpectedGlyf(); !bytes.Equal(got, want) {
		t.Fatalf("decoded glyf = % x, want % x", got, want)
	}
	if got, want := sfntTable(t, outData, tagLoca), woff2MultiGlyphExpectedShortLoca(); !bytes.Equal(got, want) {
		t.Fatalf("decoded loca = % x, want % x", got, want)
	}
}

func woff2MultiGlyphTransformedGlyf() []byte {
	return testWOFF2TransformedGlyfFromStreams(
		3,
		[]byte{0, 0, 0, 1, 0, 1},
		[]byte{1, 3},
		[]byte{11, 0, 11, 1},
		[]byte{10, 0, 0, 10, 20, 0},
		nil,
		[]byte{0, 0, 0, 0},
		nil,
	)
}

func woff2MultiGlyphWOFF2(t *testing.T, transformedGlyf []byte) []byte {
	t.Helper()

	head := testHeadTable(0)
	maxp := testMaxpTable(3)

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(4)) // maxp
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(10)) // transformed glyf
	directory.Write(testBase128(uint32(len(woff2MultiGlyphExpectedGlyf()))))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // transformed loca
	directory.Write(testBase128(uint32(len(woff2MultiGlyphExpectedShortLoca()))))
	directory.Write(testBase128(0))

	var payload bytes.Buffer
	payload.Write(head)
	payload.Write(maxp)
	payload.Write(transformedGlyf)
	return testWOFF2WithDirectoryAndPayload(t, 4, directory.Bytes(), payload.Bytes())
}

func woff2MultiGlyphExpectedGlyf() []byte {
	glyf := append([]byte(nil), testWOFF2OnePointGlyphAtX10()...)
	glyf = append(glyf,
		0, 1, // numberOfContours
		0, 0, 0, 0, 0, 10, 0, 20, // bbox
		0, 2, // endPtsOfContours[0]
		0, 0, // instructionLength
		0x31, 0x33, 0x35, 10, 20, 0, // point data and padding
	)
	return glyf
}

func woff2MultiGlyphExpectedShortLoca() []byte {
	return []byte{
		0, 0, // empty glyph starts at byte 0
		0, 0, // first simple glyph also starts at byte 0
		0, 8, // second simple glyph starts at byte 16
		0, 18, // glyf table ends at byte 36
	}
}
