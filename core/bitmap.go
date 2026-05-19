package core

import "github.com/dh-kam/freetype-go/api"

// PixelModeLCDV is kept for existing callers. New code should use
// api.MODE_LCD_V.
const PixelModeLCDV uint8 = api.MODE_LCD_V

const lcdFilterPadding26Dot6 int32 = 43

// BitmapMetrics describes the bitmap surface implied by an outline render.
// Width and Rows are the rounded render box in pixels. SurfaceWidth and
// SurfaceRows include subpixel expansion for LCD/LCDV render modes.
type BitmapMetrics struct {
	Left         int
	Top          int
	Width        int
	Rows         int
	SurfaceWidth int
	SurfaceRows  int
	Pitch        int
	PixelMode    uint8
}

// Bitmap implements api.Bitmap.
type Bitmap struct {
	Rows      int
	Width     int
	Pitch     int
	Buffer    []byte
	PixelMode uint8
	Left      int
	Top       int

	packedMono bool
}

func (b *Bitmap) GetRows() int {
	return b.Rows
}

func (b *Bitmap) GetWidth() int {
	return b.Width
}

func (b *Bitmap) GetPitch() int {
	return b.Pitch
}

func (b *Bitmap) GetBuffer() []byte {
	return b.Buffer
}

func (b *Bitmap) GetPixelMode() uint8 {
	return b.PixelMode
}

func (b *Bitmap) GetLeft() int {
	return b.Left
}

func (b *Bitmap) GetTop() int {
	return b.Top
}

func (b *Bitmap) SetPixelMode(mode uint8) {
	b.PixelMode = mode
	if mode != api.MODE_MONO {
		b.packedMono = false
	}
}

// NewBitmap creates a new Bitmap and allocates its buffer.
//
// This preserves the historical ftgo behavior of one byte per pixel. Use
// NewBitmapWithPixelMode or NewBitmapForRenderMode when FreeType-compatible
// packed mono or LCD/LCDV surface dimensions are required.
func NewBitmap(width, rows int) *Bitmap {
	width = clampBitmapDimension(width)
	rows = clampBitmapDimension(rows)
	pitch := width
	return &Bitmap{
		Rows:      rows,
		Width:     width,
		Pitch:     pitch,
		Buffer:    make([]byte, bitmapBufferLen(pitch, rows)),
		PixelMode: api.MODE_GRAY,
	}
}

// NewBitmapWithPixelMode creates a bitmap with FreeType-compatible pitch for
// the supplied pixel mode. MODE_MONO uses MSB-first packed bits.
func NewBitmapWithPixelMode(width, rows int, pixelMode uint8) *Bitmap {
	width = clampBitmapDimension(width)
	rows = clampBitmapDimension(rows)
	pitch := BitmapPitch(width, pixelMode)
	return &Bitmap{
		Rows:       rows,
		Width:      width,
		Pitch:      pitch,
		Buffer:     make([]byte, bitmapBufferLen(pitch, rows)),
		PixelMode:  pixelMode,
		packedMono: pixelMode == api.MODE_MONO,
	}
}

// NewBitmapForRenderMode creates a bitmap surface for logical glyph bounds in
// the requested render mode. LCD triples width; LCDV triples rows.
func NewBitmapForRenderMode(width, rows int, mode api.RenderMode) *Bitmap {
	surfaceWidth, surfaceRows, pixelMode := BitmapSurfaceForRenderMode(width, rows, mode)
	return NewBitmapWithPixelMode(surfaceWidth, surfaceRows, pixelMode)
}

// BitmapPitch returns the positive pitch in bytes for a bitmap row.
func BitmapPitch(width int, pixelMode uint8) int {
	width = clampBitmapDimension(width)
	if width == 0 {
		return 0
	}
	switch pixelMode {
	case api.MODE_NONE:
		return 0
	case api.MODE_MONO:
		return ((width + 15) >> 4) << 1
	case api.MODE_LCD:
		return padBitmapDimension(width, 4)
	default:
		return width
	}
}

