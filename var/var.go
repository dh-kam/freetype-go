package ftvar

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/dh-kam/freetype-go/api"
)

const (
	tupleSharedPointNumbers = 0x8000
	tupleEmbeddedPeak       = 0x8000
	tupleIntermediateRegion = 0x4000
	tuplePrivatePoints      = 0x2000
	tupleCountMask          = 0x0FFF
)

// Fixed is a 16.16 fixed-point number.
type Fixed int32

func (f Fixed) ToFloat() float32 {
	return float32(f) / 65536.0
}

func FloatToFixed(f float32) Fixed {
	return Fixed(f * 65536.0)
}

// AxisRecord represents a variation axis in 'fvar' table.
type AxisRecord struct {
	Tag          uint32
	MinValue     Fixed
	DefaultValue Fixed
	MaxValue     Fixed
	Flags        uint16
	AxisNameID   uint16
}

// FvarTable represents the 'fvar' font variations table.
type FvarTable struct {
	Axes      []AxisRecord
	Instances []InstanceRecord
}

// InstanceRecord represents a named instance in 'fvar' table.
type InstanceRecord struct {
	SubfamilyNameID  uint16
	Flags            uint16
	Coordinates      []Fixed
	PostScriptNameID uint16 // Optional
}

// AxisValueMap maps a default normalized coordinate to a modified
// normalized coordinate for one axis in the 'avar' table.
type AxisValueMap struct {
	FromCoord float32
	ToCoord   float32
}

// AvarTable represents the 'avar' axis variation table.
type AvarTable struct {
	MajorVersion    uint16
	MinorVersion    uint16
	AxisSegmentMaps [][]AxisValueMap
}

func ParseAvar(s api.Stream) (*AvarTable, error) {
	if s.Size() < 8 {
		return nil, errors.New("avar table too short")
	}
	major, _ := readUint16(s, 0)
	minor, _ := readUint16(s, 2)
	if major != 1 || minor != 0 {
		return nil, fmt.Errorf("unsupported avar version: %d.%d", major, minor)
	}
	axisCount, _ := readUint16(s, 6)
	avar := &AvarTable{
		MajorVersion:    major,
		MinorVersion:    minor,
		AxisSegmentMaps: make([][]AxisValueMap, axisCount),
	}

	offset := int64(8)
	for axis := 0; axis < int(axisCount); axis++ {
		if offset+2 > s.Size() {
			return nil, errors.New("avar segment map truncated")
		}
		positionMapCount, _ := readUint16(s, offset)
		offset += 2
		if offset+int64(positionMapCount)*4 > s.Size() {
			return nil, errors.New("avar axis value map truncated")
		}
		segmentMap := make([]AxisValueMap, positionMapCount)
		for i := 0; i < int(positionMapCount); i++ {
			from, _ := readInt16(s, offset)
			to, _ := readInt16(s, offset+2)
			segmentMap[i] = AxisValueMap{
				FromCoord: f2Dot14ToFloat(from),
				ToCoord:   f2Dot14ToFloat(to),
			}
			offset += 4
		}
		avar.AxisSegmentMaps[axis] = segmentMap
	}
	return avar, nil
}

func (t *AvarTable) MapCoordinates(coords []float32) []float32 {
	mapped := make([]float32, len(coords))
	for i, coord := range coords {
		mapped[i] = t.MapCoord(i, coord)
	}
	return mapped
}

func (t *AvarTable) MapCoord(axisIndex int, coord float32) float32 {
	coord = clampNormalizedCoord(coord)
	if t == nil || axisIndex < 0 || axisIndex >= len(t.AxisSegmentMaps) {
		return coord
	}
	segmentMap := t.AxisSegmentMaps[axisIndex]
	if len(segmentMap) == 0 {
		return coord
	}

	start := segmentMap[0]
	if coord <= start.FromCoord {
		if coord == start.FromCoord {
			return clampNormalizedCoord(start.ToCoord)
		}
		return coord
	}

	for i := 1; i < len(segmentMap); i++ {
		end := segmentMap[i]
		if end.FromCoord <= start.FromCoord || end.ToCoord < start.ToCoord {
			continue
		}
		if coord == end.FromCoord {
			return clampNormalizedCoord(end.ToCoord)
		}
		if coord < end.FromCoord {
			return interpolateAvarCoord(start, end, coord)
		}
		start = end
	}
	return coord
}

func interpolateAvarCoord(start, end AxisValueMap, coord float32) float32 {
	if end.FromCoord == start.FromCoord {
		return clampNormalizedCoord(end.ToCoord)
	}
	ratio := (coord - start.FromCoord) / (end.FromCoord - start.FromCoord)
	return clampNormalizedCoord(start.ToCoord + ratio*(end.ToCoord-start.ToCoord))
}

// GvarTable represents the 'gvar' glyph variations table.
type GvarTable struct {
	Stream           api.Stream
	SharedTuples     [][]float32
	GlyphDataOffsets []uint32
	Flags            uint16
	AxisCount        int
}

// VariationEngine handles calculation of glyph variations.
type VariationEngine struct {
	Axes         []AxisRecord
	Coords       []float32 // Normalized coordinates [-1, 1]
	SharedTuples [][]float32
	Avar         *AvarTable
	Gvar         *GvarTable
	Cvar         *CvarTable
	Hvar         *HVARTable
	Vvar         *VVARTable
	Mvar         *MVARTable
}

func NewVariationEngine(fvar *FvarTable, gvar *GvarTable, hvar *HVARTable, vvar *VVARTable) *VariationEngine {
	ve := &VariationEngine{
		Axes:   fvar.Axes,
		Coords: make([]float32, len(fvar.Axes)),
		Gvar:   gvar,
		Hvar:   hvar,
		Vvar:   vvar,
	}
	if gvar != nil {
		ve.SharedTuples = gvar.SharedTuples
	}
	return ve
}

func (ve *VariationEngine) SetAvar(avar *AvarTable) {
	ve.Avar = avar
}

func (ve *VariationEngine) SetCvar(cvar *CvarTable) {
	ve.Cvar = cvar
}

func (ve *VariationEngine) SetMVAR(mvar *MVARTable) {
	ve.Mvar = mvar
}

func (ve *VariationEngine) SetNormalizedCoordinates(coords []float32) {
	for i := range ve.Coords {
		ve.Coords[i] = 0
	}
	for i, coord := range coords {
		if i >= len(ve.Coords) {
			break
		}
		ve.Coords[i] = clampNormalizedCoord(coord)
	}
}

func (ve *VariationEngine) SetDesignCoordinates(coords []Fixed) {
	normalized := ve.NormalizeDesignCoordinates(coords)
	for i := range ve.Coords {
		ve.Coords[i] = normalized[i]
	}
}

func (ve *VariationEngine) NormalizeDesignCoordinates(coords []Fixed) []float32 {
	normalized := make([]float32, len(ve.Axes))
	for i, axis := range ve.Axes {
		if i >= len(coords) {
			break
		}
		normalized[i] = clampNormalizedCoord(ve.normalize(coords[i], axis))
	}
	if ve.Avar != nil {
		return ve.Avar.MapCoordinates(normalized)
	}
	return normalized
}

