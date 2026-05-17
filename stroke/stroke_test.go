package stroke

import (
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestStrokeLine(t *testing.T) {
	s := NewStroker()

	outline := &Outline{
		Points: []api.Vector{
			{X: 0, Y: 0},
			{X: 100 << 6, Y: 0},
		},
		Tags:     []byte{1, 1},
		Contours: []int{1},
	}

	result, err := s.Stroke(outline, 10<<6)
	if err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	if len(result.Points) != 4 {
		t.Errorf("expected 4 points for stroked line, got %d", len(result.Points))
	}

	expected := []api.Vector{
		{X: 100 << 6, Y: 10 << 6},
		{X: 0, Y: 10 << 6},
		{X: 0, Y: -10 << 6},
		{X: 100 << 6, Y: -10 << 6},
	}

	for i, p := range result.Points {
		if i >= len(expected) {
			break
		}
		if p.X != expected[i].X || p.Y != expected[i].Y {
			t.Errorf("point %d: expected %v, got %v", i, expected[i], p)
		}
	}
}

func TestOutlineTransforms(t *testing.T) {
	outline := &Outline{
		Points: []api.Vector{
			{X: 10 << 6, Y: 20 << 6},
			{X: -5 << 6, Y: 15 << 6},
		},
		Tags:     []byte{1, 1},
		Contours: []int{1},
	}

	outline.Scale(2<<16, 1<<16)
	outline.Translate(3<<6, -4<<6)
	outline.Transform(&api.Matrix{
		XX: 0,
		XY: 1 << 16,
		YX: 1 << 16,
		YY: 0,
	})

	expected := []api.Vector{
		{X: 16 << 6, Y: 23 << 6},
		{X: 11 << 6, Y: -7 << 6},
	}
	for i, want := range expected {
		if got := outline.Points[i]; got != want {
			t.Fatalf("point %d: got %+v, want %+v", i, got, want)
		}
	}

	outline.Transform(nil)
	for i, want := range expected {
		if got := outline.Points[i]; got != want {
			t.Fatalf("nil transform point %d: got %+v, want %+v", i, got, want)
		}
	}
}

func TestStrokeSharpV(t *testing.T) {
	s := NewStroker()

	// Sharp V shape
	outline := &Outline{
		Points: []api.Vector{
			{X: 100 << 6, Y: 100 << 6},
			{X: 0, Y: 0},
			{X: 100 << 6, Y: -100 << 6},
		},
		Tags:     []byte{1, 1, 1},
		Contours: []int{2},
	}

	// Huge radius
	result, err := s.Stroke(outline, 50<<6)
	if err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	// Verify no crossing segments (simple intersection check on the result)
	edges := []struct {
		p1, p2 api.Vector
	}{}

	startIdx := 0
	for _, endIdx := range result.Contours {
		for i := startIdx; i <= endIdx; i++ {
			p1 := result.Points[i]
			p2 := result.Points[startIdx]
			if i < endIdx {
				p2 = result.Points[i+1]
			}
			edges = append(edges, struct{ p1, p2 api.Vector }{p1, p2})
		}
		startIdx = endIdx + 1
	}

	for i := 0; i < len(edges); i++ {
		for j := i + 1; j < len(edges); j++ {
			// skip adjacent edges
			if i == j-1 || (i == 0 && j == len(edges)-1) {
				continue
			}

			e1 := edges[i]
			e2 := edges[j]

			P := e1.p1
			E := api.Vector{X: e1.p2.X - e1.p1.X, Y: e1.p2.Y - e1.p1.Y}
			Q := e2.p1
			F := api.Vector{X: e2.p2.X - e2.p1.X, Y: e2.p2.Y - e2.p1.Y}

			denom := int64(E.Y)*int64(F.X) - int64(E.X)*int64(F.Y)
			if denom == 0 {
				continue
			}
			t_num := int64(Q.Y-P.Y)*int64(F.X) - int64(Q.X-P.X)*int64(F.Y)
			u_num := int64(Q.Y-P.Y)*int64(E.X) - int64(Q.X-P.X)*int64(E.Y)

			tt := float64(t_num) / float64(denom)
			u := float64(u_num) / float64(denom)

			if tt > 0.01 && tt < 0.99 && u > 0.01 && u < 0.99 {
				t.Errorf("Found intersection between edge %d and %d at t=%f, u=%f", i, j, tt, u)
			}
		}
	}
}
