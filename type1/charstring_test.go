package type1

import (
	"bytes"
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestDecryptCharString(t *testing.T) {
	program := t1prog(t1nums(0, 500), t1ops(13, 14))
	plain := append([]byte{1, 2, 3, 4}, program...)
	encrypted := encryptType1Bytes(plain, 4330)

	got, err := DecryptCharString(encrypted, 4)
	if err != nil {
		t.Fatalf("DecryptCharString failed: %v", err)
	}
	if !bytes.Equal(got, program) {
		t.Fatalf("decrypted program = %v, want %v", got, program)
	}

	copyData, err := DecryptCharString(program, -1)
	if err != nil {
		t.Fatalf("DecryptCharString unencrypted failed: %v", err)
	}
	if !bytes.Equal(copyData, program) || len(copyData) > 0 && &copyData[0] == &program[0] {
		t.Fatalf("unencrypted charstring was not copied")
	}
}

func TestDecodeCharStringBasicOutlineAndMetrics(t *testing.T) {
	data := t1prog(
		t1nums(0, 500), t1ops(13), // hsbw
		t1nums(10, 20), t1ops(21), // rmoveto
		t1nums(30, 0, 0, 40, -30, 0), t1ops(5), // rlineto
		t1ops(9, 14), // closepath, endchar
	)

	result, err := DecodeCharString(data, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}
	if result.SideBearing != (api.Vector{}) {
		t.Fatalf("side bearing = %+v, want zero", result.SideBearing)
	}
	if result.Width != (api.Vector{X: 500 * 64}) {
		t.Fatalf("width = %+v, want 500 font units", result.Width)
	}
	wantPoints := []api.Vector{
		{X: 10 * 64, Y: 20 * 64},
		{X: 40 * 64, Y: 20 * 64},
		{X: 40 * 64, Y: 60 * 64},
		{X: 10 * 64, Y: 60 * 64},
	}
	if !vectorsEqual(result.Outline.Points, wantPoints) {
		t.Fatalf("points = %+v, want %+v", result.Outline.Points, wantPoints)
	}
	if got, want := result.Outline.Contours, []int{3}; !intsEqual(got, want) {
		t.Fatalf("contours = %v, want %v", got, want)
	}
	wantSegments := []CharStringSegment{
		{Kind: CharStringSegmentMove, Points: [3]api.Vector{{X: 10 * 64, Y: 20 * 64}}},
		{Kind: CharStringSegmentLine, Points: [3]api.Vector{{X: 40 * 64, Y: 20 * 64}}},
		{Kind: CharStringSegmentLine, Points: [3]api.Vector{{X: 40 * 64, Y: 60 * 64}}},
		{Kind: CharStringSegmentLine, Points: [3]api.Vector{{X: 10 * 64, Y: 60 * 64}}},
		{Kind: CharStringSegmentClose},
	}
	if !segmentsEqual(result.Segments, wantSegments) {
		t.Fatalf("segments = %+v, want %+v", result.Segments, wantSegments)
	}
}

func TestDecodeCharStringStemHintsConsumedAndRecorded(t *testing.T) {
	data := t1prog(
		t1nums(0, 500), t1ops(13),
		t1nums(10, 20, 30, 5), t1ops(1), // hstem
		t1nums(40, 7), t1ops(3), // vstem
		t1nums(0, 0), t1ops(21),
		t1nums(25), t1ops(6),
		t1ops(14),
	)

	result, err := DecodeCharString(data, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}
	wantPoints := []api.Vector{
		{},
		{X: 25 * 64},
	}
	if !vectorsEqual(result.Outline.Points, wantPoints) {
		t.Fatalf("points = %+v, want %+v", result.Outline.Points, wantPoints)
	}
	if got, want := result.Outline.Contours, []int{1}; !intsEqual(got, want) {
		t.Fatalf("contours = %v, want %v", got, want)
	}
	if got, want := len(result.Hints), 2; got != want {
		t.Fatalf("hint count = %d, want %d", got, want)
	}
	assertHint(t, result.Hints[0], CharStringHintStem, "hstem", []float64{10, 20, 30, 5}, []CharStringStemHint{
		{Orientation: CharStringStemHorizontal, Position: 10, Width: 20},
		{Orientation: CharStringStemHorizontal, Position: 30, Width: 5},
	})
	assertHint(t, result.Hints[1], CharStringHintStem, "vstem", []float64{40, 7}, []CharStringStemHint{
		{Orientation: CharStringStemVertical, Position: 40, Width: 7},
	})
}

func TestDecodeCharStringCounterHintsDotsectionAndTripleStems(t *testing.T) {
	data := t1prog(
		t1nums(0, 500), t1ops(13),
		t1nums(10, 1, 20, 2, 30, 3), t1ops(12, 1), // vstem3
		t1ops(12, 0),                              // dotsection
		t1nums(40, 4, 50, 5, 60, 6), t1ops(12, 2), // hstem3
		t1nums(0, 0), t1ops(21),
		t1nums(15), t1ops(7),
		t1ops(14),
	)

	result, err := DecodeCharString(data, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}
	wantPoints := []api.Vector{
		{},
		{Y: 15 * 64},
	}
	if !vectorsEqual(result.Outline.Points, wantPoints) {
		t.Fatalf("points = %+v, want %+v", result.Outline.Points, wantPoints)
	}
	if got, want := result.Outline.Contours, []int{1}; !intsEqual(got, want) {
		t.Fatalf("contours = %v, want %v", got, want)
	}
	if got, want := len(result.Hints), 3; got != want {
		t.Fatalf("hint count = %d, want %d", got, want)
	}
	assertHint(t, result.Hints[0], CharStringHintCounter, "vstem3", []float64{10, 1, 20, 2, 30, 3}, []CharStringStemHint{
		{Orientation: CharStringStemVertical, Position: 10, Width: 1},
		{Orientation: CharStringStemVertical, Position: 20, Width: 2},
		{Orientation: CharStringStemVertical, Position: 30, Width: 3},
	})
	assertHint(t, result.Hints[1], CharStringHintDotSection, "dotsection", nil, nil)
	assertHint(t, result.Hints[2], CharStringHintCounter, "hstem3", []float64{40, 4, 50, 5, 60, 6}, []CharStringStemHint{
		{Orientation: CharStringStemHorizontal, Position: 40, Width: 4},
		{Orientation: CharStringStemHorizontal, Position: 50, Width: 5},
		{Orientation: CharStringStemHorizontal, Position: 60, Width: 6},
	})
}

func TestDecodeCharStringSubrAndDiv(t *testing.T) {
	subrs := [][]byte{
		t1prog(t1nums(100, 2), t1ops(12, 12), t1ops(6, 11)), // 100 2 div, hlineto, return
	}
	data := t1prog(
		t1nums(0, 500), t1ops(13),
		t1nums(10, 20), t1ops(21),
		t1nums(0), t1ops(10),
		t1ops(14),
	)

	result, err := DecodeCharString(data, subrs)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}
	wantPoints := []api.Vector{
		{X: 10 * 64, Y: 20 * 64},
		{X: 60 * 64, Y: 20 * 64},
	}
	if !vectorsEqual(result.Outline.Points, wantPoints) {
		t.Fatalf("points = %+v, want %+v", result.Outline.Points, wantPoints)
	}
}

