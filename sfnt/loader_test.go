package sfnt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
	ftvar "github.com/dh-kam/freetype-go/var"
)

func TestLoadFace(t *testing.T) {
	// Create a dummy SFNT
	data := make([]byte, 200)

	// Offset Table
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], 2) // numTables

	// Table Directory
	// head
	copy(data[12:16], "head")
	binary.BigEndian.PutUint32(data[20:24], 44) // offset
	binary.BigEndian.PutUint32(data[24:28], 54) // length

	// maxp
	copy(data[28:32], "maxp")
	binary.BigEndian.PutUint32(data[36:40], 98) // offset
	binary.BigEndian.PutUint32(data[40:44], 6)  // length

	// head table data (at 44)
	binary.BigEndian.PutUint16(data[44+50:44+52], 1) // indexToLocFormat = 1 (long)

	// maxp table data (at 98)
	binary.BigEndian.PutUint16(data[98+4:98+6], 42) // numGlyphs = 42

	stream := &mockStream{data: data}
	sys := &mockSystem{}
	loader := NewLoader(sys)

	f, err := loader.LoadFace(stream)
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}

	if f.GetNumGlyphs() != 42 {
		t.Errorf("expected 42 glyphs, got %d", f.GetNumGlyphs())
	}

	// Verify indexToLocFormat
	sfntFace := f.(*Face)
	if sfntFace.head.IndexToLocFormat != 1 {
		t.Errorf("expected indexToLocFormat 1, got %d", sfntFace.head.IndexToLocFormat)
	}
}

func TestFaceGetGlyphNameUsesPostTable(t *testing.T) {
	tables := []sfntTestTable{
		{tag: "head", data: buildMetricsTestHeadTable()},
		{tag: "maxp", data: buildMetricsTestMaxpTable(2)},
		{tag: "post", data: buildGlyphNamePostTable()},
	}
	loaded, err := NewLoader(&mockSystem{}).LoadFace(&mockStream{data: buildSFNTTestData(t, tables)})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	face := loaded.(*Face)
	if name, ok := face.GetGlyphName(0); !ok || name != "A.alt" {
		t.Fatalf("GetGlyphName(0) = %q, %v; want A.alt, true", name, ok)
	}
	if name, ok := face.GetGlyphName(1); !ok || name != "A" {
		t.Fatalf("GetGlyphName(1) = %q, %v; want A, true", name, ok)
	}
	if _, ok := face.GetGlyphName(2); ok {
		t.Fatal("GetGlyphName succeeded for out-of-range glyph")
	}
	if name, ok := api.GetGlyphName(face, 0); !ok || name != "A.alt" {
		t.Fatalf("api.GetGlyphName = %q, %v; want A.alt, true", name, ok)
	}
}

func TestLoadFaceTTCFirstFace(t *testing.T) {
	data := testTTCData([]ttcFaceSpec{
		{offset: 32, headOffset: 300, maxpOffset: 354, numGlyphs: 7, indexToLocFormat: 1},
	})

	stream := &mockStream{data: data}
	loader := NewLoader(&mockSystem{})
	if !loader.Handles(stream) {
		t.Fatal("loader should handle TTC streams")
	}

	f, err := loader.LoadFace(stream)
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	if f.GetNumGlyphs() != 7 {
		t.Fatalf("numGlyphs = %d, want 7", f.GetNumGlyphs())
	}
	sfntFace := f.(*Face)
	if sfntFace.directoryOffset != 32 {
		t.Fatalf("directoryOffset = %d, want %d", sfntFace.directoryOffset, 32)
	}
	if sfntFace.head.IndexToLocFormat != 1 {
		t.Fatalf("indexToLocFormat = %d, want 1", sfntFace.head.IndexToLocFormat)
	}
}

func TestLoadFaceTTCFaceIndex(t *testing.T) {
	data := testTTCData([]ttcFaceSpec{
		{offset: 32, headOffset: 300, maxpOffset: 354, numGlyphs: 7, indexToLocFormat: 0},
		{offset: 128, headOffset: 420, maxpOffset: 474, numGlyphs: 13, indexToLocFormat: 1},
	})
	stream := &mockStream{data: data}

	numFaces, err := NumFaces(stream)
	if err != nil {
		t.Fatalf("NumFaces failed: %v", err)
	}
	if numFaces != 2 {
		t.Fatalf("NumFaces = %d, want 2", numFaces)
	}

	first, err := LoadFaceIndex(&mockSystem{}, stream, 0)
	if err != nil {
		t.Fatalf("LoadFaceIndex(0) failed: %v", err)
	}
	if first.GetNumGlyphs() != 7 {
		t.Fatalf("first face numGlyphs = %d, want 7", first.GetNumGlyphs())
	}

	second, err := LoadFaceIndex(&mockSystem{}, stream, 1)
	if err != nil {
		t.Fatalf("LoadFaceIndex(1) failed: %v", err)
	}
	if second.GetNumGlyphs() != 13 {
		t.Fatalf("second face numGlyphs = %d, want 13", second.GetNumGlyphs())
	}
	sfntFace := second.(*Face)
	if sfntFace.directoryOffset != 128 {
		t.Fatalf("second face directoryOffset = %d, want 128", sfntFace.directoryOffset)
	}
	if sfntFace.head.IndexToLocFormat != 1 {
		t.Fatalf("second face indexToLocFormat = %d, want 1", sfntFace.head.IndexToLocFormat)
	}
}

func TestLoadFaceIndexRejectsOutOfRange(t *testing.T) {
	data := testTTCData([]ttcFaceSpec{
		{offset: 32, headOffset: 300, maxpOffset: 354, numGlyphs: 7},
	})
	_, err := LoadFaceIndex(&mockSystem{}, &mockStream{data: data}, 1)
	if err == nil {
		t.Fatal("expected out-of-range face index to fail")
	}

	data = make([]byte, 200)
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	_, err = LoadFaceIndex(&mockSystem{}, &mockStream{data: data}, 1)
	if err == nil {
		t.Fatal("expected non-collection face index to fail")
	}
}

func TestNumFacesNonCollection(t *testing.T) {
	data := make([]byte, 16)
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	numFaces, err := NumFaces(&mockStream{data: data})
	if err != nil {
		t.Fatalf("NumFaces failed: %v", err)
	}
	if numFaces != 1 {
		t.Fatalf("NumFaces = %d, want 1", numFaces)
	}
}

type ttcFaceSpec struct {
	offset           uint32
	headOffset       uint32
	maxpOffset       uint32
	numGlyphs        uint16
	indexToLocFormat uint16
}

func testTTCData(faces []ttcFaceSpec) []byte {
	size := 16
	for _, face := range faces {
		for _, end := range []uint32{face.offset + 44, face.headOffset + 54, face.maxpOffset + 6} {
			if int(end) > size {
				size = int(end)
			}
		}
	}
	data := make([]byte, size)
	copy(data[0:4], "ttcf")
	binary.BigEndian.PutUint32(data[4:8], 0x00010000)
	binary.BigEndian.PutUint32(data[8:12], uint32(len(faces)))
	for i, face := range faces {
		binary.BigEndian.PutUint32(data[12+i*4:16+i*4], face.offset)
	}
	for _, face := range faces {
		base := int(face.offset)
		binary.BigEndian.PutUint32(data[base:base+4], 0x00010000)
		binary.BigEndian.PutUint16(data[base+4:base+6], 2)

		copy(data[base+12:base+16], "head")
		binary.BigEndian.PutUint32(data[base+20:base+24], face.headOffset)
		binary.BigEndian.PutUint32(data[base+24:base+28], 54)

		copy(data[base+28:base+32], "maxp")
		binary.BigEndian.PutUint32(data[base+36:base+40], face.maxpOffset)
		binary.BigEndian.PutUint32(data[base+40:base+44], 6)

		binary.BigEndian.PutUint16(data[face.headOffset+50:face.headOffset+52], face.indexToLocFormat)
		binary.BigEndian.PutUint16(data[face.maxpOffset+4:face.maxpOffset+6], face.numGlyphs)
	}
	return data
}

func TestLoadFaceTTCRejectsEmptyCollection(t *testing.T) {
	data := make([]byte, 16)
	copy(data[0:4], "ttcf")
	binary.BigEndian.PutUint32(data[4:8], 0x00010000)

	loader := NewLoader(&mockSystem{})
	_, err := loader.LoadFace(&mockStream{data: data})
	if err == nil {
		t.Fatal("expected empty TTC collection to fail")
	}
}

func TestReadUint16RejectsShortRead(t *testing.T) {
	_, err := readUint16(&mockStream{data: []byte{0x12}}, 0)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("got %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

func TestSetPixelSizes(t *testing.T) {
	f := &Face{}
	if err := f.SetPixelSizes(0, 19); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	if f.xPPEM != 19 || f.yPPEM != 19 || f.pointSize != 19 {
		t.Fatalf("unexpected size state: xPPEM=%d yPPEM=%d pointSize=%d", f.xPPEM, f.yPPEM, f.pointSize)
	}
	if err := f.SetPixelSizes(12, 34); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	if f.xPPEM != 12 || f.yPPEM != 34 || f.pointSize != 34 {
		t.Fatalf("unexpected size state: xPPEM=%d yPPEM=%d pointSize=%d", f.xPPEM, f.yPPEM, f.pointSize)
	}
	if err := f.SetPixelSizes(0, 0); err == nil {
		t.Fatal("expected zero pixel size to fail")
	}
	if err := f.SetPixelSizes(-1, 12); err == nil {
		t.Fatal("expected negative pixel size to fail")
	}
}

func TestScaleCFFOutlineUsesFreeTypeDesignUnitRounding(t *testing.T) {
	f := &Face{head: HeadTable{UnitsPerEm: 1000}}
	if err := f.SetPixelSizes(25, 25); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	outline := &core.Outline{
		Points: []api.Vector{
			{X: 261 << 6, Y: 647 << 6},
		},
	}

	f.scaleCFFOutline(outline)

	if got, want := outline.Points[0].X, int32(418); got != want {
		t.Fatalf("scaled CFF X = %d, want %d", got, want)
	}
	if got, want := outline.Points[0].Y, int32(1035); got != want {
		t.Fatalf("scaled CFF Y = %d, want %d", got, want)
	}
}

func TestSetPixelSizesScalesCVTAndRunsPrep(t *testing.T) {
	f := &Face{
		head:   HeadTable{UnitsPerEm: 1000},
		maxp:   MaxpTable{MaxStackElements: 8, MaxStorage: 1},
		cvt:    []int32{100, 200},
		prep:   []byte{0xB0, 1, 0xB0, 77, 0x44}, // PUSHB 1; PUSHB 77; WCVTP
		funcs:  make(map[int32][]byte),
		instrs: make(map[int32][]byte),
	}

	if err := f.SetPixelSizes(20, 20); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	if got, want := f.scaledCVT[0], int32(128); got != want {
		t.Fatalf("scaled CVT[0]: got %d, want %d", got, want)
	}
	if got, want := f.scaledCVT[1], int32(77); got != want {
		t.Fatalf("prep-adjusted CVT[1]: got %d, want %d", got, want)
	}

	f.scaledCVT[1] = 999
	if err := f.SetPixelSizes(10, 10); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	if got, want := f.scaledCVT[0], int32(64); got != want {
		t.Fatalf("rescaled CVT[0]: got %d, want %d", got, want)
	}
	if got, want := f.scaledCVT[1], int32(77); got != want {
		t.Fatalf("prep did not rerun for CVT[1]: got %d, want %d", got, want)
	}
}

func TestSetPixelSizesAppliesCvarBeforeScaling(t *testing.T) {
	cvar, err := ftvar.ParseCvar(&mockStream{data: buildCvarOneAxisDeltaTable(50)}, 1)
	if err != nil {
		t.Fatalf("ParseCvar failed: %v", err)
	}
	ve := ftvar.NewVariationEngine(&ftvar.FvarTable{
		Axes: []ftvar.AxisRecord{{Tag: stringToTag("wght"), MinValue: -1 << 16, MaxValue: 1 << 16}},
	}, nil, nil, nil)
	ve.SetCvar(cvar)
	ve.SetNormalizedCoordinates([]float32{1})

	f := &Face{
		head:      HeadTable{UnitsPerEm: 1000},
		maxp:      MaxpTable{MaxStackElements: 8, MaxStorage: 1},
		cvt:       []int32{100},
		funcs:     make(map[int32][]byte),
		instrs:    make(map[int32][]byte),
		varEngine: ve,
	}

	if err := f.SetPixelSizes(20, 20); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	if got, want := f.scaledCVT[0], int32(192); got != want {
		t.Fatalf("cvar-adjusted scaled CVT[0] = %d, want %d", got, want)
	}

	if err := f.SetVariationNormalizedCoordinates([]float32{0}); err != nil {
		t.Fatalf("reset variation coordinates failed: %v", err)
	}
	if got, want := f.scaledCVT[0], int32(128); got != want {
		t.Fatalf("reset scaled CVT[0] = %d, want %d", got, want)
	}
}

func TestLoadFaceParsesGDEFAndLinksLayoutTables(t *testing.T) {
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000)

	maxp := make([]byte, 6)
	binary.BigEndian.PutUint16(maxp[4:6], 1)

	gdef := make([]byte, 12)
	binary.BigEndian.PutUint16(gdef[0:2], 1)

	loaded, err := NewLoader(&mockSystem{}).LoadFace(&mockStream{data: buildSFNTTestData(t, []sfntTestTable{
		{tag: "head", data: head},
		{tag: "maxp", data: maxp},
		{tag: "GDEF", data: gdef},
		{tag: "GSUB", data: buildMinimalOpenTypeLayoutTable(1)},
		{tag: "GPOS", data: buildMinimalOpenTypeLayoutTable(2)},
	})})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}

	face := loaded.(*Face)
	if face.gdef == nil {
		t.Fatalf("GDEF was not parsed")
	}
	if face.gsub == nil {
		t.Fatalf("GSUB was not parsed")
	}
	if face.gsub.GDEF != face.gdef {
		t.Fatalf("GSUB GDEF link = %p, want %p", face.gsub.GDEF, face.gdef)
	}
	if face.gpos == nil {
		t.Fatalf("GPOS was not parsed")
	}
	if face.gpos.GDEF != face.gdef {
		t.Fatalf("GPOS GDEF link = %p, want %p", face.gpos.GDEF, face.gdef)
	}
}

