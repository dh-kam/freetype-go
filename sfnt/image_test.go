package sfnt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/color"
	"github.com/dh-kam/freetype-go/core"
)

type MockImageDecoder struct{}

func (m *MockImageDecoder) Decode(data []byte) (*api.Image, error) {
	if bytes.Equal(data, []byte("mock_png_data")) {
		return &api.Image{Width: 10, Height: 10, Pixels: []byte{1, 2, 3}}, nil
	}
	return nil, errors.New("invalid data")
}

type recordingImageDecoder struct {
	last []byte
}

func (d *recordingImageDecoder) Decode(data []byte) (*api.Image, error) {
	d.last = append(d.last[:0], data...)
	return &api.Image{Width: len(data), Height: 1, Pixels: append([]byte(nil), data...)}, nil
}

type MockStream struct {
	data []byte
}

func (m *MockStream) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(m.data)) {
		return 0, errors.New("EOF")
	}
	n := copy(p, m.data[off:])
	return n, nil
}

func (m *MockStream) Size() int64 {
	return int64(len(m.data))
}

func cblcWithIndexSubtable(firstGlyph, lastGlyph uint16, subtable []byte) []byte {
	cblcBuf := new(bytes.Buffer)
	binary.Write(cblcBuf, binary.BigEndian, uint16(2)) // major
	binary.Write(cblcBuf, binary.BigEndian, uint16(0)) // minor
	binary.Write(cblcBuf, binary.BigEndian, uint32(1)) // numSizes

	binary.Write(cblcBuf, binary.BigEndian, uint32(8+48))            // IndexSubTableArrayOffset
	binary.Write(cblcBuf, binary.BigEndian, uint32(8+len(subtable))) // indexTablesSize
	binary.Write(cblcBuf, binary.BigEndian, uint32(1))               // numberOfIndexSubTables
	binary.Write(cblcBuf, binary.BigEndian, uint32(0))               // colorRef
	cblcBuf.Write(make([]byte, 24))                                  // metrics
	binary.Write(cblcBuf, binary.BigEndian, firstGlyph)              // startGlyph
	binary.Write(cblcBuf, binary.BigEndian, lastGlyph)               // endGlyph
	cblcBuf.Write([]byte{12, 12, 8, 1})                              // ppem, etc
	binary.Write(cblcBuf, binary.BigEndian, firstGlyph)              // firstGlyph
	binary.Write(cblcBuf, binary.BigEndian, lastGlyph)               // lastGlyph
	binary.Write(cblcBuf, binary.BigEndian, uint32(8))               // additionalOffset
	cblcBuf.Write(subtable)

	return cblcBuf.Bytes()
}

type sbixStrikeFixture struct {
	ppem    uint16
	payload []byte
}

func sbixWithStrikes(strikes []sbixStrikeFixture) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint16(1))            // version
	binary.Write(buf, binary.BigEndian, uint16(0))            // flags
	binary.Write(buf, binary.BigEndian, uint32(len(strikes))) // numStrikes

	strikeData := new(bytes.Buffer)
	strikeBase := 8 + len(strikes)*4
	for _, strike := range strikes {
		binary.Write(buf, binary.BigEndian, uint32(strikeBase+strikeData.Len()))
		record := new(bytes.Buffer)
		binary.Write(record, binary.BigEndian, strike.ppem)
		binary.Write(record, binary.BigEndian, uint16(72)) // ppi
		binary.Write(record, binary.BigEndian, uint32(12))
		binary.Write(record, binary.BigEndian, uint32(12+8+len(strike.payload)))
		binary.Write(record, binary.BigEndian, int16(0))           // originX
		binary.Write(record, binary.BigEndian, int16(0))           // originY
		binary.Write(record, binary.BigEndian, uint32(0x706e6720)) // 'png '
		record.Write(strike.payload)
		strikeData.Write(record.Bytes())
	}
	buf.Write(strikeData.Bytes())
	return buf.Bytes()
}

type cblcStrikeFixture struct {
	firstGlyph      uint16
	lastGlyph       uint16
	ppem            byte
	imageDataOffset uint32
	imageDataLength uint32
}

