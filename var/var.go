package ftvar

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/dh-kam/freetype-go/api"
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
	Gvar         *GvarTable
	Hvar         *HVARTable
	Vvar         *VVARTable
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

func (ve *VariationEngine) SetNormalizedCoordinates(coords []float32) {
	copy(ve.Coords, coords)
}

func (ve *VariationEngine) SetDesignCoordinates(coords []Fixed) {
	for i, axis := range ve.Axes {
		if i >= len(coords) {
			break
		}
		ve.Coords[i] = ve.normalize(coords[i], axis)
	}
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

func (ve *VariationEngine) ApplyVariation(glyphIndex int, outline api.Outline) error {
	if ve.Gvar == nil {
		return nil
	}
	if glyphIndex < 0 || glyphIndex >= len(ve.Gvar.GlyphDataOffsets)-1 {
		return fmt.Errorf("glyph index %d out of range", glyphIndex)
	}

	startOffset := ve.Gvar.GlyphDataOffsets[glyphIndex]
	endOffset := ve.Gvar.GlyphDataOffsets[glyphIndex+1]
	if startOffset == endOffset {
		return nil // No variation for this glyph
	}

	s := ve.Gvar.Stream
	points := outline.GetPoints()
	numPoints := len(points)

	tupleVariationCount, _ := readUint16(s, int64(startOffset))
	dataOffset, _ := readUint16(s, int64(startOffset)+2)

	count := tupleVariationCount & 0x0FFF
	sharedDataOffset := int64(startOffset) + int64(dataOffset)

	currHeaderOffset := int64(startOffset) + 4
	currDataOffset := sharedDataOffset

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

		// Unpack deltas
		tupleDeltasX, tupleDeltasY, nextDataOffset, err := ve.unpackDeltas(s, currDataOffset, numPoints, tupleIndex)
		if err != nil {
			return err
		}
		currDataOffset = nextDataOffset

		for j := 0; j < numPoints; j++ {
			deltasX[j] += tupleDeltasX[j] * scalar
			deltasY[j] += tupleDeltasY[j] * scalar
		}
	}

	// Apply deltas to outline
	for i := range points {
		points[i].X += int32(deltasX[i] * 64) // Convert back to 26.6
		points[i].Y += int32(deltasY[i] * 64)
	}

	return nil
}