func (ve *VariationEngine) normalize(val Fixed, axis AxisRecord) float32 {
	if val < axis.DefaultValue {
		if axis.DefaultValue == axis.MinValue {
			return 0
		}
		return -float32(axis.DefaultValue-val) / float32(axis.DefaultValue-axis.MinValue)
	} else if val > axis.DefaultValue {
		if axis.DefaultValue == axis.MaxValue {
			return 0
		}
		return float32(val-axis.DefaultValue) / float32(axis.MaxValue-axis.DefaultValue)
	}
	return 0
}

func clampNormalizedCoord(coord float32) float32 {
	if coord < -1 {
		return -1
	}
	if coord > 1 {
		return 1
	}
	return coord
}

func (ve *VariationEngine) GetAdvanceDelta(glyphIndex int) int32 {
	if ve.Hvar == nil {
		return 0
	}
	return int32(ve.Hvar.GetAdvanceDelta(glyphIndex, ve.Coords))
}

func (ve *VariationEngine) GetLSBDelta(glyphIndex int) int32 {
	if ve.Hvar == nil {
		return 0
	}
	return int32(ve.Hvar.GetLSBDelta(glyphIndex, ve.Coords))
}

func (ve *VariationEngine) GetAdvanceHeightDelta(glyphIndex int) int32 {
	if ve.Vvar == nil {
		return 0
	}
	return int32(ve.Vvar.GetAdvanceHeightDelta(glyphIndex, ve.Coords))
}

func (ve *VariationEngine) GetTSBDelta(glyphIndex int) int32 {
	if ve.Vvar == nil {
		return 0
	}
	return int32(ve.Vvar.GetTSBDelta(glyphIndex, ve.Coords))
}

func (ve *VariationEngine) GetBSBDelta(glyphIndex int) int32 {
	if ve.Vvar == nil {
		return 0
	}
	return int32(ve.Vvar.GetBSBDelta(glyphIndex, ve.Coords))
}

func (ve *VariationEngine) GetVOrgDelta(glyphIndex int) int32 {
	if ve.Vvar == nil {
		return 0
	}
	return int32(ve.Vvar.GetVOrgDelta(glyphIndex, ve.Coords))
}

func (ve *VariationEngine) ApplyCVTDeltas(cvt []int32) error {
	if ve.Cvar == nil {
		return nil
	}
	return ve.Cvar.ApplyCVTDeltas(cvt, ve.Coords)
}

func (ve *VariationEngine) GetMetricDelta(valueTag uint32) int32 {
	if ve.Mvar == nil {
		return 0
	}
	return ve.Mvar.GetMetricDelta(valueTag, ve.Coords)
}

func (ve *VariationEngine) ApplyMetricDelta(valueTag uint32, value int32) int32 {
	if ve.Mvar == nil {
		return value
	}
	return ve.Mvar.ApplyMetricDelta(valueTag, value, ve.Coords)
}

func (ve *VariationEngine) ApplyVariation(glyphIndex int, outline api.Outline) error {
	if outline == nil {
		return nil
	}
	points := outline.GetPoints()
	deltas, err := ve.GetGlyphDeltas(glyphIndex, points, outline.GetContours())
	if err != nil {
		return err
	}
	for i := range points {
		points[i].X += deltas[i].X
		points[i].Y += deltas[i].Y
	}
	return nil
}

func (ve *VariationEngine) GetGlyphDeltas(glyphIndex int, points []api.Vector, contours []int) ([]api.Vector, error) {
	deltas := make([]api.Vector, len(points))
	if ve.Gvar == nil {
		return deltas, nil
	}
	if glyphIndex < 0 || glyphIndex >= len(ve.Gvar.GlyphDataOffsets)-1 {
		return nil, fmt.Errorf("glyph index %d out of range", glyphIndex)
	}

	startOffset := ve.Gvar.GlyphDataOffsets[glyphIndex]
	endOffset := ve.Gvar.GlyphDataOffsets[glyphIndex+1]
	if startOffset == endOffset {
		return deltas, nil // No variation for this glyph
	}

	s := ve.Gvar.Stream
	numPoints := len(points)

	tupleVariationCount, _ := readUint16(s, int64(startOffset))
	dataOffset, _ := readUint16(s, int64(startOffset)+2)

	count := tupleVariationCount & 0x0FFF
	sharedDataOffset := int64(startOffset) + int64(dataOffset)

	currHeaderOffset := int64(startOffset) + 4
	currDataOffset := sharedDataOffset
	var sharedPointIndices []int
	if tupleVariationCount&0x8000 != 0 {
		points, next, err := ve.unpackPointIndices(s, currDataOffset, numPoints)
		if err != nil {
			return nil, err
		}
		sharedPointIndices = points
		currDataOffset = next
	}

	deltasX := make([]float32, numPoints)
	deltasY := make([]float32, numPoints)

	for i := 0; i < int(count); i++ {
		variationDataSize, _ := readUint16(s, currHeaderOffset)
		tupleIndex, _ := readUint16(s, currHeaderOffset+2)
		currHeaderOffset += 4

		var peakTuple []float32
		var intermediateStart []float32
		var intermediateEnd []float32

		if tupleIndex&0x8000 != 0 { // EMBEDDED_PEAK_TUPLE
			peakTuple = make([]float32, ve.Gvar.AxisCount)
			for j := 0; j < ve.Gvar.AxisCount; j++ {
				v, _ := readInt16(s, currHeaderOffset)
				peakTuple[j] = float32(v) / 16384.0
				currHeaderOffset += 2
			}
		} else {
			sharedIndex := tupleIndex & 0x0FFF
			if int(sharedIndex) < len(ve.SharedTuples) {
				peakTuple = ve.SharedTuples[sharedIndex]
			}
		}

		if tupleIndex&0x4000 != 0 { // INTERMEDIATE_REGION
			intermediateStart = make([]float32, ve.Gvar.AxisCount)
			for j := 0; j < ve.Gvar.AxisCount; j++ {
				v, _ := readInt16(s, currHeaderOffset)
				intermediateStart[j] = float32(v) / 16384.0
				currHeaderOffset += 2
			}
			intermediateEnd = make([]float32, ve.Gvar.AxisCount)
			for j := 0; j < ve.Gvar.AxisCount; j++ {
				v, _ := readInt16(s, currHeaderOffset)
				intermediateEnd[j] = float32(v) / 16384.0
				currHeaderOffset += 2
			}
		}

		scalar := ve.calculateScalar(peakTuple, intermediateStart, intermediateEnd)
		if scalar == 0 {
			currDataOffset += int64(variationDataSize)
			continue
		}

		tupleDataOffset := currDataOffset
		tupleDeltasX, tupleDeltasY, touched, nextDataOffset, err := ve.unpackDeltas(s, tupleDataOffset, numPoints, tupleIndex, sharedPointIndices)
		if err != nil {
			return nil, err
		}
		currDataOffset = nextDataOffset
		if tupleEnd := tupleDataOffset + int64(variationDataSize); tupleEnd > currDataOffset {
			currDataOffset = tupleEnd
		}

		interpolateUntouchedDeltas(points, contours, tupleDeltasX, tupleDeltasY, touched)
		for j := 0; j < numPoints; j++ {
			deltasX[j] += tupleDeltasX[j] * scalar
			deltasY[j] += tupleDeltasY[j] * scalar
		}
	}

	for i := range deltas {
		deltas[i].X = int32(deltasX[i] * 64) // Convert back to 26.6
		deltas[i].Y = int32(deltasY[i] * 64)
	}
	return deltas, nil
}

