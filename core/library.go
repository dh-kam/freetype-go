package core

import (
	"errors"

	"github.com/dh-kam/freetype-go/api"
)

var (
	ErrInvalidLibrary    = api.NewError(api.FT_Err_Invalid_Library_Handle, errors.New("invalid library"))
	ErrInvalidStream     = api.NewError(api.FT_Err_Invalid_Stream_Handle, errors.New("invalid stream"))
	ErrUnknownFontFormat = api.NewError(api.FT_Err_Unknown_File_Format, errors.New("unknown font format"))
)

// Library acts as the central registry for FreeType modules and font drivers.
type Library struct {
	modules []api.Module
	drivers []api.Driver
}

// NewLibrary creates a new Library registry.
func NewLibrary() *Library {
	return &Library{}
}

// AddModule registers a new module with the library.
func (lib *Library) AddModule(m api.Module) {
	if lib == nil || m == nil {
		return
	}
	lib.modules = append(lib.modules, m)
	if d, ok := m.(api.Driver); ok {
		lib.drivers = append(lib.drivers, d)
	}
}

// AddDriver registers a font driver module with the library.
func (lib *Library) AddDriver(d api.Driver) {
	lib.AddModule(d)
}

// Modules returns a snapshot of registered modules.
func (lib *Library) Modules() []api.Module {
	if lib == nil || len(lib.modules) == 0 {
		return nil
	}
	modules := make([]api.Module, len(lib.modules))
	copy(modules, lib.modules)
	return modules
}

// Drivers returns a snapshot of registered font drivers.
func (lib *Library) Drivers() []api.Driver {
	if lib == nil || len(lib.drivers) == 0 {
		return nil
	}
	drivers := make([]api.Driver, len(lib.drivers))
	copy(drivers, lib.drivers)
	return drivers
}

// LoadFace iterates through registered drivers to find one that can handle the stream,
// and then uses it to load the font face.
func (lib *Library) LoadFace(stream api.Stream) (api.Face, error) {
	if lib == nil {
		return nil, ErrInvalidLibrary
	}
	if stream == nil {
		return nil, ErrInvalidStream
	}
	for _, d := range lib.drivers {
		if d == nil {
			continue
		}
		if d.Handles(stream) {
			return d.LoadFace(stream)
		}
	}
	return nil, ErrUnknownFontFormat
}
