package sfnt

import (
	"encoding/binary"
	"testing"
)

func TestParseBASEAxisScriptValuesAndMinMax(t *testing.T) {
	data := makeTestBASETable()

	base, err := ParseBASE(&mockStream{data: data})
	if err != nil {
		t.Fatalf("ParseBASE failed: %v", err)
	}
	if base.MajorVersion != 1 || base.MinorVersion != 0 {
		t.Fatalf("unexpected BASE version: %d.%d", base.MajorVersion, base.MinorVersion)
	}
	if base.HorizAxis == nil || base.VertAxis != nil {
		t.Fatalf("unexpected axis pointers: horiz=%v vert=%v", base.HorizAxis, base.VertAxis)
	}
	if got, want := base.HorizAxis.BaseTags, []uint32{stringToTag("ideo"), stringToTag("romn")}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("unexpected BASE tags: %+v", got)
	}
	if len(base.HorizAxis.BaseScriptRecords) != 1 {
		t.Fatalf("expected one BaseScriptRecord, got %d", len(base.HorizAxis.BaseScriptRecords))
	}
	record := base.HorizAxis.BaseScriptRecords[0]
	if record.BaseScriptTag != stringToTag("latn") {
		t.Fatalf("unexpected script tag: %#x", record.BaseScriptTag)
	}
	values := record.BaseScript.BaseValues
	if values == nil || values.DefaultBaselineIndex != 1 || len(values.BaseCoords) != 2 {
		t.Fatalf("unexpected BaseValues: %+v", values)
	}
	if values.BaseCoords[0] == nil || values.BaseCoords[0].Coordinate != -120 {
		t.Fatalf("unexpected ideographic baseline coord: %+v", values.BaseCoords[0])
	}
	if values.BaseCoords[1] == nil || values.BaseCoords[1].Coordinate != 0 {
		t.Fatalf("unexpected roman baseline coord: %+v", values.BaseCoords[1])
	}
	minMax := record.BaseScript.DefaultMinMax
	if minMax == nil || minMax.MinCoord == nil || minMax.MaxCoord == nil {
		t.Fatalf("unexpected MinMax: %+v", minMax)
	}
	if minMax.MinCoord.Coordinate != -300 || minMax.MaxCoord.Coordinate != 900 {
		t.Fatalf("unexpected MinMax coords: min=%+v max=%+v", minMax.MinCoord, minMax.MaxCoord)
	}
}

func TestParseBASERejectsMalformedBounds(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "short header",
			data: make([]byte, 7),
		},
		{
			name: "axis script list offset outside table",
			data: func() []byte {
				data := make([]byte, 12)
				binary.BigEndian.PutUint16(data[0:2], 1)
				binary.BigEndian.PutUint16(data[4:6], 8)
				binary.BigEndian.PutUint16(data[10:12], 100)
				return data
			}(),
		},
		{
			name: "base coord truncated",
			data: makeTestBASETable()[:46],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseBASE(&mockStream{data: tt.data}); err == nil {
				t.Fatal("expected malformed BASE table to fail")
			}
		})
	}
}

func TestParseBASECoordFormats(t *testing.T) {
	format2 := make([]byte, 8)
	binary.BigEndian.PutUint16(format2[0:2], 2)
	binary.BigEndian.PutUint16(format2[2:4], 0xffd3)
	binary.BigEndian.PutUint16(format2[4:6], 17)
	binary.BigEndian.PutUint16(format2[6:8], 3)
	coord, err := parseBASECoord(&mockStream{data: format2}, 0)
	if err != nil {
		t.Fatalf("parseBASECoord format 2 failed: %v", err)
	}
	if coord.Coordinate != -45 || coord.ReferenceGlyph != 17 || coord.BaseCoordPoint != 3 {
		t.Fatalf("unexpected format 2 coord: %+v", coord)
	}

	format3 := make([]byte, 6)
	binary.BigEndian.PutUint16(format3[0:2], 3)
	binary.BigEndian.PutUint16(format3[2:4], 240)
	binary.BigEndian.PutUint16(format3[4:6], 6)
	coord, err = parseBASECoord(&mockStream{data: format3}, 0)
	if err != nil {
		t.Fatalf("parseBASECoord format 3 failed: %v", err)
	}
	if coord.Coordinate != 240 || coord.DeviceOffset != 6 {
		t.Fatalf("unexpected format 3 coord: %+v", coord)
	}
}

func makeTestBASETable() []byte {
	data := make([]byte, 66)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 8)

	axis := 8
	binary.BigEndian.PutUint16(data[axis:axis+2], 4)
	binary.BigEndian.PutUint16(data[axis+2:axis+4], 14)

	baseTagList := axis + 4
	binary.BigEndian.PutUint16(data[baseTagList:baseTagList+2], 2)
	binary.BigEndian.PutUint32(data[baseTagList+2:baseTagList+6], stringToTag("ideo"))
	binary.BigEndian.PutUint32(data[baseTagList+6:baseTagList+10], stringToTag("romn"))

	baseScriptList := axis + 14
	binary.BigEndian.PutUint16(data[baseScriptList:baseScriptList+2], 1)
	binary.BigEndian.PutUint32(data[baseScriptList+2:baseScriptList+6], stringToTag("latn"))
	binary.BigEndian.PutUint16(data[baseScriptList+6:baseScriptList+8], 8)

	baseScript := baseScriptList + 8
	binary.BigEndian.PutUint16(data[baseScript:baseScript+2], 6)
	binary.BigEndian.PutUint16(data[baseScript+2:baseScript+4], 22)
	binary.BigEndian.PutUint16(data[baseScript+4:baseScript+6], 0)

	baseValues := baseScript + 6
	binary.BigEndian.PutUint16(data[baseValues:baseValues+2], 1)
	binary.BigEndian.PutUint16(data[baseValues+2:baseValues+4], 2)
	binary.BigEndian.PutUint16(data[baseValues+4:baseValues+6], 8)
	binary.BigEndian.PutUint16(data[baseValues+6:baseValues+8], 12)
	putBASECoordFormat1(data[baseValues+8:baseValues+12], -120)
	putBASECoordFormat1(data[baseValues+12:baseValues+16], 0)

	minMax := baseScript + 22
	binary.BigEndian.PutUint16(data[minMax:minMax+2], 6)
	binary.BigEndian.PutUint16(data[minMax+2:minMax+4], 10)
	binary.BigEndian.PutUint16(data[minMax+4:minMax+6], 0)
	putBASECoordFormat1(data[minMax+6:minMax+10], -300)
	putBASECoordFormat1(data[minMax+10:minMax+14], 900)

	return data
}

func putBASECoordFormat1(dst []byte, coordinate int16) {
	binary.BigEndian.PutUint16(dst[0:2], 1)
	binary.BigEndian.PutUint16(dst[2:4], uint16(coordinate))
}
