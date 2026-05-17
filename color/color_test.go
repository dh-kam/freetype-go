package color

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// mockStream implements api.Stream for testing
type mockStream struct {
	data []byte
}

func (m *mockStream) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 || int(off) >= len(m.data) {
		return 0, nil
	}
	n = copy(p, m.data[off:])
	return n, nil
}

func (m *mockStream) Size() int64 {
	return int64(len(m.data))
}

func colrV1WithPaint(paint []byte) []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, uint16(1))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint16(0))

	baseGlyphListOffset := uint32(34)
	binary.Write(&buf, binary.BigEndian, baseGlyphListOffset)
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))

	binary.Write(&buf, binary.BigEndian, uint32(1))
	binary.Write(&buf, binary.BigEndian, uint16(42))
	binary.Write(&buf, binary.BigEndian, uint32(10))
	buf.Write(paint)

	return buf.Bytes()
}

func cpalTable(version uint16, entries, palettes uint16, offsets []uint16, v1Offsets [3]uint32, colors []RGBA) []byte {
	var buf bytes.Buffer
	headerLen := 12 + len(offsets)*2
	if version >= 1 {
		headerLen += 12
	}

	binary.Write(&buf, binary.BigEndian, version)
	binary.Write(&buf, binary.BigEndian, entries)
	binary.Write(&buf, binary.BigEndian, palettes)
	binary.Write(&buf, binary.BigEndian, uint16(len(colors)))
	binary.Write(&buf, binary.BigEndian, uint32(headerLen))
	for _, offset := range offsets {
		binary.Write(&buf, binary.BigEndian, offset)
	}
	if version >= 1 {
		binary.Write(&buf, binary.BigEndian, v1Offsets[0])
		binary.Write(&buf, binary.BigEndian, v1Offsets[1])
		binary.Write(&buf, binary.BigEndian, v1Offsets[2])
	}
	for _, c := range colors {
		buf.Write([]byte{c.B, c.G, c.R, c.A})
	}

	return buf.Bytes()
}

func solidPaint(paletteIndex uint16, alpha int16) []byte {
	var buf bytes.Buffer
	buf.WriteByte(2)
	binary.Write(&buf, binary.BigEndian, paletteIndex)
	binary.Write(&buf, binary.BigEndian, alpha)
	return buf.Bytes()
}

func writeOffset24(buf *bytes.Buffer, offset uint32) {
	buf.Write([]byte{byte(offset >> 16), byte(offset >> 8), byte(offset)})
}

func writeColorLine(buf *bytes.Buffer, variable bool) {
	buf.WriteByte(0)
	binary.Write(buf, binary.BigEndian, uint16(1))
	binary.Write(buf, binary.BigEndian, int16(0))
	binary.Write(buf, binary.BigEndian, uint16(0))
	binary.Write(buf, binary.BigEndian, int16(16384))
	if variable {
		binary.Write(buf, binary.BigEndian, uint32(0))
	}
}

func writeAffine(buf *bytes.Buffer, variable bool) {
	for _, v := range []int32{65536, 0, 0, 65536, 0, 0} {
		binary.Write(buf, binary.BigEndian, v)
	}
	if variable {
		binary.Write(buf, binary.BigEndian, uint32(0))
	}
}

func colrWithVariationDeltas(deltas []int32) *COLR {
	rows := make([][]int32, len(deltas))
	for i, delta := range deltas {
		rows[i] = []int32{delta}
	}
	return &COLR{
		ItemVariationStore: &ItemVariationStore{
			VariationRegionList: VariationRegionList{
				AxisCount: 1,
				Regions: []VariationRegion{
					{Axes: []VariationRegionAxis{{StartCoord: 0, PeakCoord: 1, EndCoord: 1}}},
				},
			},
			ItemVariationData: []ItemVariationData{
				{
					ItemCount:        uint16(len(deltas)),
					WordDeltaCount:   1,
					ShortDeltaCount:  1,
					RegionIndexCount: 1,
					RegionIndexes:    []uint16{0},
					DeltaSetSize:     2,
					Deltas:           rows,
				},
			},
		},
	}
}

func paintWithChild(format byte, headerLen uint32, writeHeader func(*bytes.Buffer)) []byte {
	var buf bytes.Buffer
	buf.WriteByte(format)
	writeOffset24(&buf, headerLen)
	writeHeader(&buf)
	buf.Write(solidPaint(0, 16384))
	return buf.Bytes()
}