func buildMinimalOpenTypeLayoutTable(lookupType uint16) []byte {
	data := make([]byte, 24)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 12)
	binary.BigEndian.PutUint16(data[8:10], 14)
	binary.BigEndian.PutUint16(data[14:16], 1)
	binary.BigEndian.PutUint16(data[16:18], 4)
	binary.BigEndian.PutUint16(data[18:20], lookupType)
	return data
}

func TestCFF2LoadGlyphUsesSFNTVariationCoordinates(t *testing.T) {
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000)

	maxp := make([]byte, 6)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00005000)
	binary.BigEndian.PutUint16(maxp[4:6], 2)

	loaded, err := NewLoader(&mockSystem{}).LoadFace(&mockStream{data: buildSFNTTestData(t, []sfntTestTable{
		{tag: "head", data: head},
		{tag: "maxp", data: maxp},
		{tag: "fvar", data: buildFvarOneAxisTable()},
		{tag: "CFF2", data: buildCFF2VariationTestTable()},
	})})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	face := loaded.(*Face)
	if face.cff == nil || face.cff.Major != 2 {
		t.Fatalf("CFF2 table was not parsed")
	}

	slot, err := face.LoadGlyph(1, api.LoadNoScale)
	if err != nil {
		t.Fatalf("LoadGlyph default coordinates failed: %v", err)
	}
	assertSlotPoint(t, slot, 0, 10<<6, 20<<6)

	if err := face.SetVariationNormalizedCoordinates([]float32{1}); err != nil {
		t.Fatalf("SetVariationNormalizedCoordinates failed: %v", err)
	}
	slot, err = face.LoadGlyph(1, api.LoadNoScale)
	if err != nil {
		t.Fatalf("LoadGlyph varied coordinates failed: %v", err)
	}
	assertSlotPoint(t, slot, 0, 15<<6, 17<<6)
}

func assertSlotPoint(t *testing.T, slot api.GlyphSlot, index int, x, y int32) {
	t.Helper()
	points := slot.GetOutline().GetPoints()
	if index >= len(points) {
		t.Fatalf("point %d missing; outline has %d points", index, len(points))
	}
	if points[index].X != x || points[index].Y != y {
		t.Fatalf("point %d = (%d,%d), want (%d,%d)", index, points[index].X, points[index].Y, x, y)
	}
}

func TestSetPixelSizesPassesUnitsPerEmToPrepVM(t *testing.T) {
	f := &Face{
		head:   HeadTable{UnitsPerEm: 1000},
		maxp:   MaxpTable{MaxStackElements: 8, MaxStorage: 1},
		cvt:    []int32{0},
		prep:   []byte{0xB0, 0, 0xB0, 100, 0x70}, // PUSHB 0; PUSHB 100; WCVTF
		funcs:  make(map[int32][]byte),
		instrs: make(map[int32][]byte),
	}

	if err := f.SetPixelSizes(20, 20); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	if got, want := f.scaledCVT[0], int32(128); got != want {
		t.Fatalf("prep WCVTF CVT[0] = %d, want %d", got, want)
	}
}

func TestSizeScalesMetricsAndOutline(t *testing.T) {
	f := &Face{
		head: HeadTable{UnitsPerEm: 1000},
		maxp: MaxpTable{NumGlyphs: 1},
		hhea: HheaTable{NumberOfHMetrics: 1},
		hmtx: HmtxTable{
			HMetrics: []HMetric{{AdvanceWidth: 1000, LeftSideBearing: 100}},
		},
	}
	if err := f.SetPixelSizes(20, 10); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}

	advance, lsb, err := f.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 20<<6 || lsb != 2<<6 {
		t.Fatalf("metrics got advance=%d lsb=%d, want %d %d", advance, lsb, 20<<6, 2<<6)
	}

	outline := &core.Outline{
		Points: []api.Vector{
			{X: 1000 << 6, Y: 500 << 6},
			{X: 100 << 6, Y: 100 << 6},
		},
		Tags:     []byte{1, 1},
		Contours: []int{1},
	}
	f.scaleOutline(outline)

	if got, want := outline.Points[0], (api.Vector{X: 20 << 6, Y: 5 << 6}); got != want {
		t.Fatalf("point 0: got %+v, want %+v", got, want)
	}
	if got, want := outline.Points[1], (api.Vector{X: 2 << 6, Y: 1 << 6}); got != want {
		t.Fatalf("point 1: got %+v, want %+v", got, want)
	}
}

func TestLoadedFaceSetPixelSizesRescalesGlyphMetricsAndOutline(t *testing.T) {
	glyph := simpleRectGlyphData(50, -100, 550, 700)
	face := loadGlyphMetricsTestFace(t, []uint32{0, uint32(len(glyph))}, glyph, []metricsGlyph{
		{advance: 800, lsb: 50},
	})

	if err := face.SetPixelSizes(20, 40); err != nil {
		t.Fatalf("SetPixelSizes 20x40 failed: %v", err)
	}
	slot, err := face.LoadGlyph(0, api.LoadNoHinting)
	if err != nil {
		t.Fatalf("LoadGlyph at 20x40 failed: %v", err)
	}
	assertSlotPoint(t, slot, 0, 64, -256)
	assertSlotPoint(t, slot, 2, 704, 1792)

	advance, lsb, err := face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics at 20x40 failed: %v", err)
	}
	if advance != 1024 || lsb != 64 {
		t.Fatalf("20x40 metrics got advance=%d lsb=%d, want 1024 64", advance, lsb)
	}

	if err := face.SetPixelSizes(10, 20); err != nil {
		t.Fatalf("SetPixelSizes 10x20 failed: %v", err)
	}
	advance, lsb, err = face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics after resize failed: %v", err)
	}
	if advance != 512 || lsb != 32 {
		t.Fatalf("resized metrics got advance=%d lsb=%d, want 512 32", advance, lsb)
	}

	slot, err = face.LoadGlyph(0, api.LoadNoHinting)
	if err != nil {
		t.Fatalf("LoadGlyph at 10x20 failed: %v", err)
	}
	assertSlotPoint(t, slot, 0, 32, -128)
	assertSlotPoint(t, slot, 2, 352, 896)
}

func TestLoadGlyphSlotMetricsUseNonSquarePixelScales(t *testing.T) {
	glyph := simpleRectGlyphData(50, -100, 550, 700)
	face := loadGlyphMetricsTestFace(t, []uint32{0, uint32(len(glyph))}, glyph, []metricsGlyph{
		{advance: 800, lsb: 50},
	})
	face.vhea.NumOfLongVerMetrics = 1
	face.vmtx = VmtxTable{VMetrics: []VMetric{{AdvanceHeight: 900, TopSideBearing: 100}}}

	if err := face.SetPixelSizes(20, 40); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	slot, err := face.LoadGlyph(0, api.LoadNoHinting)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	metrics, ok := api.GetGlyphSlotMetrics(slot)
	if !ok {
		t.Fatal("glyph slot metrics unavailable")
	}

	want := api.GlyphMetrics{
		Width:        640,
		Height:       2048,
		HoriBearingX: 64,
		HoriBearingY: 1792,
		HoriAdvance:  1024,
		VertBearingX: -448,
		VertBearingY: 256,
		VertAdvance:  2304,
	}
	if metrics != want {
		t.Fatalf("non-square slot metrics got %+v, want %+v", metrics, want)
	}
}

func TestLoadedFaceSetPixelSizesRerunsPrepBeforeHinting(t *testing.T) {
	glyph := simpleOnePointGlyphData(50, 20, []byte{
		0xB0, 0, // PUSHB point 0
		0xB0, 0, // PUSHB CVT index 0
		0x3E, // MIAP[0]
	})
	prep := []byte{
		0x00,    // SVTCA[y]
		0xB0, 0, // PUSHB CVT index 0
		0xB0, 100, // PUSHB 100 FUnits
		0x70, // WCVTF
	}
	face := loadSingleGlyphProgramFaceWithPrograms(t, glyph, prep, make([]byte, 2))

	for _, tc := range []struct {
		width int
		wantX int32
		wantY int32
	}{
		{width: 20, wantX: 26, wantY: 128},
		{width: 10, wantX: 13, wantY: 64},
	} {
		if err := face.SetPixelSizes(tc.width, tc.width); err != nil {
			t.Fatalf("SetPixelSizes %d failed: %v", tc.width, err)
		}
		slot, err := face.LoadGlyph(0, api.LoadTargetMono)
		if err != nil {
			t.Fatalf("LoadGlyph at %d ppem failed: %v", tc.width, err)
		}
		assertSlotPoint(t, slot, 0, tc.wantX, tc.wantY)
	}
}

