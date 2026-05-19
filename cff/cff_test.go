package cff

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

func csNumber(v int) byte {
	if v < -107 || v > 107 {
		panic("test charstring number out of single-byte range")
	}
	return byte(v + 139)
}

func testIndex(objects ...[]byte) *Index {
	offsets := make([]uint32, len(objects)+1)
	offsets[0] = 1
	data := make([]byte, 0)
	next := uint32(1)
	for i, obj := range objects {
		data = append(data, obj...)
		next += uint32(len(obj))
		offsets[i+1] = next
	}
	return &Index{
		Count:   uint16(len(objects)),
		OffSize: 1,
		Offsets: offsets,
		Data:    data,
	}
}

func testIndex16Bytes(objects ...[]byte) []byte {
	total := 0
	for _, obj := range objects {
		total += len(obj)
	}
	if total > 254 {
		panic("test INDEX data too large for offSize 1")
	}

	data := make([]byte, 2, 2+1+len(objects)+1+total)
	binary.BigEndian.PutUint16(data, uint16(len(objects)))
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

func testIndex32Bytes(objects ...[]byte) []byte {
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

func dictInt32(v int) []byte {
	return []byte{
		29,
		byte(v >> 24),
		byte(v >> 16),
		byte(v >> 8),
		byte(v),
	}
}

func dictOperator(op int) []byte {
	if op >= 12<<8 {
		return []byte{12, byte(op - (12 << 8))}
	}
	return []byte{byte(op)}
}

func dictEntry(op int, values ...int) []byte {
	data := make([]byte, 0, len(values)*5+2)
	for _, v := range values {
		data = append(data, dictInt32(v)...)
	}
	return append(data, dictOperator(op)...)
}

func appendUint16(data []byte, v uint16) []byte {
	return append(data, byte(v>>8), byte(v))
}

func appendUint32(data []byte, v uint32) []byte {
	return append(data, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func appendInt16(data []byte, v int16) []byte {
	return appendUint16(data, uint16(v))
}

func appendF2Dot14(data []byte, values ...float64) []byte {
	for _, v := range values {
		raw := int16(math.Round(v * 16384))
		data = append(data, byte(raw>>8), byte(raw))
	}
	return data
}

func testVariationStoreBytes() []byte {
	itemStore := make([]byte, 0, 50)
	itemStore = append(itemStore, 0, 1)        // format
	itemStore = append(itemStore, 0, 0, 0, 16) // variationRegionListOffset
	itemStore = append(itemStore, 0, 2)        // itemVariationDataCount
	itemStore = append(itemStore, 0, 0, 0, 32) // itemVariationData[0] offset
	itemStore = append(itemStore, 0, 0, 0, 40) // itemVariationData[1] offset

	itemStore = append(itemStore, 0, 1)             // axisCount
	itemStore = append(itemStore, 0, 2)             // regionCount
	itemStore = appendF2Dot14(itemStore, 0, 1, 1)   // region 0
	itemStore = appendF2Dot14(itemStore, 0, 0.5, 1) // region 1

	itemStore = append(itemStore, 0, 0) // itemCount
	itemStore = append(itemStore, 0, 0) // wordDeltaCount
	itemStore = append(itemStore, 0, 1) // regionIndexCount
	itemStore = append(itemStore, 0, 0) // regionIndexes[0]

	itemStore = append(itemStore, 0, 0) // itemCount
	itemStore = append(itemStore, 0, 0) // wordDeltaCount
	itemStore = append(itemStore, 0, 2) // regionIndexCount
	itemStore = append(itemStore, 0, 0) // regionIndexes[0]
	itemStore = append(itemStore, 0, 1) // regionIndexes[1]

	data := make([]byte, 2, 2+len(itemStore))
	binary.BigEndian.PutUint16(data, uint16(len(itemStore)))
	return append(data, itemStore...)
}

type testCFF2Offsets struct {
	charStrings int
	fdSelect    int
	fdArray     int
	private1    int
	vstore      int
}

func testCFF2Fixture() ([]byte, testCFF2Offsets) {
	n := csNumber
	globalSubrs := testIndex32Bytes()
	charStrings := testIndex32Bytes(
		[]byte{n(0), n(0), 21},
		[]byte{n(10), n(20), n(5), n(7), n(-3), n(11), n(2), 16, 21},
	)
	fdSelect := []byte{0, 0, 1}
	private1 := append(dictInt32(1), byte(opPrivateVSIndex))
	vstore := testVariationStoreBytes()

	const topDictSize = 26
	charStringsOffset := 5 + topDictSize + len(globalSubrs)
	fdSelectOffset := charStringsOffset + len(charStrings)
	fdArrayOffset := fdSelectOffset + len(fdSelect)

	fd0 := dictEntry(opPrivate, 0, 0)
	fd1Placeholder := dictEntry(opPrivate, len(private1), 0)
	fdArrayLen := len(testIndex32Bytes(fd0, fd1Placeholder))
	private1Offset := fdArrayOffset + fdArrayLen
	fd1 := dictEntry(opPrivate, len(private1), private1Offset)
	fdArray := testIndex32Bytes(fd0, fd1)
	if len(fdArray) != fdArrayLen {
		panic("test FDArray length changed")
	}
	vstoreOffset := private1Offset + len(private1)

	topDict := make([]byte, 0, topDictSize)
	topDict = append(topDict, dictEntry(opCharStrings, charStringsOffset)...)
	topDict = append(topDict, dictEntry(opVariationStore, vstoreOffset)...)
	topDict = append(topDict, dictEntry(opFDArray, fdArrayOffset)...)
	topDict = append(topDict, dictEntry(opFDSelect, fdSelectOffset)...)
	if len(topDict) != topDictSize {
		panic("test Top DICT size changed")
	}

	header := []byte{2, 0, 5, 0, topDictSize}
	blob := append(header, topDict...)
	blob = append(blob, globalSubrs...)
	blob = append(blob, charStrings...)
	blob = append(blob, fdSelect...)
	blob = append(blob, fdArray...)
	blob = append(blob, private1...)
	blob = append(blob, vstore...)

	return blob, testCFF2Offsets{
		charStrings: charStringsOffset,
		fdSelect:    fdSelectOffset,
		fdArray:     fdArrayOffset,
		private1:    private1Offset,
		vstore:      vstoreOffset,
	}
}

func testCFF1CIDFixture() []byte {
	n := csNumber
	header := []byte{1, 0, 4, 4}
	nameIndex := testIndex16Bytes([]byte("CIDTest"))
	stringIndex := testIndex16Bytes()
	globalSubrs := testIndex16Bytes()
	charStrings := testIndex16Bytes(
		[]byte{n(0), n(0), 21, n(-107), 10, 14},
		[]byte{n(0), n(0), 21, n(-107), 10, 14},
	)
	fdSelect := []byte{0, 0, 1}

	private0Dict := dictEntry(opLocalSubrs, len(dictEntry(opLocalSubrs, 0)))
	private1Dict := dictEntry(opLocalSubrs, len(dictEntry(opLocalSubrs, 0)))
	local0 := testIndex16Bytes([]byte{n(10), n(0), 5, 11})
	local1 := testIndex16Bytes([]byte{n(0), n(20), 5, 11})

	const topDictSize = 20
	topDictIndexLen := len(testIndex16Bytes(make([]byte, topDictSize)))
	charStringsOffset := len(header) + len(nameIndex) + topDictIndexLen + len(stringIndex) + len(globalSubrs)
	fdSelectOffset := charStringsOffset + len(charStrings)
	fdArrayOffset := fdSelectOffset + len(fdSelect)

	fd0Placeholder := dictEntry(opPrivate, len(private0Dict), 0)
	fd1Placeholder := dictEntry(opPrivate, len(private1Dict), 0)
	fdArrayLen := len(testIndex16Bytes(fd0Placeholder, fd1Placeholder))
	private0Offset := fdArrayOffset + fdArrayLen
	private1Offset := private0Offset + len(private0Dict) + len(local0)

	fd0 := dictEntry(opPrivate, len(private0Dict), private0Offset)
	fd1 := dictEntry(opPrivate, len(private1Dict), private1Offset)
	fdArray := testIndex16Bytes(fd0, fd1)

	topDict := make([]byte, 0, topDictSize)
	topDict = append(topDict, dictEntry(opCharStrings, charStringsOffset)...)
	topDict = append(topDict, dictEntry(opFDArray, fdArrayOffset)...)
	topDict = append(topDict, dictEntry(opFDSelect, fdSelectOffset)...)
	if len(topDict) != topDictSize {
		panic("test CFF1 CID Top DICT size changed")
	}

	blob := append(header, nameIndex...)
	blob = append(blob, testIndex16Bytes(topDict)...)
	blob = append(blob, stringIndex...)
	blob = append(blob, globalSubrs...)
	blob = append(blob, charStrings...)
	blob = append(blob, fdSelect...)
	blob = append(blob, fdArray...)
	blob = append(blob, private0Dict...)
	blob = append(blob, local0...)
	blob = append(blob, private1Dict...)
	blob = append(blob, local1...)
	return blob
}

func testCFF1MetadataFixture(charset, encoding []byte, charStringCount int) []byte {
	return testCFF1MetadataFixtureWithStrings(charset, encoding, testIndex16Bytes(), charStringCount)
}

func testCFF1MetadataFixtureWithStrings(charset, encoding, stringIndex []byte, charStringCount int) []byte {
	header := []byte{1, 0, 4, 4}
	nameIndex := testIndex16Bytes([]byte("RawCFF"))
	globalSubrs := testIndex16Bytes()
	charStringsData := make([][]byte, charStringCount)
	for i := range charStringsData {
		charStringsData[i] = []byte{14}
	}
	charStrings := testIndex16Bytes(charStringsData...)

	const topDictSize = 39
	topDictIndexLen := len(testIndex16Bytes(make([]byte, topDictSize)))
	charsetOffset := len(header) + len(nameIndex) + topDictIndexLen + len(stringIndex) + len(globalSubrs)
	encodingOffset := charsetOffset + len(charset)
	charStringsOffset := encodingOffset + len(encoding)

	topDict := make([]byte, 0, topDictSize)
	topDict = append(topDict, dictEntry(opFontBBox, -10, -20, 700, 800)...)
	topDict = append(topDict, dictEntry(opCharset, charsetOffset)...)
	topDict = append(topDict, dictEntry(opEncoding, encodingOffset)...)
	topDict = append(topDict, dictEntry(opCharStrings, charStringsOffset)...)
	if len(topDict) != topDictSize {
		panic("test CFF1 metadata Top DICT size changed")
	}

	blob := append(header, nameIndex...)
	blob = append(blob, testIndex16Bytes(topDict)...)
	blob = append(blob, stringIndex...)
	blob = append(blob, globalSubrs...)
	blob = append(blob, charset...)
	blob = append(blob, encoding...)
	blob = append(blob, charStrings...)
	return blob
}

func testCFFCharsetFormat0(sids ...uint16) []byte {
	data := []byte{0}
	for _, sid := range sids {
		data = appendUint16(data, sid)
	}
	return data
}

func assertPoint(t *testing.T, outline *core.Outline, idx int, x, y int32) {
	t.Helper()
	if idx >= len(outline.Points) {
		t.Fatalf("point %d missing; outline has %d points", idx, len(outline.Points))
	}
	if outline.Points[idx].X != x*64 || outline.Points[idx].Y != y*64 {
		t.Fatalf("point %d: expected (%d, %d), got (%d, %d)", idx, x*64, y*64, outline.Points[idx].X, outline.Points[idx].Y)
	}
}

func interpretStack(t *testing.T, data []byte) *charStringContext {
	t.Helper()
	ctx := &charStringContext{outline: &core.Outline{}}
	if err := ctx.interpret(data); err != nil {
		t.Fatalf("interpret failed: %v", err)
	}
	return ctx
}

func assertStack(t *testing.T, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("stack length: got %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("stack[%d]: got %v, want %v; full stack %v", i, got[i], want[i], got)
		}
	}
}

func assertVector(t *testing.T, got api.Vector, wantX, wantY int32) {
	t.Helper()
	if got.X != wantX*64 || got.Y != wantY*64 {
		t.Fatalf("expected vector (%d, %d), got (%d, %d)", wantX*64, wantY*64, got.X, got.Y)
	}
}

func assertLineSegment(t *testing.T, got charStringSegment, fromX, fromY, toX, toY int32) {
	t.Helper()
	if got.kind != charStringLineSegment {
		t.Fatalf("segment kind = %d, want line", got.kind)
	}
	assertVector(t, got.from, fromX, fromY)
	assertVector(t, got.to, toX, toY)
}

func assertCubicSegment(t *testing.T, got charStringSegment, fromX, fromY, ctrl1X, ctrl1Y, ctrl2X, ctrl2Y, toX, toY int32) {
	t.Helper()
	if got.kind != charStringCubicSegment {
		t.Fatalf("segment kind = %d, want cubic", got.kind)
	}
	assertVector(t, got.from, fromX, fromY)
	assertVector(t, got.ctrl1, ctrl1X, ctrl1Y)
	assertVector(t, got.ctrl2, ctrl2X, ctrl2Y)
	assertVector(t, got.to, toX, toY)
}

func TestParseIndex(t *testing.T) {
	// Case 1: OffSize 1
	data1 := []byte{
		0x00, 0x02, // Count = 2
		0x01,             // OffSize = 1
		0x01, 0x02, 0x04, // Offsets: 1, 2, 4
		'A', 'B', 'C', // Data
	}
	stream1 := core.NewMemoryStream(data1)
	idx1, next1, err := parseIndex(stream1, 0)
	if err != nil {
		t.Fatalf("failed to parse index 1: %v", err)
	}
	if idx1.Count != 2 {
		t.Errorf("expected count 2, got %d", idx1.Count)
	}
	if next1 != int64(len(data1)) {
		t.Errorf("expected next %d, got %d", len(data1), next1)
	}
	obj0, _ := idx1.Get(0)
	if !bytes.Equal(obj0, []byte("A")) {
		t.Errorf("expected obj0 'A', got %q", obj0)
	}
	obj1, _ := idx1.Get(1)
	if !bytes.Equal(obj1, []byte("BC")) {
		t.Errorf("expected obj1 'BC', got %q", obj1)
	}

	// Case 2: OffSize 2
	data2 := []byte{
		0x00, 0x02, // Count = 2
		0x02,                               // OffSize = 2
		0x00, 0x01, 0x00, 0x03, 0x00, 0x06, // Offsets: 1, 3, 6
		'A', 'B', 'C', 'D', 'E', // Data
	}
	stream2 := core.NewMemoryStream(data2)
	idx2, _, err := parseIndex(stream2, 0)
	if err != nil {
		t.Fatalf("failed to parse index 2: %v", err)
	}
	obj2_0, _ := idx2.Get(0)
	if !bytes.Equal(obj2_0, []byte("AB")) {
		t.Errorf("expected obj2_0 'AB', got %q", obj2_0)
	}
	obj2_1, _ := idx2.Get(1)
	if !bytes.Equal(obj2_1, []byte("CDE")) {
		t.Errorf("expected obj2_1 'CDE', got %q", obj2_1)
	}

	// Case 3: Empty Index
	data3 := []byte{0x00, 0x00}
	stream3 := core.NewMemoryStream(data3)
	idx3, next3, err := parseIndex(stream3, 0)
	if err != nil {
		t.Fatalf("failed to parse index 3: %v", err)
	}
	if idx3.Count != 0 {
		t.Errorf("expected count 0, got %d", idx3.Count)
	}
	if next3 != 2 {
		t.Errorf("expected next 2, got %d", next3)
	}
}

func TestParseIndexRejectsInvalidOffsets(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "zero offset",
			data: []byte{
				0x00, 0x01, // Count = 1
				0x01,       // OffSize = 1
				0x00, 0x02, // Offsets include invalid zero
				'A',
			},
		},
		{
			name: "non-monotonic offsets",
			data: []byte{
				0x00, 0x02, // Count = 2
				0x01, // OffSize = 1
				0x01, 0x03, 0x02,
				'A',
			},
		},
		{
			name: "data extends beyond stream",
			data: []byte{
				0x00, 0x01, // Count = 1
				0x01,       // OffSize = 1
				0x01, 0xff, // Declares 254 data bytes
				'A',
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := core.NewMemoryStream(tt.data)
			if _, _, err := parseIndex(stream, 0); err == nil {
				t.Fatalf("parseIndex succeeded for invalid INDEX")
			}
		})
	}
}

func TestParseCFF(t *testing.T) {
	// Construct a minimal CFF blob
	// Header: 1, 0, 4, 4
	header := []byte{1, 0, 4, 4}

	// Name INDEX: 1 name "TestFont"
	nameIndex := []byte{
		0x00, 0x01, // Count
		0x01,       // OffSize
		0x01, 0x09, // Offsets: 1, 9
		'T', 'e', 's', 't', 'F', 'o', 'n', 't',
	}

	// Top DICT INDEX: 1 empty dict
	topDictIndex := []byte{
		0x00, 0x01,
		0x01,
		0x01, 0x02,
		0x00, // Empty dict? (actually dicts are not empty usually but let's say 1 byte of 0)
	}

	// String INDEX: empty
	stringIndex := []byte{0x00, 0x00}

	// Global Subr INDEX: empty
	globalSubrIndex := []byte{0x00, 0x00}

	cffBlob := append(header, nameIndex...)
	cffBlob = append(cffBlob, topDictIndex...)
	cffBlob = append(cffBlob, stringIndex...)
	cffBlob = append(cffBlob, globalSubrIndex...)

	stream := core.NewMemoryStream(cffBlob)
	cff, err := ParseCFF(stream, 0)
	if err != nil {
		t.Fatalf("ParseCFF failed: %v", err)
	}

	if cff.Major != 1 || cff.Minor != 0 {
		t.Errorf("wrong version: %d.%d", cff.Major, cff.Minor)
	}

	if cff.NameIndex.Count != 1 {
		t.Errorf("expected 1 name, got %d", cff.NameIndex.Count)
	}
	name, _ := cff.NameIndex.Get(0)
	if string(name) != "TestFont" {
		t.Errorf("expected TestFont, got %s", name)
	}
}

func TestParseCFFRawMetadataEncodingAndFontBBox(t *testing.T) {
	sids := make([]uint16, 20)
	for i := 0; i < 19; i++ {
		sids[i] = uint16(34 + i) // A through S.
	}
	sids[19] = 13 // comma.
	charset := testCFFCharsetFormat0(sids...)
	encoding := append([]byte{0, 20},
		1, 2, 3, 4, 5,
		6, 7, 8, 9, 10,
		11, 12, 13, 14, 15,
		16, 17, 18, 19, 59,
	)
	face, err := ParseCFF(core.NewMemoryStream(testCFF1MetadataFixture(charset, encoding, 21)), 0)
	if err != nil {
		t.Fatalf("ParseCFF failed: %v", err)
	}

	if got := face.GetUnitsPerEm(); got != 1000 {
		t.Fatalf("GetUnitsPerEm = %d, want 1000", got)
	}
	if got := face.GetNumGlyphs(); got != 21 {
		t.Fatalf("GetNumGlyphs = %d, want 21", got)
	}
	if bbox, ok := face.GetFontBBox(); !ok || bbox != ([4]float64{-10, -20, 700, 800}) {
		t.Fatalf("GetFontBBox = %v, %v; want [-10 -20 700 800], true", bbox, ok)
	}
	if gid, err := face.GetGlyphIndex(rune(59)); err != nil || gid != 20 {
		t.Fatalf("GetGlyphIndex(59) = %d, %v; want 20, nil", gid, err)
	}
	if name, ok := face.GetGlyphNameByCharCode(59); !ok || name != "comma" {
		t.Fatalf("GetGlyphNameByCharCode(59) = %q, %v; want comma, true", name, ok)
	}
	if gid, ok := face.LookupGlyphIndexByName("comma"); !ok || gid != 20 {
		t.Fatalf("LookupGlyphIndexByName(comma) = %d, %v; want 20, true", gid, ok)
	}
	if name, ok := face.GlyphName(20); !ok || name != "comma" {
		t.Fatalf("GlyphName(20) = %q, %v; want comma, true", name, ok)
	}
	if name, ok := face.GetGlyphName(20); !ok || name != "comma" {
		t.Fatalf("GetGlyphName(20) = %q, %v; want comma, true", name, ok)
	}
}

func TestParseCFFRawEncodingSupplementUsesGlyphName(t *testing.T) {
	charset := testCFFCharsetFormat0(34, 13) // A, comma.
	encoding := []byte{
		0x80, 1, 65, // format 0 with one base code: code 65 -> GID 1/A.
		1, 59, 0, 13, // supplement: code 59 -> SID 13/comma -> GID 2.
	}
	face, err := ParseCFF(core.NewMemoryStream(testCFF1MetadataFixture(charset, encoding, 3)), 0)
	if err != nil {
		t.Fatalf("ParseCFF failed: %v", err)
	}

	if gid, err := face.GetGlyphIndex(rune(59)); err != nil || gid != 2 {
		t.Fatalf("GetGlyphIndex(59) = %d, %v; want 2, nil", gid, err)
	}
	if name, ok := face.GetGlyphNameByCharCode(59); !ok || name != "comma" {
		t.Fatalf("GetGlyphNameByCharCode(59) = %q, %v; want comma, true", name, ok)
	}
	if gid, err := face.GetGlyphIndex('A'); err != nil || gid != 1 {
		t.Fatalf("GetGlyphIndex(A) = %d, %v; want 1, nil", gid, err)
	}
}

func TestParseCFFRawMetadataFormat1CharsetAndEncoding(t *testing.T) {
	charset := []byte{
		1, 0, 34, 2, // charset format 1: SIDs 34..36 => A..C.
	}
	encoding := []byte{
		1, 1, 65, 2, // Encoding format 1: codes 65..67 => GIDs 1..3.
	}

	face, err := ParseCFF(core.NewMemoryStream(testCFF1MetadataFixture(charset, encoding, 4)), 0)
	if err != nil {
		t.Fatalf("ParseCFF failed: %v", err)
	}

	if gid, err := face.GetGlyphIndex('C'); err != nil || gid != 3 {
		t.Fatalf("GetGlyphIndex(C) = %d, %v; want 3, nil", gid, err)
	}
	if name, ok := face.GetGlyphNameByCharCode(67); !ok || name != "C" {
		t.Fatalf("GetGlyphNameByCharCode(67) = %q, %v; want C, true", name, ok)
	}
}

func TestParseCFFRawMetadataStringIndexSID(t *testing.T) {
	charset := testCFFCharsetFormat0(391)
	encoding := []byte{0, 1, 42}
	stringIndex := testIndex16Bytes([]byte("customglyph"))

	face, err := ParseCFF(core.NewMemoryStream(testCFF1MetadataFixtureWithStrings(charset, encoding, stringIndex, 2)), 0)
	if err != nil {
		t.Fatalf("ParseCFF failed: %v", err)
	}

	if name, ok := face.GlyphName(1); !ok || name != "customglyph" {
		t.Fatalf("GlyphName(1) = %q, %v; want customglyph, true", name, ok)
	}
	if gid, ok := face.LookupGlyphIndexByName("customglyph"); !ok || gid != 1 {
		t.Fatalf("LookupGlyphIndexByName(customglyph) = %d, %v; want 1, true", gid, ok)
	}
	if name, ok := face.GetGlyphNameByCharCode(42); !ok || name != "customglyph" {
		t.Fatalf("GetGlyphNameByCharCode(42) = %q, %v; want customglyph, true", name, ok)
	}
}

func TestCFFStandardSIDNamesIncludeLigatures(t *testing.T) {
	face := &CFF{}
	tests := map[int]string{
		266: "ff",
		267: "ffi",
		268: "ffl",
	}
	for sid, want := range tests {
		got, err := face.stringForSID(sid)
		if err != nil || got != want {
			t.Fatalf("stringForSID(%d) = %q, %v; want %q, nil", sid, got, err, want)
		}
	}
}

func TestParseCFFRawDefaultPredefinedEncoding(t *testing.T) {
	header := []byte{1, 0, 4, 4}
	nameIndex := testIndex16Bytes([]byte("StdCFF"))
	stringIndex := testIndex16Bytes()
	globalSubrs := testIndex16Bytes()
	charStringsData := make([][]byte, 14)
	for i := range charStringsData {
		charStringsData[i] = []byte{14}
	}
	charStrings := testIndex16Bytes(charStringsData...)

	const topDictSize = 6
	topDictIndexLen := len(testIndex16Bytes(make([]byte, topDictSize)))
	charStringsOffset := len(header) + len(nameIndex) + topDictIndexLen + len(stringIndex) + len(globalSubrs)
	topDict := dictEntry(opCharStrings, charStringsOffset)

	blob := append(header, nameIndex...)
	blob = append(blob, testIndex16Bytes(topDict)...)
	blob = append(blob, stringIndex...)
	blob = append(blob, globalSubrs...)
	blob = append(blob, charStrings...)

	face, err := ParseCFF(core.NewMemoryStream(blob), 0)
	if err != nil {
		t.Fatalf("ParseCFF failed: %v", err)
	}
	if gid, err := face.GetGlyphIndex(','); err != nil || gid != 13 {
		t.Fatalf("GetGlyphIndex(comma) = %d, %v; want 13, nil", gid, err)
	}
	if name, ok := face.GetGlyphNameByCharCode(44); !ok || name != "comma" {
		t.Fatalf("GetGlyphNameByCharCode(44) = %q, %v; want comma, true", name, ok)
	}
}

func TestParseCFFCIDFDArrayFDSelectLocalSubrs(t *testing.T) {
	blob := testCFF1CIDFixture()
	face, err := ParseCFF(core.NewMemoryStream(blob), 0)
	if err != nil {
		t.Fatalf("ParseCFF CID fixture failed: %v", err)
	}

	if face.FDArrayIndex.Count != 2 || len(face.FontDicts) != 2 {
		t.Fatalf("FDArray count = %d, FontDicts = %d; want 2", face.FDArrayIndex.Count, len(face.FontDicts))
	}
	if face.FDSelect == nil || face.FDSelect.Format != 0 {
		t.Fatalf("expected format 0 FDSelect, got %#v", face.FDSelect)
	}
	if got, err := face.FDSelect.FDIndex(1); err != nil || got != 1 {
		t.Fatalf("FDSelect glyph 1 = %d, %v; want 1", got, err)
	}

	outline, err := face.LoadGlyphOutline(0)
	if err != nil {
		t.Fatalf("LoadGlyphOutline glyph 0 failed: %v", err)
	}
	assertPoint(t, outline, 1, 10, 0)

	outline, err = face.LoadGlyphOutline(1)
	if err != nil {
		t.Fatalf("LoadGlyphOutline glyph 1 failed: %v", err)
	}
	assertPoint(t, outline, 1, 0, 20)
}

func TestParseCFF2FDArrayPrivateVariationStore(t *testing.T) {
	blob, offsets := testCFF2Fixture()
	stream := core.NewMemoryStream(blob)
	face, err := ParseCFF(stream, 0)
	if err != nil {
		t.Fatalf("ParseCFF CFF2 failed: %v", err)
	}

	if face.Major != 2 || face.Minor != 0 {
		t.Fatalf("wrong CFF2 version: %d.%d", face.Major, face.Minor)
	}
	if face.TopDictSize != 26 {
		t.Fatalf("TopDictSize = %d, want 26", face.TopDictSize)
	}
	if got := int(face.TopDict[opCharStrings][0]); got != offsets.charStrings {
		t.Fatalf("CharStrings offset = %d, want %d", got, offsets.charStrings)
	}
	if face.CharStringsIndex.Count != 2 {
		t.Fatalf("CharStrings count = %d, want 2", face.CharStringsIndex.Count)
	}
	if face.FDArrayIndex.Count != 2 || len(face.FontDicts) != 2 {
		t.Fatalf("FDArray count = %d, FontDicts = %d; want 2", face.FDArrayIndex.Count, len(face.FontDicts))
	}
	if face.FDSelect == nil || face.FDSelect.Format != 0 {
		t.Fatalf("expected format 0 FDSelect, got %#v", face.FDSelect)
	}
	if got, err := face.FDSelect.FDIndex(1); err != nil || got != 1 {
		t.Fatalf("FDSelect glyph 1 = %d, %v; want 1", got, err)
	}
	if face.FontDicts[1].PrivateDict.VSIndex != 1 {
		t.Fatalf("Private DICT vsindex = %d, want 1", face.FontDicts[1].PrivateDict.VSIndex)
	}
	if face.VariationStore == nil {
		t.Fatalf("expected VariationStore")
	}
	if face.VariationStore.Offset != int64(offsets.vstore) {
		t.Fatalf("VariationStore offset = %d, want %d", face.VariationStore.Offset, offsets.vstore)
	}
	if face.VariationStore.AxisCount != 1 || face.VariationStore.RegionCount != 2 {
		t.Fatalf("VariationStore axis/region counts = %d/%d, want 1/2", face.VariationStore.AxisCount, face.VariationStore.RegionCount)
	}
	if got, ok := face.VariationStore.ActiveRegionCount(1); !ok || got != 2 {
		t.Fatalf("ActiveRegionCount(1) = %d, %v; want 2, true", got, ok)
	}
	if len(face.VariationStore.Regions) != 2 {
		t.Fatalf("regions = %d, want 2", len(face.VariationStore.Regions))
	}
	if got := face.VariationStore.Regions[1].Axes[0].Peak; got != 0.5 {
		t.Fatalf("region 1 peak = %v, want 0.5", got)
	}
	vector, ok := face.VariationStore.BlendVector(1, []float64{1})
	if !ok {
		t.Fatalf("BlendVector(1) failed")
	}
	assertStack(t, vector, []float64{1, 0})

	outline, err := face.LoadGlyphOutline(1)
	if err != nil {
		t.Fatalf("LoadGlyphOutline CFF2 failed: %v", err)
	}
	if len(outline.Points) != 1 {
		t.Fatalf("expected one point, got %d", len(outline.Points))
	}
	if len(outline.Contours) != 1 || outline.Contours[0] != 0 {
		t.Fatalf("CFF2 contours = %v, want [0]", outline.Contours)
	}
	assertPoint(t, outline, 0, 10, 20)

	outline, err = face.LoadGlyphOutlineAt(1, []float64{1})
	if err != nil {
		t.Fatalf("LoadGlyphOutlineAt CFF2 failed: %v", err)
	}
	assertPoint(t, outline, 0, 15, 17)

	face.SetVariationCoordinates([]float64{1})
	outline, err = face.LoadGlyphOutline(1)
	if err != nil {
		t.Fatalf("LoadGlyphOutline with stored CFF2 coordinates failed: %v", err)
	}
	assertPoint(t, outline, 0, 15, 17)
}

func TestLoadGlyphOutlineUsesCFF2OperandStackLimit(t *testing.T) {
	n := csNumber
	face := &CFF{
		Major: 2,
		CharStringsIndex: *testIndex(
			bytes.Repeat([]byte{n(0)}, maxCFF2ArgumentStack+1),
		),
		FontDicts: []FontDict{{}},
	}

	if _, err := face.LoadGlyphOutline(0); err == nil {
		t.Fatal("LoadGlyphOutline accepted CFF2 operand stack overflow")
	}
}

func TestParseCFF2RejectsMalformedBounds(t *testing.T) {
	valid, offsets := testCFF2Fixture()

	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "truncated top dict",
			data: []byte{2, 0, 5, 0, 10, 139},
		},
		{
			name: "truncated variation store",
			data: valid[:len(valid)-1],
		},
		{
			name: "FDSelect index out of range",
			data: func() []byte {
				data := append([]byte(nil), valid...)
				data[offsets.fdSelect+2] = 2
				return data
			}(),
		},
		{
			name: "FDSelect truncated",
			data: valid[:offsets.fdSelect+2],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseCFF(core.NewMemoryStream(tt.data), 0); err == nil {
				t.Fatalf("ParseCFF succeeded for malformed CFF2")
			}
		})
	}
}

