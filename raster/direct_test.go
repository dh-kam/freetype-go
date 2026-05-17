package raster

import (
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

func TestDirectRendering(t *testing.T) {
	sr := NewSmoothRasterizer()

	// Create a simple triangle outline
	outline := &mockOutline{
		points: []api.Vector{
			{X: 10 << 6, Y: 10 << 6},
			{X: 50 << 6, Y: 10 << 6},
			{X: 30 << 6, Y: 40 << 6},
		},
		tags:     []byte{1, 1, 1},
		contours: []int{2},
	}

	bitmap := core.NewBitmap(64, 64)
	bitmap.SetPixelMode(api.MODE_GRAY)

	spanCount := 0
	sr.GraySpans = func(y int, spans []Span) {
		spanCount += len(spans)
		for _, span := range spans {
			if span.Len == 0 {
				t.Errorf("Span at y=%d has zero length", y)
			}
			if span.Coverage == 0 {
				t.Errorf("Span at y=%d has zero coverage", y)
			}
		}
	}

	err := sr.Render(outline, bitmap)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if spanCount == 0 {
		t.Errorf("No spans were rendered in direct mode")
	}
}
