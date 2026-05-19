package helper

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2CollectionCompositeSharesTransformedGlyfLocaAcrossFaces(t *testing.T) {
	transformedGlyf := woff2CollectionCompositeTransformedGlyf()
	woff2Data := woff2CollectionCompositeWOFF2(t, transformedGlyf, 2, uint32(len(woff2CollectionCompositeExpectedGlyf())))

	outStream, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}
	outData := readTestStream(t, outStream)
	faceOffsets := woff2CollectionCompositeFaceOffsets(t, outData, 2)

	woff2CollectionCompositeAssertSameBounds(t, outData, faceOffsets, tagGlyf)
	woff2CollectionCompositeAssertSameBounds(t, outData, faceOffsets, tagLoca)

	wantGlyf := woff2CollectionCompositeExpectedGlyf()
	for i, faceOffset := range faceOffsets {
		head := sfntTableAt(t, outData, faceOffset, tagHead)
		if got, want := binary.BigEndian.Uint16(head[18:20]), uint16(900+i*100); got != want {
			t.Fatalf("face %d unitsPerEm = %d, want %d", i, got, want)
		}

		glyf := sfntTableAt(t, outData, faceOffset, tagGlyf)
		if !bytes.Equal(glyf, wantGlyf) {
			t.Fatalf("face %d glyf = % x, want % x", i, glyf, wantGlyf)
		}

		loca := sfntTableAt(t, outData, faceOffset, tagLoca)
		for j, want := range []uint16{0, 10, 26} {
			if got := binary.BigEndian.Uint16(loca[j*2 : j*2+2]); got != want {
				t.Fatalf("face %d loca[%d] = %d, want %d", i, j, got, want)
			}
		}
	}
}

func TestWOFF2CollectionCompositeRejectsMalformedSharedTransformedGlyf(t *testing.T) {
	tests := []struct {
		name        string
		transformed []byte
		want        string
	}{
		{
			name: "missing composite bbox",
			transformed: woff2CollectionCompositeOnlyTransformedGlyf(
				woff2CollectionCompositeSingleComponentStream(0),
				nil,
				[]byte{0, 0, 0, 0},
				nil,
			),
			want: "WOFF2 composite glyph missing bbox",
		},
		{
			name: "component glyph index out of range",
			transformed: woff2CollectionCompositeOnlyTransformedGlyf(
				woff2CollectionCompositeSingleComponentStream(2),
				nil,
				woff2CollectionCompositeBBoxStream(),
				nil,
			),
			want: "invalid WOFF2 composite glyph index",
		},
		{
			name: "multiple transform flags",
			transformed: woff2CollectionCompositeOnlyTransformedGlyf(
				[]byte{0, 0x48},
				nil,
				woff2CollectionCompositeBBoxStream(),
				nil,
			),
			want: "invalid WOFF2 composite transform flags",
		},
		{
			name: "truncated two-by-two transform",
			transformed: woff2CollectionCompositeOnlyTransformedGlyf(
				woff2CollectionCompositeTruncatedTwoByTwoStream(),
				nil,
				woff2CollectionCompositeBBoxStream(),
				nil,
			),
			want: "unexpected EOF",
		},
		{
			name: "truncated instruction stream",
			transformed: woff2CollectionCompositeOnlyTransformedGlyf(
				woff2CollectionCompositeInstructionComponentStream(),
				[]byte{2},
				woff2CollectionCompositeBBoxStream(),
				[]byte{0xde},
			),
			want: "invalid WOFF2 composite glyph instructions",
		},
		{
			name: "extra composite stream bytes",
			transformed: woff2CollectionCompositeOnlyTransformedGlyf(
				append(woff2CollectionCompositeSingleComponentStream(0), 0xff),
				nil,
				woff2CollectionCompositeBBoxStream(),
				nil,
			),
			want: "WOFF2 glyf substream length mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			woff2Data := woff2CollectionCompositeWOFF2(t, tt.transformed, 2, 32)

			_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
			if err == nil {
				t.Fatal("expected malformed shared transformed composite glyph to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func woff2CollectionCompositeWOFF2(t *testing.T, transformedGlyf []byte, numGlyphs uint16, glyfOrigLength uint32) []byte {
	t.Helper()

	head0 := woff2CollectionCompositeHead(900)
	head1 := woff2CollectionCompositeHead(1000)
	maxp := testMaxpTable(numGlyphs)
	locaOrigLength := uint32(numGlyphs+1) * 2

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head for face 0
	directory.Write(testBase128(uint32(len(head0))))
	directory.WriteByte(byte(1)) // head for face 1
	directory.Write(testBase128(uint32(len(head1))))
	directory.WriteByte(byte(4)) // shared maxp
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(10)) // shared transformed glyf
	directory.Write(testBase128(glyfOrigLength))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // shared transformed loca
	directory.Write(testBase128(locaOrigLength))
	directory.Write(testBase128(0))

	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(2)) // numFonts
	directory.Write(testUInt255(4)) // face 0 tables
	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(3)) // glyf
	directory.Write(testUInt255(0)) // head0
	directory.Write(testUInt255(4)) // loca
	directory.Write(testUInt255(2)) // maxp
	directory.Write(testUInt255(4)) // face 1 tables
	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(1)) // head1
	directory.Write(testUInt255(4)) // loca
	directory.Write(testUInt255(2)) // maxp
	directory.Write(testUInt255(3)) // glyf

	var payload bytes.Buffer
	payload.Write(head0)
	payload.Write(head1)
	payload.Write(maxp)
	payload.Write(transformedGlyf)

	return testWOFF2CollectionWithDirectoryAndPayload(t, 5, directory.Bytes(), payload.Bytes())
}