func TestParseFDSelectRangeFormats(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want []int
	}{
		{
			name: "format 3",
			data: []byte{
				3,
				0, 3,
				0, 0, 0,
				0, 2, 1,
				0, 5, 2,
				0, 6,
			},
			want: []int{0, 0, 1, 1, 1, 2},
		},
		{
			name: "format 4",
			data: func() []byte {
				data := []byte{4}
				data = appendUint32(data, 3)
				data = appendUint32(data, 0)
				data = appendUint16(data, 0)
				data = appendUint32(data, 1)
				data = appendUint16(data, 2)
				data = appendUint32(data, 4)
				data = appendUint16(data, 1)
				data = appendUint32(data, 6)
				return data
			}(),
			want: []int{0, 2, 2, 2, 1, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fdSelect, err := parseFDSelect(core.NewMemoryStream(tt.data), 0, len(tt.want), 3)
			if err != nil {
				t.Fatalf("parseFDSelect failed: %v", err)
			}
			for glyphID, want := range tt.want {
				got, err := fdSelect.FDIndex(glyphID)
				if err != nil {
					t.Fatalf("FDIndex(%d) failed: %v", glyphID, err)
				}
				if got != want {
					t.Fatalf("FDIndex(%d) = %d, want %d", glyphID, got, want)
				}
			}
			if _, err := fdSelect.FDIndex(len(tt.want)); err == nil {
				t.Fatalf("FDIndex accepted out-of-range glyph")
			}
		})
	}
}

