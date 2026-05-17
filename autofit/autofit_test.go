package autofit

import (
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

func TestAutoHinter_EdgeSnapping(t *testing.T) {
	h := NewAutoHinter(nil)

	// Create a simple rectangle with non-grid aligned coordinates.
	// Horizontal segments at Y=10 and Y=70 (width=60)
	// Vertical segments at X=10 and X=70 (width=60)
	outline := &core.Outline{
		Points: []api.Vector{
			{X: 10, Y: 10}, {X: 70, Y: 10},
			{X: 70, Y: 70}, {X: 10, Y: 70},
		},
		Tags:     []byte{1, 1, 1, 1},
		Contours: []int{3},
	}

	err := h.Hint(outline, 12)
	if err != nil {
		t.Fatalf("Hint failed: %v", err)
	}

	// Y=10 is closer to 0 than 64, so it snaps to 0.
	// Stem width 60 snaps to 64.
	// Thus Y=70 (which is 10+60) snaps to 0+64 = 64.
	// Same for X.

	expectedPoints := []api.Vector{
		{X: 0, Y: 0}, {X: 64, Y: 0},
		{X: 64, Y: 64}, {X: 0, Y: 64},
	}

	for i, p := range outline.Points {
		if p.X != expectedPoints[i].X || p.Y != expectedPoints[i].Y {
			t.Errorf("Point %d: expected %+v, got %+v", i, expectedPoints[i], p)
		}
	}
}

func TestAutoHinter_NearlyHorizontalEdge(t *testing.T) {
	h := NewAutoHinter(nil)

	// Segment from (10, 10) to (70, 11). dy=1, which is < 8 units.
	// It should be detected as a horizontal edge and snapped to the same Y.
	outline := &core.Outline{
		Points: []api.Vector{
			{X: 10, Y: 10}, {X: 70, Y: 11},
			{X: 70, Y: 100}, {X: 10, Y: 100},
		},
		Tags:     []byte{1, 1, 1, 1},
		Contours: []int{3},
	}

	err := h.Hint(outline, 12)
	if err != nil {
		t.Fatalf("Hint failed: %v", err)
	}

	if outline.Points[0].Y != outline.Points[1].Y {
		t.Errorf("Nearly horizontal segment points should have same snapped Y, got %d and %d", outline.Points[0].Y, outline.Points[1].Y)
	}
	if outline.Points[0].Y != 0 {
		t.Errorf("Expected Y=0, got %d", outline.Points[0].Y)
	}
}

func TestAutoHinter_StemSnappingToStandardWidth(t *testing.T) {
	// Initialize with a specific standard width
	hinter := &AutoHinter{
		metrics: HintingMetrics{
			StandardWidth:  128, // 2 pixels
			StandardHeight: 128,
		},
	}

	// Horizontal stem with width 110.
	// 110 is closer to 128 (diff 18) than to 64 (diff 46) or 192 (diff 82).
	// So it should snap to 128.
	outline := &core.Outline{
		Points: []api.Vector{
			{X: 0, Y: 0}, {X: 100, Y: 0},
			{X: 100, Y: 110}, {X: 0, Y: 110},
		},
		Tags:     []byte{1, 1, 1, 1},
		Contours: []int{3},
	}

	err := hinter.Hint(outline, 12)
	if err != nil {
		t.Fatalf("Hint failed: %v", err)
	}

	y1 := outline.Points[0].Y
	y2 := outline.Points[2].Y
	width := abs(y1 - y2)

	if width != 128 {
		t.Errorf("Expected snapped stem width 128, got %d", width)
	}
}
