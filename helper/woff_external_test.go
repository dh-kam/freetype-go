package helper

import (
	"os"
	"testing"

	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/sfnt"
)

func TestWOFF2ExternalSample(t *testing.T) {
	path := os.Getenv("FTGO_WOFF2_SAMPLE")
	if path == "" {
		t.Skip("set FTGO_WOFF2_SAMPLE to run an external WOFF2 smoke test")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read WOFF2 sample: %v", err)
	}
	decoded, err := DecodeWOFF2(core.NewMemoryStream(data))
	if err != nil {
		t.Fatalf("DecodeWOFF2 failed: %v", err)
	}

	loader := sfnt.NewLoader(core.NewSystem())
	face, err := loader.LoadFace(decoded)
	if err != nil {
		t.Fatalf("decoded SFNT failed to load: %v", err)
	}
	if face.GetNumGlyphs() == 0 {
		t.Fatal("decoded WOFF2 sample has no glyphs")
	}
}
