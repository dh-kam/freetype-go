package helper

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestWOFF2GlyfLocaEdgeReconstructsAllEmptyGlyphs(t *testing.T) {
	transformed := testWOFF2TransformedGlyfFromStreams(
		3,
		[]byte{0, 0, 0, 0, 0, 0},
		nil,
		nil,
		nil,
		nil,
		[]byte{0, 0, 0, 0},
		nil,
	)

	glyf, loca, err := reconstructWOFF2GlyfLoca(transformed, 8)
	if err != nil {
		t.Fatalf("reconstructWOFF2GlyfLoca failed: %v", err)
	}
	if len(glyf) != 0 {
		t.Fatalf("glyf length = %d, want 0", len(glyf))
	}
	if want := []byte{0, 0, 0, 0, 0, 0, 0, 0}; !bytes.Equal(loca, want) {
		t.Fatalf("loca = % x, want % x", loca, want)
	}
}

func TestWOFF2GlyfLocaEdgeRejectsShortLocaMetadataMismatch(t *testing.T) {
	woff2Data := testWOFF2WithRawOutlineTables(
		t,
		testHeadTable(0),
		testMaxpTable(2),
		make([]byte, 10),
		make([]byte, 4),
	)

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected short loca metadata mismatch")
	}
	if !strings.Contains(err.Error(), "WOFF2 glyf/loca metadata mismatch") {
		t.Fatalf("error = %q, want glyf/loca metadata mismatch", err)
	}
}

func TestWOFF2GlyfLocaEdgeRejectsCompositeComponentIndexAtNumGlyphs(t *testing.T) {
	glyf := woff2EdgeCompositeGlyph(t, 2, nil, nil)
	loca := make([]byte, 6)
	binary.BigEndian.PutUint16(loca[2:4], uint16(len(glyf)/2))
	binary.BigEndian.PutUint16(loca[4:6], uint16(len(glyf)/2))

	woff2Data := testWOFF2WithRawOutlineTables(
		t,
		testHeadTable(0),
		testMaxpTable(2),
		glyf,
		loca,
	)

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected composite component index bounds error")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 composite glyph index") {
		t.Fatalf("error = %q, want composite glyph index error", err)
	}
}

func TestWOFF2GlyfLocaEdgeRejectsRawBBoxYOrdering(t *testing.T) {
	glyf := make([]byte, 10)
	binary.BigEndian.PutUint16(glyf[4:6], 20)  // yMin
	binary.BigEndian.PutUint16(glyf[8:10], 10) // yMax
	loca := make([]byte, 4)
	binary.BigEndian.PutUint16(loca[2:4], uint16(len(glyf)/2))

	woff2Data := testWOFF2WithRawOutlineTables(
		t,
		testHeadTable(0),
		testMaxpTable(1),
		glyf,
		loca,
	)

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected invalid glyph bbox ordering")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 glyph bbox") {
		t.Fatalf("error = %q, want invalid glyph bbox", err)
	}
}

func TestWOFF2GlyfLocaEdgeAllowsCompositeZeroPadding(t *testing.T) {
	glyf := woff2EdgeCompositeGlyph(t, 0, []byte{0xab}, []byte{0})
	loca := make([]byte, 4)
	binary.BigEndian.PutUint16(loca[2:4], uint16(len(glyf)/2))

	woff2Data := testWOFF2WithRawOutlineTables(
		t,
		testHeadTable(0),
		testMaxpTable(1),
		glyf,
		loca,
	)

	if _, err := DecodeWOFF2(core.NewMemoryStream(woff2Data)); err != nil {
		t.Fatalf("DecodeWOFF2 rejected zero-padded composite glyph: %v", err)
	}
}

func TestWOFF2GlyfLocaEdgeRejectsRawCompositeInstructionBounds(t *testing.T) {
	var glyph bytes.Buffer
	appendUint16(&glyph, 0xffff) // numberOfContours = composite
	appendUint16(&glyph, 0)      // xMin
	appendUint16(&glyph, 0)      // yMin
	appendUint16(&glyph, 0)      // xMax
	appendUint16(&glyph, 0)      // yMax
	appendUint16(&glyph, 0x0102) // byte args, xy, instructions
	appendUint16(&glyph, 0)      // glyphIndex
	glyph.Write([]byte{0, 0})    // args
	appendUint16(&glyph, 2)      // instructionLength, but no instruction bytes

	loca := make([]byte, 4)
	binary.BigEndian.PutUint16(loca[2:4], uint16(glyph.Len()/2))

	woff2Data := testWOFF2WithRawOutlineTables(
		t,
		testHeadTable(0),
		testMaxpTable(1),
		glyph.Bytes(),
		loca,
	)

	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected raw composite instruction bounds error")
	}
	if !strings.Contains(err.Error(), "invalid WOFF2 composite glyph instructions") {
		t.Fatalf("error = %q, want composite instruction bounds error", err)
	}
}

func woff2EdgeCompositeGlyph(t *testing.T, componentGlyphIndex uint16, instructions, padding []byte) []byte {
	t.Helper()

	var glyph bytes.Buffer
	appendUint16(&glyph, 0xffff) // numberOfContours = composite
	appendUint16(&glyph, 0)      // xMin
	appendUint16(&glyph, 0)      // yMin
	appendUint16(&glyph, 0)      // xMax
	appendUint16(&glyph, 0)      // yMax

	flags := uint16(0x0002) // args are x/y byte offsets
	if len(instructions) > 0 {
		flags |= 0x0100
	}
	appendUint16(&glyph, flags)
	appendUint16(&glyph, componentGlyphIndex)
	glyph.Write([]byte{0, 0})
	if len(instructions) > 0 {
		appendUint16(&glyph, uint16(len(instructions)))
		glyph.Write(instructions)
	}
	glyph.Write(padding)
	return glyph.Bytes()
}