func TestGetGlyphMetricsRejectsMissingOrShortHmtx(t *testing.T) {
	f := &Face{
		maxp: MaxpTable{NumGlyphs: 2},
		hhea: HheaTable{NumberOfHMetrics: 2},
	}
	if _, _, err := f.GetGlyphMetrics(0); err == nil {
		t.Fatal("expected missing hmtx to fail")
	}

	f.hmtx = HmtxTable{
		HMetrics: []HMetric{{AdvanceWidth: 1000, LeftSideBearing: 20}},
	}
	if _, _, err := f.GetGlyphMetrics(1); err == nil {
		t.Fatal("expected short hmtx to fail")
	}
}

func loadGlyphBoundsTestFace(t *testing.T, indexToLocFormat int16, offsets []uint32, glyf []byte) *Face {
	t.Helper()
	if len(offsets) < 2 {
		t.Fatal("glyph bounds fixture requires at least two loca offsets")
	}

	align4 := func(v uint32) uint32 {
		return (v + 3) &^ 3
	}

	numGlyphs := len(offsets) - 1
	var locaLen uint32
	if indexToLocFormat == 0 {
		locaLen = uint32(len(offsets) * 2)
	} else {
		locaLen = uint32(len(offsets) * 4)
	}

	glyfOffset := uint32(128)
	headOffset := align4(glyfOffset + uint32(len(glyf)))
	maxpOffset := align4(headOffset + 54)
	locaOffset := align4(maxpOffset + 32)
	data := make([]byte, locaOffset+locaLen)

	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], 4)

	writeTable := func(index int, tag string, offset, length uint32) {
		entry := 12 + index*16
		copy(data[entry:entry+4], tag)
		binary.BigEndian.PutUint32(data[entry+8:entry+12], offset)
		binary.BigEndian.PutUint32(data[entry+12:entry+16], length)
	}
	writeTable(0, "glyf", glyfOffset, uint32(len(glyf)))
	writeTable(1, "head", headOffset, 54)
	writeTable(2, "loca", locaOffset, locaLen)
	writeTable(3, "maxp", maxpOffset, 32)

	copy(data[glyfOffset:glyfOffset+uint32(len(glyf))], glyf)

	head := int(headOffset)
	binary.BigEndian.PutUint16(data[head+18:head+20], 1000)
	binary.BigEndian.PutUint16(data[head+50:head+52], uint16(indexToLocFormat))

	maxp := int(maxpOffset)
	binary.BigEndian.PutUint32(data[maxp:maxp+4], 0x00010000)
	binary.BigEndian.PutUint16(data[maxp+4:maxp+6], uint16(numGlyphs))

	loca := int(locaOffset)
	for i, offset := range offsets {
		if indexToLocFormat == 0 {
			if offset%2 != 0 {
				t.Fatalf("short loca offset %d is not even", offset)
			}
			binary.BigEndian.PutUint16(data[loca+i*2:loca+i*2+2], uint16(offset/2))
		} else {
			binary.BigEndian.PutUint32(data[loca+i*4:loca+i*4+4], offset)
		}
	}

	f, err := NewLoader(&mockSystem{}).LoadFace(&mockStream{data: data})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	return f.(*Face)
}

func compositeGlyphData(subGlyph uint16) []byte {
	data := make([]byte, 16)
	binary.BigEndian.PutUint16(data[0:2], 0xffff)
	binary.BigEndian.PutUint16(data[10:12], ARGS_ARE_XY_VALUES)
	binary.BigEndian.PutUint16(data[12:14], subGlyph)
	return data
}

func compositeGlyphWithComponents(subGlyph uint16, count int) []byte {
	data := make([]byte, 10+count*6)
	binary.BigEndian.PutUint16(data[0:2], 0xffff)
	offset := 10
	for i := 0; i < count; i++ {
		flags := uint16(ARGS_ARE_XY_VALUES)
		if i < count-1 {
			flags |= MORE_COMPONENTS
		}
		binary.BigEndian.PutUint16(data[offset:offset+2], flags)
		binary.BigEndian.PutUint16(data[offset+2:offset+4], subGlyph)
		offset += 6
	}
	return data
}

func onePointGlyphData() []byte {
	data := make([]byte, 15)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[10:12], 0)
	binary.BigEndian.PutUint16(data[12:14], 0)
	data[14] = 0x01 | 0x10 | 0x20
	return data
}

type metricsGlyph struct {
	advance uint16
	lsb     int16
}

func loadGlyphMetricsTestFace(t *testing.T, offsets []uint32, glyf []byte, metrics []metricsGlyph) *Face {
	t.Helper()
	if len(offsets) < 2 {
		t.Fatal("glyph metrics fixture requires at least two loca offsets")
	}
	numGlyphs := len(offsets) - 1
	if len(metrics) != numGlyphs {
		t.Fatalf("metrics count = %d, want %d", len(metrics), numGlyphs)
	}

	align4 := func(v uint32) uint32 {
		return (v + 3) &^ 3
	}

	const numTables = 6
	glyfOffset := uint32(160)
	headOffset := align4(glyfOffset + uint32(len(glyf)))
	hheaOffset := align4(headOffset + 54)
	hmtxOffset := align4(hheaOffset + 36)
	hmtxLen := uint32(len(metrics) * 4)
	locaOffset := align4(hmtxOffset + hmtxLen)
	locaLen := uint32(len(offsets) * 4)
	maxpOffset := align4(locaOffset + locaLen)
	data := make([]byte, maxpOffset+32)

	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], numTables)
	writeTable := func(index int, tag string, offset, length uint32) {
		entry := 12 + index*16
		copy(data[entry:entry+4], tag)
		binary.BigEndian.PutUint32(data[entry+8:entry+12], offset)
		binary.BigEndian.PutUint32(data[entry+12:entry+16], length)
	}
	writeTable(0, "glyf", glyfOffset, uint32(len(glyf)))
	writeTable(1, "head", headOffset, 54)
	writeTable(2, "hhea", hheaOffset, 36)
	writeTable(3, "hmtx", hmtxOffset, hmtxLen)
	writeTable(4, "loca", locaOffset, locaLen)
	writeTable(5, "maxp", maxpOffset, 32)

	copy(data[glyfOffset:glyfOffset+uint32(len(glyf))], glyf)

	head := int(headOffset)
	binary.BigEndian.PutUint16(data[head+18:head+20], 1000)
	binary.BigEndian.PutUint16(data[head+50:head+52], 1)

	hhea := int(hheaOffset)
	binary.BigEndian.PutUint16(data[hhea+34:hhea+36], uint16(numGlyphs))

	hmtx := int(hmtxOffset)
	for i, metric := range metrics {
		binary.BigEndian.PutUint16(data[hmtx+i*4:hmtx+i*4+2], metric.advance)
		binary.BigEndian.PutUint16(data[hmtx+i*4+2:hmtx+i*4+4], uint16(metric.lsb))
	}

	loca := int(locaOffset)
	for i, offset := range offsets {
		binary.BigEndian.PutUint32(data[loca+i*4:loca+i*4+4], offset)
	}

	maxp := int(maxpOffset)
	binary.BigEndian.PutUint32(data[maxp:maxp+4], 0x00010000)
	binary.BigEndian.PutUint16(data[maxp+4:maxp+6], uint16(numGlyphs))

	f, err := NewLoader(&mockSystem{}).LoadFace(&mockStream{data: data})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	face := f.(*Face)
	if err := face.SetPixelSizes(1000, 1000); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	return face
}

func loadCFFSlotMetricsTestFace(t *testing.T) *Face {
	t.Helper()
	tables := []sfntTestTable{
		{tag: "head", data: buildMetricsTestHeadTable()},
		{tag: "maxp", data: buildMetricsTestMaxpTable(2)},
		{tag: "hhea", data: buildMetricsTestHheaTable(2)},
		{tag: "hmtx", data: buildMetricsTestHmtxTable([]metricsGlyph{
			{advance: 500, lsb: 0},
			{advance: 300, lsb: 20},
		})},
		{tag: "vhea", data: buildMetricsTestVheaTable(2)},
		{tag: "vmtx", data: buildMetricsTestVmtxTable([]VMetric{
			{AdvanceHeight: 600, TopSideBearing: 10},
			{AdvanceHeight: 700, TopSideBearing: 30},
		})},
		{tag: "VORG", data: buildMetricsTestVORGTable(880, []VertOriginYMetric{
			{GlyphIndex: 1, VertOriginY: 900},
		})},
		{tag: "CFF2", data: buildCFF2VariationTestTable()},
	}
	loaded, err := NewLoader(&mockSystem{}).LoadFace(&mockStream{data: buildSFNTTestData(t, tables)})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	return loaded.(*Face)
}

func buildMetricsTestHeadTable() []byte {
	data := make([]byte, 54)
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint32(data[12:16], 0x5F0F3CF5)
	binary.BigEndian.PutUint16(data[18:20], 1000)
	binary.BigEndian.PutUint16(data[50:52], 1)
	return data
}

func buildMetricsTestMaxpTable(numGlyphs int) []byte {
	data := make([]byte, 32)
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], uint16(numGlyphs))
	return data
}

func buildGlyphNamePostTable() []byte {
	data := make([]byte, 32+2+2*2+1+5)
	binary.BigEndian.PutUint32(data[0:4], 0x00020000)
	binary.BigEndian.PutUint16(data[32:34], 2)
	binary.BigEndian.PutUint16(data[34:36], 258)
	binary.BigEndian.PutUint16(data[36:38], 36)
	data[38] = 5
	copy(data[39:44], "A.alt")
	return data
}

func buildMetricsTestHheaTable(numHMetrics int) []byte {
	data := make([]byte, 36)
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[34:36], uint16(numHMetrics))
	return data
}

func buildMetricsTestHmtxTable(metrics []metricsGlyph) []byte {
	data := make([]byte, len(metrics)*4)
	for i, metric := range metrics {
		binary.BigEndian.PutUint16(data[i*4:i*4+2], metric.advance)
		binary.BigEndian.PutUint16(data[i*4+2:i*4+4], uint16(metric.lsb))
	}
	return data
}

func buildMetricsTestVheaTable(numVMetrics int) []byte {
	data := make([]byte, 36)
	binary.BigEndian.PutUint32(data[0:4], 0x00011000)
	binary.BigEndian.PutUint16(data[34:36], uint16(numVMetrics))
	return data
}

func buildMetricsTestVmtxTable(metrics []VMetric) []byte {
	data := make([]byte, len(metrics)*4)
	for i, metric := range metrics {
		binary.BigEndian.PutUint16(data[i*4:i*4+2], metric.AdvanceHeight)
		binary.BigEndian.PutUint16(data[i*4+2:i*4+4], uint16(metric.TopSideBearing))
	}
	return data
}

func buildMetricsTestVORGTable(defaultOrigin int16, metrics []VertOriginYMetric) []byte {
	data := make([]byte, 8+len(metrics)*4)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], uint16(defaultOrigin))
	binary.BigEndian.PutUint16(data[6:8], uint16(len(metrics)))
	for i, metric := range metrics {
		offset := 8 + i*4
		binary.BigEndian.PutUint16(data[offset:offset+2], metric.GlyphIndex)
		binary.BigEndian.PutUint16(data[offset+2:offset+4], uint16(metric.VertOriginY))
	}
	return data
}

func loadSingleGlyphProgramFace(t *testing.T, glyph []byte) *Face {
	return loadSingleGlyphProgramFaceWithPrograms(t, glyph, nil, make([]byte, 2))
}