func TestParseFDSelectRejectsMalformedRanges(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "first range not zero",
			data: []byte{3, 0, 1, 0, 1, 0, 0, 3},
		},
		{
			name: "ranges not increasing",
			data: []byte{3, 0, 2, 0, 0, 0, 0, 0, 1, 0, 3},
		},
		{
			name: "sentinel mismatch",
			data: []byte{3, 0, 1, 0, 0, 0, 0, 4},
		},
		{
			name: "fd index out of range",
			data: []byte{3, 0, 1, 0, 0, 3, 0, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseFDSelect(core.NewMemoryStream(tt.data), 0, 3, 3); err == nil {
				t.Fatalf("parseFDSelect succeeded for %s", tt.name)
			}
		})
	}
}

func TestLoadGlyphOutlineUsesFDSelectLocalSubrs(t *testing.T) {
	n := csNumber
	face := &CFF{
		Major: 2,
		CharStringsIndex: *testIndex(
			[]byte{n(0), n(0), 21, n(-107), 10},
			[]byte{n(0), n(0), 21, n(-107), 10},
		),
		FDSelect: &FDSelect{Format: 3, GlyphFD: []uint16{0, 1}},
		FontDicts: []FontDict{
			{PrivateDict: PrivateDict{LocalSubrIndex: *testIndex([]byte{n(10), n(0), 5, 11})}},
			{PrivateDict: PrivateDict{LocalSubrIndex: *testIndex([]byte{n(0), n(20), 5, 11})}},
		},
	}

	outline, err := face.LoadGlyphOutline(0)
	if err != nil {
		t.Fatalf("LoadGlyphOutline glyph 0 failed: %v", err)
	}
	assertPoint(t, outline, 1, 10, 0)

	outline, err = face.LoadGlyphOutline(1)
	if err != nil {
		t.Fatalf("LoadGlyphOutline glyph 1 failed: %v", err)
	}
	assertPoint(t, outline, 1, 0, 20)
}

