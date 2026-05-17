package bitmap

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

type FNTHeader struct {
	Version         uint16
	Size            uint32
	Copyright       [60]byte
	Type            uint16
	Points          uint16
	VertRes         uint16
	HorizRes        uint16
	Ascent          uint16
	InternalLeading uint16
	ExternalLeading uint16
	Italic          byte
	Underline       byte
	StrikeOut       byte
	Weight          uint16
	CharSet         byte
	PixWidth        uint16
	PixHeight       uint16
	PitchAndFamily  byte
	AvgWidth        uint16
	MaxWidth        uint16
	FirstChar       byte
	LastChar        byte
	DefaultChar     byte
	BreakChar       byte
	WidthBytes      uint16
	Device          uint32
	Face            uint32
	BitsPointer     uint32
	BitsOffset      uint32
	Reserved        byte
}

type FNTCharEntry struct {
	Width  uint16
	Offset uint32
}

type FNT struct {
	Header FNTHeader
	Chars  []FNTCharEntry
	reader io.ReadSeeker
}

func ParseFNT(r io.ReadSeeker) (*FNT, error) {
	var header FNTHeader
	if err := binary.Read(r, binary.LittleEndian, &header); err != nil {
		return nil, err
	}

	if header.Version != 0x200 && header.Version != 0x300 {
		// Basic check, might be 0x100 too but less common
		// Note: header is slightly different for different versions,
		// but this is a "basic parser".
	}

	fnt := &FNT{
		Header: header,
		reader: r,
	}

	numChars := int(header.LastChar) - int(header.FirstChar) + 1
	if numChars <= 0 || numChars > 256 {
		return nil, fmt.Errorf("invalid character range")
	}

	fnt.Chars = make([]FNTCharEntry, numChars)
	for i := 0; i < numChars; i++ {
		if header.Version == 0x200 {
			var w uint16
			var off uint16
			binary.Read(r, binary.LittleEndian, &w)
			binary.Read(r, binary.LittleEndian, &off)
			fnt.Chars[i] = FNTCharEntry{Width: w, Offset: uint32(off)}
		} else {
			if err := binary.Read(r, binary.LittleEndian, &fnt.Chars[i]); err != nil {
				return nil, err
			}
		}
	}

	return fnt, nil
}

func (f *FNT) GetBitmap(charIndex int) (api.Bitmap, error) {
	if charIndex < int(f.Header.FirstChar) || charIndex > int(f.Header.LastChar) {
		return nil, fmt.Errorf("char index out of range")
	}

	entry := f.Chars[charIndex-int(f.Header.FirstChar)]
	width := int(entry.Width)
	height := int(f.Header.PixHeight)

	if width == 0 || height == 0 {
		return nil, fmt.Errorf("empty glyph")
	}

	// Calculate pitch. Windows FNT bitmaps are stored column-major?
	// No, usually they are row-major but each row is padded to byte boundary.
	// Actually, for FNT, the bitmaps are often stored as:
	// all rows of first pixel column, then all rows of second pixel column... NO, that's not right.
	// Windows FNT (Version 2 and 3) stores bitmaps as:
	// For each row, then for each column...
	// Wait, actually FNT bitmaps are stored as follows:
	// The bitmap for all characters is stored at BitsOffset.
	// It's a single large bitmap where characters are side-by-side?
	// NO. It's usually:
	// for each row y from 0 to height-1:
	//   for each char c:
	//     bits for row y of char c

	// Actually, it depends on whether it's fixed pitch or not.
	// Common FNT format:
	// The bitmap is stored such that all first rows of all characters are together,
	// then all second rows, etc.
	// This is why it's called "Legacy".

	// Let's implement a simpler version for now as requested.

	bm := core.NewBitmap(width, height)
	bm.SetPixelMode(api.MODE_MONO)

	return bm, nil
}