func (ve *VariationEngine) calculateScalar(peak, start, end []float32) float32 {
	return calculateTupleScalar(peak, start, end, ve.Coords)
}

func (ve *VariationEngine) unpackDeltas(s api.Stream, offset int64, numPoints int, tupleIndex uint16, sharedPointIndices []int) ([]float32, []float32, []bool, int64, error) {
	dx := make([]float32, numPoints)
	dy := make([]float32, numPoints)
	touched := make([]bool, numPoints)

	curr := offset
	pointIndices := sharedPointIndices

	if tupleIndex&0x2000 != 0 { // PRIVATE_POINT_NUMBERS
		count, next, err := ve.unpackPointIndices(s, curr, numPoints)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		pointIndices = count
		curr = next
	}

	// Unpack X deltas
	dxValues, next, err := ve.unpackDeltaValues(s, curr, len(pointIndices), numPoints)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	curr = next

	// Unpack Y deltas
	dyValues, next, err := ve.unpackDeltaValues(s, curr, len(pointIndices), numPoints)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	curr = next

	if pointIndices == nil {
		for i := 0; i < numPoints; i++ {
			dx[i] = float32(dxValues[i])
			dy[i] = float32(dyValues[i])
			touched[i] = true
		}
	} else {
		for i, idx := range pointIndices {
			if idx < numPoints {
				dx[idx] = float32(dxValues[i])
				dy[idx] = float32(dyValues[i])
				touched[idx] = true
			}
		}
	}

	return dx, dy, touched, curr, nil
}

func interpolateUntouchedDeltas(points []api.Vector, contours []int, dx, dy []float32, touched []bool) {
	if len(points) == 0 || len(contours) == 0 {
		return
	}
	interpolateUntouchedDeltaAxis(points, contours, dx, touched, true)
	interpolateUntouchedDeltaAxis(points, contours, dy, touched, false)
}

func interpolateUntouchedDeltaAxis(points []api.Vector, contours []int, deltas []float32, touched []bool, useX bool) {
	start := 0
	for _, end := range contours {
		if start < 0 || start >= len(points) {
			break
		}
		if end >= len(points) {
			end = len(points) - 1
		}
		if end < start {
			start = end + 1
			continue
		}

		var refs []int
		for i := start; i <= end; i++ {
			if touched[i] {
				refs = append(refs, i)
			}
		}
		switch len(refs) {
		case 0:
		case 1:
			refDelta := deltas[refs[0]]
			for i := start; i <= end; i++ {
				if !touched[i] {
					deltas[i] = refDelta
				}
			}
		default:
			for i, ref1 := range refs {
				ref2 := refs[(i+1)%len(refs)]
				for p := nextContourPoint(ref1, start, end); p != ref2; p = nextContourPoint(p, start, end) {
					if !touched[p] {
						deltas[p] = interpolateDelta(pointCoord(points[p], useX), pointCoord(points[ref1], useX), deltas[ref1], pointCoord(points[ref2], useX), deltas[ref2])
					}
				}
			}
		}
		start = end + 1
	}
}

func nextContourPoint(point, start, end int) int {
	if point >= end {
		return start
	}
	return point + 1
}

func pointCoord(point api.Vector, useX bool) float32 {
	if useX {
		return float32(point.X)
	}
	return float32(point.Y)
}

func interpolateDelta(coord, coord1 float32, delta1 float32, coord2 float32, delta2 float32) float32 {
	if coord1 == coord2 {
		return delta1
	}
	if coord1 < coord2 {
		if coord <= coord1 {
			return delta1
		}
		if coord >= coord2 {
			return delta2
		}
	} else {
		if coord <= coord2 {
			return delta2
		}
		if coord >= coord1 {
			return delta1
		}
	}
	return delta1 + (delta2-delta1)*(coord-coord1)/(coord2-coord1)
}

func (ve *VariationEngine) unpackPointIndices(s api.Stream, offset int64, totalPoints int) ([]int, int64, error) {
	return unpackVariationPointIndices(s, offset)
}

func (ve *VariationEngine) unpackDeltaValues(s api.Stream, offset int64, count int, numPoints int) ([]int16, int64, error) {
	return unpackVariationDeltaValues(s, offset, count, numPoints)
}

func ParseFvar(s api.Stream) (*FvarTable, error) {
	if s.Size() < 16 {
		return nil, errors.New("fvar table too short")
	}
	major, _ := readUint16(s, 0)
	minor, _ := readUint16(s, 2)
	if major != 1 || minor != 0 {
		return nil, fmt.Errorf("unsupported fvar version: %d.%d", major, minor)
	}
	axesOffset, _ := readUint16(s, 4)
	axisCount, _ := readUint16(s, 8)
	axisSize, _ := readUint16(s, 10)
	instanceCount, _ := readUint16(s, 12)
	instanceSize, _ := readUint16(s, 14)
	if axisSize < 20 {
		return nil, errors.New("fvar axis record size too small")
	}
	axisRecordsSize := int64(axisCount) * int64(axisSize)
	if !hasRange(s, int64(axesOffset), axisRecordsSize) {
		return nil, errors.New("fvar axis records truncated")
	}
	baseInstanceSize := int64(axisCount)*4 + 4
	instanceSize64 := int64(instanceSize)
	if instanceCount > 0 && instanceSize64 != baseInstanceSize && instanceSize64 != baseInstanceSize+2 {
		return nil, errors.New("fvar invalid instance record size")
	}
	instancesOffset := int64(axesOffset) + axisRecordsSize
	if instanceCount > 0 && !hasRange(s, instancesOffset, int64(instanceCount)*instanceSize64) {
		return nil, errors.New("fvar instance records truncated")
	}

	fvar := &FvarTable{
		Axes:      make([]AxisRecord, axisCount),
		Instances: make([]InstanceRecord, instanceCount),
	}
	for i := 0; i < int(axisCount); i++ {
		off := int64(axesOffset) + int64(i)*int64(axisSize)
		fvar.Axes[i].Tag, _ = readUint32(s, off)
		fvar.Axes[i].MinValue = Fixed(readInt32S(s, off+4))
		fvar.Axes[i].DefaultValue = Fixed(readInt32S(s, off+8))
		fvar.Axes[i].MaxValue = Fixed(readInt32S(s, off+12))
		fvar.Axes[i].Flags, _ = readUint16(s, off+16)
		fvar.Axes[i].AxisNameID, _ = readUint16(s, off+18)
		if fvar.Axes[i].MinValue > fvar.Axes[i].DefaultValue || fvar.Axes[i].DefaultValue > fvar.Axes[i].MaxValue {
			return nil, fmt.Errorf("fvar axis %d has invalid min/default/max values", i)
		}
	}
	for i := 0; i < int(instanceCount); i++ {
		off := instancesOffset + int64(i)*instanceSize64
		instance := &fvar.Instances[i]
		instance.SubfamilyNameID, _ = readUint16(s, off)
		instance.Flags, _ = readUint16(s, off+2)
		instance.Coordinates = make([]Fixed, axisCount)
		coordOff := off + 4
		for j := 0; j < int(axisCount); j++ {
			instance.Coordinates[j] = Fixed(readInt32S(s, coordOff+int64(j)*4))
		}
		if instanceSize64 >= baseInstanceSize+2 {
			instance.PostScriptNameID, _ = readUint16(s, off+baseInstanceSize)
		}
	}
	return fvar, nil
}