func TestVariationStoreParsesRegionsItemDataAndScalars(t *testing.T) {
	itemStore := make([]byte, 0, 44)
	itemStore = appendUint16(itemStore, 1)  // format
	itemStore = appendUint32(itemStore, 12) // variationRegionListOffset
	itemStore = appendUint16(itemStore, 1)  // itemVariationDataCount
	itemStore = appendUint32(itemStore, 28) // itemVariationData[0] offset

	itemStore = appendUint16(itemStore, 1)        // axisCount
	itemStore = appendUint16(itemStore, 2)        // regionCount
	itemStore = appendF2Dot14(itemStore, 0, 1, 1) // region 0
	itemStore = appendF2Dot14(itemStore, -1, -0.5, 0)

	itemStore = appendUint16(itemStore, 2)    // itemCount
	itemStore = appendUint16(itemStore, 1)    // wordDeltaCount
	itemStore = appendUint16(itemStore, 2)    // regionIndexCount
	itemStore = appendUint16(itemStore, 0)    // regionIndexes[0]
	itemStore = appendUint16(itemStore, 1)    // regionIndexes[1]
	itemStore = appendInt16(itemStore, 100)   // row 0, word delta
	itemStore = append(itemStore, byte(0xfb)) // row 0, int8 delta -5
	itemStore = appendInt16(itemStore, -200)  // row 1, word delta
	itemStore = append(itemStore, byte(7))    // row 1, int8 delta

	vs := &VariationStore{Data: itemStore}
	if err := parseItemVariationStore(vs); err != nil {
		t.Fatalf("parseItemVariationStore failed: %v", err)
	}
	if vs.AxisCount != 1 || vs.RegionCount != 2 {
		t.Fatalf("axis/region counts = %d/%d, want 1/2", vs.AxisCount, vs.RegionCount)
	}
	if got, ok := vs.ActiveRegionCount(0); !ok || got != 2 {
		t.Fatalf("ActiveRegionCount(0) = %d, %v; want 2, true", got, ok)
	}
	scalar, ok := vs.RegionScalar(0, []float64{0.5})
	if !ok || math.Abs(scalar-0.5) > 1e-9 {
		t.Fatalf("RegionScalar(0, 0.5) = %v, %v; want 0.5, true", scalar, ok)
	}
	scalar, ok = vs.RegionScalar(1, []float64{-0.25})
	if !ok || math.Abs(scalar-0.5) > 1e-9 {
		t.Fatalf("RegionScalar(1, -0.25) = %v, %v; want 0.5, true", scalar, ok)
	}
	vector, ok := vs.BlendVector(0, []float64{0.5})
	if !ok {
		t.Fatalf("BlendVector failed")
	}
	assertStack(t, vector, []float64{0.5, 0})

	itemData := vs.ItemVariationData[0]
	if itemData.ItemCount != 2 || itemData.WordDeltaCount != 1 || itemData.LongWords {
		t.Fatalf("unexpected ItemVariationData header: %#v", itemData)
	}
	if len(itemData.DeltaSets) != 2 || itemData.DeltaSets[0][0] != 100 || itemData.DeltaSets[0][1] != -5 {
		t.Fatalf("unexpected delta sets: %#v", itemData.DeltaSets)
	}
	if itemData.DeltaSets[1][0] != -200 || itemData.DeltaSets[1][1] != 7 {
		t.Fatalf("unexpected second delta set: %#v", itemData.DeltaSets)
	}
}