func TestDecodeCharStringCurveOperatorsFlattenToLineSegments(t *testing.T) {
	data := t1prog(
		t1nums(0, 500), t1ops(13),
		t1nums(0, 0), t1ops(21),
		t1nums(10, 0, 0, 10, 10, 0), t1ops(8), // rrcurveto
		t1nums(20, 10, 0, 20), t1ops(30), // vhcurveto
		t1nums(30, 0, -10, -20), t1ops(31), // hvcurveto
		t1ops(14),
	)

	result, err := DecodeCharString(data, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}

	points := result.Outline.Points
	if got, want := len(points), 31; got != want {
		t.Fatalf("point count = %d, want %d", got, want)
	}
	if got, want := len(result.Outline.Tags), len(points); got != want {
		t.Fatalf("tag count = %d, want %d", got, want)
	}
	// core/raster currently expose only on/off-curve tags, so Type 1 cubics
	// are flattened into on-curve line points rather than stored as cubics.
	for i, tag := range result.Outline.Tags {
		if tag != 1 {
			t.Fatalf("tag[%d] = %d, want on-curve flattened line point", i, tag)
		}
	}

	wantPoints := map[int]api.Vector{
		0:  {},
		10: {X: 20 * 64, Y: 10 * 64},
		20: {X: 50 * 64, Y: 30 * 64},
		30: {X: 80 * 64, Y: 0},
	}
	for i, want := range wantPoints {
		if got := points[i]; got != want {
			t.Fatalf("point[%d] = %+v, want %+v", i, got, want)
		}
	}
	if got, want := result.Outline.Contours, []int{30}; !intsEqual(got, want) {
		t.Fatalf("contours = %v, want %v", got, want)
	}
	wantSegments := []CharStringSegment{
		{Kind: CharStringSegmentMove, Points: [3]api.Vector{{}}},
		{Kind: CharStringSegmentCubic, Points: [3]api.Vector{
			{X: 10 * 64, Y: 0},
			{X: 10 * 64, Y: 10 * 64},
			{X: 20 * 64, Y: 10 * 64},
		}},
		{Kind: CharStringSegmentCubic, Points: [3]api.Vector{
			{X: 20 * 64, Y: 30 * 64},
			{X: 30 * 64, Y: 30 * 64},
			{X: 50 * 64, Y: 30 * 64},
		}},
		{Kind: CharStringSegmentCubic, Points: [3]api.Vector{
			{X: 80 * 64, Y: 30 * 64},
			{X: 80 * 64, Y: 20 * 64},
			{X: 80 * 64, Y: 0},
		}},
	}
	if !segmentsEqual(result.Segments, wantSegments) {
		t.Fatalf("segments = %+v, want %+v", result.Segments, wantSegments)
	}
}

