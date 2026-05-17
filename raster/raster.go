package raster

import (
	"slices"
	"sync"

	"github.com/dh-kam/freetype-go/api"
)

var tcellPool = sync.Pool{
	New: func() interface{} {
		return make([]TCell, 2048)
	},
}

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
	rows := bitmap.GetRows()
	width := bitmap.GetWidth()
	pixelMode := bitmap.GetPixelMode()

	if rows == 0 || width == 0 {
		return nil
	}

	r.worker.PixelMode = pixelMode
	if pixelMode == api.MODE_LCD {
		r.worker.XScale = 3
	} else {
		r.worker.XScale = 1
	}

	r.worker.GraySpans = r.GraySpans

	// Initialize worker bounds
	r.worker.MinEx = 0
	r.worker.MinEy = 0
	r.worker.MaxEx = width * r.worker.XScale
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
			segs = append(segs, segmentPoint{
				X:  contourPoints[i].X * xScale,
				Y:  contourPoints[i].Y,
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
		r.worker.X = int(r.worker.x >> 6)
		r.worker.Y = int(r.worker.y >> 6)
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
	const segments = 8
	startX := w.x
	startY := w.y

	for i := int64(1); i <= segments; i++ {
		t := i
		invT := segments - i

		term0 := invT * invT
		term1 := 2 * t * invT
		term2 := t * t
		divisor := int64(segments * segments)

		currX := int32((int64(startX)*term0 + int64(ctrlX)*term1 + int64(toX)*term2) / divisor)
		currY := int32((int64(startY)*term0 + int64(ctrlY)*term1 + int64(toY)*term2) / divisor)

		w.renderLine(currX, currY)
	}
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
		x = w.MaxEx
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
	ex1 := int(x1 >> 6)
	ex2 := int(x2 >> 6)
	fx1 := x1 - int32(ex1<<6)
	fx2 := x2 - int32(ex2<<6)

	w.setCell(ex1, ey)

	if ex1 == ex2 {
		w.Area += int((y2 - y1) * (fx1 + fx2))
		w.Cover += int(y2 - y1)
		return
	}

	dx := x2 - x1
	dy := y2 - y1

	incr := 1
	if dx < 0 {
		incr = -1
	}

	p := x1
	firstY := y1

	for ex := ex1; ex != ex2; ex += incr {
		var nextX int32
		if incr > 0 {
			nextX = int32((ex + 1) << 6)
		} else {
			nextX = int32(ex << 6)
		}

		nextY := y1 + int32((int64(nextX-x1)*int64(dy))/int64(dx))

		fx_start := p - int32(ex<<6)
		fx_end := nextX - int32(ex<<6)

		w.Area += int((nextY - firstY) * (fx_start + fx_end))
		w.Cover += int(nextY - firstY)

		p = nextX
		firstY = nextY
		w.setCell(ex+incr, ey)
	}

	fx_start := p - int32(ex2<<6)
	fx_end := fx2
	w.Area += int((y2 - firstY) * (fx_start + fx_end))
	w.Cover += int(y2 - firstY)
}

func (w *TWorker) renderLine(toX, toY int32) {
	ey1 := int(w.y >> 6)
	ey2 := int(toY >> 6)

	if ey1 == ey2 {
		w.renderScanline(ey1, w.x, w.y, toX, toY)
		w.x = toX
		w.y = toY
		return
	}

	dx := toX - w.x
	dy := toY - w.y

	incr := 1
	if dy < 0 {
		incr = -1
	}

	p := w.y
	firstX := w.x

	for ey := ey1; ey != ey2; ey += incr {
		var nextY int32
		if incr > 0 {
			nextY = int32((ey + 1) << 6)
		} else {
			nextY = int32(ey << 6)
		}

		nextX := w.x + int32((int64(nextY-w.y)*int64(dx))/int64(dy))

		w.renderScanline(ey, firstX, p, nextX, nextY)

		p = nextY
		firstX = nextX
	}

	w.renderScanline(ey2, firstX, p, toX, toY)
	w.x = toX
	w.y = toY
}

func (w *TWorker) sweep(bitmap api.Bitmap) {
	if w.PixelMode == api.MODE_LCD {
		w.sweepLCD(bitmap)
		return
	}

	width := bitmap.GetWidth()
	rows := bitmap.GetRows()
	buffer := bitmap.GetBuffer()
	pitch := bitmap.GetPitch()

	if rows == 0 || width == 0 || len(buffer) == 0 {
		return
	}

	var spans []Span
	if w.GraySpans != nil {
		spans = make([]Span, 0, 16)
	}

	cellIdx := 0
	for y := 0; y < rows; y++ {
		// BCE hint for this scanline
		line := buffer[y*pitch : (y*pitch)+width]
		_ = line[width-1]

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
				val := w.calculateGray(cover << 7)
				if w.PixelMode == api.MODE_MONO {
					if val >= 128 {
						if w.GraySpans != nil {
							spans = append(spans, Span{X: int16(x), Len: uint16(cell.X - x), Coverage: 255})
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
			area := (cover << 7) - cell.Area
			val := w.calculateGray(area)

			if w.PixelMode == api.MODE_MONO {
				if val >= 128 && cell.X >= 0 && cell.X < width {
					if w.GraySpans != nil {
						spans = append(spans, Span{X: int16(cell.X), Len: 1, Coverage: 255})
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
			val := w.calculateGray(cover << 7)
			if w.PixelMode == api.MODE_MONO {
				if val >= 128 {
					if w.GraySpans != nil {
						spans = append(spans, Span{X: int16(x), Len: uint16(width - x), Coverage: 255})
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
			w.GraySpans(y, spans)
		}
	}
}
func (w *TWorker) sweepLCD(bitmap api.Bitmap) {
	width := bitmap.GetWidth()
	rows := bitmap.GetRows()
	buffer := bitmap.GetBuffer()
	pitch := bitmap.GetPitch()

	var spans []Span
	if w.GraySpans != nil {
		spans = make([]Span, 0, 16)
	}

	cellIdx := 0
	for y := 0; y < rows; y++ {
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
				val := w.calculateGray(cover << 7)
				if val != 0 {
					w.fillSpan(w.LCDLine, 2, x, cell.X, val)
				}
			}

			cover += cell.Cover
			area := (cover << 7) - cell.Area
			val := w.calculateGray(area)

			if val != 0 && cell.X >= 0 && cell.X < w.MaxEx {
				w.LCDLine[cell.X+2] = val
			}

			x = cell.X + 1
		}

		if x < w.MaxEx && cover != 0 {
			val := w.calculateGray(cover << 7)
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
				w.GraySpans(y, spans)
			}
		} else {
			for i := 0; i < width; i++ {
				buffer[y*pitch+i] = w.filterLCD(i)
			}
		}
	}
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
	return byte((sum + 128) >> 8)
}

func (w *TWorker) calculateGray(area int) byte {
	if area < 0 {
		area = -area
	}
	if area > 8192 {
		area = 8192
	}
	return byte((area*255 + 4096) >> 13)
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