func (ve *VariationEngine) calculateScalar(peak, start, end []float32) float32 {
	if peak == nil {
		return 1.0
	}
	var scalar float32 = 1.0
	for i := 0; i < len(ve.Coords); i++ {
		p := peak[i]
		curr := ve.Coords[i]
		var s, e float32
		if start != nil {
			s = start[i]
			e = end[i]
		} else {
			if p == 0 {
				continue
			} else if p > 0 {
				s = 0
				e = p
			} else {
				s = p
				e = 0
			}
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

func (ve *VariationEngine) unpackDeltas(s api.Stream, offset int64, numPoints int, tupleIndex uint16) ([]float32, []float32, int64, error) {
	dx := make([]float32, numPoints)
	dy := make([]float32, numPoints)

	curr := offset
	var pointIndices []int

	if tupleIndex&0x2000 != 0 { // PRIVATE_POINT_NUMBERS
		count, next, err := ve.unpackPointIndices(s, curr, numPoints)
		if err != nil {
			return nil, nil, 0, err
		}
		pointIndices = count
		curr = next
	}

	// Unpack X deltas
	dxValues, next, err := ve.unpackDeltaValues(s, curr, len(pointIndices), numPoints)
	if err != nil {
		return nil, nil, 0, err
	}
	curr = next

	// Unpack Y deltas
	dyValues, next, err := ve.unpackDeltaValues(s, curr, len(pointIndices), numPoints)
	if err != nil {
		return nil, nil, 0, err
	}
	curr = next

	if pointIndices == nil {
		for i := 0; i < numPoints; i++ {
			dx[i] = float32(dxValues[i])
			dy[i] = float32(dyValues[i])
		}
	} else {
		for i, idx := range pointIndices {
			if idx < numPoints {
				dx[idx] = float32(dxValues[i])
				dy[idx] = float32(dyValues[i])
			}
		}
	}

	return dx, dy, curr, nil
}

func (ve *VariationEngine) unpackPointIndices(s api.Stream, offset int64, totalPoints int) ([]int, int64, error) {
	first, _ := readByte(s, offset)
	offset++
	var count int
	if first == 0 {
		return nil, offset, nil // All points
	}
	if first < 128 {
		count = int(first)
	} else {
		second, _ := readByte(s, offset)
		offset++
		count = (int(first&0x7F) << 8) | int(second)
	}

	indices := make([]int, count)
	var lastIndex int
	for i := 0; i < count; {
		control, _ := readByte(s, offset)
		offset++
		n := int(control&0x7F) + 1
		if control&0x80 != 0 { // Words
			for j := 0; j < n && i < count; j++ {
				v, _ := readUint16(s, offset)
				offset += 2
				lastIndex += int(v)
				indices[i] = lastIndex
				i++
			}
		} else { // Bytes
			for j := 0; j < n && i < count; j++ {
				v, _ := readByte(s, offset)
				offset++
				lastIndex += int(v)
				indices[i] = lastIndex
				i++
			}
		}
	}
	return indices, offset, nil
}

func (ve *VariationEngine) unpackDeltaValues(s api.Stream, offset int64, count int, numPoints int) ([]int16, int64, error) {
	if count == 0 {
		count = numPoints
	}
	values := make([]int16, count)
	for i := 0; i < count; {
		control, _ := readByte(s, offset)
		offset++
		n := int(control&0x3F) + 1
		if control&0x80 != 0 { // Deltas are zero
			for j := 0; j < n && i < count; j++ {
				values[i] = 0
				i++
			}
		} else if control&0x40 != 0 { // Words
			for j := 0; j < n && i < count; j++ {
				v, _ := readInt16(s, offset)
				offset += 2
				values[i] = v
				i++
			}
		} else { // Bytes
			for j := 0; j < n && i < count; j++ {
				v, _ := readByte(s, offset)
				offset++
				values[i] = int16(int8(v))
				i++
			}
		}
	}
	return values, offset, nil
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
	// instanceSize, _ := readUint16(s, 14)

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
	Deltas          [][]int16
}

type ItemVariationStore struct {
	RegionList VariationRegionList
	Data       []VariationData
}

func (ivs *ItemVariationStore) GetDelta(outerIndex, innerIndex int, coords []float32) float32 {
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
	format, _ := readUint16(s, offset)
	if format != 1 {
		return nil, fmt.Errorf("unsupported ItemVariationStore format: %d", format)
	}
	regionListOffset, _ := readUint32(s, offset+2)
	dataCount, _ := readUint16(s, offset+6)
	dataOffsets := make([]uint32, dataCount)
	for i := 0; i < int(dataCount); i++ {
		dataOffsets[i], _ = readUint32(s, offset+8+int64(i*4))
	}

	ivs := &ItemVariationStore{
		Data: make([]VariationData, dataCount),
	}

	// Parse Region List
	rlOff := offset + int64(regionListOffset)
	axisCount, _ := readUint16(s, rlOff)
	regionCount, _ := readUint16(s, rlOff+2)
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
		}
	}

	// Parse Variation Data
	for i := 0; i < int(dataCount); i++ {
		dOff := offset + int64(dataOffsets[i])
		itemCount, _ := readUint16(s, dOff)
		shortDeltaCount, _ := readUint16(s, dOff+2)
		regionIndexCount, _ := readUint16(s, dOff+4)
		data := &ivs.Data[i]
		data.ItemCount = itemCount
		data.ShortDeltaCount = shortDeltaCount
		data.RegionIndices = make([]uint16, regionIndexCount)
		for j := 0; j < int(regionIndexCount); j++ {
			data.RegionIndices[j], _ = readUint16(s, dOff+6+int64(j*2))
		}

		deltasOff := dOff + 6 + int64(regionIndexCount*2)
		data.Deltas = make([][]int16, itemCount)
		for j := 0; j < int(itemCount); j++ {
			data.Deltas[j] = make([]int16, regionIndexCount)
			for k := 0; k < int(regionIndexCount); k++ {
				if k < int(shortDeltaCount) {
					v, _ := readInt16(s, deltasOff)
					data.Deltas[j][k] = v
					deltasOff += 2
				} else {
					v, _ := readByte(s, deltasOff)
					data.Deltas[j][k] = int16(int8(v))
					deltasOff++
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
	format, _ := readUint16(s, offset)
	if format != 0 {
		// Only format 0 is common for HVAR/VVAR
	}
	entryFormat, _ := readUint16(s, offset+2)
	mapCount, _ := readUint16(s, offset+4)

	innerIndexBitCnt := int((entryFormat & 0x000F) + 1)
	entrySize := int(((entryFormat & 0x0030) >> 4) + 1)

	m := &DeltaSetIndexMap{
		Indices: make([]uint32, mapCount),
	}
	dataOff := offset + 6
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
	outer, inner := t.getIndices(glyphIndex, t.AdvanceWidthMapping)
	return t.ItemVariationStore.GetDelta(outer, inner, coords)
}

func (t *HVARTable) GetLSBDelta(glyphIndex int, coords []float32) float32 {
	outer, inner := t.getIndices(glyphIndex, t.LsbMapping)
	return t.ItemVariationStore.GetDelta(outer, inner, coords)
}

func (t *HVARTable) getIndices(glyphIndex int, m *DeltaSetIndexMap) (int, int) {
	var index uint32
	if m != nil {
		if glyphIndex >= len(m.Indices) {
			index = m.Indices[len(m.Indices)-1]
		} else {
			index = m.Indices[glyphIndex]
		}
	} else {
		index = uint32(glyphIndex) // Default: outer=0, inner=glyphIndex
	}
	return int(index >> 16), int(index & 0xFFFF)
}

func ParseHVAR(s api.Stream) (*HVARTable, error) {
	if s.Size() < 20 {
		return nil, errors.New("HVAR table too short")
	}
	major, _ := readUint16(s, 0)
	minor, _ := readUint16(s, 2)
	ivsOff, _ := readUint32(s, 4)
	advMapOff, _ := readUint32(s, 8)
	lsbMapOff, _ := readUint32(s, 12)
	rsbMapOff, _ := readUint32(s, 16)

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
		t.AdvanceWidthMapping, _ = ParseDeltaSetIndexMap(s, int64(advMapOff))
	}
	if lsbMapOff != 0 {
		t.LsbMapping, _ = ParseDeltaSetIndexMap(s, int64(lsbMapOff))
	}
	if rsbMapOff != 0 {
		t.RsbMapping, _ = ParseDeltaSetIndexMap(s, int64(rsbMapOff))
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

func ParseVVAR(s api.Stream) (*VVARTable, error) {
	if s.Size() < 24 {
		return nil, errors.New("VVAR table too short")
	}
	major, _ := readUint16(s, 0)
	minor, _ := readUint16(s, 2)
	ivsOff, _ := readUint32(s, 4)
	advMapOff, _ := readUint32(s, 8)
	tsbMapOff, _ := readUint32(s, 12)
	bsbMapOff, _ := readUint32(s, 16)
	vorgMapOff, _ := readUint32(s, 20)

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
		t.AdvanceHeightMapping, _ = ParseDeltaSetIndexMap(s, int64(advMapOff))
	}
	if tsbMapOff != 0 {
		t.TsbMapping, _ = ParseDeltaSetIndexMap(s, int64(tsbMapOff))
	}
	if bsbMapOff != 0 {
		t.BsbMapping, _ = ParseDeltaSetIndexMap(s, int64(bsbMapOff))
	}
	if vorgMapOff != 0 {
		t.VOrgMapping, _ = ParseDeltaSetIndexMap(s, int64(vorgMapOff))
	}
	return t, nil
}

// Helpers

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
