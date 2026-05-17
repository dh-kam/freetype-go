package ftvar

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

type memStream struct {
	data []byte
}

func (s *memStream) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(s.data)) {
		return 0, nil
	}
	n = copy(p, s.data[off:])
	return n, nil
}

func (s *memStream) Size() int64 {
	return int64(len(s.data))
}

func writeUint16(buf *bytes.Buffer, v uint16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	buf.Write(b[:])
}

func writeUint32(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}

func f2Dot14Bits(v float32) uint16 {
	return uint16(int16(v * 16384))
}

func writeF2Dot14(buf *bytes.Buffer, v float32) {
	writeUint16(buf, f2Dot14Bits(v))
}

func testTag(s string) uint32 {
	return binary.BigEndian.Uint32([]byte(s))
}

func TestVariationEngine_ApplyVariation(t *testing.T) {
	// 1 axis, 1 glyph, 1 point
	// gvar header
	buf := &bytes.Buffer{}
	buf.Write([]byte{0, 1, 0, 0})  // version 1.0
	buf.Write([]byte{0, 1})        // axisCount = 1
	buf.Write([]byte{0, 0})        // sharedTupleCount = 0
	buf.Write([]byte{0, 0, 0, 0})  // sharedTuplesOffset
	buf.Write([]byte{0, 1})        // glyphCount = 1
	buf.Write([]byte{0, 0})        // flags (uint16 offsets)
	buf.Write([]byte{0, 0, 0, 24}) // glyphDataArrayOffset
	// glyphDataOffsets (uint16 * (glyphCount + 1))
	buf.Write([]byte{0, 0})  // offset for glyph 0
	buf.Write([]byte{0, 10}) // offset for glyph 1 (10 units * 2 = 20 bytes data)

	// Glyph 0 Variation Data (at offset 24)
	// tupleVariationCount = 1, dataOffset = 10 (header 4 + tuple header 4 + peak 2)
	glyphData := &bytes.Buffer{}
	glyphData.Write([]byte{0, 1})  // tupleVariationCount
	glyphData.Write([]byte{0, 10}) // dataOffset
	// TupleVariationHeader
	glyphData.Write([]byte{0, 5})    // variationDataSize (5 bytes for serialized data)
	glyphData.Write([]byte{0xA0, 0}) // tupleIndex (Embedded peak + Private point numbers)
	glyphData.Write([]byte{0x20, 0}) // peakTuple [0.5]

	// Serialized Data
	glyphData.Write([]byte{0})  // pointIndices (0 = all points)
	glyphData.Write([]byte{0})  // X delta control (byte-aligned, 1 value)
	glyphData.Write([]byte{10}) // X delta value
	glyphData.Write([]byte{0})  // Y delta control
	glyphData.Write([]byte{20}) // Y delta value

	// Pad gvar buf to reach glyphDataArrayOffset (24)
	for buf.Len() < 24 {
		buf.WriteByte(0)
	}
	buf.Write(glyphData.Bytes())

	stream := &memStream{data: buf.Bytes()}
	gvar, err := ParseGvar(stream)
	if err != nil {
		t.Fatalf("ParseGvar failed: %v", err)
	}

	fvar := &FvarTable{
		Axes: []AxisRecord{
			{Tag: 0x77676874, DefaultValue: 0, MinValue: -65536, MaxValue: 65536}, // 'wght'
		},
	}

	ve := NewVariationEngine(fvar, gvar, nil, nil)
	ve.SetNormalizedCoordinates([]float32{0.5})

	outline := &core.Outline{
		Points: []api.Vector{{X: 100 << 6, Y: 100 << 6}},
	}

	err = ve.ApplyVariation(0, outline)
	if err != nil {
		t.Fatalf("ApplyVariation failed: %v", err)
	}

	points := outline.GetPoints()
	// Expected: scalar = 1.0 (since coords[0] == peak[0])
	// DeltaX = 10, DeltaY = 20
	// New X = 100 + 10 = 110, New Y = 100 + 20 = 120 (in units)
	// In 26.6: 110 << 6, 120 << 6
	if points[0].X != 110<<6 {
		t.Errorf("Expected X %d, got %d", 110<<6, points[0].X)
	}
	if points[0].Y != 120<<6 {
		t.Errorf("Expected Y %d, got %d", 120<<6, points[0].Y)
	}
}

