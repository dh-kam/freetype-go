package type1

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/raster"
)

// loader implements api.Driver for standalone Type 1 PFA/PFB fonts.
type loader struct {
	sys api.FreetypeSystem
}

// NewLoader returns a Type 1 font driver.
func NewLoader(sys api.FreetypeSystem) api.Driver {
	return &loader{sys: sys}
}

// Face adapts a parsed Type 1 font to the common api.Face interface.
type Face struct {
	font       *Font
	sys        api.FreetypeSystem
	glyphSlot  *GlyphSlot
	xPPEM      int
	yPPEM      int
	xScale     int32
	yScale     int32
	unitsPerEm uint16
	glyphIndex map[string]int
	afm        *AFM
	pfm        *PFM
}

// GlyphSlot holds a loaded Type 1 glyph.
type GlyphSlot struct {
	outline        *core.Outline
	bitmap         api.Bitmap
	slotMetrics    api.GlyphMetrics
	hasSlotMetrics bool
}

func (l *loader) Handles(stream api.Stream) bool {
	if stream == nil || stream.Size() <= 0 {
		return false
	}
	var prefix [64]byte
	n, _ := stream.ReadAt(prefix[:], 0)
	if n <= 0 {
		return false
	}
	data := prefix[:n]
	if data[0] == 0x80 {
		return n > 1 && (data[1] == 1 || data[1] == 2)
	}
	return hasType1PFAHeader(data)
}

func (l *loader) LoadFace(stream api.Stream) (api.Face, error) {
	if stream == nil {
		return nil, errors.New("nil Type 1 stream")
	}
	data, err := readType1Stream(stream)
	if err != nil {
		return nil, err
	}
	font, err := ParseFont(data)
	if err != nil {
		return nil, err
	}
	face := newFace(font, l.sys)
	metrics, ok, err := readCompanionMetricsForStream(stream, font.Encoding)
	if err != nil {
		return nil, err
	}
	if ok {
		face.SetCompanionMetrics(metrics)
	}
	return face, nil
}

func newFace(font *Font, sys api.FreetypeSystem) *Face {
	upem := type1UnitsPerEm(font)
	f := &Face{
		font:       font,
		sys:        sys,
		glyphSlot:  &GlyphSlot{},
		xPPEM:      int(upem),
		yPPEM:      int(upem),
		unitsPerEm: upem,
		glyphIndex: make(map[string]int, len(font.GlyphNames)),
	}
	f.updateScales()
	for i, name := range font.GlyphNames {
		if _, exists := f.glyphIndex[name]; !exists {
			f.glyphIndex[name] = i
		}
	}
	return f
}

func (f *Face) GetNumGlyphs() int {
	if f == nil || f.font == nil {
		return 0
	}
	return len(f.font.GlyphNames)
}

// SetAFM attaches Adobe Font Metrics companion data to the face.
func (f *Face) SetAFM(afm *AFM) {
	if f == nil {
		return
	}
	f.afm = afm
}

// SetPFM attaches Windows Printer Font Metrics companion data to the face.
func (f *Face) SetPFM(pfm *PFM) {
	if f == nil {
		return
	}
	f.pfm = pfm
}

// SetCompanionMetrics attaches parsed optional companion metrics to the face.
//
// Nil metrics clear both companion attachments. The face uses its own encoding
// for PFM glyph-name resolution.
func (f *Face) SetCompanionMetrics(metrics *CompanionMetrics) {
	if f == nil {
		return
	}
	if metrics == nil {
		f.SetAFM(nil)
		f.SetPFM(nil)
		return
	}
	f.SetAFM(metrics.AFM)
	f.SetPFM(metrics.PFM)
}

func (f *Face) SetPixelSizes(width, height int) error {
	if width < 0 || height < 0 {
		return errors.New("invalid Type 1 pixel size")
	}
	if width == 0 {
		width = height
	}
	if height == 0 {
		height = width
	}
	if width == 0 || height == 0 {
		return errors.New("invalid Type 1 pixel size")
	}
	f.xPPEM = width
	f.yPPEM = height
	f.updateScales()
	return nil
}