func smokePaint(format byte) []byte {
	var buf bytes.Buffer
	switch format {
	case 1:
		buf.WriteByte(1)
		buf.WriteByte(1)
		binary.Write(&buf, binary.BigEndian, uint32(0))
	case 2:
		return solidPaint(0, 16384)
	case 3:
		buf.WriteByte(3)
		binary.Write(&buf, binary.BigEndian, uint16(0))
		binary.Write(&buf, binary.BigEndian, int16(16384))
		binary.Write(&buf, binary.BigEndian, uint32(0))
	case 4:
		buf.WriteByte(4)
		writeOffset24(&buf, 16)
		for i := 0; i < 6; i++ {
			binary.Write(&buf, binary.BigEndian, int16(0))
		}
		writeColorLine(&buf, false)
	case 5:
		buf.WriteByte(5)
		writeOffset24(&buf, 20)
		for i := 0; i < 6; i++ {
			binary.Write(&buf, binary.BigEndian, int16(0))
		}
		binary.Write(&buf, binary.BigEndian, uint32(0))
		writeColorLine(&buf, true)
	case 6:
		buf.WriteByte(6)
		writeOffset24(&buf, 16)
		for i := 0; i < 6; i++ {
			binary.Write(&buf, binary.BigEndian, uint16(0))
		}
		writeColorLine(&buf, false)
	case 7:
		buf.WriteByte(7)
		writeOffset24(&buf, 20)
		for i := 0; i < 6; i++ {
			binary.Write(&buf, binary.BigEndian, uint16(0))
		}
		binary.Write(&buf, binary.BigEndian, uint32(0))
		writeColorLine(&buf, true)
	case 8:
		buf.WriteByte(8)
		writeOffset24(&buf, 12)
		for i := 0; i < 4; i++ {
			binary.Write(&buf, binary.BigEndian, int16(0))
		}
		writeColorLine(&buf, false)
	case 9:
		buf.WriteByte(9)
		writeOffset24(&buf, 16)
		for i := 0; i < 4; i++ {
			binary.Write(&buf, binary.BigEndian, int16(0))
		}
		binary.Write(&buf, binary.BigEndian, uint32(0))
		writeColorLine(&buf, true)
	case 10:
		return paintWithChild(10, 6, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, uint16(7))
		})
	case 11:
		buf.WriteByte(11)
		binary.Write(&buf, binary.BigEndian, uint16(7))
	case 12:
		buf.WriteByte(12)
		writeOffset24(&buf, 7)
		writeOffset24(&buf, 12)
		buf.Write(solidPaint(0, 16384))
		writeAffine(&buf, false)
	case 13:
		buf.WriteByte(13)
		writeOffset24(&buf, 7)
		writeOffset24(&buf, 12)
		buf.Write(solidPaint(0, 16384))
		writeAffine(&buf, true)
	case 14:
		return paintWithChild(14, 8, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(1))
			binary.Write(buf, binary.BigEndian, int16(2))
		})
	case 15:
		return paintWithChild(15, 12, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(1))
			binary.Write(buf, binary.BigEndian, int16(2))
			binary.Write(buf, binary.BigEndian, uint32(0))
		})
	case 16:
		return paintWithChild(16, 8, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(16384))
			binary.Write(buf, binary.BigEndian, int16(16384))
		})
	case 17:
		return paintWithChild(17, 12, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(16384))
			binary.Write(buf, binary.BigEndian, int16(16384))
			binary.Write(buf, binary.BigEndian, uint32(0))
		})
	case 18:
		return paintWithChild(18, 12, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(16384))
			binary.Write(buf, binary.BigEndian, int16(16384))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
		})
	case 19:
		return paintWithChild(19, 16, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(16384))
			binary.Write(buf, binary.BigEndian, int16(16384))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, uint32(0))
		})
	case 20:
		return paintWithChild(20, 6, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(16384))
		})
	case 21:
		return paintWithChild(21, 10, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(16384))
			binary.Write(buf, binary.BigEndian, uint32(0))
		})
	case 22:
		return paintWithChild(22, 10, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(16384))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
		})
	case 23:
		return paintWithChild(23, 14, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(16384))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, uint32(0))
		})
	case 24:
		return paintWithChild(24, 6, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(0))
		})
	case 25:
		return paintWithChild(25, 10, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, uint32(0))
		})
	case 26:
		return paintWithChild(26, 10, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
		})
	case 27:
		return paintWithChild(27, 14, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, uint32(0))
		})
	case 28:
		return paintWithChild(28, 8, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
		})
	case 29:
		return paintWithChild(29, 12, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, uint32(0))
		})
	case 30:
		return paintWithChild(30, 12, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
		})
	case 31:
		return paintWithChild(31, 16, func(buf *bytes.Buffer) {
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, int16(0))
			binary.Write(buf, binary.BigEndian, uint32(0))
		})
	case 32:
		buf.WriteByte(32)
		writeOffset24(&buf, 8)
		buf.WriteByte(0)
		writeOffset24(&buf, 13)
		buf.Write(solidPaint(0, 16384))
		buf.Write(solidPaint(1, 16384))
	default:
		panic("unknown paint format")
	}
	return buf.Bytes()
}