func TestParseDict(t *testing.T) {
	data := []byte{
		0x8b,       // 0
		0x11,       // operator 17
		0xf7, 0x00, // 108
		0xfb, 0x00, // -108
		0x12,             // operator 18
		0x1c, 0x01, 0x00, // 256 (16-bit)
		0x1d, 0x00, 0x00, 0x01, 0x00, // 256 (32-bit)
		0x0c, 0x07, // operator 12, 7
		0x1e, 0x1a, 0x5f, // 1.5 (BCD)
		0x1e, 0xe0, 0xa1, 0x23, 0xff, // -0.123 (BCD)
		0x01,                         // operator 1
		0x1d, 0x00, 0x00, 0x00, 0xc1, // 193 (32-bit)
		0x19, // operator 25
	}

	dict, err := ParseDict(data)
	if err != nil {
		t.Fatalf("ParseDict failed: %v", err)
	}

	// Op 17: [0]
	if val, ok := dict[17]; !ok || len(val) != 1 || val[0] != 0 {
		t.Errorf("expected op 17 to be [0], got %v", val)
	}

	// Op 18: [108, -108]
	if val, ok := dict[18]; !ok || len(val) != 2 || val[0] != 108 || val[1] != -108 {
		t.Errorf("expected op 18 to be [108, -108], got %v", val)
	}

	// Op 12, 7 (3079): [256, 256]
	if val, ok := dict[12<<8|7]; !ok || len(val) != 2 || val[0] != 256 || val[1] != 256 {
		t.Errorf("expected op 3079 to be [256, 256], got %v", val)
	}

	// Op 1: [1.5, -0.123]
	if val, ok := dict[1]; !ok || len(val) != 2 || val[0] != 1.5 || val[1] != -0.123 {
		t.Errorf("expected op 1 to be [1.5, -0.123], got %v", val)
	}

	// Op 25: [193]
	if val, ok := dict[25]; !ok || len(val) != 1 || val[0] != 193 {
		t.Errorf("expected op 25 to be [193], got %v", val)
	}
}

func TestParseDictRejectsMalformedOperandStack(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "trailing operand", data: []byte{csNumber(0)}},
		{name: "operand stack overflow", data: bytes.Repeat([]byte{csNumber(0)}, maxCFF2ArgumentStack+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseDict(tt.data); err == nil {
				t.Fatal("ParseDict unexpectedly accepted malformed operand stack")
			}
		})
	}
}

func TestDecodeCharString(t *testing.T) {
	// Simple charstring: 100 100 rmoveto 50 0 lineto 0 50 lineto -50 0 lineto endchar
	// Encodings:
	// 100: 100 + 139 = 239
	// 50: 50 + 139 = 189
	// 0: 0 + 139 = 139
	// -50: -50 + 139 = 89
	// rmoveto: 21
	// lineto: 5
	// endchar: 14
	data := []byte{
		239, 239, 21, // 100 100 rmoveto
		189, 139, 5, // 50 0 lineto
		139, 189, 5, // 0 50 lineto
		89, 139, 5, // -50 0 lineto
		14, // endchar
	}

	outline, err := DecodeCharString(data, nil, nil, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}

	if len(outline.Points) != 4 {
		t.Errorf("expected 4 points, got %d", len(outline.Points))
	}

	// Point 0: (100, 100) -> 6400, 6400
	if outline.Points[0].X != 6400 || outline.Points[0].Y != 6400 {
		t.Errorf("point 0 wrong: %v", outline.Points[0])
	}
	// Point 1: (150, 100) -> 9600, 6400
	if outline.Points[1].X != 9600 || outline.Points[1].Y != 6400 {
		t.Errorf("point 1 wrong: %v", outline.Points[1])
	}
}

