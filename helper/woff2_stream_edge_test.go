package helper

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

type woff2StreamEdgeStreams struct {
	nContour    []byte
	nPoints     []byte
	flags       []byte
	glyph       []byte
	composite   []byte
	bbox        []byte
	instruction []byte
}

func TestWOFF2StreamEdgeRejectsExtraTransformedGlyfSubstreamBytes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*woff2StreamEdgeStreams)
	}{
		{
			name: "nContour",
			mutate: func(s *woff2StreamEdgeStreams) {
				s.nContour = append(s.nContour, 0)
			},
		},
		{
			name: "nPoints",
			mutate: func(s *woff2StreamEdgeStreams) {
				s.nPoints = append(s.nPoints, 0)
			},
		},
		{
			name: "flags",
			mutate: func(s *woff2StreamEdgeStreams) {
				s.flags = append(s.flags, 0)
			},
		},
		{
			name: "glyph",
			mutate: func(s *woff2StreamEdgeStreams) {
				s.glyph = append(s.glyph, 0)
			},
		},
		{
			name: "composite",
			mutate: func(s *woff2StreamEdgeStreams) {
				s.composite = append(s.composite, 0)
			},
		},
		{
			name: "bbox",
			mutate: func(s *woff2StreamEdgeStreams) {
				s.bbox = append(s.bbox, 0)
			},
		},
		{
			name: "instruction",
			mutate: func(s *woff2StreamEdgeStreams) {
				s.instruction = append(s.instruction, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams := woff2StreamEdgeSimpleStreams()
			tt.mutate(&streams)
			transformed := testWOFF2TransformedGlyfFromStreams(
				1,
				streams.nContour,
				streams.nPoints,
				streams.flags,
				streams.glyph,
				streams.composite,
				streams.bbox,
				streams.instruction,
			)

			_, _, err := reconstructWOFF2GlyfLoca(transformed, 4)
			if err == nil {
				t.Fatal("expected transformed glyf substream length mismatch")
			}
			if !strings.Contains(err.Error(), "WOFF2 glyf substream length mismatch") {
				t.Fatalf("error = %q, want glyf substream length mismatch", err)
			}
		})
	}
}

