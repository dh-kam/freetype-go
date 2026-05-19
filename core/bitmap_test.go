package core

import (
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestNewBitmapWithPixelModeMonoPacksBits(t *testing.T) {
	bitmap := NewBitmapWithPixelMode(10, 2, api.MODE_MONO)
	if bitmap.GetWidth() != 10 || bitmap.GetRows() != 2 {
		t.Fatalf("geometry = %dx%d, want 10x2", bitmap.GetWidth(), bitmap.GetRows())
	}
	if bitmap.GetPitch() != 2 {
		t.Fatalf("pitch = %d, want 2", bitmap.GetPitch())
	}
	if len(bitmap.GetBuffer()) != 4 {
		t.Fatalf("buffer length = %d, want 4", len(bitmap.GetBuffer()))
	}
	if !bitmap.IsPackedMono() {
		t.Fatal("bitmap should report packed mono storage")
	}

	bitmap.SetMonoPixel(0, 0, true)
	bitmap.SetMonoPixel(7, 0, true)
	bitmap.SetMonoPixel(8, 0, true)
	bitmap.SetMonoPixel(9, 1, true)

	want := []byte{0x81, 0x80, 0x00, 0x40}
	for i, got := range bitmap.GetBuffer() {
		if got != want[i] {
			t.Fatalf("buffer[%d] = %#02x, want %#02x", i, got, want[i])
		}
	}
	if !bitmap.MonoPixel(0, 0) || !bitmap.MonoPixel(7, 0) || !bitmap.MonoPixel(8, 0) || !bitmap.MonoPixel(9, 1) {
		t.Fatal("set mono pixels were not readable")
	}
	if bitmap.MonoPixel(1, 0) || bitmap.MonoPixel(9, 0) || bitmap.MonoPixel(8, 1) {
		t.Fatal("unset mono pixels read as set")
	}
}

func TestBitmapPlacementGettersAndLegacyMono(t *testing.T) {
	bitmap := NewBitmap(-1, 2)
	if bitmap.GetWidth() != 0 || bitmap.GetRows() != 2 || bitmap.GetPitch() != 0 || len(bitmap.GetBuffer()) != 0 {
		t.Fatalf("clamped bitmap got width=%d rows=%d pitch=%d len=%d, want 0 2 0 0",
			bitmap.GetWidth(), bitmap.GetRows(), bitmap.GetPitch(), len(bitmap.GetBuffer()))
	}

	bitmap = NewBitmap(3, 2)
	bitmap.Left = -2
	bitmap.Top = 7
	if bitmap.GetLeft() != -2 || bitmap.GetTop() != 7 {
		t.Fatalf("placement = %d,%d, want -2,7", bitmap.GetLeft(), bitmap.GetTop())
	}
	if _, _, ok := api.GetBitmapPlacement(bitmap); ok {
		t.Fatal("manual bitmap exposed unset FreeType placement")
	}

	bitmap.SetPixelMode(api.MODE_MONO)
	if bitmap.IsPackedMono() {
		t.Fatal("legacy bitmap should not become packed mono after SetPixelMode")
	}
	if !bitmap.SetMonoPixel(1, 1, true) || !bitmap.MonoPixel(1, 1) {
		t.Fatal("legacy mono pixel was not set")
	}
	if !bitmap.SetMonoPixel(1, 1, false) || bitmap.MonoPixel(1, 1) {
		t.Fatal("legacy mono pixel was not cleared")
	}
	if bitmap.SetMonoPixel(3, 1, true) || bitmap.MonoPixel(-1, 0) {
		t.Fatal("out-of-range legacy mono operation succeeded")
	}

	bitmap.SetPixelMode(api.MODE_GRAY)
	if bitmap.GetPixelMode() != api.MODE_GRAY {
		t.Fatalf("pixel mode = %d, want MODE_GRAY", bitmap.GetPixelMode())
	}
}

func TestNewBitmapForRenderModeSurfaceDimensions(t *testing.T) {
	tests := []struct {
		name       string
		mode       api.RenderMode
		width      int
		rows       int
		pitch      int
		pixelMode  uint8
		packedMono bool
	}{
		{name: "normal", mode: api.RenderModeNormal, width: 7, rows: 5, pitch: 7, pixelMode: api.MODE_GRAY},
		{name: "light", mode: api.RenderModeLight, width: 7, rows: 5, pitch: 7, pixelMode: api.MODE_GRAY},
		{name: "mono", mode: api.RenderModeMono, width: 7, rows: 5, pitch: 2, pixelMode: api.MODE_MONO, packedMono: true},
		{name: "lcd", mode: api.RenderModeLCD, width: 21, rows: 5, pitch: 24, pixelMode: api.MODE_LCD},
		{name: "lcdv", mode: api.RenderModeLCDV, width: 7, rows: 15, pitch: 7, pixelMode: api.MODE_LCD_V},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bitmap := NewBitmapForRenderMode(7, 5, tc.mode)
			if bitmap.GetWidth() != tc.width || bitmap.GetRows() != tc.rows || bitmap.GetPitch() != tc.pitch {
				t.Fatalf("geometry got width=%d rows=%d pitch=%d, want width=%d rows=%d pitch=%d",
					bitmap.GetWidth(), bitmap.GetRows(), bitmap.GetPitch(), tc.width, tc.rows, tc.pitch)
			}
			if bitmap.GetPixelMode() != tc.pixelMode {
				t.Fatalf("pixel mode = %d, want %d", bitmap.GetPixelMode(), tc.pixelMode)
			}
			if bitmap.IsPackedMono() != tc.packedMono {
				t.Fatalf("packed mono = %v, want %v", bitmap.IsPackedMono(), tc.packedMono)
			}
		})
	}
}

