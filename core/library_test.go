package core

import (
	"errors"
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestLibraryRegistersModulesAndReturnsSnapshots(t *testing.T) {
	lib := NewLibrary()
	module := struct{}{}
	driver := &coreTestDriver{handles: true, face: &coreTestFace{}}

	lib.AddModule(nil)
	lib.AddModule(module)
	lib.AddDriver(driver)

	modules := lib.Modules()
	if len(modules) != 2 {
		t.Fatalf("modules = %d, want 2", len(modules))
	}
	modules[0] = nil
	if lib.Modules()[0] == nil {
		t.Fatal("Modules returned mutable library storage")
	}

	drivers := lib.Drivers()
	if len(drivers) != 1 {
		t.Fatalf("drivers = %d, want 1", len(drivers))
	}
	drivers[0] = nil
	if lib.Drivers()[0] == nil {
		t.Fatal("Drivers returned mutable library storage")
	}
}

func TestNilLibrarySnapshotsAreNil(t *testing.T) {
	var lib *Library
	if lib.Modules() != nil {
		t.Fatal("nil library returned modules")
	}
	if lib.Drivers() != nil {
		t.Fatal("nil library returned drivers")
	}
}

func TestLibraryLoadFaceErrorCodes(t *testing.T) {
	var nilLib *Library
	if _, err := nilLib.LoadFace(NewMemoryStream([]byte{1})); !errors.Is(err, ErrInvalidLibrary) {
		t.Fatalf("nil library err = %v, want ErrInvalidLibrary", err)
	} else if got := api.ErrorToCode(err); got != api.FT_Err_Invalid_Library_Handle {
		t.Fatalf("nil library code = %#x, want %#x", got, api.FT_Err_Invalid_Library_Handle)
	}

	lib := NewLibrary()
	if _, err := lib.LoadFace(nil); !errors.Is(err, ErrInvalidStream) {
		t.Fatalf("nil stream err = %v, want ErrInvalidStream", err)
	} else if got := api.ErrorToCode(err); got != api.FT_Err_Invalid_Stream_Handle {
		t.Fatalf("nil stream code = %#x, want %#x", got, api.FT_Err_Invalid_Stream_Handle)
	}

	lib.AddDriver(nil)
	lib.AddDriver(&coreTestDriver{handles: false})
	if _, err := lib.LoadFace(NewMemoryStream([]byte{1})); !errors.Is(err, ErrUnknownFontFormat) {
		t.Fatalf("unknown format err = %v, want ErrUnknownFontFormat", err)
	} else if got := api.ErrorToCode(err); got != api.FT_Err_Unknown_File_Format {
		t.Fatalf("unknown format code = %#x, want %#x", got, api.FT_Err_Unknown_File_Format)
	}
}

func TestLibraryLoadFaceUsesFirstHandlingDriver(t *testing.T) {
	face := &coreTestFace{}
	skip := &coreTestDriver{handles: false}
	match := &coreTestDriver{handles: true, face: face}
	lib := NewLibrary()
	lib.AddDriver(skip)
	lib.AddDriver(match)

	got, err := lib.LoadFace(NewMemoryStream([]byte{1}))
	if err != nil {
		t.Fatalf("LoadFace failed: %v", err)
	}
	if got != face {
		t.Fatal("LoadFace returned wrong face")
	}
	if skip.loads != 0 {
		t.Fatalf("non-handling driver loads = %d, want 0", skip.loads)
	}
	if match.loads != 1 {
		t.Fatalf("matching driver loads = %d, want 1", match.loads)
	}
}

type coreTestDriver struct {
	handles bool
	face    api.Face
	loads   int
}

func (d *coreTestDriver) Handles(stream api.Stream) bool {
	return d.handles
}

func (d *coreTestDriver) LoadFace(stream api.Stream) (api.Face, error) {
	d.loads++
	return d.face, nil
}

type coreTestFace struct{}

func (*coreTestFace) GetNumGlyphs() int { return 0 }
func (*coreTestFace) SetPixelSizes(width, height int) error {
	return nil
}
func (*coreTestFace) LoadGlyph(glyphIndex int, loadFlags int) (api.GlyphSlot, error) {
	return nil, nil
}
func (*coreTestFace) GetGlyphSlot() api.GlyphSlot { return nil }
func (*coreTestFace) GetUnitsPerEm() uint16       { return 0 }
func (*coreTestFace) GetGlyphIndex(char rune) (int, error) {
	return 0, nil
}
func (*coreTestFace) GetGlyphMetrics(glyphIndex int) (advance int32, lsb int32, err error) {
	return 0, 0, nil
}
func (*coreTestFace) Shape(text string) ([]int, []api.Vector) {
	return nil, nil
}