func TestDecodeCharStringLineAndCurveOperators(t *testing.T) {
	n := csNumber
	data := []byte{
		n(0), n(0), 21, // rmoveto
		n(10), n(20), n(30), 6, // hlineto
		n(5), n(7), 7, // vlineto
		n(10), n(0), n(10), n(10), n(0), n(10), 8, // rrcurveto
		n(10), n(20), n(30), n(40), 31, // hvcurveto
		n(10), n(20), n(30), n(40), 30, // vhcurveto
		14,
	}

	outline, err := DecodeCharString(data, nil, nil, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}
	if len(outline.Points) != 15 {
		t.Fatalf("expected 15 points, got %d", len(outline.Points))
	}
	assertPoint(t, outline, 0, 0, 0)
	assertPoint(t, outline, 3, 40, 20)
	assertPoint(t, outline, 5, 47, 25)
	assertPoint(t, outline, 8, 67, 45)
	assertPoint(t, outline, 11, 97, 115)
	assertPoint(t, outline, 14, 157, 155)
}

func TestDecodeCharStringPreservesNativeCubicSegments(t *testing.T) {
	n := csNumber
	data := []byte{
		n(0), n(0), 21,
		n(10), n(20), n(30), n(40), n(50), n(60), 8,
		14,
	}

	result, err := decodeCharString(data, charStringDecodeOptions{})
	if err != nil {
		t.Fatalf("decodeCharString failed: %v", err)
	}
	if len(result.outline.Points) != 4 {
		t.Fatalf("cubic outline points = %d, want 4", len(result.outline.Points))
	}
	if len(result.segments) != 2 {
		t.Fatalf("segments = %d, want 2", len(result.segments))
	}
	if result.segments[0].kind != charStringMoveSegment {
		t.Fatalf("segment 0 kind = %d, want move", result.segments[0].kind)
	}
	if result.segments[1].kind != charStringCubicSegment {
		t.Fatalf("segment 1 kind = %d, want cubic", result.segments[1].kind)
	}

	cubic := result.segments[1]
	assertVector(t, cubic.from, 0, 0)
	assertVector(t, cubic.ctrl1, 10, 20)
	assertVector(t, cubic.ctrl2, 40, 60)
	assertVector(t, cubic.to, 90, 120)
	if got, want := result.outline.Tags, []byte{1, 2, 2, 1}; !bytes.Equal(got, want) {
		t.Fatalf("outline tags = %v, want %v", got, want)
	}
	assertPoint(t, result.outline, 3, 90, 120)
}

func TestDecodeCharStringCurveOperatorSegments(t *testing.T) {
	n := csNumber

	t.Run("rcurveline", func(t *testing.T) {
		data := []byte{
			n(0), n(0), 21,
			n(10), n(0), n(0), n(10), n(20), n(0),
			n(5), n(5), n(5), n(-5), n(10), n(0),
			n(7), n(8), 24,
			14,
		}

		result, err := decodeCharString(data, charStringDecodeOptions{})
		if err != nil {
			t.Fatalf("decodeCharString failed: %v", err)
		}
		if len(result.segments) != 4 {
			t.Fatalf("segments = %d, want 4", len(result.segments))
		}
		assertCubicSegment(t, result.segments[1], 0, 0, 10, 0, 10, 10, 30, 10)
		assertCubicSegment(t, result.segments[2], 30, 10, 35, 15, 40, 10, 50, 10)
		assertLineSegment(t, result.segments[3], 50, 10, 57, 18)
		assertPoint(t, result.outline, len(result.outline.Points)-1, 57, 18)
	})

	t.Run("rlinecurve", func(t *testing.T) {
		data := []byte{
			n(0), n(0), 21,
			n(10), n(0), n(0), n(5),
			n(5), n(0), n(0), n(5), n(10), n(0), 25,
			14,
		}

		result, err := decodeCharString(data, charStringDecodeOptions{})
		if err != nil {
			t.Fatalf("decodeCharString failed: %v", err)
		}
		if len(result.segments) != 4 {
			t.Fatalf("segments = %d, want 4", len(result.segments))
		}
		assertLineSegment(t, result.segments[1], 0, 0, 10, 0)
		assertLineSegment(t, result.segments[2], 10, 0, 10, 5)
		assertCubicSegment(t, result.segments[3], 10, 5, 15, 5, 15, 10, 25, 10)
		assertPoint(t, result.outline, len(result.outline.Points)-1, 25, 10)
	})

	t.Run("vvcurveto optional dx1", func(t *testing.T) {
		data := []byte{
			n(0), n(0), 21,
			n(3),
			n(10), n(5), n(6), n(20),
			n(7), n(8), n(9), n(10), 26,
			14,
		}

		result, err := decodeCharString(data, charStringDecodeOptions{})
		if err != nil {
			t.Fatalf("decodeCharString failed: %v", err)
		}
		if len(result.segments) != 3 {
			t.Fatalf("segments = %d, want 3", len(result.segments))
		}
		assertCubicSegment(t, result.segments[1], 0, 0, 3, 10, 8, 16, 8, 36)
		assertCubicSegment(t, result.segments[2], 8, 36, 8, 43, 16, 52, 16, 62)
		assertPoint(t, result.outline, len(result.outline.Points)-1, 16, 62)
	})

	t.Run("hhcurveto optional dy1", func(t *testing.T) {
		data := []byte{
			n(0), n(0), 21,
			n(4),
			n(10), n(5), n(6), n(20),
			n(7), n(8), n(9), n(10), 27,
			14,
		}

		result, err := decodeCharString(data, charStringDecodeOptions{})
		if err != nil {
			t.Fatalf("decodeCharString failed: %v", err)
		}
		if len(result.segments) != 3 {
			t.Fatalf("segments = %d, want 3", len(result.segments))
		}
		assertCubicSegment(t, result.segments[1], 0, 0, 10, 4, 15, 10, 35, 10)
		assertCubicSegment(t, result.segments[2], 35, 10, 42, 10, 50, 19, 60, 19)
		assertPoint(t, result.outline, len(result.outline.Points)-1, 60, 19)
	})
}

func TestDecodeCharStringFlexOperators(t *testing.T) {
	n := csNumber
	data := []byte{
		n(0), n(0), 21, // rmoveto
		n(10), n(20), n(5), n(30), n(40), n(50), n(60), 12, 34, // hflex
		n(10), n(2), n(20), n(3), n(30), n(4), n(40), n(5), n(50), n(6), n(7), 12, 37, // flex1
		14,
	}

	outline, err := DecodeCharString(data, nil, nil, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}
	if len(outline.Points) != 13 {
		t.Fatalf("expected 13 points, got %d", len(outline.Points))
	}
	assertPoint(t, outline, 6, 210, 0)
	assertPoint(t, outline, 12, 367, 0)
}

func TestDecodeCharStringIgnoresDotsection(t *testing.T) {
	n := csNumber
	data := []byte{
		n(0), n(0), 21,
		12, 0, // deprecated dotsection no-op
		n(10), n(0), 5,
		14,
	}

	result, err := decodeCharString(data, charStringDecodeOptions{})
	if err != nil {
		t.Fatalf("decodeCharString failed: %v", err)
	}
	if len(result.segments) != 2 {
		t.Fatalf("segments = %d, want 2", len(result.segments))
	}
	assertLineSegment(t, result.segments[1], 0, 0, 10, 0)
}

func TestDecodeCharStringFlexOperatorSegments(t *testing.T) {
	n := csNumber

	t.Run("flex", func(t *testing.T) {
		data := []byte{
			n(0), n(0), 21,
			n(1), n(2), n(3), n(4), n(5), n(6),
			n(7), n(8), n(9), n(10), n(11), n(12),
			n(99), 12, 35,
			14,
		}

		result, err := decodeCharString(data, charStringDecodeOptions{})
		if err != nil {
			t.Fatalf("decodeCharString failed: %v", err)
		}
		if len(result.segments) != 3 {
			t.Fatalf("segments = %d, want 3", len(result.segments))
		}
		assertCubicSegment(t, result.segments[1], 0, 0, 1, 2, 4, 6, 9, 12)
		assertCubicSegment(t, result.segments[2], 9, 12, 16, 20, 25, 30, 36, 42)
		assertPoint(t, result.outline, len(result.outline.Points)-1, 36, 42)
	})

	t.Run("hflex1", func(t *testing.T) {
		data := []byte{
			n(0), n(0), 21,
			n(10), n(2), n(20), n(3), n(30),
			n(40), n(50), n(4), n(60), 12, 36,
			14,
		}

		result, err := decodeCharString(data, charStringDecodeOptions{})
		if err != nil {
			t.Fatalf("decodeCharString failed: %v", err)
		}
		if len(result.segments) != 3 {
			t.Fatalf("segments = %d, want 3", len(result.segments))
		}
		assertCubicSegment(t, result.segments[1], 0, 0, 10, 2, 30, 5, 60, 5)
		assertCubicSegment(t, result.segments[2], 60, 5, 100, 5, 150, 9, 210, 0)
		assertPoint(t, result.outline, len(result.outline.Points)-1, 210, 0)
	})
}

