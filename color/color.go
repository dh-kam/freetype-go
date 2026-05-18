package color

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/dh-kam/freetype-go/api"
)

const (
	maxPaintDepth    = 32
	maxCompositeMode = 27

	// NoVariationIndex disables variation for COLR varIndexBase fields.
	NoVariationIndex = uint32(0xFFFFFFFF)
)

// Fixed1616 represents a 16.16 fixed-point value
type Fixed1616 float32

// Fixed248 represents a 24.8 fixed-point value
type Fixed248 float32

// F2Dot14 represents a 2.14 fixed-point value
type F2Dot14 float32

func ReadFixed1616(b []byte) Fixed1616 {
	v := int32(binary.BigEndian.Uint32(b))
	return Fixed1616(float32(v) / 65536.0)
}

func ReadFixed248(b []byte) Fixed248 {
	v := int32(binary.BigEndian.Uint32(b))
	return Fixed248(float32(v) / 256.0)
}

func ReadF2Dot14(b []byte) F2Dot14 {
	v := int16(binary.BigEndian.Uint16(b))
	return F2Dot14(float32(v) / 16384.0)
}

func readOffset24(b []byte) uint32 {
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}

func streamHasBytes(s api.Stream, offset int64, length int64) bool {
	if offset < 0 || length < 0 {
		return false
	}
	size := s.Size()
	if offset > size {
		return false
	}
	return length <= size-offset
}

