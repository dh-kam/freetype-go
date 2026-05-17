package color

import (
	"encoding/binary"
	"errors"

	"github.com/dh-kam/freetype-go/api"
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
	Version           uint16
	NumPaletteEntries uint16
	NumPalettes       uint16
	NumColorRecords   uint16
	ColorRecords      []RGBA
	PaletteOffsets    []uint16

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
	if _, err := s.ReadAt(buf[:], 0); err != nil {
		return nil, err
	}

	version := binary.BigEndian.Uint16(buf[0:2])
	numPaletteEntries := binary.BigEndian.Uint16(buf[2:4])
	numPalettes := binary.BigEndian.Uint16(buf[4:6])
	numColorRecords := binary.BigEndian.Uint16(buf[6:8])
	colorRecordsOffset := binary.BigEndian.Uint32(buf[8:12])

	cpal := &CPAL{
		Version:           version,
		NumPaletteEntries: numPaletteEntries,
		NumPalettes:       numPalettes,
		NumColorRecords:   numColorRecords,
		ColorRecords:      make([]RGBA, numColorRecords),
		PaletteOffsets:    make([]uint16, numPalettes),
	}

	if version >= 1 {
		if s.Size() < 24 {
			return nil, errors.New("CPAL table version 1 too short")
		}
		var bufV1 [12]byte
		if _, err := s.ReadAt(bufV1[:], 12); err != nil {
			return nil, err
		}
		cpal.PaletteTypesArrayOffset = binary.BigEndian.Uint32(bufV1[0:4])
		cpal.PaletteLabelsArrayOffset = binary.BigEndian.Uint32(bufV1[4:8])
		cpal.PaletteEntryLabelsArrayOffset = binary.BigEndian.Uint32(bufV1[8:12])
	}

	if s.Size() < int64(colorRecordsOffset)+int64(numColorRecords)*4 {
		return nil, errors.New("CPAL table too short for color records")
	}

	// Read palette offsets
	for i := 0; i < int(numPalettes); i++ {
		var offBuf [2]byte
		if _, err := s.ReadAt(offBuf[:], 12+int64(i*2)); err != nil {
			return nil, err
		}
		cpal.PaletteOffsets[i] = binary.BigEndian.Uint16(offBuf[:])
	}

	// Read color records (BGRA)
	for i := 0; i < int(numColorRecords); i++ {
		var cBuf [4]byte
		if _, err := s.ReadAt(cBuf[:], int64(colorRecordsOffset)+int64(i*4)); err != nil {
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

// COLR represents the 'COLR' color table.
type COLR struct {
	Version            uint16
	BaseGlyphRecords   map[uint16]BaseGlyphRecord
	LayerRecords       []LayerRecord
	BaseGlyphV1Records map[uint16]BaseGlyphV1Record
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

// Paint interface for COLR v1
type Paint interface {
	Format() uint8
}

type ColorStop struct {
	StopOffset   F2Dot14
	PaletteIndex uint16
	Alpha        F2Dot14
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

type PaintLinearGradient struct {
	ColorLine              ColorLine
	X0, Y0, X1, Y1, X2, Y2 int16
}

func (p *PaintLinearGradient) Format() uint8 { return 4 }

type PaintRadialGradient struct {
	ColorLine ColorLine
	X0, Y0    int16
	R0        uint16
	X1, Y1    int16
	R1        uint16
}

func (p *PaintRadialGradient) Format() uint8 { return 6 }

type PaintSweepGradient struct {
	ColorLine            ColorLine
	CenterX, CenterY     int16
	StartAngle, EndAngle F2Dot14
}

func (p *PaintSweepGradient) Format() uint8 { return 8 }

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

type PaintTranslate struct {
	Paint  Paint
	DX, DY int16
}

func (p *PaintTranslate) Format() uint8 { return 14 }

type PaintScale struct {
	Paint          Paint
	ScaleX, ScaleY F2Dot14
}

func (p *PaintScale) Format() uint8 { return 16 }

type PaintRotate struct {
	Paint Paint
	Angle F2Dot14
}

func (p *PaintRotate) Format() uint8 { return 24 }

type PaintSkew struct {
	Paint                  Paint
	XSkewAngle, YSkewAngle F2Dot14
}

func (p *PaintSkew) Format() uint8 { return 28 }

type PaintComposite struct {
	SourcePaint   Paint
	CompositeMode uint8
	BackdropPaint Paint
}

func (p *PaintComposite) Format() uint8 { return 32 }

func readColorLine(s api.Stream, offset int64) (ColorLine, error) {
	var buf [3]byte
	if _, err := s.ReadAt(buf[:], offset); err != nil {
		return ColorLine{}, err
	}
	extend := buf[0]
	numStops := binary.BigEndian.Uint16(buf[1:3])
	stops := make([]ColorStop, numStops)
	for i := 0; i < int(numStops); i++ {
		var sBuf [6]byte
		if _, err := s.ReadAt(sBuf[:], offset+3+int64(i*6)); err != nil {
			return ColorLine{}, err
		}
		stops[i] = ColorStop{
			StopOffset:   ReadF2Dot14(sBuf[0:2]),
			PaletteIndex: binary.BigEndian.Uint16(sBuf[2:4]),
			Alpha:        ReadF2Dot14(sBuf[4:6]),
		}
	}
	return ColorLine{Extend: extend, ColorStops: stops}, nil
}

func readAffine2x3(s api.Stream, offset int64) (Affine2x3, error) {
	var buf [24]byte
	if _, err := s.ReadAt(buf[:], offset); err != nil {
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

func readPaint(s api.Stream, baseOffset int64, paintOffset int64) (Paint, error) {
	absOffset := baseOffset + paintOffset
	var fmtBuf [1]byte
	if _, err := s.ReadAt(fmtBuf[:], absOffset); err != nil {
		return nil, err
	}
	format := fmtBuf[0]

	switch format {
	case 1:
		var buf [5]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
			return nil, err
		}
		return &PaintColrLayers{
			NumLayers:       buf[0],
			FirstLayerIndex: binary.BigEndian.Uint32(buf[1:5]),
		}, nil
	case 2:
		var buf [4]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
			return nil, err
		}
		return &PaintSolid{
			PaletteIndex: binary.BigEndian.Uint16(buf[0:2]),
			Alpha:        ReadF2Dot14(buf[2:4]),
		}, nil
	case 4:
		var buf [15]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
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
	case 6:
		var buf [15]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
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
	case 8:
		var buf [11]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
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
	case 10:
		var buf [5]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaint(s, baseOffset, int64(pOffset))
		if err != nil {
			return nil, err
		}
		return &PaintGlyph{
			Paint:   paint,
			GlyphID: binary.BigEndian.Uint16(buf[3:5]),
		}, nil
	case 11:
		var buf [2]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
			return nil, err
		}
		return &PaintColrGlyph{
			GlyphID: binary.BigEndian.Uint16(buf[0:2]),
		}, nil
	case 12:
		var buf [6]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		tOffset := readOffset24(buf[3:6])
		paint, err := readPaint(s, baseOffset, int64(pOffset))
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
	case 14:
		var buf [7]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaint(s, baseOffset, int64(pOffset))
		if err != nil {
			return nil, err
		}
		return &PaintTranslate{
			Paint: paint,
			DX:    int16(binary.BigEndian.Uint16(buf[3:5])),
			DY:    int16(binary.BigEndian.Uint16(buf[5:7])),
		}, nil
	case 16:
		var buf [7]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaint(s, baseOffset, int64(pOffset))
		if err != nil {
			return nil, err
		}
		return &PaintScale{
			Paint:  paint,
			ScaleX: ReadF2Dot14(buf[3:5]),
			ScaleY: ReadF2Dot14(buf[5:7]),
		}, nil
	case 24:
		var buf [5]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaint(s, baseOffset, int64(pOffset))
		if err != nil {
			return nil, err
		}
		return &PaintRotate{
			Paint: paint,
			Angle: ReadF2Dot14(buf[3:5]),
		}, nil
	case 28:
		var buf [7]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
			return nil, err
		}
		pOffset := readOffset24(buf[0:3])
		paint, err := readPaint(s, baseOffset, int64(pOffset))
		if err != nil {
			return nil, err
		}
		return &PaintSkew{
			Paint:      paint,
			XSkewAngle: ReadF2Dot14(buf[3:5]),
			YSkewAngle: ReadF2Dot14(buf[5:7]),
		}, nil
	case 32:
		var buf [7]byte
		if _, err := s.ReadAt(buf[:], absOffset+1); err != nil {
			return nil, err
		}
		srcOffset := readOffset24(buf[0:3])
		compositeMode := buf[3]
		bgOffset := readOffset24(buf[4:7])

		srcPaint, err := readPaint(s, baseOffset, int64(srcOffset))
		if err != nil {
			return nil, err
		}
		bgPaint, err := readPaint(s, baseOffset, int64(bgOffset))
		if err != nil {
			return nil, err
		}
		return &PaintComposite{
			SourcePaint:   srcPaint,
			CompositeMode: compositeMode,
			BackdropPaint: bgPaint,
		}, nil
	}
	return nil, errors.New("unsupported paint format")
}

func ParseCOLR(s api.Stream) (*COLR, error) {
	if s.Size() < 14 {
		return nil, errors.New("COLR table too short")
	}

	var buf [14]byte
	if _, err := s.ReadAt(buf[:], 0); err != nil {
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
		LayerRecords:       make([]LayerRecord, numLayerRecords),
		BaseGlyphV1Records: make(map[uint16]BaseGlyphV1Record),
	}

	// Read v0 base glyph records if offset != 0
	if baseGlyphRecordsOffset != 0 {
		for i := 0; i < int(numBaseGlyphRecords); i++ {
			var rBuf [6]byte
			if _, err := s.ReadAt(rBuf[:], int64(baseGlyphRecordsOffset)+int64(i*6)); err != nil {
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
		for i := 0; i < int(numLayerRecords); i++ {
			var rBuf [4]byte
			if _, err := s.ReadAt(rBuf[:], int64(layerRecordsOffset)+int64(i*4)); err != nil {
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
		if _, err := s.ReadAt(v1Buf[:], 14); err != nil {
			return nil, err
		}
		baseGlyphListOffset := binary.BigEndian.Uint32(v1Buf[0:4])

		if baseGlyphListOffset != 0 {
			var numRecordsBuf [4]byte
			if _, err := s.ReadAt(numRecordsBuf[:], int64(baseGlyphListOffset)); err != nil {
				return nil, err
			}
			numV1Records := binary.BigEndian.Uint32(numRecordsBuf[0:4])

			for i := 0; i < int(numV1Records); i++ {
				var recBuf [6]byte
				if _, err := s.ReadAt(recBuf[:], int64(baseGlyphListOffset)+4+int64(i*6)); err != nil {
					return nil, err
				}
				glyphID := binary.BigEndian.Uint16(recBuf[0:2])
				paintOffset := binary.BigEndian.Uint32(recBuf[2:6])

				paint, err := readPaint(s, int64(baseGlyphListOffset), int64(paintOffset))
				if err != nil {
					continue
				}

				colr.BaseGlyphV1Records[glyphID] = BaseGlyphV1Record{
					GlyphID: glyphID,
					Paint:   paint,
				}
			}
		}
	}

	return colr, nil
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
		if cpal != nil && paletteIndex < int(cpal.NumPalettes) {
			baseColorIndex := int(cpal.PaletteOffsets[paletteIndex])
			colorIndex := baseColorIndex + int(lr.PaletteIndex)
			if colorIndex < len(cpal.ColorRecords) {
				color = cpal.ColorRecords[colorIndex]
			}
		} else if lr.PaletteIndex == 0xFFFF {
			color = RGBA{0, 0, 0, 255}
		}

		layers[i] = Layer{
			GlyphID:      lr.GlyphID,
			Color:        color,
			PaletteIndex: lr.PaletteIndex,
		}
	}

	return layers, nil
}