func ParseGvar(s api.Stream) (*GvarTable, error) {
	if s.Size() < 20 {
		return nil, errors.New("gvar table too short")
	}
	major, _ := readUint16(s, 0)
	minor, _ := readUint16(s, 2)
	if major != 1 || minor != 0 {
		return nil, fmt.Errorf("unsupported gvar version: %d.%d", major, minor)
	}
	axisCount, _ := readUint16(s, 4)
	sharedTupleCount, _ := readUint16(s, 6)
	sharedTuplesOffset, _ := readUint32(s, 8)
	glyphCount, _ := readUint16(s, 12)
	flags, _ := readUint16(s, 14)
	glyphDataArrayOffset, _ := readUint32(s, 16)

	gvar := &GvarTable{
		Stream:           s,
		Flags:            flags,
		GlyphDataOffsets: make([]uint32, glyphCount+1),
		AxisCount:        int(axisCount),
	}
	if sharedTupleCount > 0 {
		gvar.SharedTuples = make([][]float32, sharedTupleCount)
		for i := 0; i < int(sharedTupleCount); i++ {
			gvar.SharedTuples[i] = make([]float32, axisCount)
			for j := 0; j < int(axisCount); j++ {
				v, _ := readInt16(s, int64(sharedTuplesOffset)+int64(i)*int64(axisCount)*2+int64(j)*2)
				gvar.SharedTuples[i][j] = float32(v) / 16384.0
			}
		}
	}
	offsetSize := 2
	if flags&0x0001 != 0 {
		offsetSize = 4
	}
	for i := 0; i <= int(glyphCount); i++ {
		off := int64(20) + int64(i*offsetSize)
		if offsetSize == 2 {
			v, _ := readUint16(s, off)
			gvar.GlyphDataOffsets[i] = glyphDataArrayOffset + uint32(v)*2
		} else {
			v, _ := readUint32(s, off)
			gvar.GlyphDataOffsets[i] = glyphDataArrayOffset + v
		}
	}
	return gvar, nil
}

// CvarTable represents the 'cvar' control value variations table.
type CvarTable struct {
	MajorVersion     uint16
	MinorVersion     uint16
	AxisCount        int
	SharedCVTIndices []int
	Tuples           []CvarTupleVariation
	Stream           api.Stream
}

type CvarTupleVariation struct {
	VariationDataSize uint16
	TupleIndex        uint16
	PeakTuple         []float32
	IntermediateStart []float32
	IntermediateEnd   []float32
	dataOffset        int64
}

func ParseCvar(s api.Stream, axisCount int) (*CvarTable, error) {
	if axisCount < 0 {
		return nil, errors.New("cvar axis count cannot be negative")
	}
	if s.Size() < 8 {
		return nil, errors.New("cvar table too short")
	}
	major, _ := readUint16(s, 0)
	minor, _ := readUint16(s, 2)
	if major != 1 || minor != 0 {
		return nil, fmt.Errorf("unsupported cvar version: %d.%d", major, minor)
	}
	tupleVariationCount, _ := readUint16(s, 4)
	dataOffset, _ := readUint16(s, 6)
	count := int(tupleVariationCount & tupleCountMask)
	if int64(dataOffset) > s.Size() {
		return nil, errors.New("cvar data offset out of range")
	}

	t := &CvarTable{
		MajorVersion: major,
		MinorVersion: minor,
		AxisCount:    axisCount,
		Tuples:       make([]CvarTupleVariation, count),
		Stream:       s,
	}

	headerOffset := int64(8)
	for i := 0; i < count; i++ {
		if headerOffset+4 > s.Size() {
			return nil, errors.New("cvar tuple variation header truncated")
		}
		variationDataSize, _ := readUint16(s, headerOffset)
		tupleIndex, _ := readUint16(s, headerOffset+2)
		headerOffset += 4
		if tupleIndex&tupleEmbeddedPeak == 0 {
			return nil, fmt.Errorf("cvar tuple %d missing embedded peak tuple", i)
		}
		peak, next, err := readF2Dot14Tuple(s, headerOffset, axisCount)
		if err != nil {
			return nil, err
		}
		headerOffset = next

		var start, end []float32
		if tupleIndex&tupleIntermediateRegion != 0 {
			start, next, err = readF2Dot14Tuple(s, headerOffset, axisCount)
			if err != nil {
				return nil, err
			}
			headerOffset = next
			end, next, err = readF2Dot14Tuple(s, headerOffset, axisCount)
			if err != nil {
				return nil, err
			}
			headerOffset = next
		}

		t.Tuples[i] = CvarTupleVariation{
			VariationDataSize: variationDataSize,
			TupleIndex:        tupleIndex,
			PeakTuple:         peak,
			IntermediateStart: start,
			IntermediateEnd:   end,
		}
	}

	currDataOffset := int64(dataOffset)
	if tupleVariationCount&tupleSharedPointNumbers != 0 {
		sharedIndices, next, err := unpackVariationPointIndices(s, currDataOffset)
		if err != nil {
			return nil, err
		}
		t.SharedCVTIndices = sharedIndices
		currDataOffset = next
	}
	for i := range t.Tuples {
		t.Tuples[i].dataOffset = currDataOffset
		currDataOffset += int64(t.Tuples[i].VariationDataSize)
		if currDataOffset > s.Size() {
			return nil, errors.New("cvar tuple variation data truncated")
		}
	}
	return t, nil
}

func (t *CvarTable) GetCVTDeltas(cvtCount int, coords []float32) ([]float32, error) {
	if cvtCount < 0 {
		return nil, errors.New("cvt count cannot be negative")
	}
	deltas := make([]float32, cvtCount)
	if t == nil {
		return deltas, nil
	}

	for i := range t.Tuples {
		tuple := &t.Tuples[i]
		scalar := calculateTupleScalar(tuple.PeakTuple, tuple.IntermediateStart, tuple.IntermediateEnd, coords)
		if scalar == 0 {
			continue
		}

		pointIndices := t.SharedCVTIndices
		curr := tuple.dataOffset
		if tuple.TupleIndex&tuplePrivatePoints != 0 {
			privateIndices, next, err := unpackVariationPointIndices(t.Stream, curr)
			if err != nil {
				return nil, err
			}
			pointIndices = privateIndices
			curr = next
		}

		deltaCount := len(pointIndices)
		if pointIndices == nil {
			deltaCount = cvtCount
		}
		values, _, err := unpackVariationDeltaValues(t.Stream, curr, deltaCount, cvtCount)
		if err != nil {
			return nil, err
		}
		if pointIndices == nil {
			for i := 0; i < cvtCount; i++ {
				deltas[i] += float32(values[i]) * scalar
			}
			continue
		}
		for i, idx := range pointIndices {
			if idx >= 0 && idx < cvtCount {
				deltas[idx] += float32(values[i]) * scalar
			}
		}
	}
	return deltas, nil
}