func TestDecodeCharStringSEAC(t *testing.T) {
	result, err := DecodeCharString(t1prog(t1nums(10, 20, 30, 65, 180), t1ops(12, 6, 14)), nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}
	if result.SEAC == nil {
		t.Fatal("expected SEAC metadata")
	}
	if *result.SEAC != (CharStringSEAC{ASB: 10, ADX: 20, ADY: 30, BaseChar: 65, AccentChar: 180}) {
		t.Fatalf("SEAC = %+v", *result.SEAC)
	}
}

func TestDecodeMetrics(t *testing.T) {
	sideBearing, width, seac, err := DecodeMetrics(
		t1prog(
			t1nums(7, 9, 600, 0), t1ops(12, 7), // sbw
			t1nums(10, 20, 30, 65, 180), t1ops(12, 6),
			t1ops(14),
		),
		nil,
	)
	if err != nil {
		t.Fatalf("DecodeMetrics failed: %v", err)
	}
	if sideBearing != (api.Vector{X: 7 * 64, Y: 9 * 64}) {
		t.Fatalf("side bearing = %+v", sideBearing)
	}
	if width != (api.Vector{X: 600 * 64}) {
		t.Fatalf("width = %+v", width)
	}
	if seac == nil || *seac != (CharStringSEAC{ASB: 10, ADX: 20, ADY: 30, BaseChar: 65, AccentChar: 180}) {
		t.Fatalf("seac = %+v", seac)
	}
}

