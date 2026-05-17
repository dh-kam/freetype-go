package main

import (
	"flag"
	"fmt"
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
	fontPath := flag.String("font", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", "Path to TTF/OTF/TTC/WOFF/WOFF2 font file")
	charToRender := flag.String("char", "G", "Character to render")
	size := flag.Int("size", 24, "Font size in pixels")
	faceIndex := flag.Int("face-index", 0, "Face index for TTC/OTC collections")
	flag.Parse()

	if len(*charToRender) == 0 {
		log.Fatal("Please provide a character to render")
	}
	r := []rune(*charToRender)[0]

	fmt.Printf("Loading font: %s\n", *fontPath)
	f, err := os.Open(*fontPath)
	if err != nil {
		log.Fatalf("Failed to open font file: %v", err)
	}
	defer f.Close()

	fileStream, err := core.NewFileStream(f)
	if err != nil {
		log.Fatalf("Failed to create font stream: %v", err)
	}
	var stream api.Stream = fileStream
	stream, err = helper.DecodeWOFFIfNeeded(stream)
	if err != nil {
		log.Fatalf("Failed to decode WOFF container: %v", err)
	}
	sys := core.NewSystem()
	face, err := sfnt.LoadFaceIndex(sys, stream, *faceIndex)
	if err != nil {
		log.Fatalf("Failed to load face: %v", err)
	}
	if err := face.SetPixelSizes(*size, *size); err != nil {
		log.Fatalf("Invalid pixel size: %v", err)
	}

	fmt.Printf("Looking up glyph index for character '%c'\n", r)
	glyphIndex, err := face.GetGlyphIndex(r)
	if err != nil {
		log.Fatalf("Glyph not found for character '%c': %v", r, err)
	}

	fmt.Printf("Loading glyph index: %d\n", glyphIndex)
	slot, err := face.LoadGlyph(glyphIndex, api.LoadDefault)
	if err != nil {
		log.Fatalf("Failed to load glyph: %v", err)
	}

	advance, lsb, err := face.GetGlyphMetrics(glyphIndex)
	if err == nil {
		fmt.Printf("Metrics: Advance=%d, LSB=%d\n", advance, lsb)
	}

	outline := slot.GetOutline()
	if outline == nil {
		log.Fatal("Outline is empty")
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
		log.Fatalf("Rasterizer failed: %v", err)
	}

	fmt.Printf("\n--- Rendering of '%c' (Size: %d) ---\n\n", r, *size)
	printASCII(bitmap)
}

func getBBox(outline api.Outline) (minX, minY, maxX, maxY int32) {
	points := outline.GetPoints()
	contours := outline.GetContours()
	if len(contours) == 0 {
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

func printASCII(b *core.Bitmap) {
	buf := b.GetBuffer()
	pitch := b.GetPitch()
	rows := b.GetRows()
	width := b.GetWidth()

	shades := []string{" ", ".", ":", "-", "=", "+", "*", "#", "%", "@"}

	for y := 0; y < rows; y++ {
		var line strings.Builder
		emptyLine := true
		for x := 0; x < width; x++ {
			val := buf[y*pitch+x]
			if val > 0 {
				emptyLine = false
			}
			idx := int(val) * (len(shades) - 1) / 255
			line.WriteString(shades[idx])
			line.WriteString(shades[idx]) // Double wide for aspect ratio
		}
		if !emptyLine {
			fmt.Println(line.String())
		}
	}
}
