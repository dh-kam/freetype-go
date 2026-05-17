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
	GetGlyphMetrics(glyphIndex int) (advance int32, lsb int32, err error)
	Shape(text string) ([]int, []Vector)
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
