# Conformance and Verification Harness

This project remains CGO-free by default. FreeType C library checks are optional
and must not be required by normal CI or by `go test ./...`.

## Current CI Tier

Default CI should stay dependency-light and bounded:

- `make test`
- `make vet`
- `make test-race`
- `make fuzz-smoke FUZZTIME=2s`

The fuzz smoke target is intentionally short. It is a regression tripwire for
parser crashes and excessive allocation, not a substitute for longer local or
scheduled fuzzing.

## JSON Dump Tool

The conformance CLI lives in `tools/conformance`. Its default `dump` command
uses only the Go engine and has no FreeType/cgo dependency:

```sh
go run ./tools/conformance dump \
  -font /path/to/font.ttf \
  -out conformance-go.json \
  -ppem 12,16,24 \
  -glyphs 0 \
  -chars U+0020,U+0030,U+0041,U+0061 \
  -load-flags no-hinting,target-light+no-hinting \
  -render-mode none
```

The same command is wrapped by:

```sh
make conformance-dump CONFORMANCE_FONT=/path/to/font.ttf
```

`dump` records face metadata, selected charmap lookups, glyph metrics, optional
rich slot metrics, outline geometry, glyph format, bitmap metadata, and embedded
image metadata when present. Metrics and outline coordinates are serialized as
signed 26.6 fixed-point integers. When a loader exposes non-FreeType helper
points, the JSON schema stores FreeType-comparable outline points separately
from `phantom_points`.

`-load-flags` accepts comma-separated flag sets. Within a set, combine flags
with `+`, for example `default,no-hinting+target-light,no-bitmap+target-mono`.
The Go engine currently implements only a subset; unsupported concepts are still
recorded so the reference comparison shows the semantic gap. `-render-mode`
accepts `none`, `normal`, `light`, `mono`, `lcd`, and `lcd-v`. FreeType
reference dumps call `FT_Render_Glyph` for non-`none` modes. The Go dump records
an explicit render error until a matching render-to-slot API exists.

Top-level schema:

```json
{
  "schema": "ftgo.conformance.dump/v1",
  "engine": {"name": "go-freetype"},
  "source": {"path": "font.ttf", "size": 1234, "sha256": "..."},
  "request": {
    "face_index": 0,
    "ppem": ["12"],
    "glyphs": [0],
    "load_flags": "no-hinting",
    "render_mode": "none"
  },
  "face": {"face_index": 0, "num_faces": 1, "num_glyphs": 42, "units_per_em": 2048},
  "sizes": []
}
```

Each glyph record contains:

- `metrics`: `advance` and `lsb` in 26.6 pixels.
- `slot_metrics`: FreeType-style width, height, bearings, and advances when the
  engine exposes loaded glyph slot metrics.
- `outline`: contour endpoints, point tags, bbox, points, raw point count, and
  phantom points.
- `bitmap`: rows, width, pitch, pixel mode/name, left/top bearing, byte length,
  and SHA-256 when a bitmap is exposed.
- `image`: decoded embedded image dimensions and SHA-256 when an image decoder
  supplies one.

Unavailable data is represented explicitly with `available: false` and an
`error` string. This lets early conformance runs show porting gaps without
crashing the harness.

## Optional FreeType Reference Dump

The `ftdump` subcommand emits the same JSON schema using the C FreeType library.
It is behind the `freetype_conformance` build tag and `cgo`, so it is excluded
from default builds:

```sh
CGO_ENABLED=1 go run -tags freetype_conformance ./tools/conformance ftdump \
  -font /path/to/font.ttf \
  -out conformance-freetype.json \
  -ppem 12,16,24 \
  -glyphs 0 \
  -chars U+0020,U+0030,U+0041,U+0061 \
  -load-flags no-hinting,target-light+no-hinting \
  -render-mode none,normal
```

The same reference path is wrapped by:

```sh
make conformance-ftdump CONFORMANCE_FONT=/path/to/font.ttf
```

The host must provide `pkg-config freetype2`. Keep reference dumps out of the
repository unless the fixture font license and dump contents are safe to commit.

The older math parity harness remains available under the existing build tag:

```sh
CGO_ENABLED=1 go test -tags freetype_harness ./harness
```

`make test-harness` wraps that target and remains outside default CI.

## Comparing Dumps

Compare a FreeType reference dump and a Go dump with:

```sh
go run ./tools/conformance compare \
  -reference conformance-freetype.json \
  -candidate conformance-go.json \
  -metric-tolerance 0 \
  -point-tolerance 0 \
  -allow-missing-slot-metrics
```

The Makefile wrapper is:

```sh
make conformance-compare \
  CONFORMANCE_REF=conformance-freetype.json \
  CONFORMANCE_CANDIDATE=conformance-go.json
```

The comparer checks matching source hashes, face metadata, charmap lookups,
glyph load/render errors, glyph formats, metrics, rich slot metrics, outline
counts, raw/phantom point counts, bbox, contours, tags, points, and bitmap
metadata/hashes. It exits non-zero when mismatches are found. Use non-zero
tolerances only for known fixed-point rounding differences under active
investigation.

When the candidate is the current Go engine, render-mode and rich slot metric
gaps are called out separately before the raw diff list. For example, a rendered
FreeType glyph compared against an unrendered Go slot is grouped as
`go render unsupported`, `go render bitmap missing`, or `go render output
mismatch`; missing `GlyphSlot` metrics are grouped as
`go slot metrics unavailable`. These labels are report annotations only; they do
not hide diffs or change the non-zero exit status.

## Request Files and Corpora

Dump commands can read a request JSON file. Explicit CLI flags override request
fields; Makefile request mode defers selection fields to the request file.

```json
{
  "font": "/path/to/font.ttf",
  "face_index": 0,
  "ppem": ["12", "16x20"],
  "glyphs": [0, 3],
  "glyph_ranges": ["10-12"],
  "chars": ["U+0020", "U+0041", "U+0061"],
  "load_flags": ["default", "no-hinting+target-light"],
  "render_mode": ["none", "normal"],
  "corpus": "smoke"
}
```

Useful wrappers:

```sh
make conformance-dump CONFORMANCE_REQUEST=request.json
make conformance-corpus CONFORMANCE_FONTDIR=/path/to/fonts CONFORMANCE_REQUEST=request.json
```

`glyphs` can be a number list (`[0, 3]`), a comma-separated string
(`"0,3,10-12"`), or a string list (`["0", "3-5"]`). `glyph_ranges` remains
available for request files that keep ranges separate. `description` is accepted
for human-readable notes and is ignored by the runner.

## Batch Request Workflow

Use `batch` when the corpus is described by multiple request JSON files:

```sh
go run ./tools/conformance batch \
  -requests 'testdata/conformance/*.json' \
  -font /path/to/font.ttf \
  -out-dir conformance-out
```

The Go Makefile wrapper is:

```sh
make conformance-batch \
  CONFORMANCE_REQUESTS='testdata/conformance/*.json' \
  CONFORMANCE_FONT=/path/to/font.ttf
```

For an optional FreeType reference corpus, use the cgo-gated engine:

```sh
make conformance-ftbatch \
  CONFORMANCE_REQUESTS='testdata/conformance/*.json' \
  CONFORMANCE_FONT=/path/to/font.ttf
```

By default, batch outputs are named from the request basename:
`ascii-smoke.go.json` for the Go engine and `ascii-smoke.freetype.json` for the
FreeType engine. Compare the per-request outputs with:

```sh
go run ./tools/conformance batch-compare \
  -requests 'testdata/conformance/*.json' \
  -reference-dir conformance-out \
  -candidate-dir conformance-out
```

The Makefile wrapper is:

```sh
make conformance-batch-compare \
  CONFORMANCE_REQUESTS='testdata/conformance/*.json'
```

`batch` and `batch-compare` accept repeated `-request`/`-requests` flags,
comma-separated lists, and quoted glob patterns. The default Go batch path has
no FreeType or cgo dependency; only `ftdump` and `conformance-ftbatch` require
the external FreeType library.

## Fixture Strategy

Use fixture fonts supplied out of band, for example through
`FTGO_CONFORMANCE_FONTDIR` or a local path passed to `CONFORMANCE_FONT`, unless
the font license allows committing the fixture.

Recommended glyph sets for the next conformance rounds:

- Smoke: `.notdef`, space, `0`, `A`, `a`.
- Contours: simple on-curve, quadratic off-curve, empty glyph, composite glyph.
- Scripts: representative Latin, CJK, symbol, and missing-glyph lookups.
- Bitmap/color: bitmap strikes, COLR/CPAL layers, sbix/CBDT payloads.
- Variations: default instance plus one non-default axis tuple.

## Next Expansion Points

- Add a small licensed fixture under `testdata/conformance` once a suitable font
  is selected.
- Add scheduled CI that installs FreeType and runs `ftdump` plus `compare` over
  the request corpus.
- Implement Go glyph slot metrics and render-to-slot bitmap APIs so
  `slot_metrics`, bitmap offsets, and render-mode dumps can converge.
- Promote stable reference dumps to golden tests only when fixture licensing is
  clear and the compared field is implemented intentionally.