func loadSingleGlyphProgramFaceWithPrograms(t *testing.T, glyph []byte, prep []byte, cvt []byte) *Face {
	t.Helper()

	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000)
	binary.BigEndian.PutUint16(head[50:52], 1)

	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:36], 1)

	hmtx := make([]byte, 4)
	binary.BigEndian.PutUint16(hmtx[0:2], 300)
	binary.BigEndian.PutUint16(hmtx[2:4], 20)

	loca := make([]byte, 8)
	binary.BigEndian.PutUint32(loca[4:8], uint32(len(glyph)))

	maxp := make([]byte, 32)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], 1)

	tables := []sfntTestTable{
		{tag: "glyf", data: glyph},
		{tag: "head", data: head},
		{tag: "hhea", data: hhea},
		{tag: "hmtx", data: hmtx},
		{tag: "loca", data: loca},
		{tag: "maxp", data: maxp},
		{tag: "cvt ", data: cvt},
	}
	if prep != nil {
		tables = append(tables, sfntTestTable{tag: "prep", data: prep})
	}

	loaded, err := NewLoader(&mockSystem{}).LoadFace(&mockStream{data: buildSFNTTestData(t, tables)})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	face := loaded.(*Face)
	if err := face.SetPixelSizes(1000, 1000); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	return face
}

func simpleOnePointGlyphData(x, y int16, instructions []byte) []byte {
	data := make([]byte, 19+len(instructions))
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], uint16(x))
	binary.BigEndian.PutUint16(data[4:6], uint16(y))
	binary.BigEndian.PutUint16(data[6:8], uint16(x))
	binary.BigEndian.PutUint16(data[8:10], uint16(y))
	binary.BigEndian.PutUint16(data[10:12], 0)
	binary.BigEndian.PutUint16(data[12:14], uint16(len(instructions)))
	copy(data[14:14+len(instructions)], instructions)
	flagsOffset := 14 + len(instructions)
	data[flagsOffset] = 0x01
	binary.BigEndian.PutUint16(data[flagsOffset+1:flagsOffset+3], uint16(x))
	binary.BigEndian.PutUint16(data[flagsOffset+3:flagsOffset+5], uint16(y))
	return data
}

func simpleRectGlyphData(xMin, yMin, xMax, yMax int16) []byte {
	data := make([]byte, 34)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], uint16(xMin))
	binary.BigEndian.PutUint16(data[4:6], uint16(yMin))
	binary.BigEndian.PutUint16(data[6:8], uint16(xMax))
	binary.BigEndian.PutUint16(data[8:10], uint16(yMax))
	binary.BigEndian.PutUint16(data[10:12], 3)
	binary.BigEndian.PutUint16(data[12:14], 0)

	for i := 0; i < 4; i++ {
		data[14+i] = 0x01
	}

	offset := 18
	for _, dx := range []int16{xMin, xMax - xMin, 0, xMin - xMax} {
		binary.BigEndian.PutUint16(data[offset:offset+2], uint16(dx))
		offset += 2
	}
	for _, dy := range []int16{yMin, 0, yMax - yMin, 0} {
		binary.BigEndian.PutUint16(data[offset:offset+2], uint16(dy))
		offset += 2
	}
	return data
}

type compositeMetricComponent struct {
	glyph uint16
	flags uint16
	dx    int16
	dy    int16
}

func compositeGlyphWithMetricComponents(components []compositeMetricComponent) []byte {
	data := make([]byte, 10+len(components)*8)
	binary.BigEndian.PutUint16(data[0:2], 0xffff)
	offset := 10
	for i, component := range components {
		flags := component.flags | ARG_1_AND_2_ARE_WORDS | ARGS_ARE_XY_VALUES
		if i < len(components)-1 {
			flags |= MORE_COMPONENTS
		}
		binary.BigEndian.PutUint16(data[offset:offset+2], flags)
		binary.BigEndian.PutUint16(data[offset+2:offset+4], component.glyph)
		binary.BigEndian.PutUint16(data[offset+4:offset+6], uint16(component.dx))
		binary.BigEndian.PutUint16(data[offset+6:offset+8], uint16(component.dy))
		offset += 8
	}
	return data
}

type sfntTestTable struct {
	tag  string
	data []byte
}

func align4Test(v uint32) uint32 {
	return (v + 3) &^ 3
}

func buildSFNTTestData(t *testing.T, tables []sfntTestTable) []byte {
	t.Helper()
	headerLen := align4Test(uint32(12 + len(tables)*16))
	data := make([]byte, headerLen)
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], uint16(len(tables)))

	offset := headerLen
	for i, table := range tables {
		if len(table.tag) != 4 {
			t.Fatalf("table tag %q is not 4 bytes", table.tag)
		}
		offset = align4Test(offset)
		for uint32(len(data)) < offset {
			data = append(data, 0)
		}
		tableOffset := offset
		data = append(data, table.data...)

		entry := 12 + i*16
		copy(data[entry:entry+4], table.tag)
		binary.BigEndian.PutUint32(data[entry+8:entry+12], tableOffset)
		binary.BigEndian.PutUint32(data[entry+12:entry+16], uint32(len(table.data)))
		offset = tableOffset + uint32(len(table.data))
	}
	return data
}

type loaderTestImageDecoder struct{}

func (loaderTestImageDecoder) Decode(data []byte) (*api.Image, error) {
	if bytes.Equal(data, []byte("mock_png_data")) {
		return &api.Image{Width: 10, Height: 10, Pixels: []byte{1, 2, 3}}, nil
	}
	return nil, errors.New("invalid data")
}

func testSbixData() []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint16(1))
	binary.Write(buf, binary.BigEndian, uint16(1))
	binary.Write(buf, binary.BigEndian, uint32(1))
	binary.Write(buf, binary.BigEndian, uint32(12))

	binary.Write(buf, binary.BigEndian, uint16(109))
	binary.Write(buf, binary.BigEndian, uint16(72))
	binary.Write(buf, binary.BigEndian, uint32(12))
	binary.Write(buf, binary.BigEndian, uint32(33))

	binary.Write(buf, binary.BigEndian, int16(0))
	binary.Write(buf, binary.BigEndian, int16(0))
	binary.Write(buf, binary.BigEndian, uint32(0x706e6720))
	buf.Write([]byte("mock_png_data"))
	return buf.Bytes()
}

func hasNonZeroByte(buf []byte) bool {
	for _, b := range buf {
		if b != 0 {
			return true
		}
	}
	return false
}

func testF2Dot14Bits(v float32) uint16 {
	return uint16(int16(v * 16384))
}

func buildFvarOneAxisTable() []byte {
	data := make([]byte, 36)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[4:6], 16)
	binary.BigEndian.PutUint16(data[6:8], 2)
	binary.BigEndian.PutUint16(data[8:10], 1)
	binary.BigEndian.PutUint16(data[10:12], 20)
	binary.BigEndian.PutUint16(data[14:16], 4)
	copy(data[16:20], "wght")
	minValue := int32(-1 << 16)
	binary.BigEndian.PutUint32(data[20:24], uint32(minValue))
	binary.BigEndian.PutUint32(data[24:28], 0)
	binary.BigEndian.PutUint32(data[28:32], uint32(int32(1<<16)))
	return data
}

func buildAvarOneAxisHalfToOneTable() []byte {
	data := make([]byte, 26)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[6:8], 1)
	binary.BigEndian.PutUint16(data[8:10], 4)
	offset := 10
	for _, pair := range [][2]float32{
		{-1, -1},
		{0, 0},
		{0.5, 1},
		{1, 1},
	} {
		binary.BigEndian.PutUint16(data[offset:offset+2], testF2Dot14Bits(pair[0]))
		binary.BigEndian.PutUint16(data[offset+2:offset+4], testF2Dot14Bits(pair[1]))
		offset += 4
	}
	return data
}

const (
	testCFFOpCharStrings    = 17
	testCFFOpPrivate        = 18
	testCFFOpVariationStore = 24
	testCFFOpFDArray        = 12<<8 | 36
	testCFFOpFDSelect       = 12<<8 | 37
	testCFFOpPrivateVSIndex = 22
)

func buildCFF2VariationTestTable() []byte {
	n := testCFF2CharStringNumber
	globalSubrs := testCFF2Index32Bytes()
	charStrings := testCFF2Index32Bytes(
		[]byte{n(0), n(0), 21},
		[]byte{n(10), n(20), n(5), n(7), n(-3), n(11), n(2), 16, 21},
	)
	fdSelect := []byte{0, 0, 1}
	private1 := append(testCFF2DictInt32(1), byte(testCFFOpPrivateVSIndex))
	vstore := testCFF2VariationStoreBytes()

	const topDictSize = 26
	charStringsOffset := 5 + topDictSize + len(globalSubrs)
	fdSelectOffset := charStringsOffset + len(charStrings)
	fdArrayOffset := fdSelectOffset + len(fdSelect)

	fd0 := testCFF2DictEntry(testCFFOpPrivate, 0, 0)
	fd1Placeholder := testCFF2DictEntry(testCFFOpPrivate, len(private1), 0)
	fdArrayLen := len(testCFF2Index32Bytes(fd0, fd1Placeholder))
	private1Offset := fdArrayOffset + fdArrayLen
	fd1 := testCFF2DictEntry(testCFFOpPrivate, len(private1), private1Offset)
	fdArray := testCFF2Index32Bytes(fd0, fd1)
	vstoreOffset := private1Offset + len(private1)

	topDict := make([]byte, 0, topDictSize)
	topDict = append(topDict, testCFF2DictEntry(testCFFOpCharStrings, charStringsOffset)...)
	topDict = append(topDict, testCFF2DictEntry(testCFFOpVariationStore, vstoreOffset)...)
	topDict = append(topDict, testCFF2DictEntry(testCFFOpFDArray, fdArrayOffset)...)
	topDict = append(topDict, testCFF2DictEntry(testCFFOpFDSelect, fdSelectOffset)...)

	header := []byte{2, 0, 5, 0, topDictSize}
	data := append([]byte{}, header...)
	data = append(data, topDict...)
	data = append(data, globalSubrs...)
	data = append(data, charStrings...)
	data = append(data, fdSelect...)
	data = append(data, fdArray...)
	data = append(data, private1...)
	data = append(data, vstore...)
	return data
}

func testCFF2CharStringNumber(v int) byte {
	if v < -107 || v > 107 {
		panic("test charstring number out of single-byte range")
	}
	return byte(v + 139)
}

func testCFF2Index32Bytes(objects ...[]byte) []byte {
	total := 0
	for _, obj := range objects {
		total += len(obj)
	}
	if total > 254 {
		panic("test INDEX data too large for offSize 1")
	}

	data := make([]byte, 4, 4+1+len(objects)+1+total)
	binary.BigEndian.PutUint32(data, uint32(len(objects)))
	if len(objects) == 0 {
		return data
	}
	data = append(data, 1)
	next := 1
	data = append(data, byte(next))
	for _, obj := range objects {
		next += len(obj)
		data = append(data, byte(next))
	}
	for _, obj := range objects {
		data = append(data, obj...)
	}
	return data
}

func testCFF2DictInt32(v int) []byte {
	return []byte{
		29,
		byte(v >> 24),
		byte(v >> 16),
		byte(v >> 8),
		byte(v),
	}
}

func testCFF2DictOperator(op int) []byte {
	if op >= 12<<8 {
		return []byte{12, byte(op - (12 << 8))}
	}
	return []byte{byte(op)}
}

func testCFF2DictEntry(op int, values ...int) []byte {
	data := make([]byte, 0, len(values)*5+2)
	for _, v := range values {
		data = append(data, testCFF2DictInt32(v)...)
	}
	return append(data, testCFF2DictOperator(op)...)
}