func TestDecodeCharStringFlexOtherSubrs(t *testing.T) {
	data := t1prog(
		t1nums(0, 500), t1ops(13),
		t1nums(0, 0), t1ops(21),
		t1nums(0, 1), t1ops(12, 16), // start flex
		t1nums(0, 2), t1ops(12, 16),
		t1nums(10, 0), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(10, 10), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(10, 0), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(10, 0), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(10, -10), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(10, 0), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(0, 0, 0, 3, 0), t1ops(12, 16), // end flex
		t1ops(12, 17, 12, 17, 12, 33), // pop pop setcurrentpoint
		t1ops(14),
	)

	result, err := DecodeCharString(data, nil)
	if err != nil {
		t.Fatalf("DecodeCharString flex failed: %v", err)
	}
	points := result.Outline.Points
	if len(points) != 21 {
		t.Fatalf("flex point count = %d, want 21", len(points))
	}
	if points[0] != (api.Vector{}) {
		t.Fatalf("flex start point = %+v, want zero", points[0])
	}
	if got, want := points[len(points)-1], (api.Vector{X: 60 * 64, Y: 0}); got != want {
		t.Fatalf("flex end point = %+v, want %+v", got, want)
	}
	if got, want := result.Outline.Contours, []int{20}; !intsEqual(got, want) {
		t.Fatalf("flex contours = %v, want %v", got, want)
	}
}

func TestDecodeCharStringRejectsFlexSetcurrentpointWithMissingPop(t *testing.T) {
	data := t1prog(
		t1nums(0, 500), t1ops(13),
		t1nums(0, 0), t1ops(21),
		t1nums(0, 1), t1ops(12, 16),
		t1nums(0, 2), t1ops(12, 16),
		t1nums(10, 0), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(10, 10), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(10, 0), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(10, 0), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(10, -10), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(10, 0), t1ops(21), t1nums(0, 2), t1ops(12, 16),
		t1nums(0, 0, 0, 3, 0), t1ops(12, 16),
		t1ops(12, 17, 12, 33), // one pop is not enough for setcurrentpoint.
		t1ops(14),
	)

	if _, err := DecodeCharString(data, nil); err == nil {
		t.Fatal("DecodeCharString unexpectedly accepted flex setcurrentpoint with a missing pop")
	}
}

func TestDecodeCharStringStandardFlexSubrsPreserveCallSubrOperands(t *testing.T) {
	subrs := standardType1FlexSubrs()
	data := t1prog(
		t1nums(0, 500), t1ops(13),
		t1nums(0, 0), t1ops(21),
		t1nums(1), t1ops(10), // start flex
		t1nums(2), t1ops(10),
		t1nums(10, 0), t1ops(21), t1nums(2), t1ops(10),
		t1nums(10, 10), t1ops(21), t1nums(2), t1ops(10),
		t1nums(10, 0), t1ops(21), t1nums(2), t1ops(10),
		t1nums(10, 0), t1ops(21), t1nums(2), t1ops(10),
		t1nums(10, -3), t1ops(21), t1nums(2), t1ops(10),
		t1nums(10, 0), t1ops(21), t1nums(2), t1ops(10),
		t1nums(10, 60, 7, 0), t1ops(10), // flex depth, final x/y, Subr 0
		t1nums(5), t1ops(6),
		t1ops(14),
	)

	result, err := DecodeCharString(data, subrs)
	if err != nil {
		t.Fatalf("DecodeCharString flex subrs failed: %v", err)
	}
	if got, want := result.Outline.Points[len(result.Outline.Points)-1], (api.Vector{X: 65 * 64, Y: 7 * 64}); got != want {
		t.Fatalf("point after flex = %+v, want %+v", got, want)
	}
	if got, want := result.Outline.Contours, []int{21}; !intsEqual(got, want) {
		t.Fatalf("flex subr contours = %v, want %v", got, want)
	}
	wantSegments := []CharStringSegment{
		{Kind: CharStringSegmentMove, Points: [3]api.Vector{{}}},
		{Kind: CharStringSegmentCubic, Points: [3]api.Vector{
			{X: 10 * 64, Y: 0},
			{X: 20 * 64, Y: 10 * 64},
			{X: 30 * 64, Y: 10 * 64},
		}},
		{Kind: CharStringSegmentCubic, Points: [3]api.Vector{
			{X: 40 * 64, Y: 10 * 64},
			{X: 50 * 64, Y: 7 * 64},
			{X: 60 * 64, Y: 7 * 64},
		}},
		{Kind: CharStringSegmentLine, Points: [3]api.Vector{{X: 65 * 64, Y: 7 * 64}}},
	}
	if !segmentsEqual(result.Segments, wantSegments) {
		t.Fatalf("flex subr segments = %+v, want %+v", result.Segments, wantSegments)
	}
}

