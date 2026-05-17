package core

import (
	"errors"

	"github.com/dh-kam/freetype-go/api"
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
	lib.modules = append(lib.modules, m)
	if d, ok := m.(api.Driver); ok {
		lib.drivers = append(lib.drivers, d)
	}
}

// LoadFace iterates through registered drivers to find one that can handle the stream,
// and then uses it to load the font face.
func (lib *Library) LoadFace(stream api.Stream) (api.Face, error) {
	for _, d := range lib.drivers {
		if d.Handles(stream) {
			return d.LoadFace(stream)
		}
	}
	return nil, errors.New("unknown font format")
}