func (t *CvarTable) ApplyCVTDeltas(cvt []int32, coords []float32) error {
	deltas, err := t.GetCVTDeltas(len(cvt), coords)
	if err != nil {
		return err
	}
	for i := range cvt {
		cvt[i] += roundVariationDelta(deltas[i])
	}
	return nil
}

// Item Variation Store

type VariationRegion struct {
	StartCoord float32
	PeakCoord  float32
	EndCoord   float32
}

type VariationRegionList struct {
	AxisCount int
	Regions   [][]VariationRegion
}

type VariationData struct {
	ItemCount       uint16
	ShortDeltaCount uint16
	RegionIndices   []uint16
	LongWords       bool
	Deltas          [][]int32
}

type ItemVariationStore struct {
	RegionList VariationRegionList
	Data       []VariationData
}

func (ivs *ItemVariationStore) GetDelta(outerIndex, innerIndex int, coords []float32) float32 {
	if ivs == nil || outerIndex < 0 || innerIndex < 0 {
		return 0
	}
	if outerIndex == 0xFFFF && innerIndex == 0xFFFF {
		return 0
	}
	if outerIndex >= len(ivs.Data) {
		return 0
	}
	data := &ivs.Data[outerIndex]
	if innerIndex >= int(data.ItemCount) {
		return 0
	}

	var totalDelta float32
	for i, regionIndex := range data.RegionIndices {
		if int(regionIndex) >= len(ivs.RegionList.Regions) {
			continue
		}
		region := ivs.RegionList.Regions[regionIndex]
		scalar := calculateScalar(region, coords)
		if scalar != 0 {
			totalDelta += float32(data.Deltas[innerIndex][i]) * scalar
		}
	}
	return totalDelta
}

func calculateScalar(region []VariationRegion, coords []float32) float32 {
	var scalar float32 = 1.0
	for i := 0; i < len(region); i++ {
		if i >= len(coords) {
			break
		}
		r := region[i]
		curr := coords[i]

		if r.PeakCoord == 0 || curr == r.PeakCoord {
			continue
		}
		if curr <= r.StartCoord || curr >= r.EndCoord {
			return 0
		}
		if curr < r.PeakCoord {
			scalar *= (curr - r.StartCoord) / (r.PeakCoord - r.StartCoord)
		} else {
			scalar *= (r.EndCoord - curr) / (r.EndCoord - r.PeakCoord)
		}
	}
	return scalar
}

func ParseItemVariationStore(s api.Stream, offset int64) (*ItemVariationStore, error) {
	if !hasRange(s, offset, 8) {
		return nil, errors.New("ItemVariationStore header truncated")
	}
	format, _ := readUint16(s, offset)
	if format != 1 {
		return nil, fmt.Errorf("unsupported ItemVariationStore format: %d", format)
	}
	regionListOffset, _ := readUint32(s, offset+2)
	dataCount, _ := readUint16(s, offset+6)
	if !hasRange(s, offset+8, int64(dataCount)*4) {
		return nil, errors.New("ItemVariationStore data offsets truncated")
	}
	dataOffsets := make([]uint32, dataCount)
	for i := 0; i < int(dataCount); i++ {
		dataOffsets[i], _ = readUint32(s, offset+8+int64(i*4))
	}

	ivs := &ItemVariationStore{
		Data: make([]VariationData, dataCount),
	}

	// Parse Region List
	rlOff := offset + int64(regionListOffset)
	if regionListOffset == 0 || !hasRange(s, rlOff, 4) {
		return nil, errors.New("ItemVariationStore region list offset out of range")
	}
	axisCount, _ := readUint16(s, rlOff)
	regionCount, _ := readUint16(s, rlOff+2)
	if regionCount&0x8000 != 0 {
		return nil, errors.New("ItemVariationStore region count reserved bit set")
	}
	regionListSize := int64(4) + int64(regionCount)*int64(axisCount)*6
	if !hasRange(s, rlOff, regionListSize) {
		return nil, errors.New("ItemVariationStore region list truncated")
	}
	ivs.RegionList.AxisCount = int(axisCount)
	ivs.RegionList.Regions = make([][]VariationRegion, regionCount)
	for i := 0; i < int(regionCount); i++ {
		ivs.RegionList.Regions[i] = make([]VariationRegion, axisCount)
		for j := 0; j < int(axisCount); j++ {
			off := rlOff + 4 + int64(i)*int64(axisCount)*6 + int64(j)*6
			v1, _ := readInt16(s, off)
			v2, _ := readInt16(s, off+2)
			v3, _ := readInt16(s, off+4)
			ivs.RegionList.Regions[i][j] = VariationRegion{
				StartCoord: float32(v1) / 16384.0,
				PeakCoord:  float32(v2) / 16384.0,
				EndCoord:   float32(v3) / 16384.0,
			}
			if err := validateVariationRegion(ivs.RegionList.Regions[i][j]); err != nil {
				return nil, fmt.Errorf("ItemVariationStore region %d axis %d: %w", i, j, err)
			}
		}
	}

	// Parse Variation Data
	for i := 0; i < int(dataCount); i++ {
		if dataOffsets[i] == 0 {
			continue
		}
		dOff := offset + int64(dataOffsets[i])
		if !hasRange(s, dOff, 6) {
			return nil, errors.New("ItemVariationData header truncated")
		}
		itemCount, _ := readUint16(s, dOff)
		wordDeltaCountField, _ := readUint16(s, dOff+2)
		regionIndexCount, _ := readUint16(s, dOff+4)
		longWords := wordDeltaCountField&0x8000 != 0
		wordDeltaCount := wordDeltaCountField & 0x7FFF
		if wordDeltaCount > regionIndexCount {
			return nil, errors.New("ItemVariationData word delta count exceeds region index count")
		}
		regionIndicesSize := int64(regionIndexCount) * 2
		if !hasRange(s, dOff+6, regionIndicesSize) {
			return nil, errors.New("ItemVariationData region indices truncated")
		}
		rowSize := itemVariationDeltaRowSize(regionIndexCount, wordDeltaCount, longWords)
		if !hasRange(s, dOff+6+regionIndicesSize, int64(itemCount)*rowSize) {
			return nil, errors.New("ItemVariationData delta sets truncated")
		}
		data := &ivs.Data[i]
		data.ItemCount = itemCount
		data.ShortDeltaCount = wordDeltaCount
		data.LongWords = longWords
		data.RegionIndices = make([]uint16, regionIndexCount)
		for j := 0; j < int(regionIndexCount); j++ {
			data.RegionIndices[j], _ = readUint16(s, dOff+6+int64(j*2))
			if data.RegionIndices[j] >= regionCount {
				return nil, errors.New("ItemVariationData region index out of range")
			}
		}

		deltasOff := dOff + 6 + int64(regionIndexCount*2)
		data.Deltas = make([][]int32, itemCount)
		for j := 0; j < int(itemCount); j++ {
			data.Deltas[j] = make([]int32, regionIndexCount)
			for k := 0; k < int(regionIndexCount); k++ {
				if longWords {
					if k < int(wordDeltaCount) {
						data.Deltas[j][k] = readInt32S(s, deltasOff)
						deltasOff += 4
					} else {
						v, _ := readInt16(s, deltasOff)
						data.Deltas[j][k] = int32(v)
						deltasOff += 2
					}
				} else {
					if k < int(wordDeltaCount) {
						v, _ := readInt16(s, deltasOff)
						data.Deltas[j][k] = int32(v)
						deltasOff += 2
					} else {
						v, _ := readByte(s, deltasOff)
						data.Deltas[j][k] = int32(int8(v))
						deltasOff++
					}
				}
			}
		}
	}

	return ivs, nil
}

