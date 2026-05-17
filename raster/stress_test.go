package raster

import (
	"math/rand"
	"runtime"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

func TestUltraHighResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping ultra-high resolution test in short mode")
	}

	const size = 8192
	rasterizer := NewSmoothRasterizer()

	outline := &core.Outline{
		Points: []api.Vector{
			{X: 100 * 64, Y: 100 * 64},
			{X: 200 * 64, Y: 100 * 64},
			{X: 200 * 64, Y: 200 * 64},
			{X: 100 * 64, Y: 200 * 64},
		},
		Tags:     []byte{1, 1, 1, 1},
		Contours: []int{3},
	}

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	bitmap := core.NewBitmap(size, size)
	err := rasterizer.Render(outline, bitmap)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	runtime.ReadMemStats(&m2)
	t.Logf("Memory used for 8K bitmap and render: %v MB", (m2.Alloc-m1.Alloc)/(1024*1024))
	t.Logf("Total Alloc: %v MB", m2.Alloc/(1024*1024))
	t.Logf("Total Sys: %v MB", m2.Sys/(1024*1024))
	t.Logf("NumGC: %v", m2.NumGC)

	// Verify some pixels
	if bitmap.Buffer[150*size+150] != 255 {
		t.Errorf("Expected 255 at (150, 150), got %d", bitmap.Buffer[150*size+150])
	}
}

func TestComplexGeometry(t *testing.T) {
	const size = 1024
	const numSegments = 10000
	rasterizer := NewSmoothRasterizer()
	bitmap := core.NewBitmap(size, size)

	points := make([]api.Vector, numSegments+1)
	rng := rand.New(rand.NewSource(42))
	for i := 0; i <= numSegments; i++ {
		points[i] = api.Vector{
			X: int32(rng.Intn(size * 64)),
			Y: int32(rng.Intn(size * 64)),
		}
	}

	outline := &core.Outline{
		Points:   points,
		Tags:     make([]byte, numSegments+1),
		Contours: []int{numSegments},
	}

	err := rasterizer.Render(outline, bitmap)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	t.Logf("Rendered %d segments successfully", numSegments)
}

func TestOutOfBounds(t *testing.T) {
	const size = 256
	rasterizer := NewSmoothRasterizer()
	bitmap := core.NewBitmap(size, size)

	outline := &core.Outline{
		Points: []api.Vector{
			{X: -1000000 * 64, Y: -1000000 * 64},
			{X: 1000000 * 64, Y: -1000000 * 64},
			{X: 1000000 * 64, Y: 1000000 * 64},
			{X: -1000000 * 64, Y: 1000000 * 64},
		},
		Tags:     []byte{1, 1, 1, 1},
		Contours: []int{3},
	}

	// This should not panic and handle clamping correctly.
	err := rasterizer.Render(outline, bitmap)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	// If the entire viewport is covered by this huge square, all pixels should be 255.
	// However, depending on winding and how large lines are handled, let's just check it doesn't crash first.
	// For this huge square, it covers everything.
	allFilled := true
	for i := 0; i < size*size; i += size {
		if bitmap.Buffer[i+size/2] != 255 {
			allFilled = false
			t.Errorf("Expected pixel at (%d, %d) to be 255, got %d", size/2, i/size, bitmap.Buffer[i+size/2])
			break
		}
	}
	if allFilled {
		t.Log("Successfully rendered out-of-bounds huge square covering entire bitmap")
	}
}

func TestDegenerateCases(t *testing.T) {
	const size = 64
	rasterizer := NewSmoothRasterizer()
	bitmap := core.NewBitmap(size, size)

	// 1. Zero length lines
	// 2. Horizontal/Vertical lines exactly on pixel boundaries
	// 3. Overlapping contours
	outline := &core.Outline{
		Points: []api.Vector{
			// Zero length contour
			{X: 10 * 64, Y: 10 * 64},
			{X: 10 * 64, Y: 10 * 64},
			// Pixel boundary aligned square
			{X: 20 * 64, Y: 20 * 64},
			{X: 30 * 64, Y: 20 * 64},
			{X: 30 * 64, Y: 30 * 64},
			{X: 20 * 64, Y: 30 * 64},
			// Overlapping contour (another square on top)
			{X: 25 * 64, Y: 25 * 64},
			{X: 35 * 64, Y: 25 * 64},
			{X: 35 * 64, Y: 35 * 64},
			{X: 25 * 64, Y: 35 * 64},
		},
		Tags:     []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		Contours: []int{1, 5, 9},
	}

	err := rasterizer.Render(outline, bitmap)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	t.Log("Rendered degenerate cases successfully")
}

func BenchmarkSmoothRasterizer(b *testing.B) {
	const size = 1024
	const numPoints = 1000
	rasterizer := NewSmoothRasterizer()
	bitmap := core.NewBitmap(size, size)

	points := make([]api.Vector, numPoints)
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < numPoints; i++ {
		points[i] = api.Vector{
			X: int32(rng.Intn(size * 64)),
			Y: int32(rng.Intn(size * 64)),
		}
	}

	outline := &core.Outline{
		Points:   points,
		Tags:     make([]byte, numPoints),
		Contours: []int{numPoints - 1},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rasterizer.Render(outline, bitmap)
	}
}
