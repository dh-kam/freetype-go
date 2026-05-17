package ftvar

import (
	"bytes"
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
