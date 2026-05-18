package core

import (
	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/math"
)

// System implements api.FreetypeSystem interface.
type System struct {
	mathEngine   api.MathEngine
	rasterizer   api.Rasterizer
	hinter       api.Hinter
	imageDecoder api.ImageDecoder
}

// NewSystem creates and returns a new instance of FreetypeSystem.
func NewSystem() api.FreetypeSystem {
	return NewSystemWithServices(nil, nil, nil)
}

// NewSystemWithServices creates a System with optional module services.
func NewSystemWithServices(mathEngine api.MathEngine, rasterizer api.Rasterizer, hinter api.Hinter) *System {
	if mathEngine == nil {
		mathEngine = math.NewMathEngine()
	}
	return &System{
		mathEngine: mathEngine,
		rasterizer: rasterizer,
		hinter:     hinter,
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

// SetRasterizer installs the rasterizer service returned through FreetypeSystem.
func (s *System) SetRasterizer(r api.Rasterizer) {
	s.rasterizer = r
}

// Rasterizer returns the configured rasterizer service.
func (s *System) Rasterizer() api.Rasterizer {
	return s.rasterizer
}

// SetHinter installs the hinter service returned through FreetypeSystem.
func (s *System) SetHinter(h api.Hinter) {
	s.hinter = h
}

// Hinter returns the configured hinter service.
func (s *System) Hinter() api.Hinter {
	return s.hinter
}
