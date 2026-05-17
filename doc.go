// Package freetype documents the freetype-go module.
//
// The implementation currently lives in subpackages such as core, sfnt, raster,
// layout, cff, truetype, and bitmap. This root package is intentionally small:
// it exists so tools can discover the module with "go list ." and pkg.go.dev
// without implying a stable top-level facade API.
//
// Public APIs in this repository should be treated as experimental until the
// project documents compatibility guarantees.
package freetype