func TestBitmapPitchAndMonoSpanEdges(t *testing.T) {
	if got := BitmapPitch(8, api.MODE_NONE); got != 0 {
		t.Fatalf("MODE_NONE pitch = %d, want 0", got)
	}
	if got := BitmapPitch(-2, api.MODE_GRAY); got != 0 {
		t.Fatalf("negative width pitch = %d, want 0", got)
	}
	if got := BitmapPitch(17, api.MODE_MONO); got != 4 {
		t.Fatalf("mono pitch = %d, want 4", got)
	}
	if got := BitmapPitch(5, api.MODE_LCD); got != 8 {
		t.Fatalf("lcd pitch = %d, want 8", got)
	}
	if got := PixelModeForRenderMode(api.RenderMode(99)); got != api.MODE_GRAY {
		t.Fatalf("unknown render mode pixel mode = %d, want MODE_GRAY", got)
	}

	buf := make([]byte, 2)
	if !FillMonoSpan(buf, 2, 0, 1, 15) {
		t.Fatal("FillMonoSpan returned false for clipped span")
	}
	if buf[0] != 0x7f || buf[1] != 0xfe {
		t.Fatalf("span buffer = [%#02x %#02x], want [0x7f 0xfe]", buf[0], buf[1])
	}
	if FillMonoSpan(buf, 0, 0, 0, 1) {
		t.Fatal("FillMonoSpan accepted zero pitch")
	}
	if FillMonoSpan(buf, 2, -1, 0, 1) {
		t.Fatal("FillMonoSpan accepted negative y")
	}
	if FillMonoSpan(buf, 2, 0, 3, 3) {
		t.Fatal("FillMonoSpan accepted empty span")
	}
	if MonoPixel(buf, -1, 0, 0) || SetMonoPixel(buf, -1, 0, 0, true) {
		t.Fatal("mono helpers accepted negative pitch")
	}
}