func testCFF2VariationStoreBytes() []byte {
	itemStore := make([]byte, 0, 50)
	itemStore = append(itemStore, 0, 1)
	itemStore = append(itemStore, 0, 0, 0, 16)
	itemStore = append(itemStore, 0, 2)
	itemStore = append(itemStore, 0, 0, 0, 32)
	itemStore = append(itemStore, 0, 0, 0, 40)

	itemStore = append(itemStore, 0, 1)
	itemStore = append(itemStore, 0, 2)
	itemStore = appendCFF2F2Dot14(itemStore, 0, 1, 1)
	itemStore = appendCFF2F2Dot14(itemStore, 0, 0.5, 1)

	itemStore = append(itemStore, 0, 0)
	itemStore = append(itemStore, 0, 0)
	itemStore = append(itemStore, 0, 1)
	itemStore = append(itemStore, 0, 0)

	itemStore = append(itemStore, 0, 0)
	itemStore = append(itemStore, 0, 0)
	itemStore = append(itemStore, 0, 2)
	itemStore = append(itemStore, 0, 0)
	itemStore = append(itemStore, 0, 1)

	data := make([]byte, 2, 2+len(itemStore))
	binary.BigEndian.PutUint16(data, uint16(len(itemStore)))
	return append(data, itemStore...)
}

func appendCFF2F2Dot14(data []byte, values ...float64) []byte {
	for _, v := range values {
		raw := int16(v * 16384)
		data = append(data, byte(raw>>8), byte(raw))
	}
	return data
}

func buildCvarOneAxisDeltaTable(delta int8) []byte {
	data := make([]byte, 16)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint16(data[6:8], 14)
	binary.BigEndian.PutUint16(data[8:10], 2)
	binary.BigEndian.PutUint16(data[10:12], 0x8000)
	binary.BigEndian.PutUint16(data[12:14], testF2Dot14Bits(1))
	data[14] = 0
	data[15] = byte(delta)
	return data
}

type mvarTestRecord struct {
	tag   string
	delta int16
}

func buildMvarOneAxisTable(records []mvarTestRecord) []byte {
	headerLen := 12 + len(records)*8
	data := make([]byte, headerLen+26+8+len(records)*2)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[6:8], 8)
	binary.BigEndian.PutUint16(data[8:10], uint16(len(records)))
	binary.BigEndian.PutUint16(data[10:12], uint16(headerLen))
	for i, record := range records {
		off := 12 + i*8
		copy(data[off:off+4], record.tag)
		binary.BigEndian.PutUint16(data[off+6:off+8], uint16(i))
	}

	ivs := headerLen
	binary.BigEndian.PutUint16(data[ivs:ivs+2], 1)
	binary.BigEndian.PutUint32(data[ivs+2:ivs+6], 16)
	binary.BigEndian.PutUint16(data[ivs+6:ivs+8], 1)
	binary.BigEndian.PutUint32(data[ivs+8:ivs+12], 26)

	regionList := ivs + 16
	binary.BigEndian.PutUint16(data[regionList:regionList+2], 1)
	binary.BigEndian.PutUint16(data[regionList+2:regionList+4], 1)
	binary.BigEndian.PutUint16(data[regionList+6:regionList+8], testF2Dot14Bits(1))
	binary.BigEndian.PutUint16(data[regionList+8:regionList+10], testF2Dot14Bits(1))

	variationData := ivs + 26
	binary.BigEndian.PutUint16(data[variationData:variationData+2], uint16(len(records)))
	binary.BigEndian.PutUint16(data[variationData+2:variationData+4], 1)
	binary.BigEndian.PutUint16(data[variationData+4:variationData+6], 1)
	deltaOffset := variationData + 8
	for i, record := range records {
		binary.BigEndian.PutUint16(data[deltaOffset+i*2:deltaOffset+i*2+2], uint16(record.delta))
	}
	return data
}

func buildGvarOneGlyphDeltasTable() []byte {
	glyphData := []byte{
		0, 1, // tupleVariationCount
		0, 10, // dataOffset
		0, 7, // variationDataSize
		0x80, 0, // embedded peak tuple
		0x40, 0, // peak = 1.0
		4, 10, 0, 30, 0, 0, // five X byte deltas: contour, left, right, top, bottom
		0x84, // five zero Y deltas
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
	return append(data, glyphData...)
}

func buildGvarCompositePseudoDeltasTable() []byte {
	glyphData := []byte{
		0, 1, // tupleVariationCount
		0, 10, // dataOffset
		0, 8, // variationDataSize
		0x80, 0, // embedded peak tuple
		0x40, 0, // peak = 1.0
		5, 10, 246, 0, 40, 0, 0, // component0 +10, component1 -10, right phantom +40
		0x85, // six zero Y deltas
	}
	if len(glyphData)%2 != 0 {
		glyphData = append(glyphData, 0)
	}

	data := make([]byte, 28)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint16(data[12:14], 3)
	binary.BigEndian.PutUint32(data[16:20], 28)
	endOffset := uint16(len(glyphData) / 2)
	binary.BigEndian.PutUint16(data[22:24], endOffset)
	binary.BigEndian.PutUint16(data[24:26], endOffset)
	binary.BigEndian.PutUint16(data[26:28], endOffset)
	return append(data, glyphData...)
}

func buildHvarAdvanceDeltaTable(delta int16) []byte {
	data := make([]byte, 56)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint32(data[4:8], 20)

	ivs := 20
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
	binary.BigEndian.PutUint16(data[variationData+8:variationData+10], uint16(delta))
	return data
}

func loadVariableGlyphTestFace(t *testing.T, gvar []byte, hvar []byte) *Face {
	return loadVariableGlyphTestFaceWithTables(t, gvar, hvar)
}

func loadVariableGlyphTestFaceWithTables(t *testing.T, gvar []byte, hvar []byte, extraTables ...sfntTestTable) *Face {
	t.Helper()
	glyph := simpleOnePointGlyphData(50, 0, nil)

	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000)
	binary.BigEndian.PutUint16(head[50:52], 1)

	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:36], 1)

	hmtx := make([]byte, 4)
	binary.BigEndian.PutUint16(hmtx[0:2], 300)
	binary.BigEndian.PutUint16(hmtx[2:4], 20)

	loca := make([]byte, 8)
	binary.BigEndian.PutUint32(loca[4:8], uint32(len(glyph)))

	maxp := make([]byte, 32)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], 1)

	tables := []sfntTestTable{
		{tag: "glyf", data: glyph},
		{tag: "head", data: head},
		{tag: "hhea", data: hhea},
		{tag: "hmtx", data: hmtx},
		{tag: "loca", data: loca},
		{tag: "maxp", data: maxp},
		{tag: "fvar", data: buildFvarOneAxisTable()},
	}
	if gvar != nil {
		tables = append(tables, sfntTestTable{tag: "gvar", data: gvar})
	}
	if hvar != nil {
		tables = append(tables, sfntTestTable{tag: "HVAR", data: hvar})
	}
	tables = append(tables, extraTables...)

	loaded, err := NewLoader(&mockSystem{}).LoadFace(&mockStream{data: buildSFNTTestData(t, tables)})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	face := loaded.(*Face)
	if err := face.SetPixelSizes(1000, 1000); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	return face
}

func TestLoadGlyphAppliesGvarDeltas(t *testing.T) {
	face := loadVariableGlyphTestFace(t, buildGvarOneGlyphDeltasTable(), nil)
	if err := face.SetVariationNormalizedCoordinates([]float32{1}); err != nil {
		t.Fatalf("SetVariationNormalizedCoordinates failed: %v", err)
	}

	advance, lsb, err := face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 300<<6 || lsb != 20<<6 {
		t.Fatalf("pre-load metrics got advance=%d lsb=%d, want %d %d", advance, lsb, 300<<6, 20<<6)
	}

	slot, err := face.LoadGlyph(0, api.LoadNoHinting)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	points := slot.GetOutline().GetPoints()
	if len(points) != 1 {
		t.Fatalf("point count = %d, want 1", len(points))
	}
	if got, want := points[0].X, int32(30<<6); got != want {
		t.Fatalf("gvar-adjusted point X = %d, want %d", got, want)
	}

	advance, lsb, err = face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics after LoadGlyph failed: %v", err)
	}
	if advance != 330<<6 || lsb != 30<<6 {
		t.Fatalf("gvar-adjusted metrics got advance=%d lsb=%d, want %d %d", advance, lsb, 330<<6, 30<<6)
	}
}

func TestVariationCoordinatesApplyHVARAdvanceAndClearMetricsCache(t *testing.T) {
	face := loadVariableGlyphTestFace(t, nil, buildHvarAdvanceDeltaTable(40))
	if err := face.SetVariationNormalizedCoordinates([]float32{1}); err != nil {
		t.Fatalf("SetVariationNormalizedCoordinates failed: %v", err)
	}

	advance, lsb, err := face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 340<<6 || lsb != 20<<6 {
		t.Fatalf("HVAR metrics got advance=%d lsb=%d, want %d %d", advance, lsb, 340<<6, 20<<6)
	}

	if _, err := face.LoadGlyph(0, api.LoadNoHinting); err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	if err := face.SetVariationNormalizedCoordinates([]float32{0}); err != nil {
		t.Fatalf("reset variation coordinates failed: %v", err)
	}

	advance, lsb, err = face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics after reset failed: %v", err)
	}
	if advance != 300<<6 || lsb != 20<<6 {
		t.Fatalf("reset metrics got advance=%d lsb=%d, want %d %d", advance, lsb, 300<<6, 20<<6)
	}
}

func TestVariationDesignCoordinatesUseLoadedAvarMapping(t *testing.T) {
	face := loadVariableGlyphTestFaceWithTables(
		t,
		nil,
		buildHvarAdvanceDeltaTable(40),
		sfntTestTable{tag: "avar", data: buildAvarOneAxisHalfToOneTable()},
	)
	if err := face.SetVariationDesignCoordinates([]ftvar.Fixed{ftvar.FloatToFixed(0.5)}); err != nil {
		t.Fatalf("SetVariationDesignCoordinates failed: %v", err)
	}

	advance, lsb, err := face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 340<<6 || lsb != 20<<6 {
		t.Fatalf("avar-adjusted HVAR metrics got advance=%d lsb=%d, want %d %d", advance, lsb, 340<<6, 20<<6)
	}
}

func TestMVARMetricDeltasRefreshWithVariationCoordinates(t *testing.T) {
	mvar, err := ftvar.ParseMVAR(&mockStream{data: buildMvarOneAxisTable([]mvarTestRecord{
		{tag: "cpht", delta: 4},
		{tag: "hasc", delta: 20},
		{tag: "hcof", delta: 2},
		{tag: "unds", delta: 3},
		{tag: "vasc", delta: 10},
	})})
	if err != nil {
		t.Fatalf("ParseMVAR failed: %v", err)
	}
	ve := ftvar.NewVariationEngine(&ftvar.FvarTable{
		Axes: []ftvar.AxisRecord{{Tag: stringToTag("wght"), MinValue: -1 << 16, MaxValue: 1 << 16}},
	}, nil, nil, nil)
	ve.SetMVAR(mvar)

	baseHhea := HheaTable{CaretOffset: 5}
	baseVhea := VheaTable{Ascent: 700}
	baseOS2 := OS2Table{STypoAscender: 800, SCapHeight: 600}
	basePost := PostTable{UnderlineThickness: 50}
	face := &Face{
		varEngine:   ve,
		hhea:        baseHhea,
		baseHhea:    baseHhea,
		hasBaseHhea: true,
		vhea:        baseVhea,
		baseVhea:    baseVhea,
		hasBaseVhea: true,
		os2:         baseOS2,
		baseOS2:     baseOS2,
		hasBaseOS2:  true,
		post:        basePost,
		basePost:    basePost,
		hasBasePost: true,
	}

	if err := face.SetVariationNormalizedCoordinates([]float32{1}); err != nil {
		t.Fatalf("SetVariationNormalizedCoordinates failed: %v", err)
	}
	if face.os2.STypoAscender != 820 || face.os2.SCapHeight != 604 {
		t.Fatalf("OS/2 MVAR metrics = ascender %d capHeight %d, want 820 604", face.os2.STypoAscender, face.os2.SCapHeight)
	}
	if face.vhea.Ascent != 710 {
		t.Fatalf("vhea ascent = %d, want 710", face.vhea.Ascent)
	}
	if face.hhea.CaretOffset != 7 {
		t.Fatalf("hhea caret offset = %d, want 7", face.hhea.CaretOffset)
	}
	if face.post.UnderlineThickness != 53 {
		t.Fatalf("post underline thickness = %d, want 53", face.post.UnderlineThickness)
	}

	if err := face.SetVariationNormalizedCoordinates([]float32{0}); err != nil {
		t.Fatalf("reset variation coordinates failed: %v", err)
	}
	if face.os2.STypoAscender != 800 || face.vhea.Ascent != 700 || face.hhea.CaretOffset != 5 || face.post.UnderlineThickness != 50 {
		t.Fatalf("MVAR reset did not restore base metrics: os2=%+v vhea=%+v hhea=%+v post=%+v", face.os2, face.vhea, face.hhea, face.post)
	}
}