func TestParseCOLR_V1_PaintLinearGradient(t *testing.T) {
	var buf bytes.Buffer

	// COLR Header (14 bytes)
	binary.Write(&buf, binary.BigEndian, uint16(1)) // Version
	binary.Write(&buf, binary.BigEndian, uint16(0)) // NumBaseGlyphRecords
	binary.Write(&buf, binary.BigEndian, uint32(0)) // BaseGlyphRecordsOffset
	binary.Write(&buf, binary.BigEndian, uint32(0)) // LayerRecordsOffset
	binary.Write(&buf, binary.BigEndian, uint16(0)) // NumLayerRecords

	// COLR v1 Extensions (20 bytes)
	baseGlyphListOffset := uint32(34)
	binary.Write(&buf, binary.BigEndian, baseGlyphListOffset)
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))

	// BaseGlyphList Table (starts at 34)
	binary.Write(&buf, binary.BigEndian, uint32(1)) // numRecords

	// BaseGlyphPaintRecord[0]
	binary.Write(&buf, binary.BigEndian, uint16(42)) // GlyphID
	binary.Write(&buf, binary.BigEndian, uint32(10)) // PaintOffset (from start of BaseGlyphList = 34)

	// PaintLinearGradient (Format 4)
	buf.WriteByte(4) // format

	// colorLineOffset (24-bit), relative to this paint table (offset 44)
	// PaintLinearGradient table header is 1 + 3 + 12 = 16 bytes.
	// We'll put ColorLine exactly after it, so offset is 16.
	buf.Write([]byte{0x00, 0x00, 0x10})

	// x0, y0, x1, y1, x2, y2 (int16)
	binary.Write(&buf, binary.BigEndian, int16(100))
	binary.Write(&buf, binary.BigEndian, int16(200))
	binary.Write(&buf, binary.BigEndian, int16(300))
	binary.Write(&buf, binary.BigEndian, int16(400))
	binary.Write(&buf, binary.BigEndian, int16(500))
	binary.Write(&buf, binary.BigEndian, int16(600))

	// ColorLine (starts at offset 16 from paint table)
	buf.WriteByte(1)                                // extend = 1 (pad)
	binary.Write(&buf, binary.BigEndian, uint16(2)) // numStops = 2

	// Stop 1
	binary.Write(&buf, binary.BigEndian, int16(0))     // stopOffset (F2Dot14) = 0.0
	binary.Write(&buf, binary.BigEndian, uint16(0))    // paletteIndex
	binary.Write(&buf, binary.BigEndian, int16(16384)) // alpha (F2Dot14) = 1.0

	// Stop 2
	binary.Write(&buf, binary.BigEndian, int16(16384)) // stopOffset = 1.0
	binary.Write(&buf, binary.BigEndian, uint16(1))    // paletteIndex
	binary.Write(&buf, binary.BigEndian, int16(16384)) // alpha = 1.0

	stream := &mockStream{data: buf.Bytes()}

	colr, err := ParseCOLR(stream)
	if err != nil {
		t.Fatalf("ParseCOLR failed: %v", err)
	}

	record, ok := colr.BaseGlyphV1Records[42]
	if !ok {
		t.Fatalf("BaseGlyphV1Record for glyph 42 not found")
	}

	paint, ok := record.Paint.(*PaintLinearGradient)
	if !ok {
		t.Fatalf("Expected PaintLinearGradient, got %T", record.Paint)
	}

	if paint.X0 != 100 || paint.Y0 != 200 {
		t.Errorf("Unexpected x0/y0: %d/%d", paint.X0, paint.Y0)
	}

	if paint.ColorLine.Extend != 1 {
		t.Errorf("Unexpected extend: %d", paint.ColorLine.Extend)
	}

	if len(paint.ColorLine.ColorStops) != 2 {
		t.Fatalf("Expected 2 color stops, got %d", len(paint.ColorLine.ColorStops))
	}
}

func TestParseCPALV1ReadsExtensionAfterPaletteOffsets(t *testing.T) {
	data := cpalTable(1, 1, 2, []uint16{0, 1}, [3]uint32{36, 44, 48}, []RGBA{
		{R: 10, G: 20, B: 30, A: 40},
		{R: 50, G: 60, B: 70, A: 80},
	})
	data = append(data, make([]byte, 14)...)
	stream := &mockStream{data: data}

	cpal, err := ParseCPAL(stream)
	if err != nil {
		t.Fatalf("ParseCPAL failed: %v", err)
	}
	if cpal.PaletteTypesArrayOffset != 36 || cpal.PaletteLabelsArrayOffset != 44 || cpal.PaletteEntryLabelsArrayOffset != 48 {
		t.Fatalf("unexpected CPAL v1 offsets: %d/%d/%d", cpal.PaletteTypesArrayOffset, cpal.PaletteLabelsArrayOffset, cpal.PaletteEntryLabelsArrayOffset)
	}

	got, ok := cpal.Color(1, 0)
	if !ok {
		t.Fatalf("expected palette color")
	}
	want := (RGBA{R: 50, G: 60, B: 70, A: 80})
	if got != want {
		t.Fatalf("unexpected palette color: got %+v want %+v", got, want)
	}
}

