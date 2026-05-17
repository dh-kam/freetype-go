package cff

import (
	"testing"
)

func TestBlendOperator(t *testing.T) {
	// 2 default values, 1 region (k=1)
	// Base values: 10, 20
	// Deltas for base 10: +5 (so region 1 gets 10+5=15)
	// Deltas for base 20: -5 (so region 1 gets 20-5=15)
	// Stack: 10 20 5 -5 2 blend -> 15 15
	// Then `rmoveto` (21)
	// Then `endchar` (14)

	// Encodings:
	// 10: 10 + 139 = 149
	// 20: 20 + 139 = 159
	// 5:  5 + 139 = 144
	// -5: -5 + 139 = 134
	// 2:  2 + 139 = 141

	data := []byte{
		149, // 10
		159, // 20
		144, // 5
		134, // -5
		141, // 2 (n)
		16,  // blend
		21,  // rmoveto
		14,  // endchar
	}

	blendVector := []float64{1.0} // Active region weight = 1.0

	outline, err := DecodeCharString(data, nil, nil, blendVector)
	if err != nil {
		t.Fatalf("DecodeCharString failed: %v", err)
	}

	if len(outline.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(outline.Points))
	}
	if outline.Points[0].X != 15*64 || outline.Points[0].Y != 15*64 {
		t.Errorf("expected (15*64, 15*64), got (%d, %d)", outline.Points[0].X, outline.Points[0].Y)
	}
}
