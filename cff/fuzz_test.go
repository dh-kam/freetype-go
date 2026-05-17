package cff

import (
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func FuzzParseCFF(f *testing.F) {
	// Construct a minimal CFF blob from TestParseCFF
	header := []byte{1, 0, 4, 4}
	nameIndex := []byte{
		0x00, 0x01, // Count
		0x01,       // OffSize
		0x01, 0x09, // Offsets: 1, 9
		'T', 'e', 's', 't', 'F', 'o', 'n', 't',
	}
	topDictIndex := []byte{
		0x00, 0x01,
		0x01,
		0x01, 0x02,
		0x00,
	}
	stringIndex := []byte{0x00, 0x00}
	globalSubrIndex := []byte{0x00, 0x00}

	cffBlob := append(header, nameIndex...)
	cffBlob = append(cffBlob, topDictIndex...)
	cffBlob = append(cffBlob, stringIndex...)
	cffBlob = append(cffBlob, globalSubrIndex...)

	f.Add(cffBlob)
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		stream := core.NewMemoryStream(data)
		face, err := ParseCFF(stream, 0)
		if err != nil {
			return
		}

		if face.CharStringsIndex.Count > 0 {
			face.LoadGlyphOutline(0)
			face.LoadGlyphOutline(int(face.CharStringsIndex.Count) - 1)
		}
	})
}
