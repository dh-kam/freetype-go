# Porting Status and Remaining Gaps

This document tracks the current FreeType-Go port at an area level. It is a
working status page, not a compatibility promise. Do not read any row here as
claiming full FreeType replacement status unless its parity criteria are also
met.

## How to Read the Status

The project does not assign a single global completion percentage. A percentage
would be misleading until the corpus and pixel/reference gates are stable.
Instead, each area is tracked with these states:

- **Implemented**: Code exists and has package-level tests or harness coverage.
- **Comparable**: The conformance harness can compare this area against a C
  FreeType reference dump.
- **Expected gap**: Differences are known and should be annotated in request
  files with `expected_gaps` until they are fixed.
- **Not yet a parity claim**: The implementation may be useful, but it is not
  validated against enough FreeType behavior or real fonts to call complete.

An area should move toward "complete" only when synthetic tests, real-font
corpus runs, and FreeType reference comparisons all agree for the documented
scope.

## Current Area Status

| Area | Current status | Why it is not complete yet |
| --- | --- | --- |
| SFNT/OpenType shell | Implemented for directory parsing, common tables, collections, glyph lookup, glyph loading, and selected image/color/variation tables. | Needs broader real-font corpus coverage and more table edge cases before this can be treated as FreeType-compatible coverage. |
| TrueType outlines | Implemented and comparable for outline geometry, metrics, charmap lookups, selected load flags, and current size/CVT/prep regression coverage. | Hinting-dependent output is not a full FreeType VM parity claim. Composite, transform, variation, loader-level size model behavior, and malformed-font edge cases still need corpus expansion. |
| TrueType hinting VM | Implemented enough for ongoing hinting work and package tests, including prep execution, CVT scaling/mutation, cvar-before-scaling, size-reset regression coverage, selected MIRP CVT[-1] and SDPVTL vector fallback coverage, negative SLOOP and DELTAP invalid-point guards, loop operand preflight coverage for SHP, ALIGNRP, FLIPPT, SHPIX, and IP, and glyph-program rollback checks. | FreeType instruction semantics are broad. Remaining VM opcode behavior, graphics-state edge cases, twilight/phantom point behavior, target modes, size/driver state interactions, and rollback/error handling need systematic parity tests. |
| Smooth/mono/LCD raster output | Render-to-bitmap paths and conformance bitmap metadata are present. | FreeType bitmap bit parity is not closed. Pixel mode, pitch, placement, coverage, LCD filtering, mono packing, and byte-for-byte buffer hashes can still differ. |
| Bitmap fonts and embedded bitmaps | BDF, PCF, FNT, and selected embedded bitmap table support exist. | Bitmap bit parity against FreeType is still an expected gap. Strike selection, placement, pixel modes, and packed buffer layout need reference comparisons across real bitmap fonts. |
| CFF/CFF2 | Parser and charstring-related support exist, including CFF2 variation-related work, CFF2 `maxstack` parsing, and operand stack bounds for CFF1/CFF2 charstrings. | Synthetic coverage is not enough. A licensed CFF/CFF2 corpus is needed to verify dicts, subrs, FDSelect/FDArray, blends, private dict behavior, stack limits, and outline parity against FreeType. |
| Type 1 | PFB/PFA input, eexec decoding, dictionary parsing, StandardEncoding/ISOLatin1Encoding/ExpertEncoding, private hint metadata parsing, AFM/PFM companion metric parsers plus explicit face attachment through `SetAFM`, `SetPFM`, `SetCompanionMetrics`, `SetCompanionMetricsFiles`, and `SetCompanionMetricsForFont`, explicit companion-file path loading through `ReadCompanionMetricsFiles`, same-stem adjacent companion discovery through `DiscoverCompanionMetricsFiles` and `ReadCompanionMetricsForFont`, optional loader auto-attachment for streams implementing `FontPathStream`, including file-backed core streams when `core.FileStream.Type1FontPath()` is present, AFM kerning in `Shape`, basic charstring outlines/metrics/Subrs/flex/stem hint records/native segment records, charstring operand stack bounds plus top-level terminator, Subr-return, unterminated-flex, limited OtherSubr 3 result-pop handling, and pending OtherSubr result validation, native cubic outline tags plus segment inspection/dump/compare records, SEAC outline composition, conservative hint snap scaffolding, an `api.Face` loader path, and Go conformance Type 1 fallback smoke coverage exist. | This is useful but not a parity claim. Companion metric discovery is path-based and only auto-attaches when the supplied stream exposes `Type1FontPath()`; plain `api.Stream` values that do not expose a path still require explicit companion attachment. Discovery only probes same-stem `.afm/.AFM` and `.pfm/.PFM` files. Type 1 hinting is not FreeType-equivalent stem/blue-zone fitting, and full OtherSubrs beyond the currently guarded flex/result-pop cases, Multiple Masters, plus real-corpus differential testing remain unsupported or unproven. |
| WOFF/WOFF2 | WOFF and WOFF2 streams can decode into SFNT/TTC, including transformed `glyf`/`loca`, transformed `hmtx`, and collection directory reconstruction. Focused package tests now cover instruction and explicit-bbox reconstruction, short/long `loca` multiglyph cases, per-glyph overlap bitmap handling, collection long-multiglyph sharing, shared collection `glyf`/`loca` plus `hmtx` dependency behavior, raw/transformed outline mismatch rejection, malformed shared-`hmtx` outline metadata, malformed long-`loca` and raw-glyph shared-`hmtx` rejection paths, and malformed shared-composite rejection. | Needs broader real corpus coverage for collection cases, transformed tables, remaining collection variants, error handling, and comparison of decoded faces against FreeType-loaded equivalents. Malformed long-`loca` and raw-glyph shared-`hmtx` coverage is narrow rejection coverage, not a WOFF2 parity claim. |
| Color fonts | COLR/CPAL and sbix parsing/helpers exist for selected data. | There is no full COLR pixel renderer parity claim. COLR v0/v1 composition, gradients, transforms, clips, palette/variation handling, blending, and glyph recursion need rendered pixel comparisons. |
| Variations | Selected variation tables such as `fvar` and `gvar` are supported. | Needs more multi-axis real-font corpus coverage and FreeType comparisons for outlines, metrics, CFF2 blends, and interaction with hinting/render modes. |
| Layout | GSUB/GPOS parsing and selected application helpers exist. | FreeType itself is not a full shaping engine, so completion must be scoped carefully. The current layout support should be tracked as helper coverage, not FreeType rendering parity. |
| API facade | FreeType-like load flags, render modes, errors, glyph slot metrics, bitmap, and image plumbing are present. | Drivers may expose only subsets. API compatibility should be judged by documented behavior and conformance data, not by matching names alone. |
| Conformance harness | Go dumps, optional C FreeType dumps, batch runs, smoke discovery, render metadata, bitmap hashes, Type 1 native segment comparison, and `expected_gaps` policy are present. | The harness is now capable of tracking gaps, but coverage is only as strong as the request corpus and fixture fonts used with it. |

