package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/helper"
	"github.com/dh-kam/freetype-go/raster"
	"github.com/dh-kam/freetype-go/sfnt"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("ftgo", flag.ContinueOnError)
	fs.SetOutput(out)
	fontPath := fs.String("font", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", "Path to TTF/OTF/TTC/WOFF/WOFF2 font file")
	charToRender := fs.String("char", "G", "Character to render")
	size := fs.Int("size", 24, "Font size in pixels")
	faceIndex := fs.Int("face-index", 0, "Face index for TTC/OTC collections")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if len(*charToRender) == 0 {
		return fmt.Errorf("please provide a character to render")
	}
	r := []rune(*charToRender)[0]

	fmt.Fprintf(out, "Loading font: %s\n", *fontPath)
	f, err := os.Open(*fontPath)
	if err != nil {
		return fmt.Errorf("failed to open font file: %w", err)
	}
	defer f.Close()

	fileStream, err := core.NewFileStream(f)
	if err != nil {
		return fmt.Errorf("failed to create font stream: %w", err)
	}
	var stream api.Stream = fileStream
	stream, err = helper.DecodeWOFFIfNeeded(stream)
	if err != nil {
		return fmt.Errorf("failed to decode WOFF container: %w", err)
	}
	sys := core.NewSystem()
	lib := core.NewLibrary()
	lib.AddDriver(sfnt.NewLoader(sys))

	var face api.Face
	if *faceIndex == 0 {
		face, err = lib.LoadFace(stream)
	} else {
		face, err = sfnt.LoadFaceIndex(sys, stream, *faceIndex)
	}
	if err != nil {
		return fmt.Errorf("failed to load face: %w", err)
	}
	if err := face.SetPixelSizes(*size, *size); err != nil {
		return fmt.Errorf("invalid pixel size: %w", err)
	}

	fmt.Fprintf(out, "Looking up glyph index for character '%c'\n", r)
	glyphIndex, err := face.GetGlyphIndex(r)
	if err != nil {
		return fmt.Errorf("glyph not found for character '%c': %w", r, err)
	}

	fmt.Fprintf(out, "Loading glyph index: %d\n", glyphIndex)
	slot, err := face.LoadGlyph(glyphIndex, api.LoadDefault)
	if err != nil {
		return fmt.Errorf("failed to load glyph: %w", err)
	}

	advance, lsb, err := face.GetGlyphMetrics(glyphIndex)
	if err == nil {
		fmt.Fprintf(out, "Metrics: Advance=%d, LSB=%d\n", advance, lsb)
	}

	outline := slot.GetOutline()
	if outline == nil {
		return fmt.Errorf("outline is empty")
	}

	// Calculate bounding box after scaling
	minX, minY, maxX, maxY := getBBox(outline)

	// Flip and translate to positive coordinates
	// Standard fonts use Y-up, but bitmaps use Y-down.
	// We'll transform y to maxY - y to flip it and keep it non-negative.
	points := outline.GetPoints()
	for i := range points {
		points[i].X = points[i].X - minX
		points[i].Y = maxY - points[i].Y
	}

	bitmapWidth := int((maxX-minX)>>6) + 1
	bitmapRows := int((maxY-minY)>>6) + 1
	bitmap := core.NewBitmap(bitmapWidth, bitmapRows)

	rast := raster.NewSmoothRasterizer()
	if err := rast.Render(outline, bitmap); err != nil {
		return fmt.Errorf("rasterizer failed: %w", err)
	}

	fmt.Fprintf(out, "\n--- Rendering of '%c' (Size: %d) ---\n\n", r, *size)
	printASCII(out, bitmap)
	return nil
}

func getBBox(outline api.Outline) (minX, minY, maxX, maxY int32) {
	if outline == nil {
		return 0, 0, 0, 0
	}
	points := outline.GetPoints()
	contours := outline.GetContours()
	if len(points) == 0 || len(contours) == 0 {
		return 0, 0, 0, 0
	}
	lastPoint := contours[len(contours)-1]
	if lastPoint < 0 {
		return 0, 0, 0, 0
	}
	if lastPoint >= len(points) {
		lastPoint = len(points) - 1
	}

	minX, maxX = points[0].X, points[0].X
	minY, maxY = points[0].Y, points[0].Y
	for i := 0; i <= lastPoint; i++ {
		p := points[i]
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return
}

func printASCII(w io.Writer, b *core.Bitmap) {
	if b == nil {
		return
	}
	buf := b.GetBuffer()
	pitch := b.GetPitch()
	rows := b.GetRows()
	width := b.GetWidth()

	shades := []string{" ", ".", ":", "-", "=", "+", "*", "#", "%", "@"}

	for y := 0; y < rows; y++ {
		var line strings.Builder
		emptyLine := true
		for x := 0; x < width; x++ {
			offset := y*pitch + x
			if offset < 0 || offset >= len(buf) {
				break
			}
			val := buf[offset]
			if val > 0 {
				emptyLine = false
			}
			idx := int(val) * (len(shades) - 1) / 255
			line.WriteString(shades[idx])
			line.WriteString(shades[idx]) // Double wide for aspect ratio
		}
		if !emptyLine {
			fmt.Fprintln(w, line.String())
		}
	}
}