type DeltaSetIndexMap struct {
	Indices []uint32 // (outerIndex << 16) | innerIndex
}

func ParseDeltaSetIndexMap(s api.Stream, offset int64) (*DeltaSetIndexMap, error) {
	if !hasRange(s, offset, 2) {
		return nil, errors.New("DeltaSetIndexMap header truncated")
	}
	format, _ := readByte(s, offset)
	entryFormatByte, _ := readByte(s, offset+1)
	entryFormat := uint16(entryFormatByte)
	var mapCount uint32
	var dataOff int64
	switch format {
	case 0:
		if !hasRange(s, offset, 4) {
			return nil, errors.New("DeltaSetIndexMap format 0 header truncated")
		}
		v, _ := readUint16(s, offset+2)
		mapCount = uint32(v)
		dataOff = offset + 4
	case 1:
		if !hasRange(s, offset, 6) {
			return nil, errors.New("DeltaSetIndexMap format 1 header truncated")
		}
		mapCount, _ = readUint32(s, offset+2)
		dataOff = offset + 6
	default:
		return nil, fmt.Errorf("unsupported DeltaSetIndexMap format: %d", format)
	}
	if entryFormat&0x00C0 != 0 {
		return nil, errors.New("DeltaSetIndexMap entry format reserved bits set")
	}
	if uint64(mapCount) > uint64(maxInt()) {
		return nil, errors.New("DeltaSetIndexMap map count too large")
	}

	innerIndexBitCnt := int((entryFormat & 0x000F) + 1)
	entrySize := int(((entryFormat & 0x0030) >> 4) + 1)
	if !hasRange(s, dataOff, int64(mapCount)*int64(entrySize)) {
		return nil, errors.New("DeltaSetIndexMap map data truncated")
	}

	m := &DeltaSetIndexMap{
		Indices: make([]uint32, mapCount),
	}
	for i := 0; i < int(mapCount); i++ {
		var entry uint32
		if entrySize == 1 {
			v, _ := readByte(s, dataOff)
			entry = uint32(v)
		} else if entrySize == 2 {
			v, _ := readUint16(s, dataOff)
			entry = uint32(v)
		} else if entrySize == 3 {
			v1, _ := readByte(s, dataOff)
			v2, _ := readByte(s, dataOff+1)
			v3, _ := readByte(s, dataOff+2)
			entry = uint32(v1)<<16 | uint32(v2)<<8 | uint32(v3)
		} else if entrySize == 4 {
			v, _ := readUint32(s, dataOff)
			entry = v
		}
		dataOff += int64(entrySize)

		outerIndex := entry >> innerIndexBitCnt
		innerIndex := entry & ((1 << innerIndexBitCnt) - 1)
		m.Indices[i] = (outerIndex << 16) | innerIndex
	}
	return m, nil
}

// HVAR Table

type HVARTable struct {
	MajorVersion        uint16
	MinorVersion        uint16
	ItemVariationStore  *ItemVariationStore
	AdvanceWidthMapping *DeltaSetIndexMap
	LsbMapping          *DeltaSetIndexMap
	RsbMapping          *DeltaSetIndexMap
}

func (t *HVARTable) GetAdvanceDelta(glyphIndex int, coords []float32) float32 {
	if t == nil || t.ItemVariationStore == nil {
		return 0
	}
	outer, inner, ok := metricDeltaSetIndex(glyphIndex, t.AdvanceWidthMapping, true)
	if !ok {
		return 0
	}
	return t.ItemVariationStore.GetDelta(outer, inner, coords)
}

func (t *HVARTable) GetLSBDelta(glyphIndex int, coords []float32) float32 {
	if t == nil || t.ItemVariationStore == nil || t.LsbMapping == nil {
		return 0
	}
	outer, inner, ok := metricDeltaSetIndex(glyphIndex, t.LsbMapping, false)
	if !ok {
		return 0
	}
	return t.ItemVariationStore.GetDelta(outer, inner, coords)
}

