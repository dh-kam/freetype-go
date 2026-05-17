package raster

import (
	"math"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

var benchmarkRasterByte byte

func BenchmarkSmoothRasterizerScale(b *testing.B) {
	for _, tc := range []struct {
		name    string
		outline api.Outline
	}{
		{name: "points_32", outline: benchmarkPointOutline(32)},
		{name: "points_256", outline: benchmarkPointOutline(256)},
		{name: "curves_16", outline: benchmarkQuadraticOutline(16)},
		{name: "curves_64", outline: benchmarkQuadraticOutline(64)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			rasterizer := NewSmoothRasterizer()
			bitmap := core.NewBitmap(160, 160)
			bitmap.SetPixelMode(api.MODE_GRAY)
			if err := rasterizer.Render(tc.outline, bitmap); err != nil {
				b.Fatalf("warmup render failed: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := rasterizer.Render(tc.outline, bitmap); err != nil {
					b.Fatalf("Render failed: %v", err)
				}
				benchmarkRasterByte ^= bitmap.Buffer[(i*97)%len(bitmap.Buffer)]
			}
		})
	}
}

func benchmarkPointOutline(numPoints int) *core.Outline {
	points := make([]api.Vector, numPoints)
	tags := make([]byte, numPoints)
	for i := range points {
		angle := float64(i) * 2 * math.Pi / float64(numPoints)
		radius := 48.0 + 10.0*math.Sin(float64(i)*3.0)
		points[i] = benchmarkVector(80+math.Cos(angle)*radius, 80+math.Sin(angle)*radius)
		tags[i] = 1
	}
	return &core.Outline{
		Points:   points,
		Tags:     tags,
		Contours: []int{numPoints - 1},
	}
}

func benchmarkQuadraticOutline(numCurves int) *core.Outline {
	points := make([]api.Vector, 0, 1+numCurves*2)
	tags := make([]byte, 0, 1+numCurves*2)

	points = append(points, benchmarkVector(128, 80))
	tags = append(tags, 1)
	for i := 0; i < numCurves; i++ {
		startAngle := float64(i) * 2 * math.Pi / float64(numCurves)
		endAngle := float64(i+1) * 2 * math.Pi / float64(numCurves)
		ctrlAngle := (startAngle + endAngle) / 2

		points = append(points, benchmarkVector(80+math.Cos(ctrlAngle)*62, 80+math.Sin(ctrlAngle)*62))
		tags = append(tags, 0)
		points = append(points, benchmarkVector(80+math.Cos(endAngle)*48, 80+math.Sin(endAngle)*48))
		tags = append(tags, 1)
	}

	return &core.Outline{
		Points:   points,
		Tags:     tags,
		Contours: []int{len(points) - 1},
	}
}

func benchmarkVector(x, y float64) api.Vector {
	return api.Vector{
		X: int32(math.Round(x * 64)),
		Y: int32(math.Round(y * 64)),
	}
}
