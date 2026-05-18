package helper

import (
	"bytes"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2GlyfLocaInstructionDecodeSimpleGlyphWithExplicitBBox(t *testing.T) {
	transformed := woff2InstructionExplicitBBoxTransformedGlyf()
	wantGlyf := woff2InstructionExplicitBBoxExpectedGlyf()
	wantLoca := woff2InstructionExplicitBBoxExpectedShortLoca()

	glyf, loca, err := reconstructWOFF2GlyfLoca(transformed, uint32(len(wantLoca)))
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	if !bytes.Equal(glyf, wantGlyf) {
		t.Fatalf("reconstructed glyf = % x, want % x", glyf, wantGlyf)
	}
	if !bytes.Equal(loca, wantLoca) {
		t.Fatalf("reconstructed loca = % x, want % x", loca, wantLoca)
	}

	woff2Data := woff2InstructionExplicitBBoxWOFF2(t, transformed)
	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	if got := sfntTable(t, outData, tagGlyf); !bytes.Equal(got, wantGlyf) {
		t.Fatalf("decoded glyf = % x, want % x", got, wantGlyf)
	}
	if got := sfntTable(t, outData, tagLoca); !bytes.Equal(got, wantLoca) {
		t.Fatalf("decoded loca = % x, want % x", got, wantLoca)
	}
}

func woff2InstructionExplicitBBoxTransformedGlyf() []byte {
	return testWOFF2TransformedGlyfFromStreams(
		1,
		[]byte{0, 1},
		[]byte{3},
		[]byte{0, 11, 1},
		[]byte{0, 10, 20, 3},
		nil,
		[]byte{
			0x80, 0, 0, 0, // bbox bitmap: glyph 0 has explicit bbox
			0xff, 0xf6, // xMin = -10
			0xff, 0xfb, // yMin = -5
			0, 0x78, // xMax = 120
			0, 0x8c, // yMax = 140
		},
		[]byte{0xb0, 0x04, 0x2b},
	)
}

func woff2InstructionExplicitBBoxWOFF2(t *testing.T, transformedGlyf []byte) []byte {
	t.Helper()

	head := testHeadTable(0)
	maxp := testMaxpTable(1)
	glyf := woff2InstructionExplicitBBoxExpectedGlyf()
	loca := woff2InstructionExplicitBBoxExpectedShortLoca()

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

func woff2InstructionExplicitBBoxExpectedGlyf() []byte {
	return []byte{
		0, 1, // numberOfContours
		0xff, 0xf6, 0xff, 0xfb, 0, 0x78, 0, 0x8c, // explicit bbox
		0, 2, // endPtsOfContours[0]
		0, 3, // instructionLength
		0xb0, 0x04, 0x2b, // instructions
		0x31, 0x33, 0x35, 10, 20, // point data
		0, 0, // padding
	}
}

func woff2InstructionExplicitBBoxExpectedShortLoca() []byte {
	return []byte{
		0, 0,
		0, 12,
	}
}