func TestDecodeCharStringSubrBias(t *testing.T) {
	n := csNumber
	localSubrs := testIndex([]byte{n(10), n(0), 5, 11})
	globalSubrs := testIndex([]byte{n(0), n(20), 5, 11})
	data := []byte{
		n(0), n(0), 21,
		n(-107), 10, // local subr index 0 with bias 107
		n(-107), 29, // global subr index 0 with bias 107
		14,
	}

	outline, err := DecodeCharString(data, globalSubrs, localSubrs, nil)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}
	if len(outline.Points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(outline.Points))
	}
	assertPoint(t, outline, 2, 10, 20)
}

func TestDecodeCharStringRejectsSubrEOFWithoutReturn(t *testing.T) {
	n := csNumber

	t.Run("local", func(t *testing.T) {
		localSubrs := testIndex([]byte{n(10), n(0), 5})
		data := []byte{
			n(0), n(0), 21,
			n(-107), 10,
			14,
		}

		_, err := DecodeCharString(data, nil, localSubrs, nil)
		if err == nil || !strings.Contains(err.Error(), "CFF local subr") {
			t.Fatalf("DecodeCharString error = %v, want local subr missing return", err)
		}
	})

	t.Run("global", func(t *testing.T) {
		globalSubrs := testIndex([]byte{n(0), n(10), 5})
		data := []byte{
			n(0), n(0), 21,
			n(-107), 29,
			14,
		}

		_, err := DecodeCharString(data, globalSubrs, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "CFF global subr") {
			t.Fatalf("DecodeCharString error = %v, want global subr missing return", err)
		}
	})
}

func TestDecodeCharStringRejectsTopLevelReturn(t *testing.T) {
	_, err := DecodeCharString([]byte{11}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "return in top-level charstring") {
		t.Fatalf("DecodeCharString error = %v, want top-level return rejection", err)
	}
}

func TestDecodeCharStringRejectsCFF1EOFWithoutEndchar(t *testing.T) {
	n := csNumber
	_, err := DecodeCharString([]byte{n(0), n(0), 21}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "missing endchar") {
		t.Fatalf("DecodeCharString error = %v, want missing endchar", err)
	}
}

func TestCharStringWidthHintsAndMasks(t *testing.T) {
	n := csNumber
	data := []byte{
		n(55), n(0), n(10), 1, // width + hstem
		19, 0xff, // hintmask with one byte that must be skipped
		n(0), n(0), 21,
		14,
	}
	ctx := &charStringContext{outline: &core.Outline{}}

	if err := ctx.interpret(data); err != nil {
		t.Fatalf("interpret failed: %v", err)
	}
	if !ctx.widthParsed {
		t.Fatalf("expected width to be parsed")
	}
	if ctx.hintCount != 1 {
		t.Fatalf("expected one stem hint, got %d", ctx.hintCount)
	}
	if len(ctx.stack) != 0 {
		t.Fatalf("expected empty stack, got %v", ctx.stack)
	}
	assertPoint(t, ctx.outline, 0, 0, 0)
}

func TestCharStringArithmeticAndLogicalOperators(t *testing.T) {
	n := csNumber
	data := []byte{
		n(10), n(5), 12, 10, // add
		n(20), n(3), 12, 11, // sub
		n(4), n(6), 12, 24, // mul
		n(22), n(7), 12, 12, // div
		n(-9), 12, 9, // abs
		n(9), 12, 14, // neg
		n(9), 12, 26, // sqrt
		n(1), n(0), 12, 3, // and
		n(1), n(0), 12, 4, // or
		n(0), 12, 5, // not
		n(5), n(5), 12, 15, // eq
	}

	ctx := interpretStack(t, data)
	assertStack(t, ctx.stack, []float64{
		15,
		17,
		24,
		float64(22) / 7,
		9,
		-9,
		3,
		0,
		1,
		1,
		1,
	})
}

func TestCharStringStackOperators(t *testing.T) {
	n := csNumber
	tests := []struct {
		name string
		data []byte
		want []float64
	}{
		{name: "drop", data: []byte{n(1), n(2), 12, 18}, want: []float64{1}},
		{name: "dup", data: []byte{n(7), 12, 27}, want: []float64{7, 7}},
		{name: "exch", data: []byte{n(1), n(2), 12, 28}, want: []float64{2, 1}},
		{name: "index", data: []byte{n(10), n(20), n(30), n(1), 12, 29}, want: []float64{10, 20, 30, 20}},
		{name: "index clamps negative", data: []byte{n(10), n(20), n(-1), 12, 29}, want: []float64{10, 20, 20}},
		{name: "index clamps high", data: []byte{n(10), n(20), n(9), 12, 29}, want: []float64{10, 20, 10}},
		{name: "roll positive", data: []byte{n(1), n(2), n(3), n(4), n(4), n(1), 12, 30}, want: []float64{4, 1, 2, 3}},
		{name: "roll negative", data: []byte{n(1), n(2), n(3), n(4), n(4), n(-1), 12, 30}, want: []float64{2, 3, 4, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := interpretStack(t, tt.data)
			assertStack(t, ctx.stack, tt.want)
		})
	}
}

func TestCharStringStorageIfelseRandomAndVsindex(t *testing.T) {
	n := csNumber

	t.Run("put get", func(t *testing.T) {
		ctx := interpretStack(t, []byte{n(42), n(3), 12, 20, n(3), 12, 21})
		assertStack(t, ctx.stack, []float64{42})
	})

	t.Run("ifelse true", func(t *testing.T) {
		ctx := interpretStack(t, []byte{n(10), n(20), n(3), n(4), 12, 22})
		assertStack(t, ctx.stack, []float64{10})
	})

	t.Run("ifelse false", func(t *testing.T) {
		ctx := interpretStack(t, []byte{n(10), n(20), n(4), n(3), 12, 22})
		assertStack(t, ctx.stack, []float64{20})
	})

	t.Run("random", func(t *testing.T) {
		ctx := interpretStack(t, []byte{12, 23, 12, 23})
		if len(ctx.stack) != 2 {
			t.Fatalf("expected two random values, got %v", ctx.stack)
		}
		for _, v := range ctx.stack {
			if v <= 0 || v > 1 {
				t.Fatalf("random value out of range: %v", ctx.stack)
			}
		}
		if ctx.stack[0] == ctx.stack[1] {
			t.Fatalf("expected deterministic sequence to advance, got %v", ctx.stack)
		}
	})

	t.Run("vsindex and blend", func(t *testing.T) {
		ctx := &charStringContext{outline: &core.Outline{}, blendVector: []float64{0.5}}
		data := []byte{
			n(2), 15, // vsindex
			n(10), n(20), n(4), n(-2), n(2), 16, // blend
		}
		if err := ctx.interpret(data); err != nil {
			t.Fatalf("interpret failed: %v", err)
		}
		if ctx.vsIndex != 2 {
			t.Fatalf("vsIndex = %d, want 2", ctx.vsIndex)
		}
		assertStack(t, ctx.stack, []float64{12, 19})
	})
}

func TestLoadGlyphOutlineDirectFixture(t *testing.T) {
	n := csNumber
	face := &CFF{
		CharStringsIndex: *testIndex([]byte{
			n(10), n(20), 21,
			n(5), n(0), 5,
			14,
		}),
	}

	outline, err := face.LoadGlyphOutline(0)
	if err != nil {
		t.Fatalf("LoadGlyphOutline failed: %v", err)
	}
	if len(outline.Points) != 2 {
		t.Fatalf("expected two points, got %d", len(outline.Points))
	}
	assertPoint(t, outline, 0, 10, 20)
	assertPoint(t, outline, 1, 15, 20)

	if _, err := face.LoadGlyphOutline(1); err == nil {
		t.Fatalf("LoadGlyphOutline succeeded for out-of-range glyph")
	}
}

func TestCalculateBias(t *testing.T) {
	tests := []struct {
		count int
		want  int
	}{
		{0, 107},
		{1239, 107},
		{1240, 1131},
		{33899, 1131},
		{33900, 32768},
	}

	for _, tt := range tests {
		if got := calculateBias(tt.count); got != tt.want {
			t.Fatalf("calculateBias(%d) = %d, want %d", tt.count, got, tt.want)
		}
	}
}