func TestWOFF2StreamEdgeRejectsMalformedTransformedGlyfSubstreams(t *testing.T) {
	tests := []struct {
		name        string
		transformed []byte
		want        string
	}{
		{
			name: "missing contour count",
			transformed: testWOFF2TransformedGlyfFromStreams(
				1,
				nil,
				nil,
				nil,
				nil,
				nil,
				[]byte{0, 0, 0, 0},
				nil,
			),
			want: "unexpected EOF",
		},
		{
			name: "missing simple point count",
			transformed: testWOFF2TransformedGlyfFromStreams(
				1,
				[]byte{0, 1},
				nil,
				nil,
				nil,
				nil,
				[]byte{0, 0, 0, 0},
				nil,
			),
			want: "unexpected EOF",
		},
		{
			name: "truncated simple flag stream",
			transformed: testWOFF2TransformedGlyfFromStreams(
				1,
				[]byte{0, 1},
				[]byte{3},
				[]byte{0, 11},
				[]byte{0, 10, 20, 0},
				nil,
				[]byte{0, 0, 0, 0},
				nil,
			),
			want: "unexpected EOF",
		},
		{
			name: "truncated simple glyph triplets",
			transformed: testWOFF2TransformedGlyfFromStreams(
				1,
				[]byte{0, 1},
				[]byte{3},
				[]byte{0, 11, 1},
				[]byte{0, 10},
				nil,
				[]byte{0, 0, 0, 0},
				nil,
			),
			want: "unexpected EOF",
		},
		{
			name: "truncated bbox bitmap",
			transformed: testWOFF2TransformedGlyfFromStreams(
				1,
				[]byte{0, 1},
				[]byte{3},
				[]byte{0, 11, 1},
				[]byte{0, 10, 20, 0},
				nil,
				[]byte{0, 0, 0},
				nil,
			),
			want: "invalid WOFF2 bbox stream",
		},
		{
			name: "truncated explicit bbox",
			transformed: testWOFF2TransformedGlyfFromStreams(
				1,
				[]byte{0, 1},
				[]byte{3},
				[]byte{0, 11, 1},
				[]byte{0, 10, 20, 0},
				nil,
				[]byte{
					0x80, 0, 0, 0,
					0, 0, 0, 0, 0, 10, 0,
				},
				nil,
			),
			want: "unexpected EOF",
		},
		{
			name: "truncated composite component",
			transformed: testWOFF2TransformedGlyfFromStreams(
				1,
				[]byte{0xff, 0xff},
				nil,
				nil,
				nil,
				[]byte{0, 2, 0},
				[]byte{
					0x80, 0, 0, 0,
					0, 0, 0, 0, 0, 0, 0, 0,
				},
				nil,
			),
			want: "unexpected EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := reconstructWOFF2GlyfLoca(tt.transformed, 4)
			if err == nil {
				t.Fatal("expected malformed transformed glyf substream to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestWOFF2StreamEdgeRejectsNonZeroTransformedLocaPayload(t *testing.T) {
	transformedGlyf := testWOFF2TransformedSimpleGlyf()

	var directory bytes.Buffer
	directory.WriteByte(byte(10)) // transformed glyf
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // transformed loca
	directory.Write(testBase128(4))
	directory.Write(testBase128(1))

	var payload bytes.Buffer
	payload.Write(transformedGlyf)
	payload.WriteByte(0)

	woff2Data := testWOFF2WithDirectoryAndPayload(t, 2, directory.Bytes(), payload.Bytes())
	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected non-zero transformed loca payload to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 transformed loca table must have zero length") {
		t.Fatalf("error = %q, want transformed loca zero-length error", err)
	}
}

func TestWOFF2StreamEdgeRejectsCollectionSeparatedTransformedGlyfLoca(t *testing.T) {
	head := testHeadTable(0)
	maxp := testMaxpTable(1)
	transformedGlyf := testWOFF2TransformedSimpleGlyf()

	var directory bytes.Buffer
	directory.WriteByte(byte(1)) // head
	directory.Write(testBase128(uint32(len(head))))
	directory.WriteByte(byte(10)) // transformed glyf
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(4)) // maxp separates glyf from loca
	directory.Write(testBase128(uint32(len(maxp))))
	directory.WriteByte(byte(11)) // transformed loca
	directory.Write(testBase128(4))
	directory.Write(testBase128(0))

	appendUint32(&directory, 0x00010000)
	directory.Write(testUInt255(1)) // numFonts
	directory.Write(testUInt255(4)) // table refs in font
	appendUint32(&directory, 0x00010000)
	for i := 0; i < 4; i++ {
		directory.Write(testUInt255(uint16(i)))
	}

	var payload bytes.Buffer
	payload.Write(head)
	payload.Write(transformedGlyf)
	payload.Write(maxp)

	woff2Data := testWOFF2CollectionWithDirectoryAndPayload(t, 4, directory.Bytes(), payload.Bytes())
	_, err := DecodeWOFF2(core.NewMemoryStream(woff2Data))
	if err == nil {
		t.Fatal("expected separated collection glyf/loca transform to fail")
	}
	if !strings.Contains(err.Error(), "WOFF2 transformed glyf table must be followed by loca table") {
		t.Fatalf("error = %q, want collection glyf/loca adjacency error", err)
	}
}

func woff2StreamEdgeSimpleStreams() woff2StreamEdgeStreams {
	return woff2StreamEdgeStreams{
		nContour:    []byte{0, 1},
		nPoints:     []byte{3},
		flags:       []byte{0, 11, 1},
		glyph:       []byte{0, 10, 20, 0},
		composite:   nil,
		bbox:        []byte{0, 0, 0, 0},
		instruction: nil,
	}
}
