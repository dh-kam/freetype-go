package math

import (
	"math"
	"testing"
)

func TestMulFix(t *testing.T) {
	engine := NewMathEngine()

	tests := []struct {
		a, b int32
		want int32
	}{
		{0, 0, 0},
		{1 << 16, 1 << 16, 1 << 16},
		{1 << 16, 2 << 16, 2 << 16},
		{0x8000, 0x8000, 0x4000}, // 0.5 * 0.5 = 0.25
		{-1 << 16, 1 << 16, -1 << 16},
		{-1 << 16, -1 << 16, 1 << 16},
		{0x10000, 0x08000, 0x08000}, // 1.0 * 0.5 = 0.5
		{0x10000, 0x08001, 0x08001},
		{0x08001, 0x08001, 0x04001},
	}

	for _, tt := range tests {
		got := engine.MulFix(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("MulFix(%d, %d) = %d; want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestDivFix(t *testing.T) {
	engine := NewMathEngine()

	tests := []struct {
		a, b int32
		want int32
	}{
		{0, 1 << 16, 0},
		{1 << 16, 1 << 16, 1 << 16},
		{2 << 16, 1 << 16, 2 << 16},
		{1 << 16, 2 << 16, 0x8000}, // 1.0 / 2.0 = 0.5
		{-1 << 16, 1 << 16, -1 << 16},
		{1 << 16, -1 << 16, -1 << 16},
		{-1 << 16, -1 << 16, 1 << 16},
		{1 << 16, 3 << 16, 21845}, // 1/3 in 16.16 is approx 0.33333 * 65536 = 21845.333... -> 21845
		{1 << 16, 2 << 16, 32768}, // 1/2 in 16.16 is 0.5 * 65536 = 32768
	}

	for _, tt := range tests {
		got := engine.DivFix(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("DivFix(%d, %d) = %d; want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCosSin(t *testing.T) {
	engine := NewMathEngine()

	// Test 0 radians
	if got := engine.Cos(0); got != 1<<16 {
		t.Errorf("Cos(0) = %d; want %d", got, 1<<16)
	}
	if got := engine.Sin(0); got != 0 {
		t.Errorf("Sin(0) = %d; want 0", got)
	}

	// Test PI/2 radians (approx 1.570796 * 65536 = 102943.7... -> 102944)
	pi2 := int32(math.Round(math.Pi / 2 * 65536))
	if got := engine.Cos(pi2); got != 0 {
		// math.Cos(math.Pi/2) is extremely small, but let's see what we get
		if got > 1 || got < -1 {
			t.Errorf("Cos(PI/2) = %d; want approx 0", got)
		}
	}
	if got := engine.Sin(pi2); got != 1<<16 {
		t.Errorf("Sin(PI/2) = %d; want %d", got, 1<<16)
	}

	// Test PI radians
	pi := int32(math.Round(math.Pi * 65536))
	if got := engine.Cos(pi); got != -1<<16 {
		t.Errorf("Cos(PI) = %d; want %d", got, -1<<16)
	}
	if got := engine.Sin(pi); got > 1 || got < -1 {
		t.Errorf("Sin(PI) = %d; want approx 0", got)
	}
}
