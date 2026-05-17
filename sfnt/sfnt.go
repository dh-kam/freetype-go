package sfnt

import (
	"encoding/binary"
	"io"

	"github.com/dh-kam/freetype-go/api"
)

// Table represents an SFNT table record.
type Table struct {
	Tag      uint32
	Checksum uint32
	Offset   uint32
	Length   uint32
}

// OffsetTable represents the SFNT offset subtable.
type OffsetTable struct {
	ScalerType    uint32
	NumTables     uint16
	SearchRange   uint16
	EntrySelector uint16
	RangeShift    uint16
}

// tableStream implements api.Stream for a sub-range of another stream.
type tableStream struct {
	base   api.Stream
	offset int64
	length int64
}

func (s *tableStream) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off >= s.length {
		return 0, io.EOF
	}
	remaining := s.length - off
	if int64(len(p)) > remaining {
		p = p[:remaining]
		n, err = s.base.ReadAt(p, s.offset+off)
		if err == nil {
			err = io.EOF
		}
		return n, err
	}
	return s.base.ReadAt(p, s.offset+off)
}

func (s *tableStream) Size() int64 {
	return s.length
}

func readExactAt(s api.Stream, p []byte, off int64) error {
	if len(p) == 0 {
		return nil
	}
	n, err := s.ReadAt(p, off)
	if err != nil {
		return err
	}
	if n != len(p) {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func readStreamData(s api.Stream) ([]byte, error) {
	data := make([]byte, s.Size())
	if err := readExactAt(s, data, 0); err != nil {
		return nil, err
	}
	return data, nil
}

func tagToString(tag uint32) string {
	return string([]byte{
		byte(tag >> 24),
		byte(tag >> 16),
		byte(tag >> 8),
		byte(tag),
	})
}

func stringToTag(s string) uint32 {
	if len(s) != 4 {
		return 0
	}
	return uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3])
}

func readUint16(s api.Stream, off int64) (uint16, error) {
	var buf [2]byte
	if err := readExactAt(s, buf[:], off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

func readUint32(s api.Stream, off int64) (uint32, error) {
	var buf [4]byte
	if err := readExactAt(s, buf[:], off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}

func readInt16(s api.Stream, off int64) (int16, error) {
	v, err := readUint16(s, off)
	return int16(v), err
}

func readInt32(s api.Stream, off int64) (int32, error) {
	v, err := readUint32(s, off)
	return int32(v), err
}

func readByte(s api.Stream, off int64) (byte, error) {
	var buf [1]byte
	if err := readExactAt(s, buf[:], off); err != nil {
		return 0, err
	}
	return buf[0], nil
}

func readUint64(s api.Stream, off int64) (uint64, error) {
	var buf [8]byte
	if err := readExactAt(s, buf[:], off); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(buf[:]), nil
}
