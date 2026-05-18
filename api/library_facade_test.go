package api_test

import (
	"errors"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

type libraryTestStream struct {
	data []byte
}

func (s libraryTestStream) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(s.data)) {
		return 0, errors.New("out of bounds")
	}
	return copy(p, s.data[off:]), nil
}

func (s libraryTestStream) Size() int64 {
	return int64(len(s.data))
}

type libraryTestDriver struct {
	magic byte
	face  api.Face
	loads int
}

func (d *libraryTestDriver) Handles(stream api.Stream) bool {
	var buf [1]byte
	n, err := stream.ReadAt(buf[:], 0)
	return err == nil && n == 1 && buf[0] == d.magic
}

func (d *libraryTestDriver) LoadFace(stream api.Stream) (api.Face, error) {
	d.loads++
	return d.face, nil
}

type libraryTestFace struct{}

func (libraryTestFace) GetNumGlyphs() int { return 0 }
func (libraryTestFace) SetPixelSizes(width, height int) error {
	return nil
}
func (libraryTestFace) LoadGlyph(glyphIndex int, loadFlags int) (api.GlyphSlot, error) {
	return nil, nil
}
func (libraryTestFace) GetGlyphSlot() api.GlyphSlot { return nil }
func (libraryTestFace) GetUnitsPerEm() uint16       { return 0 }
func (libraryTestFace) GetGlyphIndex(char rune) (int, error) {
	return 0, nil
}
func (libraryTestFace) GetGlyphMetrics(glyphIndex int) (int32, int32, error) {
	return 0, 0, nil
}
func (libraryTestFace) Shape(text string) ([]int, []api.Vector) {
	return nil, nil
}

func TestLibraryRegistersDriversAndLoadsMatchingFace(t *testing.T) {
	lib := core.NewLibrary()
	face := libraryTestFace{}
	driver := &libraryTestDriver{magic: 0x42, face: face}
	lib.AddDriver(driver)

	if got := len(lib.Modules()); got != 1 {
		t.Fatalf("modules = %d, want 1", got)
	}
	if got := len(lib.Drivers()); got != 1 {
		t.Fatalf("drivers = %d, want 1", got)
	}

	loaded, err := lib.LoadFace(libraryTestStream{data: []byte{0x42}})
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	if loaded != face {
		t.Fatalf("loaded face mismatch")
	}
	if driver.loads != 1 {
		t.Fatalf("driver loads = %d, want 1", driver.loads)
	}
}

func TestLibraryLoadFaceErrorsAreStable(t *testing.T) {
	lib := core.NewLibrary()
	if _, err := lib.LoadFace(nil); !errors.Is(err, core.ErrInvalidStream) {
		t.Fatalf("LoadFace(nil) err = %v, want ErrInvalidStream", err)
	}
	if _, err := lib.LoadFace(libraryTestStream{data: []byte{0}}); !errors.Is(err, core.ErrUnknownFontFormat) {
		t.Fatalf("LoadFace(unknown) err = %v, want ErrUnknownFontFormat", err)
	}
}
