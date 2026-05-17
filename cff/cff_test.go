package cff

import (
	"bytes"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestParseIndex(t *testing.T) {
	// Case 1: OffSize 1
	data1 := []byte{
		0x00, 0x02, // Count = 2
		0x01,             // OffSize = 1
		0x01, 0x02, 0x04, // Offsets: 1, 2, 4
		'A', 'B', 'C', // Data
	}
	stream1 := core.NewMemoryStream(data1)
	idx1, next1, err := parseIndex(stream1, 0)
	if err != nil {
		t.Fatalf("failed to parse index 1: %v", err)
	}
	if idx1.Count != 2 {
		t.Errorf("expected count 2, got %d", idx1.Count)
	}
	if next1 != int64(len(data1)) {
		t.Errorf("expected next %d, got %d", len(data1), next1)
	}
	obj0, _ := idx1.Get(0)
	if !bytes.Equal(obj0, []byte("A")) {
		t.Errorf("expected obj0 'A', got %q", obj0)
	}
	obj1, _ := idx1.Get(1)
	if !bytes.Equal(obj1, []byte("BC")) {
		t.Errorf("expected obj1 'BC', got %q", obj1)
	}

	// Case 2: OffSize 2
	data2 := []byte{
		0x00, 0x02, // Count = 2
		0x02,                               // OffSize = 2
		0x00, 0x01, 0x00, 0x03, 0x00, 0x06, // Offsets: 1, 3, 6
		'A', 'B', 'C', 'D', 'E', // Data
	}
	stream2 := core.NewMemoryStream(data2)
	idx2, _, err := parseIndex(stream2, 0)
	if err != nil {
		t.Fatalf("failed to parse index 2: %v", err)
	}
	obj2_0, _ := idx2.Get(0)
	if !bytes.Equal(obj2_0, []byte("AB")) {
		t.Errorf("expected obj2_0 'AB', got %q", obj2_0)
	}
	obj2_1, _ := idx2.Get(1)
	if !bytes.Equal(obj2_1, []byte("CDE")) {
		t.Errorf("expected obj2_1 'CDE', got %q", obj2_1)
	}

	// Case 3: Empty Index
	data3 := []byte{0x00, 0x00}
	stream3 := core.NewMemoryStream(data3)
	idx3, next3, err := parseIndex(stream3, 0)
	if err != nil {
		t.Fatalf("failed to parse index 3: %v", err)
	}
	if idx3.Count != 0 {
		t.Errorf("expected count 0, got %d", idx3.Count)
	}
	if next3 != 2 {
		t.Errorf("expected next 2, got %d", next3)
	}
}

func TestParseIndexRejectsInvalidOffsets(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "zero offset",
			data: []byte{
				0x00, 0x01, // Count = 1
				0x01,       // OffSize = 1
				0x00, 0x02, // Offsets include invalid zero
				'A',
			},
		},
		{
			name: "non-monotonic offsets",
			data: []byte{
				0x00, 0x02, // Count = 2
				0x01, // OffSize = 1
				0x01, 0x03, 0x02,
				'A',
			},
		},
		{
			name: "data extends beyond stream",
			data: []byte{
				0x00, 0x01, // Count = 1
				0x01,       // OffSize = 1
				0x01, 0xff, // Declares 254 data bytes
				'A',
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := core.NewMemoryStream(tt.data)
			if _, _, err := parseIndex(stream, 0); err == nil {
				t.Fatalf("parseIndex succeeded for invalid INDEX")
			}
		})
	}
}

func TestParseCFF(t *testing.T) {
	// Construct a minimal CFF blob
	// Header: 1, 0, 4, 4
	header := []byte{1, 0, 4, 4}

	// Name INDEX: 1 name "TestFont"
	nameIndex := []byte{
		0x00, 0x01, // Count
		0x01,       // OffSize
		0x01, 0x09, // Offsets: 1, 9
		'T', 'e', 's', 't', 'F', 'o', 'n', 't',
	}

	// Top DICT INDEX: 1 empty dict
	topDictIndex := []byte{
		0x00, 0x01,
		0x01,
		0x01, 0x02,
		0x00, // Empty dict? (actually dicts are not empty usually but let's say 1 byte of 0)
	}

	// String INDEX: empty
	stringIndex := []byte{0x00, 0x00}

	// Global Subr INDEX: empty
	globalSubrIndex := []byte{0x00, 0x00}

	cffBlob := append(header, nameIndex...)
	cffBlob = append(cffBlob, topDictIndex...)
	cffBlob = append(cffBlob, stringIndex...)
	cffBlob = append(cffBlob, globalSubrIndex...)

	stream := core.NewMemoryStream(cffBlob)
	cff, err := ParseCFF(stream, 0)
	if err != nil {
		t.Fatalf("ParseCFF failed: %v", err)
	}

	if cff.Major != 1 || cff.Minor != 0 {
		t.Errorf("wrong version: %d.%d", cff.Major, cff.Minor)
	}

	if cff.NameIndex.Count != 1 {
		t.Errorf("expected 1 name, got %d", cff.NameIndex.Count)
	}
	name, _ := cff.NameIndex.Get(0)
	if string(name) != "TestFont" {
		t.Errorf("expected TestFont, got %s", name)
	}
}