## Recent Work Reflected Here

Recent work closed several implementation and regression-test gaps, but it does
not change the overall rule that parity requires corpus and FreeType reference
comparison:

- Type 1 companion metrics now cover AFM/PFM parsing, public `ReadAFM`,
  `ReadPFM`, `ReadCompanionMetrics` `io.Reader` helpers, explicit
  `ReadCompanionMetricsFiles` path helpers, same-stem adjacent discovery
  through `DiscoverCompanionMetricsFiles` and `ReadCompanionMetricsForFont`,
  lookup helpers, explicit
  `Face.SetAFM`, `Face.SetPFM`, `Face.SetCompanionMetrics`, and
  `Face.SetCompanionMetricsFiles`/`Face.SetCompanionMetricsForFont`
  attachment on loaded Type 1 faces, optional `LoadFace` auto-attachment when
  the stream implements `FontPathStream`, AFM width/left-side-bearing
  overrides, PFM fallback widths, and AFM KPX kerning during `Shape`. The
  file-stream companion change adds `core.FileStream.Type1FontPath()`, so
  ordinary `core.NewFileStream` Type 1 loads can use the same path-based
  auto-attachment. This only broadens same-stem file-backed discovery.
- Type 1 encoding support includes ExpertEncoding, and Type 1 charstring
  decoding now has bounded operand-stack handling for malformed inputs,
  including `callsubr` paths. Top-level charstrings must terminate with
  `endchar` or SEAC, and local Subrs must return instead of silently ending at
  EOF. Pending OtherSubr pop-result queues are rejected before any non-`pop`
  operand or operator, including `endchar`, SEAC, `return`, or another
  `callothersubr`. OtherSubr return-value handling is limited to the currently
  supported flex and OtherSubr 3 pop-value paths. This is guarded compatibility
  work, not full OtherSubrs support.
- Type 1 flex coverage includes direct `callothersubr` flex handling and the
  standard flex Subr pattern where `callsubr` must preserve flex-end operands;
  unterminated flex sequences are rejected before `endchar` or SEAC.
- Native Type 1 path segments are available through inspection helpers and the
  conformance JSON dump, including stable JSON-friendly segment records. The
  comparison tool can now compare `type1_segments` records for kind, units, and
  point coordinates when the reference dump includes them.
