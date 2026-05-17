package sfnt

import (
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

var benchmarkGlyphSlot api.GlyphSlot

func BenchmarkLoadGlyphSynthetic(b *testing.B) {
	for _, tc := range []struct {
		name       string
		glyphIndex int
	}{
		{name: "simple", glyphIndex: 1},
		{name: "composite", glyphIndex: 2},
	} {
		b.Run(tc.name, func(b *testing.B) {
			face := benchmarkSyntheticFace(b)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				slot, err := face.LoadGlyph(tc.glyphIndex, api.LoadNoHinting)
				if err != nil {
					b.Fatalf("LoadGlyph(%d) failed: %v", tc.glyphIndex, err)
				}
				benchmarkGlyphSlot = slot
			}
		})
	}
}

func benchmarkSyntheticFace(tb testing.TB) api.Face {
	tb.Helper()

	stream := &mockStream{data: benchmarkSyntheticSFNT()}
	face, err := NewLoader(core.NewSystem()).LoadFace(stream)
	if err != nil {
		tb.Fatalf("LoadFace failed: %v", err)
	}
	if err := face.SetPixelSizes(18, 18); err != nil {
		tb.Fatalf("SetPixelSizes failed: %v", err)
	}
	return face
}

type benchmarkPoint struct {
	x int16
	y int16
}

type benchmarkSFNTTable struct {
	tag  string
	data []byte
}

func benchmarkSyntheticSFNT() []byte {
	glyf, loca := benchmarkGlyfLoca([][]byte{
		nil,
		benchmarkSimpleGlyph(32),
		benchmarkCompositeGlyph(),
	})

	tables := []benchmarkSFNTTable{
		{tag: "head", data: benchmarkHeadTable()},
		{tag: "hhea", data: benchmarkHheaTable()},
		{tag: "hmtx", data: benchmarkHmtxTable(3)},
		{tag: "loca", data: loca},
		{tag: "maxp", data: benchmarkMaxpTable(3, 32)},
		{tag: "glyf", data: glyf},
	}
	return benchmarkBuildSFNT(tables)
}

func benchmarkSimpleGlyph(numPoints int) []byte {
	points := make([]benchmarkPoint, numPoints)
	for i := range points {
		points[i] = benchmarkPoint{
			x: int16(50 + (i*37)%700),
			y: int16(80 + (i*91)%650),
		}
	}

	xMin, yMin, xMax, yMax := benchmarkBounds(points)
	buf := make([]byte, 0, 10+2+2+numPoints+numPoints*4)
	buf = benchmarkAppendInt16(buf, 1)
	buf = benchmarkAppendInt16(buf, xMin)
	buf = benchmarkAppendInt16(buf, yMin)
	buf = benchmarkAppendInt16(buf, xMax)
	buf = benchmarkAppendInt16(buf, yMax)
	buf = benchmarkAppendUint16(buf, uint16(numPoints-1))
	buf = benchmarkAppendUint16(buf, 0)

	for range points {
		buf = append(buf, 0x01) // on-curve, signed int16 x/y deltas follow.
	}

	var lastX, lastY int16
	for _, p := range points {
		buf = benchmarkAppendInt16(buf, p.x-lastX)
		lastX = p.x
	}
	for _, p := range points {
		buf = benchmarkAppendInt16(buf, p.y-lastY)
		lastY = p.y
	}
	return buf
}

func benchmarkCompositeGlyph() []byte {
	const (
		componentFlags = ARG_1_AND_2_ARE_WORDS | ARGS_ARE_XY_VALUES
	)

	buf := make([]byte, 0, 10+4*8)
	buf = benchmarkAppendInt16(buf, -1)
	buf = benchmarkAppendInt16(buf, 0)
	buf = benchmarkAppendInt16(buf, 0)
	buf = benchmarkAppendInt16(buf, 1100)
	buf = benchmarkAppendInt16(buf, 1100)

	components := []struct {
		flags uint16
		dx    int16
		dy    int16
	}{
		{flags: componentFlags | MORE_COMPONENTS, dx: 0, dy: 0},
		{flags: componentFlags | MORE_COMPONENTS, dx: 380, dy: 0},
		{flags: componentFlags | MORE_COMPONENTS, dx: 0, dy: 380},
		{flags: componentFlags, dx: 380, dy: 380},
	}
	for _, component := range components {
		buf = benchmarkAppendUint16(buf, component.flags)
		buf = benchmarkAppendUint16(buf, 1)
		buf = benchmarkAppendInt16(buf, component.dx)
		buf = benchmarkAppendInt16(buf, component.dy)
	}
	return buf
}

