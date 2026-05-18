//go:build cgo && freetype_conformance

package main

/*
#cgo pkg-config: freetype2
#include <stdlib.h>
#include <ft2build.h>
#include FT_FREETYPE_H

#ifndef FT_LOAD_COMPUTE_METRICS
#define FT_LOAD_COMPUTE_METRICS 0
#endif
#ifndef FT_LOAD_BITMAP_METRICS_ONLY
#define FT_LOAD_BITMAP_METRICS_ONLY 0
#endif
#ifndef FT_LOAD_NO_SVG
#define FT_LOAD_NO_SVG 0
#endif

static long ftgo_face_num_faces(FT_Face face) {
	return face->num_faces;
}

static long ftgo_face_num_glyphs(FT_Face face) {
	return face->num_glyphs;
}

static unsigned short ftgo_face_units_per_em(FT_Face face) {
	return face->units_per_EM;
}

static long ftgo_metric_hori_advance(FT_Face face) {
	return face->glyph->metrics.horiAdvance;
}

static long ftgo_metric_width(FT_Face face) {
	return face->glyph->metrics.width;
}

static long ftgo_metric_height(FT_Face face) {
	return face->glyph->metrics.height;
}

static long ftgo_metric_hori_bearing_x(FT_Face face) {
	return face->glyph->metrics.horiBearingX;
}

static long ftgo_metric_hori_bearing_y(FT_Face face) {
	return face->glyph->metrics.horiBearingY;
}

static long ftgo_metric_vert_bearing_x(FT_Face face) {
	return face->glyph->metrics.vertBearingX;
}

static long ftgo_metric_vert_bearing_y(FT_Face face) {
	return face->glyph->metrics.vertBearingY;
}

static long ftgo_metric_vert_advance(FT_Face face) {
	return face->glyph->metrics.vertAdvance;
}

static int ftgo_glyph_format(FT_Face face) {
	return face->glyph->format;
}

static int ftgo_format_outline(void) {
	return FT_GLYPH_FORMAT_OUTLINE;
}

static int ftgo_format_bitmap(void) {
	return FT_GLYPH_FORMAT_BITMAP;
}

static int ftgo_format_composite(void) {
	return FT_GLYPH_FORMAT_COMPOSITE;
}

static int ftgo_render_glyph(FT_Face face, FT_Render_Mode mode) {
	return FT_Render_Glyph(face->glyph, mode);
}

static short ftgo_outline_n_points(FT_Face face) {
	return face->glyph->outline.n_points;
}

static short ftgo_outline_n_contours(FT_Face face) {
	return face->glyph->outline.n_contours;
}

static long ftgo_outline_point_x(FT_Face face, int i) {
	return face->glyph->outline.points[i].x;
}

static long ftgo_outline_point_y(FT_Face face, int i) {
	return face->glyph->outline.points[i].y;
}

static unsigned char ftgo_outline_tag(FT_Face face, int i) {
	return (unsigned char)face->glyph->outline.tags[i];
}

static short ftgo_outline_contour(FT_Face face, int i) {
	return face->glyph->outline.contours[i];
}

static unsigned int ftgo_bitmap_rows(FT_Face face) {
	return face->glyph->bitmap.rows;
}

static unsigned int ftgo_bitmap_width(FT_Face face) {
	return face->glyph->bitmap.width;
}

static int ftgo_bitmap_pitch(FT_Face face) {
	return face->glyph->bitmap.pitch;
}

static unsigned char ftgo_bitmap_pixel_mode(FT_Face face) {
	return face->glyph->bitmap.pixel_mode;
}

static int ftgo_bitmap_left(FT_Face face) {
	return face->glyph->bitmap_left;
}

static int ftgo_bitmap_top(FT_Face face) {
	return face->glyph->bitmap_top;
}

static unsigned char* ftgo_bitmap_buffer(FT_Face face) {
	return face->glyph->bitmap.buffer;
}
*/
import "C"

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"unsafe"

	"github.com/dh-kam/freetype-go/api"
)