- WOFF2 transformed `glyf`/`loca` reconstruction has focused tests for simple
  transformed glyph streams, empty/simple multiglyph streams with short and
  long `loca`, per-glyph overlap bitmaps, instruction bytes, explicit bounding
  boxes, malformed substreams, zero-length transformed `loca`,
  collection/shared-outline table reconstruction, collection long-multiglyph
  sharing, shared transformed composite glyphs in WOFF2 collections, malformed
  shared-composite rejection, and transformed `hmtx`
  reconstruction/dependency validation for collection faces, including
  multiglyph side bearings derived from short and long outline tables,
  malformed shared-`hmtx` outline metadata, malformed long-`loca` and raw-glyph
  shared-`hmtx` dependencies, and mixed raw/transformed outline dependencies.
- TrueType size/CVT/prep regression coverage now exercises `SetPixelSizes`,
  CVT scaling, cvar application before scaling, prep reruns, WCVTF units-per-em
  scaling, reset of prep-related storage/twilight state, and public loaded-face
  resize paths where metrics/outlines are reloaded after size changes.
- TrueType VM regression coverage includes MIRP handling for CVT index `-1` as
  a zero-distance case. SDPVTL coverage now keeps the perpendicular projection
  when an original-point dual vector needs fallback, SLOOP rejects negative
  loop counts without mutating state, DELTAP backward-compatibility handling
  skips invalid negative point indexes, and looped mutation opcodes now have
  focused preflight coverage for SHP, ALIGNRP, FLIPPT, SHPIX, and IP before
  mutating points, tags, or resetting loop state. Glyph-program failure
  coverage checks that outline/CVT mutations are rolled back after an error.
- Go and optional C FreeType JSON dumps, batch requests, render metadata,
  bitmap hashes, and `expected_gaps` remain the main way to measure progress.

These improvements make the remaining work measurable. They do not mean the
port has reached full FreeType parity.

## Type 1 Status

Type 1 support has moved beyond a stub loader, but it should still be described
as early implementation coverage rather than FreeType-compatible Type 1
support.

Implemented or partially implemented:

- PFB and PFA container/input handling.
- eexec decryption/decoding for encrypted Type 1 program data.
- Top-level and private dictionary parsing for the data needed by the current
  loader.
- StandardEncoding, ISOLatin1Encoding, and ExpertEncoding handling plus custom
  encoding overrides.
- Standalone AFM and PFM companion metric parsers, `ReadAFM`/`ReadPFM`
  `io.Reader` helpers, `ReadCompanionMetricsFiles` for explicit AFM/PFM paths,
  `DiscoverCompanionMetricsFiles`/`ReadCompanionMetricsForFont` for same-stem
  adjacent discovery, and lookup helpers.
- Explicit Type 1 face companion attachment through `SetAFM`, `SetPFM`, and
  `SetCompanionMetrics`, plus `SetCompanionMetricsFiles` for explicit path
  attachment and `SetCompanionMetricsForFont` for discovered same-stem
  companions.
  Attached AFM data can override glyph widths and left-side bearings, while PFM
  data can provide encoded fallback widths.
- AFM KPX kerning is applied by the Type 1 `Shape` path and scales with the
  selected pixel size.
- Private hint metadata parsing for blue zones, standard stems, stem snaps,
  force-bold, and language group values.
- Basic Type 1 charstring outline construction.
- Native Type 1 path segment recording, including cubic segments for
  inspection/conformance work, segment utility helpers, conformance dump
  records, and `core.Outline` cubic tags for the raster path.
- Basic charstring metrics, including width/side-bearing data used by the
  loader.
- Operand stack, top-level terminator, local Subr return, unterminated flex,
  pending OtherSubr result queue, and subroutine nesting checks reject
  malformed or runaway Type 1 charstrings.
- Stem hint and counter hint records are decoded for inspection. A conservative
  exact-edge snap scaffold exists for hinted scaled loads, but it is not a
  FreeType parity claim.
- Subr calls used by the current charstring interpreter, including regression
  coverage that `callsubr` preserves operands needed by standard flex Subrs and
  still respects the operand stack limit. Limited OtherSubr 3 result-pop queue
  handling is implemented, with guards for malformed terminators and pending
  result queues. Supported OtherSubr return values are intentionally narrow:
  they must be consumed by `pop` before any later operand or non-`pop`
  operator.
- Basic flex handling through direct `callothersubr` and standard flex Subr
  patterns.
- SEAC composite glyph outline composition through the Type 1 face loader.
- A Type 1 `api.Face` loader path, so Type 1 faces can be surfaced through the
  public facade instead of only package-local parsing tests.
- Go conformance dump fallback coverage for generated standalone Type 1 PFA/PFB
  smoke fonts.

Still unsupported or not yet proven:

- Automatic discovery/loading of `.afm` and `.pfm` companion files for plain
  public `api.Stream` workflows. Current loader auto-attachment requires a
  stream that implements `FontPathStream`; `core.FileStream.Type1FontPath()`
  narrows this for ordinary file-backed core streams, but non-path streams
  still require explicit helper calls after a face is loaded.