func (f *Face) LoadGlyph(glyphIndex int, loadFlags int) (api.GlyphSlot, error) {
	if f == nil || f.font == nil {
		return nil, errors.New("nil Type 1 face")
	}
	name, err := f.glyphName(glyphIndex)
	if err != nil {
		return nil, err
	}
	result, err := f.font.DecodeGlyph(name)
	if err != nil {
		return nil, err
	}
	if result.SEAC != nil {
		result, err = f.loadSEACGlyph(result, map[string]bool{name: true})
		if err != nil {
			return nil, err
		}
	}

	outline := result.Outline
	if outline == nil {
		outline = &core.Outline{}
	}
	if loadFlags&api.LoadNoScale == 0 {
		var hintSnaps []type1HintPointSnap
		if loadFlags&api.LoadNoHinting == 0 {
			hintSnaps = type1HintPointSnaps(outline, BuildHintContext(f.font, result, f.xScale, f.yScale))
		}
		outline.Scale(f.xScale, f.yScale)
		applyType1HintPointSnaps(outline, hintSnaps)
	}

	metrics := f.slotMetrics(name, outline, result, loadFlags)
	bitmap, err := f.renderGlyphBitmap(outline, loadFlags)
	if err != nil {
		return nil, err
	}
	slot := &GlyphSlot{
		outline:        outline,
		bitmap:         bitmap,
		slotMetrics:    metrics,
		hasSlotMetrics: true,
	}
	f.glyphSlot = slot
	return slot, nil
}

func (f *Face) GetGlyphSlot() api.GlyphSlot {
	if f == nil {
		return nil
	}
	return f.glyphSlot
}

func (f *Face) GetUnitsPerEm() uint16 {
	if f == nil || f.unitsPerEm == 0 {
		return 1000
	}
	return f.unitsPerEm
}

func (f *Face) GetGlyphIndex(char rune) (int, error) {
	if f == nil || f.font == nil {
		return 0, errors.New("nil Type 1 face")
	}
	if char < 0 || char >= 256 {
		return 0, nil
	}
	name := f.font.GlyphName(int(char))
	if name == "" {
		return 0, nil
	}
	idx, ok := f.glyphIndex[name]
	if !ok {
		return 0, nil
	}
	return idx, nil
}

func (f *Face) GetGlyphMetrics(glyphIndex int) (advance int32, lsb int32, err error) {
	if f == nil || f.font == nil {
		return 0, 0, errors.New("nil Type 1 face")
	}
	name, err := f.glyphName(glyphIndex)
	if err != nil {
		return 0, 0, err
	}
	sideBearing, width, _, err := f.font.DecodeGlyphMetrics(name)
	if err != nil {
		return 0, 0, err
	}
	sideBearing, width, _ = f.companionHorizontalMetrics(name, sideBearing, width)
	return f.scale26Dot6X(width.X), f.scale26Dot6X(sideBearing.X), nil
}

func (f *Face) Shape(text string) ([]int, []api.Vector) {
	glyphs := make([]int, 0, len(text))
	positions := make([]api.Vector, 0, len(text))
	glyphNames := make([]string, 0, len(text))
	for _, r := range text {
		gid, _ := f.GetGlyphIndex(r)
		glyphs = append(glyphs, gid)
		name := ""
		if f != nil && f.font != nil && gid > 0 {
			if glyphName, err := f.glyphName(gid); err == nil {
				name = glyphName
			}
		}
		glyphNames = append(glyphNames, name)
		advance, _, _ := f.GetGlyphMetrics(gid)
		positions = append(positions, api.Vector{X: advance})
	}
	f.applyCompanionKerning(glyphNames, positions)
	return glyphs, positions
}

func (f *Face) applyCompanionKerning(glyphNames []string, positions []api.Vector) {
	metrics := f.companionMetrics()
	if metrics == nil {
		return
	}
	for i := 1; i < len(glyphNames) && i < len(positions); i++ {
		left, right := glyphNames[i-1], glyphNames[i]
		if left == "" || right == "" {
			continue
		}
		kern, ok := metrics.KernXByGlyphName(left, right)
		if !ok {
			continue
		}
		fixed, ok := type1DesignUnitsTo26Dot6(kern)
		if !ok {
			continue
		}
		positions[i-1].X += f.scale26Dot6X(fixed)
	}
}

func (gs *GlyphSlot) GetOutline() api.Outline {
	if gs == nil {
		return nil
	}
	return gs.outline
}

func (gs *GlyphSlot) SetOutline(outline api.Outline) {
	if o, ok := outline.(*core.Outline); ok {
		gs.outline = o
	}
}

func (gs *GlyphSlot) GetBitmap() api.Bitmap {
	if gs == nil {
		return nil
	}
	return gs.bitmap
}

func (gs *GlyphSlot) GetImage() *api.Image {
	return nil
}

func (gs *GlyphSlot) GetMetrics() (api.GlyphMetrics, bool) {
	if gs == nil || !gs.hasSlotMetrics {
		return api.GlyphMetrics{}, false
	}
	return gs.slotMetrics, true
}

func (f *Face) glyphName(glyphIndex int) (string, error) {
	if glyphIndex < 0 || glyphIndex >= len(f.font.GlyphNames) {
		return "", errors.New("invalid Type 1 glyph index")
	}
	return f.font.GlyphNames[glyphIndex], nil
}