func buildFreeTypeDump(opts dumpOptions) (*Dump, error) {
	source, err := buildSourceInfo(opts.FontPath)
	if err != nil {
		return nil, err
	}

	var library C.FT_Library
	if code := C.FT_Init_FreeType(&library); code != 0 {
		return nil, fmt.Errorf("FT_Init_FreeType failed: %d", int(code))
	}
	defer C.FT_Done_FreeType(library)

	path := C.CString(opts.FontPath)
	defer C.free(unsafe.Pointer(path))

	var face C.FT_Face
	if code := C.FT_New_Face(library, path, C.FT_Long(opts.FaceIndex), &face); code != 0 {
		return nil, fmt.Errorf("FT_New_Face(%q, %d) failed: %d", opts.FontPath, opts.FaceIndex, int(code))
	}
	defer C.FT_Done_Face(face)
	_ = C.FT_Select_Charmap(face, C.FT_ENCODING_UNICODE)

	selections, charmap := resolveFreeTypeSelections(face, opts)
	dump := newDump("freetype", freeTypeVersion(library), source, opts, FaceInfo{
		FaceIndex:  opts.FaceIndex,
		NumFaces:   int(C.ftgo_face_num_faces(face)),
		NumGlyphs:  int(C.ftgo_face_num_glyphs(face)),
		UnitsPerEm: uint16(C.ftgo_face_units_per_em(face)),
	}, charmap)

	for _, loadFlags := range opts.LoadFlags {
		for _, renderMode := range opts.RenderModes {
			for _, ppem := range opts.PPEMs {
				sizeDump := SizeDump{
					PPEMX:      ppem.X,
					PPEMY:      ppem.Y,
					LoadFlags:  loadFlags.Name,
					RenderMode: renderMode.Name,
					Glyphs:     []GlyphRecord{},
				}
				if code := C.FT_Set_Pixel_Sizes(face, C.FT_UInt(ppem.X), C.FT_UInt(ppem.Y)); code != 0 {
					sizeDump.Error = fmt.Sprintf("FT_Set_Pixel_Sizes failed: %d", int(code))
					dump.Sizes = append(dump.Sizes, sizeDump)
					continue
				}
				for _, sel := range selections {
					sizeDump.Glyphs = append(sizeDump.Glyphs, dumpFreeTypeGlyph(face, sel, loadFlags, renderMode, opts.IncludeBitmapBuffer))
				}
				dump.Sizes = append(dump.Sizes, sizeDump)
			}
		}
	}

	return dump, nil
}

func resolveFreeTypeSelections(face C.FT_Face, opts dumpOptions) ([]glyphSelection, []CharMapRecord) {
	order := make([]int, 0, len(opts.Glyphs)+len(opts.Chars))
	seen := make(map[int]bool)
	charsByGlyph := make(map[int][]string)

	for _, glyph := range opts.Glyphs {
		if !seen[glyph] {
			seen[glyph] = true
			order = append(order, glyph)
		}
	}

	charmap := make([]CharMapRecord, 0, len(opts.Chars))
	for _, ch := range opts.Chars {
		glyph := int(C.FT_Get_Char_Index(face, C.FT_ULong(ch.Rune)))
		charmap = append(charmap, CharMapRecord{
			Char:       ch.Label,
			GlyphIndex: glyph,
		})
		charsByGlyph[glyph] = append(charsByGlyph[glyph], ch.Label)
		if !seen[glyph] {
			seen[glyph] = true
			order = append(order, glyph)
		}
	}

	if len(order) == 0 {
		order = append(order, 0)
	}

	selections := make([]glyphSelection, 0, len(order))
	for _, glyph := range order {
		selections = append(selections, glyphSelection{
			GlyphIndex: glyph,
			Chars:      append([]string(nil), charsByGlyph[glyph]...),
		})
	}
	return selections, charmap
}

