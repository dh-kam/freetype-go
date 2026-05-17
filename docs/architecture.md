# Architecture

FreeType-Go is organized as a set of focused Go packages rather than a stable
root-level facade. The public API surface is still experimental; consumers
should import the specific packages they need and expect compatibility details
to evolve while the port matures.

## Package Map

- `api`: Shared interfaces and error values used across packages.
- `core`: System helpers, streams, bitmaps, outlines, and library-level types.
- `sfnt`: SFNT/OpenType table parsing and face/glyph loading.
- `truetype`: TrueType bytecode interpreter support.
- `cff` and `type1`: PostScript outline parsing and related font support.
- `raster`: Smooth rasterization of outlines into bitmaps.
- `layout`: Early GSUB/GPOS layout parsing and application helpers.
- `bitmap`, `color`, `var`, `stroke`, `cache`, and `math`: Supporting font
  functionality split by domain.
- `cmd/ftgo`: Small command-line renderer/example.
- `wasm`: Browser demo assets.

## Data Flow

Typical use starts with an `api.Stream`, usually from `core.NewFileStream` or
`core.NewMemoryStream`. A format driver such as `sfnt.NewLoader` reads font
tables and returns an `api.Face`. Callers can map characters to glyph indexes,
load glyph outlines, and pass those outlines to `raster.NewSmoothRasterizer`
with a `core.Bitmap` destination.

The packages intentionally expose lower-level pieces today. That keeps the port
testable while avoiding a premature promise that the module has a complete,
stable FreeType-compatible facade.
