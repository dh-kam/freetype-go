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

func TestStrokeUsesConfiguredRadiusWhenArgumentIsZero(t *testing.T) {
	s := NewStroker()
	s.SetRadius(5 << 6)

	result, err := s.Stroke(horizontalLine(), 0)
	if err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	_, _, minY, maxY := bounds(result.Points)
	if minY != -(5<<6) || maxY != 5<<6 {
		t.Fatalf("stroke y bounds = %d..%d, want %d..%d", minY, maxY, -(5 << 6), 5<<6)
	}
}

func TestStrokeSquareCapExtendsLineEnds(t *testing.T) {
	s := NewStroker()
	s.SetCaps(LineCapSquare)

	result, err := s.Stroke(horizontalLine(), 10<<6)
	if err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	minX, maxX, _, _ := bounds(result.Points)
	if minX != -(10<<6) || maxX != 110<<6 {
		t.Fatalf("stroke x bounds = %d..%d, want %d..%d", minX, maxX, -(10 << 6), 110<<6)
	}
}

func TestStrokeRoundCapAddsArcPoints(t *testing.T) {
	s := NewStroker()
	s.SetCaps(LineCapRound)

	result, err := s.Stroke(horizontalLine(), 10<<6)
	if err != nil {
		t.Fatalf("Stroke failed: %v", err)
	}

	if len(result.Points) <= 4 {
		t.Fatalf("round-capped line has %d points, want more than butt cap", len(result.Points))
	}
}

func TestStrokeJoinStyles(t *testing.T) {
	outline := &Outline{
		Points: []api.Vector{
			{X: 0, Y: 0},
			{X: 100 << 6, Y: 0},
			{X: 100 << 6, Y: 100 << 6},
		},
		Tags:     []byte{1, 1, 1},
		Contours: []int{2},
	}

	bevel := NewStroker()
	bevel.SetJoins(LineJoinBevel)
	bevelResult, err := bevel.Stroke(outline, 10<<6)
	if err != nil {
		t.Fatalf("bevel stroke failed: %v", err)
	}

	round := NewStroker()
	round.SetJoins(LineJoinRound)
	roundResult, err := round.Stroke(outline, 10<<6)
	if err != nil {
		t.Fatalf("round stroke failed: %v", err)
	}

	miter := NewStroker()
	miter.SetJoins(LineJoinMiter)
	miterResult, err := miter.Stroke(outline, 10<<6)
	if err != nil {
		t.Fatalf("miter stroke failed: %v", err)
	}

	if len(roundResult.Points) <= len(bevelResult.Points) {
		t.Fatalf("round join points = %d, want more than bevel %d", len(roundResult.Points), len(bevelResult.Points))
	}
	if len(miterResult.Points) <= len(bevelResult.Points) {
		t.Fatalf("miter join points = %d, want more than bevel %d", len(miterResult.Points), len(bevelResult.Points))
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

func horizontalLine() *Outline {
	return &Outline{
		Points: []api.Vector{
			{X: 0, Y: 0},
			{X: 100 << 6, Y: 0},
		},
		Tags:     []byte{1, 1},
		Contours: []int{1},
	}
}

func bounds(points []api.Vector) (minX, maxX, minY, maxY int32) {
	if len(points) == 0 {
		return 0, 0, 0, 0
	}
	minX, maxX = points[0].X, points[0].X
	minY, maxY = points[0].Y, points[0].Y
	for _, p := range points[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return minX, maxX, minY, maxY
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