func TestParseCPALV1ReadsLabelsAndTypes(t *testing.T) {
	var buf bytes.Buffer
	const (
		numPaletteEntries = 2
		numPalettes       = 2
		numColorRecords   = 4
		headerLen         = 12 + numPalettes*2 + 12
		colorsOffset      = headerLen
		typesOffset       = colorsOffset + numColorRecords*4
		labelsOffset      = typesOffset + numPalettes*4
		entryLabelsOffset = labelsOffset + numPalettes*2
	)

	binary.Write(&buf, binary.BigEndian, uint16(1))
	binary.Write(&buf, binary.BigEndian, uint16(numPaletteEntries))
	binary.Write(&buf, binary.BigEndian, uint16(numPalettes))
	binary.Write(&buf, binary.BigEndian, uint16(numColorRecords))
	binary.Write(&buf, binary.BigEndian, uint32(colorsOffset))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(2))
	binary.Write(&buf, binary.BigEndian, uint32(typesOffset))
	binary.Write(&buf, binary.BigEndian, uint32(labelsOffset))
	binary.Write(&buf, binary.BigEndian, uint32(entryLabelsOffset))
	for i := 0; i < numColorRecords; i++ {
		buf.Write([]byte{0, 0, byte(i), 255})
	}
	binary.Write(&buf, binary.BigEndian, uint32(1))
	binary.Write(&buf, binary.BigEndian, uint32(2))
	binary.Write(&buf, binary.BigEndian, uint16(300))
	binary.Write(&buf, binary.BigEndian, uint16(301))
	binary.Write(&buf, binary.BigEndian, uint16(400))
	binary.Write(&buf, binary.BigEndian, uint16(401))

	cpal, err := ParseCPAL(&mockStream{data: buf.Bytes()})
	if err != nil {
		t.Fatalf("ParseCPAL failed: %v", err)
	}
	if got, ok := cpal.PaletteType(1); !ok || got != 2 {
		t.Fatalf("unexpected palette type: %d/%v", got, ok)
	}
	if got, ok := cpal.PaletteLabel(0); !ok || got != 300 {
		t.Fatalf("unexpected palette label: %d/%v", got, ok)
	}
	if got, ok := cpal.PaletteEntryLabel(1); !ok || got != 401 {
		t.Fatalf("unexpected palette entry label: %d/%v", got, ok)
	}
	if _, ok := cpal.PaletteType(2); ok {
		t.Fatalf("expected out-of-range palette type lookup to fail")
	}
}

func TestCPALPaletteAndLayerLookup(t *testing.T) {
	cpal, err := ParseCPAL(&mockStream{data: cpalTable(0, 2, 2, []uint16{0, 2}, [3]uint32{}, []RGBA{
		{R: 1, G: 2, B: 3, A: 4},
		{R: 5, G: 6, B: 7, A: 8},
		{R: 20, G: 30, B: 40, A: 50},
		{R: 60, G: 70, B: 80, A: 90},
	})})
	if err != nil {
		t.Fatalf("ParseCPAL failed: %v", err)
	}

	palette, ok := cpal.Palette(1)
	if !ok {
		t.Fatalf("expected palette")
	}
	if len(palette) != 2 || palette[1] != (RGBA{R: 60, G: 70, B: 80, A: 90}) {
		t.Fatalf("unexpected palette: %+v", palette)
	}

	colr := &COLR{
		BaseGlyphRecords: map[uint16]BaseGlyphRecord{
			7: {GlyphID: 7, FirstLayerIndex: 0, NumLayers: 3},
		},
		LayerRecords: []LayerRecord{
			{GlyphID: 70, PaletteIndex: 0},
			{GlyphID: 71, PaletteIndex: 1},
			{GlyphID: 72, PaletteIndex: 0xFFFF},
		},
	}

	layers, err := colr.GetLayers(7, cpal, 1)
	if err != nil {
		t.Fatalf("GetLayers failed: %v", err)
	}
	if len(layers) != 3 {
		t.Fatalf("expected 3 layers, got %d", len(layers))
	}
	if layers[0].Color != (RGBA{R: 20, G: 30, B: 40, A: 50}) {
		t.Fatalf("unexpected first layer color: %+v", layers[0].Color)
	}
	if layers[1].Color != (RGBA{R: 60, G: 70, B: 80, A: 90}) {
		t.Fatalf("unexpected second layer color: %+v", layers[1].Color)
	}
	if layers[2].Color != (RGBA{R: 0, G: 0, B: 0, A: 255}) {
		t.Fatalf("unexpected foreground fallback color: %+v", layers[2].Color)
	}

	layers, err = colr.GetLayers(7, cpal, -1)
	if err != nil {
		t.Fatalf("GetLayers with invalid palette failed: %v", err)
	}
	if layers[0].Color != (RGBA{R: 0, G: 0, B: 0, A: 255}) {
		t.Fatalf("unexpected invalid palette fallback color: %+v", layers[0].Color)
	}
}

func TestParseCPALRejectsPaletteOffsetOutOfBounds(t *testing.T) {
	stream := &mockStream{data: cpalTable(0, 2, 1, []uint16{1}, [3]uint32{}, []RGBA{
		{R: 1, A: 255},
		{R: 2, A: 255},
	})}

	if _, err := ParseCPAL(stream); err == nil {
		t.Fatalf("expected palette offset bounds error")
	}
}

