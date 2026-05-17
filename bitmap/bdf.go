package bitmap

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

// BDFGlyph represents a single glyph in a BDF font.
type BDFGlyph struct {
	Name     string
	Encoding int
	BBX      [4]int // width, height, xoff, yoff
	Bitmap   api.Bitmap
}

// BDF represents a parsed BDF font.
type BDF struct {
	Name       string
	Properties map[string]string
	Glyphs     map[int]*BDFGlyph
}

// ParseBDF parses a BDF font from the provided reader.
func ParseBDF(r io.Reader) (*BDF, error) {
	scanner := bufio.NewScanner(r)
	bdf := &BDF{
		Properties: make(map[string]string),
		Glyphs:     make(map[int]*BDFGlyph),
	}

	var currentGlyph *BDFGlyph
	inBitmap := false
	y := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		if inBitmap {
			if fields[0] == "ENDCHAR" {
				inBitmap = false
				if currentGlyph != nil {
					bdf.Glyphs[currentGlyph.Encoding] = currentGlyph
				}
				continue
			}
			// Hex data for bitmap
			data, err := hex.DecodeString(line)
			if err != nil {
				return nil, fmt.Errorf("invalid bitmap data: %v", err)
			}

			if currentGlyph != nil && currentGlyph.Bitmap != nil {
				buffer := currentGlyph.Bitmap.GetBuffer()
				pitch := currentGlyph.Bitmap.GetPitch()
				if y < currentGlyph.Bitmap.GetRows() {
					// BDF is 1-bit per pixel. We'll store it as MODE_MONO (1 byte per pixel for simplicity in ftgo, or actually mono?)
					// Core Bitmap uses 1 byte per pixel even for MONO?
					// Let's check core/bitmap.go again.
					// core/bitmap.go: NewBitmap uses make([]byte, pitch*rows) where pitch = width.
					// So it seems it uses 1 byte per pixel.

					width := currentGlyph.BBX[0]
					for i := 0; i < width; i++ {
						byteIdx := i / 8
						bitIdx := 7 - (i % 8)
						if byteIdx < len(data) {
							if (data[byteIdx] & (1 << bitIdx)) != 0 {
								buffer[y*pitch+i] = 255
							} else {
								buffer[y*pitch+i] = 0
							}
						}
					}
					y++
				}
			}
			continue
		}

		switch fields[0] {
		case "STARTFONT":
			// skip
		case "FONT":
			if len(fields) > 1 {
				bdf.Name = strings.Join(fields[1:], " ")
			}
		case "STARTPROPERTIES":
			// Properties parsing could be more robust
		case "ENDPROPERTIES":
			// skip
		case "CHARS":
			// skip
		case "STARTCHAR":
			currentGlyph = &BDFGlyph{
				Name: strings.Join(fields[1:], " "),
			}
		case "ENCODING":
			if len(fields) > 1 {
				enc, _ := strconv.Atoi(fields[1])
				currentGlyph.Encoding = enc
			}
		case "BBX":
			if len(fields) >= 5 {
				for i := 0; i < 4; i++ {
					currentGlyph.BBX[i], _ = strconv.Atoi(fields[i+1])
				}
				// Allocate bitmap
				w, h := currentGlyph.BBX[0], currentGlyph.BBX[1]
				if w > 0 && h > 0 {
					bm := core.NewBitmap(w, h)
					bm.SetPixelMode(api.MODE_MONO)
					currentGlyph.Bitmap = bm
				}
			}
		case "BITMAP":
			inBitmap = true
			y = 0
		case "ENDCHAR":
			if currentGlyph != nil {
				bdf.Glyphs[currentGlyph.Encoding] = currentGlyph
			}
			currentGlyph = nil
		case "ENDFONT":
			return bdf, nil
		default:
			if len(fields) >= 2 {
				// Potentially a property
				bdf.Properties[fields[0]] = strings.Join(fields[1:], " ")
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return bdf, nil
}