func cblcWithStrikes(strikes []cblcStrikeFixture) []byte {
	cblcBuf := new(bytes.Buffer)
	binary.Write(cblcBuf, binary.BigEndian, uint16(2))            // major
	binary.Write(cblcBuf, binary.BigEndian, uint16(0))            // minor
	binary.Write(cblcBuf, binary.BigEndian, uint32(len(strikes))) // numSizes

	sizeRecords := make([][]byte, len(strikes))
	indexData := new(bytes.Buffer)
	indexDataBase := 8 + len(strikes)*48
	for i, strike := range strikes {
		subtable := new(bytes.Buffer)
		binary.Write(subtable, binary.BigEndian, uint16(1)) // indexFormat
		binary.Write(subtable, binary.BigEndian, uint16(5)) // imageFormat
		binary.Write(subtable, binary.BigEndian, strike.imageDataOffset)
		binary.Write(subtable, binary.BigEndian, uint32(0))
		binary.Write(subtable, binary.BigEndian, strike.imageDataLength)

		arrayOffset := uint32(indexDataBase + indexData.Len())
		record := new(bytes.Buffer)
		binary.Write(record, binary.BigEndian, arrayOffset)
		binary.Write(record, binary.BigEndian, uint32(8+subtable.Len())) // indexTablesSize
		binary.Write(record, binary.BigEndian, uint32(1))                // numberOfIndexSubTables
		binary.Write(record, binary.BigEndian, uint32(0))                // colorRef
		record.Write(make([]byte, 24))                                   // metrics
		binary.Write(record, binary.BigEndian, strike.firstGlyph)
		binary.Write(record, binary.BigEndian, strike.lastGlyph)
		record.Write([]byte{strike.ppem, strike.ppem, 8, 1})
		sizeRecords[i] = record.Bytes()

		binary.Write(indexData, binary.BigEndian, strike.firstGlyph)
		binary.Write(indexData, binary.BigEndian, strike.lastGlyph)
		binary.Write(indexData, binary.BigEndian, uint32(8)) // additionalOffset
		indexData.Write(subtable.Bytes())
	}
	for _, record := range sizeRecords {
		cblcBuf.Write(record)
	}
	cblcBuf.Write(indexData.Bytes())
	return cblcBuf.Bytes()
}

func embeddedBitmapRecord(metricsLen int, data []byte) []byte {
	record := make([]byte, metricsLen+len(data))
	copy(record[metricsLen:], data)
	return record
}

func embeddedPNGRecord(metricsLen int, data []byte) []byte {
	record := new(bytes.Buffer)
	record.Write(make([]byte, metricsLen))
	binary.Write(record, binary.BigEndian, uint32(len(data)))
	record.Write(data)
	return record.Bytes()
}

func TestSbixImage(t *testing.T) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint16(1))  // version
	binary.Write(buf, binary.BigEndian, uint16(1))  // flags
	binary.Write(buf, binary.BigEndian, uint32(1))  // numStrikes
	binary.Write(buf, binary.BigEndian, uint32(12)) // strike offset

	// Strike
	binary.Write(buf, binary.BigEndian, uint16(109)) // ppem
	binary.Write(buf, binary.BigEndian, uint16(72))  // ppi
	binary.Write(buf, binary.BigEndian, uint32(12))  // glyphDataOffset[0]
	binary.Write(buf, binary.BigEndian, uint32(33))  // glyphDataOffset[1]

	// GlyphData
	binary.Write(buf, binary.BigEndian, int16(0))           // originX
	binary.Write(buf, binary.BigEndian, int16(0))           // originY
	binary.Write(buf, binary.BigEndian, uint32(0x706e6720)) // 'png '
	buf.Write([]byte("mock_png_data"))

	sys := core.NewSystem()
	sys.SetImageDecoder(&MockImageDecoder{})

	face := &Face{
		sys: sys,
	}
	sbix, err := parseSbix(&MockStream{data: buf.Bytes()})
	if err != nil {
		t.Fatalf("parseSbix failed: %v", err)
	}
	face.sbix = &sbix

	slot, err := face.LoadGlyph(0, 0)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}

	img := slot.GetImage()
	if img == nil {
		t.Fatalf("Expected image, got nil")
	}
	if img.Width != 10 {
		t.Errorf("Expected width 10, got %d", img.Width)
	}
}

