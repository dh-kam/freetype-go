package raster

import (
	"github.com/dh-kam/freetype-go/api"
	"testing"
)

type mockOutline struct {
	points   []api.Vector
	tags     []byte
	contours []int
}

func (o *mockOutline) GetPoints() []api.Vector    { return o.points }
func (o *mockOutline) GetTags() []byte            { return o.tags }
func (o *mockOutline) GetContours() []int         { return o.contours }
func (o *mockOutline) Scale(xScale, yScale int32) {}
func (o *mockOutline) Translate(dx, dy int32)     {}
func (o *mockOutline) Transform(m *api.Matrix)    {}

type mockBitmap struct {
	rows   int
	width  int
	pitch  int
	buffer []byte
	mode   uint8
}

func (b *mockBitmap) GetRows() int         { return b.rows }
func (b *mockBitmap) GetWidth() int        { return b.width }
func (b *mockBitmap) GetPitch() int        { return b.pitch }
func (b *mockBitmap) GetBuffer() []byte    { return b.buffer }
func (b *mockBitmap) GetPixelMode() uint8  { return b.mode }
func (b *mockBitmap) SetPixelMode(m uint8) { b.mode = m }

func TestSmoothRasterizer(t *testing.T) {
	rasterizer := NewSmoothRasterizer()

	// A simple square outline: (10,10) to (20,10) to (20,20) to (10,20)
	outline := &mockOutline{
		points: []api.Vector{
			{X: 10 * 64, Y: 10 * 64},
			{X: 20 * 64, Y: 10 * 64},
			{X: 20 * 64, Y: 20 * 64},
			{X: 10 * 64, Y: 20 * 64},
		},
		tags:     []byte{1, 1, 1, 1},
		contours: []int{3},
	}

	bitmap := &mockBitmap{
		rows:   32,
		width:  32,
		pitch:  32,
		buffer: make([]byte, 32*32),
	}

	err := rasterizer.Render(outline, bitmap)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	// Check a pixel inside the square (15, 15)
	val := bitmap.buffer[15*32+15]
	if val != 255 {
		t.Errorf("Pixel at (15,15) should be 255, got %d", val)
	}

	// Check a pixel outside (5, 5)
	val = bitmap.buffer[5*32+5]
	if val != 0 {
		t.Errorf("Pixel at (5,5) should be 0, got %d", val)
	}
}

func TestSmoothRasterizerAntiAliasing(t *testing.T) {
	rasterizer := NewSmoothRasterizer()

	// A rectangle from x=10.5 to x=11.5, y=10 to y=20
	outline := &mockOutline{
		points: []api.Vector{
			{X: 10*64 + 32, Y: 10 * 64},
			{X: 11*64 + 32, Y: 10 * 64},
			{X: 11*64 + 32, Y: 20 * 64},
			{X: 10*64 + 32, Y: 20 * 64},
		},
		tags:     []byte{1, 1, 1, 1},
		contours: []int{3},
	}

	bitmap := &mockBitmap{
		rows:   32,
		width:  32,
		pitch:  32,
		buffer: make([]byte, 32*32),
	}

	rasterizer.Render(outline, bitmap)

	// Pixel at x=10 should be roughly 128 (half-filled)
	val10 := bitmap.buffer[15*32+10]
	val11 := bitmap.buffer[15*32+11]
	t.Logf("val10 = %d, val11 = %d", val10, val11)

	if val10 < 120 || val10 > 135 {
		t.Errorf("Pixel at x=10 should be ~128, got %d", val10)
	}

	// Pixel at x=11 should be roughly 128 (half-filled)
	if val11 < 120 || val11 > 135 {
		t.Errorf("Pixel at x=11 should be ~128, got %d", val11)
	}
}

func TestSmoothRasterizerWinding(t *testing.T) {
	rasterizer := NewSmoothRasterizer()

	// Two squares: one inside another with opposite winding should create a hole.
	// Outer: (10,10) to (30,10) to (30,30) to (10,30) - Clockwise
	// Inner: (15,15) to (15,25) to (25,25) to (25,15) - Counter-clockwise
	outline := &mockOutline{
		points: []api.Vector{
			// Outer
			{X: 10 * 64, Y: 10 * 64},
			{X: 30 * 64, Y: 10 * 64},
			{X: 30 * 64, Y: 30 * 64},
			{X: 10 * 64, Y: 30 * 64},
			// Inner
			{X: 15 * 64, Y: 15 * 64},
			{X: 15 * 64, Y: 25 * 64},
			{X: 25 * 64, Y: 25 * 64},
			{X: 25 * 64, Y: 15 * 64},
		},
		tags:     []byte{1, 1, 1, 1, 1, 1, 1, 1},
		contours: []int{3, 7},
	}

	bitmap := &mockBitmap{
		rows:   40,
		width:  40,
		pitch:  40,
		buffer: make([]byte, 40*40),
	}

	rasterizer.Render(outline, bitmap)

	// Pixel at (12, 12) should be filled
	val12 := bitmap.buffer[12*40+12]
	if val12 != 255 {
		t.Errorf("Pixel at (12,12) should be 255, got %d", val12)
	}

	// Pixel at (20, 20) should be 0 (the hole)
	val20 := bitmap.buffer[20*40+20]
	if val20 != 0 {
		t.Errorf("Pixel at (20,20) should be 0 (hole), got %d", val20)
	}
}
