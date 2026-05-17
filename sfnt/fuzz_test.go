package sfnt

import (
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func FuzzParseSFNT(f *testing.F) {
	// Seed with dummy SFNT data from TestLoadGlyphSimple
	data := make([]byte, 1000)
	binary.BigEndian.PutUint32(data[0:4], 0x00010000)
	binary.BigEndian.PutUint16(data[4:6], 4)
	copy(data[12:16], "glyf")
	binary.BigEndian.PutUint32(data[20:24], 200)
	binary.BigEndian.PutUint32(data[24:28], 100)
	copy(data[28:32], "head")
	binary.BigEndian.PutUint32(data[36:40], 300)
	binary.BigEndian.PutUint32(data[40:44], 54)
	copy(data[44:48], "loca")
	binary.BigEndian.PutUint32(data[52:56], 400)
	binary.BigEndian.PutUint32(data[56:60], 20)
	copy(data[60:64], "maxp")
	binary.BigEndian.PutUint32(data[68:72], 500)
	binary.BigEndian.PutUint32(data[72:76], 6)
	binary.BigEndian.PutUint16(data[300+50:300+52], 0)
	binary.BigEndian.PutUint16(data[500+4:500+6], 2)
	binary.BigEndian.PutUint16(data[400:402], 0)
	binary.BigEndian.PutUint16(data[402:404], 5)
	binary.BigEndian.PutUint16(data[404:406], 25)

	f.Add(data)
	f.Add([]byte{}) // Empty seed

	f.Fuzz(func(t *testing.T, data []byte) {
		stream := core.NewMemoryStream(data)
		loader := NewLoader(core.NewSystem())
		face, err := loader.LoadFace(stream)
		if err != nil {
			return
		}

		numGlyphs := face.GetNumGlyphs()
		if numGlyphs > 0 {
			face.LoadGlyph(0, 0)
			if numGlyphs > 1 {
				face.LoadGlyph(numGlyphs-1, 0)
			}
		}
		face.GetGlyphIndex('A')
		face.GetGlyphIndex(' ')
		face.GetUnitsPerEm()
	})
}
