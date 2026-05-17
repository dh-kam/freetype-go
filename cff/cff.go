package cff

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

// Index represents a CFF INDEX structure.
type Index struct {
	Count   uint16
	OffSize uint8
	Offsets []uint32
	Data    []byte
}

// Get returns the i-th object in the INDEX.
func (idx *Index) Get(i int) ([]byte, error) {
	if i < 0 || i >= int(idx.Count) {
		return nil, errors.New("index out of range")
	}
	start := idx.Offsets[i] - 1
	end := idx.Offsets[i+1] - 1
	if start > end || end > uint32(len(idx.Data)) {
		return nil, errors.New("invalid offset in index")
	}
	return idx.Data[start:end], nil
}

// CFF represents the Compact Font Format table.
type CFF struct {
	Major   uint8
	Minor   uint8
	HdrSize uint8
	OffSize uint8 // Absolute offset size

	NameIndex        Index
	TopDictIndex     Index
	StringIndex      Index
	GlobalSubrIndex  Index
	CharStringsIndex Index
	LocalSubrIndex   Index

	TopDict map[int][]float64
	Stream  api.Stream
}

// ParseCFF parses the CFF table from the given stream and offset.
func ParseCFF(stream api.Stream, offset int64) (*CFF, error) {
	cff := &CFF{Stream: stream}

	// Read Header
	major, err := readByte(stream, offset)
	if err != nil {
		return nil, err
	}
	minor, err := readByte(stream, offset+1)
	if err != nil {
		return nil, err
	}
	hdrSize, err := readByte(stream, offset+2)
	if err != nil {
		return nil, err
	}
	offSize, err := readByte(stream, offset+3)
	if err != nil {
		return nil, err
	}

	cff.Major = major
	cff.Minor = minor
	cff.HdrSize = hdrSize
	cff.OffSize = offSize

	curr := offset + int64(hdrSize)

	// Name INDEX
	nameIndex, next, err := parseIndex(stream, curr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Name INDEX: %v", err)
	}
	cff.NameIndex = *nameIndex
	curr = next

	// Top DICT INDEX
	topDictIndex, next, err := parseIndex(stream, curr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Top DICT INDEX: %v", err)
	}
	cff.TopDictIndex = *topDictIndex
	curr = next

	// String INDEX
	stringIndex, next, err := parseIndex(stream, curr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse String INDEX: %v", err)
	}
	cff.StringIndex = *stringIndex
	curr = next

	// Global Subr INDEX
	globalSubrIndex, _, err := parseIndex(stream, curr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Global Subr INDEX: %v", err)
	}
	cff.GlobalSubrIndex = *globalSubrIndex

	// Parse the first Top DICT
	if cff.TopDictIndex.Count > 0 {
		data, err := cff.TopDictIndex.Get(0)
		if err != nil {
			return nil, fmt.Errorf("failed to get first Top DICT: %v", err)
		}
		topDict, err := ParseDict(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Top DICT: %v", err)
		}
		cff.TopDict = topDict

		// Parse CharStrings INDEX
		if csOff, ok := topDict[17]; ok && len(csOff) > 0 {
			csIdx, _, err := parseIndex(stream, offset+int64(csOff[0]))
			if err != nil {
				return nil, fmt.Errorf("failed to parse CharStrings INDEX: %v", err)
			}
			cff.CharStringsIndex = *csIdx
		}

		// Parse Private DICT and Local Subrs (Simplified)
		if priv, ok := topDict[18]; ok && len(priv) >= 2 {
			privSize := int64(priv[0])
			privOff := int64(priv[1])
			privData := make([]byte, privSize)
			_, err = stream.ReadAt(privData, offset+privOff)
			if err == nil {
				privDict, err := ParseDict(privData)
				if err == nil {
					if subrsOff, ok := privDict[19]; ok && len(subrsOff) > 0 {
						lsIdx, _, err := parseIndex(stream, offset+privOff+int64(subrsOff[0]))
						if err == nil {
							cff.LocalSubrIndex = *lsIdx
						}
					}
				}
			}
		}
	}

	return cff, nil
}

func (c *CFF) LoadGlyphOutline(glyphIndex int) (*core.Outline, error) {
	data, err := c.CharStringsIndex.Get(glyphIndex)
	if err != nil {
		return nil, err
	}
	return DecodeCharString(data, &c.GlobalSubrIndex, &c.LocalSubrIndex, nil)
}

func parseIndex(stream api.Stream, offset int64) (*Index, int64, error) {
	count, err := readUint16(stream, offset)
	if err != nil {
		return nil, 0, err
	}

	if count == 0 {
		return &Index{Count: 0}, offset + 2, nil
	}

	offSize, err := readByte(stream, offset+2)
	if err != nil {
		return nil, 0, err
	}

	if offSize < 1 || offSize > 4 {
		return nil, 0, fmt.Errorf("invalid offSize %d in INDEX", offSize)
	}

	offsets := make([]uint32, count+1)
	for i := 0; i <= int(count); i++ {
		off, err := readOffset(stream, offset+3+int64(i)*int64(offSize), offSize)
		if err != nil {
			return nil, 0, err
		}
		if off == 0 {
			return nil, 0, fmt.Errorf("invalid zero offset %d in INDEX", i)
		}
		if i > 0 && off < offsets[i-1] {
			return nil, 0, fmt.Errorf("non-monotonic offset %d in INDEX", i)
		}
		offsets[i] = off
	}

	dataSize := offsets[count] - 1
	dataOffset := offset + 3 + int64(count+1)*int64(offSize)
	if dataOffset > stream.Size() || uint64(dataSize) > uint64(stream.Size()-dataOffset) {
		return nil, 0, io.ErrUnexpectedEOF
	}
	data := make([]byte, dataSize)
	n, err := stream.ReadAt(data, dataOffset)
	if err != nil && err != io.EOF {
		return nil, 0, err
	}
	if n != len(data) {
		return nil, 0, io.ErrUnexpectedEOF
	}

	return &Index{
		Count:   count,
		OffSize: offSize,
		Offsets: offsets,
		Data:    data,
	}, dataOffset + int64(dataSize), nil
}

// --- Helpers ---

func readByte(s api.Stream, off int64) (byte, error) {
	var buf [1]byte
	_, err := s.ReadAt(buf[:], off)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

func readUint16(s api.Stream, off int64) (uint16, error) {
	var buf [2]byte
	_, err := s.ReadAt(buf[:], off)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

func readUint32(s api.Stream, off int64) (uint32, error) {
	var buf [4]byte
	_, err := s.ReadAt(buf[:], off)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}

func readOffset(s api.Stream, off int64, offSize uint8) (uint32, error) {
	switch offSize {
	case 1:
		v, err := readByte(s, off)
		return uint32(v), err
	case 2:
		v, err := readUint16(s, off)
		return uint32(v), err
	case 3:
		var buf [3]byte
		_, err := s.ReadAt(buf[:], off)
		if err != nil {
			return 0, err
		}
		return uint32(buf[0])<<16 | uint32(buf[1])<<8 | uint32(buf[2]), nil
	case 4:
		return readUint32(s, off)
	default:
		return 0, fmt.Errorf("invalid offSize %d", offSize)
	}
}