func woff2CollectionCompositeHead(unitsPerEm uint16) []byte {
	head := testHeadTable(0)
	binary.BigEndian.PutUint16(head[18:20], unitsPerEm)
	return head
}

func woff2CollectionCompositeTransformedGlyf() []byte {
	var compositeStream bytes.Buffer
	appendUint16(&compositeStream, 0x002b) // words, xy, scale, more
	appendUint16(&compositeStream, 0)      // glyphIndex
	appendUint16(&compositeStream, 5)      // x offset
	appendUint16(&compositeStream, 0)      // y offset
	appendUint16(&compositeStream, 0x4000) // scale = 1.0
	appendUint16(&compositeStream, 0x0042) // byte args, xy scale
	appendUint16(&compositeStream, 0)      // glyphIndex
	compositeStream.Write([]byte{0xfb, 2}) // x offset = -5, y offset = 2
	appendUint16(&compositeStream, 0x4000) // xscale = 1.0
	appendUint16(&compositeStream, 0x2000) // yscale = 0.5

	return testWOFF2TransformedGlyfFromStreams(
		2,
		[]byte{0, 1, 0xff, 0xff},
		[]byte{3},
		[]byte{0, 11, 1},
		[]byte{0, 10, 20, 0},
		compositeStream.Bytes(),
		woff2CollectionCompositeBBoxStream(),
		nil,
	)
}

func woff2CollectionCompositeOnlyTransformedGlyf(compositeStream, glyphStream, bboxStream, instructionStream []byte) []byte {
	return testWOFF2TransformedGlyfFromStreams(
		2,
		[]byte{0, 0, 0xff, 0xff},
		nil,
		nil,
		glyphStream,
		compositeStream,
		bboxStream,
		instructionStream,
	)
}

func woff2CollectionCompositeBBoxStream() []byte {
	return []byte{
		0x40, 0, 0, 0, // bbox bitmap: glyph 1 has explicit bbox
		0xff, 0xfb, // xMin = -5
		0, 0, // yMin = 0
		0, 20, // xMax = 20
		0, 20, // yMax = 20
	}
}

func woff2CollectionCompositeSingleComponentStream(glyphIndex uint16) []byte {
	var compositeStream bytes.Buffer
	appendUint16(&compositeStream, 0x0002) // byte args, xy
	appendUint16(&compositeStream, glyphIndex)
	compositeStream.Write([]byte{0, 0})
	return compositeStream.Bytes()
}

func woff2CollectionCompositeInstructionComponentStream() []byte {
	var compositeStream bytes.Buffer
	appendUint16(&compositeStream, 0x0102) // byte args, xy, instructions
	appendUint16(&compositeStream, 0)
	compositeStream.Write([]byte{0, 0})
	return compositeStream.Bytes()
}

func woff2CollectionCompositeTruncatedTwoByTwoStream() []byte {
	var compositeStream bytes.Buffer
	appendUint16(&compositeStream, 0x0083) // words, xy, two-by-two matrix
	appendUint16(&compositeStream, 0)
	appendUint16(&compositeStream, 0)
	appendUint16(&compositeStream, 0)
	appendUint16(&compositeStream, 0x4000) // xscale
	appendUint16(&compositeStream, 0)      // scale01
	appendUint16(&compositeStream, 0)      // scale10; yscale is missing
	return compositeStream.Bytes()
}

func woff2CollectionCompositeExpectedGlyf() []byte {
	return []byte{
		0, 1, // simple glyph numberOfContours
		0, 0, 0, 0, 0, 10, 0, 20, // simple glyph bbox
		0, 2, // endPtsOfContours[0]
		0, 0, // instructionLength
		0x31, 0x33, 0x35, 10, 20, 0, // point data and padding

		0xff, 0xff, // composite glyph numberOfContours = -1
		0xff, 0xfb, 0, 0, 0, 20, 0, 20, // composite glyph bbox
		0, 0x2b, 0, 0, 0, 5, 0, 0, 0x40, 0,
		0, 0x42, 0, 0, 0xfb, 2, 0x40, 0, 0x20, 0,
		0, 0, // padding
	}
}

func woff2CollectionCompositeFaceOffsets(t *testing.T, font []byte, wantNumFonts int) []int {
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

func woff2CollectionCompositeAssertSameBounds(t *testing.T, font []byte, faceOffsets []int, tag uint32) {
	t.Helper()
	wantOffset, wantLength := woff2CollectionCompositeTableBounds(t, font, faceOffsets[0], tag)
	for i, faceOffset := range faceOffsets[1:] {
		gotOffset, gotLength := woff2CollectionCompositeTableBounds(t, font, faceOffset, tag)
		if gotOffset != wantOffset || gotLength != wantLength {
			t.Fatalf("face %d %s bounds = (%d,%d), want (%d,%d)", i+1, tagString(tag), gotOffset, gotLength, wantOffset, wantLength)
		}
	}
}

func woff2CollectionCompositeTableBounds(t *testing.T, font []byte, base int, tag uint32) (uint32, uint32) {
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
