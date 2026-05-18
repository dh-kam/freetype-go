# FreeType-Go Features

This page summarizes implemented areas in the current repository. It is not a
statement of full FreeType compatibility or stable API coverage.
Use [Porting Status and Remaining Gaps](porting-status.md) for the current
parity tracking criteria and known incomplete areas.

## Implemented Areas

### 1. Font Formats
- **TrueType (.ttf)**: SFNT directory parsing, common table parsing, glyph
  loading, cmap/hmtx support, and TrueType instruction machinery.
- **OpenType/CFF (.otf)**: CFF parsing and charstring-related support are
  present. Coverage should be treated as evolving.
- **TrueType/OpenType collections (.ttc/.otc)**: The SFNT loader can open the
  first face by default or a requested face via `sfnt.LoadFaceIndex`.
- **WOFF/WOFF2 containers**: WOFF streams and WOFF2 streams can be decoded to
  SFNT or TTC; WOFF2 includes transformed `glyf`/`loca`, `hmtx`, and collection
  directory reconstruction.
- **Bitmap Fonts**: BDF, PCF, FNT, and selected embedded bitmap table support.
- **Color Fonts**: Parsers/helpers for selected `COLR`/`CPAL` and `sbix`
  data exist.

### 2. Rendering Engine
- **Fixed-point Math**: Go implementations for common 26.6 and 16.16
  operations, with package tests.
- **Smooth Rasterizer**: Anti-aliased outline rasterization into `api.Bitmap`
  destinations.
- **LCD Subpixel Rendering**: LCD pixel mode and filtering support are present
  in the rasterizer.
- **Outline Stroker**: Vector path stroking support.

### 3. Typography & Layout
- **TrueType VM**: Bytecode interpreter code exists for hinting-related work,
  but this project does not currently claim complete VM compatibility.
- **Variation Engine**: Support for selected OpenType variation tables such as
  `fvar` and `gvar`.
- **OpenType Layout**: GSUB/GPOS parsing and selected application helpers for
  ligatures, contextual substitution, kerning, and mark positioning.

### 4. System Utilities
- **LRU Cache**: Mutex-protected generic LRU cache plus face/glyph manager
  helpers.
- **Stream I/O**: Abstracted interface for file and memory-based reading.
- **Error Mapping**: FreeType-style numeric error values are represented in Go.
