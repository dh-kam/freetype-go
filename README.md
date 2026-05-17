# FreeType-Go

A work-in-progress, pure Go port inspired by the **FreeType 2.13.2** font engine. The code is organized into lower-level packages for font parsing, glyph loading, rasterization, layout experiments, and related utilities.

[![Go Reference](https://pkg.go.dev/badge/github.com/dh-kam/freetype-go.svg)](https://pkg.go.dev/github.com/dh-kam/freetype-go)
[![License: FTL](https://img.shields.io/badge/License-FTL-blue.svg)](https://www.freetype.org/license.html)

## Status

- **CGO-free by design**: The repository is implemented in Go and can be built without linking to the C FreeType library.
- **Experimental API**: Import the subpackages you need. The root package currently provides documentation only so `go list .` and pkg.go.dev can discover the module.
- **Partial FreeType coverage**: The project includes implementations for SFNT/OpenType parsing, TrueType/CFF-related code, rasterization, selected bitmap/color/variation tables, and early layout support. It is not yet a drop-in replacement for FreeType.
- **Conformance is evolving**: Tests exist for many packages, and `tools/conformance` can compare Go dumps against optional C FreeType dumps. The project does not currently claim bit-for-bit rendering parity or full specification coverage.
- **FreeType-like API concepts**: `api` exposes load flag names, render modes, and optional glyph slot metrics plumbing. Drivers may implement only a subset while conformance tracks the remaining semantic gaps.

## Quick Start

### Installation
```bash
go get github.com/dh-kam/freetype-go
```

### Basic Usage
```go
package main

import (
	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/helper"
	"github.com/dh-kam/freetype-go/raster"
	"github.com/dh-kam/freetype-go/sfnt"
	"os"
)

func main() {
	f, _ := os.Open("font.ttf") // TTF, OTF, TTC/OTC, WOFF, or WOFF2
	defer f.Close()

	// 1. Load Font
	fileStream, _ := core.NewFileStream(f)
	var stream api.Stream = fileStream
	stream, _ = helper.DecodeWOFFIfNeeded(stream)
	face, _ := sfnt.LoadFaceIndex(core.NewSystem(), stream, 0)

	// 2. Set Size and Load Glyph
	face.SetPixelSizes(64, 64)
	glyphIndex, _ := face.GetGlyphIndex('A')
	slot, _ := face.LoadGlyph(glyphIndex, api.LoadDefault)

	// 3. Render to Bitmap
	outline := slot.GetOutline()
	bitmap := core.NewBitmap(64, 64)
	rasterizer := raster.NewSmoothRasterizer()
	rasterizer.Render(outline, bitmap)
}
```

## Documentation
- [Full Feature List](docs/features.md)
- [Architecture Guide](docs/architecture.md)
- [Conformance Workflow](docs/conformance.md)
- [CLI ASCII Renderer](cmd/ftgo)
- [Web Demo (WASM)](https://dh-kam.github.com/freetype-go/)

## Licensing
This project is released under the **FreeType License (FTL)** to maintain compatibility and respect for the original project. See [LICENSE](LICENSE) for details.

---
*Maintained by dh-kam and the Gemini CLI Panel.*