func TestVariationEngine_HVAR(t *testing.T) {
	buf := &bytes.Buffer{}
	buf.Write([]byte{0, 1, 0, 0})  // version 1.0
	buf.Write([]byte{0, 0, 0, 20}) // itemVariationStoreOffset = 20
	buf.Write([]byte{0, 0, 0, 0})  // advanceWidthMappingOffset = 0 (use implicit mapping)
	buf.Write([]byte{0, 0, 0, 0})  // lsbMappingOffset = 0
	buf.Write([]byte{0, 0, 0, 0})  // rsbMappingOffset = 0

	// Pad to offset 20
	for buf.Len() < 20 {
		buf.WriteByte(0)
	}

	// ItemVariationStore
	ivsOffset := buf.Len()
	buf.Write([]byte{0, 1})        // format 1
	buf.Write([]byte{0, 0, 0, 16}) // variationRegionListOffset = 16
	buf.Write([]byte{0, 1})        // variationDataCount = 1
	buf.Write([]byte{0, 0, 0, 26}) // variationDataOffsets[0] = 26

	// VariationRegionList (at ivsOffset + 16 = 36)
	for buf.Len() < ivsOffset+16 {
		buf.WriteByte(0)
	}
	buf.Write([]byte{0, 1}) // axisCount = 1
	buf.Write([]byte{0, 1}) // regionCount = 1
	// Region 0: StartCoord=0, PeakCoord=0.5, EndCoord=1.0 (in F2DOT14, 0.5 = 8192 = 0x2000, 1.0 = 16384 = 0x4000)
	buf.Write([]byte{0, 0, 0x20, 0, 0x40, 0})

	// VariationData (at ivsOffset + 26 = 46)
	for buf.Len() < ivsOffset+26 {
		buf.WriteByte(0)
	}
	buf.Write([]byte{0, 1})   // itemCount = 1
	buf.Write([]byte{0, 1})   // shortDeltaCount = 1
	buf.Write([]byte{0, 1})   // regionIndexCount = 1
	buf.Write([]byte{0, 0})   // regionIndices[0] = 0
	buf.Write([]byte{0, 100}) // deltas = 100

	stream := &memStream{data: buf.Bytes()}
	hvar, err := ParseHVAR(stream)
	if err != nil {
		t.Fatalf("ParseHVAR failed: %v", err)
	}

	fvar := &FvarTable{
		Axes: []AxisRecord{
			{Tag: 0x77676874, DefaultValue: 0, MinValue: -65536, MaxValue: 65536}, // 'wght'
		},
	}

	ve := NewVariationEngine(fvar, nil, hvar, nil)

	// Test at coordinate 0.5 (should get 100% of delta = 100)
	ve.SetNormalizedCoordinates([]float32{0.5})
	delta := ve.GetAdvanceDelta(0)
	if delta != 100 {
		t.Errorf("Expected delta 100, got %d", delta)
	}

	// Test at coordinate 0.25 (should get 50% of delta = 50)
	ve.SetNormalizedCoordinates([]float32{0.25})
	delta = ve.GetAdvanceDelta(0)
	if delta != 50 {
		t.Errorf("Expected delta 50, got %d", delta)
	}
}

func TestVariationEngine_ApplyVariationIUPInterpolatesSparseDeltas(t *testing.T) {
	glyphData := []byte{
		0, 1, // tupleVariationCount
		0, 10, // dataOffset
		0, 8, // variationDataSize
		0xA0, 0, // embedded peak tuple + private point numbers
		0x40, 0, // peak = 1.0
		2, 1, 0, 2, // points 0 and 2
		1, 0, 100, // two X byte deltas: 0, 100
		0x81, // two zero Y deltas
	}
	if len(glyphData)%2 != 0 {
		glyphData = append(glyphData, 0)
	}

	data := make([]byte, 24)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint16(data[12:14], 1)
	binary.BigEndian.PutUint32(data[16:20], 24)
	binary.BigEndian.PutUint16(data[22:24], uint16(len(glyphData)/2))
	stream := &memStream{data: append(data, glyphData...)}

	gvar, err := ParseGvar(stream)
	if err != nil {
		t.Fatalf("ParseGvar failed: %v", err)
	}
	fvar := &FvarTable{
		Axes: []AxisRecord{
			{Tag: 0x77676874, DefaultValue: 0, MinValue: -65536, MaxValue: 65536},
		},
	}
	ve := NewVariationEngine(fvar, gvar, nil, nil)
	ve.SetNormalizedCoordinates([]float32{1})

	outline := &core.Outline{
		Points: []api.Vector{
			{X: 0, Y: 0},
			{X: 50 << 6, Y: 0},
			{X: 100 << 6, Y: 0},
			{X: 150 << 6, Y: 0},
		},
		Contours: []int{3},
	}
	if err := ve.ApplyVariation(0, outline); err != nil {
		t.Fatalf("ApplyVariation failed: %v", err)
	}

	wantX := []int32{0, 100 << 6, 200 << 6, 250 << 6}
	for i, want := range wantX {
		if got := outline.Points[i].X; got != want {
			t.Fatalf("point %d X = %d, want %d", i, got, want)
		}
	}
}