func readAtExact(s api.Stream, p []byte, offset int64) error {
	if !streamHasBytes(s, offset, int64(len(p))) {
		return io.ErrUnexpectedEOF
	}
	n, err := s.ReadAt(p, offset)
	if err != nil {
		return err
	}
	if n != len(p) {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func readUint16At(s api.Stream, offset int64) (uint16, error) {
	var buf [2]byte
	if err := readAtExact(s, buf[:], offset); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

func readUint32At(s api.Stream, offset int64) (uint32, error) {
	var buf [4]byte
	if err := readAtExact(s, buf[:], offset); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}

func readInt16At(s api.Stream, offset int64) (int16, error) {
	v, err := readUint16At(s, offset)
	return int16(v), err
}

func readInt32At(s api.Stream, offset int64) (int32, error) {
	v, err := readUint32At(s, offset)
	return int32(v), err
}

// RGBA represents a color in RGBA format.
type RGBA struct {
	R, G, B, A uint8
}

// Layer represents a color layer for a glyph.
type Layer struct {
	GlyphID      uint16
	Color        RGBA
	PaletteIndex uint16
}

// CPAL represents the 'CPAL' color palette table.
type CPAL struct {
	Version            uint16
	NumPaletteEntries  uint16
	NumPalettes        uint16
	NumColorRecords    uint16
	ColorRecords       []RGBA
	PaletteOffsets     []uint16
	PaletteTypes       []uint32
	PaletteLabels      []uint16
	PaletteEntryLabels []uint16

	// Version 1 fields
	PaletteTypesArrayOffset       uint32
	PaletteLabelsArrayOffset      uint32
	PaletteEntryLabelsArrayOffset uint32
}

func ParseCPAL(s api.Stream) (*CPAL, error) {
	if s.Size() < 12 {
		return nil, errors.New("CPAL table too short")
	}

	var buf [12]byte
	if err := readAtExact(s, buf[:], 0); err != nil {
		return nil, err
	}

	version := binary.BigEndian.Uint16(buf[0:2])
	numPaletteEntries := binary.BigEndian.Uint16(buf[2:4])
	numPalettes := binary.BigEndian.Uint16(buf[4:6])
	numColorRecords := binary.BigEndian.Uint16(buf[6:8])
	colorRecordsOffset := binary.BigEndian.Uint32(buf[8:12])

	if !streamHasBytes(s, int64(colorRecordsOffset), int64(numColorRecords)*4) {
		return nil, errors.New("CPAL table too short for color records")
	}
	if !streamHasBytes(s, 12, int64(numPalettes)*2) {
		return nil, errors.New("CPAL table too short for palette offsets")
	}

	paletteOffsetsEnd := int64(12) + int64(numPalettes)*2
	cpal := &CPAL{
		Version:           version,
		NumPaletteEntries: numPaletteEntries,
		NumPalettes:       numPalettes,
		NumColorRecords:   numColorRecords,
		ColorRecords:      make([]RGBA, int(numColorRecords)),
		PaletteOffsets:    make([]uint16, int(numPalettes)),
	}

	// Read palette offsets
	for i := 0; i < int(numPalettes); i++ {
		var offBuf [2]byte
		if err := readAtExact(s, offBuf[:], 12+int64(i*2)); err != nil {
			return nil, err
		}
		cpal.PaletteOffsets[i] = binary.BigEndian.Uint16(offBuf[:])
	}

	if version >= 1 {
		if !streamHasBytes(s, paletteOffsetsEnd, 12) {
			return nil, errors.New("CPAL table version 1 too short")
		}
		var bufV1 [12]byte
		if err := readAtExact(s, bufV1[:], paletteOffsetsEnd); err != nil {
			return nil, err
		}
		cpal.PaletteTypesArrayOffset = binary.BigEndian.Uint32(bufV1[0:4])
		cpal.PaletteLabelsArrayOffset = binary.BigEndian.Uint32(bufV1[4:8])
		cpal.PaletteEntryLabelsArrayOffset = binary.BigEndian.Uint32(bufV1[8:12])

		if cpal.PaletteTypesArrayOffset != 0 {
			if !streamHasBytes(s, int64(cpal.PaletteTypesArrayOffset), int64(numPalettes)*4) {
				return nil, errors.New("CPAL palette types array out of bounds")
			}
			cpal.PaletteTypes = make([]uint32, int(numPalettes))
			for i := 0; i < int(numPalettes); i++ {
				var typeBuf [4]byte
				if err := readAtExact(s, typeBuf[:], int64(cpal.PaletteTypesArrayOffset)+int64(i)*4); err != nil {
					return nil, err
				}
				cpal.PaletteTypes[i] = binary.BigEndian.Uint32(typeBuf[:])
			}
		}

		if cpal.PaletteLabelsArrayOffset != 0 {
			if !streamHasBytes(s, int64(cpal.PaletteLabelsArrayOffset), int64(numPalettes)*2) {
				return nil, errors.New("CPAL palette labels array out of bounds")
			}
			cpal.PaletteLabels = make([]uint16, int(numPalettes))
			for i := 0; i < int(numPalettes); i++ {
				var labelBuf [2]byte
				if err := readAtExact(s, labelBuf[:], int64(cpal.PaletteLabelsArrayOffset)+int64(i)*2); err != nil {
					return nil, err
				}
				cpal.PaletteLabels[i] = binary.BigEndian.Uint16(labelBuf[:])
			}
		}

		if cpal.PaletteEntryLabelsArrayOffset != 0 {
			if !streamHasBytes(s, int64(cpal.PaletteEntryLabelsArrayOffset), int64(numPaletteEntries)*2) {
				return nil, errors.New("CPAL palette entry labels array out of bounds")
			}
			cpal.PaletteEntryLabels = make([]uint16, int(numPaletteEntries))
			for i := 0; i < int(numPaletteEntries); i++ {
				var labelBuf [2]byte
				if err := readAtExact(s, labelBuf[:], int64(cpal.PaletteEntryLabelsArrayOffset)+int64(i)*2); err != nil {
					return nil, err
				}
				cpal.PaletteEntryLabels[i] = binary.BigEndian.Uint16(labelBuf[:])
			}
		}
	}

	if numPaletteEntries > 0 {
		for _, offset := range cpal.PaletteOffsets {
			end := uint32(offset) + uint32(numPaletteEntries)
			if end > uint32(numColorRecords) {
				return nil, errors.New("CPAL palette offset out of bounds")
			}
		}
	}

	// Read color records (BGRA)
	for i := 0; i < int(numColorRecords); i++ {
		var cBuf [4]byte
		if err := readAtExact(s, cBuf[:], int64(colorRecordsOffset)+int64(i*4)); err != nil {
			return nil, err
		}
		cpal.ColorRecords[i] = RGBA{
			B: cBuf[0],
			G: cBuf[1],
			R: cBuf[2],
			A: cBuf[3],
		}
	}

	return cpal, nil
}

func (c *CPAL) Color(paletteIndex int, paletteEntryIndex uint16) (RGBA, bool) {
	if c == nil || paletteIndex < 0 || paletteIndex >= int(c.NumPalettes) || paletteIndex >= len(c.PaletteOffsets) {
		return RGBA{}, false
	}
	if paletteEntryIndex == 0xFFFF || int(paletteEntryIndex) >= int(c.NumPaletteEntries) {
		return RGBA{}, false
	}

	colorIndex := int(c.PaletteOffsets[paletteIndex]) + int(paletteEntryIndex)
	if colorIndex < 0 || colorIndex >= len(c.ColorRecords) {
		return RGBA{}, false
	}
	return c.ColorRecords[colorIndex], true
}

func (c *CPAL) Palette(paletteIndex int) ([]RGBA, bool) {
	if c == nil || paletteIndex < 0 || paletteIndex >= int(c.NumPalettes) || paletteIndex >= len(c.PaletteOffsets) {
		return nil, false
	}
	start := int(c.PaletteOffsets[paletteIndex])
	end := start + int(c.NumPaletteEntries)
	if start < 0 || end > len(c.ColorRecords) {
		return nil, false
	}
	palette := make([]RGBA, int(c.NumPaletteEntries))
	copy(palette, c.ColorRecords[start:end])
	return palette, true
}

func (c *CPAL) PaletteType(paletteIndex int) (uint32, bool) {
	if c == nil || paletteIndex < 0 || paletteIndex >= len(c.PaletteTypes) {
		return 0, false
	}
	return c.PaletteTypes[paletteIndex], true
}

func (c *CPAL) PaletteLabel(paletteIndex int) (uint16, bool) {
	if c == nil || paletteIndex < 0 || paletteIndex >= len(c.PaletteLabels) {
		return 0, false
	}
	return c.PaletteLabels[paletteIndex], true
}

func (c *CPAL) PaletteEntryLabel(paletteEntryIndex uint16) (uint16, bool) {
	if c == nil || int(paletteEntryIndex) >= len(c.PaletteEntryLabels) {
		return 0, false
	}
	return c.PaletteEntryLabels[paletteEntryIndex], true
}

// COLR represents the 'COLR' color table.
type COLR struct {
	Version                  uint16
	BaseGlyphRecords         map[uint16]BaseGlyphRecord
	LayerRecords             []LayerRecord
	BaseGlyphV1Records       map[uint16]BaseGlyphV1Record
	LayerList                []Paint
	ClipList                 *ClipList
	VarIndexMapOffset        uint32
	VarIndexMap              *DeltaSetIndexMap
	ItemVariationStoreOffset uint32
	ItemVariationStore       *ItemVariationStore
}

// BaseGlyphRecord represents a record for a base glyph in COLR table (v0).
type BaseGlyphRecord struct {
	GlyphID         uint16
	FirstLayerIndex uint16
	NumLayers       uint16
}

// LayerRecord represents a record for a layer in COLR table (v0).
type LayerRecord struct {
	GlyphID      uint16
	PaletteIndex uint16
}

// BaseGlyphV1Record represents a record for a base glyph in COLR v1 table.
type BaseGlyphV1Record struct {
	GlyphID uint16
	Paint   Paint
}

type ClipList struct {
	Format  uint8
	Records []ClipRecord
}

type ClipRecord struct {
	StartGlyphID  uint16
	EndGlyphID    uint16
	ClipBoxOffset uint32
	ClipBox       *ClipBox
}

type ClipBox struct {
	Format       uint8
	XMin, YMin   int16
	XMax, YMax   int16
	VarIndexBase uint32
}

type ItemVariationStore struct {
	Format              uint16
	VariationRegionList VariationRegionList
	ItemVariationData   []ItemVariationData
}

type VariationRegionList struct {
	AxisCount uint16
	Regions   []VariationRegion
}

type VariationRegion struct {
	Axes []VariationRegionAxis
}

type VariationRegionAxis struct {
	StartCoord F2Dot14
	PeakCoord  F2Dot14
	EndCoord   F2Dot14
}

type ItemVariationData struct {
	ItemCount        uint16
	ShortDeltaCount  uint16
	WordDeltaCount   uint16
	LongWords        bool
	RegionIndexCount uint16
	RegionIndexes    []uint16
	DeltaSetSize     uint32
	Deltas           [][]int32
}

type DeltaSetIndex struct {
	OuterIndex uint32
	InnerIndex uint32
}

func (d DeltaSetIndex) NoVariation() bool {
	return d.OuterIndex == 0xFFFF && d.InnerIndex == 0xFFFF
}

type DeltaSetIndexMap struct {
	Format      uint8
	EntryFormat uint8
	MapCount    uint32
	Indices     []DeltaSetIndex
}

func (m *DeltaSetIndexMap) Lookup(index uint32) (DeltaSetIndex, bool) {
	if m == nil || len(m.Indices) == 0 {
		return DeltaSetIndex{}, false
	}
	if index >= uint32(len(m.Indices)) {
		index = uint32(len(m.Indices) - 1)
	}
	deltaSetIndex := m.Indices[index]
	if deltaSetIndex.NoVariation() {
		return DeltaSetIndex{}, false
	}
	return deltaSetIndex, true
}

func (c *COLR) DeltaSetIndex(varIndex uint32) (DeltaSetIndex, bool) {
	if c == nil || c.ItemVariationStore == nil {
		return DeltaSetIndex{}, false
	}
	if c.VarIndexMap != nil {
		return c.VarIndexMap.Lookup(varIndex)
	}

	deltaSetIndex := DeltaSetIndex{
		OuterIndex: uint32(varIndex >> 16),
		InnerIndex: uint32(varIndex & 0xFFFF),
	}
	if deltaSetIndex.NoVariation() {
		return DeltaSetIndex{}, false
	}
	return deltaSetIndex, true
}

func (c *COLR) VariationDelta(varIndex uint32, coords []float32) float32 {
	deltaSetIndex, ok := c.DeltaSetIndex(varIndex)
	if !ok {
		return 0
	}
	return c.ItemVariationStore.Delta(deltaSetIndex.OuterIndex, deltaSetIndex.InnerIndex, coords)
}

func (c *COLR) VarIndexDelta(varIndexBase uint32, sequence uint32, coords []float32) float32 {
	if varIndexBase == NoVariationIndex || sequence > NoVariationIndex-varIndexBase {
		return 0
	}
	return c.VariationDelta(varIndexBase+sequence, coords)
}

func (ivs *ItemVariationStore) Delta(outerIndex, innerIndex uint32, coords []float32) float32 {
	if ivs == nil || outerIndex >= uint32(len(ivs.ItemVariationData)) {
		return 0
	}
	data := &ivs.ItemVariationData[outerIndex]
	if innerIndex >= uint32(data.ItemCount) || innerIndex >= uint32(len(data.Deltas)) {
		return 0
	}

	row := data.Deltas[innerIndex]
	var total float32
	for i, regionIndex := range data.RegionIndexes {
		if i >= len(row) || int(regionIndex) >= len(ivs.VariationRegionList.Regions) {
			continue
		}
		scalar := variationRegionScalar(ivs.VariationRegionList.Regions[regionIndex], coords)
		if scalar != 0 {
			total += float32(row[i]) * scalar
		}
	}
	return total
}

func variationRegionScalar(region VariationRegion, coords []float32) float32 {
	scalar := float32(1)
	for i, axis := range region.Axes {
		coord := float32(0)
		if i < len(coords) {
			coord = coords[i]
		}
		start := float32(axis.StartCoord)
		peak := float32(axis.PeakCoord)
		end := float32(axis.EndCoord)

		if peak == 0 || coord == peak {
			continue
		}
		if coord <= start || coord >= end {
			return 0
		}
		if coord < peak {
			denom := peak - start
			if denom == 0 {
				return 0
			}
			scalar *= (coord - start) / denom
		} else {
			denom := end - peak
			if denom == 0 {
				return 0
			}
			scalar *= (end - coord) / denom
		}
	}
	return scalar
}

// Paint interface for COLR v1
type Paint interface {
	Format() uint8
}

type ColorStop struct {
	StopOffset   F2Dot14
	PaletteIndex uint16
	Alpha        F2Dot14
	VarIndexBase uint32
}

type ColorLine struct {
	Extend     uint8
	ColorStops []ColorStop
}

type Affine2x3 struct {
	XX, XY, YX, YY Fixed1616
	DX, DY         Fixed248
}

type PaintColrLayers struct {
	NumLayers       uint8
	FirstLayerIndex uint32
}

func (p *PaintColrLayers) Format() uint8 { return 1 }

type PaintSolid struct {
	PaletteIndex uint16
	Alpha        F2Dot14
}

func (p *PaintSolid) Format() uint8 { return 2 }

type PaintVarSolid struct {
	PaletteIndex uint16
	Alpha        F2Dot14
	VarIndexBase uint32
}

func (p *PaintVarSolid) Format() uint8 { return 3 }

type PaintLinearGradient struct {
	ColorLine              ColorLine
	X0, Y0, X1, Y1, X2, Y2 int16
}

func (p *PaintLinearGradient) Format() uint8 { return 4 }

type PaintVarLinearGradient struct {
	ColorLine              ColorLine
	X0, Y0, X1, Y1, X2, Y2 int16
	VarIndexBase           uint32
}

func (p *PaintVarLinearGradient) Format() uint8 { return 5 }

type PaintRadialGradient struct {
	ColorLine ColorLine
	X0, Y0    int16
	R0        uint16
	X1, Y1    int16
	R1        uint16
}

func (p *PaintRadialGradient) Format() uint8 { return 6 }

type PaintVarRadialGradient struct {
	ColorLine    ColorLine
	X0, Y0       int16
	R0           uint16
	X1, Y1       int16
	R1           uint16
	VarIndexBase uint32
}

func (p *PaintVarRadialGradient) Format() uint8 { return 7 }

type PaintSweepGradient struct {
	ColorLine            ColorLine
	CenterX, CenterY     int16
	StartAngle, EndAngle F2Dot14
}

func (p *PaintSweepGradient) Format() uint8 { return 8 }

type PaintVarSweepGradient struct {
	ColorLine            ColorLine
	CenterX, CenterY     int16
	StartAngle, EndAngle F2Dot14
	VarIndexBase         uint32
}

func (p *PaintVarSweepGradient) Format() uint8 { return 9 }

type PaintGlyph struct {
	Paint   Paint
	GlyphID uint16
}

func (p *PaintGlyph) Format() uint8 { return 10 }

type PaintColrGlyph struct {
	GlyphID uint16
}

func (p *PaintColrGlyph) Format() uint8 { return 11 }

type PaintTransform struct {
	Paint     Paint
	Transform Affine2x3
}

func (p *PaintTransform) Format() uint8 { return 12 }

type PaintVarTransform struct {
	Paint        Paint
	Transform    Affine2x3
	VarIndexBase uint32
}

func (p *PaintVarTransform) Format() uint8 { return 13 }

type PaintTranslate struct {
	Paint  Paint
	DX, DY int16
}

func (p *PaintTranslate) Format() uint8 { return 14 }

type PaintVarTranslate struct {
	Paint        Paint
	DX, DY       int16
	VarIndexBase uint32
}

func (p *PaintVarTranslate) Format() uint8 { return 15 }

type PaintScale struct {
	Paint          Paint
	ScaleX, ScaleY F2Dot14
}

func (p *PaintScale) Format() uint8 { return 16 }

type PaintVarScale struct {
	Paint          Paint
	ScaleX, ScaleY F2Dot14
	VarIndexBase   uint32
}

func (p *PaintVarScale) Format() uint8 { return 17 }

type PaintScaleAroundCenter struct {
	Paint            Paint
	ScaleX, ScaleY   F2Dot14
	CenterX, CenterY int16
}

func (p *PaintScaleAroundCenter) Format() uint8 { return 18 }

type PaintVarScaleAroundCenter struct {
	Paint            Paint
	ScaleX, ScaleY   F2Dot14
	CenterX, CenterY int16
	VarIndexBase     uint32
}

func (p *PaintVarScaleAroundCenter) Format() uint8 { return 19 }

type PaintScaleUniform struct {
	Paint Paint
	Scale F2Dot14
}

func (p *PaintScaleUniform) Format() uint8 { return 20 }

type PaintVarScaleUniform struct {
	Paint        Paint
	Scale        F2Dot14
	VarIndexBase uint32
}

func (p *PaintVarScaleUniform) Format() uint8 { return 21 }

type PaintScaleUniformAroundCenter struct {
	Paint            Paint
	Scale            F2Dot14
	CenterX, CenterY int16
}

func (p *PaintScaleUniformAroundCenter) Format() uint8 { return 22 }

type PaintVarScaleUniformAroundCenter struct {
	Paint            Paint
	Scale            F2Dot14
	CenterX, CenterY int16
	VarIndexBase     uint32
}

func (p *PaintVarScaleUniformAroundCenter) Format() uint8 { return 23 }

type PaintRotate struct {
	Paint Paint
	Angle F2Dot14
}

func (p *PaintRotate) Format() uint8 { return 24 }

type PaintVarRotate struct {
	Paint        Paint
	Angle        F2Dot14
	VarIndexBase uint32
}

func (p *PaintVarRotate) Format() uint8 { return 25 }

type PaintRotateAroundCenter struct {
	Paint            Paint
	Angle            F2Dot14
	CenterX, CenterY int16
}

func (p *PaintRotateAroundCenter) Format() uint8 { return 26 }

type PaintVarRotateAroundCenter struct {
	Paint            Paint
	Angle            F2Dot14
	CenterX, CenterY int16
	VarIndexBase     uint32
}

func (p *PaintVarRotateAroundCenter) Format() uint8 { return 27 }

type PaintSkew struct {
	Paint                  Paint
	XSkewAngle, YSkewAngle F2Dot14
}

func (p *PaintSkew) Format() uint8 { return 28 }

type PaintVarSkew struct {
	Paint                  Paint
	XSkewAngle, YSkewAngle F2Dot14
	VarIndexBase           uint32
}

func (p *PaintVarSkew) Format() uint8 { return 29 }

type PaintSkewAroundCenter struct {
	Paint                  Paint
	XSkewAngle, YSkewAngle F2Dot14
	CenterX, CenterY       int16
}

func (p *PaintSkewAroundCenter) Format() uint8 { return 30 }

type PaintVarSkewAroundCenter struct {
	Paint                  Paint
	XSkewAngle, YSkewAngle F2Dot14
	CenterX, CenterY       int16
	VarIndexBase           uint32
}

func (p *PaintVarSkewAroundCenter) Format() uint8 { return 31 }

type PaintComposite struct {
	SourcePaint   Paint
	CompositeMode uint8
	BackdropPaint Paint
}

func (p *PaintComposite) Format() uint8 { return 32 }

func readColorLine(s api.Stream, offset int64) (ColorLine, error) {
	return readColorLineAt(s, offset, false)
}

func readVarColorLine(s api.Stream, offset int64) (ColorLine, error) {
	return readColorLineAt(s, offset, true)
}

func readColorLineAt(s api.Stream, offset int64, variable bool) (ColorLine, error) {
	var buf [3]byte
	if err := readAtExact(s, buf[:], offset); err != nil {
		return ColorLine{}, err
	}
	extend := buf[0]
	numStops := binary.BigEndian.Uint16(buf[1:3])
	stopSize := 6
	if variable {
		stopSize = 10
	}
	if !streamHasBytes(s, offset+3, int64(numStops)*int64(stopSize)) {
		return ColorLine{}, io.ErrUnexpectedEOF
	}
	stops := make([]ColorStop, int(numStops))
	for i := 0; i < int(numStops); i++ {
		var sBuf [10]byte
		if err := readAtExact(s, sBuf[:stopSize], offset+3+int64(i)*int64(stopSize)); err != nil {
			return ColorLine{}, err
		}
		stops[i] = ColorStop{
			StopOffset:   ReadF2Dot14(sBuf[0:2]),
			PaletteIndex: binary.BigEndian.Uint16(sBuf[2:4]),
			Alpha:        ReadF2Dot14(sBuf[4:6]),
		}
		if variable {
			stops[i].VarIndexBase = binary.BigEndian.Uint32(sBuf[6:10])
		}
	}
	return ColorLine{Extend: extend, ColorStops: stops}, nil
}

func readAffine2x3(s api.Stream, offset int64) (Affine2x3, error) {
	var buf [24]byte
	if err := readAtExact(s, buf[:], offset); err != nil {
		return Affine2x3{}, err
	}
	return Affine2x3{
		XX: ReadFixed1616(buf[0:4]),
		XY: ReadFixed1616(buf[4:8]),
		YX: ReadFixed1616(buf[8:12]),
		YY: ReadFixed1616(buf[12:16]),
		DX: ReadFixed248(buf[16:20]),
		DY: ReadFixed248(buf[20:24]),
	}, nil
}

func readVarAffine2x3(s api.Stream, offset int64) (Affine2x3, uint32, error) {
	var buf [28]byte
	if err := readAtExact(s, buf[:], offset); err != nil {
		return Affine2x3{}, 0, err
	}
	return Affine2x3{
		XX: ReadFixed1616(buf[0:4]),
		XY: ReadFixed1616(buf[4:8]),
		YX: ReadFixed1616(buf[8:12]),
		YY: ReadFixed1616(buf[12:16]),
		DX: ReadFixed248(buf[16:20]),
		DY: ReadFixed248(buf[20:24]),
	}, binary.BigEndian.Uint32(buf[24:28]), nil
}

func readPaint(s api.Stream, baseOffset int64, paintOffset int64) (Paint, error) {
	return readPaintAt(s, baseOffset, paintOffset, 0)
}

func readPaintAt(s api.Stream, baseOffset int64, paintOffset int64, depth int) (Paint, error) {
	if depth > maxPaintDepth {
		return nil, errors.New("COLR paint graph too deep")
	}
	if paintOffset < 0 || baseOffset < 0 || paintOffset > s.Size()-baseOffset {
		return nil, io.ErrUnexpectedEOF
	}
	absOffset := baseOffset + paintOffset
	var fmtBuf [1]byte
	if err := readAtExact(s, fmtBuf[:], absOffset); err != nil {
		return nil, err
	}
	format := fmtBuf[0]

	switch format {
	case 1:
		var buf [5]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		return &PaintColrLayers{
			NumLayers:       buf[0],
			FirstLayerIndex: binary.BigEndian.Uint32(buf[1:5]),
		}, nil
	case 2:
		var buf [4]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		return &PaintSolid{
			PaletteIndex: binary.BigEndian.Uint16(buf[0:2]),
			Alpha:        ReadF2Dot14(buf[2:4]),
		}, nil
	case 3:
		var buf [8]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		return &PaintVarSolid{
			PaletteIndex: binary.BigEndian.Uint16(buf[0:2]),
			Alpha:        ReadF2Dot14(buf[2:4]),
			VarIndexBase: binary.BigEndian.Uint32(buf[4:8]),
		}, nil
	case 4:
		var buf [15]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		clOffset := readOffset24(buf[0:3])
		colorLine, err := readColorLine(s, absOffset+int64(clOffset))
		if err != nil {
			return nil, err
		}
		return &PaintLinearGradient{
			ColorLine: colorLine,
			X0:        int16(binary.BigEndian.Uint16(buf[3:5])),
			Y0:        int16(binary.BigEndian.Uint16(buf[5:7])),
			X1:        int16(binary.BigEndian.Uint16(buf[7:9])),
			Y1:        int16(binary.BigEndian.Uint16(buf[9:11])),
			X2:        int16(binary.BigEndian.Uint16(buf[11:13])),
			Y2:        int16(binary.BigEndian.Uint16(buf[13:15])),
		}, nil
	case 5:
		var buf [19]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		clOffset := readOffset24(buf[0:3])
		colorLine, err := readVarColorLine(s, absOffset+int64(clOffset))
		if err != nil {
			return nil, err
		}
		return &PaintVarLinearGradient{
			ColorLine:    colorLine,
			X0:           int16(binary.BigEndian.Uint16(buf[3:5])),
			Y0:           int16(binary.BigEndian.Uint16(buf[5:7])),
			X1:           int16(binary.BigEndian.Uint16(buf[7:9])),
			Y1:           int16(binary.BigEndian.Uint16(buf[9:11])),
			X2:           int16(binary.BigEndian.Uint16(buf[11:13])),
			Y2:           int16(binary.BigEndian.Uint16(buf[13:15])),
			VarIndexBase: binary.BigEndian.Uint32(buf[15:19]),
		}, nil
	case 6:
		var buf [15]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		clOffset := readOffset24(buf[0:3])
		colorLine, err := readColorLine(s, absOffset+int64(clOffset))
		if err != nil {
			return nil, err
		}
		return &PaintRadialGradient{
			ColorLine: colorLine,
			X0:        int16(binary.BigEndian.Uint16(buf[3:5])),
			Y0:        int16(binary.BigEndian.Uint16(buf[5:7])),
			R0:        binary.BigEndian.Uint16(buf[7:9]),
			X1:        int16(binary.BigEndian.Uint16(buf[9:11])),
			Y1:        int16(binary.BigEndian.Uint16(buf[11:13])),
			R1:        binary.BigEndian.Uint16(buf[13:15]),
		}, nil
	case 7:
		var buf [19]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		clOffset := readOffset24(buf[0:3])
		colorLine, err := readVarColorLine(s, absOffset+int64(clOffset))
		if err != nil {
			return nil, err
		}
		return &PaintVarRadialGradient{
			ColorLine:    colorLine,
			X0:           int16(binary.BigEndian.Uint16(buf[3:5])),
			Y0:           int16(binary.BigEndian.Uint16(buf[5:7])),
			R0:           binary.BigEndian.Uint16(buf[7:9]),
			X1:           int16(binary.BigEndian.Uint16(buf[9:11])),
			Y1:           int16(binary.BigEndian.Uint16(buf[11:13])),
			R1:           binary.BigEndian.Uint16(buf[13:15]),
			VarIndexBase: binary.BigEndian.Uint32(buf[15:19]),
		}, nil
	case 8:
		var buf [11]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		clOffset := readOffset24(buf[0:3])
		colorLine, err := readColorLine(s, absOffset+int64(clOffset))
		if err != nil {
			return nil, err
		}
		return &PaintSweepGradient{
			ColorLine:  colorLine,
			CenterX:    int16(binary.BigEndian.Uint16(buf[3:5])),
			CenterY:    int16(binary.BigEndian.Uint16(buf[5:7])),
			StartAngle: ReadF2Dot14(buf[7:9]),
			EndAngle:   ReadF2Dot14(buf[9:11]),
		}, nil
	case 9:
		var buf [15]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		clOffset := readOffset24(buf[0:3])
		colorLine, err := readVarColorLine(s, absOffset+int64(clOffset))
		if err != nil {
			return nil, err
		}
		return &PaintVarSweepGradient{
			ColorLine:    colorLine,
			CenterX:      int16(binary.BigEndian.Uint16(buf[3:5])),
			CenterY:      int16(binary.BigEndian.Uint16(buf[5:7])),
			StartAngle:   ReadF2Dot14(buf[7:9]),
			EndAngle:     ReadF2Dot14(buf[9:11]),
			VarIndexBase: binary.BigEndian.Uint32(buf[11:15]),
		}, nil
	case 10:
		var buf [5]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintGlyph{
			Paint:   paint,
			GlyphID: binary.BigEndian.Uint16(buf[3:5]),
		}, nil
	case 11:
		var buf [2]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		return &PaintColrGlyph{
			GlyphID: binary.BigEndian.Uint16(buf[0:2]),
		}, nil
	case 12:
		var buf [6]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		tOffset := readOffset24(buf[3:6])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		transform, err := readAffine2x3(s, absOffset+int64(tOffset))
		if err != nil {
			return nil, err
		}
		return &PaintTransform{
			Paint:     paint,
			Transform: transform,
		}, nil
	case 13:
		var buf [6]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		tOffset := readOffset24(buf[3:6])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		transform, varIndexBase, err := readVarAffine2x3(s, absOffset+int64(tOffset))
		if err != nil {
			return nil, err
		}
		return &PaintVarTransform{
			Paint:        paint,
			Transform:    transform,
			VarIndexBase: varIndexBase,
		}, nil
	case 14:
		var buf [7]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintTranslate{
			Paint: paint,
			DX:    int16(binary.BigEndian.Uint16(buf[3:5])),
			DY:    int16(binary.BigEndian.Uint16(buf[5:7])),
		}, nil
	case 15:
		var buf [11]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintVarTranslate{
			Paint:        paint,
			DX:           int16(binary.BigEndian.Uint16(buf[3:5])),
			DY:           int16(binary.BigEndian.Uint16(buf[5:7])),
			VarIndexBase: binary.BigEndian.Uint32(buf[7:11]),
		}, nil
	case 16:
		var buf [7]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScale{
			Paint:  paint,
			ScaleX: ReadF2Dot14(buf[3:5]),
			ScaleY: ReadF2Dot14(buf[5:7]),
		}, nil
	case 17:
		var buf [11]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintVarScale{
			Paint:        paint,
			ScaleX:       ReadF2Dot14(buf[3:5]),
			ScaleY:       ReadF2Dot14(buf[5:7]),
			VarIndexBase: binary.BigEndian.Uint32(buf[7:11]),
		}, nil
	case 18:
		var buf [11]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScaleAroundCenter{
			Paint:   paint,
			ScaleX:  ReadF2Dot14(buf[3:5]),
			ScaleY:  ReadF2Dot14(buf[5:7]),
			CenterX: int16(binary.BigEndian.Uint16(buf[7:9])),
			CenterY: int16(binary.BigEndian.Uint16(buf[9:11])),
		}, nil
	case 19:
		var buf [15]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintVarScaleAroundCenter{
			Paint:        paint,
			ScaleX:       ReadF2Dot14(buf[3:5]),
			ScaleY:       ReadF2Dot14(buf[5:7]),
			CenterX:      int16(binary.BigEndian.Uint16(buf[7:9])),
			CenterY:      int16(binary.BigEndian.Uint16(buf[9:11])),
			VarIndexBase: binary.BigEndian.Uint32(buf[11:15]),
		}, nil
	case 20:
		var buf [5]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScaleUniform{
			Paint: paint,
			Scale: ReadF2Dot14(buf[3:5]),
		}, nil
	case 21:
		var buf [9]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintVarScaleUniform{
			Paint:        paint,
			Scale:        ReadF2Dot14(buf[3:5]),
			VarIndexBase: binary.BigEndian.Uint32(buf[5:9]),
		}, nil
	case 22:
		var buf [9]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScaleUniformAroundCenter{
			Paint:   paint,
			Scale:   ReadF2Dot14(buf[3:5]),
			CenterX: int16(binary.BigEndian.Uint16(buf[5:7])),
			CenterY: int16(binary.BigEndian.Uint16(buf[7:9])),
		}, nil
	case 23:
		var buf [13]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintVarScaleUniformAroundCenter{
			Paint:        paint,
			Scale:        ReadF2Dot14(buf[3:5]),
			CenterX:      int16(binary.BigEndian.Uint16(buf[5:7])),
			CenterY:      int16(binary.BigEndian.Uint16(buf[7:9])),
			VarIndexBase: binary.BigEndian.Uint32(buf[9:13]),
		}, nil
	case 24:
		var buf [5]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintRotate{
			Paint: paint,
			Angle: ReadF2Dot14(buf[3:5]),
		}, nil
	case 25:
		var buf [9]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintVarRotate{
			Paint:        paint,
			Angle:        ReadF2Dot14(buf[3:5]),
			VarIndexBase: binary.BigEndian.Uint32(buf[5:9]),
		}, nil
	case 26:
		var buf [9]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintRotateAroundCenter{
			Paint:   paint,
			Angle:   ReadF2Dot14(buf[3:5]),
			CenterX: int16(binary.BigEndian.Uint16(buf[5:7])),
			CenterY: int16(binary.BigEndian.Uint16(buf[7:9])),
		}, nil
	case 27:
		var buf [13]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintVarRotateAroundCenter{
			Paint:        paint,
			Angle:        ReadF2Dot14(buf[3:5]),
			CenterX:      int16(binary.BigEndian.Uint16(buf[5:7])),
			CenterY:      int16(binary.BigEndian.Uint16(buf[7:9])),
			VarIndexBase: binary.BigEndian.Uint32(buf[9:13]),
		}, nil
	case 28:
		var buf [7]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintSkew{
			Paint:      paint,
			XSkewAngle: ReadF2Dot14(buf[3:5]),
			YSkewAngle: ReadF2Dot14(buf[5:7]),
		}, nil
	case 29:
		var buf [11]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintVarSkew{
			Paint:        paint,
			XSkewAngle:   ReadF2Dot14(buf[3:5]),
			YSkewAngle:   ReadF2Dot14(buf[5:7]),
			VarIndexBase: binary.BigEndian.Uint32(buf[7:11]),
		}, nil
	case 30:
		var buf [11]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintSkewAroundCenter{
			Paint:      paint,
			XSkewAngle: ReadF2Dot14(buf[3:5]),
			YSkewAngle: ReadF2Dot14(buf[5:7]),
			CenterX:    int16(binary.BigEndian.Uint16(buf[7:9])),
			CenterY:    int16(binary.BigEndian.Uint16(buf[9:11])),
		}, nil
	case 31:
		var buf [15]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaintAt(s, absOffset, int64(pOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintVarSkewAroundCenter{
			Paint:        paint,
			XSkewAngle:   ReadF2Dot14(buf[3:5]),
			YSkewAngle:   ReadF2Dot14(buf[5:7]),
			CenterX:      int16(binary.BigEndian.Uint16(buf[7:9])),
			CenterY:      int16(binary.BigEndian.Uint16(buf[9:11])),
			VarIndexBase: binary.BigEndian.Uint32(buf[11:15]),
		}, nil
	case 32:
		var buf [7]byte
		if err := readAtExact(s, buf[:], absOffset+1); err != nil {
			return nil, err
		}
		srcOffset := readOffset24(buf[0:3])
		compositeMode := normalizeCompositeMode(buf[3])
		bgOffset := readOffset24(buf[4:7])

		srcPaint, err := readPaintAt(s, absOffset, int64(srcOffset), depth+1)
		if err != nil {
			return nil, err
		}
		bgPaint, err := readPaintAt(s, absOffset, int64(bgOffset), depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintComposite{
			SourcePaint:   srcPaint,
			CompositeMode: compositeMode,
			BackdropPaint: bgPaint,
		}, nil
	}
	return nil, fmt.Errorf("unsupported paint format %d", format)
}

func normalizeCompositeMode(mode uint8) uint8 {
	if mode > maxCompositeMode {
		return 0
	}
	return mode
}

func parseLayerList(s api.Stream, layerListOffset uint32) ([]Paint, error) {
	baseOffset := int64(layerListOffset)
	numLayers, err := readUint32At(s, baseOffset)
	if err != nil {
		return nil, err
	}
	if !streamHasBytes(s, baseOffset+4, int64(numLayers)*4) {
		return nil, errors.New("COLR layer list out of bounds")
	}

	layers := make([]Paint, int(numLayers))
	for i := 0; i < int(numLayers); i++ {
		paintOffset, err := readUint32At(s, baseOffset+4+int64(i)*4)
		if err != nil {
			return nil, err
		}
		paint, err := readPaint(s, baseOffset, int64(paintOffset))
		if err != nil {
			return nil, fmt.Errorf("COLR layer list paint %d: %w", i, err)
		}
		layers[i] = paint
	}
	return layers, nil
}

func parseClipList(s api.Stream, clipListOffset uint32) (*ClipList, error) {
	baseOffset := int64(clipListOffset)
	var header [5]byte
	if err := readAtExact(s, header[:], baseOffset); err != nil {
		return nil, err
	}

	format := header[0]
	if format != 1 {
		return nil, fmt.Errorf("unsupported COLR clip list format %d", format)
	}
	numClips := binary.BigEndian.Uint32(header[1:5])
	recordsOffset := baseOffset + 5
	if !streamHasBytes(s, recordsOffset, int64(numClips)*7) {
		return nil, errors.New("COLR clip list records out of bounds")
	}

	clipList := &ClipList{
		Format:  format,
		Records: make([]ClipRecord, int(numClips)),
	}
	for i := 0; i < int(numClips); i++ {
		recordOffset := recordsOffset + int64(i)*7
		var recBuf [7]byte
		if err := readAtExact(s, recBuf[:], recordOffset); err != nil {
			return nil, err
		}
		clipBoxOffset := readOffset24(recBuf[4:7])
		record := ClipRecord{
			StartGlyphID:  binary.BigEndian.Uint16(recBuf[0:2]),
			EndGlyphID:    binary.BigEndian.Uint16(recBuf[2:4]),
			ClipBoxOffset: clipBoxOffset,
		}
		if clipBoxOffset != 0 {
			clipBox, err := parseClipBox(s, baseOffset+int64(clipBoxOffset))
			if err != nil {
				return nil, fmt.Errorf("COLR clip box %d: %w", i, err)
			}
			record.ClipBox = clipBox
		}
		clipList.Records[i] = record
	}
	return clipList, nil
}

func parseClipBox(s api.Stream, offset int64) (*ClipBox, error) {
	var formatBuf [1]byte
	if err := readAtExact(s, formatBuf[:], offset); err != nil {
		return nil, err
	}

	switch formatBuf[0] {
	case 1:
		var buf [8]byte
		if err := readAtExact(s, buf[:], offset+1); err != nil {
			return nil, err
		}
		return &ClipBox{
			Format: formatBuf[0],
			XMin:   int16(binary.BigEndian.Uint16(buf[0:2])),
			YMin:   int16(binary.BigEndian.Uint16(buf[2:4])),
			XMax:   int16(binary.BigEndian.Uint16(buf[4:6])),
			YMax:   int16(binary.BigEndian.Uint16(buf[6:8])),
		}, nil
	case 2:
		var buf [12]byte
		if err := readAtExact(s, buf[:], offset+1); err != nil {
			return nil, err
		}
		return &ClipBox{
			Format:       formatBuf[0],
			XMin:         int16(binary.BigEndian.Uint16(buf[0:2])),
			YMin:         int16(binary.BigEndian.Uint16(buf[2:4])),
			XMax:         int16(binary.BigEndian.Uint16(buf[4:6])),
			YMax:         int16(binary.BigEndian.Uint16(buf[6:8])),
			VarIndexBase: binary.BigEndian.Uint32(buf[8:12]),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported COLR clip box format %d", formatBuf[0])
	}
}

func parseDeltaSetIndexMap(s api.Stream, mapOffset uint32) (*DeltaSetIndexMap, error) {
	baseOffset := int64(mapOffset)
	var header [6]byte
	if err := readAtExact(s, header[:2], baseOffset); err != nil {
		return nil, err
	}

	format := header[0]
	entryFormat := header[1]
	innerIndexBitCount := uint(entryFormat&0x0F) + 1
	entrySize := int((entryFormat&0x30)>>4) + 1
	if innerIndexBitCount > uint(entrySize*8) {
		return nil, fmt.Errorf("invalid delta set index map entry format %d", entryFormat)
	}

	var mapCount uint32
	dataOffset := baseOffset
	switch format {
	case 0:
		if err := readAtExact(s, header[2:4], baseOffset+2); err != nil {
			return nil, err
		}
		mapCount = uint32(binary.BigEndian.Uint16(header[2:4]))
		dataOffset += 4
	case 1:
		if err := readAtExact(s, header[2:6], baseOffset+2); err != nil {
			return nil, err
		}
		mapCount = binary.BigEndian.Uint32(header[2:6])
		dataOffset += 6
	default:
		return nil, fmt.Errorf("unsupported delta set index map format %d", format)
	}

	if !streamHasBytes(s, dataOffset, int64(mapCount)*int64(entrySize)) {
		return nil, errors.New("delta set index map data out of bounds")
	}
	if uint64(mapCount) > uint64(int(^uint(0)>>1)) {
		return nil, errors.New("delta set index map too large")
	}

	indexMap := &DeltaSetIndexMap{
		Format:      format,
		EntryFormat: entryFormat,
		MapCount:    mapCount,
		Indices:     make([]DeltaSetIndex, int(mapCount)),
	}
	innerMask := uint32((uint64(1) << innerIndexBitCount) - 1)
	for i := 0; i < int(mapCount); i++ {
		var entryBuf [4]byte
		entryOffset := dataOffset + int64(i*entrySize)
		if err := readAtExact(s, entryBuf[4-entrySize:], entryOffset); err != nil {
			return nil, err
		}
		entry := binary.BigEndian.Uint32(entryBuf[:])
		indexMap.Indices[i] = DeltaSetIndex{
			OuterIndex: entry >> innerIndexBitCount,
			InnerIndex: entry & innerMask,
		}
	}
	return indexMap, nil
}

func parseItemVariationStore(s api.Stream, storeOffset uint32) (*ItemVariationStore, error) {
	baseOffset := int64(storeOffset)
	var header [8]byte
	if err := readAtExact(s, header[:], baseOffset); err != nil {
		return nil, err
	}

	format := binary.BigEndian.Uint16(header[0:2])
	if format != 1 {
		return nil, fmt.Errorf("unsupported item variation store format %d", format)
	}
	regionListOffset := binary.BigEndian.Uint32(header[2:6])
	itemVariationDataCount := binary.BigEndian.Uint16(header[6:8])
	if !streamHasBytes(s, baseOffset+8, int64(itemVariationDataCount)*4) {
		return nil, errors.New("item variation data offsets out of bounds")
	}

	store := &ItemVariationStore{
		Format:            format,
		ItemVariationData: make([]ItemVariationData, int(itemVariationDataCount)),
	}
	if regionListOffset != 0 {
		regionList, err := parseVariationRegionList(s, baseOffset+int64(regionListOffset))
		if err != nil {
			return nil, err
		}
		store.VariationRegionList = regionList
	}

	for i := 0; i < int(itemVariationDataCount); i++ {
		dataOffset, err := readUint32At(s, baseOffset+8+int64(i)*4)
		if err != nil {
			return nil, err
		}
		if dataOffset == 0 {
			continue
		}
		data, err := parseItemVariationData(s, baseOffset+int64(dataOffset))
		if err != nil {
			return nil, fmt.Errorf("item variation data %d: %w", i, err)
		}
		store.ItemVariationData[i] = data
	}
	return store, nil
}

func parseVariationRegionList(s api.Stream, offset int64) (VariationRegionList, error) {
	var header [4]byte
	if err := readAtExact(s, header[:], offset); err != nil {
		return VariationRegionList{}, err
	}
	axisCount := binary.BigEndian.Uint16(header[0:2])
	regionCount := binary.BigEndian.Uint16(header[2:4])
	regionSize := int64(axisCount) * 6
	if !streamHasBytes(s, offset+4, int64(regionCount)*regionSize) {
		return VariationRegionList{}, errors.New("variation region list out of bounds")
	}

	list := VariationRegionList{
		AxisCount: axisCount,
		Regions:   make([]VariationRegion, int(regionCount)),
	}
	for i := 0; i < int(regionCount); i++ {
		region := VariationRegion{Axes: make([]VariationRegionAxis, int(axisCount))}
		for j := 0; j < int(axisCount); j++ {
			axisOffset := offset + 4 + int64(i)*regionSize + int64(j)*6
			var buf [6]byte
			if err := readAtExact(s, buf[:], axisOffset); err != nil {
				return VariationRegionList{}, err
			}
			region.Axes[j] = VariationRegionAxis{
				StartCoord: ReadF2Dot14(buf[0:2]),
				PeakCoord:  ReadF2Dot14(buf[2:4]),
				EndCoord:   ReadF2Dot14(buf[4:6]),
			}
		}
		list.Regions[i] = region
	}
	return list, nil
}

func parseItemVariationData(s api.Stream, offset int64) (ItemVariationData, error) {
	var header [6]byte
	if err := readAtExact(s, header[:], offset); err != nil {
		return ItemVariationData{}, err
	}
	itemCount := binary.BigEndian.Uint16(header[0:2])
	wordDeltaCountField := binary.BigEndian.Uint16(header[2:4])
	longWords := wordDeltaCountField&0x8000 != 0
	wordDeltaCount := wordDeltaCountField & 0x7FFF
	regionIndexCount := binary.BigEndian.Uint16(header[4:6])
	if wordDeltaCount > regionIndexCount {
		return ItemVariationData{}, errors.New("word delta count exceeds region index count")
	}

	regionIndexesOffset := offset + 6
	if !streamHasBytes(s, regionIndexesOffset, int64(regionIndexCount)*2) {
		return ItemVariationData{}, errors.New("item variation region indexes out of bounds")
	}
	data := ItemVariationData{
		ItemCount:        itemCount,
		ShortDeltaCount:  wordDeltaCount,
		WordDeltaCount:   wordDeltaCount,
		LongWords:        longWords,
		RegionIndexCount: regionIndexCount,
		RegionIndexes:    make([]uint16, int(regionIndexCount)),
		Deltas:           make([][]int32, int(itemCount)),
	}
	for i := 0; i < int(regionIndexCount); i++ {
		regionIndex, err := readUint16At(s, regionIndexesOffset+int64(i)*2)
		if err != nil {
			return ItemVariationData{}, err
		}
		data.RegionIndexes[i] = regionIndex
	}

	var deltaSetSize int64
	if longWords {
		deltaSetSize = int64(wordDeltaCount)*4 + int64(regionIndexCount-wordDeltaCount)*2
	} else {
		deltaSetSize = int64(wordDeltaCount)*2 + int64(regionIndexCount-wordDeltaCount)
	}
	data.DeltaSetSize = uint32(deltaSetSize)
	deltaSetsOffset := regionIndexesOffset + int64(regionIndexCount)*2
	if !streamHasBytes(s, deltaSetsOffset, int64(itemCount)*deltaSetSize) {
		return ItemVariationData{}, errors.New("item variation delta sets out of bounds")
	}

	for item := 0; item < int(itemCount); item++ {
		row := make([]int32, int(regionIndexCount))
		rowOffset := deltaSetsOffset + int64(item)*deltaSetSize
		for region := 0; region < int(regionIndexCount); region++ {
			wordDelta := region < int(wordDeltaCount)
			if longWords {
				if wordDelta {
					v, err := readInt32At(s, rowOffset)
					if err != nil {
						return ItemVariationData{}, err
					}
					row[region] = v
					rowOffset += 4
				} else {
					v, err := readInt16At(s, rowOffset)
					if err != nil {
						return ItemVariationData{}, err
					}
					row[region] = int32(v)
					rowOffset += 2
				}
				continue
			}

			if wordDelta {
				v, err := readInt16At(s, rowOffset)
				if err != nil {
					return ItemVariationData{}, err
				}
				row[region] = int32(v)
				rowOffset += 2
			} else {
				var b [1]byte
				if err := readAtExact(s, b[:], rowOffset); err != nil {
					return ItemVariationData{}, err
				}
				row[region] = int32(int8(b[0]))
				rowOffset++
			}
		}
		data.Deltas[item] = row
	}

	return data, nil
}

func ParseCOLR(s api.Stream) (*COLR, error) {
	if s.Size() < 14 {
		return nil, errors.New("COLR table too short")
	}

	var buf [14]byte
	if err := readAtExact(s, buf[:], 0); err != nil {
		return nil, err
	}

	version := binary.BigEndian.Uint16(buf[0:2])
	numBaseGlyphRecords := binary.BigEndian.Uint16(buf[2:4])
	baseGlyphRecordsOffset := binary.BigEndian.Uint32(buf[4:8])
	layerRecordsOffset := binary.BigEndian.Uint32(buf[8:12])
	numLayerRecords := binary.BigEndian.Uint16(buf[12:14])

	colr := &COLR{
		Version:            version,
		BaseGlyphRecords:   make(map[uint16]BaseGlyphRecord),
		LayerRecords:       make([]LayerRecord, int(numLayerRecords)),
		BaseGlyphV1Records: make(map[uint16]BaseGlyphV1Record),
	}

	// Read v0 base glyph records if offset != 0
	if baseGlyphRecordsOffset != 0 {
		if !streamHasBytes(s, int64(baseGlyphRecordsOffset), int64(numBaseGlyphRecords)*6) {
			return nil, errors.New("COLR table too short for base glyph records")
		}
		for i := 0; i < int(numBaseGlyphRecords); i++ {
			var rBuf [6]byte
			if err := readAtExact(s, rBuf[:], int64(baseGlyphRecordsOffset)+int64(i*6)); err != nil {
				return nil, err
			}
			record := BaseGlyphRecord{
				GlyphID:         binary.BigEndian.Uint16(rBuf[0:2]),
				FirstLayerIndex: binary.BigEndian.Uint16(rBuf[2:4]),
				NumLayers:       binary.BigEndian.Uint16(rBuf[4:6]),
			}
			colr.BaseGlyphRecords[record.GlyphID] = record
		}
	}

	// Read layer records if offset != 0
	if layerRecordsOffset != 0 {
		if !streamHasBytes(s, int64(layerRecordsOffset), int64(numLayerRecords)*4) {
			return nil, errors.New("COLR table too short for layer records")
		}
		for i := 0; i < int(numLayerRecords); i++ {
			var rBuf [4]byte
			if err := readAtExact(s, rBuf[:], int64(layerRecordsOffset)+int64(i*4)); err != nil {
				return nil, err
			}
			colr.LayerRecords[i] = LayerRecord{
				GlyphID:      binary.BigEndian.Uint16(rBuf[0:2]),
				PaletteIndex: binary.BigEndian.Uint16(rBuf[2:4]),
			}
		}
	}

	if version >= 1 {
		if s.Size() < 34 {
			return nil, errors.New("COLR table v1 too short")
		}
		var v1Buf [20]byte
		if err := readAtExact(s, v1Buf[:], 14); err != nil {
			return nil, err
		}
		baseGlyphListOffset := binary.BigEndian.Uint32(v1Buf[0:4])
		layerListOffset := binary.BigEndian.Uint32(v1Buf[4:8])
		clipListOffset := binary.BigEndian.Uint32(v1Buf[8:12])
		colr.VarIndexMapOffset = binary.BigEndian.Uint32(v1Buf[12:16])
		colr.ItemVariationStoreOffset = binary.BigEndian.Uint32(v1Buf[16:20])

		if baseGlyphListOffset != 0 {
			var numRecordsBuf [4]byte
			if err := readAtExact(s, numRecordsBuf[:], int64(baseGlyphListOffset)); err != nil {
				return nil, err
			}
			numV1Records := binary.BigEndian.Uint32(numRecordsBuf[0:4])
			if !streamHasBytes(s, int64(baseGlyphListOffset)+4, int64(numV1Records)*6) {
				return nil, errors.New("COLR table too short for v1 base glyph records")
			}

			for i := 0; i < int(numV1Records); i++ {
				var recBuf [6]byte
				if err := readAtExact(s, recBuf[:], int64(baseGlyphListOffset)+4+int64(i*6)); err != nil {
					return nil, err
				}
				glyphID := binary.BigEndian.Uint16(recBuf[0:2])
				paintOffset := binary.BigEndian.Uint32(recBuf[2:6])

				paint, err := readPaint(s, int64(baseGlyphListOffset), int64(paintOffset))
				if err != nil {
					return nil, fmt.Errorf("COLR paint for glyph %d: %w", glyphID, err)
				}

				colr.BaseGlyphV1Records[glyphID] = BaseGlyphV1Record{
					GlyphID: glyphID,
					Paint:   paint,
				}
			}
		}

		if layerListOffset != 0 {
			layers, err := parseLayerList(s, layerListOffset)
			if err != nil {
				return nil, err
			}
			colr.LayerList = layers
		}

		if clipListOffset != 0 {
			clipList, err := parseClipList(s, clipListOffset)
			if err != nil {
				return nil, err
			}
			colr.ClipList = clipList
		}

		if colr.VarIndexMapOffset != 0 {
			indexMap, err := parseDeltaSetIndexMap(s, colr.VarIndexMapOffset)
			if err != nil {
				return nil, err
			}
			colr.VarIndexMap = indexMap
		}

		if colr.ItemVariationStoreOffset != 0 {
			store, err := parseItemVariationStore(s, colr.ItemVariationStoreOffset)
			if err != nil {
				return nil, err
			}
			colr.ItemVariationStore = store
		}
	}

	return colr, nil
}

func (c *COLR) LayerPaint(index uint32) (Paint, bool) {
	if c == nil || index >= uint32(len(c.LayerList)) {
		return nil, false
	}
	return c.LayerList[index], true
}

func (c *COLR) LayerPaints(firstLayerIndex uint32, numLayers uint8) ([]Paint, bool) {
	if c == nil {
		return nil, false
	}
	end := uint64(firstLayerIndex) + uint64(numLayers)
	if end > uint64(len(c.LayerList)) {
		return nil, false
	}
	layers := make([]Paint, int(numLayers))
	copy(layers, c.LayerList[firstLayerIndex:uint32(end)])
	return layers, true
}

func (l *ClipList) ClipBoxForGlyph(glyphID uint16) (*ClipBox, bool) {
	if l == nil {
		return nil, false
	}
	for i := range l.Records {
		record := &l.Records[i]
		if glyphID >= record.StartGlyphID && glyphID <= record.EndGlyphID && record.ClipBox != nil {
			return record.ClipBox, true
		}
	}
	return nil, false
}

func (c *COLR) ClipBoxForGlyph(glyphID uint16) (*ClipBox, bool) {
	if c == nil {
		return nil, false
	}
	return c.ClipList.ClipBoxForGlyph(glyphID)
}

func (c *COLR) EvaluateClipBox(box *ClipBox, coords []float32) *ClipBox {
	if box == nil {
		return nil
	}
	out := *box
	if box.Format != 2 {
		return &out
	}
	out.Format = 1
	out.XMin = clampInt16(math.Floor(float64(box.XMin) + float64(c.VarIndexDelta(box.VarIndexBase, 0, coords))))
	out.YMin = clampInt16(math.Floor(float64(box.YMin) + float64(c.VarIndexDelta(box.VarIndexBase, 1, coords))))
	out.XMax = clampInt16(math.Ceil(float64(box.XMax) + float64(c.VarIndexDelta(box.VarIndexBase, 2, coords))))
	out.YMax = clampInt16(math.Ceil(float64(box.YMax) + float64(c.VarIndexDelta(box.VarIndexBase, 3, coords))))
	out.VarIndexBase = 0
	return &out
}

func (c *COLR) EvaluatePaint(p Paint, coords []float32) (Paint, error) {
	return c.evaluatePaint(p, coords, 0)
}

func (c *COLR) evaluatePaint(p Paint, coords []float32, depth int) (Paint, error) {
	if p == nil {
		return nil, nil
	}
	if depth > maxPaintDepth {
		return nil, errors.New("COLR paint graph too deep")
	}

	switch paint := p.(type) {
	case *PaintColrLayers:
		out := *paint
		if c != nil && len(c.LayerList) > 0 {
			layers, ok := c.LayerPaints(paint.FirstLayerIndex, paint.NumLayers)
			if !ok {
				return nil, errors.New("COLR paint layer range out of bounds")
			}
			for _, layer := range layers {
				if _, err := c.evaluatePaint(layer, coords, depth+1); err != nil {
					return nil, err
				}
			}
		}
		return &out, nil
	case *PaintSolid:
		out := *paint
		out.Alpha = clampF2Dot1401(out.Alpha)
		return &out, nil
	case *PaintVarSolid:
		return &PaintSolid{
			PaletteIndex: paint.PaletteIndex,
			Alpha:        clampF2Dot1401(c.addDeltaF2Dot14(paint.Alpha, paint.VarIndexBase, 0, coords)),
		}, nil
	case *PaintLinearGradient:
		out := *paint
		out.ColorLine = evaluateColorLine(nil, paint.ColorLine, coords, false)
		return &out, nil
	case *PaintVarLinearGradient:
		return &PaintLinearGradient{
			ColorLine: evaluateColorLine(c, paint.ColorLine, coords, true),
			X0:        c.addDeltaInt16(paint.X0, paint.VarIndexBase, 0, coords),
			Y0:        c.addDeltaInt16(paint.Y0, paint.VarIndexBase, 1, coords),
			X1:        c.addDeltaInt16(paint.X1, paint.VarIndexBase, 2, coords),
			Y1:        c.addDeltaInt16(paint.Y1, paint.VarIndexBase, 3, coords),
			X2:        c.addDeltaInt16(paint.X2, paint.VarIndexBase, 4, coords),
			Y2:        c.addDeltaInt16(paint.Y2, paint.VarIndexBase, 5, coords),
		}, nil
	case *PaintRadialGradient:
		out := *paint
		out.ColorLine = evaluateColorLine(nil, paint.ColorLine, coords, false)
		return &out, nil
	case *PaintVarRadialGradient:
		return &PaintRadialGradient{
			ColorLine: evaluateColorLine(c, paint.ColorLine, coords, true),
			X0:        c.addDeltaInt16(paint.X0, paint.VarIndexBase, 0, coords),
			Y0:        c.addDeltaInt16(paint.Y0, paint.VarIndexBase, 1, coords),
			R0:        c.addDeltaUint16(paint.R0, paint.VarIndexBase, 2, coords),
			X1:        c.addDeltaInt16(paint.X1, paint.VarIndexBase, 3, coords),
			Y1:        c.addDeltaInt16(paint.Y1, paint.VarIndexBase, 4, coords),
			R1:        c.addDeltaUint16(paint.R1, paint.VarIndexBase, 5, coords),
		}, nil
	case *PaintSweepGradient:
		out := *paint
		out.ColorLine = evaluateColorLine(nil, paint.ColorLine, coords, false)
		return &out, nil
	case *PaintVarSweepGradient:
		return &PaintSweepGradient{
			ColorLine:  evaluateColorLine(c, paint.ColorLine, coords, true),
			CenterX:    c.addDeltaInt16(paint.CenterX, paint.VarIndexBase, 0, coords),
			CenterY:    c.addDeltaInt16(paint.CenterY, paint.VarIndexBase, 1, coords),
			StartAngle: c.addDeltaF2Dot14(paint.StartAngle, paint.VarIndexBase, 2, coords),
			EndAngle:   c.addDeltaF2Dot14(paint.EndAngle, paint.VarIndexBase, 3, coords),
		}, nil
	case *PaintGlyph:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintGlyph{Paint: child, GlyphID: paint.GlyphID}, nil
	case *PaintColrGlyph:
		out := *paint
		if c != nil {
			if record, ok := c.BaseGlyphV1Records[paint.GlyphID]; ok {
				if _, err := c.evaluatePaint(record.Paint, coords, depth+1); err != nil {
					return nil, err
				}
			}
		}
		return &out, nil
	case *PaintTransform:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintTransform{Paint: child, Transform: paint.Transform}, nil
	case *PaintVarTransform:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		transform := Affine2x3{
			XX: c.addDeltaFixed1616(paint.Transform.XX, paint.VarIndexBase, 0, coords),
			XY: c.addDeltaFixed1616(paint.Transform.XY, paint.VarIndexBase, 1, coords),
			YX: c.addDeltaFixed1616(paint.Transform.YX, paint.VarIndexBase, 2, coords),
			YY: c.addDeltaFixed1616(paint.Transform.YY, paint.VarIndexBase, 3, coords),
			DX: c.addDeltaFixed248(paint.Transform.DX, paint.VarIndexBase, 4, coords),
			DY: c.addDeltaFixed248(paint.Transform.DY, paint.VarIndexBase, 5, coords),
		}
		return &PaintTransform{Paint: child, Transform: transform}, nil
	case *PaintTranslate:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintTranslate{Paint: child, DX: paint.DX, DY: paint.DY}, nil
	case *PaintVarTranslate:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintTranslate{
			Paint: child,
			DX:    c.addDeltaInt16(paint.DX, paint.VarIndexBase, 0, coords),
			DY:    c.addDeltaInt16(paint.DY, paint.VarIndexBase, 1, coords),
		}, nil
	case *PaintScale:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScale{Paint: child, ScaleX: paint.ScaleX, ScaleY: paint.ScaleY}, nil
	case *PaintVarScale:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScale{
			Paint:  child,
			ScaleX: c.addDeltaF2Dot14(paint.ScaleX, paint.VarIndexBase, 0, coords),
			ScaleY: c.addDeltaF2Dot14(paint.ScaleY, paint.VarIndexBase, 1, coords),
		}, nil
	case *PaintScaleAroundCenter:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScaleAroundCenter{Paint: child, ScaleX: paint.ScaleX, ScaleY: paint.ScaleY, CenterX: paint.CenterX, CenterY: paint.CenterY}, nil
	case *PaintVarScaleAroundCenter:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScaleAroundCenter{
			Paint:   child,
			ScaleX:  c.addDeltaF2Dot14(paint.ScaleX, paint.VarIndexBase, 0, coords),
			ScaleY:  c.addDeltaF2Dot14(paint.ScaleY, paint.VarIndexBase, 1, coords),
			CenterX: c.addDeltaInt16(paint.CenterX, paint.VarIndexBase, 2, coords),
			CenterY: c.addDeltaInt16(paint.CenterY, paint.VarIndexBase, 3, coords),
		}, nil
	case *PaintScaleUniform:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScaleUniform{Paint: child, Scale: paint.Scale}, nil
	case *PaintVarScaleUniform:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScaleUniform{
			Paint: child,
			Scale: c.addDeltaF2Dot14(paint.Scale, paint.VarIndexBase, 0, coords),
		}, nil
	case *PaintScaleUniformAroundCenter:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScaleUniformAroundCenter{Paint: child, Scale: paint.Scale, CenterX: paint.CenterX, CenterY: paint.CenterY}, nil
	case *PaintVarScaleUniformAroundCenter:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintScaleUniformAroundCenter{
			Paint:   child,
			Scale:   c.addDeltaF2Dot14(paint.Scale, paint.VarIndexBase, 0, coords),
			CenterX: c.addDeltaInt16(paint.CenterX, paint.VarIndexBase, 1, coords),
			CenterY: c.addDeltaInt16(paint.CenterY, paint.VarIndexBase, 2, coords),
		}, nil
	case *PaintRotate:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintRotate{Paint: child, Angle: paint.Angle}, nil
	case *PaintVarRotate:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintRotate{
			Paint: child,
			Angle: c.addDeltaF2Dot14(paint.Angle, paint.VarIndexBase, 0, coords),
		}, nil
	case *PaintRotateAroundCenter:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintRotateAroundCenter{Paint: child, Angle: paint.Angle, CenterX: paint.CenterX, CenterY: paint.CenterY}, nil
	case *PaintVarRotateAroundCenter:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintRotateAroundCenter{
			Paint:   child,
			Angle:   c.addDeltaF2Dot14(paint.Angle, paint.VarIndexBase, 0, coords),
			CenterX: c.addDeltaInt16(paint.CenterX, paint.VarIndexBase, 1, coords),
			CenterY: c.addDeltaInt16(paint.CenterY, paint.VarIndexBase, 2, coords),
		}, nil
	case *PaintSkew:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintSkew{Paint: child, XSkewAngle: paint.XSkewAngle, YSkewAngle: paint.YSkewAngle}, nil
	case *PaintVarSkew:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintSkew{
			Paint:      child,
			XSkewAngle: c.addDeltaF2Dot14(paint.XSkewAngle, paint.VarIndexBase, 0, coords),
			YSkewAngle: c.addDeltaF2Dot14(paint.YSkewAngle, paint.VarIndexBase, 1, coords),
		}, nil
	case *PaintSkewAroundCenter:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintSkewAroundCenter{Paint: child, XSkewAngle: paint.XSkewAngle, YSkewAngle: paint.YSkewAngle, CenterX: paint.CenterX, CenterY: paint.CenterY}, nil
	case *PaintVarSkewAroundCenter:
		child, err := c.evaluatePaint(paint.Paint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintSkewAroundCenter{
			Paint:      child,
			XSkewAngle: c.addDeltaF2Dot14(paint.XSkewAngle, paint.VarIndexBase, 0, coords),
			YSkewAngle: c.addDeltaF2Dot14(paint.YSkewAngle, paint.VarIndexBase, 1, coords),
			CenterX:    c.addDeltaInt16(paint.CenterX, paint.VarIndexBase, 2, coords),
			CenterY:    c.addDeltaInt16(paint.CenterY, paint.VarIndexBase, 3, coords),
		}, nil
	case *PaintComposite:
		source, err := c.evaluatePaint(paint.SourcePaint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		backdrop, err := c.evaluatePaint(paint.BackdropPaint, coords, depth+1)
		if err != nil {
			return nil, err
		}
		return &PaintComposite{SourcePaint: source, CompositeMode: normalizeCompositeMode(paint.CompositeMode), BackdropPaint: backdrop}, nil
	default:
		return nil, fmt.Errorf("unsupported paint type %T", p)
	}
}

func evaluateColorLine(c *COLR, line ColorLine, coords []float32, variable bool) ColorLine {
	stops := make([]ColorStop, len(line.ColorStops))
	for i, stop := range line.ColorStops {
		stops[i] = stop
		stops[i].Alpha = clampF2Dot1401(stop.Alpha)
		if variable {
			stops[i].StopOffset = c.addDeltaF2Dot14(stop.StopOffset, stop.VarIndexBase, 0, coords)
			stops[i].Alpha = clampF2Dot1401(c.addDeltaF2Dot14(stop.Alpha, stop.VarIndexBase, 1, coords))
			stops[i].VarIndexBase = 0
		}
	}
	if variable {
		sort.SliceStable(stops, func(i, j int) bool {
			return stops[i].StopOffset < stops[j].StopOffset
		})
	}
	return ColorLine{Extend: line.Extend, ColorStops: stops}
}

func (c *COLR) addDeltaInt16(value int16, varIndexBase uint32, sequence uint32, coords []float32) int16 {
	return clampInt16(math.Round(float64(value) + float64(c.VarIndexDelta(varIndexBase, sequence, coords))))
}

func (c *COLR) addDeltaUint16(value uint16, varIndexBase uint32, sequence uint32, coords []float32) uint16 {
	return clampUint16(math.Round(float64(value) + float64(c.VarIndexDelta(varIndexBase, sequence, coords))))
}

func (c *COLR) addDeltaF2Dot14(value F2Dot14, varIndexBase uint32, sequence uint32, coords []float32) F2Dot14 {
	raw := float64(value)*16384 + float64(c.VarIndexDelta(varIndexBase, sequence, coords))
	if raw < -32768 {
		raw = -32768
	} else if raw > 32767 {
		raw = 32767
	}
	return F2Dot14(float32(raw) / 16384)
}

func (c *COLR) addDeltaFixed1616(value Fixed1616, varIndexBase uint32, sequence uint32, coords []float32) Fixed1616 {
	raw := float64(value)*65536 + float64(c.VarIndexDelta(varIndexBase, sequence, coords))
	if raw < -2147483648 {
		raw = -2147483648
	} else if raw > 2147483647 {
		raw = 2147483647
	}
	return Fixed1616(float32(raw) / 65536)
}

func (c *COLR) addDeltaFixed248(value Fixed248, varIndexBase uint32, sequence uint32, coords []float32) Fixed248 {
	raw := float64(value)*256 + float64(c.VarIndexDelta(varIndexBase, sequence, coords))
	if raw < -2147483648 {
		raw = -2147483648
	} else if raw > 2147483647 {
		raw = 2147483647
	}
	return Fixed248(float32(raw) / 256)
}

func clampF2Dot1401(value F2Dot14) F2Dot14 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func clampInt16(value float64) int16 {
	if value < -32768 {
		return -32768
	}
	if value > 32767 {
		return 32767
	}
	return int16(value)
}

func clampUint16(value float64) uint16 {
	if value < 0 {
		return 0
	}
	if value > 65535 {
		return 65535
	}
	return uint16(value)
}

func (c *COLR) GetLayers(glyphIndex uint16, cpal *CPAL, paletteIndex int) ([]Layer, error) {
	record, ok := c.BaseGlyphRecords[glyphIndex]
	if !ok {
		return nil, nil
	}

	if int(record.FirstLayerIndex)+int(record.NumLayers) > len(c.LayerRecords) {
		return nil, errors.New("invalid layer index in COLR table")
	}

	layers := make([]Layer, record.NumLayers)
	for i := 0; i < int(record.NumLayers); i++ {
		lr := c.LayerRecords[int(record.FirstLayerIndex)+i]

		color := RGBA{0, 0, 0, 255}
		if lr.PaletteIndex != 0xFFFF {
			if c, ok := cpal.Color(paletteIndex, lr.PaletteIndex); ok {
				color = c
			}
		}

		layers[i] = Layer{
			GlyphID:      lr.GlyphID,
			Color:        color,
			PaletteIndex: lr.PaletteIndex,
		}
	}

	return layers, nil
}