func (f *Face) GetGlyphName(glyphIndex int) (string, bool) {
	if f == nil {
		return "", false
	}
	name, err := f.glyphName(glyphIndex)
	return name, err == nil && name != ""
}

func (f *Face) loadGlyphResultByName(name string, active map[string]bool) (*CharStringResult, error) {
	if name == "" {
		return nil, errors.New("empty Type 1 glyph name")
	}
	if active[name] {
		return nil, fmt.Errorf("nested Type 1 SEAC cycle through glyph %q", name)
	}
	active[name] = true
	defer delete(active, name)

	result, err := f.font.DecodeGlyph(name)
	if err != nil {
		return nil, err
	}
	if result.Outline == nil {
		result.Outline = &core.Outline{}
	}
	if result.SEAC == nil {
		return result, nil
	}
	return f.loadSEACGlyph(result, active)
}

func (f *Face) loadSEACGlyph(result *CharStringResult, active map[string]bool) (*CharStringResult, error) {
	if result == nil || result.SEAC == nil {
		return result, nil
	}
	seac := result.SEAC
	baseName := StandardEncodingGlyphName(int(seac.BaseChar))
	if baseName == "" {
		return nil, fmt.Errorf("Type 1 SEAC base character code %d has no StandardEncoding glyph", seac.BaseChar)
	}
	accentName := StandardEncodingGlyphName(int(seac.AccentChar))
	if accentName == "" {
		return nil, fmt.Errorf("Type 1 SEAC accent character code %d has no StandardEncoding glyph", seac.AccentChar)
	}

	base, err := f.loadGlyphResultByName(baseName, active)
	if err != nil {
		return nil, err
	}
	accent, err := f.loadGlyphResultByName(accentName, active)
	if err != nil {
		return nil, err
	}

	outline := cloneType1Outline(base.Outline)
	accentOutline := cloneType1Outline(accent.Outline)

	sideBearing := result.SideBearing
	width := result.Width
	if width == (api.Vector{}) && sideBearing == (api.Vector{}) && base.Width != (api.Vector{}) {
		sideBearing = base.SideBearing
		width = base.Width
	}
	accentOutline.Translate(sideBearing.X+seac.ADX*64-seac.ASB*64, seac.ADY*64)
	appendType1Outline(outline, accentOutline)

	return &CharStringResult{
		Outline:     outline,
		SideBearing: sideBearing,
		Width:       width,
		SEAC:        seac,
	}, nil
}

func cloneType1Outline(outline *core.Outline) *core.Outline {
	if outline == nil {
		return &core.Outline{}
	}
	return &core.Outline{
		Points:   append([]api.Vector(nil), outline.Points...),
		Tags:     append([]byte(nil), outline.Tags...),
		Contours: append([]int(nil), outline.Contours...),
	}
}

func appendType1Outline(dst, src *core.Outline) {
	if dst == nil || src == nil || len(src.Points) == 0 {
		return
	}
	pointOffset := len(dst.Points)
	dst.Points = append(dst.Points, src.Points...)
	dst.Tags = append(dst.Tags, src.Tags...)
	for _, contour := range src.Contours {
		dst.Contours = append(dst.Contours, contour+pointOffset)
	}
}

func (f *Face) updateScales() {
	upem := int32(f.GetUnitsPerEm())
	if upem <= 0 {
		upem = 1000
	}
	f.xScale = int32((int64(f.xPPEM) << 16) / int64(upem))
	f.yScale = int32((int64(f.yPPEM) << 16) / int64(upem))
}

func (f *Face) scale26Dot6X(v int32) int32 {
	return int32((int64(v) * int64(f.xScale)) >> 16)
}

func (f *Face) scale26Dot6Y(v int32) int32 {
	return int32((int64(v) * int64(f.yScale)) >> 16)
}

func (f *Face) slotMetrics(name string, outline *core.Outline, result *CharStringResult, loadFlags int) api.GlyphMetrics {
	sideBearing := result.SideBearing
	width := result.Width
	sideBearing, width, explicitLSB := f.companionHorizontalMetrics(name, sideBearing, width)
	if loadFlags&api.LoadNoScale == 0 {
		sideBearing.X = f.scale26Dot6X(sideBearing.X)
		sideBearing.Y = f.scale26Dot6Y(sideBearing.Y)
		width.X = f.scale26Dot6X(width.X)
		width.Y = f.scale26Dot6Y(width.Y)
	}

	metrics := api.GlyphMetrics{
		HoriBearingX: sideBearing.X,
		HoriAdvance:  width.X,
	}
	if len(outline.Points) > 0 {
		minX, minY, maxX, maxY := outlineBounds(outline)
		metrics.Width = maxX - minX
		metrics.Height = maxY - minY
		if sideBearing.X == 0 && !explicitLSB {
			metrics.HoriBearingX = minX
		}
		metrics.HoriBearingY = maxY
	}
	return metrics
}

