package api_test

import (
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

type systemTestRasterizer struct{}

func (systemTestRasterizer) Render(outline api.Outline, bitmap api.Bitmap) error {
	return nil
}

func (systemTestRasterizer) SetLCDFilter(filterType int) {}

type systemTestHinter struct{}

func (systemTestHinter) Hint(outline api.Outline, size int32) error {
	return nil
}

func TestSystemCanExposeConfiguredServices(t *testing.T) {
	rasterizer := systemTestRasterizer{}
	hinter := systemTestHinter{}
	sys := core.NewSystemWithServices(nil, rasterizer, hinter)

	if sys.Math() == nil {
		t.Fatal("Math() returned nil")
	}
	if sys.Rasterizer() != rasterizer {
		t.Fatal("Rasterizer() did not return configured service")
	}
	if sys.Hinter() != hinter {
		t.Fatal("Hinter() did not return configured service")
	}

	sys.SetRasterizer(nil)
	sys.SetHinter(nil)
	if sys.Rasterizer() != nil || sys.Hinter() != nil {
		t.Fatal("services were not cleared")
	}
}