func ParseHVAR(s api.Stream) (*HVARTable, error) {
	if s.Size() < 20 {
		return nil, errors.New("HVAR table too short")
	}
	major, _ := readUint16(s, 0)
	minor, _ := readUint16(s, 2)
	if major != 1 || minor != 0 {
		return nil, fmt.Errorf("unsupported HVAR version: %d.%d", major, minor)
	}
	ivsOff, _ := readUint32(s, 4)
	advMapOff, _ := readUint32(s, 8)
	lsbMapOff, _ := readUint32(s, 12)
	rsbMapOff, _ := readUint32(s, 16)
	if ivsOff == 0 || int64(ivsOff) >= s.Size() {
		return nil, errors.New("HVAR item variation store offset out of range")
	}

	t := &HVARTable{
		MajorVersion: major,
		MinorVersion: minor,
	}
	var err error
	t.ItemVariationStore, err = ParseItemVariationStore(s, int64(ivsOff))
	if err != nil {
		return nil, err
	}
	if advMapOff != 0 {
		t.AdvanceWidthMapping, err = ParseDeltaSetIndexMap(s, int64(advMapOff))
		if err != nil {
			return nil, err
		}
	}
	if lsbMapOff != 0 {
		t.LsbMapping, err = ParseDeltaSetIndexMap(s, int64(lsbMapOff))
		if err != nil {
			return nil, err
		}
	}
	if rsbMapOff != 0 {
		t.RsbMapping, err = ParseDeltaSetIndexMap(s, int64(rsbMapOff))
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}

// VVAR Table

type VVARTable struct {
	MajorVersion         uint16
	MinorVersion         uint16
	ItemVariationStore   *ItemVariationStore
	AdvanceHeightMapping *DeltaSetIndexMap
	TsbMapping           *DeltaSetIndexMap
	BsbMapping           *DeltaSetIndexMap
	VOrgMapping          *DeltaSetIndexMap
}

func (t *VVARTable) GetAdvanceHeightDelta(glyphIndex int, coords []float32) float32 {
	if t == nil || t.ItemVariationStore == nil {
		return 0
	}
	outer, inner, ok := metricDeltaSetIndex(glyphIndex, t.AdvanceHeightMapping, true)
	if !ok {
		return 0
	}
	return t.ItemVariationStore.GetDelta(outer, inner, coords)
}

func (t *VVARTable) GetTSBDelta(glyphIndex int, coords []float32) float32 {
	if t == nil || t.ItemVariationStore == nil || t.TsbMapping == nil {
		return 0
	}
	outer, inner, ok := metricDeltaSetIndex(glyphIndex, t.TsbMapping, false)
	if !ok {
		return 0
	}
	return t.ItemVariationStore.GetDelta(outer, inner, coords)
}

func (t *VVARTable) GetBSBDelta(glyphIndex int, coords []float32) float32 {
	if t == nil || t.ItemVariationStore == nil || t.BsbMapping == nil {
		return 0
	}
	outer, inner, ok := metricDeltaSetIndex(glyphIndex, t.BsbMapping, false)
	if !ok {
		return 0
	}
	return t.ItemVariationStore.GetDelta(outer, inner, coords)
}

func (t *VVARTable) GetVOrgDelta(glyphIndex int, coords []float32) float32 {
	if t == nil || t.ItemVariationStore == nil || t.VOrgMapping == nil {
		return 0
	}
	outer, inner, ok := metricDeltaSetIndex(glyphIndex, t.VOrgMapping, false)
	if !ok {
		return 0
	}
	return t.ItemVariationStore.GetDelta(outer, inner, coords)
}

func ParseVVAR(s api.Stream) (*VVARTable, error) {
	if s.Size() < 24 {
		return nil, errors.New("VVAR table too short")
	}
	major, _ := readUint16(s, 0)
	minor, _ := readUint16(s, 2)
	if major != 1 || minor != 0 {
		return nil, fmt.Errorf("unsupported VVAR version: %d.%d", major, minor)
	}
	ivsOff, _ := readUint32(s, 4)
	advMapOff, _ := readUint32(s, 8)
	tsbMapOff, _ := readUint32(s, 12)
	bsbMapOff, _ := readUint32(s, 16)
	vorgMapOff, _ := readUint32(s, 20)
	if ivsOff == 0 || int64(ivsOff) >= s.Size() {
		return nil, errors.New("VVAR item variation store offset out of range")
	}

	t := &VVARTable{
		MajorVersion: major,
		MinorVersion: minor,
	}
	var err error
	t.ItemVariationStore, err = ParseItemVariationStore(s, int64(ivsOff))
	if err != nil {
		return nil, err
	}
	if advMapOff != 0 {
		t.AdvanceHeightMapping, err = ParseDeltaSetIndexMap(s, int64(advMapOff))
		if err != nil {
			return nil, err
		}
	}
	if tsbMapOff != 0 {
		t.TsbMapping, err = ParseDeltaSetIndexMap(s, int64(tsbMapOff))
		if err != nil {
			return nil, err
		}
	}
	if bsbMapOff != 0 {
		t.BsbMapping, err = ParseDeltaSetIndexMap(s, int64(bsbMapOff))
		if err != nil {
			return nil, err
		}
	}
	if vorgMapOff != 0 {
		t.VOrgMapping, err = ParseDeltaSetIndexMap(s, int64(vorgMapOff))
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}

// MVAR Table

type MVARValueRecord struct {
	ValueTag           uint32
	DeltaSetOuterIndex uint16
	DeltaSetInnerIndex uint16
}

type MVARTable struct {
	MajorVersion       uint16
	MinorVersion       uint16
	ValueRecords       []MVARValueRecord
	ItemVariationStore *ItemVariationStore
}

func ParseMVAR(s api.Stream) (*MVARTable, error) {
	if s.Size() < 12 {
		return nil, errors.New("MVAR table too short")
	}
	major, _ := readUint16(s, 0)
	minor, _ := readUint16(s, 2)
	if major != 1 || minor != 0 {
		return nil, fmt.Errorf("unsupported MVAR version: %d.%d", major, minor)
	}
	valueRecordSize, _ := readUint16(s, 6)
	valueRecordCount, _ := readUint16(s, 8)
	ivsOff, _ := readUint16(s, 10)
	if valueRecordSize == 0 {
		return nil, errors.New("MVAR value record size cannot be zero")
	}
	if valueRecordCount > 0 && valueRecordSize < 8 {
		return nil, errors.New("MVAR value record size too small")
	}
	if valueRecordCount > 0 && ivsOff == 0 {
		return nil, errors.New("MVAR item variation store offset missing")
	}
	recordsEnd := int64(12) + int64(valueRecordCount)*int64(valueRecordSize)
	if recordsEnd > s.Size() {
		return nil, errors.New("MVAR value records truncated")
	}

	t := &MVARTable{
		MajorVersion: major,
		MinorVersion: minor,
		ValueRecords: make([]MVARValueRecord, valueRecordCount),
	}
	for i := 0; i < int(valueRecordCount); i++ {
		off := int64(12) + int64(i)*int64(valueRecordSize)
		t.ValueRecords[i].ValueTag, _ = readUint32(s, off)
		t.ValueRecords[i].DeltaSetOuterIndex, _ = readUint16(s, off+4)
		t.ValueRecords[i].DeltaSetInnerIndex, _ = readUint16(s, off+6)
		if i > 0 && t.ValueRecords[i].ValueTag <= t.ValueRecords[i-1].ValueTag {
			return nil, errors.New("MVAR value records out of order")
		}
	}
	if ivsOff != 0 {
		if int64(ivsOff) >= s.Size() {
			return nil, errors.New("MVAR item variation store offset out of range")
		}
		var err error
		t.ItemVariationStore, err = ParseItemVariationStore(s, int64(ivsOff))
		if err != nil {
			return nil, err
		}
	}
	return t, nil
}

func (t *MVARTable) FindValueRecord(valueTag uint32) *MVARValueRecord {
	if t == nil {
		return nil
	}
	for i := range t.ValueRecords {
		if t.ValueRecords[i].ValueTag == valueTag {
			return &t.ValueRecords[i]
		}
	}
	return nil
}

func (t *MVARTable) GetDelta(valueTag uint32, coords []float32) float32 {
	if t == nil || t.ItemVariationStore == nil {
		return 0
	}
	record := t.FindValueRecord(valueTag)
	if record == nil {
		return 0
	}
	return t.ItemVariationStore.GetDelta(int(record.DeltaSetOuterIndex), int(record.DeltaSetInnerIndex), coords)
}

func (t *MVARTable) GetMetricDelta(valueTag uint32, coords []float32) int32 {
	return roundVariationDelta(t.GetDelta(valueTag, coords))
}

func (t *MVARTable) ApplyMetricDelta(valueTag uint32, value int32, coords []float32) int32 {
	return value + t.GetMetricDelta(valueTag, coords)
}

// Helpers

func metricDeltaSetIndex(glyphIndex int, m *DeltaSetIndexMap, implicit bool) (int, int, bool) {
	if glyphIndex < 0 {
		return 0, 0, false
	}
	var index uint32
	if m == nil {
		if !implicit {
			return 0, 0, false
		}
		index = uint32(glyphIndex)
	} else {
		if len(m.Indices) == 0 {
			return 0, 0, false
		}
		if glyphIndex >= len(m.Indices) {
			index = m.Indices[len(m.Indices)-1]
		} else {
			index = m.Indices[glyphIndex]
		}
	}
	return int(index >> 16), int(index & 0xFFFF), true
}

func calculateTupleScalar(peak, start, end []float32, coords []float32) float32 {
	if peak == nil {
		return 1.0
	}
	var scalar float32 = 1.0
	for i := 0; i < len(peak); i++ {
		if i >= len(coords) {
			break
		}
		p := peak[i]
		curr := coords[i]
		var s, e float32
		if start != nil {
			if i >= len(start) || i >= len(end) {
				break
			}
			s = start[i]
			e = end[i]
		} else if p > 0 {
			s = 0
			e = p
		} else if p < 0 {
			s = p
			e = 0
		} else {
			continue
		}

		if p == 0 || curr == p {
			continue
		}
		if curr <= s || curr >= e {
			return 0
		}
		if curr < p {
			scalar *= (curr - s) / (p - s)
		} else {
			scalar *= (e - curr) / (e - p)
		}
	}
	return scalar
}

func readF2Dot14Tuple(s api.Stream, offset int64, axisCount int) ([]float32, int64, error) {
	if axisCount < 0 {
		return nil, 0, errors.New("axis count cannot be negative")
	}
	if offset+int64(axisCount)*2 > s.Size() {
		return nil, 0, errors.New("F2DOT14 tuple truncated")
	}
	tuple := make([]float32, axisCount)
	for i := 0; i < axisCount; i++ {
		v, _ := readInt16(s, offset)
		tuple[i] = f2Dot14ToFloat(v)
		offset += 2
	}
	return tuple, offset, nil
}

func hasRange(s api.Stream, offset, length int64) bool {
	if offset < 0 || length < 0 {
		return false
	}
	size := s.Size()
	return offset <= size && length <= size-offset
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func validateVariationRegion(region VariationRegion) error {
	if region.StartCoord < -1 || region.StartCoord > 1 || region.PeakCoord < -1 || region.PeakCoord > 1 || region.EndCoord < -1 || region.EndCoord > 1 {
		return errors.New("coordinates out of normalized range")
	}
	if region.StartCoord > region.PeakCoord || region.PeakCoord > region.EndCoord {
		return errors.New("coordinates are not ordered")
	}
	if region.PeakCoord == 0 {
		if region.StartCoord > 0 || region.EndCoord < 0 {
			return errors.New("zero peak must span zero")
		}
		return nil
	}
	if (region.StartCoord < 0 || region.PeakCoord < 0 || region.EndCoord < 0) &&
		(region.StartCoord > 0 || region.PeakCoord > 0 || region.EndCoord > 0) {
		return errors.New("coordinates must be all non-positive or all non-negative")
	}
	return nil
}

func itemVariationDeltaRowSize(regionIndexCount, wordDeltaCount uint16, longWords bool) int64 {
	wordCount := int64(wordDeltaCount)
	shortCount := int64(regionIndexCount - wordDeltaCount)
	if longWords {
		return wordCount*4 + shortCount*2
	}
	return wordCount*2 + shortCount
}

func f2Dot14ToFloat(v int16) float32 {
	return float32(v) / 16384.0
}

func unpackVariationPointIndices(s api.Stream, offset int64) ([]int, int64, error) {
	if offset >= s.Size() {
		return nil, offset, errors.New("point number data truncated")
	}
	first, err := readByte(s, offset)
	if err != nil {
		return nil, offset, err
	}
	offset++
	var count int
	if first == 0 {
		return nil, offset, nil // All points or all CVTs.
	}
	if first < 128 {
		count = int(first)
	} else {
		if offset >= s.Size() {
			return nil, offset, errors.New("point number count truncated")
		}
		second, err := readByte(s, offset)
		if err != nil {
			return nil, offset, err
		}
		offset++
		count = (int(first&0x7F) << 8) | int(second)
	}

	indices := make([]int, count)
	var lastIndex int
	for i := 0; i < count; {
		if offset >= s.Size() {
			return nil, offset, errors.New("point number run truncated")
		}
		control, err := readByte(s, offset)
		if err != nil {
			return nil, offset, err
		}
		offset++
		n := int(control&0x7F) + 1
		if control&0x80 != 0 {
			for j := 0; j < n && i < count; j++ {
				if offset+2 > s.Size() {
					return nil, offset, errors.New("point number word run truncated")
				}
				v, _ := readUint16(s, offset)
				offset += 2
				lastIndex += int(v)
				indices[i] = lastIndex
				i++
			}
		} else {
			for j := 0; j < n && i < count; j++ {
				if offset >= s.Size() {
					return nil, offset, errors.New("point number byte run truncated")
				}
				v, err := readByte(s, offset)
				if err != nil {
					return nil, offset, err
				}
				offset++
				lastIndex += int(v)
				indices[i] = lastIndex
				i++
			}
		}
	}
	return indices, offset, nil
}

func unpackVariationDeltaValues(s api.Stream, offset int64, count int, numPoints int) ([]int16, int64, error) {
	if count == 0 {
		count = numPoints
	}
	if count < 0 {
		return nil, offset, errors.New("delta count cannot be negative")
	}
	values := make([]int16, count)
	for i := 0; i < count; {
		if offset >= s.Size() {
			return nil, offset, errors.New("delta run truncated")
		}
		control, err := readByte(s, offset)
		if err != nil {
			return nil, offset, err
		}
		offset++
		n := int(control&0x3F) + 1
		if control&0x80 != 0 {
			for j := 0; j < n && i < count; j++ {
				values[i] = 0
				i++
			}
		} else if control&0x40 != 0 {
			for j := 0; j < n && i < count; j++ {
				if offset+2 > s.Size() {
					return nil, offset, errors.New("delta word run truncated")
				}
				v, _ := readInt16(s, offset)
				offset += 2
				values[i] = v
				i++
			}
		} else {
			for j := 0; j < n && i < count; j++ {
				if offset >= s.Size() {
					return nil, offset, errors.New("delta byte run truncated")
				}
				v, err := readByte(s, offset)
				if err != nil {
					return nil, offset, err
				}
				offset++
				values[i] = int16(int8(v))
				i++
			}
		}
	}
	return values, offset, nil
}

func roundVariationDelta(delta float32) int32 {
	f := float64(delta)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	if delta >= 0 {
		return int32(delta + 0.5)
	}
	return int32(delta - 0.5)
}

func readUint16(s api.Stream, off int64) (uint16, error) {
	var buf [2]byte
	_, err := s.ReadAt(buf[:], off)
	return binary.BigEndian.Uint16(buf[:]), err
}

func readUint32(s api.Stream, off int64) (uint32, error) {
	var buf [4]byte
	_, err := s.ReadAt(buf[:], off)
	return binary.BigEndian.Uint32(buf[:]), err
}

func readInt16(s api.Stream, off int64) (int16, error) {
	v, err := readUint16(s, off)
	return int16(v), err
}

func readInt32S(s api.Stream, off int64) int32 {
	v, _ := readUint32(s, off)
	return int32(v)
}

func readByte(s api.Stream, off int64) (byte, error) {
	var buf [1]byte
	_, err := s.ReadAt(buf[:], off)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}
