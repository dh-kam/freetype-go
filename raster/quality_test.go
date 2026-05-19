package raster

import (
	"encoding/hex"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
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

func TestPackedMonochromeRendering(t *testing.T) {
	rasterizer := NewSmoothRasterizer()
	outline := &mockOutline{
		points: []api.Vector{
			{X: 1 << 6, Y: 1 << 6},
			{X: 9 << 6, Y: 1 << 6},
			{X: 9 << 6, Y: 9 << 6},
			{X: 1 << 6, Y: 9 << 6},
		},
		tags:     []byte{1, 1, 1, 1},
		contours: []int{3},
	}
	bitmap := core.NewBitmapWithPixelMode(10, 10, api.MODE_MONO)

	if err := rasterizer.Render(outline, bitmap); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if bitmap.GetPitch() != 2 {
		t.Fatalf("packed mono pitch = %d, want 2", bitmap.GetPitch())
	}
	if !bitmap.MonoPixel(5, 5) {
		t.Fatal("expected packed mono interior pixel to be set")
	}
	if bitmap.MonoPixel(0, 0) || bitmap.MonoPixel(9, 5) {
		t.Fatal("expected outside packed mono pixels to remain clear")
	}
	if got := bitmap.Buffer[5*bitmap.Pitch]; got != 0x7f {
		t.Fatalf("row byte 0 = %#02x, want 0x7f for pixels 1..7", got)
	}
	if got := bitmap.Buffer[5*bitmap.Pitch+1]; got != 0x80 {
		t.Fatalf("row byte 1 = %#02x, want 0x80 for pixel 8 and clear padding", got)
	}
}

func TestGrayCoverageQuantizationAtHalfPixel(t *testing.T) {
	rasterizer := NewSmoothRasterizer()
	outline := &mockOutline{
		points: []api.Vector{
			{X: 1 << 6, Y: 1 << 6},
			{X: 1<<6 + 32, Y: 1 << 6},
			{X: 1<<6 + 32, Y: 3 << 6},
			{X: 1 << 6, Y: 3 << 6},
		},
		tags:     []byte{1, 1, 1, 1},
		contours: []int{3},
	}
	bitmap := core.NewBitmapWithPixelMode(4, 4, api.MODE_GRAY)

	if err := rasterizer.Render(outline, bitmap); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if got := bitmap.Buffer[1*bitmap.Pitch+1]; got != 128 {
		t.Fatalf("half-pixel gray coverage = %d, want 128", got)
	}
	if got := bitmap.Buffer[1*bitmap.Pitch+2]; got != 0 {
		t.Fatalf("outside gray coverage = %d, want 0", got)
	}
}

func TestGrayCoverageUsesFreeTypeShiftQuantization(t *testing.T) {
	rasterizer := NewSmoothRasterizer()
	outline := &mockOutline{
		points: []api.Vector{
			{X: 1 << 6, Y: 1 << 6},
			{X: 1<<6 + 51, Y: 1 << 6},
			{X: 1<<6 + 51, Y: 3 << 6},
			{X: 1 << 6, Y: 3 << 6},
		},
		tags:     []byte{1, 1, 1, 1},
		contours: []int{3},
	}
	bitmap := core.NewBitmapWithPixelMode(4, 4, api.MODE_GRAY)

	if err := rasterizer.Render(outline, bitmap); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if got := bitmap.Buffer[1*bitmap.Pitch+1]; got != 204 {
		t.Fatalf("51/64 gray coverage = %d, want 204", got)
	}
}

func TestPlacedGrayOutlineMatchesFreeTypeCellTieBreaking(t *testing.T) {
	outline := &core.Outline{
		Points: []api.Vector{
			{X: 547, Y: 136},
			{X: 246, Y: 136},
			{X: 199, Y: 0},
			{X: 5, Y: 0},
			{X: 282, Y: 747},
			{X: 511, Y: 747},
			{X: 788, Y: 0},
			{X: 594, Y: 0},
			{X: 294, Y: 275},
			{X: 499, Y: 275},
			{X: 397, Y: 572},
		},
		Tags:     []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		Contours: []int{7, 10},
	}
	renderOutline, bitmap, _, ok := core.PrepareBitmapForOutline(outline, -1, api.RenderModeNormal)
	if !ok {
		t.Fatal("PrepareBitmapForOutline returned !ok")
	}

	if err := NewSmoothRasterizer().Render(renderOutline, bitmap); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	const wantHex = "000000007bacacab12000000000000000ff7ffffff6a0000000000000066ffffffffc900000000000000c5fffab8ffff28000000000024ffffb34effff87000000000083ffff5b06efffe40200000001e0fff60c009effff4500000041ffffcc4c4c86ffffa4000000a0fffffffffffffffff50d000bf3fffae0e0e0e0edffff62005effffa0000000003dffffc100bdffff480000000001e2fffe21"
	gotHex := hex.EncodeToString(bitmap.Buffer)
	if gotHex != wantHex {
		t.Fatalf("placed outline buffer mismatch\n got %s\nwant %s", gotHex, wantHex)
	}
}

func TestQuadraticCurveUsesAdaptiveConicSubdivision(t *testing.T) {
	outline := &core.Outline{
		Points: []api.Vector{
			{X: 1 << 6, Y: 10 << 6},
			{X: 4 << 6, Y: -8 << 6},
			{X: 7 << 6, Y: 10 << 6},
		},
		Tags:     []byte{1, 0, 1},
		Contours: []int{2},
	}
	bitmap := core.NewBitmapWithPixelMode(9, 12, api.MODE_GRAY)

	if err := NewSmoothRasterizer().Render(outline, bitmap); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	const wantHex = "000000000000000000000000a4a400000000000035ffff35000000000092ffff910000000000ddffffdc000000001dffffffff1d00000057ffffffff560000008bffffffff8b000000bcffffffffbb000000e9ffffffffe90000000000000000000000000000000000000000"
	gotHex := hex.EncodeToString(bitmap.Buffer)
	if gotHex != wantHex {
		t.Fatalf("adaptive conic buffer mismatch\n got %s\nwant %s", gotHex, wantHex)
	}
}

func TestMonochromeThresholdAtHalfCoverage(t *testing.T) {
	rasterizer := NewSmoothRasterizer()
	bitmap := core.NewBitmapWithPixelMode(4, 4, api.MODE_MONO)

	halfPixel := &mockOutline{
		points: []api.Vector{
			{X: 1 << 6, Y: 1 << 6},
			{X: 1<<6 + 32, Y: 1 << 6},
			{X: 1<<6 + 32, Y: 3 << 6},
			{X: 1 << 6, Y: 3 << 6},
		},
		tags:     []byte{1, 1, 1, 1},
		contours: []int{3},
	}
	if err := rasterizer.Render(halfPixel, bitmap); err != nil {
		t.Fatalf("Render half-pixel outline failed: %v", err)
	}
	if !bitmap.MonoPixel(1, 1) {
		t.Fatal("50% covered mono pixel was clear, want set at FreeType threshold")
	}

	bitmap = core.NewBitmapWithPixelMode(4, 4, api.MODE_MONO)
	underHalfPixel := &mockOutline{
		points: []api.Vector{
			{X: 1 << 6, Y: 1 << 6},
			{X: 1<<6 + 31, Y: 1 << 6},
			{X: 1<<6 + 31, Y: 3 << 6},
			{X: 1 << 6, Y: 3 << 6},
		},
		tags:     []byte{1, 1, 1, 1},
		contours: []int{3},
	}
	if err := rasterizer.Render(underHalfPixel, bitmap); err != nil {
		t.Fatalf("Render under-half-pixel outline failed: %v", err)
	}
	if bitmap.MonoPixel(1, 1) {
		t.Fatal("under-50% covered mono pixel was set, want clear")
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

func TestLCDFilterUsesFreeTypeFloorQuantization(t *testing.T) {
	worker := TWorker{
		LCDFilter: api.LCD_FILTER_DEFAULT,
		LCDLine:   []byte{0, 0, 0, 255, 0},
	}
	if got := worker.filterLCD(0); got != 76 {
		t.Fatalf("default LCD filter rounded to %d, want floor value 76", got)
	}

	worker.LCDFilter = api.LCD_FILTER_LIGHT
	worker.LCDLine = []byte{0, 0, 0, 255, 0}
	if got := worker.filterLCD(0); got != 84 {
		t.Fatalf("light LCD filter rounded to %d, want floor value 84", got)
	}

	worker.LCDFilter = api.LCD_FILTER_DEFAULT
	surface := []byte{0, 0, 0, 255, 0}
	if got := worker.filterLCDV(surface, 1, 5, 0, 2); got != 76 {
		t.Fatalf("default LCDV filter rounded to %d, want floor value 76", got)
	}
}

func TestLCDVRenderingUsesVerticalSubpixelSurface(t *testing.T) {
	rasterizer := NewSmoothRasterizer()
	outline := &mockOutline{
		points: []api.Vector{
			{X: 4 << 6, Y: 10<<6 + 32},
			{X: 16 << 6, Y: 10<<6 + 32},
			{X: 16 << 6, Y: 11<<6 + 32},
			{X: 4 << 6, Y: 11<<6 + 32},
		},
		tags:     []byte{1, 1, 1, 1},
		contours: []int{3},
	}
	bitmap := core.NewBitmapForRenderMode(20, 20, api.RenderModeLCDV)

	if bitmap.GetWidth() != 20 || bitmap.GetRows() != 60 || bitmap.GetPitch() != 20 || bitmap.GetPixelMode() != api.MODE_LCD_V {
		t.Fatalf("LCDV geometry got width=%d rows=%d pitch=%d mode=%d, want 20 60 20 %d",
			bitmap.GetWidth(), bitmap.GetRows(), bitmap.GetPitch(), bitmap.GetPixelMode(), api.MODE_LCD_V)
	}
	if err := rasterizer.Render(outline, bitmap); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	hasVerticalSubpixel := false
	for y := 30; y <= 35; y++ {
		if bitmap.Buffer[y*bitmap.Pitch+10] != 0 {
			hasVerticalSubpixel = true
			break
		}
	}
	if !hasVerticalSubpixel {
		t.Fatal("LCDV rendering did not set expected vertical subpixel rows")
	}
	if got := bitmap.Buffer[29*bitmap.Pitch+10]; got == 0 {
		t.Fatalf("LCDV default filter row 29 = %d, want vertical filter spill from covered row", got)
	}
	if got := bitmap.Buffer[5*bitmap.Pitch+10]; got != 0 {
		t.Fatalf("outside LCDV row = %d, want 0", got)
	}
}

func TestPlacedLCDVRenderingUsesLogicalFlipHeight(t *testing.T) {
	rasterizer := NewSmoothRasterizer()
	outline := &mockOutline{
		points: []api.Vector{
			{X: 4 << 6, Y: 10<<6 + 32},
			{X: 16 << 6, Y: 10<<6 + 32},
			{X: 16 << 6, Y: 11<<6 + 32},
			{X: 4 << 6, Y: 11<<6 + 32},
		},
		tags:     []byte{1, 1, 1, 1},
		contours: []int{3},
	}
	bitmap := core.NewBitmapForRenderMode(20, 20, api.RenderModeLCDV)
	bitmap.Top = 20

	if err := rasterizer.Render(outline, bitmap); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	for _, val := range bitmap.Buffer {
		if val != 0 {
			return
		}
	}
	t.Fatal("placed LCDV rendering produced an empty bitmap")
}