func TestDecodeCharStringRejectsMalformedStandardFlexSubrs(t *testing.T) {
	subrs := standardType1FlexSubrs()
	data := t1prog(
		t1nums(0, 500), t1ops(13),
		t1nums(0, 0), t1ops(21),
		t1nums(1), t1ops(10),
		t1nums(2), t1ops(10),
		t1nums(10, 0), t1ops(21), t1nums(2), t1ops(10),
		t1nums(10, 10), t1ops(21), t1nums(2), t1ops(10),
		t1nums(10, 0), t1ops(21), t1nums(2), t1ops(10),
		t1nums(10, 0), t1ops(21), t1nums(2), t1ops(10),
		t1nums(10, -3), t1ops(21), t1nums(2), t1ops(10),
		t1nums(10, 0), t1ops(21), t1nums(2), t1ops(10),
		t1nums(0), t1ops(10), // Subr 0 without the three flex end arguments.
	)

	if _, err := DecodeCharString(data, subrs); err == nil {
		t.Fatal("DecodeCharString unexpectedly accepted malformed flex subr end")
	}
}

func TestDecodeCharStringCallSubrPreservesOperandStackLimit(t *testing.T) {
	subrs := [][]byte{
		t1prog(t1nums(1, 2), t1ops(11)),
	}
	data := t1prog(
		t1repeatNum(23, 0),
		t1nums(0), t1ops(10),
	)

	if _, err := DecodeCharString(data, subrs); err == nil {
		t.Fatal("DecodeCharString unexpectedly exceeded operand stack limit through callsubr")
	}
}

func TestDecodeCharStringOtherSubrPopPreservesOperandStackLimit(t *testing.T) {
	data := t1prog(
		t1nums(0, 500), t1ops(13),
		t1nums(7, 1, 3), t1ops(12, 16), // OtherSubr 3 returns one value via pop.
		t1repeatNum(24, 0),
		t1ops(12, 17, 14),
	)

	if _, err := DecodeCharString(data, nil); err == nil {
		t.Fatal("DecodeCharString unexpectedly exceeded operand stack limit through pop")
	}
}

func TestDecodeCharStringCounterControlOtherSubrsClearStack(t *testing.T) {
	tests := []struct {
		name   string
		subrNo int
		op     string
	}{
		{name: "othersubr 12", subrNo: 12, op: "othersubr12"},
		{name: "othersubr 13", subrNo: 13, op: "othersubr13"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := t1prog(
				t1nums(0, 500), t1ops(13),
				t1nums(0, 0), t1ops(21),
				t1nums(123, 1, 2, 2, tt.subrNo), t1ops(12, 16),
				t1nums(10), t1ops(6),
				t1ops(14),
			)

			result, err := DecodeCharString(data, nil)
			if err != nil {
				t.Fatalf("DecodeCharString failed: %v", err)
			}
			wantPoints := []api.Vector{
				{},
				{X: 10 * 64},
			}
			if !vectorsEqual(result.Outline.Points, wantPoints) {
				t.Fatalf("points = %+v, want %+v", result.Outline.Points, wantPoints)
			}
			if got, want := result.Outline.Contours, []int{1}; !intsEqual(got, want) {
				t.Fatalf("contours = %v, want %v", got, want)
			}
			if got, want := len(result.Hints), 1; got != want {
				t.Fatalf("hint count = %d, want %d", got, want)
			}
			assertHint(t, result.Hints[0], CharStringHintCounter, tt.op, []float64{1, 2}, nil)
		})
	}
}