func TestVariationEngine_VVARAdvanceHeightDelta(t *testing.T) {
	data := make([]byte, 60)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[4:8], 24)

	ivs := 24
	binary.BigEndian.PutUint16(data[ivs:ivs+2], 1)
	binary.BigEndian.PutUint32(data[ivs+2:ivs+6], 16)
	binary.BigEndian.PutUint16(data[ivs+6:ivs+8], 1)
	binary.BigEndian.PutUint32(data[ivs+8:ivs+12], 26)

	regionList := ivs + 16
	binary.BigEndian.PutUint16(data[regionList:regionList+2], 1)
	binary.BigEndian.PutUint16(data[regionList+2:regionList+4], 1)
	binary.BigEndian.PutUint16(data[regionList+6:regionList+8], 0x4000)
	binary.BigEndian.PutUint16(data[regionList+8:regionList+10], 0x4000)

	variationData := ivs + 26
	binary.BigEndian.PutUint16(data[variationData:variationData+2], 1)
	binary.BigEndian.PutUint16(data[variationData+2:variationData+4], 1)
	binary.BigEndian.PutUint16(data[variationData+4:variationData+6], 1)
	binary.BigEndian.PutUint16(data[variationData+8:variationData+10], 80)

	vvar, err := ParseVVAR(&memStream{data: data})
	if err != nil {
		t.Fatalf("ParseVVAR failed: %v", err)
	}
	fvar := &FvarTable{
		Axes: []AxisRecord{
			{Tag: 0x77676874, DefaultValue: 0, MinValue: -65536, MaxValue: 65536},
		},
	}
	ve := NewVariationEngine(fvar, nil, nil, vvar)
	ve.SetNormalizedCoordinates([]float32{1})
	if delta := ve.GetAdvanceHeightDelta(0); delta != 80 {
		t.Fatalf("advance height delta = %d, want 80", delta)
	}
}

func TestVariationEngine_SetCoordinatesClampAndReset(t *testing.T) {
	fvar := &FvarTable{
		Axes: []AxisRecord{
			{Tag: 0x77676874, MinValue: FloatToFixed(100), DefaultValue: FloatToFixed(400), MaxValue: FloatToFixed(900)},
			{Tag: 0x77647468, MinValue: FloatToFixed(50), DefaultValue: FloatToFixed(100), MaxValue: FloatToFixed(200)},
		},
	}
	ve := NewVariationEngine(fvar, nil, nil, nil)

	ve.SetNormalizedCoordinates([]float32{2, -2})
	if ve.Coords[0] != 1 || ve.Coords[1] != -1 {
		t.Fatalf("normalized coords got %v, want [1 -1]", ve.Coords)
	}

	ve.SetNormalizedCoordinates([]float32{0.25})
	if ve.Coords[0] != 0.25 || ve.Coords[1] != 0 {
		t.Fatalf("partial normalized coords got %v, want [0.25 0]", ve.Coords)
	}

	ve.SetDesignCoordinates([]Fixed{FloatToFixed(900)})
	if ve.Coords[0] != 1 || ve.Coords[1] != 0 {
		t.Fatalf("design coords got %v, want [1 0]", ve.Coords)
	}
}

func TestParseAvarMapsDesignCoordinates(t *testing.T) {
	buf := &bytes.Buffer{}
	writeUint16(buf, 1) // major
	writeUint16(buf, 0) // minor
	writeUint16(buf, 0) // reserved
	writeUint16(buf, 2) // axisCount

	writeUint16(buf, 4) // axis 0 map count
	writeF2Dot14(buf, -1)
	writeF2Dot14(buf, -1)
	writeF2Dot14(buf, 0)
	writeF2Dot14(buf, 0)
	writeF2Dot14(buf, 0.5)
	writeF2Dot14(buf, 0.75)
	writeF2Dot14(buf, 1)
	writeF2Dot14(buf, 1)

	writeUint16(buf, 3) // axis 1 identity
	writeF2Dot14(buf, -1)
	writeF2Dot14(buf, -1)
	writeF2Dot14(buf, 0)
	writeF2Dot14(buf, 0)
	writeF2Dot14(buf, 1)
	writeF2Dot14(buf, 1)

	avar, err := ParseAvar(&memStream{data: buf.Bytes()})
	if err != nil {
		t.Fatalf("ParseAvar failed: %v", err)
	}
	if got := avar.MapCoord(0, 0.25); got < 0.3749 || got > 0.3751 {
		t.Fatalf("MapCoord(0, 0.25) = %v, want 0.375", got)
	}

	fvar := &FvarTable{
		Axes: []AxisRecord{
			{Tag: 0x77676874, MinValue: FloatToFixed(100), DefaultValue: FloatToFixed(400), MaxValue: FloatToFixed(900)},
			{Tag: 0x77647468, MinValue: FloatToFixed(50), DefaultValue: FloatToFixed(100), MaxValue: FloatToFixed(200)},
		},
	}
	ve := NewVariationEngine(fvar, nil, nil, nil)
	ve.SetAvar(avar)
	ve.SetDesignCoordinates([]Fixed{FloatToFixed(650), FloatToFixed(150)})
	if ve.Coords[0] < 0.7499 || ve.Coords[0] > 0.7501 {
		t.Fatalf("avar-adjusted wght coord = %v, want 0.75", ve.Coords[0])
	}
	if ve.Coords[1] < 0.4999 || ve.Coords[1] > 0.5001 {
		t.Fatalf("identity wdth coord = %v, want 0.5", ve.Coords[1])
	}
}

