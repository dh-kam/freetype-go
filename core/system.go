package core

import (
	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/math"
)

// System implements api.FreetypeSystem interface.
type System struct {
	mathEngine   api.MathEngine
	imageDecoder api.ImageDecoder
}

// NewSystem creates and returns a new instance of FreetypeSystem.
func NewSystem() api.FreetypeSystem {
	return &System{
		mathEngine: math.NewMathEngine(),
	}
}

func (s *System) SetImageDecoder(dec api.ImageDecoder) {
	s.imageDecoder = dec
}

func (s *System) GetImageDecoder() api.ImageDecoder {
	return s.imageDecoder
}

// Math returns the math engine.
func (s *System) Math() api.MathEngine {
	return s.mathEngine
}

// Rasterizer returns the rasterizer (currently nil).
func (s *System) Rasterizer() api.Rasterizer {
	return nil
}

// Hinter returns the hinter (currently nil).
func (s *System) Hinter() api.Hinter {
	return nil
}
