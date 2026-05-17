package api

// --- Core Data Structures (Abstracted) ---

// Face represents a typeface object.
type Face interface {
	GetNumGlyphs() int
	SetPixelSizes(width, height int) error
	LoadGlyph(glyphIndex int, loadFlags int) (GlyphSlot, error)
	GetGlyphSlot() GlyphSlot
	GetUnitsPerEm() uint16
	GetGlyphIndex(char rune) (int, error)
	// GetGlyphMetrics returns horizontal metrics in 26.6 pixel units.
	GetGlyphMetrics(glyphIndex int) (advance int32, lsb int32, err error)
	Shape(text string) ([]int, []Vector)
}

// LoadFlag values are passed to Face.LoadGlyph. The first two values are the
// historical Go API flags; later values model FreeType load concepts for API
// and conformance plumbing even when a driver only implements a subset.
type LoadFlag = int

const (
	LoadDefault   = 0
	LoadNoHinting = 1 << 0

	LoadNoScale                  = 1 << 1
	LoadRender                   = 1 << 2
	LoadNoBitmap                 = 1 << 3
	LoadVerticalLayout           = 1 << 4
	LoadForceAutohint            = 1 << 5
	LoadCropBitmap               = 1 << 6
	LoadPedantic                 = 1 << 7
	LoadIgnoreGlobalAdvanceWidth = 1 << 8
	LoadNoRecurse                = 1 << 9
	LoadIgnoreTransform          = 1 << 10
	LoadMonochrome               = 1 << 11
	LoadLinearDesign             = 1 << 12
	LoadNoAutohint               = 1 << 13
	LoadColor                    = 1 << 14
	LoadComputeMetrics           = 1 << 15
	LoadBitmapMetricsOnly        = 1 << 20
	LoadNoSVG                    = 1 << 24

	LoadTargetNormal = 0
	LoadTargetLight  = 1 << 16
	LoadTargetMono   = 2 << 16
	LoadTargetLCD    = 3 << 16
	LoadTargetLCDV   = 4 << 16
	LoadTargetMask   = 15 << 16
)

// RenderMode mirrors FreeType's glyph render modes. RenderModeNone is a Go
// sentinel used by conformance tooling when only glyph loading is requested.
type RenderMode int

const (
	RenderModeNone RenderMode = -1
)

const (
	RenderModeNormal RenderMode = iota
	RenderModeLight
	RenderModeMono
	RenderModeLCD
	RenderModeLCDV
)

// GlyphMetrics mirrors the FreeType FT_Glyph_Metrics fields in 26.6 pixel
// units after glyph load and hinting.
type GlyphMetrics struct {
	Width        int32
	Height       int32
	HoriBearingX int32
	HoriBearingY int32
	HoriAdvance  int32
	VertBearingX int32
	VertBearingY int32
	VertAdvance  int32
}

// GlyphSlotMetricsProvider is optional. Implementations can expose loaded slot
// metrics without changing the stable GlyphSlot interface.
type GlyphSlotMetricsProvider interface {
	GetMetrics() (GlyphMetrics, bool)
}

// GetGlyphSlotMetrics returns loaded glyph metrics when a slot implementation
// exposes them through GlyphSlotMetricsProvider.
func GetGlyphSlotMetrics(slot GlyphSlot) (GlyphMetrics, bool) {
	if slot == nil {
		return GlyphMetrics{}, false
	}
	provider, ok := slot.(GlyphSlotMetricsProvider)
	if !ok {
		return GlyphMetrics{}, false
	}
	return provider.GetMetrics()
}

// GlyphSlot holds the currently loaded glyph.
type GlyphSlot interface {
	GetOutline() Outline
	SetOutline(Outline)
	GetBitmap() Bitmap
	GetImage() *Image
}

// Outline represents the vector outline of a glyph.
type Outline interface {
	GetPoints() []Vector
	GetTags() []byte
	GetContours() []int
	Scale(xScale, yScale int32)
	Translate(x, y int32)
	Transform(matrix *Matrix)
}

// Matrix represents a 2x2 affine transform in 16.16 fixed point format.
type Matrix struct {
	XX, XY int32
	YX, YY int32
}

// Bitmap represents a rendered pixel grid.
type Bitmap interface {
	GetRows() int
	GetWidth() int
	GetPitch() int
	GetBuffer() []byte
	GetPixelMode() uint8
	SetPixelMode(mode uint8)
}

const (
	MODE_NONE uint8 = iota
	MODE_MONO       // 1-bit per pixel
	MODE_GRAY       // 8-bit per pixel (standard smooth)
	MODE_LCD        // 3x horizontal resolution for LCD
)

const (
	LCD_FILTER_DEFAULT int = iota
	LCD_FILTER_LIGHT
	LCD_FILTER_LEGACY
	LCD_FILTER_NONE
)

// Vector represents a 2D point, usually in 26.6 fixed-point format.
type Vector struct {
	X int32
	Y int32
}

// --- Minimum Module Interfaces ---

// MathEngine provides high-precision fixed-point math operations.
type MathEngine interface {
	MulFix(a, b int32) int32
	DivFix(a, b int32) int32
	Cos(angle int32) int32
	Sin(angle int32) int32
}

// Rasterizer converts vector outlines to bitmaps.
type Rasterizer interface {
	Render(outline Outline, bitmap Bitmap) error
	SetLCDFilter(filterType int)
}

// Module represents a generic FreeType module.
type Module interface {
}

// Driver represents a font driver module capable of handling specific font formats.
type Driver interface {
	Module
	LoadFace(stream Stream) (Face, error)
	Handles(stream Stream) bool
}

// Hinter applies grid-fitting to outlines.
type Hinter interface {
	Hint(outline Outline, size int32) error
}

// Stream abstracts file or memory reading.
type Stream interface {
	ReadAt(p []byte, off int64) (n int, err error)
	Size() int64
}

// --- Image Interfaces ---

// Image represents a decoded image payload.
type Image struct {
	Width, Height int
	Pixels        []byte
}

// ImageDecoder decodes a raw byte slice into an Image.
type ImageDecoder interface {
	Decode(data []byte) (*Image, error)
}

// --- Aggregated System Interface ---

// FreetypeSystem aggregates the module interfaces to act as the main communication hub.
// Modules will receive this interface to access services they don't implement directly.
type FreetypeSystem interface {
	Math() MathEngine
	Rasterizer() Rasterizer
	Hinter() Hinter
	SetImageDecoder(dec ImageDecoder)
	GetImageDecoder() ImageDecoder
}
