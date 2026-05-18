package helper

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2GlyfLocaCompositeHmtxDerivesLSBFromCompositeBBox(t *testing.T) {
	transformedGlyf := woff2CollectionCompositeTransformedGlyf()
	woff2Data := woff2CompositeHmtxWOFF2(t, transformedGlyf)

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)

	glyf := sfntTable(t, outData, tagGlyf)
	loca := sfntTable(t, outData, tagLoca)
	compositeStart := int(binary.BigEndian.Uint16(loca[2:4])) * 2
	if compositeStart+4 > len(glyf) {
		t.Fatalf("composite glyph start = %d beyond glyf length %d", compositeStart, len(glyf))
	}
	if got := int16(binary.BigEndian.Uint16(glyf[compositeStart+2 : compositeStart+4])); got != -5 {
		t.Fatalf("composite glyph xMin = %d, want -5", got)
	}

	hmtx := sfntTable(t, outData, tagHmtx)
	wantHmtx := []byte{
		0x01, 0xf4, 0, 0, // glyph 0: advanceWidth=500, lsb=xMin=0
		0x02, 0x58, 0xff, 0xfb, // glyph 1: advanceWidth=600, lsb=composite xMin=-5
	}
	if !bytes.Equal(hmtx, wantHmtx) {
		t.Fatalf("hmtx = % x, want % x", hmtx, wantHmtx)
	}
}

func woff2CompositeHmtxWOFF2(t *testing.T, transformedGlyf []byte) []byte {
	t.Helper()

	head := testHeadTable(0)
	hhea := testHheaTable(2)
	maxp := testMaxpTable(2)
	hmtx := []byte{1, 0x01, 0xf4, 0x02, 0x58}
	wantGlyf := woff2CollectionCompositeExpectedGlyf()

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(2)) // hhea
	directory.Write(testBase128(uint32(len(hhea))))
	directory.WriteByte(byte(0x40 | 3)) // transformed hmtx
	directory.Write(testBase128(8))
	directory.Write(testBase128(uint32(len(hmtx))))
	directory.WriteByte(byte(4)) // maxp
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(10)) // transformed glyf
	directory.Write(testBase128(uint32(len(wantGlyf))))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // transformed loca
	directory.Write(testBase128(6))
	directory.Write(testBase128(0))

	var payload bytes.Buffer
	payload.Write(head)
	payload.Write(hhea)
	payload.Write(hmtx)
	payload.Write(maxp)
	payload.Write(transformedGlyf)

	return testWOFF2WithDirectoryAndPayload(t, 6, directory.Bytes(), payload.Bytes())
}
