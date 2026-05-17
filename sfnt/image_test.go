package sfnt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

type MockImageDecoder struct{}

func (m *MockImageDecoder) Decode(data []byte) (*api.Image, error) {
	if bytes.Equal(data, []byte("mock_png_data")) {
		return &api.Image{Width: 10, Height: 10, Pixels: []byte{1, 2, 3}}, nil
	}
	return nil, errors.New("invalid data")
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
	binary.Write(cblcBuf, binary.BigEndian, uint32(13)) // offsetArray[1]

	cbdtBuf := new(bytes.Buffer)
	cbdtBuf.Write([]byte("mock_png_data"))

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
	binary.Write(subtable, binary.BigEndian, uint16(17))
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
	binary.Write(subtable, binary.BigEndian, uint16(17))
	binary.Write(subtable, binary.BigEndian, uint32(0)) // imageDataOffset
	binary.Write(subtable, binary.BigEndian, uint16(0)) // offsetArray[0]
	binary.Write(subtable, binary.BigEndian, uint16(3)) // offsetArray[1]
	binary.Write(subtable, binary.BigEndian, uint16(8)) // offsetArray[2]

	cblc, err := parseCBLC(&MockStream{data: cblcWithIndexSubtable(1, 2, subtable.Bytes())})
	if err != nil {
		t.Fatalf("parseCBLC failed: %v", err)
	}
	cbdt, err := parseCBDT(&MockStream{data: []byte("aaabbbbb")})
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
	binary.Write(subtable, binary.BigEndian, uint16(17)) // imageFormat
	binary.Write(subtable, binary.BigEndian, uint32(0))  // imageDataOffset
	binary.Write(subtable, binary.BigEndian, uint32(2))  // numGlyphs
	binary.Write(subtable, binary.BigEndian, uint16(10)) // glyphID
	binary.Write(subtable, binary.BigEndian, uint16(0))  // sbitOffset
	binary.Write(subtable, binary.BigEndian, uint16(25)) // glyphID
	binary.Write(subtable, binary.BigEndian, uint16(4))  // sbitOffset
	binary.Write(subtable, binary.BigEndian, uint16(26)) // sentinel glyphID
	binary.Write(subtable, binary.BigEndian, uint16(12)) // sentinel sbitOffset

	cblc, err := parseCBLC(&MockStream{data: cblcWithIndexSubtable(10, 30, subtable.Bytes())})
	if err != nil {
		t.Fatalf("parseCBLC failed: %v", err)
	}
	cbdt, err := parseCBDT(&MockStream{data: []byte("skipfmt4data")})
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
	binary.Write(subtable, binary.BigEndian, uint16(5))  // indexFormat
	binary.Write(subtable, binary.BigEndian, uint16(17)) // imageFormat
	binary.Write(subtable, binary.BigEndian, uint32(0))  // imageDataOffset
	binary.Write(subtable, binary.BigEndian, uint32(5))  // imageSize
	subtable.Write(make([]byte, 8))                      // bigMetrics
	binary.Write(subtable, binary.BigEndian, uint32(2))  // numGlyphs
	binary.Write(subtable, binary.BigEndian, uint16(3))  // glyphID
	binary.Write(subtable, binary.BigEndian, uint16(9))  // glyphID

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