func TestCompositeGlyphAppliesGvarPseudoPointDeltas(t *testing.T) {
	composite := compositeGlyphWithMetricComponents([]compositeMetricComponent{
		{glyph: 1},
		{glyph: 2},
	})
	component1 := simpleOnePointGlyphData(30, 0, nil)
	component2 := simpleOnePointGlyphData(90, 0, nil)
	glyf := append(append(append([]byte{}, composite...), component1...), component2...)

	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000)
	binary.BigEndian.PutUint16(head[50:52], 1)

	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:36], 3)

	hmtx := make([]byte, 12)
	binary.BigEndian.PutUint16(hmtx[0:2], 100)
	binary.BigEndian.PutUint16(hmtx[2:4], 5)
	binary.BigEndian.PutUint16(hmtx[4:6], 300)
	binary.BigEndian.PutUint16(hmtx[6:8], 30)
	binary.BigEndian.PutUint16(hmtx[8:10], 700)
	binary.BigEndian.PutUint16(hmtx[10:12], 70)

	loca := make([]byte, 16)
	binary.BigEndian.PutUint32(loca[4:8], uint32(len(composite)))
	binary.BigEndian.PutUint32(loca[8:12], uint32(len(composite)+len(component1)))
	binary.BigEndian.PutUint32(loca[12:16], uint32(len(glyf)))

	maxp := make([]byte, 32)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], 3)

	tables := []sfntTestTable{
		{tag: "glyf", data: glyf},
		{tag: "head", data: head},
		{tag: "hhea", data: hhea},
		{tag: "hmtx", data: hmtx},
		{tag: "loca", data: loca},
		{tag: "maxp", data: maxp},
		{tag: "fvar", data: buildFvarOneAxisTable()},
		{tag: "gvar", data: buildGvarCompositePseudoDeltasTable()},
	}
	loaded, err := NewLoader(&mockSystem{}).LoadFace(&mockStream{data: buildSFNTTestData(t, tables)})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	face := loaded.(*Face)
	if err := face.SetPixelSizes(1000, 1000); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}
	if err := face.SetVariationNormalizedCoordinates([]float32{1}); err != nil {
		t.Fatalf("SetVariationNormalizedCoordinates failed: %v", err)
	}

	slot, err := face.LoadGlyph(0, api.LoadNoHinting)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	points := slot.GetOutline().GetPoints()
	if len(points) != 2 {
		t.Fatalf("composite point count = %d, want 2", len(points))
	}
	if got, want := points[0].X, int32(45<<6); got != want {
		t.Fatalf("component 0 X = %d, want %d", got, want)
	}
	if got, want := points[1].X, int32(85<<6); got != want {
		t.Fatalf("component 1 X = %d, want %d", got, want)
	}

	advance, lsb, err := face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 140<<6 || lsb != 45<<6 {
		t.Fatalf("composite gvar metrics got advance=%d lsb=%d, want %d %d", advance, lsb, 140<<6, 45<<6)
	}
}

func TestLoadGlyphUnhintedMetricsUseBBoxLSBPhantoms(t *testing.T) {
	glyph := simpleOnePointGlyphData(50, 0, nil)
	face := loadGlyphMetricsTestFace(t, []uint32{0, uint32(len(glyph))}, glyph, []metricsGlyph{
		{advance: 300, lsb: 20},
	})

	advance, lsb, err := face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 300<<6 || lsb != 20<<6 {
		t.Fatalf("table metrics got advance=%d lsb=%d, want %d %d", advance, lsb, 300<<6, 20<<6)
	}

	slot, err := face.LoadGlyph(0, api.LoadNoHinting)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	points := slot.GetOutline().GetPoints()
	if len(points) != 1 {
		t.Fatalf("external outline point count = %d, want 1", len(points))
	}
	if got, want := points[0].X, int32(20<<6); got != want {
		t.Fatalf("origin-translated point X = %d, want %d", got, want)
	}

	advance, lsb, err = face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics after LoadGlyph failed: %v", err)
	}
	if advance != 300<<6 || lsb != 20<<6 {
		t.Fatalf("loaded metrics got advance=%d lsb=%d, want %d %d", advance, lsb, 300<<6, 20<<6)
	}
}

func TestLoadGlyphHintedPhantomAdvanceUpdatesMetrics(t *testing.T) {
	instructions := []byte{
		0xB1, 2, 64, // PUSHB[2] point 2, 1px distance
		0x38, // SHPIX
	}
	glyph := simpleOnePointGlyphData(20, 0, instructions)
	face := loadGlyphMetricsTestFace(t, []uint32{0, uint32(len(glyph))}, glyph, []metricsGlyph{
		{advance: 300, lsb: 20},
	})

	slot, err := face.LoadGlyph(0, api.LoadTargetMono)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	points := slot.GetOutline().GetPoints()
	if len(points) != 1 {
		t.Fatalf("external outline point count = %d, want 1", len(points))
	}

	advance, lsb, err := face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 301<<6 || lsb != 20<<6 {
		t.Fatalf("hinted metrics got advance=%d lsb=%d, want %d %d", advance, lsb, 301<<6, 20<<6)
	}
	if concrete, ok := slot.(*GlyphSlot); !ok || !concrete.hasMetrics || concrete.metrics.advance != 301<<6 {
		t.Fatalf("slot metrics were not updated from hinted phantoms: %#v", slot)
	}
}

func TestLoadGlyphHintedMetricsArePixelFitted(t *testing.T) {
	glyph := simpleRectGlyphData(50, -150, 550, 700)
	face := loadGlyphMetricsTestFace(t, []uint32{0, uint32(len(glyph))}, glyph, []metricsGlyph{
		{advance: 755, lsb: 50},
	})
	if err := face.SetPixelSizes(12, 12); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}

	slot, err := face.LoadGlyph(0, api.LoadDefault)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	metrics, ok := api.GetGlyphSlotMetrics(slot)
	if !ok {
		t.Fatal("glyph slot metrics unavailable")
	}

	scaledMinX := face.scaleFUnitsX(50)
	scaledMinY := face.scaleFUnitsY(-150)
	scaledMaxX := face.scaleFUnitsX(550)
	scaledMaxY := face.scaleFUnitsY(700)
	wantBearingX := floorToPixel26Dot6(scaledMinX)
	wantBearingY := ceilToPixel26Dot6(scaledMaxY)
	wantWidth := ceilToPixel26Dot6(scaledMaxX) - wantBearingX
	wantHeight := wantBearingY - floorToPixel26Dot6(scaledMinY)
	wantAdvance := roundToPixel26Dot6(face.scaleFUnitsX(755))

	if metrics.Width != wantWidth || metrics.Height != wantHeight {
		t.Fatalf("pixel-fitted bounds got width=%d height=%d, want %d %d", metrics.Width, metrics.Height, wantWidth, wantHeight)
	}
	if metrics.HoriBearingX != wantBearingX || metrics.HoriBearingY != wantBearingY || metrics.HoriAdvance != wantAdvance {
		t.Fatalf("pixel-fitted horizontal metrics mismatch: %+v, want bearingX=%d bearingY=%d advance=%d",
			metrics, wantBearingX, wantBearingY, wantAdvance)
	}

	advance, lsb, err := face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != wantAdvance || lsb != wantBearingX {
		t.Fatalf("loaded hinted metrics got advance=%d lsb=%d, want %d %d", advance, lsb, wantAdvance, wantBearingX)
	}
}

func TestGlyphSlotMetricsProviderUsesLoadedPhantoms(t *testing.T) {
	glyph := simpleRectGlyphData(50, -10, 90, 40)
	face := loadGlyphMetricsTestFace(t, []uint32{0, uint32(len(glyph))}, glyph, []metricsGlyph{
		{advance: 300, lsb: 20},
	})

	slot, err := face.LoadGlyph(0, api.LoadNoHinting)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}

	metrics, ok := api.GetGlyphSlotMetrics(slot)
	if !ok {
		t.Fatal("glyph slot metrics unavailable")
	}
	if metrics.Width != 40<<6 || metrics.Height != 50<<6 {
		t.Fatalf("slot bounds got width=%d height=%d, want %d %d", metrics.Width, metrics.Height, 40<<6, 50<<6)
	}
	if metrics.HoriBearingX != 20<<6 || metrics.HoriBearingY != 40<<6 || metrics.HoriAdvance != 300<<6 {
		t.Fatalf("horizontal slot metrics mismatch: %+v", metrics)
	}
	if metrics.VertAdvance != 1000<<6 {
		t.Fatalf("vertical advance = %d, want %d", metrics.VertAdvance, 1000<<6)
	}
	if metrics.VertBearingX != (20<<6)-(300<<6)/2 {
		t.Fatalf("synthetic vertical bearing X = %d", metrics.VertBearingX)
	}
	if metrics.VertBearingY != ((1000<<6)-(50<<6))/2 {
		t.Fatalf("synthetic vertical bearing Y = %d", metrics.VertBearingY)
	}
}

func TestGlyphSlotMetricsUseExplicitVerticalMetrics(t *testing.T) {
	glyph := simpleRectGlyphData(50, -10, 90, 40)
	face := loadGlyphMetricsTestFace(t, []uint32{0, uint32(len(glyph))}, glyph, []metricsGlyph{
		{advance: 300, lsb: 20},
	})
	face.vhea.NumOfLongVerMetrics = 1
	face.vmtx = VmtxTable{VMetrics: []VMetric{{AdvanceHeight: 700, TopSideBearing: 30}}}

	slot, err := face.LoadGlyph(0, api.LoadNoHinting|api.LoadVerticalLayout)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	metrics, ok := api.GetGlyphSlotMetrics(slot)
	if !ok {
		t.Fatal("glyph slot metrics unavailable")
	}
	if metrics.VertAdvance != 700<<6 {
		t.Fatalf("vertical advance = %d, want %d", metrics.VertAdvance, 700<<6)
	}
	if metrics.VertBearingX != (20<<6)-(300<<6)/2 {
		t.Fatalf("vertical bearing X = %d, want %d", metrics.VertBearingX, (20<<6)-(300<<6)/2)
	}
	if metrics.VertBearingY != 30<<6 {
		t.Fatalf("vertical bearing Y = %d, want %d", metrics.VertBearingY, 30<<6)
	}
}

