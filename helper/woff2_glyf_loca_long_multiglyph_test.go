package helper

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2GlyfLocaLongMultiGlyphReconstructsAndDecodesTables(t *testing.T) {
	transformed := woff2LongMultiGlyphTransformedGlyf()
	wantGlyf := woff2MultiGlyphExpectedGlyf()
	wantLoca := woff2LongMultiGlyphExpectedLoca()

	glyf, loca, err := reconstructWOFF2GlyfLoca(transformed, uint32(len(wantLoca)))
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	if !bytes.Equal(glyf, wantGlyf) {
		t.Fatalf("glyf = % x, want % x", glyf, wantGlyf)
	}
	if !bytes.Equal(loca, wantLoca) {
		t.Fatalf("long loca = % x, want % x", loca, wantLoca)
	}

	woff2Data := woff2LongMultiGlyphWOFF2(t, transformed)
	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)

	head := sfntTable(t, outData, tagHead)
	if got := int16(binary.BigEndian.Uint16(head[50:52])); got != 1 {
		t.Fatalf("decoded indexToLocFormat = %d, want 1", got)
	}
	if got := sfntTable(t, outData, tagGlyf); !bytes.Equal(got, wantGlyf) {
		t.Fatalf("decoded glyf = % x, want % x", got, wantGlyf)
	}
	if got := sfntTable(t, outData, tagLoca); !bytes.Equal(got, wantLoca) {
		t.Fatalf("decoded long loca = % x, want % x", got, wantLoca)
	}
}

func woff2LongMultiGlyphTransformedGlyf() []byte {
	transformed := append([]byte(nil), woff2MultiGlyphTransformedGlyf()...)
	binary.BigEndian.PutUint16(transformed[6:8], 1)
	return transformed
}

func woff2LongMultiGlyphWOFF2(t *testing.T, transformedGlyf []byte) []byte {
	t.Helper()

	head := testHeadTable(1)
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
	directory.Write(testBase128(uint32(len(woff2LongMultiGlyphExpectedLoca()))))
	directory.Write(testBase128(0))

	var payload bytes.Buffer
	payload.Write(head)
	payload.Write(maxp)
	payload.Write(transformedGlyf)
	return testWOFF2WithDirectoryAndPayload(t, 4, directory.Bytes(), payload.Bytes())
}

func woff2LongMultiGlyphExpectedLoca() []byte {
	return []byte{
		0, 0, 0, 0, // empty glyph starts at byte 0
		0, 0, 0, 0, // first simple glyph also starts at byte 0
		0, 0, 0, 16, // second simple glyph starts at byte 16
		0, 0, 0, 36, // glyf table ends at byte 36
	}
}
