package cff

import (
	"bytes"
	"math"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func csNumber(v int) byte {
	if v < -107 || v > 107 {
		panic("test charstring number out of single-byte range")
	}
	return byte(v + 139)
}

func testIndex(objects ...[]byte) *Index {
	offsets := make([]uint32, len(objects)+1)
	offsets[0] = 1
	data := make([]byte, 0)
	next := uint32(1)
	for i, obj := range objects {
		data = append(data, obj...)
		next += uint32(len(obj))
		offsets[i+1] = next
	}
	return &Index{
		Count:   uint16(len(objects)),
		OffSize: 1,
		Offsets: offsets,
		Data:    data,
	}
}

func assertPoint(t *testing.T, outline *core.Outline, idx int, x, y int32) {
	t.Helper()
	if idx >= len(outline.Points) {
		t.Fatalf("point %d missing; outline has %d points", idx, len(outline.Points))
	}
	if outline.Points[idx].X != x*64 || outline.Points[idx].Y != y*64 {
		t.Fatalf("point %d: expected (%d, %d), got (%d, %d)", idx, x*64, y*64, outline.Points[idx].X, outline.Points[idx].Y)
	}
}

func interpretStack(t *testing.T, data []byte) *charStringContext {
	t.Helper()
	ctx := &charStringContext{outline: &core.Outline{}}
	if err := ctx.interpret(data); err != nil {
		t.Fatalf("interpret failed: %v", err)
	}
	return ctx
}

func assertStack(t *testing.T, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("stack length: got %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("stack[%d]: got %v, want %v; full stack %v", i, got[i], want[i], got)
		}
	}
}

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

func TestDecodeCharStringLineAndCurveOperators(t *testing.T) {
	n := csNumber
	data := []byte{
		n(0), n(0), 21, // rmoveto
		n(10), n(20), n(30), 6, // hlineto
		n(5), n(7), 7, // vlineto
		n(10), n(0), n(10), n(10), n(0), n(10), 8, // rrcurveto
		n(10), n(20), n(30), n(40), 31, // hvcurveto
		n(10), n(20), n(30), n(40), 30, // vhcurveto
		14,
	}

	outline, err := DecodeCharString(data, nil, nil, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}
	if len(outline.Points) != 36 {
		t.Fatalf("expected 36 points, got %d", len(outline.Points))
	}
	assertPoint(t, outline, 0, 0, 0)
	assertPoint(t, outline, 3, 40, 20)
	assertPoint(t, outline, 5, 47, 25)
	assertPoint(t, outline, 15, 67, 45)
	assertPoint(t, outline, 25, 97, 115)
	assertPoint(t, outline, 35, 157, 155)
}

func TestDecodeCharStringFlexOperators(t *testing.T) {
	n := csNumber
	data := []byte{
		n(0), n(0), 21, // rmoveto
		n(10), n(20), n(5), n(30), n(40), n(50), n(60), 12, 34, // hflex
		n(10), n(2), n(20), n(3), n(30), n(4), n(40), n(5), n(50), n(6), n(7), 12, 37, // flex1
		14,
	}

	outline, err := DecodeCharString(data, nil, nil, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}
	if len(outline.Points) != 41 {
		t.Fatalf("expected 41 points, got %d", len(outline.Points))
	}
	assertPoint(t, outline, 20, 210, 0)
	assertPoint(t, outline, 40, 367, 0)
}

func TestDecodeCharStringSubrBias(t *testing.T) {
	n := csNumber
	localSubrs := testIndex([]byte{n(10), n(0), 5, 11})
	globalSubrs := testIndex([]byte{n(0), n(20), 5, 11})
	data := []byte{
		n(0), n(0), 21,
		n(-107), 10, // local subr index 0 with bias 107
		n(-107), 29, // global subr index 0 with bias 107
		14,
	}

	outline, err := DecodeCharString(data, globalSubrs, localSubrs, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}
	if len(outline.Points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(outline.Points))
	}
	assertPoint(t, outline, 2, 10, 20)
}

func TestCharStringWidthHintsAndMasks(t *testing.T) {
	n := csNumber
	data := []byte{
		n(55), n(0), n(10), 1, // width + hstem
		19, 0xff, // hintmask with one byte that must be skipped
		n(0), n(0), 21,
		14,
	}
	ctx := &charStringContext{outline: &core.Outline{}}

	if err := ctx.interpret(data); err != nil {
		t.Fatalf("interpret failed: %v", err)
	}
	if !ctx.widthParsed {
		t.Fatalf("expected width to be parsed")
	}
	if ctx.hintCount != 1 {
		t.Fatalf("expected one stem hint, got %d", ctx.hintCount)
	}
	if len(ctx.stack) != 0 {
		t.Fatalf("expected empty stack, got %v", ctx.stack)
	}
	assertPoint(t, ctx.outline, 0, 0, 0)
}

func TestCharStringArithmeticAndLogicalOperators(t *testing.T) {
	n := csNumber
	data := []byte{
		n(10), n(5), 12, 10, // add
		n(20), n(3), 12, 11, // sub
		n(4), n(6), 12, 24, // mul
		n(22), n(7), 12, 12, // div
		n(-9), 12, 9, // abs
		n(9), 12, 14, // neg
		n(9), 12, 26, // sqrt
		n(1), n(0), 12, 3, // and
		n(1), n(0), 12, 4, // or
		n(0), 12, 5, // not
		n(5), n(5), 12, 15, // eq
	}

	ctx := interpretStack(t, data)
	assertStack(t, ctx.stack, []float64{
		15,
		17,
		24,
		float64(22) / 7,
		9,
		-9,
		3,
		0,
		1,
		1,
		1,
	})
}

