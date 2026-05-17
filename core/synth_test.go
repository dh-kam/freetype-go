package core

import (
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

// MockGlyphSlot implements api.GlyphSlot for testing
type MockGlyphSlot struct {
	outline *Outline
}

func (m *MockGlyphSlot) GetOutline() api.Outline {
	if m.outline == nil {
		return nil
	}
	return m.outline
}

func (m *MockGlyphSlot) SetOutline(outline api.Outline) {
	if o, ok := outline.(*Outline); ok {
		m.outline = o
	}
}

func (m *MockGlyphSlot) GetBitmap() api.Bitmap {
	return nil
}

func (m *MockGlyphSlot) GetImage() *api.Image {
	return nil
}

func createSquareOutline() *Outline {
	return &Outline{
		Points: []api.Vector{
			{X: 0, Y: 0},
			{X: 100 << 6, Y: 0},
			{X: 100 << 6, Y: 100 << 6},
			{X: 0, Y: 100 << 6},
		},
		Tags: []byte{
			1,
			1,
			1,
			1,
		},
		Contours: []int{3},
	}
}

func TestOblique(t *testing.T) {
	outline := createSquareOutline()
	slot := &MockGlyphSlot{outline: outline}

	err := Oblique(slot)
	if err != nil {
		t.Fatalf("Oblique failed: %v", err)
	}

	// Matrix was: XX: 0x10000, XY: 0x0366A, YX: 0, YY: 0x10000
	// For point 2 (X: 100<<6, Y: 100<<6):
	// x' = (x*XX + y*XY) >> 16
	// y' = (x*YX + y*YY) >> 16
	// y' = y = 100<<6
	// x' = 100<<6 + (100<<6 * 0x0366A) >> 16

	p2 := slot.outline.Points[2]
	expectedY := int32(100 << 6)
	if p2.Y != expectedY {
		t.Errorf("Expected Y=%d, got %d", expectedY, p2.Y)
	}

	// 0x0366A is 13930. (100<<6 * 13930) >> 16 = (6400 * 13930) >> 16 = 89152000 >> 16 = 1360
	expectedX := int32(100<<6) + 1360 // 6400 + 1360 = 7760
	if p2.X != expectedX {
		t.Errorf("Expected X close to %d, got %d", expectedX, p2.X)
	}
}

func TestEmbolden(t *testing.T) {
	outline := createSquareOutline()
	slot := &MockGlyphSlot{outline: outline}

	err := Embolden(slot)
	if err != nil {
		t.Fatalf("Embolden failed: %v", err)
	}

	if slot.outline == outline {
		t.Errorf("Expected outline to be replaced")
	}

	// Stroker should produce more points for a square
	if len(slot.outline.Points) <= 4 {
		t.Errorf("Expected more points after embolden, got %d", len(slot.outline.Points))
	}
}
