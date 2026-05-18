# FreeType-Go Features

This page summarizes implemented areas in the current repository. It is not a
statement of full FreeType compatibility or stable API coverage.
Use [Porting Status and Remaining Gaps](porting-status.md) for the current
parity tracking criteria, narrow next tracking items, and known incomplete
areas.

## Implemented Areas

### 1. Font Formats
- **TrueType (.ttf)**: SFNT directory parsing, common table parsing, glyph
  loading, cmap/hmtx support, and TrueType instruction machinery with focused
  size/CVT/prep, MIRP CVT[-1], SDPVTL, negative SLOOP, DELTAP invalid-point,
  ALIGNRP preflight, and rollback regression coverage.
- **OpenType/CFF (.otf)**: CFF parsing and charstring-related support are
  present. Coverage should be treated as evolving.
- **Type 1 (.pfa/.pfb)**: PFA/PFB parsing, Type 1 dictionary and encoding
  handling, charstring outlines/metrics/Subrs/flex/SEAC coverage, and explicit
  AFM/PFM companion metric attachment through `SetAFM`, `SetPFM`, and
  `SetCompanionMetrics`, with explicit path loading and attachment through
  `ReadCompanionMetricsFiles` and `SetCompanionMetricsFiles`, plus same-stem
  adjacent companion discovery through `DiscoverCompanionMetricsFiles`,
  `ReadCompanionMetricsForFont`, and `SetCompanionMetricsForFont`. `LoadFace`
  can auto-attach discovered companions when the stream implements
  `FontPathStream`, including ordinary `core.FileStream` values when
  `Type1FontPath()` is present. Charstring guard coverage includes terminators,
  Subr returns, unterminated flex, limited OtherSubr 3 result-pop handling, and
  pending OtherSubr result queues, with return values scoped to supported
  pop-result cases.
  Coverage should be treated as evolving.
- **TrueType/OpenType collections (.ttc/.otc)**: The SFNT loader can open the
  first face by default or a requested face via `sfnt.LoadFaceIndex`.
- **WOFF/WOFF2 containers**: WOFF streams and WOFF2 streams can be decoded to
  SFNT or TTC; WOFF2 includes transformed `glyf`/`loca`, transformed `hmtx`,
  and collection directory reconstruction. Current focused regression coverage
  includes transformed-hmtx, instruction/bbox, short/long-loca multiglyph,
  per-glyph overlap bitmap, collection long-multiglyph, collection hmtx
  dependency cases, malformed shared-hmtx metadata, raw/transformed outline
  mismatch rejection, malformed long-loca/raw-glyph shared-hmtx rejection, and
  malformed shared-composite rejection. This is still not a parity claim.
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
