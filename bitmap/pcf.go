package bitmap

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

const (
	PCF_FILE_VERSION = 0x66637001 // "\x01fcp" in little-endian? No, wait.
	// PCF header is 1, 'p', 'c', 'f'
)

const (
	PCF_PROPERTIES       = 1 << 0
	PCF_ACCELERATORS     = 1 << 1
	PCF_METRICS          = 1 << 2
	PCF_BITMAPS          = 1 << 3
	PCF_INK_METRICS      = 1 << 4
	PCF_BDF_ENCODINGS    = 1 << 5
	PCF_SWIDTHS          = 1 << 6
	PCF_GLYPH_NAMES      = 1 << 7
	PCF_BDF_ACCELERATORS = 1 << 8
)

const (
	PCF_DEFAULT_FORMAT     = 0x00000000
	PCF_INKBOUNDS          = 0x00000200
	PCF_ACCEL_W_INKBOUNDS  = 0x00000100
	PCF_COMPRESSED_METRICS = 0x00000100
	PCF_GLYPH_PAD_MASK     = 3 << 0
	PCF_BYTE_MASK          = 1 << 2
	PCF_BIT_MASK           = 1 << 3
	PCF_SCAN_UNIT_MASK     = 3 << 4
)

type PCFTable struct {
	Type   uint32
	Format uint32
	Size   uint32
	Offset uint32
}

type PCFMetrics struct {
	Left   int16
	Right  int16
	Width  int16
	Ascent int16
	Desent int16
}

type PCF struct {
	Tables  map[uint32]PCFTable
	Metrics []PCFMetrics
	reader  io.ReadSeeker
}

func ParsePCF(r io.ReadSeeker) (*PCF, error) {
	var magic [4]byte
	if _, err := r.Read(magic[:]); err != nil {
		return nil, err
	}
	if magic[0] != 0x01 || magic[1] != 'p' || magic[2] != 'c' || magic[3] != 'f' {
		return nil, fmt.Errorf("not a PCF file")
	}

	var tableCount uint32
	if err := binary.Read(r, binary.LittleEndian, &tableCount); err != nil {
		return nil, err
	}

	pcf := &PCF{
		Tables: make(map[uint32]PCFTable),
		reader: r,
	}

	for i := uint32(0); i < tableCount; i++ {
		var table PCFTable
		if err := binary.Read(r, binary.LittleEndian, &table.Type); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &table.Format); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &table.Size); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &table.Offset); err != nil {
			return nil, err
		}
		pcf.Tables[table.Type] = table
	}

	if err := pcf.parseMetrics(); err != nil {
		return nil, err
	}

	return pcf, nil
}

func (p *PCF) parseMetrics() error {
	table, ok := p.Tables[PCF_METRICS]
	if !ok {
		return fmt.Errorf("no metrics table")
	}

	if _, err := p.reader.Seek(int64(table.Offset), io.SeekStart); err != nil {
		return err
	}

	var format uint32
	if err := binary.Read(p.reader, binary.LittleEndian, &format); err != nil {
		return err
	}

	var order binary.ByteOrder = binary.LittleEndian
	if (format & PCF_BYTE_MASK) != 0 {
		order = binary.BigEndian
	}

	var count uint32
	if (format & PCF_COMPRESSED_METRICS) != 0 {
		var c16 uint16
		if err := binary.Read(p.reader, order, &c16); err != nil {
			return err
		}
		count = uint32(c16)
		p.Metrics = make([]PCFMetrics, count)
		for i := uint32(0); i < count; i++ {
			var b [5]byte
			if _, err := io.ReadFull(p.reader, b[:]); err != nil {
				return err
			}
			p.Metrics[i] = PCFMetrics{
				Left:   int16(b[0]) - 0x80,
				Right:  int16(b[1]) - 0x80,
				Width:  int16(b[2]) - 0x80,
				Ascent: int16(b[3]) - 0x80,
				Desent: int16(b[4]) - 0x80,
			}
		}
	} else {
		if err := binary.Read(p.reader, order, &count); err != nil {
			return err
		}
		p.Metrics = make([]PCFMetrics, count)
		for i := uint32(0); i < count; i++ {
			var m struct {
				Left, Right, Width, Ascent, Descent, Attr int16
			}
			if err := binary.Read(p.reader, order, &m); err != nil {
				return err
			}
			p.Metrics[i] = PCFMetrics{
				Left:   m.Left,
				Right:  m.Right,
				Width:  m.Width,
				Ascent: m.Ascent,
				Desent: m.Descent,
			}
		}
	}
	return nil
}