func benchmarkGlyfLoca(glyphs [][]byte) ([]byte, []byte) {
	glyf := make([]byte, 0)
	loca := make([]byte, (len(glyphs)+1)*2)
	for i, glyph := range glyphs {
		binary.BigEndian.PutUint16(loca[i*2:i*2+2], uint16(len(glyf)/2))
		glyf = append(glyf, glyph...)
		if len(glyf)%2 != 0 {
			glyf = append(glyf, 0)
		}
	}
	binary.BigEndian.PutUint16(loca[len(glyphs)*2:len(glyphs)*2+2], uint16(len(glyf)/2))
	return glyf, loca
}

func benchmarkHeadTable() []byte {
	head := make([]byte, 54)
	binary.BigEndian.PutUint16(head[18:20], 1000)
	binary.BigEndian.PutUint16(head[50:52], 0)
	return head
}

func benchmarkHheaTable() []byte {
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[10:12], 1000)
	binary.BigEndian.PutUint16(hhea[34:36], 1)
	return hhea
}

func benchmarkHmtxTable(numGlyphs int) []byte {
	hmtx := make([]byte, 4+(numGlyphs-1)*2)
	binary.BigEndian.PutUint16(hmtx[0:2], 1000)
	return hmtx
}

func benchmarkMaxpTable(numGlyphs, maxPoints int) []byte {
	maxp := make([]byte, 32)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], uint16(numGlyphs))
	binary.BigEndian.PutUint16(maxp[6:8], uint16(maxPoints))
	binary.BigEndian.PutUint16(maxp[8:10], 1)
	binary.BigEndian.PutUint16(maxp[10:12], uint16(maxPoints*4))
	binary.BigEndian.PutUint16(maxp[12:14], 4)
	binary.BigEndian.PutUint16(maxp[24:26], 64)
	binary.BigEndian.PutUint16(maxp[28:30], 4)
	binary.BigEndian.PutUint16(maxp[30:32], 2)
	return maxp
}

func benchmarkBuildSFNT(tables []benchmarkSFNTTable) []byte {
	numTables := len(tables)
	dataSize := benchmarkAlign4(12 + numTables*16)
	for _, table := range tables {
		dataSize += benchmarkAlign4(len(table.data))
	}

	data := make([]byte, dataSize)
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], uint16(numTables))
	binary.BigEndian.PutUint16(data[6:8], 64)
	binary.BigEndian.PutUint16(data[8:10], 2)
	binary.BigEndian.PutUint16(data[10:12], uint16(numTables*16-64))

	tableOffset := benchmarkAlign4(12 + numTables*16)
	for i, table := range tables {
		dirOffset := 12 + i*16
		copy(data[dirOffset:dirOffset+4], table.tag)
		binary.BigEndian.PutUint32(data[dirOffset+8:dirOffset+12], uint32(tableOffset))
		binary.BigEndian.PutUint32(data[dirOffset+12:dirOffset+16], uint32(len(table.data)))
		copy(data[tableOffset:tableOffset+len(table.data)], table.data)
		tableOffset += benchmarkAlign4(len(table.data))
	}
	return data
}

func benchmarkBounds(points []benchmarkPoint) (int16, int16, int16, int16) {
	xMin, yMin := points[0].x, points[0].y
	xMax, yMax := points[0].x, points[0].y
	for _, p := range points[1:] {
		if p.x < xMin {
			xMin = p.x
		}
		if p.y < yMin {
			yMin = p.y
		}
		if p.x > xMax {
			xMax = p.x
		}
		if p.y > yMax {
			yMax = p.y
		}
	}
	return xMin, yMin, xMax, yMax
}

func benchmarkAppendUint16(buf []byte, value uint16) []byte {
	return append(buf, byte(value>>8), byte(value))
}

func benchmarkAppendInt16(buf []byte, value int16) []byte {
	return benchmarkAppendUint16(buf, uint16(value))
}

func benchmarkAlign4(n int) int {
	return (n + 3) &^ 3
}