func dumpFreeTypeGlyph(face C.FT_Face, sel glyphSelection, loadFlags loadFlagSpec, renderMode renderModeSpec, includeBitmapBuffer bool) GlyphRecord {
	record := GlyphRecord{
		GlyphIndex: sel.GlyphIndex,
		Chars:      sel.Chars,
		Outline:    OutlineRecord{Available: false},
		Bitmap:     BitmapRecord{Available: false},
	}

	if code := C.FT_Load_Glyph(face, C.FT_UInt(sel.GlyphIndex), freeTypeLoadFlags(loadFlags)); code != 0 {
		err := fmt.Sprintf("FT_Load_Glyph failed: %d", int(code))
		record.LoadError = err
		record.Metrics = MetricsRecord{Available: false, Error: err}
		record.Outline.Error = err
		record.Bitmap.Error = err
		return record
	}

	record.Metrics = MetricsRecord{
		Available: true,
		Advance:   int32(C.ftgo_metric_hori_advance(face)),
		LSB:       int32(C.ftgo_metric_hori_bearing_x(face)),
	}
	record.SlotMetrics = dumpFreeTypeSlotMetrics(face)
	record.Format = freeTypeGlyphFormatName(face)
	record.Outline = dumpFreeTypeOutline(face)
	if renderMode.Value != api.RenderModeNone {
		if code := C.ftgo_render_glyph(face, freeTypeRenderMode(renderMode)); code != 0 {
			record.RenderError = fmt.Sprintf("FT_Render_Glyph(%s) failed: %d", renderMode.Name, int(code))
		}
	}
	record.RenderedFormat = freeTypeGlyphFormatName(face)
	record.Bitmap = dumpFreeTypeBitmap(face, includeBitmapBuffer)
	if record.RenderError != "" && !record.Bitmap.Available {
		record.Bitmap.Error = record.RenderError
	}
	return record
}

func dumpFreeTypeSlotMetrics(face C.FT_Face) *SlotMetricsRecord {
	return slotMetricsRecord(api.GlyphMetrics{
		Width:        int32(C.ftgo_metric_width(face)),
		Height:       int32(C.ftgo_metric_height(face)),
		HoriBearingX: int32(C.ftgo_metric_hori_bearing_x(face)),
		HoriBearingY: int32(C.ftgo_metric_hori_bearing_y(face)),
		HoriAdvance:  int32(C.ftgo_metric_hori_advance(face)),
		VertBearingX: int32(C.ftgo_metric_vert_bearing_x(face)),
		VertBearingY: int32(C.ftgo_metric_vert_bearing_y(face)),
		VertAdvance:  int32(C.ftgo_metric_vert_advance(face)),
	})
}

func dumpFreeTypeOutline(face C.FT_Face) OutlineRecord {
	if C.ftgo_glyph_format(face) != C.ftgo_format_outline() {
		return OutlineRecord{Available: false, Error: "outline unavailable"}
	}

	pointCount := int(C.ftgo_outline_n_points(face))
	contourCount := int(C.ftgo_outline_n_contours(face))
	points := make([]Point, pointCount)
	tags := make([]int, pointCount)
	contours := make([]int, contourCount)
	for i := 0; i < pointCount; i++ {
		points[i] = Point{
			X: int32(C.ftgo_outline_point_x(face, C.int(i))),
			Y: int32(C.ftgo_outline_point_y(face, C.int(i))),
		}
		tags[i] = int(C.ftgo_outline_tag(face, C.int(i)))
	}
	for i := 0; i < contourCount; i++ {
		contours[i] = int(C.ftgo_outline_contour(face, C.int(i)))
	}

	return OutlineRecord{
		Available:     true,
		PointCount:    pointCount,
		ContourCount:  contourCount,
		RawPointCount: pointCount,
		Points:        points,
		Tags:          tags,
		Contours:      contours,
		BBox:          bbox(points),
	}
}

func dumpFreeTypeBitmap(face C.FT_Face, includeBuffer bool) BitmapRecord {
	rows := int(C.ftgo_bitmap_rows(face))
	width := int(C.ftgo_bitmap_width(face))
	pitch := int(C.ftgo_bitmap_pitch(face))
	buffer := C.ftgo_bitmap_buffer(face)
	if rows == 0 || width == 0 || pitch == 0 || buffer == nil {
		return BitmapRecord{Available: false, Error: "bitmap unavailable"}
	}

	stride := pitch
	if stride < 0 {
		stride = -stride
	}
	size := rows * stride
	data := C.GoBytes(unsafe.Pointer(buffer), C.int(size))
	sum := sha256.Sum256(data)
	pixelMode := uint8(C.ftgo_bitmap_pixel_mode(face))
	record := BitmapRecord{
		Available:     true,
		Rows:          rows,
		Width:         width,
		Pitch:         pitch,
		PixelMode:     pixelMode,
		PixelModeName: pixelModeName(pixelMode),
		Left:          int(C.ftgo_bitmap_left(face)),
		Top:           int(C.ftgo_bitmap_top(face)),
		BufferSize:    len(data),
		SHA256:        fmt.Sprintf("%x", sum),
	}
	if includeBuffer {
		record.BufferHex = hex.EncodeToString(data)
	}
	return record
}