// PixelModeForRenderMode maps a FreeType render mode to the bitmap pixel mode
// used by core/raster helpers.
func PixelModeForRenderMode(mode api.RenderMode) uint8 {
	switch mode {
	case api.RenderModeNone:
		return api.MODE_NONE
	case api.RenderModeMono:
		return api.MODE_MONO
	case api.RenderModeLCD:
		return api.MODE_LCD
	case api.RenderModeLCDV:
		return api.MODE_LCD_V
	default:
		return api.MODE_GRAY
	}
}

// BitmapSurfaceForRenderMode returns surface dimensions and pixel mode for
// logical glyph bounds in a FreeType render mode.
func BitmapSurfaceForRenderMode(width, rows int, mode api.RenderMode) (surfaceWidth, surfaceRows int, pixelMode uint8) {
	surfaceWidth = clampBitmapDimension(width)
	surfaceRows = clampBitmapDimension(rows)
	pixelMode = PixelModeForRenderMode(mode)
	switch mode {
	case api.RenderModeLCD:
		surfaceWidth *= 3
	case api.RenderModeLCDV:
		surfaceRows *= 3
	}
	return surfaceWidth, surfaceRows, pixelMode
}

// IsPackedMono reports whether this bitmap stores MODE_MONO pixels as
// FreeType-compatible packed bits instead of one byte per pixel.
func (b *Bitmap) IsPackedMono() bool {
	return b != nil && b.PixelMode == api.MODE_MONO && b.packedMono
}

// SetMonoPixel writes one packed mono pixel using FreeType's MSB-first bit
// order. It returns false when the target position is outside the buffer.
func SetMonoPixel(buffer []byte, pitch, x, y int, on bool) bool {
	if pitch <= 0 || x < 0 || y < 0 {
		return false
	}
	offset := y*pitch + (x >> 3)
	if offset < 0 || offset >= len(buffer) {
		return false
	}
	mask := byte(0x80 >> uint(x&7))
	if on {
		buffer[offset] |= mask
	} else {
		buffer[offset] &^= mask
	}
	return true
}

// MonoPixel reads one packed mono pixel using FreeType's MSB-first bit order.
func MonoPixel(buffer []byte, pitch, x, y int) bool {
	if pitch <= 0 || x < 0 || y < 0 {
		return false
	}
	offset := y*pitch + (x >> 3)
	if offset < 0 || offset >= len(buffer) {
		return false
	}
	return buffer[offset]&(0x80>>uint(x&7)) != 0
}

// FillMonoSpan sets a packed mono half-open span [x1, x2) on one scanline.
// The caller is responsible for clipping x2 to the logical bitmap width.
func FillMonoSpan(buffer []byte, pitch, y, x1, x2 int) bool {
	if pitch <= 0 || y < 0 || x1 >= x2 {
		return false
	}
	if x1 < 0 {
		x1 = 0
	}
	maxX := pitch * 8
	if x2 > maxX {
		x2 = maxX
	}
	if x1 >= x2 {
		return false
	}
	rowStart := y * pitch
	if rowStart < 0 || rowStart >= len(buffer) {
		return false
	}
	rowEnd := rowStart + pitch
	if rowEnd > len(buffer) {
		return false
	}
	line := buffer[rowStart:rowEnd]
	firstByte := x1 >> 3
	lastByte := (x2 - 1) >> 3
	if firstByte < 0 || firstByte >= len(line) {
		return false
	}
	if lastByte >= len(line) {
		lastByte = len(line) - 1
	}
	firstMask := byte(0xff >> uint(x1&7))
	lastMask := byte(0xff << uint(7-((x2-1)&7)))
	if firstByte == lastByte {
		line[firstByte] |= firstMask & lastMask
		return true
	}
	line[firstByte] |= firstMask
	for i := firstByte + 1; i < lastByte; i++ {
		line[i] = 0xff
	}
	line[lastByte] |= lastMask
	return true
}