func TestParseCOLRV1ReadsLayerClipAndVariationStore(t *testing.T) {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, uint16(1))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint32(34)) // baseGlyphListOffset
	binary.Write(&buf, binary.BigEndian, uint32(38)) // layerListOffset
	binary.Write(&buf, binary.BigEndian, uint32(60)) // clipListOffset
	binary.Write(&buf, binary.BigEndian, uint32(0))  // varIndexMapOffset
	binary.Write(&buf, binary.BigEndian, uint32(85)) // itemVariationStoreOffset

	binary.Write(&buf, binary.BigEndian, uint32(0))  // BaseGlyphList numRecords
	binary.Write(&buf, binary.BigEndian, uint32(2))  // LayerList numLayers
	binary.Write(&buf, binary.BigEndian, uint32(12)) // PaintOffset[0]
	binary.Write(&buf, binary.BigEndian, uint32(17)) // PaintOffset[1]
	buf.Write(solidPaint(3, 16384))
	buf.Write(solidPaint(4, 8192))

	buf.WriteByte(1)                                // ClipList format
	binary.Write(&buf, binary.BigEndian, uint32(1)) // numClips
	binary.Write(&buf, binary.BigEndian, uint16(10))
	binary.Write(&buf, binary.BigEndian, uint16(20))
	writeOffset24(&buf, 12)
	buf.WriteByte(2) // ClipBox format
	for _, v := range []int16{-10, -20, 30, 40} {
		binary.Write(&buf, binary.BigEndian, v)
	}
	binary.Write(&buf, binary.BigEndian, uint32(77))

	binary.Write(&buf, binary.BigEndian, uint16(1))  // item variation store format
	binary.Write(&buf, binary.BigEndian, uint32(12)) // variationRegionListOffset
	binary.Write(&buf, binary.BigEndian, uint16(1))  // itemVariationDataCount
	binary.Write(&buf, binary.BigEndian, uint32(22)) // itemVariationDataOffset[0]
	binary.Write(&buf, binary.BigEndian, uint16(1))  // axisCount
	binary.Write(&buf, binary.BigEndian, uint16(1))  // regionCount
	binary.Write(&buf, binary.BigEndian, int16(0))
	binary.Write(&buf, binary.BigEndian, int16(16384))
	binary.Write(&buf, binary.BigEndian, int16(0))
	binary.Write(&buf, binary.BigEndian, uint16(2)) // itemCount
	binary.Write(&buf, binary.BigEndian, uint16(1)) // shortDeltaCount
	binary.Write(&buf, binary.BigEndian, uint16(2)) // regionIndexCount
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	buf.Write([]byte{0, 1, 2, 3, 4, 5})

	colr, err := ParseCOLR(&mockStream{data: buf.Bytes()})
	if err != nil {
		t.Fatalf("ParseCOLR failed: %v", err)
	}
	if len(colr.LayerList) != 2 {
		t.Fatalf("expected 2 layer paints, got %d", len(colr.LayerList))
	}
	if got := colr.LayerList[1].(*PaintSolid).PaletteIndex; got != 4 {
		t.Fatalf("unexpected layer paint palette index: %d", got)
	}
	if colr.ClipList == nil || len(colr.ClipList.Records) != 1 {
		t.Fatalf("expected one clip record, got %#v", colr.ClipList)
	}
	clipBox := colr.ClipList.Records[0].ClipBox
	if clipBox == nil || clipBox.Format != 2 || clipBox.XMin != -10 || clipBox.VarIndexBase != 77 {
		t.Fatalf("unexpected clip box: %+v", clipBox)
	}
	if colr.ItemVariationStore == nil {
		t.Fatalf("expected item variation store")
	}
	if got := colr.ItemVariationStore.VariationRegionList.Regions[0].Axes[0].PeakCoord; got != F2Dot14(1) {
		t.Fatalf("unexpected variation peak coord: %v", got)
	}
	if got := colr.ItemVariationStore.ItemVariationData[0].DeltaSetSize; got != 3 {
		t.Fatalf("unexpected delta set size: %d", got)
	}
}