func TestPrepareBitmapForOutlinePlacementAndTranslation(t *testing.T) {
	outline := &Outline{
		Points: []api.Vector{
			{X: -3*64 - 32, Y: -2*64 - 1},
			{X: 5*64 + 1, Y: 7*64 + 32},
		},
		Tags:     []byte{1, 1},
		Contours: []int{1},
	}

	renderOutline, bitmap, metrics, ok := PrepareBitmapForOutline(outline, -1, api.RenderModeLCD)
	if !ok {
		t.Fatal("PrepareBitmapForOutline returned !ok")
	}
	if metrics.Left != -5 || metrics.Top != 8 || metrics.Width != 11 || metrics.Rows != 11 {
		t.Fatalf("metrics got left=%d top=%d width=%d rows=%d, want -5 8 11 11",
			metrics.Left, metrics.Top, metrics.Width, metrics.Rows)
	}
	if metrics.SurfaceWidth != 33 || metrics.SurfaceRows != 11 || metrics.Pitch != 36 || metrics.PixelMode != api.MODE_LCD {
		t.Fatalf("surface got width=%d rows=%d pitch=%d mode=%d, want 33 11 36 %d",
			metrics.SurfaceWidth, metrics.SurfaceRows, metrics.Pitch, metrics.PixelMode, api.MODE_LCD)
	}
	if bitmap.Left != -5 || bitmap.Top != 8 {
		t.Fatalf("bitmap placement = left %d top %d, want -5 8", bitmap.Left, bitmap.Top)
	}
	if left, top, ok := api.GetBitmapPlacement(bitmap); !ok || left != -5 || top != 8 {
		t.Fatalf("placement provider = left %d top %d ok %v, want -5 8 true", left, top, ok)
	}
	if renderOutline == nil || len(renderOutline.Points) != len(outline.Points) {
		t.Fatalf("translated outline length = %d, want %d", len(renderOutline.Points), len(outline.Points))
	}
	if got := renderOutline.Points[0]; got.X != 96 || got.Y != 8*64-(-2*64-1) {
		t.Fatalf("translated first point = (%d,%d), want (96,%d)", got.X, got.Y, 8*64-(-2*64-1))
	}

	topZeroOutline := &Outline{
		Points: []api.Vector{
			{X: 0, Y: -10 << 6},
			{X: 10 << 6, Y: 0},
		},
		Tags:     []byte{1, 1},
		Contours: []int{1},
	}
	_, topZeroBitmap, _, ok := PrepareBitmapForOutline(topZeroOutline, -1, api.RenderModeNormal)
	if !ok {
		t.Fatal("PrepareBitmapForOutline returned !ok for top-zero outline")
	}
	if left, top, ok := api.GetBitmapPlacement(topZeroBitmap); !ok || left != 0 || top != 0 {
		t.Fatalf("top-zero placement provider = left %d top %d ok %v, want 0 0 true", left, top, ok)
	}
}

func TestPrepareBitmapForOutlineEmpty(t *testing.T) {
	if _, _, _, ok := PrepareBitmapForOutline(&Outline{}, -1, api.RenderModeNormal); ok {
		t.Fatal("empty outline unexpectedly produced bitmap metrics")
	}
	if _, ok := OutlineBitmapMetrics(nil, -1, api.RenderModeNormal); ok {
		t.Fatal("nil outline unexpectedly produced bitmap metrics")
	}
}

func TestOutlineBitmapMetricsPointCountAndEmptyBitmap(t *testing.T) {
	outline := &Outline{
		Points: []api.Vector{
			{X: 64, Y: 64},
			{X: 64, Y: 64},
			{X: 40 << 6, Y: 40 << 6},
		},
		Tags:     []byte{1, 1, 1},
		Contours: []int{2},
	}

	_, bitmap, metrics, ok := PrepareBitmapForOutline(outline, 2, api.RenderModeNormal)
	if !ok {
		t.Fatal("PrepareBitmapForOutline returned !ok")
	}
	if metrics.Left != 1 || metrics.Top != 1 || metrics.Width != 0 || metrics.Rows != 0 {
		t.Fatalf("metrics = left %d top %d width %d rows %d, want 1 1 0 0",
			metrics.Left, metrics.Top, metrics.Width, metrics.Rows)
	}
	if bitmap.GetWidth() != 0 || bitmap.GetRows() != 0 || bitmap.GetPitch() != 0 {
		t.Fatalf("empty bitmap geometry = %dx%d pitch %d, want 0x0 pitch 0",
			bitmap.GetWidth(), bitmap.GetRows(), bitmap.GetPitch())
	}
}

