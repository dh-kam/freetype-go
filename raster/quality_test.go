package raster

import (
	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
	"testing"
)

func TestMonochromeRendering(t *testing.T) {
	rasterizer := NewSmoothRasterizer()

	// A simple triangle
	outline := &mockOutline{
		points: []api.Vector{
			{X: 10 << 6, Y: 10 << 6},
			{X: 50 << 6, Y: 10 << 6},
			{X: 30 << 6, Y: 50 << 6},
		},
		tags:     []byte{1, 1, 1},
		contours: []int{2},
	}

	bitmap := core.NewBitmap(60, 60)
	bitmap.SetPixelMode(api.MODE_MONO)

	err := rasterizer.Render(outline, bitmap)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	buffer := bitmap.GetBuffer()
	hasNonZero := false
	for i, val := range buffer {
		if val != 0 && val != 255 {
			t.Errorf("Buffer at %d contains non-mono value: %d", i, val)
		}
		if val == 255 {
			hasNonZero = true
		}
	}
	if !hasNonZero {
		t.Error("Monochrome rendering produced empty bitmap")
	}
}

func TestLCDFilters(t *testing.T) {
	// A simple vertical line, slightly offset to ensure partial subpixel coverage
	// (10.5 to 11.5) -> scaled by 3 -> (31.5 to 34.5)
	outline := &mockOutline{
		points: []api.Vector{
			{X: 10<<6 + 32, Y: 10 << 6},
			{X: 11<<6 + 32, Y: 10 << 6},
			{X: 11<<6 + 32, Y: 50 << 6},
			{X: 10<<6 + 32, Y: 50 << 6},
		},
		tags:     []byte{1, 1, 1, 1},
		contours: []int{3},
	}

	filters := []struct {
		name   string
		filter int
	}{
		{"DEFAULT", api.LCD_FILTER_DEFAULT},
		{"LIGHT", api.LCD_FILTER_LIGHT},
		{"LEGACY", api.LCD_FILTER_LEGACY},
		{"NONE", api.LCD_FILTER_NONE},
	}

	for _, tc := range filters {
		t.Run(tc.name, func(t *testing.T) {
			rasterizer := NewSmoothRasterizer()
			rasterizer.SetLCDFilter(tc.filter)

			// For LCD, we provide 3x width
			width := 20
			bitmap := core.NewBitmap(width*3, 60)
			bitmap.SetPixelMode(api.MODE_LCD)

			err := rasterizer.Render(outline, bitmap)
			if err != nil {
				t.Fatalf("Render failed: %v", err)
			}

			// Verify that we have some subpixel data (values not just 0 or 255)
			hasSubpixel := false
			for _, val := range bitmap.GetBuffer() {
				if val > 0 && val < 255 {
					hasSubpixel = true
					break
				}
			}
			if !hasSubpixel {
				t.Errorf("LCD rendering with filter %s did not produce subpixel values (only 0 or 255)", tc.name)
			}
		})
	}
}