// SetMonoPixel writes a pixel into the bitmap. Packed mono bitmaps use
// FreeType bit packing; legacy mono bitmaps use one byte per pixel.
func (b *Bitmap) SetMonoPixel(x, y int, on bool) bool {
	if b == nil || x < 0 || y < 0 || x >= b.Width || y >= b.Rows {
		return false
	}
	if b.IsPackedMono() {
		return SetMonoPixel(b.Buffer, b.Pitch, x, y, on)
	}
	offset := y*b.Pitch + x
	if offset < 0 || offset >= len(b.Buffer) {
		return false
	}
	if on {
		b.Buffer[offset] = 255
	} else {
		b.Buffer[offset] = 0
	}
	return true
}

// MonoPixel reads a mono pixel from either packed or legacy one-byte storage.
func (b *Bitmap) MonoPixel(x, y int) bool {
	if b == nil || x < 0 || y < 0 || x >= b.Width || y >= b.Rows {
		return false
	}
	if b.IsPackedMono() {
		return MonoPixel(b.Buffer, b.Pitch, x, y)
	}
	offset := y*b.Pitch + x
	return offset >= 0 && offset < len(b.Buffer) && b.Buffer[offset] != 0
}

// OutlineBitmapMetrics returns FreeType-style bitmap placement and surface
// metrics for an outline in 26.6 coordinates. pointCount limits bounds
// calculation so callers can exclude phantom points; pass -1 to use all points.
func OutlineBitmapMetrics(outline api.Outline, pointCount int, mode api.RenderMode) (BitmapMetrics, bool) {
	minX, minY, maxX, maxY, ok := outlineBounds26Dot6(outline, pointCount)
	if !ok {
		return BitmapMetrics{}, false
	}
	box := presetBitmapBox26Dot6(minX, minY, maxX, maxY, mode)
	width := int(box.xMax - box.xMin)
	rows := int(box.yMax - box.yMin)
	if width < 0 {
		width = 0
	}
	if rows < 0 {
		rows = 0
	}
	surfaceWidth, surfaceRows, pixelMode := BitmapSurfaceForRenderMode(width, rows, mode)
	return BitmapMetrics{
		Left:         int(box.xMin),
		Top:          int(box.yMax),
		Width:        width,
		Rows:         rows,
		SurfaceWidth: surfaceWidth,
		SurfaceRows:  surfaceRows,
		Pitch:        BitmapPitch(surfaceWidth, pixelMode),
		PixelMode:    pixelMode,
	}, true
}

// PrepareBitmapForOutline returns a translated copy of outline plus a bitmap
// whose surface matches FreeType render-mode dimensions. The copied outline is
// translated into bitmap coordinates with Y pointing down.
func PrepareBitmapForOutline(outline api.Outline, pointCount int, mode api.RenderMode) (*Outline, *Bitmap, BitmapMetrics, bool) {
	metrics, ok := OutlineBitmapMetrics(outline, pointCount, mode)
	if !ok {
		return nil, nil, BitmapMetrics{}, false
	}
	bitmap := NewBitmapWithPixelMode(metrics.SurfaceWidth, metrics.SurfaceRows, metrics.PixelMode)
	bitmap.Left = metrics.Left
	bitmap.Top = metrics.Top
	if metrics.Width == 0 || metrics.Rows == 0 {
		return nil, bitmap, metrics, true
	}
	renderOutline := translatedBitmapOutline(outline, int32(metrics.Left)<<6, int32(metrics.Top)<<6)
	return renderOutline, bitmap, metrics, true
}

func translatedBitmapOutline(outline api.Outline, minX26Dot6, maxY26Dot6 int32) *Outline {
	points := outline.GetPoints()
	renderOutline := &Outline{
		Points:   make([]api.Vector, len(points)),
		Tags:     append([]byte{}, outline.GetTags()...),
		Contours: append([]int{}, outline.GetContours()...),
	}
	for i, p := range points {
		renderOutline.Points[i] = api.Vector{
			X: p.X - minX26Dot6,
			Y: maxY26Dot6 - p.Y,
		}
	}
	return renderOutline
}

