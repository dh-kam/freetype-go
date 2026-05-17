//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"

	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/raster"
	"github.com/dh-kam/freetype-go/sfnt"
)

func main() {
	fmt.Println("FreeType-Go WASM Loaded")

	// Expose a JS function: renderGlyph(fontData, charCode, size)
	js.Global().Set("renderGlyph", js.FuncOf(renderGlyph))

	// Keep the Go program running
	select {}
}

func renderGlyph(this js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return "Error: missing arguments"
	}

	// 1. Get font data from JS Uint8Array
	jsData := args[0]
	fontData := make([]byte, jsData.Length())
	js.CopyBytesToGo(fontData, jsData)

	charCode := args[1].Int()
	size := args[2].Int()

	// 2. Load Font
	stream := core.NewMemoryStream(fontData)
	sys := core.NewSystem()
	loader := sfnt.NewLoader(sys)
	face, err := loader.LoadFace(stream)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	// 3. Get Glyph Index
	glyphIndex, err := face.GetGlyphIndex(rune(charCode))
	if err != nil {
		return fmt.Sprintf("Error: glyph not found for %c", charCode)
	}

	// 4. Load and Hint
	slot, err := face.LoadGlyph(glyphIndex, 0)
	if err != nil {
		return fmt.Sprintf("Error: load glyph failed: %v", err)
	}

	outline := slot.GetOutline()

	// Scaling
	scale := int32((int64(size) << 16) / int64(face.GetUnitsPerEm()))
	outline.Scale(scale, scale)
	outline.Translate(0, int32(size)<<6)

	// 5. Render
	bitmap := core.NewBitmap(size+10, size+10)
	rast := raster.NewSmoothRasterizer()
	rast.Render(outline, bitmap)

	// 6. Return as JS Uint8Array
	buf := bitmap.GetBuffer()
	dst := js.Global().Get("Uint8Array").New(len(buf))
	js.CopyBytesToJS(dst, buf)

	return map[string]interface{}{
		"width":  bitmap.GetWidth(),
		"height": bitmap.GetRows(),
		"pixels": dst,
	}
}
