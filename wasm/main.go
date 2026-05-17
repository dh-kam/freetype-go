//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/helper"
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
	faceIndex := 0
	if len(args) >= 4 {
		faceIndex = args[3].Int()
	}

	// 2. Load Font
	var stream api.Stream = core.NewMemoryStream(fontData)
	stream, err := helper.DecodeWOFFIfNeeded(stream)
	if err != nil {
		return fmt.Sprintf("Error: decode WOFF container failed: %v", err)
	}
	sys := core.NewSystem()
	face, err := sfnt.LoadFaceIndex(sys, stream, faceIndex)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if err := face.SetPixelSizes(size, size); err != nil {
		return fmt.Sprintf("Error: invalid pixel size: %v", err)
	}

	// 3. Get Glyph Index
	glyphIndex, err := face.GetGlyphIndex(rune(charCode))
	if err != nil {
		return fmt.Sprintf("Error: glyph not found for %c", charCode)
	}

	// 4. Load and Hint
	slot, err := face.LoadGlyph(glyphIndex, api.LoadDefault)
	if err != nil {
		return fmt.Sprintf("Error: load glyph failed: %v", err)
	}

	outline := slot.GetOutline()

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