func (f *Face) companionHorizontalMetrics(name string, sideBearing, width api.Vector) (api.Vector, api.Vector, bool) {
	metrics := f.companionMetrics()
	if metrics == nil {
		return sideBearing, width, false
	}

	if advance, ok := metrics.WidthByGlyphName(name); ok {
		if fixed, ok := type1DesignUnitsTo26Dot6(advance); ok {
			width.X = fixed
		}
	}

	explicitLSB := false
	if lsb, ok := metrics.LeftSideBearingByGlyphName(name); ok {
		if fixed, ok := type1DesignUnitsTo26Dot6(lsb); ok {
			sideBearing.X = fixed
			explicitLSB = true
		}
	}
	return sideBearing, width, explicitLSB
}

func (f *Face) companionMetrics() *CompanionMetrics {
	if f == nil || f.font == nil || (f.afm == nil && f.pfm == nil) {
		return nil
	}
	return &CompanionMetrics{
		AFM:      f.afm,
		PFM:      f.pfm,
		Encoding: f.font.Encoding,
	}
}

func type1DesignUnitsTo26Dot6(value float64) (int32, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	value *= 64
	const (
		minInt32 = -1 << 31
		maxInt32 = 1<<31 - 1
	)
	if value < minInt32 || value > maxInt32 {
		return 0, false
	}
	return int32(value), true
}

func (f *Face) renderGlyphBitmap(outline *core.Outline, loadFlags int) (api.Bitmap, error) {
	if loadFlags&api.LoadRender == 0 {
		return nil, nil
	}
	renderOutline, bitmap, _, ok := core.PrepareBitmapForOutline(outline, -1, renderModeForLoadFlags(loadFlags))
	if !ok || renderOutline == nil {
		return bitmap, nil
	}
	var rast api.Rasterizer
	if f.sys != nil {
		rast = f.sys.Rasterizer()
	}
	if rast == nil {
		rast = raster.NewSmoothRasterizer()
	}
	if err := rast.Render(renderOutline, bitmap); err != nil {
		return nil, err
	}
	return bitmap, nil
}

func renderModeForLoadFlags(loadFlags int) api.RenderMode {
	if loadFlags&api.LoadMonochrome != 0 {
		return api.RenderModeMono
	}
	switch loadFlags & api.LoadTargetMask {
	case api.LoadTargetLight:
		return api.RenderModeLight
	case api.LoadTargetMono:
		return api.RenderModeMono
	case api.LoadTargetLCD:
		return api.RenderModeLCD
	case api.LoadTargetLCDV:
		return api.RenderModeLCDV
	}
	return api.RenderModeNormal
}

func outlineBounds(outline *core.Outline) (minX, minY, maxX, maxY int32) {
	for i, p := range outline.Points {
		if i == 0 || p.X < minX {
			minX = p.X
		}
		if i == 0 || p.Y < minY {
			minY = p.Y
		}
		if i == 0 || p.X > maxX {
			maxX = p.X
		}
		if i == 0 || p.Y > maxY {
			maxY = p.Y
		}
	}
	return minX, minY, maxX, maxY
}

func type1UnitsPerEm(font *Font) uint16 {
	if font == nil {
		return 1000
	}
	scale := math.Abs(font.FontMatrix[0])
	if scale == 0 {
		scale = math.Abs(font.FontMatrix[3])
	}
	if scale == 0 {
		return 1000
	}
	upem := int(math.Round(1 / scale))
	if upem <= 0 {
		return 1000
	}
	if upem > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(upem)
}

func readType1Stream(stream api.Stream) ([]byte, error) {
	size := stream.Size()
	if size < 0 {
		return nil, errors.New("invalid Type 1 stream size")
	}
	data := make([]byte, size)
	n, err := stream.ReadAt(data, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return data[:n], nil
}

func hasType1PFAHeader(data []byte) bool {
	if len(data) < 2 || data[0] != '%' || data[1] != '!' {
		return false
	}
	header := string(data)
	return containsAny(header, []string{
		"PS-AdobeFont-1.0",
		"FontType1",
		"PS-Adobe-3.0 Resource-Font",
	})
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if len(needle) > 0 && strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

var _ api.Driver = (*loader)(nil)
var _ api.Face = (*Face)(nil)
var _ api.GlyphSlotMetricsProvider = (*GlyphSlot)(nil)