func TestParseCOLRV1VarIndexMapAndDeltaLookup(t *testing.T) {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, uint16(1))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint32(34)) // baseGlyphListOffset
	binary.Write(&buf, binary.BigEndian, uint32(0))  // layerListOffset
	binary.Write(&buf, binary.BigEndian, uint32(0))  // clipListOffset
	binary.Write(&buf, binary.BigEndian, uint32(38)) // varIndexMapOffset
	binary.Write(&buf, binary.BigEndian, uint32(46)) // itemVariationStoreOffset

	binary.Write(&buf, binary.BigEndian, uint32(0)) // BaseGlyphList numRecords
	buf.WriteByte(0)                                // DeltaSetIndexMap format 0
	buf.WriteByte(0x10)                             // two-byte entries, one inner-index bit
	binary.Write(&buf, binary.BigEndian, uint16(2))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(1))

	binary.Write(&buf, binary.BigEndian, uint16(1))  // item variation store format
	binary.Write(&buf, binary.BigEndian, uint32(12)) // variationRegionListOffset
	binary.Write(&buf, binary.BigEndian, uint16(1))  // itemVariationDataCount
	binary.Write(&buf, binary.BigEndian, uint32(22)) // itemVariationDataOffset[0]
	binary.Write(&buf, binary.BigEndian, uint16(1))  // axisCount
	binary.Write(&buf, binary.BigEndian, uint16(1))  // regionCount
	binary.Write(&buf, binary.BigEndian, int16(0))
	binary.Write(&buf, binary.BigEndian, int16(16384))
	binary.Write(&buf, binary.BigEndian, int16(16384))
	binary.Write(&buf, binary.BigEndian, uint16(2)) // itemCount
	binary.Write(&buf, binary.BigEndian, uint16(1)) // wordDeltaCount
	binary.Write(&buf, binary.BigEndian, uint16(1)) // regionIndexCount
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, int16(10))
	binary.Write(&buf, binary.BigEndian, int16(20))

	colr, err := ParseCOLR(&mockStream{data: buf.Bytes()})
	if err != nil {
		t.Fatalf("ParseCOLR failed: %v", err)
	}
	if colr.VarIndexMap == nil || colr.VarIndexMap.MapCount != 2 {
		t.Fatalf("unexpected var index map: %+v", colr.VarIndexMap)
	}
	index, ok := colr.VarIndexMap.Lookup(1)
	if !ok || index.OuterIndex != 0 || index.InnerIndex != 1 {
		t.Fatalf("unexpected mapped index: %+v/%v", index, ok)
	}
	if got := colr.VariationDelta(1, []float32{1}); got != 20 {
		t.Fatalf("mapped variation delta = %v, want 20", got)
	}
	if got := colr.VariationDelta(99, []float32{1}); got != 20 {
		t.Fatalf("out-of-range var index should use last mapping, got %v", got)
	}
	if got := colr.VarIndexDelta(NoVariationIndex, 0, []float32{1}); got != 0 {
		t.Fatalf("NoVariationIndex varIndexBase should disable variation, got %v", got)
	}
}

func TestParseItemVariationStoreLongWordsDelta(t *testing.T) {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, uint16(1))
	binary.Write(&buf, binary.BigEndian, uint32(12))
	binary.Write(&buf, binary.BigEndian, uint16(1))
	binary.Write(&buf, binary.BigEndian, uint32(22))
	binary.Write(&buf, binary.BigEndian, uint16(1))
	binary.Write(&buf, binary.BigEndian, uint16(1))
	binary.Write(&buf, binary.BigEndian, int16(0))
	binary.Write(&buf, binary.BigEndian, int16(16384))
	binary.Write(&buf, binary.BigEndian, int16(16384))
	binary.Write(&buf, binary.BigEndian, uint16(1))      // itemCount
	binary.Write(&buf, binary.BigEndian, uint16(0x8001)) // long words, one word delta
	binary.Write(&buf, binary.BigEndian, uint16(1))      // regionIndexCount
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, int32(65536))

	store, err := parseItemVariationStore(&mockStream{data: buf.Bytes()}, 0)
	if err != nil {
		t.Fatalf("parseItemVariationStore failed: %v", err)
	}
	data := store.ItemVariationData[0]
	if !data.LongWords || data.DeltaSetSize != 4 {
		t.Fatalf("unexpected long-word metadata: %+v", data)
	}
	if got := store.Delta(0, 0, []float32{1}); got != 65536 {
		t.Fatalf("long-word delta = %v, want 65536", got)
	}
}

func TestCOLRLayerListAndClipListSafeAccessors(t *testing.T) {
	box := &ClipBox{Format: 1, XMin: -10, YMin: -20, XMax: 30, YMax: 40}
	colr := &COLR{
		LayerList: []Paint{
			&PaintSolid{PaletteIndex: 1, Alpha: 1},
			&PaintSolid{PaletteIndex: 2, Alpha: 1},
		},
		ClipList: &ClipList{
			Records: []ClipRecord{
				{StartGlyphID: 10, EndGlyphID: 20, ClipBox: box},
			},
		},
	}

	if paint, ok := colr.LayerPaint(1); !ok || paint.(*PaintSolid).PaletteIndex != 2 {
		t.Fatalf("unexpected layer paint lookup: %#v/%v", paint, ok)
	}
	if _, ok := colr.LayerPaint(2); ok {
		t.Fatalf("out-of-range layer lookup should fail")
	}
	layers, ok := colr.LayerPaints(0, 2)
	if !ok || len(layers) != 2 {
		t.Fatalf("unexpected layer slice lookup: len=%d ok=%v", len(layers), ok)
	}
	if _, ok := colr.LayerPaints(1, 2); ok {
		t.Fatalf("out-of-range layer slice should fail")
	}
	if got, ok := colr.ClipBoxForGlyph(12); !ok || got != box {
		t.Fatalf("unexpected clip lookup: %+v/%v", got, ok)
	}
	if _, ok := colr.ClipBoxForGlyph(21); ok {
		t.Fatalf("out-of-range clip lookup should fail")
	}
	if _, ok := (*COLR)(nil).LayerPaint(0); ok {
		t.Fatalf("nil COLR layer lookup should fail")
	}
	if _, ok := (*ClipList)(nil).ClipBoxForGlyph(12); ok {
		t.Fatalf("nil ClipList lookup should fail")
	}
}