func TestCBLCCBDTImage(t *testing.T) {
	cblcBuf := new(bytes.Buffer)
	binary.Write(cblcBuf, binary.BigEndian, uint16(2)) // major
	binary.Write(cblcBuf, binary.BigEndian, uint16(0)) // minor
	binary.Write(cblcBuf, binary.BigEndian, uint32(1)) // numSizes

	// Size table (48 bytes)
	binary.Write(cblcBuf, binary.BigEndian, uint32(8+48)) // IndexSubTableArrayOffset
	binary.Write(cblcBuf, binary.BigEndian, uint32(8))    // indexTablesSize
	binary.Write(cblcBuf, binary.BigEndian, uint32(1))    // numberOfIndexSubTables
	binary.Write(cblcBuf, binary.BigEndian, uint32(0))    // colorRef
	cblcBuf.Write(make([]byte, 24))                       // metrics
	binary.Write(cblcBuf, binary.BigEndian, uint16(0))    // startGlyph
	binary.Write(cblcBuf, binary.BigEndian, uint16(0))    // endGlyph
	cblcBuf.Write([]byte{12, 12, 8, 1})                   // ppem, etc

	// IndexSubTableArray
	binary.Write(cblcBuf, binary.BigEndian, uint16(0)) // firstGlyph
	binary.Write(cblcBuf, binary.BigEndian, uint16(0)) // lastGlyph
	binary.Write(cblcBuf, binary.BigEndian, uint32(8)) // additionalOffset

	// IndexSubTable (Format 1)
	binary.Write(cblcBuf, binary.BigEndian, uint16(1))  // indexFormat
	binary.Write(cblcBuf, binary.BigEndian, uint16(17)) // imageFormat
	binary.Write(cblcBuf, binary.BigEndian, uint32(0))  // imageDataOffset
	binary.Write(cblcBuf, binary.BigEndian, uint32(0))  // offsetArray[0]
	binary.Write(cblcBuf, binary.BigEndian, uint32(22)) // offsetArray[1]

	cbdtBuf := new(bytes.Buffer)
	cbdtBuf.Write(embeddedPNGRecord(5, []byte("mock_png_data")))

	sys := core.NewSystem()
	sys.SetImageDecoder(&MockImageDecoder{})

	face := &Face{
		sys: sys,
	}
	cblc, err := parseCBLC(&MockStream{data: cblcBuf.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	cbdt, err := parseCBDT(&MockStream{data: cbdtBuf.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	face.cblc = &cblc
	face.cbdt = &cbdt

	slot, err := face.LoadGlyph(0, 0)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}

	img := slot.GetImage()
	if img == nil {
		t.Fatalf("Expected image, got nil")
	}
	if img.Width != 10 {
		t.Errorf("Expected width 10, got %d", img.Width)
	}
}

func TestSbixRejectsInvalidGlyphImageLength(t *testing.T) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint16(1))  // version
	binary.Write(buf, binary.BigEndian, uint16(0))  // flags
	binary.Write(buf, binary.BigEndian, uint32(1))  // numStrikes
	binary.Write(buf, binary.BigEndian, uint32(12)) // strike offset

	binary.Write(buf, binary.BigEndian, uint16(12)) // ppem
	binary.Write(buf, binary.BigEndian, uint16(72)) // ppi
	binary.Write(buf, binary.BigEndian, uint32(40)) // glyphDataOffset[0]
	binary.Write(buf, binary.BigEndian, uint32(20)) // glyphDataOffset[1]

	sbix, err := parseSbix(&MockStream{data: buf.Bytes()})
	if err != nil {
		t.Fatalf("parseSbix failed: %v", err)
	}

	if _, err := sbix.GetImage(0, 12); err == nil {
		t.Fatalf("expected invalid sbix image length error")
	}
}

func TestSbixSelectsStrikeForPPEM(t *testing.T) {
	sbix, err := parseSbix(&MockStream{data: sbixWithStrikes([]sbixStrikeFixture{
		{ppem: 12, payload: []byte("small")},
		{ppem: 24, payload: []byte("large")},
		{ppem: 48, payload: []byte("huge")},
	})})
	if err != nil {
		t.Fatalf("parseSbix failed: %v", err)
	}

	tests := []struct {
		ppem uint16
		want string
	}{
		{ppem: 0, want: "small"},
		{ppem: 10, want: "small"},
		{ppem: 24, want: "large"},
		{ppem: 40, want: "large"},
		{ppem: 50, want: "huge"},
	}
	for _, tt := range tests {
		got, err := sbix.GetImage(0, tt.ppem)
		if err != nil {
			t.Fatalf("GetImage(%d) failed: %v", tt.ppem, err)
		}
		if string(got) != tt.want {
			t.Fatalf("GetImage(%d) = %q, want %q", tt.ppem, got, tt.want)
		}
	}
}

func TestCBLCCBDTRejectsOversizedImageBeforeAllocation(t *testing.T) {
	cblcBuf := new(bytes.Buffer)
	binary.Write(cblcBuf, binary.BigEndian, uint16(2)) // major
	binary.Write(cblcBuf, binary.BigEndian, uint16(0)) // minor
	binary.Write(cblcBuf, binary.BigEndian, uint32(1)) // numSizes

	binary.Write(cblcBuf, binary.BigEndian, uint32(8+48)) // IndexSubTableArrayOffset
	binary.Write(cblcBuf, binary.BigEndian, uint32(16))   // indexTablesSize
	binary.Write(cblcBuf, binary.BigEndian, uint32(1))    // numberOfIndexSubTables
	binary.Write(cblcBuf, binary.BigEndian, uint32(0))    // colorRef
	cblcBuf.Write(make([]byte, 24))                       // metrics
	binary.Write(cblcBuf, binary.BigEndian, uint16(0))    // startGlyph
	binary.Write(cblcBuf, binary.BigEndian, uint16(0))    // endGlyph
	cblcBuf.Write([]byte{12, 12, 8, 1})                   // ppem, etc

	binary.Write(cblcBuf, binary.BigEndian, uint16(0)) // firstGlyph
	binary.Write(cblcBuf, binary.BigEndian, uint16(0)) // lastGlyph
	binary.Write(cblcBuf, binary.BigEndian, uint32(8)) // additionalOffset

	binary.Write(cblcBuf, binary.BigEndian, uint16(2))                            // indexFormat
	binary.Write(cblcBuf, binary.BigEndian, uint16(17))                           // imageFormat
	binary.Write(cblcBuf, binary.BigEndian, uint32(0))                            // imageDataOffset
	binary.Write(cblcBuf, binary.BigEndian, uint32(maxEmbeddedBitmapImageSize+1)) // imageSize

	cblc, err := parseCBLC(&MockStream{data: cblcBuf.Bytes()})
	if err != nil {
		t.Fatalf("parseCBLC failed: %v", err)
	}
	cbdt, err := parseCBDT(&MockStream{data: []byte("tiny")})
	if err != nil {
		t.Fatalf("parseCBDT failed: %v", err)
	}

	if _, err := GetCBLCImage(cblc, cbdt, 0); err == nil {
		t.Fatalf("expected oversized CBDT image error")
	}
}

func TestCBLCCBDTIndexFormat2ConstantSize(t *testing.T) {
	subtable := new(bytes.Buffer)
	binary.Write(subtable, binary.BigEndian, uint16(2)) // indexFormat
	binary.Write(subtable, binary.BigEndian, uint16(5))
	binary.Write(subtable, binary.BigEndian, uint32(0)) // imageDataOffset
	binary.Write(subtable, binary.BigEndian, uint32(4)) // imageSize
	subtable.Write(make([]byte, 8))                     // bigMetrics

	cblc, err := parseCBLC(&MockStream{data: cblcWithIndexSubtable(5, 7, subtable.Bytes())})
	if err != nil {
		t.Fatalf("parseCBLC failed: %v", err)
	}
	cbdt, err := parseCBDT(&MockStream{data: []byte("aaaabbbbcccc")})
	if err != nil {
		t.Fatalf("parseCBDT failed: %v", err)
	}

	got, err := GetCBLCImage(cblc, cbdt, 6)
	if err != nil {
		t.Fatalf("GetCBLCImage failed: %v", err)
	}
	if string(got) != "bbbb" {
		t.Fatalf("unexpected image data: %q", got)
	}
}

func TestCBLCCBDTIndexFormat3ShortOffsets(t *testing.T) {
	subtable := new(bytes.Buffer)
	binary.Write(subtable, binary.BigEndian, uint16(3)) // indexFormat
	binary.Write(subtable, binary.BigEndian, uint16(1))
	binary.Write(subtable, binary.BigEndian, uint32(0))  // imageDataOffset
	binary.Write(subtable, binary.BigEndian, uint16(0))  // offsetArray[0]
	binary.Write(subtable, binary.BigEndian, uint16(8))  // offsetArray[1]
	binary.Write(subtable, binary.BigEndian, uint16(18)) // offsetArray[2]

	cblc, err := parseCBLC(&MockStream{data: cblcWithIndexSubtable(1, 2, subtable.Bytes())})
	if err != nil {
		t.Fatalf("parseCBLC failed: %v", err)
	}
	cbdt, err := parseCBDT(&MockStream{data: append(embeddedBitmapRecord(5, []byte("aaa")), embeddedBitmapRecord(5, []byte("bbbbb"))...)})
	if err != nil {
		t.Fatalf("parseCBDT failed: %v", err)
	}

	got, err := GetCBLCImage(cblc, cbdt, 2)
	if err != nil {
		t.Fatalf("GetCBLCImage failed: %v", err)
	}
	if string(got) != "bbbbb" {
		t.Fatalf("unexpected image data: %q", got)
	}
}

func TestCBLCCBDTIndexFormat4SparseGlyphOffsets(t *testing.T) {
	subtable := new(bytes.Buffer)
	binary.Write(subtable, binary.BigEndian, uint16(4))  // indexFormat
	binary.Write(subtable, binary.BigEndian, uint16(1))  // imageFormat
	binary.Write(subtable, binary.BigEndian, uint32(0))  // imageDataOffset
	binary.Write(subtable, binary.BigEndian, uint32(2))  // numGlyphs
	binary.Write(subtable, binary.BigEndian, uint16(10)) // glyphID
	binary.Write(subtable, binary.BigEndian, uint16(0))  // sbitOffset
	binary.Write(subtable, binary.BigEndian, uint16(25)) // glyphID
	binary.Write(subtable, binary.BigEndian, uint16(9))  // sbitOffset
	binary.Write(subtable, binary.BigEndian, uint16(26)) // sentinel glyphID
	binary.Write(subtable, binary.BigEndian, uint16(22)) // sentinel sbitOffset

	cblc, err := parseCBLC(&MockStream{data: cblcWithIndexSubtable(10, 30, subtable.Bytes())})
	if err != nil {
		t.Fatalf("parseCBLC failed: %v", err)
	}
	cbdt, err := parseCBDT(&MockStream{data: append(embeddedBitmapRecord(5, []byte("skip")), embeddedBitmapRecord(5, []byte("fmt4data"))...)})
	if err != nil {
		t.Fatalf("parseCBDT failed: %v", err)
	}

	got, err := GetCBLCImage(cblc, cbdt, 25)
	if err != nil {
		t.Fatalf("GetCBLCImage failed: %v", err)
	}
	if string(got) != "fmt4data" {
		t.Fatalf("unexpected image data: %q", got)
	}

	got, err = GetCBLCImage(cblc, cbdt, 11)
	if err != nil {
		t.Fatalf("GetCBLCImage missing glyph failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for sparse missing glyph, got %q", got)
	}
}

func TestCBLCCBDTIndexFormat5SparseConstantMetrics(t *testing.T) {
	subtable := new(bytes.Buffer)
	binary.Write(subtable, binary.BigEndian, uint16(5)) // indexFormat
	binary.Write(subtable, binary.BigEndian, uint16(5)) // imageFormat
	binary.Write(subtable, binary.BigEndian, uint32(0)) // imageDataOffset
	binary.Write(subtable, binary.BigEndian, uint32(5)) // imageSize
	subtable.Write(make([]byte, 8))                     // bigMetrics
	binary.Write(subtable, binary.BigEndian, uint32(2)) // numGlyphs
	binary.Write(subtable, binary.BigEndian, uint16(3)) // glyphID
	binary.Write(subtable, binary.BigEndian, uint16(9)) // glyphID

	cblc, err := parseCBLC(&MockStream{data: cblcWithIndexSubtable(3, 9, subtable.Bytes())})
	if err != nil {
		t.Fatalf("parseCBLC failed: %v", err)
	}
	cbdt, err := parseCBDT(&MockStream{data: []byte("abcdeVWXYZ")})
	if err != nil {
		t.Fatalf("parseCBDT failed: %v", err)
	}

	got, err := GetCBLCImage(cblc, cbdt, 9)
	if err != nil {
		t.Fatalf("GetCBLCImage failed: %v", err)
	}
	if string(got) != "VWXYZ" {
		t.Fatalf("unexpected image data: %q", got)
	}
}

func TestCBLCCBDTSelectsStrikeForPPEM(t *testing.T) {
	cblc, err := parseCBLC(&MockStream{data: cblcWithStrikes([]cblcStrikeFixture{
		{firstGlyph: 0, lastGlyph: 0, ppem: 12, imageDataOffset: 0, imageDataLength: 5},
		{firstGlyph: 0, lastGlyph: 0, ppem: 24, imageDataOffset: 5, imageDataLength: 5},
		{firstGlyph: 0, lastGlyph: 0, ppem: 48, imageDataOffset: 10, imageDataLength: 4},
	})})
	if err != nil {
		t.Fatalf("parseCBLC failed: %v", err)
	}
	cbdt, err := parseCBDT(&MockStream{data: []byte("smalllargehuge")})
	if err != nil {
		t.Fatalf("parseCBDT failed: %v", err)
	}

	tests := []struct {
		ppem uint16
		want string
	}{
		{ppem: 0, want: "small"},
		{ppem: 10, want: "small"},
		{ppem: 24, want: "large"},
		{ppem: 40, want: "large"},
		{ppem: 50, want: "huge"},
	}
	for _, tt := range tests {
		got, err := GetCBLCImageAtPPEM(cblc, cbdt, 0, tt.ppem)
		if err != nil {
			t.Fatalf("GetCBLCImageAtPPEM(%d) failed: %v", tt.ppem, err)
		}
		if string(got) != tt.want {
			t.Fatalf("GetCBLCImageAtPPEM(%d) = %q, want %q", tt.ppem, got, tt.want)
		}
	}
}

func TestLoadGlyphCBLCUsesCurrentPPEM(t *testing.T) {
	cblc, err := parseCBLC(&MockStream{data: cblcWithStrikes([]cblcStrikeFixture{
		{firstGlyph: 0, lastGlyph: 0, ppem: 12, imageDataOffset: 0, imageDataLength: 5},
		{firstGlyph: 0, lastGlyph: 0, ppem: 24, imageDataOffset: 5, imageDataLength: 5},
	})})
	if err != nil {
		t.Fatalf("parseCBLC failed: %v", err)
	}
	cbdt, err := parseCBDT(&MockStream{data: []byte("smalllarge")})
	if err != nil {
		t.Fatalf("parseCBDT failed: %v", err)
	}
	decoder := &recordingImageDecoder{}
	sys := core.NewSystem()
	sys.SetImageDecoder(decoder)
	face := &Face{sys: sys, cblc: &cblc, cbdt: &cbdt}
	if err := face.SetPixelSizes(24, 24); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}

	slot, err := face.LoadGlyph(0, 0)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	if string(decoder.last) != "large" {
		t.Fatalf("decoded payload = %q, want large", decoder.last)
	}
	if slot.GetImage() == nil || slot.GetImage().Width != len("large") {
		t.Fatalf("image = %+v, want decoded large payload", slot.GetImage())
	}
}

