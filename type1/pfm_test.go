package type1

import (
	"encoding/binary"
	"reflect"
	"testing"
)

func TestParsePFM(t *testing.T) {
	data, faceOffset, extentOffset := testPFM()

	pfm, err := ParsePFM(data)
	if err != nil {
		t.Fatalf("ParsePFM failed: %v", err)
	}

	if pfm.HeaderSize != PFMHeaderSize {
		t.Fatalf("HeaderSize = %d, want %d", pfm.HeaderSize, PFMHeaderSize)
	}
	if pfm.Version != 0x0100 {
		t.Fatalf("Version = %#04x, want 0x0100", pfm.Version)
	}
	if pfm.DFSize != uint32(len(data)) {
		t.Fatalf("DFSize = %d, want %d", pfm.DFSize, len(data))
	}
	if pfm.FaceOffset != uint32(faceOffset) || pfm.FaceName != "Demo Sans" {
		t.Fatalf("face = offset %d name %q, want offset %d name %q", pfm.FaceOffset, pfm.FaceName, faceOffset, "Demo Sans")
	}
	if pfm.FirstChar != 'A' || pfm.LastChar != 'C' || pfm.DefaultChar != 'A' || pfm.BreakChar != 'B' {
		t.Fatalf("chars = first %d last %d default %d break %d", pfm.FirstChar, pfm.LastChar, pfm.DefaultChar, pfm.BreakChar)
	}
	if pfm.AvgWidth != 500 || pfm.MaxWidth != 900 {
		t.Fatalf("widths = avg %d max %d, want 500 900", pfm.AvgWidth, pfm.MaxWidth)
	}
	if pfm.Weight != 700 || !pfm.Italic || pfm.Charset != 0xEE || pfm.PitchAndFamily != 0x21 {
		t.Fatalf("style = weight %d italic %t charset %#02x pitch/family %#02x", pfm.Weight, pfm.Italic, pfm.Charset, pfm.PitchAndFamily)
	}
	if pfm.ExtensionOffset != PFMHeaderSize || pfm.ExtensionSize != 30 {
		t.Fatalf("extension = offset %d size %d, want %d 30", pfm.ExtensionOffset, pfm.ExtensionSize, PFMHeaderSize)
	}
	if pfm.ExtentTableOffset != uint32(extentOffset) {
		t.Fatalf("ExtentTableOffset = %d, want %d", pfm.ExtentTableOffset, extentOffset)
	}
	if want := []uint16{600, 610, 620}; !reflect.DeepEqual(pfm.ExtentWidths, want) {
		t.Fatalf("ExtentWidths = %v, want %v", pfm.ExtentWidths, want)
	}
}

func TestPFMWidthHelpersUseExtentWidths(t *testing.T) {
	data, _, _ := testPFM()
	pfm, err := ParsePFM(data)
	if err != nil {
		t.Fatalf("ParsePFM failed: %v", err)
	}

	width, ok := pfm.WidthByCode('B')
	if !ok || width != 610 {
		t.Fatalf("WidthByCode('B') = %d, %t, want 610, true", width, ok)
	}

	width, ok = pfm.WidthByRune('C')
	if !ok || width != 620 {
		t.Fatalf("WidthByRune('C') = %d, %t, want 620, true", width, ok)
	}

	if width := pfm.DefaultWidth(); width != 600 {
		t.Fatalf("DefaultWidth() = %d, want 600", width)
	}
}

func TestPFMWidthHelpersReportOutOfRange(t *testing.T) {
	data, _, _ := testPFM()
	pfm, err := ParsePFM(data)
	if err != nil {
		t.Fatalf("ParsePFM failed: %v", err)
	}

	defaultWidth := pfm.DefaultWidth()
	tests := []struct {
		name   string
		lookup func() (uint16, bool)
	}{
		{
			name: "code before range",
			lookup: func() (uint16, bool) {
				return pfm.WidthByCode('A' - 1)
			},
		},
		{
			name: "code after range",
			lookup: func() (uint16, bool) {
				return pfm.WidthByCode('D')
			},
		},
		{
			name: "rune outside byte range",
			lookup: func() (uint16, bool) {
				return pfm.WidthByRune('\u0100')
			},
		},
	}

	for _, tt := range tests {
		width, ok := tt.lookup()
		if ok {
			t.Fatalf("%s ok = true, want false", tt.name)
		}
		if width != defaultWidth {
			t.Fatalf("%s width = %d, want default width %d", tt.name, width, defaultWidth)
		}
	}
}