func TestEvaluateClipBoxAppliesVariationAndExpandsBounds(t *testing.T) {
	colr := colrWithVariationDeltas([]int32{-2, -3, 4, 5})
	box := &ClipBox{Format: 2, XMin: 10, YMin: 20, XMax: 30, YMax: 40, VarIndexBase: 0}

	got := colr.EvaluateClipBox(box, []float32{1})
	if got.Format != 1 || got.XMin != 8 || got.YMin != 17 || got.XMax != 34 || got.YMax != 45 || got.VarIndexBase != 0 {
		t.Fatalf("unexpected effective clip box: %+v", got)
	}
	if box.Format != 2 || box.XMin != 10 {
		t.Fatalf("EvaluateClipBox should not mutate input: %+v", box)
	}
}

func TestEvaluatePaintAppliesVarLinearGradientDeltas(t *testing.T) {
	colr := colrWithVariationDeltas([]int32{10, 20, 30, 40, 50, -5, 8192, -4096})
	paint := &PaintVarLinearGradient{
		ColorLine: ColorLine{
			Extend: 0,
			ColorStops: []ColorStop{
				{StopOffset: F2Dot14(0.5), PaletteIndex: 1, Alpha: 1, VarIndexBase: 6},
				{StopOffset: F2Dot14(0.25), PaletteIndex: 2, Alpha: 1, VarIndexBase: NoVariationIndex},
			},
		},
		X0:           100,
		Y0:           200,
		X1:           300,
		Y1:           400,
		X2:           500,
		Y2:           600,
		VarIndexBase: 0,
	}

	effective, err := colr.EvaluatePaint(paint, []float32{1})
	if err != nil {
		t.Fatalf("EvaluatePaint failed: %v", err)
	}
	got, ok := effective.(*PaintLinearGradient)
	if !ok {
		t.Fatalf("expected PaintLinearGradient, got %T", effective)
	}
	if got.X0 != 110 || got.Y0 != 220 || got.X2 != 550 || got.Y2 != 595 {
		t.Fatalf("unexpected effective coordinates: %+v", got)
	}
	if len(got.ColorLine.ColorStops) != 2 {
		t.Fatalf("expected two stops, got %d", len(got.ColorLine.ColorStops))
	}
	if got.ColorLine.ColorStops[0].PaletteIndex != 2 || got.ColorLine.ColorStops[1].PaletteIndex != 1 {
		t.Fatalf("expected stops sorted by effective offset: %+v", got.ColorLine.ColorStops)
	}
	if got.ColorLine.ColorStops[1].StopOffset != 1 || got.ColorLine.ColorStops[1].Alpha != F2Dot14(0.75) {
		t.Fatalf("unexpected effective var color stop: %+v", got.ColorLine.ColorStops[1])
	}
}

func TestEvaluatePaintVarTransformAppliesFixedDeltasAndChild(t *testing.T) {
	colr := colrWithVariationDeltas([]int32{8192, 65536, 0, 0, 0, 256})
	paint := &PaintVarTransform{
		Paint:        &PaintVarSolid{PaletteIndex: 3, Alpha: F2Dot14(0.5), VarIndexBase: 0},
		Transform:    Affine2x3{XX: 1, YY: 1, DX: 2},
		VarIndexBase: 1,
	}

	effective, err := colr.EvaluatePaint(paint, []float32{1})
	if err != nil {
		t.Fatalf("EvaluatePaint failed: %v", err)
	}
	got, ok := effective.(*PaintTransform)
	if !ok {
		t.Fatalf("expected PaintTransform, got %T", effective)
	}
	if got.Transform.XX != 2 || got.Transform.DX != 3 {
		t.Fatalf("unexpected effective transform: %+v", got.Transform)
	}
	child, ok := got.Paint.(*PaintSolid)
	if !ok || child.PaletteIndex != 3 || child.Alpha != 1 {
		t.Fatalf("unexpected effective child paint: %#v", got.Paint)
	}
}

func TestEvaluatePaintVarFormatsReturnStaticPaint(t *testing.T) {
	wantFormats := map[byte]uint8{
		3:  2,
		5:  4,
		7:  6,
		9:  8,
		13: 12,
		15: 14,
		17: 16,
		19: 18,
		21: 20,
		23: 22,
		25: 24,
		27: 26,
		29: 28,
		31: 30,
	}
	for format, want := range wantFormats {
		colr, err := ParseCOLR(&mockStream{data: colrV1WithPaint(smokePaint(format))})
		if err != nil {
			t.Fatalf("format %d parse failed: %v", format, err)
		}
		effective, err := colr.EvaluatePaint(colr.BaseGlyphV1Records[42].Paint, []float32{1})
		if err != nil {
			t.Fatalf("format %d evaluate failed: %v", format, err)
		}
		if effective.Format() != want {
			t.Fatalf("format %d evaluated as %d, want %d (%T)", format, effective.Format(), want, effective)
		}
	}
}

