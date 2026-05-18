package type1

// CompanionMetrics adapts optional AFM and PFM companion metrics to glyph-name
// and encoded-code lookups.
type CompanionMetrics struct {
	AFM      *AFM
	PFM      *PFM
	Encoding [256]string
}

// GlyphNameByCode returns the glyph name mapped by Encoding for an encoded
// character code.
func (m *CompanionMetrics) GlyphNameByCode(code int) (string, bool) {
	if m == nil || code < 0 || code >= len(m.Encoding) {
		return "", false
	}
	name := m.Encoding[code]
	if name == "" {
		return "", false
	}
	return name, true
}

// CodeByGlyphName returns the lowest encoded character code mapped to name.
func (m *CompanionMetrics) CodeByGlyphName(name string) (int, bool) {
	if m == nil || name == "" {
		return 0, false
	}
	for code, glyphName := range m.Encoding {
		if glyphName == name {
			return code, true
		}
	}
	return 0, false
}

// WidthByGlyphName returns the horizontal width for a named glyph.
//
// AFM glyph-name metrics take precedence. When AFM does not have the glyph,
// PFM is used only if the glyph name appears in Encoding. If the same glyph
// name appears more than once in Encoding, the lowest code is used.
func (m *CompanionMetrics) WidthByGlyphName(name string) (float64, bool) {
	if m == nil || name == "" {
		return 0, false
	}
	if width, ok := m.AFM.WidthXByName(name); ok {
		return width, true
	}
	code, ok := m.CodeByGlyphName(name)
	if !ok {
		return 0, false
	}
	return m.pfmWidthByCode(code)
}

// WidthByCode returns the horizontal width for an encoded character code.
//
// When Encoding maps the code to a glyph name, AFM glyph-name metrics take
// precedence over code-based AFM and PFM metrics.
func (m *CompanionMetrics) WidthByCode(code int) (float64, bool) {
	if m == nil || code < 0 || code >= len(m.Encoding) {
		return 0, false
	}
	if name, ok := m.GlyphNameByCode(code); ok {
		if width, ok := m.AFM.WidthXByName(name); ok {
			return width, true
		}
	}
	if width, ok := m.AFM.WidthXByCode(code); ok {
		return width, true
	}
	return m.pfmWidthByCode(code)
}

// LeftSideBearingByGlyphName returns the AFM glyph bounding box lower-left x
// coordinate for a named glyph.
func (m *CompanionMetrics) LeftSideBearingByGlyphName(name string) (float64, bool) {
	if m == nil || name == "" {
		return 0, false
	}
	metric, ok := m.AFM.GlyphMetricByName(name)
	if !ok {
		return 0, false
	}
	return metric.BBox[0], true
}

// LeftSideBearingByCode returns the AFM glyph bounding box lower-left x
// coordinate for an encoded character code.
//
// When Encoding maps the code to a glyph name, AFM glyph-name metrics take
// precedence over code-based AFM metrics.
func (m *CompanionMetrics) LeftSideBearingByCode(code int) (float64, bool) {
	if m == nil || code < 0 || code >= len(m.Encoding) {
		return 0, false
	}
	if name, ok := m.GlyphNameByCode(code); ok {
		if metric, ok := m.AFM.GlyphMetricByName(name); ok {
			return metric.BBox[0], true
		}
	}
	metric, ok := m.AFM.GlyphMetricByCode(code)
	if !ok {
		return 0, false
	}
	return metric.BBox[0], true
}

// KernX returns the AFM horizontal kerning adjustment for a glyph pair.
//
// It is equivalent to KernXByGlyphName.
func (m *CompanionMetrics) KernX(left, right string) (float64, bool) {
	return m.KernXByGlyphName(left, right)
}

// KernXByGlyphName returns the AFM horizontal kerning adjustment for a named
// glyph pair.
func (m *CompanionMetrics) KernXByGlyphName(left, right string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	return m.AFM.KernX(left, right)
}

// KernXByCode returns the AFM horizontal kerning adjustment for an encoded
// character-code pair.
func (m *CompanionMetrics) KernXByCode(leftCode, rightCode int) (float64, bool) {
	left, ok := m.GlyphNameByCode(leftCode)
	if !ok {
		return 0, false
	}
	right, ok := m.GlyphNameByCode(rightCode)
	if !ok {
		return 0, false
	}
	return m.KernXByGlyphName(left, right)
}

func (m *CompanionMetrics) pfmWidthByCode(code int) (float64, bool) {
	width, ok := m.PFM.WidthByCode(code)
	if !ok {
		return 0, false
	}
	return float64(width), true
}