func outlineBounds26Dot6(outline api.Outline, pointCount int) (minX, minY, maxX, maxY int32, ok bool) {
	if outline == nil {
		return 0, 0, 0, 0, false
	}
	points := outline.GetPoints()
	tags := outline.GetTags()
	if pointCount < 0 {
		pointCount = len(points)
	}
	if pointCount > len(points) {
		pointCount = len(points)
	}
	if pointCount > len(tags) {
		pointCount = len(tags)
	}
	if pointCount <= 0 {
		return 0, 0, 0, 0, false
	}
	minX = points[0].X
	minY = points[0].Y
	maxX = minX
	maxY = minY
	for i := 1; i < pointCount; i++ {
		p := points[i]
		if p.X < minX {
			minX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return minX, minY, maxX, maxY, true
}

type bitmapPixelBox struct {
	xMin, yMin int32
	xMax, yMax int32
}

func presetBitmapBox26Dot6(minX, minY, maxX, maxY int32, mode api.RenderMode) bitmapPixelBox {
	xMin, xMinFrac := split26Dot6(minX)
	yMin, yMinFrac := split26Dot6(minY)
	xMax, xMaxFrac := split26Dot6(maxX)
	yMax, yMaxFrac := split26Dot6(maxY)

	switch mode {
	case api.RenderModeMono:
		xMin, xMax = adjustMonoAxis(xMin, xMinFrac, xMax, xMaxFrac)
		yMin, yMax = adjustMonoAxis(yMin, yMinFrac, yMax, yMaxFrac)
	case api.RenderModeLCD:
		xMinFrac -= lcdFilterPadding26Dot6
		xMaxFrac += lcdFilterPadding26Dot6
		xMin, xMax = adjustCeilAxis(xMin, xMinFrac, xMax, xMaxFrac)
		yMin, yMax = adjustCeilAxis(yMin, yMinFrac, yMax, yMaxFrac)
	case api.RenderModeLCDV:
		yMinFrac -= lcdFilterPadding26Dot6
		yMaxFrac += lcdFilterPadding26Dot6
		xMin, xMax = adjustCeilAxis(xMin, xMinFrac, xMax, xMaxFrac)
		yMin, yMax = adjustCeilAxis(yMin, yMinFrac, yMax, yMaxFrac)
	default:
		xMin, xMax = adjustCeilAxis(xMin, xMinFrac, xMax, xMaxFrac)
		yMin, yMax = adjustCeilAxis(yMin, yMinFrac, yMax, yMaxFrac)
	}

	return bitmapPixelBox{xMin: xMin, yMin: yMin, xMax: xMax, yMax: yMax}
}

func split26Dot6(v int32) (whole, frac int32) {
	return v >> 6, v & 63
}

func adjustCeilAxis(minWhole, minFrac, maxWhole, maxFrac int32) (int32, int32) {
	minWhole += minFrac >> 6
	maxWhole += (maxFrac + 63) >> 6
	return minWhole, maxWhole
}

func adjustMonoAxis(minWhole, minFrac, maxWhole, maxFrac int32) (int32, int32) {
	minWhole += (minFrac + 31) >> 6
	maxWhole += (maxFrac + 32) >> 6
	if minWhole == maxWhole {
		if monoCollapsedBoxExtendsMin(minFrac, maxFrac) {
			minWhole--
		} else {
			maxWhole++
		}
	}
	return minWhole, maxWhole
}

func monoCollapsedBoxExtendsMin(minFrac, maxFrac int32) bool {
	minRemainder := ((minFrac + 31) & 63) - 31
	maxRemainder := ((maxFrac + 32) & 63) - 32
	return minRemainder+maxRemainder < 0
}

func clampBitmapDimension(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func padBitmapDimension(v, alignment int) int {
	if v <= 0 {
		return 0
	}
	return (v + alignment - 1) &^ (alignment - 1)
}

func bitmapBufferLen(pitch, rows int) int {
	if pitch <= 0 || rows <= 0 {
		return 0
	}
	return pitch * rows
}