func TestLoadGlyphSbixUsesCurrentPPEM(t *testing.T) {
	sbix, err := parseSbix(&MockStream{data: sbixWithStrikes([]sbixStrikeFixture{
		{ppem: 12, payload: []byte("small")},
		{ppem: 24, payload: []byte("large")},
	})})
	if err != nil {
		t.Fatalf("parseSbix failed: %v", err)
	}
	decoder := &recordingImageDecoder{}
	sys := core.NewSystem()
	sys.SetImageDecoder(decoder)
	face := &Face{sys: sys, sbix: &sbix}
	if err := face.SetPixelSizes(24, 24); err != nil {
		t.Fatalf("SetPixelSizes failed: %v", err)
	}

	slot, err := face.LoadGlyph(0, 0)
	if err != nil {
		t.Fatalf("LoadGlyph failed: %v", err)
	}
	if string(decoder.last) != "large" {
		t.Fatalf("decoded payload = %q, want large", decoder.last)
	}
	if slot.GetImage() == nil || slot.GetImage().Width != len("large") {
		t.Fatalf("image = %+v, want decoded large payload", slot.GetImage())
	}
}

func TestCBLCCBDTStripsEmbeddedBitmapHeaders(t *testing.T) {
	tests := []struct {
		name        string
		imageFormat uint16
		record      []byte
		want        string
	}{
		{name: "small metrics byte aligned", imageFormat: 1, record: embeddedBitmapRecord(5, []byte("bits")), want: "bits"},
		{name: "big metrics byte aligned", imageFormat: 6, record: embeddedBitmapRecord(8, []byte("bigbits")), want: "bigbits"},
		{name: "small metrics PNG", imageFormat: 17, record: embeddedPNGRecord(5, []byte("png17")), want: "png17"},
		{name: "big metrics PNG", imageFormat: 18, record: embeddedPNGRecord(8, []byte("png18")), want: "png18"},
		{name: "CBLC metrics PNG", imageFormat: 19, record: embeddedPNGRecord(0, []byte("png19")), want: "png19"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractEmbeddedBitmapPayload(tt.imageFormat, tt.record)
			if err != nil {
				t.Fatalf("extractEmbeddedBitmapPayload failed: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("payload = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCBLCCBDTRejectsTruncatedEmbeddedBitmapHeaders(t *testing.T) {
	if _, err := extractEmbeddedBitmapPayload(17, []byte{0, 1, 2}); err == nil {
		t.Fatal("expected truncated PNG payload error")
	}
	record := embeddedPNGRecord(0, []byte("png"))
	record[3] = 10
	if _, err := extractEmbeddedBitmapPayload(19, record); err == nil {
		t.Fatal("expected PNG length out of bounds error")
	}
}

func TestFaceGetColorLayersForPalette(t *testing.T) {
	face := &Face{
		colr: &color.COLR{
			BaseGlyphRecords: map[uint16]color.BaseGlyphRecord{
				7: {GlyphID: 7, FirstLayerIndex: 1, NumLayers: 2},
			},
			LayerRecords: []color.LayerRecord{
				{GlyphID: 99, PaletteIndex: 0},
				{GlyphID: 10, PaletteIndex: 1},
				{GlyphID: 11, PaletteIndex: 0xffff},
			},
		},
		cpal: &color.CPAL{
			NumPaletteEntries: 2,
			NumPalettes:       2,
			PaletteOffsets:    []uint16{0, 2},
			ColorRecords: []color.RGBA{
				{R: 1, G: 2, B: 3, A: 4},
				{R: 5, G: 6, B: 7, A: 8},
				{R: 9, G: 10, B: 11, A: 12},
				{R: 13, G: 14, B: 15, A: 16},
			},
		},
	}

	layers, err := face.GetColorLayersForPalette(7, 1)
	if err != nil {
		t.Fatalf("GetColorLayersForPalette failed: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}
	if layers[0].GlyphID != 10 || layers[0].Color != (color.RGBA{R: 13, G: 14, B: 15, A: 16}) {
		t.Fatalf("unexpected palette layer: %+v", layers[0])
	}
	if layers[1].GlyphID != 11 || layers[1].Color != (color.RGBA{R: 0, G: 0, B: 0, A: 255}) {
		t.Fatalf("unexpected foreground fallback layer: %+v", layers[1])
	}

	face.cpal = nil
	layers, err = face.GetColorLayers(7)
	if err != nil {
		t.Fatalf("GetColorLayers with nil CPAL failed: %v", err)
	}
	if layers[0].Color != (color.RGBA{R: 0, G: 0, B: 0, A: 255}) {
		t.Fatalf("expected nil CPAL fallback color, got %+v", layers[0].Color)
	}
}