func TestDecodeCharStringRejectsTruncatedNumbers(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "short number", data: []byte{28, 0}},
		{name: "positive short number", data: []byte{247}},
		{name: "negative short number", data: []byte{251}},
		{name: "fixed number", data: []byte{255, 0, 1, 2}},
		{name: "escape operator", data: []byte{12}},
		{name: "hintmask", data: []byte{139, 139, 1, 19}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeCharString(tt.data, nil, nil, nil); err == nil {
				t.Fatalf("DecodeCharString succeeded for truncated %s", tt.name)
			}
		})
	}
}

func TestDecodeCharStringRejectsOperandUnderflow(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "rmoveto missing operands", data: []byte{21}},
		{name: "lineto odd operands", data: []byte{139, 5}},
		{name: "hlineto missing operands", data: []byte{6}},
		{name: "rrcurveto short operands", data: []byte{139, 139, 139, 139, 139, 8}},
		{name: "hvcurveto short operands", data: []byte{139, 139, 139, 31}},
		{name: "flex short operands", data: []byte{139, 139, 12, 35}},
		{name: "callsubr missing operand", data: []byte{10}},
		{name: "callgsubr missing operand", data: []byte{29}},
		{name: "blend missing count", data: []byte{16}},
		{name: "vsindex missing operand", data: []byte{15}},
		{name: "add missing operand", data: []byte{139, 12, 10}},
		{name: "drop missing operand", data: []byte{12, 18}},
		{name: "dup missing operand", data: []byte{12, 27}},
		{name: "exch missing operand", data: []byte{139, 12, 28}},
		{name: "index missing operand", data: []byte{139, 12, 29}},
		{name: "roll missing operand", data: []byte{139, 12, 30}},
		{name: "put missing operand", data: []byte{139, 12, 20}},
		{name: "get missing operand", data: []byte{12, 21}},
		{name: "ifelse missing operand", data: []byte{139, 139, 139, 12, 22}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeCharString(tt.data, nil, nil, []float64{1}); err == nil {
				t.Fatalf("DecodeCharString succeeded for %s", tt.name)
			}
		})
	}
}

func TestDecodeCharStringRejectsNonIntegerIntegerOperands(t *testing.T) {
	n := csNumber
	half := []byte{255, 0, 0, 0x80, 0}
	tests := []struct {
		name        string
		data        []byte
		globalSubrs *Index
		localSubrs  *Index
		blendVector []float64
		wantErr     string
	}{
		{
			name:       "callsubr index",
			data:       append(append([]byte{}, half...), 10),
			localSubrs: testIndex([]byte{11}),
			wantErr:    "invalid subroutine index in callsubr",
		},
		{
			name:        "callgsubr index",
			data:        append(append([]byte{}, half...), 29),
			globalSubrs: testIndex([]byte{11}),
			wantErr:     "invalid subroutine index in callgsubr",
		},
		{
			name:        "blend count",
			data:        append(append([]byte{}, half...), 16),
			blendVector: []float64{1},
			wantErr:     "invalid operand count in blend",
		},
		{
			name:    "vsindex",
			data:    append(append([]byte{}, half...), 15),
			wantErr: "invalid variation store index in vsindex",
		},
		{
			name:    "index",
			data:    append([]byte{n(10), n(20)}, append(half, 12, 29)...),
			wantErr: "invalid stack index in index",
		},
		{
			name:    "roll count",
			data:    append([]byte{n(1), n(2)}, append(half, n(1), 12, 30)...),
			wantErr: "invalid roll count in roll",
		},
		{
			name:    "roll shift",
			data:    append([]byte{n(1), n(2), n(2)}, append(half, 12, 30)...),
			wantErr: "invalid roll shift in roll",
		},
		{
			name:    "put index",
			data:    append([]byte{n(10)}, append(half, 12, 20)...),
			wantErr: "invalid transient array index in put",
		},
		{
			name:    "get index",
			data:    append(append([]byte{}, half...), 12, 21),
			wantErr: "invalid transient array index in get",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeCharString(tt.data, tt.globalSubrs, tt.localSubrs, tt.blendVector)
			if err == nil {
				t.Fatal("DecodeCharString unexpectedly accepted non-integer operand")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeCharStringRejectsOperandStackOverflow(t *testing.T) {
	n := csNumber
	tests := []struct {
		name string
		data []byte
	}{
		{name: "number push", data: bytes.Repeat([]byte{n(0)}, maxCFF1ArgumentStack+1)},
		{name: "dup push", data: append(bytes.Repeat([]byte{n(0)}, maxCFF1ArgumentStack), 12, 27)},
		{name: "random push", data: append(bytes.Repeat([]byte{n(0)}, maxCFF1ArgumentStack), 12, 23)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeCharString(tt.data, nil, nil, nil); err == nil {
				t.Fatalf("DecodeCharString succeeded for %s stack overflow", tt.name)
			}
		})
	}
}

func TestDecodeCharStringCFF2OperandStackLimit(t *testing.T) {
	n := csNumber
	if _, err := decodeCharString(bytes.Repeat([]byte{n(0)}, maxCFF1ArgumentStack+1), charStringDecodeOptions{
		maxStack: maxCFF2ArgumentStack,
	}); err != nil {
		t.Fatalf("CFF2-sized stack failed below configured limit: %v", err)
	}
	if _, err := decodeCharString(bytes.Repeat([]byte{n(0)}, maxCFF2ArgumentStack+1), charStringDecodeOptions{
		maxStack: maxCFF2ArgumentStack,
	}); err == nil {
		t.Fatal("decodeCharString accepted CFF2 operand stack overflow")
	}
}

func TestDecodeCharStringRejectsCFF2Endchar(t *testing.T) {
	_, err := decodeCharString([]byte{14}, charStringDecodeOptions{
		maxStack: maxCFF2ArgumentStack,
	})
	if err == nil || !strings.Contains(err.Error(), "CFF2") {
		t.Fatalf("decodeCharString error = %v, want CFF2 endchar rejection", err)
	}
}

func TestDecodeCharStringRejectsMalformedBounds(t *testing.T) {
	n := csNumber

	t.Run("unsupported single-byte operator", func(t *testing.T) {
		if _, err := DecodeCharString([]byte{0}, nil, nil, nil); err == nil {
			t.Fatalf("DecodeCharString succeeded for unsupported operator")
		}
	})

	t.Run("unsupported escaped operator", func(t *testing.T) {
		if _, err := DecodeCharString([]byte{12, 1}, nil, nil, nil); err == nil {
			t.Fatalf("DecodeCharString succeeded for unsupported escaped operator")
		}
	})

	t.Run("local subr index out of range", func(t *testing.T) {
		localSubrs := testIndex([]byte{11})
		if _, err := DecodeCharString([]byte{n(-106), 10}, nil, localSubrs, nil); err == nil {
			t.Fatalf("DecodeCharString succeeded for out-of-range local subr")
		}
	})

	t.Run("global subr index out of range", func(t *testing.T) {
		globalSubrs := testIndex([]byte{11})
		if _, err := DecodeCharString([]byte{n(-106), 29}, globalSubrs, nil, nil); err == nil {
			t.Fatalf("DecodeCharString succeeded for out-of-range global subr")
		}
	})

	t.Run("recursive local subr nesting", func(t *testing.T) {
		localSubrs := testIndex([]byte{n(-107), 10, 11})
		if _, err := DecodeCharString([]byte{n(-107), 10}, nil, localSubrs, nil); err == nil {
			t.Fatalf("DecodeCharString succeeded for recursive local subr")
		}
	})

	t.Run("vsindex out of variation store range", func(t *testing.T) {
		_, err := decodeCharString([]byte{n(2), 15}, charStringDecodeOptions{
			variationStore: &VariationStore{ItemVariationRegionCounts: []uint16{1}},
		})
		if err == nil {
			t.Fatalf("decodeCharString succeeded for out-of-range vsindex")
		}
	})

	t.Run("blend uses variation store region count for bounds", func(t *testing.T) {
		_, err := decodeCharString([]byte{n(10), n(1), 16}, charStringDecodeOptions{
			variationStore: &VariationStore{ItemVariationRegionCounts: []uint16{2}},
		})
		if err == nil {
			t.Fatalf("decodeCharString succeeded for undersized blend stack")
		}
	})

	t.Run("transient array put index out of range", func(t *testing.T) {
		if _, err := DecodeCharString([]byte{n(1), n(32), 12, 20}, nil, nil, nil); err == nil {
			t.Fatalf("DecodeCharString succeeded for out-of-range put")
		}
	})
}