func TestParseDict(t *testing.T) {
	data := []byte{
		0x8b,       // 0
		0x11,       // operator 17
		0xf7, 0x00, // 108
		0xfb, 0x00, // -108
		0x12,             // operator 18
		0x1c, 0x01, 0x00, // 256 (16-bit)
		0x1d, 0x00, 0x00, 0x01, 0x00, // 256 (32-bit)
		0x0c, 0x07, // operator 12, 7
		0x1e, 0x1a, 0x5f, // 1.5 (BCD)
		0x1e, 0xe0, 0xa1, 0x23, 0xff, // -0.123 (BCD)
		0x01, // operator 1
	}

	dict, err := ParseDict(data)
	if err != nil {
		t.Fatalf("ParseDict failed: %v", err)
	}

	// Op 17: [0]
	if val, ok := dict[17]; !ok || len(val) != 1 || val[0] != 0 {
		t.Errorf("expected op 17 to be [0], got %v", val)
	}

	// Op 18: [108, -108]
	if val, ok := dict[18]; !ok || len(val) != 2 || val[0] != 108 || val[1] != -108 {
		t.Errorf("expected op 18 to be [108, -108], got %v", val)
	}

	// Op 12, 7 (3079): [256, 256]
	if val, ok := dict[12<<8|7]; !ok || len(val) != 2 || val[0] != 256 || val[1] != 256 {
		t.Errorf("expected op 3079 to be [256, 256], got %v", val)
	}

	// Op 1: [1.5, -0.123]
	if val, ok := dict[1]; !ok || len(val) != 2 || val[0] != 1.5 || val[1] != -0.123 {
		t.Errorf("expected op 1 to be [1.5, -0.123], got %v", val)
	}
}

func TestDecodeCharString(t *testing.T) {
	// Simple charstring: 100 100 rmoveto 50 0 lineto 0 50 lineto -50 0 lineto endchar
	// Encodings:
	// 100: 100 + 139 = 239
	// 50: 50 + 139 = 189
	// 0: 0 + 139 = 139
	// -50: -50 + 139 = 89
	// rmoveto: 21
	// lineto: 5
	// endchar: 14
	data := []byte{
		239, 239, 21, // 100 100 rmoveto
		189, 139, 5, // 50 0 lineto
		139, 189, 5, // 0 50 lineto
		89, 139, 5, // -50 0 lineto
		14, // endchar
	}

	outline, err := DecodeCharString(data, nil, nil, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}

	if len(outline.Points) != 4 {
		t.Errorf("expected 4 points, got %d", len(outline.Points))
	}

	// Point 0: (100, 100) -> 6400, 6400
	if outline.Points[0].X != 6400 || outline.Points[0].Y != 6400 {
		t.Errorf("point 0 wrong: %v", outline.Points[0])
	}
	// Point 1: (150, 100) -> 9600, 6400
	if outline.Points[1].X != 9600 || outline.Points[1].Y != 6400 {
		t.Errorf("point 1 wrong: %v", outline.Points[1])
	}
}

func TestDecodeCharStringRejectsTruncatedNumbers(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "short number", data: []byte{28, 0}},
		{name: "positive short number", data: []byte{247}},
		{name: "negative short number", data: []byte{251}},
		{name: "fixed number", data: []byte{255, 0, 1, 2}},
		{name: "escape operator", data: []byte{12}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeCharString(tt.data, nil, nil, nil); err == nil {
				t.Fatalf("DecodeCharString succeeded for truncated %s", tt.name)
			}
		})
	}
}

func TestDecodeCharStringRejectsOperandUnderflow(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "rmoveto missing operands", data: []byte{21}},
		{name: "lineto odd operands", data: []byte{139, 5}},
		{name: "hlineto missing operands", data: []byte{6}},
		{name: "rrcurveto short operands", data: []byte{139, 139, 139, 139, 139, 8}},
		{name: "callsubr missing operand", data: []byte{10}},
		{name: "callgsubr missing operand", data: []byte{29}},
		{name: "blend missing count", data: []byte{16}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeCharString(tt.data, nil, nil, []float64{1}); err == nil {
				t.Fatalf("DecodeCharString succeeded for %s", tt.name)
			}
		})
	}
}
