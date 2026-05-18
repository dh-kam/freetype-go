package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

func TestRunValidatesCharacterAndFontPath(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-char", ""}, &out); err == nil || !strings.Contains(err.Error(), "provide a character") {
		t.Fatalf("run empty char err = %v, want character validation", err)
	}
	if out.Len() != 0 {
		t.Fatalf("empty char wrote output %q, want none", out.String())
	}

	missingFont := filepath.Join(t.TempDir(), "missing.ttf")
	err := run([]string{"-font", missingFont, "-char", "A"}, &out)
	if err == nil {
		t.Fatal("run missing font returned nil error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing font err = %v, want os.ErrNotExist", err)
	}
	if !strings.Contains(out.String(), "Loading font: "+missingFont) {
		t.Fatalf("missing font output = %q, want loading line", out.String())
	}
}

func TestGetBBoxHandlesOutlineBounds(t *testing.T) {
	if minX, minY, maxX, maxY := getBBox(nil); minX != 0 || minY != 0 || maxX != 0 || maxY != 0 {
		t.Fatalf("nil bbox = %d,%d,%d,%d, want zeros", minX, minY, maxX, maxY)
	}

	outline := &core.Outline{
		Points: []api.Vector{
			{X: 10, Y: 20},
			{X: -5, Y: 30},
			{X: 100, Y: -40},
		},
		Tags:     []byte{1, 1, 1},
		Contours: []int{1},
	}
	minX, minY, maxX, maxY := getBBox(outline)
	if minX != -5 || minY != 20 || maxX != 10 || maxY != 30 {
		t.Fatalf("bbox first contour = %d,%d,%d,%d, want -5,20,10,30", minX, minY, maxX, maxY)
	}

	outline.Contours = []int{99}
	minX, minY, maxX, maxY = getBBox(outline)
	if minX != -5 || minY != -40 || maxX != 100 || maxY != 30 {
		t.Fatalf("bbox clamped contour = %d,%d,%d,%d, want -5,-40,100,30", minX, minY, maxX, maxY)
	}

	outline.Contours = []int{-1}
	minX, minY, maxX, maxY = getBBox(outline)
	if minX != 0 || minY != 0 || maxX != 0 || maxY != 0 {
		t.Fatalf("negative contour bbox = %d,%d,%d,%d, want zeros", minX, minY, maxX, maxY)
	}
}

func TestPrintASCIISkipsBlankRowsAndDoublesColumns(t *testing.T) {
	bitmap := &core.Bitmap{
		Rows:      2,
		Width:     3,
		Pitch:     3,
		Buffer:    []byte{0, 0, 0, 0, 255, 128},
		PixelMode: api.MODE_GRAY,
	}

	var out bytes.Buffer
	printASCII(&out, bitmap)
	if got, want := out.String(), "  @@==\n"; got != want {
		t.Fatalf("ASCII output = %q, want %q", got, want)
	}

	out.Reset()
	printASCII(&out, nil)
	if out.Len() != 0 {
		t.Fatalf("nil bitmap output = %q, want none", out.String())
	}
}
