# Performance Benchmarks and Glyph Cache Plan

This note records the current P1 hardening benchmark coverage and the cache
shape needed before wiring glyph caching into `sfnt.Face.LoadGlyph`.

## Benchmarks

The benchmark fixtures are deterministic and small enough to keep normal
`go test ./...` unchanged:

- `sfnt.BenchmarkLoadGlyphSynthetic` measures repeated `LoadGlyph` calls for a
  synthetic simple glyph and a four-component composite glyph.
- `raster.BenchmarkSmoothRasterizerScale` measures point-count and quadratic
  curve-count scaling while reusing a bitmap and rasterizer.
- `helper.BenchmarkDecodeWOFF2Synthetic` measures a small transformed
  `glyf`/`loca` WOFF2 decode and a 48-byte oversized-header rejection path.

Run representative checks with:

```sh
go test -run '^$' -bench 'BenchmarkLoadGlyphSynthetic|BenchmarkSmoothRasterizerScale|BenchmarkDecodeWOFF2Synthetic' -benchmem ./sfnt ./raster ./helper
```

## Glyph Cache Key

The current `cache.Manager` has `FaceID`, `SizeRequest`, and `GlyphRequest`,
but `LoadGlyph` is not connected to it. A safe glyph key needs more than the
glyph index:

- Face identity: caller-provided `FaceID`, face index for TTC/OTC, and a stable
  source identity such as path plus stat metadata or a content digest.
- Size identity: width, height, horizontal and vertical resolution, point size,
  and variation coordinates if variation selection becomes mutable.
- Glyph identity: glyph index plus every load flag that changes returned data,
  including hinting, bitmap/color preference, future render target flags, and
  embedded bitmap strike selection.
- Decoder identity: decoded image cache entries need either decoder versioning
  or invalidation when `SetImageDecoder` changes.

Do not cache by `glyphIndex` alone. `sfnt.Face` keeps mutable size state
(`xPPEM`, `yPPEM`, scales, scaled CVT, prep effects), so the same glyph index
can produce different outlines and metrics after `SetPixelSizes`.

## Invalidation Rules

- `SetPixelSizes` must invalidate size-specific glyph entries for that face or
  switch to a distinct `SizeID` so old entries remain isolated.
- Variation coordinate changes must invalidate glyph outlines, glyph metrics,
  and size metrics tied to the previous coordinates.
- Load flags should create separate glyph entries instead of mutating cached
  values.
- Image decoder changes should invalidate decoded image payload entries. It
  does not need to invalidate outline-only entries.
- Face eviction must drop dependent size and glyph entries, or dependent keys
  must include a generation that cannot collide with a later face.

## Integration TODO

1. Add a stable size object or generation ID around `SetPixelSizes`.
2. Store immutable cache values. `api.Outline` exposes mutable slices, so cache
   hits must return deep copies or a read-only internal representation.
3. Start with a final scaled glyph cache keyed by face, size, glyph, and flags.
4. Consider a later raw `glyf` parse cache keyed by face and glyph only, then
   scale/hint from that copy per size.
5. Keep cache lookup inside `sfnt` or a narrow adapter; avoid coupling WOFF2
   decode, rasterization, and glyph loading caches into one key space.