func TestCharStringStackOperators(t *testing.T) {
	n := csNumber
	tests := []struct {
		name string
		data []byte
		want []float64
	}{
		{name: "drop", data: []byte{n(1), n(2), 12, 18}, want: []float64{1}},
		{name: "dup", data: []byte{n(7), 12, 27}, want: []float64{7, 7}},
		{name: "exch", data: []byte{n(1), n(2), 12, 28}, want: []float64{2, 1}},
		{name: "index", data: []byte{n(10), n(20), n(30), n(1), 12, 29}, want: []float64{10, 20, 30, 20}},
		{name: "index clamps negative", data: []byte{n(10), n(20), n(-1), 12, 29}, want: []float64{10, 20, 20}},
		{name: "index clamps high", data: []byte{n(10), n(20), n(9), 12, 29}, want: []float64{10, 20, 10}},
		{name: "roll positive", data: []byte{n(1), n(2), n(3), n(4), n(4), n(1), 12, 30}, want: []float64{4, 1, 2, 3}},
		{name: "roll negative", data: []byte{n(1), n(2), n(3), n(4), n(4), n(-1), 12, 30}, want: []float64{2, 3, 4, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := interpretStack(t, tt.data)
			assertStack(t, ctx.stack, tt.want)
		})
	}
}

func TestCharStringStorageIfelseRandomAndVsindex(t *testing.T) {
	n := csNumber

	t.Run("put get", func(t *testing.T) {
		ctx := interpretStack(t, []byte{n(42), n(3), 12, 20, n(3), 12, 21})
		assertStack(t, ctx.stack, []float64{42})
	})

	t.Run("ifelse true", func(t *testing.T) {
		ctx := interpretStack(t, []byte{n(10), n(20), n(3), n(4), 12, 22})
		assertStack(t, ctx.stack, []float64{10})
	})

	t.Run("ifelse false", func(t *testing.T) {
		ctx := interpretStack(t, []byte{n(10), n(20), n(4), n(3), 12, 22})
		assertStack(t, ctx.stack, []float64{20})
	})

	t.Run("random", func(t *testing.T) {
		ctx := interpretStack(t, []byte{12, 23, 12, 23})
		if len(ctx.stack) != 2 {
			t.Fatalf("expected two random values, got %v", ctx.stack)
		}
		for _, v := range ctx.stack {
			if v <= 0 || v > 1 {
				t.Fatalf("random value out of range: %v", ctx.stack)
			}
		}
		if ctx.stack[0] == ctx.stack[1] {
			t.Fatalf("expected deterministic sequence to advance, got %v", ctx.stack)
		}
	})

	t.Run("vsindex and blend", func(t *testing.T) {
		ctx := &charStringContext{outline: &core.Outline{}, blendVector: []float64{0.5}}
		data := []byte{
			n(2), 15, // vsindex
			n(10), n(20), n(4), n(-2), n(2), 16, // blend
		}
		if err := ctx.interpret(data); err != nil {
			t.Fatalf("interpret failed: %v", err)
		}
		if ctx.vsIndex != 2 {
			t.Fatalf("vsIndex = %d, want 2", ctx.vsIndex)
		}
		assertStack(t, ctx.stack, []float64{12, 19})
	})
}

func TestLoadGlyphOutlineDirectFixture(t *testing.T) {
	n := csNumber
	face := &CFF{
		CharStringsIndex: *testIndex([]byte{
			n(10), n(20), 21,
			n(5), n(0), 5,
			14,
		}),
	}

	outline, err := face.LoadGlyphOutline(0)
	if err != nil {
		t.Fatalf("LoadGlyphOutline failed: %v", err)
	}
	if len(outline.Points) != 2 {
		t.Fatalf("expected two points, got %d", len(outline.Points))
	}
	assertPoint(t, outline, 0, 10, 20)
	assertPoint(t, outline, 1, 15, 20)

	if _, err := face.LoadGlyphOutline(1); err == nil {
		t.Fatalf("LoadGlyphOutline succeeded for out-of-range glyph")
	}
}

func TestCalculateBias(t *testing.T) {
	tests := []struct {
		count int
		want  int
	}{
		{0, 107},
		{1239, 107},
		{1240, 1131},
		{33899, 1131},
		{33900, 32768},
	}

	for _, tt := range tests {
		if got := calculateBias(tt.count); got != tt.want {
			t.Fatalf("calculateBias(%d) = %d, want %d", tt.count, got, tt.want)
		}
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
		{name: "hintmask", data: []byte{139, 139, 1, 19}},
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
		{name: "hvcurveto short operands", data: []byte{139, 139, 139, 31}},
		{name: "flex short operands", data: []byte{139, 139, 12, 35}},
		{name: "callsubr missing operand", data: []byte{10}},
		{name: "callgsubr missing operand", data: []byte{29}},
		{name: "blend missing count", data: []byte{16}},
		{name: "vsindex missing operand", data: []byte{15}},
		{name: "add missing operand", data: []byte{139, 12, 10}},
		{name: "drop missing operand", data: []byte{12, 18}},
		{name: "dup missing operand", data: []byte{12, 27}},
		{name: "exch missing operand", data: []byte{139, 12, 28}},
		{name: "index missing operand", data: []byte{139, 12, 29}},
		{name: "roll missing operand", data: []byte{139, 12, 30}},
		{name: "put missing operand", data: []byte{139, 12, 20}},
		{name: "get missing operand", data: []byte{12, 21}},
		{name: "ifelse missing operand", data: []byte{139, 139, 139, 12, 22}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeCharString(tt.data, nil, nil, []float64{1}); err == nil {
				t.Fatalf("DecodeCharString succeeded for %s", tt.name)
			}
		})
	}
}
