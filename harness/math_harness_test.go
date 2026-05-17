//go:build cgo && freetype_harness

package harness

import (
	"math/rand"
	"testing"
)

func TestParityMulFix(t *testing.T) {
	// Test edge cases
	edgeCases := []int32{0, 1, -1, 0x10000, -0x10000, 0x7FFFFFFF, -0x7FFFFFFF, -2147483648, 32768, -32768, 0x7FFF, -0x7FFF}
	for _, a := range edgeCases {
		for _, b := range edgeCases {
			goRes, cRes, equal := CompareMulFix(a, b)
			if !equal {
				t.Errorf("MulFix(%d, %d) mismatch: Go=%d, C=%d", a, b, goRes, cRes)
			}
		}
	}

	// Random tests
	for i := 0; i < 10000; i++ {
		a := rand.Int31()
		if rand.Intn(2) == 0 {
			a = -a
		}
		b := rand.Int31()
		if rand.Intn(2) == 0 {
			b = -b
		}
		goRes, cRes, equal := CompareMulFix(a, b)
		if !equal {
			t.Errorf("MulFix(%d, %d) mismatch: Go=%d, C=%d", a, b, goRes, cRes)
		}
	}
}

func TestParityDivFix(t *testing.T) {
	// Test edge cases
	edgeCases := []int32{0, 1, -1, 0x10000, -0x10000, 0x7FFFFFFF, -0x7FFFFFFF, 0x7FFF, -0x7FFF}
	for _, a := range edgeCases {
		for _, b := range edgeCases {
			if b == 0 {
				continue // Skip division by zero for now or handle it if C version handles it
			}
			goRes, cRes, equal := CompareDivFix(a, b)
			if !equal {
				t.Errorf("DivFix(%d, %d) mismatch: Go=%d, C=%d", a, b, goRes, cRes)
			}
		}
	}

	// Random tests
	for i := 0; i < 10000; i++ {
		a := rand.Int31()
		if rand.Intn(2) == 0 {
			a = -a
		}
		b := rand.Int31()
		if b == 0 {
			b = 1
		}
		if rand.Intn(2) == 0 {
			b = -b
		}
		goRes, cRes, equal := CompareDivFix(a, b)
		if !equal {
			t.Errorf("DivFix(%d, %d) mismatch: Go=%d, C=%d", a, b, goRes, cRes)
		}
	}
}