func TestDecodeCharStringRejectsOperandStackOverflow(t *testing.T) {
	if _, err := DecodeCharString(t1repeatNum(25, 0), nil); err == nil {
		t.Fatal("DecodeCharString succeeded with an overfull operand stack")
	}
}

func TestDecodeCharStringRejectsMalformedPrograms(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "truncated long number", data: []byte{255, 0, 0, 0}},
		{name: "line without contour", data: t1prog(t1nums(10, 0), t1ops(5))},
		{name: "unsupported callothersubr", data: t1prog(t1nums(0, 16), t1ops(12, 16))},
		{name: "flex end without vectors", data: t1prog(t1nums(0, 0, 0, 3, 0), t1ops(12, 16))},
		{name: "subr out of range", data: t1prog(t1nums(4), t1ops(10))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeCharString(tt.data, nil); err == nil {
				t.Fatal("DecodeCharString unexpectedly succeeded")
			}
		})
	}
}

func t1prog(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func t1nums(vals ...int) []byte {
	var out []byte
	for _, v := range vals {
		out = append(out, t1n(v)...)
	}
	return out
}

func t1repeatNum(count, val int) []byte {
	var out []byte
	for i := 0; i < count; i++ {
		out = append(out, t1n(val)...)
	}
	return out
}

func standardType1FlexSubrs() [][]byte {
	return [][]byte{
		t1prog(t1nums(3, 0), t1ops(12, 16, 12, 17, 12, 17, 12, 33, 11)),
		t1prog(t1nums(0, 1), t1ops(12, 16, 11)),
		t1prog(t1nums(0, 2), t1ops(12, 16, 11)),
	}
}

func t1ops(ops ...int) []byte {
	out := make([]byte, len(ops))
	for i, op := range ops {
		out[i] = byte(op)
	}
	return out
}

func t1n(v int) []byte {
	switch {
	case v >= -107 && v <= 107:
		return []byte{byte(v + 139)}
	case v >= 108 && v <= 1131:
		x := v - 108
		return []byte{byte(x/256 + 247), byte(x % 256)}
	case v <= -108 && v >= -1131:
		x := -v - 108
		return []byte{byte(x/256 + 251), byte(x % 256)}
	default:
		return []byte{255, byte(uint32(v) >> 24), byte(uint32(v) >> 16), byte(uint32(v) >> 8), byte(uint32(v))}
	}
}

func encryptType1Bytes(data []byte, seed uint16) []byte {
	const c1 = 52845
	const c2 = 22719

	out := make([]byte, len(data))
	r := seed
	for i, plain := range data {
		cipher := plain ^ byte(r>>8)
		out[i] = cipher
		r = (uint16(cipher)+r)*c1 + c2
	}
	return out
}

func vectorsEqual(a, b []api.Vector) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func segmentsEqual(a, b []CharStringSegment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func assertHint(t *testing.T, got CharStringHint, kind CharStringHintKind, op string, operands []float64, stems []CharStringStemHint) {
	t.Helper()
	if got.Kind != kind || got.Operator != op {
		t.Fatalf("hint = %+v, want kind %q operator %q", got, kind, op)
	}
	if !float64sEqual(got.Operands, operands) {
		t.Fatalf("hint operands = %v, want %v", got.Operands, operands)
	}
	if !stemHintsEqual(got.Stems, stems) {
		t.Fatalf("hint stems = %+v, want %+v", got.Stems, stems)
	}
}

func float64sEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stemHintsEqual(a, b []CharStringStemHint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
