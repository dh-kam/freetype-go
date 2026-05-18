package raster

import (
	"slices"
	"sync"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

var tcellPool = sync.Pool{
	New: func() interface{} {
		return make([]TCell, 2048)
	},
}

const (
	pixelBits = 8
	onePixel  = 1 << pixelBits
	upscale   = onePixel >> 6
)

// TCell represents a single pixel cell in the smooth rasterizer.
// Based on FreeType's TCell in ftgrays.c.
type TCell struct {
	X, Y  int
	Cover int
	Area  int
}

// TWorker encapsulates the state of the rasterization process.
type TWorker struct {
	NumCells int
	Cells    []TCell

	X, Y  int // current cell coordinates
	Area  int // current cell area (multiplied by 2)
	Cover int // current cell coverage

	x, y int32 // current point in 26.6 fixed-point

	MinEy, MaxEy int
	MinEx, MaxEx int

	PixelMode uint8
	XScale    int
	YScale    int
	FlipY     bool
	LCDLine   []byte
	LCDFilter int

	GraySpans func(y int, spans []Span)
}

// Span represents a horizontal span of pixels with the same coverage.
// Based on FreeType's FT_Span.
type Span struct {
	X        int16
	Len      uint16
	Coverage uint8
}

// SmoothRasterizer implements the api.Rasterizer interface.
type SmoothRasterizer struct {
	worker    TWorker
	GraySpans func(y int, spans []Span)
}

// NewSmoothRasterizer creates a new instance of the smooth rasterizer.
func NewSmoothRasterizer() *SmoothRasterizer {
	return &SmoothRasterizer{}
}

// SetLCDFilter implements the api.Rasterizer interface.
func (r *SmoothRasterizer) SetLCDFilter(filterType int) {
	r.worker.LCDFilter = filterType
}

// Render implements the api.Rasterizer interface.
func (r *SmoothRasterizer) Render(outline api.Outline, bitmap api.Bitmap) error {
	if outline == nil || bitmap == nil {
		return nil
	}

	rows := bitmap.GetRows()
	width := bitmap.GetWidth()
	pixelMode := bitmap.GetPixelMode()

	if rows == 0 || width == 0 {
		return nil
	}

	r.worker.PixelMode = pixelMode
	r.worker.XScale = 1
	r.worker.YScale = 1
	r.worker.FlipY = false
	if _, top, ok := api.GetBitmapPlacement(bitmap); ok && top != 0 && pixelMode != core.PixelModeLCDV {
		r.worker.FlipY = true
	}
	switch pixelMode {
	case api.MODE_LCD:
		r.worker.XScale = 3
	case core.PixelModeLCDV:
		r.worker.YScale = 3
	}

	r.worker.GraySpans = r.GraySpans

	// Initialize worker bounds
	r.worker.MinEx = 0
	r.worker.MinEy = 0
	r.worker.MaxEx = width
	r.worker.MaxEy = rows

	r.worker.NumCells = 0

	r.worker.Cells = tcellPool.Get().([]TCell)
	defer func() {
		tcellPool.Put(r.worker.Cells)
		r.worker.Cells = nil
	}()

	if pixelMode == api.MODE_LCD {
		if len(r.worker.LCDLine) < r.worker.MaxEx+4 {
			r.worker.LCDLine = make([]byte, r.worker.MaxEx+4)
		}
	}

	// Decompose outline into line segments and populate cells
	err := r.decompose(outline)
	if err != nil {
		return err
	}

	// Sort cells by Y, then X
	slices.SortFunc(r.worker.Cells[:r.worker.NumCells], func(a, b TCell) int {
		if a.Y != b.Y {
			return a.Y - b.Y
		}
		return a.X - b.X
	})

	// Sweep the cells to produce the final bitmap
	r.worker.sweep(bitmap)

	return nil
}

func (r *SmoothRasterizer) decompose(outline api.Outline) error {
	points := outline.GetPoints()
	tags := outline.GetTags()
	contours := outline.GetContours()
	xScale := int32(r.worker.XScale)
	yScale := int32(r.worker.YScale)

	start := 0
	for _, end := range contours {
		if end < start || end >= len(points) || end >= len(tags) {
			continue
		}

		contourPoints := points[start : end+1]
		contourTags := tags[start : end+1]
		if len(contourPoints) < 2 {
			start = end + 1
			continue
		}

		type segmentPoint struct {
			X, Y int32
			On   bool
		}
		var segs []segmentPoint

		for i := 0; i < len(contourPoints); i++ {
			y := contourPoints[i].Y
			if r.worker.FlipY {
				y = int32((r.worker.MaxEy/r.worker.YScale)<<6) - y
			}
			segs = append(segs, segmentPoint{
				X:  contourPoints[i].X * xScale * upscale,
				Y:  y * yScale * upscale,
				On: (contourTags[i] & 1) == 1,
			})
		}

		// Handle implicit on-curve points or shift to start with an on-curve point
		if !segs[0].On && !segs[len(segs)-1].On {
			midX := (segs[0].X + segs[len(segs)-1].X) / 2
			midY := (segs[0].Y + segs[len(segs)-1].Y) / 2
			segs = append([]segmentPoint{{X: midX, Y: midY, On: true}}, segs...)
		} else if !segs[0].On && segs[len(segs)-1].On {
			last := segs[len(segs)-1]
			segs = append([]segmentPoint{last}, segs[:len(segs)-1]...)
		}

		// Close the contour
		segs = append(segs, segs[0])

		r.worker.x = segs[0].X
		r.worker.y = segs[0].Y
		r.worker.X = int(r.worker.x >> pixelBits)
		r.worker.Y = int(r.worker.y >> pixelBits)
		r.worker.Area = 0
		r.worker.Cover = 0

		for i := 1; i < len(segs); i++ {
			pt := segs[i]
			if pt.On {
				r.worker.renderLine(pt.X, pt.Y)
			} else {
				nextPt := segs[i+1]
				if nextPt.On {
					r.worker.renderQuadratic(pt.X, pt.Y, nextPt.X, nextPt.Y)
					i++
				} else {
					midX := (pt.X + nextPt.X) / 2
					midY := (pt.Y + nextPt.Y) / 2
					r.worker.renderQuadratic(pt.X, pt.Y, midX, midY)
				}
			}
		}

		// Record the last active cell
		if r.worker.Area != 0 || r.worker.Cover != 0 {
			r.worker.recordCell()
		}

		start = end + 1
	}
	return nil
}

func (w *TWorker) renderQuadratic(ctrlX, ctrlY, toX, toY int32) {
	// Match ftgrays' adaptive conic subdivision: each bisection reduces the
	// control-point deviation by 4, then forward-difference the segments.
	p0x, p0y := int64(w.x), int64(w.y)
	p1x, p1y := int64(ctrlX), int64(ctrlY)
	p2x, p2y := int64(toX), int64(toY)

	if (int(p0y>>pixelBits) >= w.MaxEy && int(p1y>>pixelBits) >= w.MaxEy && int(p2y>>pixelBits) >= w.MaxEy) ||
		(int(p0y>>pixelBits) < w.MinEy && int(p1y>>pixelBits) < w.MinEy && int(p2y>>pixelBits) < w.MinEy) {
		w.x = toX
		w.y = toY
		return
	}

	bx := p1x - p0x
	by := p1y - p0y
	ax := p2x - p1x - bx
	ay := p2y - p1y - by

	dx := absInt64(ax)
	if dy := absInt64(ay); dx < dy {
		dx = dy
	}

	if dx <= onePixel/4 {
		w.renderLine(toX, toY)
		return
	}

	shift := 16
	for {
		dx >>= 2
		shift--
		if dx <= onePixel/4 {
			break
		}
	}
	count := 0x10000 >> shift

	rx := leftShiftInt64(ax, shift+shift)
	ry := leftShiftInt64(ay, shift+shift)
	qx := leftShiftInt64(bx, shift+17) + rx
	qy := leftShiftInt64(by, shift+17) + ry
	rx *= 2
	ry *= 2

	px := leftShiftInt64(p0x, 32)
	py := leftShiftInt64(p0y, 32)
	for ; count > 0; count-- {
		px += qx
		py += qy
		qx += rx
		qy += ry
		w.renderLine(int32(px>>32), int32(py>>32))
	}
}

func leftShiftInt64(v int64, shift int) int64 {
	return int64(uint64(v) << uint(shift))
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func (w *TWorker) setCell(ex, ey int) {
	if ex != w.X || ey != w.Y {
		if w.Area != 0 || w.Cover != 0 {
			w.recordCell()
		}
		w.X = ex
		w.Y = ey
		w.Area = 0
		w.Cover = 0
	}
}

func (w *TWorker) recordCell() {
	if w.Y < w.MinEy || w.Y >= w.MaxEy {
		return
	}

	x := w.X
	// Clamp X for coverage calculation outside the viewport
	if x < w.MinEx {
		x = w.MinEx - 1
	} else if x >= w.MaxEx {
		return
	}

	// Growth check
	if w.NumCells >= len(w.Cells) {
		newCells := make([]TCell, len(w.Cells)*2)
		copy(newCells, w.Cells)
		w.Cells = newCells
	}

	// Just append
	w.Cells[w.NumCells] = TCell{
		X:     x,
		Y:     w.Y,
		Area:  w.Area,
		Cover: w.Cover,
	}
	w.NumCells++
}

func (w *TWorker) renderScanline(ey int, x1, y1, x2, y2 int32) {
	ex1 := int(x1 >> pixelBits)
	ex2 := int(x2 >> pixelBits)

	if y1 == y2 {
		w.setCell(ex2, ey)
		return
	}

	w.setCell(ex1, ey)
	fx1 := int32(int(x1) & (onePixel - 1))
	fx2 := int32(int(x2) & (onePixel - 1))

	if ex1 != ex2 {
		dx := int64(x2 - x1)
		dy := y2 - y1

		var p int64
		var first int32
		incr := 1
		if dx > 0 {
			p = int64(onePixel-fx1) * int64(dy)
			first = onePixel
		} else {
			p = int64(fx1) * int64(dy)
			first = 0
			incr = -1
			dx = -dx
		}

		delta, mod := divModFloor(p, dx)
		w.integrate(delta, fx1+first)
		y1 += delta
		ex1 += incr
		w.setCell(ex1, ey)

		if ex1 != ex2 {
			lift, rem := divModFloor(int64(onePixel)*int64(dy), dx)
			for ex1 != ex2 {
				delta = lift
				mod += rem
				if mod >= int32(dx) {
					mod -= int32(dx)
					delta++
				}

				w.integrate(delta, onePixel)
				y1 += delta
				ex1 += incr
				w.setCell(ex1, ey)
			}
		}

		fx1 = onePixel - first
	}

	w.integrate(y2-y1, fx1+fx2)
}

func (w *TWorker) renderLine(toX, toY int32) {
	ey1 := int(w.y >> pixelBits)
	ey2 := int(toY >> pixelBits)

	if (ey1 >= w.MaxEy && ey2 >= w.MaxEy) || (ey1 < w.MinEy && ey2 < w.MinEy) {
		w.x = toX
		w.y = toY
		return
	}

	fy1 := int32(int(w.y) & (onePixel - 1))
	ex1 := int(w.x >> pixelBits)
	ex2 := int(toX >> pixelBits)
	fx1 := int32(int(w.x) & (onePixel - 1))

	dx := int64(toX - w.x)
	dy := int64(toY - w.y)

	if ex1 == ex2 && ey1 == ey2 {
		// The final integration below handles a segment that stays in one cell.
	} else if dy == 0 {
		w.setCell(ex2, ey2)
		w.x = toX
		w.y = toY
		return
	} else if dx == 0 {
		if dy > 0 {
			for {
				fy2 := int32(onePixel)
				w.integrate(fy2-fy1, fx1*2)
				fy1 = 0
				ey1++
				w.setCell(ex1, ey1)
				if ey1 == ey2 {
					break
				}
			}
		} else {
			for {
				fy2 := int32(0)
				w.integrate(fy2-fy1, fx1*2)
				fy1 = onePixel
				ey1--
				w.setCell(ex1, ey1)
				if ey1 == ey2 {
					break
				}
			}
		}
	} else {
		prod := dx*int64(fy1) - dy*int64(fx1)
		dxReciprocal := int64(0)
		if ex1 != ex2 {
			dxReciprocal = 0xFFFFFFFF / dx
		}
		dyReciprocal := int64(0)
		if ey1 != ey2 {
			dyReciprocal = 0xFFFFFFFF / dy
		}
		for ex1 != ex2 || ey1 != ey2 {
			switch {
			case prod-dx*onePixel > 0 && prod <= 0:
				fx2 := int32(0)
				fy2 := udiv(-prod, -dxReciprocal)
				prod -= dy * onePixel
				w.integrate(fy2-fy1, fx1+fx2)
				fx1 = onePixel
				fy1 = fy2
				ex1--
			case prod-dx*onePixel+dy*onePixel > 0 && prod-dx*onePixel <= 0:
				prod -= dx * onePixel
				fx2 := udiv(-prod, dyReciprocal)
				fy2 := int32(onePixel)
				w.integrate(fy2-fy1, fx1+fx2)
				fx1 = fx2
				fy1 = 0
				ey1++
			case prod+dy*onePixel >= 0 && prod-dx*onePixel+dy*onePixel <= 0:
				prod += dy * onePixel
				fx2 := int32(onePixel)
				fy2 := udiv(prod, dxReciprocal)
				w.integrate(fy2-fy1, fx1+fx2)
				fx1 = 0
				fy1 = fy2
				ex1++
			default:
				fx2 := udiv(prod, -dyReciprocal)
				fy2 := int32(0)
				prod += dx * onePixel
				w.integrate(fy2-fy1, fx1+fx2)
				fx1 = fx2
				fy1 = onePixel
				ey1--
			}
			w.setCell(ex1, ey1)
		}
	}

	fx2 := int32(int(toX) & (onePixel - 1))
	fy2 := int32(int(toY) & (onePixel - 1))
	w.integrate(fy2-fy1, fx1+fx2)
	w.x = toX
	w.y = toY
}

func (w *TWorker) integrate(delta, scale int32) {
	w.Cover += int(delta)
	w.Area += int(delta) * int(scale)
}

func divModFloor(dividend, divisor int64) (int32, int32) {
	quotient := dividend / divisor
	remainder := dividend % divisor
	if remainder < 0 {
		quotient--
		remainder += divisor
	}
	return int32(quotient), int32(remainder)
}

func udiv(dividend, divisor int64) int32 {
	return int32((uint64(dividend) * uint64(divisor)) >> 32)
}

func (w *TWorker) sweep(bitmap api.Bitmap) {
	switch w.PixelMode {
	case api.MODE_LCD:
		w.sweepLCD(bitmap)
		return
	case core.PixelModeLCDV:
		w.sweepLCDV(bitmap)
		return
	}

	width := bitmap.GetWidth()
	rows := bitmap.GetRows()
	buffer := bitmap.GetBuffer()
	pitch := bitmap.GetPitch()
	packedMono := usePackedMonoBitmap(bitmap, width, pitch)

	if rows == 0 || width == 0 || len(buffer) == 0 {
		return
	}
	lineBytes := width
	if packedMono {
		lineBytes = pitch
	}
	if pitch <= 0 || pitch < lineBytes || lineBytes <= 0 || len(buffer) < (rows-1)*pitch+lineBytes {
		return
	}

	var spans []Span
	if w.GraySpans != nil {
		spans = make([]Span, 0, 16)
	}

	cellIdx := 0
	for y := 0; y < rows; y++ {
		outputY := w.outputY(y, rows)
		// BCE hint for this scanline
		line := buffer[outputY*pitch : (outputY*pitch)+lineBytes]
		_ = line[lineBytes-1]

		if w.GraySpans != nil {
			spans = spans[:0]
		}
		cover := 0
		x := 0

		for cellIdx < w.NumCells && w.Cells[cellIdx].Y == y {
			cell := w.Cells[cellIdx]
			// Merge cells with same X
			for cellIdx+1 < w.NumCells && w.Cells[cellIdx+1].Y == y && w.Cells[cellIdx+1].X == cell.X {
				cellIdx++
				cell.Area += w.Cells[cellIdx].Area
				cell.Cover += w.Cells[cellIdx].Cover
			}
			cellIdx++

			if cell.X > x && cover != 0 {
				val := w.calculateGray(cover << (pixelBits + 1))
				if w.PixelMode == api.MODE_MONO {
					if val >= 128 {
						if w.GraySpans != nil {
							spans = append(spans, Span{X: int16(x), Len: uint16(cell.X - x), Coverage: 255})
						} else if packedMono {
							core.FillMonoSpan(line, pitch, 0, x, cell.X)
						} else {
							w.fillSpan(line, 0, x, cell.X, 255)
						}
					}
				} else if val != 0 {
					if w.GraySpans != nil {
						spans = append(spans, Span{X: int16(x), Len: uint16(cell.X - x), Coverage: val})
					} else {
						w.fillSpan(line, 0, x, cell.X, val)
					}
				}
			}

			cover += cell.Cover
			area := (cover << (pixelBits + 1)) - cell.Area
			val := w.calculateGray(area)

			if w.PixelMode == api.MODE_MONO {
				if val >= 128 && cell.X >= 0 && cell.X < width {
					if w.GraySpans != nil {
						spans = append(spans, Span{X: int16(cell.X), Len: 1, Coverage: 255})
					} else if packedMono {
						core.SetMonoPixel(line, pitch, cell.X, 0, true)
					} else {
						line[cell.X] = 255
					}
				}
			} else if val != 0 && cell.X >= 0 && cell.X < width {
				if w.GraySpans != nil {
					spans = append(spans, Span{X: int16(cell.X), Len: 1, Coverage: val})
				} else {
					line[cell.X] = val
				}
			}

			x = cell.X + 1
		}

		if x < width && cover != 0 {
			val := w.calculateGray(cover << (pixelBits + 1))
			if w.PixelMode == api.MODE_MONO {
				if val >= 128 {
					if w.GraySpans != nil {
						spans = append(spans, Span{X: int16(x), Len: uint16(width - x), Coverage: 255})
					} else if packedMono {
						core.FillMonoSpan(line, pitch, 0, x, width)
					} else {
						w.fillSpan(line, 0, x, width, 255)
					}
				}
			} else if val != 0 {
				if w.GraySpans != nil {
					spans = append(spans, Span{X: int16(x), Len: uint16(width - x), Coverage: val})
				} else {
					w.fillSpan(line, 0, x, width, val)
				}
			}
		}

		if w.GraySpans != nil && len(spans) > 0 {
			w.GraySpans(outputY, spans)
		}
	}
}

type packedMonoBitmap interface {
	IsPackedMono() bool
}

func usePackedMonoBitmap(bitmap api.Bitmap, width, pitch int) bool {
	if bitmap.GetPixelMode() != api.MODE_MONO || width <= 0 || pitch <= 0 {
		return false
	}
	if packed, ok := bitmap.(packedMonoBitmap); ok {
		return packed.IsPackedMono()
	}
	return pitch == core.BitmapPitch(width, api.MODE_MONO) && pitch < width
}

func (w *TWorker) sweepLCD(bitmap api.Bitmap) {
	width := bitmap.GetWidth()
	rows := bitmap.GetRows()
	buffer := bitmap.GetBuffer()
	pitch := bitmap.GetPitch()
	if rows == 0 || width == 0 || pitch < width || len(buffer) < (rows-1)*pitch+width {
		return
	}

	var spans []Span
	if w.GraySpans != nil {
		spans = make([]Span, 0, 16)
	}

	cellIdx := 0
	for y := 0; y < rows; y++ {
		outputY := w.outputY(y, rows)
		// BCE hint for LCDLine
		_ = w.LCDLine[width+3]

		// Clear LCDLine
		for i := 0; i < width+4; i++ {
			w.LCDLine[i] = 0
		}

		cover := 0
		x := 0

		for cellIdx < w.NumCells && w.Cells[cellIdx].Y == y {
			cell := w.Cells[cellIdx]
			// Merge cells with same X
			for cellIdx+1 < w.NumCells && w.Cells[cellIdx+1].Y == y && w.Cells[cellIdx+1].X == cell.X {
				cellIdx++
				cell.Area += w.Cells[cellIdx].Area
				cell.Cover += w.Cells[cellIdx].Cover
			}
			cellIdx++

			if cell.X > x && cover != 0 {
				val := w.calculateGray(cover << (pixelBits + 1))
				if val != 0 {
					w.fillSpan(w.LCDLine, 2, x, cell.X, val)
				}
			}

			cover += cell.Cover
			area := (cover << (pixelBits + 1)) - cell.Area
			val := w.calculateGray(area)

			if val != 0 && cell.X >= 0 && cell.X < w.MaxEx {
				w.LCDLine[cell.X+2] = val
			}

			x = cell.X + 1
		}

		if x < w.MaxEx && cover != 0 {
			val := w.calculateGray(cover << (pixelBits + 1))
			if val != 0 {
				w.fillSpan(w.LCDLine, 2, x, w.MaxEx, val)
			}
		}

		if w.GraySpans != nil {
			spans = spans[:0]
			var currentSpan *Span
			for i := 0; i < width; i++ {
				val := w.filterLCD(i)
				if val != 0 {
					if currentSpan != nil && currentSpan.Coverage == val && currentSpan.X+int16(currentSpan.Len) == int16(i) {
						currentSpan.Len++
					} else {
						spans = append(spans, Span{X: int16(i), Len: 1, Coverage: val})
						currentSpan = &spans[len(spans)-1]
					}
				} else {
					currentSpan = nil
				}
			}
			if len(spans) > 0 {
				w.GraySpans(outputY, spans)
			}
		} else {
			for i := 0; i < width; i++ {
				buffer[outputY*pitch+i] = w.filterLCD(i)
			}
		}
	}
}

func (w *TWorker) sweepLCDV(bitmap api.Bitmap) {
	width := bitmap.GetWidth()
	rows := bitmap.GetRows()
	buffer := bitmap.GetBuffer()
	pitch := bitmap.GetPitch()
	if rows == 0 || width == 0 || pitch < width || len(buffer) < (rows-1)*pitch+width {
		return
	}

	surface := w.renderGraySurface(width, rows)

	var spans []Span
	if w.GraySpans != nil {
		spans = make([]Span, 0, 16)
	}

	for y := 0; y < rows; y++ {
		outputY := w.outputY(y, rows)
		if w.GraySpans != nil {
			spans = spans[:0]
			var currentSpan *Span
			for x := 0; x < width; x++ {
				val := w.filterLCDV(surface, width, rows, x, y)
				if val != 0 {
					if currentSpan != nil && currentSpan.Coverage == val && currentSpan.X+int16(currentSpan.Len) == int16(x) {
						currentSpan.Len++
					} else {
						spans = append(spans, Span{X: int16(x), Len: 1, Coverage: val})
						currentSpan = &spans[len(spans)-1]
					}
				} else {
					currentSpan = nil
				}
			}
			if len(spans) > 0 {
				w.GraySpans(outputY, spans)
			}
			continue
		}

		line := buffer[outputY*pitch : outputY*pitch+width]
		_ = line[width-1]
		for x := 0; x < width; x++ {
			line[x] = w.filterLCDV(surface, width, rows, x, y)
		}
	}
}

func (w *TWorker) renderGraySurface(width, rows int) []byte {
	surface := make([]byte, width*rows)
	cellIdx := 0
	for y := 0; y < rows; y++ {
		outputY := w.outputY(y, rows)
		line := surface[outputY*width : (outputY+1)*width]
		cover := 0
		x := 0

		for cellIdx < w.NumCells && w.Cells[cellIdx].Y == y {
			cell := w.Cells[cellIdx]
			for cellIdx+1 < w.NumCells && w.Cells[cellIdx+1].Y == y && w.Cells[cellIdx+1].X == cell.X {
				cellIdx++
				cell.Area += w.Cells[cellIdx].Area
				cell.Cover += w.Cells[cellIdx].Cover
			}
			cellIdx++

			if cell.X > x && cover != 0 {
				val := w.calculateGray(cover << (pixelBits + 1))
				if val != 0 {
					w.fillSpan(line, 0, x, cell.X, val)
				}
			}

			cover += cell.Cover
			area := (cover << (pixelBits + 1)) - cell.Area
			val := w.calculateGray(area)

			if val != 0 && cell.X >= 0 && cell.X < width {
				line[cell.X] = val
			}

			x = cell.X + 1
		}

		if x < width && cover != 0 {
			val := w.calculateGray(cover << (pixelBits + 1))
			if val != 0 {
				w.fillSpan(line, 0, x, width, val)
			}
		}
	}
	return surface
}

func (w *TWorker) outputY(y, rows int) int {
	if w.FlipY {
		return rows - 1 - y
	}
	return y
}

func (w *TWorker) filterLCDV(surface []byte, width, rows, x, y int) byte {
	var sum uint32
	switch w.LCDFilter {
	case api.LCD_FILTER_LIGHT:
		sum = w.lcdvSample(surface, width, rows, x, y-1)*85 +
			w.lcdvSample(surface, width, rows, x, y)*86 +
			w.lcdvSample(surface, width, rows, x, y+1)*85
	case api.LCD_FILTER_LEGACY:
		sum = w.lcdvSample(surface, width, rows, x, y) * 255
	case api.LCD_FILTER_NONE:
		sum = w.lcdvSample(surface, width, rows, x, y) * 256
	default: // api.LCD_FILTER_DEFAULT
		sum = w.lcdvSample(surface, width, rows, x, y-2)*8 +
			w.lcdvSample(surface, width, rows, x, y-1)*77 +
			w.lcdvSample(surface, width, rows, x, y)*86 +
			w.lcdvSample(surface, width, rows, x, y+1)*77 +
			w.lcdvSample(surface, width, rows, x, y+2)*8
	}
	if sum > 0xff00 {
		return 255
	}
	return byte(sum >> 8)
}

func (w *TWorker) lcdvSample(surface []byte, width, rows, x, y int) uint32 {
	if y < 0 || y >= rows {
		return 0
	}
	return uint32(surface[y*width+x])
}

func (w *TWorker) filterLCD(i int) byte {
	var sum uint32
	switch w.LCDFilter {
	case api.LCD_FILTER_LIGHT:
		sum = uint32(w.LCDLine[i+1])*85 +
			uint32(w.LCDLine[i+2])*86 +
			uint32(w.LCDLine[i+3])*85
	case api.LCD_FILTER_LEGACY:
		sum = uint32(w.LCDLine[i+2]) * 255
	case api.LCD_FILTER_NONE:
		sum = uint32(w.LCDLine[i+2]) * 256
	default: // api.LCD_FILTER_DEFAULT
		sum = uint32(w.LCDLine[i])*8 +
			uint32(w.LCDLine[i+1])*77 +
			uint32(w.LCDLine[i+2])*86 +
			uint32(w.LCDLine[i+3])*77 +
			uint32(w.LCDLine[i+4])*8
	}
	if sum > 0xff00 {
		return 255
	}
	return byte(sum >> 8)
}

func (w *TWorker) calculateGray(area int) byte {
	if area < 0 {
		area = -area
	}
	coverage := area >> (pixelBits*2 + 1 - 8)
	if coverage > 255 {
		coverage = 255
	}
	return byte(coverage)
}

func (w *TWorker) fillSpan(buffer []byte, offset int, x1, x2 int, val byte) {
	if x1 >= x2 {
		return
	}
	// BCE hint
	_ = buffer[offset+x2-1]
	for x := x1; x < x2; x++ {
		buffer[offset+x] = val
	}
}