func TestParseCOLR_V1_PaintRotateAroundCenter(t *testing.T) {
	var paint bytes.Buffer
	paint.WriteByte(26)
	paint.Write([]byte{0x00, 0x00, 0x0a})
	binary.Write(&paint, binary.BigEndian, int16(8192))
	binary.Write(&paint, binary.BigEndian, int16(12))
	binary.Write(&paint, binary.BigEndian, int16(-4))
	paint.Write(solidPaint(3, 16384))

	colr, err := ParseCOLR(&mockStream{data: colrV1WithPaint(paint.Bytes())})
	if err != nil {
		t.Fatalf("ParseCOLR failed: %v", err)
	}

	got, ok := colr.BaseGlyphV1Records[42].Paint.(*PaintRotateAroundCenter)
	if !ok {
		t.Fatalf("expected PaintRotateAroundCenter, got %T", colr.BaseGlyphV1Records[42].Paint)
	}
	if got.Angle != F2Dot14(0.5) || got.CenterX != 12 || got.CenterY != -4 {
		t.Fatalf("unexpected rotate around center values: %+v", got)
	}
	if child, ok := got.Paint.(*PaintSolid); !ok || child.PaletteIndex != 3 {
		t.Fatalf("unexpected child paint: %#v", got.Paint)
	}
}

func TestParseCOLR_V1_PaintVarLinearGradient(t *testing.T) {
	var paint bytes.Buffer
	paint.WriteByte(5)
	paint.Write([]byte{0x00, 0x00, 0x14})
	for _, v := range []int16{1, 2, 3, 4, 5, 6} {
		binary.Write(&paint, binary.BigEndian, v)
	}
	binary.Write(&paint, binary.BigEndian, uint32(1234))
	paint.WriteByte(2)
	binary.Write(&paint, binary.BigEndian, uint16(1))
	binary.Write(&paint, binary.BigEndian, int16(0))
	binary.Write(&paint, binary.BigEndian, uint16(3))
	binary.Write(&paint, binary.BigEndian, int16(16384))
	binary.Write(&paint, binary.BigEndian, uint32(99))

	colr, err := ParseCOLR(&mockStream{data: colrV1WithPaint(paint.Bytes())})
	if err != nil {
		t.Fatalf("ParseCOLR failed: %v", err)
	}

	got, ok := colr.BaseGlyphV1Records[42].Paint.(*PaintVarLinearGradient)
	if !ok {
		t.Fatalf("expected PaintVarLinearGradient, got %T", colr.BaseGlyphV1Records[42].Paint)
	}
	if got.VarIndexBase != 1234 || got.X0 != 1 || got.Y2 != 6 {
		t.Fatalf("unexpected var linear values: %+v", got)
	}
	if len(got.ColorLine.ColorStops) != 1 || got.ColorLine.ColorStops[0].VarIndexBase != 99 {
		t.Fatalf("unexpected var color line: %+v", got.ColorLine)
	}
}

func TestParseCOLR_V1_AllSpecPaintFormats(t *testing.T) {
	for format := byte(1); format <= 32; format++ {
		colr, err := ParseCOLR(&mockStream{data: colrV1WithPaint(smokePaint(format))})
		if err != nil {
			t.Fatalf("format %d failed: %v", format, err)
		}
		paint := colr.BaseGlyphV1Records[42].Paint
		if paint.Format() != format {
			t.Fatalf("format %d parsed as %d (%T)", format, paint.Format(), paint)
		}
	}
}

func TestParseCOLR_V1_UnsupportedPaintReturnsError(t *testing.T) {
	stream := &mockStream{data: colrV1WithPaint([]byte{0xff})}

	if _, err := ParseCOLR(stream); err == nil {
		t.Fatalf("expected unsupported paint format error")
	}
}

func TestParseCOLR_V1_TruncatedPaintColorLineReturnsError(t *testing.T) {
	paint := []byte{
		4,
		0x00, 0x00, 0x10,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00,
	}
	stream := &mockStream{data: colrV1WithPaint(paint)}

	if _, err := ParseCOLR(stream); err == nil {
		t.Fatalf("expected truncated color line error")
	}
}

func TestParseCOLR_V1_CyclicPaintReturnsError(t *testing.T) {
	paint := []byte{
		10,
		0x00, 0x00, 0x00,
		0x00, 0x2a,
	}
	stream := &mockStream{data: colrV1WithPaint(paint)}

	if _, err := ParseCOLR(stream); err == nil {
		t.Fatalf("expected cyclic paint graph error")
	}
}
