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
