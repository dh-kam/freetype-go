package api

import "testing"

func TestLoadFlagTargetMaskCanonicalization(t *testing.T) {
	flags := LoadNoHinting | LoadNoBitmap | LoadTargetLCD
	canonical := (flags &^ LoadTargetMask) | LoadTargetMono

	if canonical&LoadNoHinting == 0 {
		t.Fatalf("LoadNoHinting was cleared: %#x", canonical)
	}
	if canonical&LoadNoBitmap == 0 {
		t.Fatalf("LoadNoBitmap was cleared: %#x", canonical)
	}
	if got := canonical & LoadTargetMask; got != LoadTargetMono {
		t.Fatalf("target bits = %#x, want %#x", got, LoadTargetMono)
	}
}

func TestLoadFlagBitsDoNotCollideWithTargetMask(t *testing.T) {
	flags := map[string]int{
		"no-hinting":                  LoadNoHinting,
		"no-scale":                    LoadNoScale,
		"render":                      LoadRender,
		"no-bitmap":                   LoadNoBitmap,
		"vertical-layout":             LoadVerticalLayout,
		"force-autohint":              LoadForceAutohint,
		"crop-bitmap":                 LoadCropBitmap,
		"pedantic":                    LoadPedantic,
		"ignore-global-advance-width": LoadIgnoreGlobalAdvanceWidth,
		"no-recurse":                  LoadNoRecurse,
		"ignore-transform":            LoadIgnoreTransform,
		"monochrome":                  LoadMonochrome,
		"linear-design":               LoadLinearDesign,
		"no-autohint":                 LoadNoAutohint,
		"color":                       LoadColor,
		"compute-metrics":             LoadComputeMetrics,
		"bitmap-metrics-only":         LoadBitmapMetricsOnly,
		"no-svg":                      LoadNoSVG,
	}
	for name, flag := range flags {
		if flag&LoadTargetMask != 0 {
			t.Fatalf("%s collides with target mask: %#x", name, flag)
		}
	}
}

func TestRenderModeValuesAreStable(t *testing.T) {
	tests := []struct {
		name string
		got  RenderMode
		want RenderMode
	}{
		{"none", RenderModeNone, -1},
		{"normal", RenderModeNormal, 0},
		{"light", RenderModeLight, 1},
		{"mono", RenderModeMono, 2},
		{"lcd", RenderModeLCD, 3},
		{"lcd-v", RenderModeLCDV, 4},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Fatalf("%s render mode = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestPixelModeValuesAreStable(t *testing.T) {
	tests := []struct {
		name string
		got  uint8
		want uint8
	}{
		{"none", MODE_NONE, 0},
		{"mono", MODE_MONO, 1},
		{"gray", MODE_GRAY, 2},
		{"lcd", MODE_LCD, 3},
		{"lcd-v", MODE_LCD_V, 4},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Fatalf("%s pixel mode = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

type placementBitmap struct{}

func (placementBitmap) GetRows() int            { return 0 }
func (placementBitmap) GetWidth() int           { return 0 }
func (placementBitmap) GetPitch() int           { return 0 }
func (placementBitmap) GetBuffer() []byte       { return nil }
func (placementBitmap) GetPixelMode() uint8     { return MODE_NONE }
func (placementBitmap) SetPixelMode(mode uint8) {}
func (placementBitmap) GetLeft() int            { return -3 }
func (placementBitmap) GetTop() int             { return 9 }

func TestGetBitmapPlacementUsesOptionalProvider(t *testing.T) {
	left, top, ok := GetBitmapPlacement(placementBitmap{})
	if !ok || left != -3 || top != 9 {
		t.Fatalf("placement = %d,%d ok=%v, want -3,9 true", left, top, ok)
	}
}

func TestGetBitmapPlacementMissingProvider(t *testing.T) {
	if _, _, ok := GetBitmapPlacement(nil); ok {
		t.Fatal("nil bitmap returned placement")
	}
	if _, _, ok := GetBitmapPlacement(bitmapWithoutPlacement{}); ok {
		t.Fatal("bitmap without placement provider returned placement")
	}
}

type bitmapWithoutPlacement struct{}

func (bitmapWithoutPlacement) GetRows() int            { return 0 }
func (bitmapWithoutPlacement) GetWidth() int           { return 0 }
func (bitmapWithoutPlacement) GetPitch() int           { return 0 }
func (bitmapWithoutPlacement) GetBuffer() []byte       { return nil }
func (bitmapWithoutPlacement) GetPixelMode() uint8     { return MODE_NONE }
func (bitmapWithoutPlacement) SetPixelMode(mode uint8) {}
