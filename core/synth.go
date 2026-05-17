package core

import (
	"errors"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/stroke"
)

// Oblique applies a shear matrix to the outline in the slot to slant it.
func Oblique(slot api.GlyphSlot) error {
	if slot == nil {
		return errors.New("nil GlyphSlot")
	}
	outline := slot.GetOutline()
	if outline == nil {
		return errors.New("nil outline in GlyphSlot")
	}

	// Standard FreeType slant is approx 12 degrees (0x0366A in 16.16)
	matrix := &api.Matrix{
		XX: 0x10000,
		XY: 0x0366A,
		YX: 0,
		YY: 0x10000,
	}

	outline.Transform(matrix)
	return nil
}

// Embolden strokes the outline to thicken it and replaces the slot's outline.
func Embolden(slot api.GlyphSlot) error {
	if slot == nil {
		return errors.New("nil GlyphSlot")
	}
	outline := slot.GetOutline()
	if outline == nil {
		return errors.New("nil outline in GlyphSlot")
	}

	s := stroke.NewStroker()
	// Set a reasonable stroke radius, e.g., 1 pixel (1 << 6 in 26.6)
	radius := int32(1 << 6)
	s.SetRadius(radius)
	s.SetCaps(stroke.LineCapRound)
	s.SetJoins(stroke.LineJoinRound)

	newOutline, err := s.Stroke(outline, radius)
	if err != nil {
		return err
	}

	coreOutline := &Outline{
		Points:   newOutline.GetPoints(),
		Tags:     newOutline.GetTags(),
		Contours: newOutline.GetContours(),
	}

	slot.SetOutline(coreOutline)
	return nil
}