- FreeType-comparable Type 1 hint application, including stem fitting, blue
  zones, and alignment zone behavior.
- Full OtherSubrs semantics beyond the limited flex and result-pop queue cases
  currently exercised.
- Multiple Masters Type 1 fonts.
- Real Type 1 corpus differential testing against C FreeType for parsing,
  encoding, metrics, outlines, Subrs, flex, full OtherSubrs, hints, malformed
  inputs, and loader behavior.

## Minimum Criteria Before Claiming Completion

Do not describe the port, or a major rendering/font area, as complete until all
applicable criteria are true:

- The area has package tests for normal cases, boundary cases, and malformed
  inputs that the parser or renderer is expected to reject.
- The conformance corpus includes licensed real fonts, not only synthetic
  fixtures or locally discovered fonts.
- The same corpus can be dumped with the Go engine and with C FreeType, then
  compared with zero unexpected differences for the scoped fields.
- Any remaining expected gaps are narrow, documented, and intentionally outside
  the claimed scope.
- Rendered output claims include bitmap geometry, placement, pixel mode, pitch,
  and buffer/hash comparisons for every claimed render mode.
- Hinting claims include VM behavior under relevant load flags and target modes,
  not only unhinted outline parity.

## Primary Remaining Gaps

- **FreeType bitmap bit parity**: Rendered bitmap bytes, pitch, placement, mono
  packing, LCD/LCD-V filtering, and strike output still need byte-level
  convergence.
- **Hinting VM parity**: The interpreter needs broader opcode, graphics-state,
  target-mode, composite, variation, and error-path comparisons against
  FreeType. SHP/ALIGNRP/FLIPPT/SHPIX/IP preflight and rollback coverage is a
  focused regression slice, not a complete instruction-family claim.
- **TrueType size model parity**: Regression tests now cover important
  `SetPixelSizes`, CVT, cvar, prep, storage, and twilight-state behavior, but
  the loader-level size model is not yet a full FreeType size/driver-state
  compatibility claim.
- **Real corpus coverage**: The checked-in requests intentionally avoid bundling
  font binaries. Licensed fixture selection and scheduled FreeType comparison
  runs are still needed.
- **WOFF2 corpus and collection coverage**: Transformed `glyf`/`loca`,
  instruction/bbox reconstruction, short/long `loca` multiglyph cases,
  per-glyph overlap bitmaps, collection long-multiglyph sharing, collection
  reconstruction, shared transformed composite glyphs, and transformed `hmtx`
  dependency guards, including raw/transformed outline mismatch, malformed
  shared-`hmtx` metadata, malformed long-`loca`/raw-glyph shared-`hmtx` cases,
  and shared-composite rejection, have focused synthetic tests, but real WOFF2
  corpus coverage, remaining collection variants, and FreeType comparison
  remain open.
- **COLR pixel renderer**: Parsing and paint evaluation are not the same as a
  FreeType-comparable COLR renderer. Pixel composition, gradients, transforms,
  clips, palette variation, and blending remain open.
- **CFF/CFF2 and Type 1 corpus**: Synthetic tests need to be backed by real
  corpus runs that cover charstrings, subroutines, dict variants, blends, hints,
  encodings, metrics, loader behavior, and malformed inputs. Type 1 companion
  discovery is still path-based and only automatic for `FontPathStream`
  implementations, and Type 1 still lacks Multiple Masters, full OtherSubrs
  beyond the current guarded cases, and hinting/stem-zone parity.
- **Broad TrueType size and driver parity**: Public loaded-face size regression
  tests now cover important resize behavior, but this is still narrower than
  FreeType size object, driver state, target mode, and hinted rendering parity.
- **Broad FreeType conformance deltas**: Many areas have package coverage but
  still need sustained Go-vs-FreeType comparison runs before any broad
  compatibility statement is justified.
- **Stable public API semantics**: FreeType-like names exist, but driver-level
  behavior and supported combinations still need documentation and compatibility
  gates.

## Suggested Tracking Workflow

1. Add or update a request JSON file that isolates the area being measured.
2. Run the Go dump and optional FreeType dump for the same licensed font set.
3. Compare with zero tolerances first.
4. Add `expected_gaps` only for known, intentional differences.
5. Treat stale expected gaps as cleanup work or evidence that an area may be
   ready for a narrower parity claim.

The checked-in request templates now include dedicated tracking slices for
Type 1 native segment/outline deltas, WOFF2 transformed table and collection
decode coverage, and hinted TrueType size/VM behavior. These templates are
measurement scopes only: they require suitable out-of-band licensed fonts and
do not by themselves prove parity.