func TestGlyphSlotMetricsExplicitVerticalNoScaleStaysUnscaled(t *testing.T) {
	glyph := simpleRectGlyphData(50, -10, 90, 40)
	face := loadGlyphMetricsTestFace(t, []uint32{0, uint32(len(glyph))}, glyph, []metricsGlyph{
		{advance: 300, lsb: 20},
	})
	face.vhea.NumOfLongVerMetrics = 1
	face.vmtx = VmtxTable{VMetrics: []VMetric{{AdvanceHeight: 701, TopSideBearing: 31}}}
	if err := face.SetPixelSizes(333, 333); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}

	scaledSlot, err := face.LoadGlyph(0, api.LoadNoHinting)
	if err != nil {
		t.Fatalf("LoadGlyph scaled failed: %v", err)
	}
	scaledMetrics, ok := api.GetGlyphSlotMetrics(scaledSlot)
	if !ok {
		t.Fatal("scaled glyph slot metrics unavailable")
	}
	if scaledMetrics.VertAdvance != face.scaleFUnitsY(701) || scaledMetrics.VertBearingY != face.scaleFUnitsY(31) {
		t.Fatalf("scaled vertical metrics mismatch: %+v", scaledMetrics)
	}

	noScaleSlot, err := face.LoadGlyph(0, api.LoadNoScale|api.LoadVerticalLayout)
	if err != nil {
		t.Fatalf("LoadGlyph no-scale failed: %v", err)
	}
	noScaleMetrics, ok := api.GetGlyphSlotMetrics(noScaleSlot)
	if !ok {
		t.Fatal("no-scale glyph slot metrics unavailable")
	}
	if noScaleMetrics.VertAdvance != 701<<6 || noScaleMetrics.VertBearingY != 31<<6 {
		t.Fatalf("no-scale vertical metrics mismatch: %+v", noScaleMetrics)
	}
	if noScaleMetrics.VertBearingX != (20<<6)-(300<<6)/2 {
		t.Fatalf("no-scale vertical bearing X = %d, want %d", noScaleMetrics.VertBearingX, (20<<6)-(300<<6)/2)
	}
}

func TestCFFSlotMetricsUseVORGVerticalOrigin(t *testing.T) {
	face := loadCFFSlotMetricsTestFace(t)
	if err := face.SetPixelSizes(500, 500); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}

	slot, err := face.LoadGlyph(1, api.LoadNoScale|api.LoadVerticalLayout)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	metrics, ok := api.GetGlyphSlotMetrics(slot)
	if !ok {
		t.Fatal("glyph slot metrics unavailable")
	}
	if metrics.HoriBearingX != 20<<6 || metrics.HoriAdvance != 300<<6 {
		t.Fatalf("horizontal CFF slot metrics mismatch: %+v", metrics)
	}
	if metrics.VertAdvance != 700<<6 {
		t.Fatalf("CFF vertical advance = %d, want %d", metrics.VertAdvance, 700<<6)
	}
	if metrics.VertBearingX != (20<<6)-(300<<6)/2 {
		t.Fatalf("CFF vertical bearing X = %d, want %d", metrics.VertBearingX, (20<<6)-(300<<6)/2)
	}
	if metrics.VertBearingY != (900<<6)-metrics.HoriBearingY {
		t.Fatalf("CFF VORG vertical bearing Y = %d, want %d", metrics.VertBearingY, (900<<6)-metrics.HoriBearingY)
	}
}

func TestLoadGlyphNoScaleLeavesMetricsUnscaledAndAppliesOriginTranslation(t *testing.T) {
	glyph := simpleRectGlyphData(50, 0, 90, 40)
	face := loadGlyphMetricsTestFace(t, []uint32{0, uint32(len(glyph))}, glyph, []metricsGlyph{
		{advance: 300, lsb: 20},
	})
	if err := face.SetPixelSizes(500, 500); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}

	slot, err := face.LoadGlyph(0, api.LoadNoScale)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	points := slot.GetOutline().GetPoints()
	if got, want := points[0].X, int32(20<<6); got != want {
		t.Fatalf("NoScale point X = %d, want %d", got, want)
	}

	metrics, ok := api.GetGlyphSlotMetrics(slot)
	if !ok {
		t.Fatal("glyph slot metrics unavailable")
	}
	if metrics.Width != 40<<6 || metrics.HoriBearingX != 20<<6 || metrics.HoriAdvance != 300<<6 {
		t.Fatalf("NoScale slot metrics mismatch: %+v", metrics)
	}

	advance, lsb, err := face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics failed: %v", err)
	}
	if advance != 150<<6 || lsb != 10<<6 {
		t.Fatalf("NoScale load polluted scaled metrics: advance=%d lsb=%d", advance, lsb)
	}
}

func TestLoadGlyphRenderStoresBitmap(t *testing.T) {
	glyph := simpleRectGlyphData(0, 0, 10, 10)
	face := loadGlyphMetricsTestFace(t, []uint32{0, uint32(len(glyph))}, glyph, []metricsGlyph{
		{advance: 12, lsb: 0},
	})

	slot, err := face.LoadGlyph(0, api.LoadRender|api.LoadNoHinting|api.LoadTargetMono)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	bitmap := slot.GetBitmap()
	if bitmap == nil {
		t.Fatal("rendered bitmap is nil")
	}
	if bitmap.GetPixelMode() != api.MODE_MONO {
		t.Fatalf("bitmap pixel mode = %d, want MODE_MONO", bitmap.GetPixelMode())
	}
	if bitmap.GetWidth() != 10 || bitmap.GetRows() != 10 || bitmap.GetPitch() != 2 {
		t.Fatalf("bitmap geometry got width=%d rows=%d pitch=%d", bitmap.GetWidth(), bitmap.GetRows(), bitmap.GetPitch())
	}
	if left, top, ok := api.GetBitmapPlacement(bitmap); !ok || left != 0 || top != 10 {
		t.Fatalf("bitmap placement got left=%d top=%d ok=%v, want 0 10 true", left, top, ok)
	}
	if !hasNonZeroByte(bitmap.GetBuffer()) {
		t.Fatal("rendered bitmap buffer is empty")
	}
}

func TestLoadGlyphRenderModesUseFreeTypeBitmapSurfaces(t *testing.T) {
	glyph := simpleRectGlyphData(-4, -2, 6, 8)
	tests := []struct {
		name      string
		flags     int
		width     int
		rows      int
		pitch     int
		pixelMode uint8
		left      int
		top       int
	}{
		{name: "normal", flags: api.LoadRender | api.LoadNoHinting, width: 10, rows: 10, pitch: 10, pixelMode: api.MODE_GRAY, left: -4, top: 8},
		{name: "lcd", flags: api.LoadRender | api.LoadNoHinting | api.LoadTargetLCD, width: 36, rows: 10, pitch: 36, pixelMode: api.MODE_LCD, left: -5, top: 8},
		{name: "lcd-v", flags: api.LoadRender | api.LoadNoHinting | api.LoadTargetLCDV, width: 10, rows: 36, pitch: 10, pixelMode: api.MODE_LCD_V, left: -4, top: 9},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			face := loadGlyphMetricsTestFace(t, []uint32{0, uint32(len(glyph))}, glyph, []metricsGlyph{
				{advance: 12, lsb: -4},
			})
			slot, err := face.LoadGlyph(0, tc.flags)
			if err != nil {
				t.Fatalf("LoadGlyph failed: %v", err)
			}
			bitmap := slot.GetBitmap()
			if bitmap == nil {
				t.Fatal("rendered bitmap is nil")
			}
			if bitmap.GetWidth() != tc.width || bitmap.GetRows() != tc.rows || bitmap.GetPitch() != tc.pitch || bitmap.GetPixelMode() != tc.pixelMode {
				t.Fatalf("bitmap got width=%d rows=%d pitch=%d mode=%d, want %d %d %d %d",
					bitmap.GetWidth(), bitmap.GetRows(), bitmap.GetPitch(), bitmap.GetPixelMode(), tc.width, tc.rows, tc.pitch, tc.pixelMode)
			}
			if left, top, ok := api.GetBitmapPlacement(bitmap); !ok || left != tc.left || top != tc.top {
				t.Fatalf("bitmap placement got left=%d top=%d ok=%v, want %d %d true", left, top, ok, tc.left, tc.top)
			}
		})
	}
}

func TestLoadGlyphPassesTagsToVM(t *testing.T) {
	glyph := simpleOnePointGlyphData(50, 0, []byte{
		0xB0, 0, // PUSHB point 0
		0x80, // FLIPPT
	})
	face := loadSingleGlyphProgramFace(t, glyph)

	slot, err := face.LoadGlyph(0, api.LoadTargetMono)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	tags := slot.GetOutline().GetTags()
	if len(tags) != 1 {
		t.Fatalf("tag count = %d, want 1", len(tags))
	}
	if tags[0]&0x01 != 0 {
		t.Fatalf("FLIPPT did not update glyph tag: %#x", tags[0])
	}
}

func TestLoadGlyphPropagatesScanModeTagFromVM(t *testing.T) {
	glyph := simpleOnePointGlyphData(50, 0, []byte{
		0xB0, 0x02, // PUSHB scan type 2
		0x8D,       // SCANTYPE
		0xB0, 0xFF, // PUSHB always enable scan conversion control
		0x85, // SCANCTRL
	})
	face := loadSingleGlyphProgramFace(t, glyph)

	slot, err := face.LoadGlyph(0, api.LoadTargetMono)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	tags := slot.GetOutline().GetTags()
	if len(tags) != 1 {
		t.Fatalf("tag count = %d, want 1", len(tags))
	}
	if got, want := tags[0], byte(0x45); got != want {
		t.Fatalf("scan mode tag = %#x, want %#x", got, want)
	}
}

func TestLoadGlyphDoesNotSetScanModeTagWhenScanControlDisabled(t *testing.T) {
	glyph := simpleOnePointGlyphData(50, 0, []byte{
		0xB0, 0x02, // PUSHB scan type 2
		0x8D, // SCANTYPE
	})
	face := loadSingleGlyphProgramFace(t, glyph)

	slot, err := face.LoadGlyph(0, api.LoadTargetMono)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	tags := slot.GetOutline().GetTags()
	if len(tags) != 1 {
		t.Fatalf("tag count = %d, want 1", len(tags))
	}
	if got, want := tags[0], byte(0x01); got != want {
		t.Fatalf("scan mode tag = %#x, want %#x", got, want)
	}
}

func TestLoadGlyphPassesRenderTargetToVM(t *testing.T) {
	glyph := simpleOnePointGlyphData(50, 0, []byte{
		0xB0, 0, // PUSHB CVT index
		0xB0, 0x20, // PUSHB GETINFO grayscale selector
		0x88, // GETINFO
		0x44, // WCVTP
	})
	face := loadSingleGlyphProgramFace(t, glyph)

	if _, err := face.LoadGlyph(0, api.LoadTargetLCD); err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	if got, want := face.scaledCVT[0], int32(1<<12); got != want {
		t.Fatalf("GETINFO grayscale CVT[0] = %d, want %d", got, want)
	}
}

func TestLoadGlyphUsesPrepGraphicsState(t *testing.T) {
	glyph := simpleOnePointGlyphData(50, 20, []byte{
		0xB0, 0, // PUSHB point 0
		0xB0, 0, // PUSHB CVT index 0
		0x3E, // MIAP[0], using the projection vector inherited from prep
	})
	cvt := make([]byte, 2)
	binary.BigEndian.PutUint16(cvt, 100)
	face := loadSingleGlyphProgramFaceWithPrograms(t, glyph, []byte{0x00}, cvt) // prep: SVTCA[y]

	slot, err := face.LoadGlyph(0, api.LoadTargetMono)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	points := slot.GetOutline().GetPoints()
	if len(points) != 1 {
		t.Fatalf("outline point count = %d, want 1", len(points))
	}
	if got, want := points[0].X, int32(20<<6); got != want {
		t.Fatalf("prep-inherited X = %d, want %d", got, want)
	}
	if got, want := points[0].Y, int32(100<<6); got != want {
		t.Fatalf("prep-inherited Y = %d, want %d", got, want)
	}
}