func TestOutlineBitmapMetricsMonoRoundingAndCollapse(t *testing.T) {
	outline := &Outline{
		Points: []api.Vector{
			{X: 64, Y: 64},
			{X: 64, Y: 64},
			{X: 40 << 6, Y: 40 << 6},
		},
		Tags:     []byte{1, 1, 1},
		Contours: []int{2},
	}

	_, bitmap, metrics, ok := PrepareBitmapForOutline(outline, 2, api.RenderModeMono)
	if !ok {
		t.Fatal("PrepareBitmapForOutline returned !ok")
	}
	if metrics.Left != 1 || metrics.Top != 2 || metrics.Width != 1 || metrics.Rows != 1 {
		t.Fatalf("mono collapsed metrics = left %d top %d width %d rows %d, want 1 2 1 1",
			metrics.Left, metrics.Top, metrics.Width, metrics.Rows)
	}
	if bitmap.GetWidth() != 1 || bitmap.GetRows() != 1 || bitmap.GetPitch() != 2 {
		t.Fatalf("mono collapsed bitmap geometry = %dx%d pitch %d, want 1x1 pitch 2",
			bitmap.GetWidth(), bitmap.GetRows(), bitmap.GetPitch())
	}

	fractional := &Outline{
		Points: []api.Vector{
			{X: -3*64 - 32, Y: -2*64 - 1},
			{X: 5*64 + 1, Y: 7*64 + 32},
		},
		Tags:     []byte{1, 1},
		Contours: []int{1},
	}
	_, _, metrics, ok = PrepareBitmapForOutline(fractional, -1, api.RenderModeMono)
	if !ok {
		t.Fatal("PrepareBitmapForOutline returned !ok for fractional mono outline")
	}
	if metrics.Left != -4 || metrics.Top != 8 || metrics.Width != 9 || metrics.Rows != 10 || metrics.Pitch != 2 {
		t.Fatalf("fractional mono metrics = left %d top %d width %d rows %d pitch %d, want -4 8 9 10 2",
			metrics.Left, metrics.Top, metrics.Width, metrics.Rows, metrics.Pitch)
	}
}

func TestOutlineBitmapMetricsLCDVPaddingAffectsTopAndRows(t *testing.T) {
	outline := &Outline{
		Points: []api.Vector{
			{X: 1 << 6, Y: -2 << 6},
			{X: 6 << 6, Y: 8 << 6},
		},
		Tags:     []byte{1, 1},
		Contours: []int{1},
	}

	_, bitmap, metrics, ok := PrepareBitmapForOutline(outline, -1, api.RenderModeLCDV)
	if !ok {
		t.Fatal("PrepareBitmapForOutline returned !ok")
	}
	if metrics.Left != 1 || metrics.Top != 9 || metrics.Width != 5 || metrics.Rows != 12 {
		t.Fatalf("lcd-v metrics = left %d top %d width %d rows %d, want 1 9 5 12",
			metrics.Left, metrics.Top, metrics.Width, metrics.Rows)
	}
	if bitmap.GetWidth() != 5 || bitmap.GetRows() != 36 || bitmap.GetPitch() != 5 {
		t.Fatalf("lcd-v bitmap geometry = %dx%d pitch %d, want 5x36 pitch 5",
			bitmap.GetWidth(), bitmap.GetRows(), bitmap.GetPitch())
	}
}