func freeTypeGlyphFormatName(face C.FT_Face) string {
	format := C.ftgo_glyph_format(face)
	switch format {
	case C.ftgo_format_outline():
		return "outline"
	case C.ftgo_format_bitmap():
		return "bitmap"
	case C.ftgo_format_composite():
		return "composite"
	default:
		return fmt.Sprintf("0x%08x", uint32(format))
	}
}

func freeTypeLoadFlags(loadFlags loadFlagSpec) C.FT_Int32 {
	flags := C.FT_Int32(C.FT_LOAD_DEFAULT)
	for _, component := range loadFlags.Components {
		switch component {
		case "default":
		case "no-hinting":
			flags |= C.FT_Int32(C.FT_LOAD_NO_HINTING)
		case "no-scale":
			flags |= C.FT_Int32(C.FT_LOAD_NO_SCALE)
		case "render":
			flags |= C.FT_Int32(C.FT_LOAD_RENDER)
		case "no-bitmap":
			flags |= C.FT_Int32(C.FT_LOAD_NO_BITMAP)
		case "vertical-layout":
			flags |= C.FT_Int32(C.FT_LOAD_VERTICAL_LAYOUT)
		case "force-autohint":
			flags |= C.FT_Int32(C.FT_LOAD_FORCE_AUTOHINT)
		case "crop-bitmap":
			flags |= C.FT_Int32(C.FT_LOAD_CROP_BITMAP)
		case "pedantic":
			flags |= C.FT_Int32(C.FT_LOAD_PEDANTIC)
		case "ignore-global-advance-width":
			flags |= C.FT_Int32(C.FT_LOAD_IGNORE_GLOBAL_ADVANCE_WIDTH)
		case "no-recurse":
			flags |= C.FT_Int32(C.FT_LOAD_NO_RECURSE)
		case "ignore-transform":
			flags |= C.FT_Int32(C.FT_LOAD_IGNORE_TRANSFORM)
		case "monochrome":
			flags |= C.FT_Int32(C.FT_LOAD_MONOCHROME)
		case "linear-design":
			flags |= C.FT_Int32(C.FT_LOAD_LINEAR_DESIGN)
		case "no-autohint":
			flags |= C.FT_Int32(C.FT_LOAD_NO_AUTOHINT)
		case "color":
			flags |= C.FT_Int32(C.FT_LOAD_COLOR)
		case "compute-metrics":
			flags |= C.FT_Int32(C.FT_LOAD_COMPUTE_METRICS)
		case "bitmap-metrics-only":
			flags |= C.FT_Int32(C.FT_LOAD_BITMAP_METRICS_ONLY)
		case "no-svg":
			flags |= C.FT_Int32(C.FT_LOAD_NO_SVG)
		case "target-normal":
			flags |= C.FT_Int32(C.FT_LOAD_TARGET_NORMAL)
		case "target-light":
			flags |= C.FT_Int32(C.FT_LOAD_TARGET_LIGHT)
		case "target-mono":
			flags |= C.FT_Int32(C.FT_LOAD_TARGET_MONO)
		case "target-lcd":
			flags |= C.FT_Int32(C.FT_LOAD_TARGET_LCD)
		case "target-lcd-v":
			flags |= C.FT_Int32(C.FT_LOAD_TARGET_LCD_V)
		}
	}
	return flags
}

func freeTypeRenderMode(renderMode renderModeSpec) C.FT_Render_Mode {
	switch renderMode.Value {
	case api.RenderModeNormal:
		return C.FT_RENDER_MODE_NORMAL
	case api.RenderModeLight:
		return C.FT_RENDER_MODE_LIGHT
	case api.RenderModeMono:
		return C.FT_RENDER_MODE_MONO
	case api.RenderModeLCD:
		return C.FT_RENDER_MODE_LCD
	case api.RenderModeLCDV:
		return C.FT_RENDER_MODE_LCD_V
	default:
		return C.FT_RENDER_MODE_NORMAL
	}
}

func freeTypeVersion(library C.FT_Library) string {
	var major C.FT_Int
	var minor C.FT_Int
	var patch C.FT_Int
	C.FT_Library_Version(library, &major, &minor, &patch)
	return fmt.Sprintf("%d.%d.%d", int(major), int(minor), int(patch))
}
