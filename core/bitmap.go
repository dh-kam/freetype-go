package core

// Bitmap implements api.Bitmap.
type Bitmap struct {
	Rows      int
	Width     int
	Pitch     int
	Buffer    []byte
	PixelMode uint8
}

func (b *Bitmap) GetRows() int {
	return b.Rows
}

func (b *Bitmap) GetWidth() int {
	return b.Width
}

func (b *Bitmap) GetPitch() int {
	return b.Pitch
}

func (b *Bitmap) GetBuffer() []byte {
	return b.Buffer
}

func (b *Bitmap) GetPixelMode() uint8 {
	return b.PixelMode
}

func (b *Bitmap) SetPixelMode(mode uint8) {
	b.PixelMode = mode
}

// NewBitmap creates a new Bitmap and allocates its buffer.
func NewBitmap(width, rows int) *Bitmap {
	pitch := width
	return &Bitmap{
		Rows:   rows,
		Width:  width,
		Pitch:  pitch,
		Buffer: make([]byte, pitch*rows),
	}
}