// GetBitmaps extracts bitmaps from the PCF file.
func (p *PCF) GetBitmaps() ([]api.Bitmap, error) {
	table, ok := p.Tables[PCF_BITMAPS]
	if !ok {
		return nil, fmt.Errorf("no bitmaps table found")
	}

	if len(p.Metrics) == 0 {
		return nil, fmt.Errorf("metrics required for bitmap extraction")
	}

	if _, err := p.reader.Seek(int64(table.Offset), io.SeekStart); err != nil {
		return nil, err
	}

	var format uint32
	if err := binary.Read(p.reader, binary.LittleEndian, &format); err != nil {
		return nil, err
	}

	var order binary.ByteOrder = binary.LittleEndian
	if (format & PCF_BYTE_MASK) != 0 {
		order = binary.BigEndian
	}

	var glyphCount uint32
	if err := binary.Read(p.reader, order, &glyphCount); err != nil {
		return nil, err
	}

	if uint64(glyphCount) > uint64(len(p.Metrics)) {
		return nil, fmt.Errorf("bitmap glyph count %d exceeds metrics count %d", glyphCount, len(p.Metrics))
	}

	offsets := make([]uint32, glyphCount)
	for i := uint32(0); i < glyphCount; i++ {
		if err := binary.Read(p.reader, order, &offsets[i]); err != nil {
			return nil, err
		}
	}

	var bitmapSizes [4]uint32
	for i := 0; i < 4; i++ {
		if err := binary.Read(p.reader, order, &bitmapSizes[i]); err != nil {
			return nil, err
		}
	}

	glyphPadIndex := int(format & PCF_GLYPH_PAD_MASK)
	glyphPad := 1 << glyphPadIndex
	bitmapSize := bitmapSizes[glyphPadIndex]
	if bitmapSize > table.Size {
		return nil, fmt.Errorf("bitmap data size %d exceeds table size %d", bitmapSize, table.Size)
	}

	bitmapData := make([]byte, bitmapSize)
	if _, err := io.ReadFull(p.reader, bitmapData); err != nil {
		return nil, err
	}

	bitmaps := make([]api.Bitmap, glyphCount)
	for i := uint32(0); i < glyphCount; i++ {
		m := p.Metrics[i]
		w := int(m.Right - m.Left)
		h := int(m.Ascent + m.Desent)
		if w <= 0 {
			w = 1
		}
		if h <= 0 {
			h = 1
		}
		bm := core.NewBitmap(w, h)
		bm.SetPixelMode(api.MODE_MONO)
		if err := unpackPCFBitmap(bm.GetBuffer(), bm.GetPitch(), w, h, bitmapData, offsets[i], glyphPad, format); err != nil {
			return nil, fmt.Errorf("glyph %d bitmap: %w", i, err)
		}
		bitmaps[i] = bm
	}

	return bitmaps, nil
}

func unpackPCFBitmap(dst []byte, dstPitch, width, rows int, src []byte, offset uint32, glyphPad int, format uint32) error {
	if width <= 0 || rows <= 0 {
		return nil
	}
	rowBytes := (width + 7) >> 3
	paddedRowBytes := alignPCFRow(rowBytes, glyphPad)
	need := uint64(offset) + uint64(paddedRowBytes*rows)
	if need > uint64(len(src)) {
		return fmt.Errorf("offset %d needs %d bytes, have %d", offset, paddedRowBytes*rows, len(src))
	}

	msbFirst := (format & PCF_BIT_MASK) != 0
	start := int(offset)
	for y := 0; y < rows; y++ {
		row := src[start+y*paddedRowBytes : start+y*paddedRowBytes+rowBytes]
		for x := 0; x < width; x++ {
			byteIdx := x >> 3
			bitIdx := uint(x & 7)
			if msbFirst {
				bitIdx = 7 - bitIdx
			}
			if row[byteIdx]&(1<<bitIdx) != 0 {
				dst[y*dstPitch+x] = 255
			}
		}
	}
	return nil
}

func alignPCFRow(n, pad int) int {
	if pad <= 1 {
		return n
	}
	return (n + pad - 1) &^ (pad - 1)
}