func TestPFMWidthHelpersFallbackWithoutExtentTable(t *testing.T) {
	pfm := &PFM{
		FirstChar:      'A',
		LastChar:       'C',
		DefaultChar:    0,
		AvgWidth:       500,
		MaxWidth:       900,
		PitchAndFamily: 0x20,
	}

	width, ok := pfm.WidthByCode('B')
	if !ok || width != 500 {
		t.Fatalf("WidthByCode('B') without extents = %d, %t, want 500, true", width, ok)
	}
	if width := pfm.DefaultWidth(); width != 500 {
		t.Fatalf("DefaultWidth() without extents = %d, want 500", width)
	}

	pfm.AvgWidth = 0
	width, ok = pfm.WidthByRune('B')
	if !ok || width != 900 {
		t.Fatalf("WidthByRune('B') without avg width = %d, %t, want 900, true", width, ok)
	}
	if width := pfm.DefaultWidth(); width != 900 {
		t.Fatalf("DefaultWidth() without avg width = %d, want 900", width)
	}
}

func TestPFMMetadataHelpers(t *testing.T) {
	data, _, _ := testPFM()
	pfm, err := ParsePFM(data)
	if err != nil {
		t.Fatalf("ParsePFM failed: %v", err)
	}

	if count := pfm.GlyphCount(); count != 3 {
		t.Fatalf("GlyphCount() = %d, want 3", count)
	}
	first, last := pfm.CharRange()
	if first != 'A' || last != 'C' {
		t.Fatalf("CharRange() = %d, %d, want %d, %d", first, last, 'A', 'C')
	}
	if pfm.IsFixedPitch() {
		t.Fatalf("IsFixedPitch() = true for variable-pitch PFM bit")
	}

	pfm.PitchAndFamily = 0x30
	if !pfm.IsFixedPitch() {
		t.Fatalf("IsFixedPitch() = false for clear variable-pitch bit")
	}
}

func TestParsePFMRejectsMalformedData(t *testing.T) {
	tests := []struct {
		name string
		data func() []byte
	}{
		{
			name: "truncated header",
			data: func() []byte {
				return make([]byte, PFMHeaderSize-1)
			},
		},
		{
			name: "dfSize exceeds data",
			data: func() []byte {
				data, _, _ := testPFM()
				binary.LittleEndian.PutUint32(data[2:6], uint32(len(data)+1))
				return data
			},
		},
		{
			name: "face offset out of bounds",
			data: func() []byte {
				data, _, _ := testPFM()
				binary.LittleEndian.PutUint32(data[105:109], uint32(len(data)))
				return data
			},
		},
		{
			name: "missing face offset",
			data: func() []byte {
				data, _, _ := testPFM()
				binary.LittleEndian.PutUint32(data[105:109], 0)
				return data
			},
		},
		{
			name: "unterminated face name",
			data: func() []byte {
				data, faceOffset, _ := testPFM()
				data[faceOffset+len("Demo Sans")] = '!'
				return data
			},
		},
		{
			name: "extension exceeds dfSize",
			data: func() []byte {
				data, _, _ := testPFM()
				binary.LittleEndian.PutUint16(data[PFMHeaderSize:PFMHeaderSize+2], uint16(len(data)))
				return data
			},
		},
		{
			name: "extent table truncated",
			data: func() []byte {
				data, _, _ := testPFM()
				binary.LittleEndian.PutUint32(data[PFMHeaderSize+6:PFMHeaderSize+10], uint32(len(data)-2))
				return data
			},
		},
		{
			name: "invalid character range",
			data: func() []byte {
				data, _, _ := testPFM()
				data[95] = 'C'
				data[96] = 'A'
				return data
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParsePFM(tt.data()); err == nil {
				t.Fatalf("ParsePFM succeeded for malformed data")
			}
		})
	}
}

func testPFM() ([]byte, int, int) {
	const extensionSize = 30

	faceName := []byte("Demo Sans\x00")
	faceOffset := PFMHeaderSize + extensionSize
	extentOffset := faceOffset + len(faceName)
	size := extentOffset + 3*2

	data := make([]byte, size)
	binary.LittleEndian.PutUint16(data[0:2], 0x0100)
	binary.LittleEndian.PutUint32(data[2:6], uint32(size))
	data[80] = 1
	binary.LittleEndian.PutUint16(data[83:85], 700)
	data[85] = 0xEE
	data[90] = 0x21
	binary.LittleEndian.PutUint16(data[91:93], 500)
	binary.LittleEndian.PutUint16(data[93:95], 900)
	data[95] = 'A'
	data[96] = 'C'
	data[97] = 'A'
	data[98] = 'B'
	binary.LittleEndian.PutUint32(data[105:109], uint32(faceOffset))

	binary.LittleEndian.PutUint16(data[PFMHeaderSize:PFMHeaderSize+2], extensionSize)
	binary.LittleEndian.PutUint32(data[PFMHeaderSize+6:PFMHeaderSize+10], uint32(extentOffset))
	copy(data[faceOffset:], faceName)
	for i, width := range []uint16{600, 610, 620} {
		binary.LittleEndian.PutUint16(data[extentOffset+i*2:extentOffset+i*2+2], width)
	}
	return data, faceOffset, extentOffset
}