func TestLoadGlyphNoBitmapSkipsEmbeddedImages(t *testing.T) {
	sys := core.NewSystem()
	sys.SetImageDecoder(loaderTestImageDecoder{})
	sbix, err := parseSbix(&mockStream{data: testSbixData()})
	if err != nil {
		t.Fatalf("parseSbix failed: %v", err)
	}
	face := &Face{sys: sys, sbix: &sbix}

	slot, err := face.LoadGlyph(0, 0)
	if err != nil {
		t.Fatalf("LoadGlyph with bitmap failed: %v", err)
	}
	if slot.GetImage() == nil {
		t.Fatal("expected embedded image")
	}
	if _, err := face.LoadGlyph(0, api.LoadNoBitmap); err == nil {
		t.Fatal("expected LoadNoBitmap to skip embedded image and expose missing outline error")
	}
}

func TestCompositeUseMyMetricsUsesComponentPhantoms(t *testing.T) {
	composite := compositeGlyphWithMetricComponents([]compositeMetricComponent{
		{glyph: 1},
		{glyph: 2, flags: USE_MY_METRICS, dx: 8},
	})
	component1 := simpleOnePointGlyphData(30, 0, nil)
	component2 := simpleOnePointGlyphData(90, 0, nil)
	glyf := append(append(append([]byte{}, composite...), component1...), component2...)
	offsets := []uint32{0, uint32(len(composite)), uint32(len(composite) + len(component1)), uint32(len(glyf))}
	face := loadGlyphMetricsTestFace(t, offsets, glyf, []metricsGlyph{
		{advance: 100, lsb: 5},
		{advance: 300, lsb: 30},
		{advance: 700, lsb: 70},
	})

	advance, lsb, err := face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics before LoadGlyph failed: %v", err)
	}
	if advance != 700<<6 || lsb != 70<<6 {
		t.Fatalf("pre-load composite metrics got advance=%d lsb=%d, want %d %d", advance, lsb, 700<<6, 70<<6)
	}

	slot, err := face.LoadGlyph(0, api.LoadNoHinting)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	points := slot.GetOutline().GetPoints()
	if len(points) != 2 {
		t.Fatalf("external composite outline point count = %d, want 2", len(points))
	}

	advance, lsb, err = face.GetGlyphMetrics(0)
	if err != nil {
		t.Fatalf("GetGlyphMetrics after LoadGlyph failed: %v", err)
	}
	if advance != 700<<6 || lsb != 10<<6 {
		t.Fatalf("USE_MY_METRICS got advance=%d lsb=%d, want %d %d", advance, lsb, 700<<6, 10<<6)
	}
	metrics, ok := api.GetGlyphSlotMetrics(slot)
	if !ok {
		t.Fatal("glyph slot metrics unavailable")
	}
	if metrics.HoriAdvance != 700<<6 || metrics.HoriBearingX != 10<<6 {
		t.Fatalf("USE_MY_METRICS slot metrics mismatch: %+v", metrics)
	}
	if metrics.VertBearingX != (10<<6)-(700<<6)/2 {
		t.Fatalf("USE_MY_METRICS vertical bearing X = %d, want %d", metrics.VertBearingX, (10<<6)-(700<<6)/2)
	}
}

func TestLoadGlyphRejectsDecreasingLocaOffsets(t *testing.T) {
	tests := []struct {
		name             string
		indexToLocFormat int16
	}{
		{name: "short", indexToLocFormat: 0},
		{name: "long", indexToLocFormat: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			face := loadGlyphBoundsTestFace(t, tc.indexToLocFormat, []uint32{20, 10}, make([]byte, 20))
			if _, err := face.LoadGlyph(0, 0); err == nil {
				t.Fatal("expected decreasing loca offsets to fail")
			}
		})
	}
}

func TestLoadGlyphRejectsGlyfRangeOutOfBounds(t *testing.T) {
	face := loadGlyphBoundsTestFace(t, 1, []uint32{4, 20}, make([]byte, 10))
	if _, err := face.LoadGlyph(0, 0); err == nil {
		t.Fatal("expected glyph range outside glyf table to fail")
	}
}

func TestLoadGlyphHonorsLocaLength(t *testing.T) {
	face := loadGlyphBoundsTestFace(t, 1, []uint32{0, 12}, onePointGlyphData())
	if _, err := face.LoadGlyph(0, 0); err == nil {
		t.Fatal("expected glyph parser to reject reads past loca length")
	}
}

func TestLoadGlyphRejectsCompositeCycles(t *testing.T) {
	t.Run("self", func(t *testing.T) {
		glyf := compositeGlyphData(0)
		face := loadGlyphBoundsTestFace(t, 1, []uint32{0, uint32(len(glyf))}, glyf)
		if _, err := face.LoadGlyph(0, 0); err == nil {
			t.Fatal("expected self-referential composite glyph to fail")
		}
	})

	t.Run("mutual", func(t *testing.T) {
		glyph0 := compositeGlyphData(1)
		glyph1 := compositeGlyphData(0)
		glyf := append(append([]byte{}, glyph0...), glyph1...)
		face := loadGlyphBoundsTestFace(t, 1, []uint32{0, uint32(len(glyph0)), uint32(len(glyf))}, glyf)
		if _, err := face.LoadGlyph(0, 0); err == nil {
			t.Fatal("expected mutually recursive composite glyphs to fail")
		}
	})
}

func TestLoadGlyphRejectsCompositeDepthLimit(t *testing.T) {
	numGlyphs := maxCompositeGlyphDepth + 2
	offsets := make([]uint32, numGlyphs+1)
	var glyf []byte
	for i := 0; i < numGlyphs-1; i++ {
		offsets[i] = uint32(len(glyf))
		glyf = append(glyf, compositeGlyphData(uint16(i+1))...)
	}
	offsets[numGlyphs-1] = uint32(len(glyf))
	offsets[numGlyphs] = uint32(len(glyf))

	face := loadGlyphBoundsTestFace(t, 1, offsets, glyf)
	if _, err := face.LoadGlyph(0, 0); err == nil {
		t.Fatal("expected excessive composite glyph depth to fail")
	}
}

func TestLoadGlyphRejectsCompositeComponentLimit(t *testing.T) {
	glyph0 := compositeGlyphWithComponents(1, maxCompositeGlyphComponents+1)
	glyf := append([]byte{}, glyph0...)
	face := loadGlyphBoundsTestFace(t, 1, []uint32{0, uint32(len(glyph0)), uint32(len(glyf))}, glyf)
	if _, err := face.LoadGlyph(0, 0); err == nil {
		t.Fatal("expected excessive composite glyph components to fail")
	}
}

func TestLoadGlyphSimple(t *testing.T) {
	// Create a dummy SFNT with head, maxp, loca, and glyf
	data := make([]byte, 1000)

	// Offset Table
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], 4) // numTables

	// Table Directory entries (sorted by tag: glyf, head, loca, maxp)
	// glyf
	copy(data[12:16], "glyf")
	binary.BigEndian.PutUint32(data[20:24], 200)
	binary.BigEndian.PutUint32(data[24:28], 100)
	// head
	copy(data[28:32], "head")
	binary.BigEndian.PutUint32(data[36:40], 300)
	binary.BigEndian.PutUint32(data[40:44], 54)
	// loca
	copy(data[44:48], "loca")
	binary.BigEndian.PutUint32(data[52:56], 400)
	binary.BigEndian.PutUint32(data[56:60], 20)
	// maxp
	copy(data[60:64], "maxp")
	binary.BigEndian.PutUint32(data[68:72], 500)
	binary.BigEndian.PutUint32(data[72:76], 6)

	// head table data (indexToLocFormat = 0, short)
	binary.BigEndian.PutUint16(data[300+50:300+52], 0)

	// maxp table data (numGlyphs = 2)
	binary.BigEndian.PutUint16(data[500+4:500+6], 2)

	// loca table data (offsets for glyph 0, 1, and end of 1)
	// Glyph 0: offset 0, Glyph 1: offset 10, end: offset 50
	binary.BigEndian.PutUint16(data[400:402], 0)
	binary.BigEndian.PutUint16(data[402:404], 5)  // 10 / 2
	binary.BigEndian.PutUint16(data[404:406], 25) // 50 / 2

	// glyf table data for Glyph 1 (starts at glyfOffset + 10)
	glyphOffset := 200 + 10
	// numberOfContours = 1
	binary.BigEndian.PutUint16(data[glyphOffset:glyphOffset+2], 1)
	// Bounding box (xMin, yMin, xMax, yMax)
	binary.BigEndian.PutUint16(data[glyphOffset+2:glyphOffset+4], 0)
	binary.BigEndian.PutUint16(data[glyphOffset+4:glyphOffset+6], 0)
	binary.BigEndian.PutUint16(data[glyphOffset+6:glyphOffset+8], 100)
	binary.BigEndian.PutUint16(data[glyphOffset+8:glyphOffset+10], 100)

	// endPtsOfContours[0] = 2 (3 points: 0, 1, 2)
	binary.BigEndian.PutUint16(data[glyphOffset+10:glyphOffset+12], 2)
	// instructionLength = 0
	binary.BigEndian.PutUint16(data[glyphOffset+12:glyphOffset+14], 0)

	// Flags (3 points)
	data[glyphOffset+14] = 0x01 | 0x02 | 0x04 | 0x10 | 0x20 // 0x37
	data[glyphOffset+15] = 0x01 | 0x02 | 0x04 | 0x00 | 0x20 // 0x25
	data[glyphOffset+16] = 0x01 | 0x02 | 0x04 | 0x10 | 0x00 // 0x17

	// X coordinates (3 bytes)
	data[glyphOffset+17] = 10 // x0 = 10
	data[glyphOffset+18] = 5  // x1 = 10 - 5 = 5
	data[glyphOffset+19] = 20 // x2 = 5 + 20 = 25

	// Y coordinates (3 bytes)
	data[glyphOffset+20] = 30 // y0 = 30
	data[glyphOffset+21] = 10 // y1 = 30 + 10 = 40
	data[glyphOffset+22] = 15 // y2 = 40 - 15 = 25

	stream := &mockStream{data: data}
	sys := &mockSystem{}
	loader := NewLoader(sys)
	f, _ := loader.LoadFace(stream)

	slot, err := f.LoadGlyph(1, 0)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	if got := f.GetGlyphSlot(); got != slot {
		t.Fatalf("GetGlyphSlot returned %T, want loaded slot %T", got, slot)
	}

	outline := slot.GetOutline()
	if outline == nil {
		t.Fatal("Outline is nil")
	}

	points := outline.GetPoints()
	if len(points) != 3 {
		t.Errorf("expected 3 points, got %d", len(points))
	}

	// Point 0: (10, 30) -> (640, 1920) in 26.6
	if points[0].X != 640 || points[0].Y != 1920 {
		t.Errorf("point 0 mismatch: got (%d, %d)", points[0].X, points[0].Y)
	}
	// Point 1: (5, 40) -> (320, 2560)
	if points[1].X != 320 || points[1].Y != 2560 {
		t.Errorf("point 1 mismatch: got (%d, %d)", points[1].X, points[1].Y)
	}
	// Point 2: (25, 25) -> (1600, 1600)
	if points[2].X != 1600 || points[2].Y != 1600 {
		t.Errorf("point 2 mismatch: got (%d, %d)", points[2].X, points[2].Y)
	}
}