func TestParseCvarAppliesSharedCVTDeltas(t *testing.T) {
	buf := &bytes.Buffer{}
	writeUint16(buf, 1) // major
	writeUint16(buf, 0) // minor
	writeUint16(buf, tupleSharedPointNumbers|1)
	writeUint16(buf, 14) // dataOffset: header + tuple header + peak
	writeUint16(buf, 2)  // variationDataSize: one packed delta byte
	writeUint16(buf, tupleEmbeddedPeak)
	writeF2Dot14(buf, 0.5)

	buf.Write([]byte{1, 0, 1}) // shared CVT index 1
	buf.Write([]byte{0, 20})   // delta +20

	cvar, err := ParseCvar(&memStream{data: buf.Bytes()}, 1)
	if err != nil {
		t.Fatalf("ParseCvar failed: %v", err)
	}

	cvt := []int32{10, 100, 300}
	if err := cvar.ApplyCVTDeltas(cvt, []float32{0.5}); err != nil {
		t.Fatalf("ApplyCVTDeltas failed: %v", err)
	}
	want := []int32{10, 120, 300}
	for i := range want {
		if cvt[i] != want[i] {
			t.Fatalf("cvt[%d] = %d, want %d", i, cvt[i], want[i])
		}
	}

	deltas, err := cvar.GetCVTDeltas(len(cvt), []float32{0.25})
	if err != nil {
		t.Fatalf("GetCVTDeltas failed: %v", err)
	}
	if deltas[1] != 10 {
		t.Fatalf("half scalar cvar delta = %v, want 10", deltas[1])
	}
}

func TestParseMVARMetricDelta(t *testing.T) {
	ivs := make([]byte, 36)
	binary.BigEndian.PutUint16(ivs[0:2], 1)
	binary.BigEndian.PutUint32(ivs[2:6], 16)
	binary.BigEndian.PutUint16(ivs[6:8], 1)
	binary.BigEndian.PutUint32(ivs[8:12], 26)
	binary.BigEndian.PutUint16(ivs[16:18], 1)
	binary.BigEndian.PutUint16(ivs[18:20], 1)
	binary.BigEndian.PutUint16(ivs[22:24], f2Dot14Bits(0.5))
	binary.BigEndian.PutUint16(ivs[24:26], f2Dot14Bits(1))
	binary.BigEndian.PutUint16(ivs[26:28], 1)
	binary.BigEndian.PutUint16(ivs[28:30], 1)
	binary.BigEndian.PutUint16(ivs[30:32], 1)
	binary.BigEndian.PutUint16(ivs[34:36], 40)

	buf := &bytes.Buffer{}
	writeUint16(buf, 1) // major
	writeUint16(buf, 0) // minor
	writeUint16(buf, 0) // reserved
	writeUint16(buf, 8) // valueRecordSize
	writeUint16(buf, 1) // valueRecordCount
	writeUint16(buf, 20)
	writeUint32(buf, testTag("hasc"))
	writeUint16(buf, 0)
	writeUint16(buf, 0)
	buf.Write(ivs)

	mvar, err := ParseMVAR(&memStream{data: buf.Bytes()})
	if err != nil {
		t.Fatalf("ParseMVAR failed: %v", err)
	}
	if delta := mvar.GetMetricDelta(testTag("hasc"), []float32{0.25}); delta != 20 {
		t.Fatalf("hasc metric delta = %d, want 20", delta)
	}
	if got := mvar.ApplyMetricDelta(testTag("hasc"), 700, []float32{0.5}); got != 740 {
		t.Fatalf("adjusted metric = %d, want 740", got)
	}
	if delta := mvar.GetMetricDelta(testTag("hcla"), []float32{0.5}); delta != 0 {
		t.Fatalf("missing metric delta = %d, want 0", delta)
	}
}
