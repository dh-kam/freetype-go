package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/sfnt"
)

const dumpSchema = "ftgo.conformance.dump/v1"

type dumpOptions struct {
	FontPath    string
	OutputPath  string
	RequestPath string
	Corpus      string
	FaceIndex   int
	PPEMs       []SizeSpec
	Glyphs      []int
	Chars       []CharSpec
	LoadFlags   []loadFlagSpec
	RenderModes []renderModeSpec
}

type SizeSpec struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type CharSpec struct {
	Label string
	Rune  rune
}

type loadFlagSpec struct {
	Name       string
	Value      int
	Components []string
}

type renderModeSpec struct {
	Name  string
	Value api.RenderMode
}

type Dump struct {
	Schema  string          `json:"schema"`
	Engine  EngineInfo      `json:"engine"`
	Source  SourceInfo      `json:"source"`
	Request RequestInfo     `json:"request"`
	Face    FaceInfo        `json:"face"`
	Charmap []CharMapRecord `json:"charmap,omitempty"`
	Sizes   []SizeDump      `json:"sizes"`
}

type EngineInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type SourceInfo struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type RequestInfo struct {
	FaceIndex    int      `json:"face_index"`
	PPEM         []string `json:"ppem"`
	Glyphs       []int    `json:"glyphs,omitempty"`
	Chars        []string `json:"chars,omitempty"`
	LoadFlags    string   `json:"load_flags"`
	LoadFlagSets []string `json:"load_flag_sets,omitempty"`
	RenderMode   string   `json:"render_mode,omitempty"`
	RenderModes  []string `json:"render_modes,omitempty"`
	RequestPath  string   `json:"request_path,omitempty"`
	Corpus       string   `json:"corpus,omitempty"`
}

type FaceInfo struct {
	FaceIndex  int    `json:"face_index"`
	NumFaces   int    `json:"num_faces"`
	NumGlyphs  int    `json:"num_glyphs"`
	UnitsPerEm uint16 `json:"units_per_em"`
}

type CharMapRecord struct {
	Char       string `json:"char"`
	GlyphIndex int    `json:"glyph_index"`
	Error      string `json:"error,omitempty"`
}

type SizeDump struct {
	PPEMX      int           `json:"ppem_x"`
	PPEMY      int           `json:"ppem_y"`
	LoadFlags  string        `json:"load_flags"`
	RenderMode string        `json:"render_mode,omitempty"`
	Error      string        `json:"error,omitempty"`
	Glyphs     []GlyphRecord `json:"glyphs"`
}

type GlyphRecord struct {
	GlyphIndex     int                `json:"glyph_index"`
	Chars          []string           `json:"chars,omitempty"`
	Format         string             `json:"format,omitempty"`
	RenderedFormat string             `json:"rendered_format,omitempty"`
	LoadError      string             `json:"load_error,omitempty"`
	RenderError    string             `json:"render_error,omitempty"`
	Metrics        MetricsRecord      `json:"metrics"`
	SlotMetrics    *SlotMetricsRecord `json:"slot_metrics,omitempty"`
	Outline        OutlineRecord      `json:"outline"`
	Bitmap         BitmapRecord       `json:"bitmap"`
	Image          *ImageRecord       `json:"image,omitempty"`
}

type MetricsRecord struct {
	Available bool   `json:"available"`
	Advance   int32  `json:"advance"`
	LSB       int32  `json:"lsb"`
	Error     string `json:"error,omitempty"`
}

type SlotMetricsRecord struct {
	Available    bool   `json:"available"`
	Width        int32  `json:"width,omitempty"`
	Height       int32  `json:"height,omitempty"`
	HoriBearingX int32  `json:"hori_bearing_x,omitempty"`
	HoriBearingY int32  `json:"hori_bearing_y,omitempty"`
	HoriAdvance  int32  `json:"hori_advance,omitempty"`
	VertBearingX int32  `json:"vert_bearing_x,omitempty"`
	VertBearingY int32  `json:"vert_bearing_y,omitempty"`
	VertAdvance  int32  `json:"vert_advance,omitempty"`
	Error        string `json:"error,omitempty"`
}

type OutlineRecord struct {
	Available         bool    `json:"available"`
	PointCount        int     `json:"point_count"`
	ContourCount      int     `json:"contour_count"`
	RawPointCount     int     `json:"raw_point_count"`
	PhantomPointCount int     `json:"phantom_point_count"`
	Points            []Point `json:"points,omitempty"`
	Tags              []int   `json:"tags,omitempty"`
	Contours          []int   `json:"contours,omitempty"`
	PhantomPoints     []Point `json:"phantom_points,omitempty"`
	BBox              *BBox   `json:"bbox,omitempty"`
	Error             string  `json:"error,omitempty"`
}

type Point struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
}

type BBox struct {
	XMin int32 `json:"x_min"`
	YMin int32 `json:"y_min"`
	XMax int32 `json:"x_max"`
	YMax int32 `json:"y_max"`
}

type BitmapRecord struct {
	Available     bool   `json:"available"`
	Rows          int    `json:"rows,omitempty"`
	Width         int    `json:"width,omitempty"`
	Pitch         int    `json:"pitch,omitempty"`
	PixelMode     uint8  `json:"pixel_mode,omitempty"`
	PixelModeName string `json:"pixel_mode_name,omitempty"`
	Left          int    `json:"left,omitempty"`
	Top           int    `json:"top,omitempty"`
	BufferSize    int    `json:"buffer_size,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	Error         string `json:"error,omitempty"`
}

type ImageRecord struct {
	Available bool   `json:"available"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Pixels    int    `json:"pixels,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

type glyphSelection struct {
	GlyphIndex int
	Chars      []string
}

type fileStream struct {
	file *os.File
	size int64
}

func (s *fileStream) ReadAt(p []byte, off int64) (int, error) {
	return s.file.ReadAt(p, off)
}

func (s *fileStream) Size() int64 {
	return s.size
}

func runGoDumpCommand(args []string, stdout, stderr io.Writer) error {
	opts, err := parseDumpOptions("dump", args, stdout)
	if err != nil {
		if isHelp(err) {
			return nil
		}
		return err
	}

	dump, err := buildGoDump(opts)
	if err != nil {
		return err
	}
	return writeDump(dump, opts.OutputPath, stdout)
}

func runFreeTypeDumpCommand(args []string, stdout, stderr io.Writer) error {
	opts, err := parseDumpOptions("ftdump", args, stdout)
	if err != nil {
		if isHelp(err) {
			return nil
		}
		return err
	}

	dump, err := buildFreeTypeDump(opts)
	if err != nil {
		return err
	}
	return writeDump(dump, opts.OutputPath, stdout)
}

func parseDumpOptions(name string, args []string, output io.Writer) (dumpOptions, error) {
	var opts dumpOptions
	var ppemList string
	var glyphList string
	var charList string
	var loadFlagList string
	var renderModeList string

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&opts.FontPath, "font", "", "path to a TTF, OTF, or TTC font")
	fs.StringVar(&opts.OutputPath, "out", "-", "output JSON path, or '-' for stdout")
	fs.StringVar(&opts.RequestPath, "request", "", "optional JSON request file for font, ppem, glyph, char, load flag, and render mode selections")
	fs.StringVar(&opts.Corpus, "corpus", "", "optional corpus label recorded in the dump request")
	fs.IntVar(&opts.FaceIndex, "face", 0, "face index for TTC collections")
	fs.StringVar(&ppemList, "ppem", "12,16,24", "comma-separated ppem list, e.g. 12,16x20,24")
	fs.StringVar(&glyphList, "glyphs", "0", "comma-separated glyph IDs and ranges, e.g. 0,3,10-12")
	fs.StringVar(&charList, "chars", "", "comma-separated chars or codepoints, e.g. U+0041,U+0061")
	fs.StringVar(&loadFlagList, "load-flags", "no-hinting", "comma-separated load flag sets, e.g. default,no-hinting+target-light,no-bitmap+target-mono")
	fs.StringVar(&renderModeList, "render-mode", "none", "comma-separated render modes: none, normal, light, mono, lcd, lcd-v")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	visited := visitedFlags(fs)
	if opts.RequestPath != "" {
		request, err := readDumpRequest(opts.RequestPath)
		if err != nil {
			return opts, err
		}
		applyDumpRequest(&opts, request, visited, &ppemList, &glyphList, &charList, &loadFlagList, &renderModeList)
	}
	if opts.FontPath == "" {
		return opts, errors.New("missing required -font")
	}

	ppems, err := parsePPEMList(ppemList)
	if err != nil {
		return opts, err
	}
	glyphs, err := parseGlyphList(glyphList)
	if err != nil {
		return opts, err
	}
	chars, err := parseCharList(charList)
	if err != nil {
		return opts, err
	}
	loadFlags, err := parseLoadFlagSets(loadFlagList)
	if err != nil {
		return opts, err
	}
	renderModes, err := parseRenderModeList(renderModeList)
	if err != nil {
		return opts, err
	}

	opts.PPEMs = ppems
	opts.Glyphs = glyphs
	opts.Chars = chars
	opts.LoadFlags = loadFlags
	opts.RenderModes = renderModes
	return opts, nil
}

func buildGoDump(opts dumpOptions) (*Dump, error) {
	stream, closeFn, err := openFileStream(opts.FontPath)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	source, err := buildSourceInfo(opts.FontPath)
	if err != nil {
		return nil, err
	}

	numFaces, err := sfnt.NumFaces(stream)
	if err != nil {
		return nil, fmt.Errorf("read face count: %w", err)
	}

	face, err := sfnt.LoadFaceIndex(core.NewSystem(), stream, opts.FaceIndex)
	if err != nil {
		return nil, fmt.Errorf("load face %d: %w", opts.FaceIndex, err)
	}

	selections, charmap := resolveGoSelections(face, opts)
	dump := newDump("go-freetype", "", source, opts, FaceInfo{
		FaceIndex:  opts.FaceIndex,
		NumFaces:   numFaces,
		NumGlyphs:  face.GetNumGlyphs(),
		UnitsPerEm: face.GetUnitsPerEm(),
	}, charmap)

	for _, loadFlags := range opts.LoadFlags {
		for _, renderMode := range opts.RenderModes {
			for _, ppem := range opts.PPEMs {
				sizeDump := SizeDump{
					PPEMX:      ppem.X,
					PPEMY:      ppem.Y,
					LoadFlags:  loadFlags.Name,
					RenderMode: renderMode.Name,
					Glyphs:     []GlyphRecord{},
				}
				if err := face.SetPixelSizes(ppem.X, ppem.Y); err != nil {
					sizeDump.Error = err.Error()
					dump.Sizes = append(dump.Sizes, sizeDump)
					continue
				}
				for _, sel := range selections {
					sizeDump.Glyphs = append(sizeDump.Glyphs, dumpGoGlyph(face, sel, loadFlags, renderMode))
				}
				dump.Sizes = append(dump.Sizes, sizeDump)
			}
		}
	}

	return dump, nil
}

func dumpGoGlyph(face api.Face, sel glyphSelection, loadFlags loadFlagSpec, renderMode renderModeSpec) GlyphRecord {
	record := GlyphRecord{
		GlyphIndex: sel.GlyphIndex,
		Chars:      sel.Chars,
		Metrics:    MetricsRecord{Available: false, Error: "glyph not loaded"},
		Outline:    OutlineRecord{Available: false},
		Bitmap:     BitmapRecord{Available: false},
	}

	slot, err := face.LoadGlyph(sel.GlyphIndex, loadFlags.Value)
	if err != nil {
		record.LoadError = err.Error()
		record.Metrics.Error = err.Error()
		record.Outline.Error = err.Error()
		record.Bitmap.Error = err.Error()
		return record
	}

	record.Metrics = dumpGoMetrics(face, sel.GlyphIndex)
	record.SlotMetrics = dumpGoSlotMetrics(slot)
	record.Outline = dumpOutline(slot.GetOutline())
	record.Bitmap = dumpBitmap(slot.GetBitmap())
	record.Format = goGlyphFormat(record.Outline, record.Bitmap, slot.GetImage())
	record.RenderedFormat = record.Format
	if renderMode.Value != api.RenderModeNone && !record.Bitmap.Available {
		record.RenderError = fmt.Sprintf("render mode %s unsupported by Go conformance dumper", renderMode.Name)
		record.Bitmap.Error = record.RenderError
	}
	if image := slot.GetImage(); image != nil {
		sum := sha256.Sum256(image.Pixels)
		record.Image = &ImageRecord{
			Available: true,
			Width:     image.Width,
			Height:    image.Height,
			Pixels:    len(image.Pixels),
			SHA256:    fmt.Sprintf("%x", sum),
		}
	}
	return record
}

func dumpGoSlotMetrics(slot api.GlyphSlot) *SlotMetricsRecord {
	metrics, ok := api.GetGlyphSlotMetrics(slot)
	if !ok {
		return &SlotMetricsRecord{Available: false, Error: "glyph slot metrics unavailable"}
	}
	return slotMetricsRecord(metrics)
}

func dumpGoMetrics(face api.Face, glyphIndex int) MetricsRecord {
	advance, lsb, err := face.GetGlyphMetrics(glyphIndex)
	if err != nil {
		return MetricsRecord{Available: false, Error: err.Error()}
	}
	return MetricsRecord{Available: true, Advance: advance, LSB: lsb}
}

func slotMetricsRecord(metrics api.GlyphMetrics) *SlotMetricsRecord {
	return &SlotMetricsRecord{
		Available:    true,
		Width:        metrics.Width,
		Height:       metrics.Height,
		HoriBearingX: metrics.HoriBearingX,
		HoriBearingY: metrics.HoriBearingY,
		HoriAdvance:  metrics.HoriAdvance,
		VertBearingX: metrics.VertBearingX,
		VertBearingY: metrics.VertBearingY,
		VertAdvance:  metrics.VertAdvance,
	}
}

func goGlyphFormat(outline OutlineRecord, bitmap BitmapRecord, image *api.Image) string {
	switch {
	case outline.Available:
		return "outline"
	case bitmap.Available:
		return "bitmap"
	case image != nil:
		return "image"
	default:
		return "none"
	}
}

func dumpOutline(outline api.Outline) OutlineRecord {
	if outline == nil {
		return OutlineRecord{Available: false, Error: "outline unavailable"}
	}

	sourcePoints := outline.GetPoints()
	sourceTags := outline.GetTags()
	sourceContours := outline.GetContours()
	rawPointCount := len(sourcePoints)
	realPointCount := realOutlinePointCount(sourcePoints, sourceContours)
	if realPointCount > len(sourcePoints) {
		realPointCount = len(sourcePoints)
	}
	if realPointCount > len(sourceTags) {
		realPointCount = len(sourceTags)
	}

	points := make([]Point, realPointCount)
	for i := range points {
		points[i] = Point{X: sourcePoints[i].X, Y: sourcePoints[i].Y}
	}
	tags := make([]int, realPointCount)
	for i := range tags {
		tags[i] = int(sourceTags[i])
	}
	contours := append([]int{}, sourceContours...)

	phantomPoints := make([]Point, 0, len(sourcePoints)-realPointCount)
	for _, p := range sourcePoints[realPointCount:] {
		phantomPoints = append(phantomPoints, Point{X: p.X, Y: p.Y})
	}

	return OutlineRecord{
		Available:         true,
		PointCount:        len(points),
		ContourCount:      len(contours),
		RawPointCount:     rawPointCount,
		PhantomPointCount: len(phantomPoints),
		Points:            points,
		Tags:              tags,
		Contours:          contours,
		PhantomPoints:     phantomPoints,
		BBox:              bbox(points),
	}
}

func dumpBitmap(bitmap api.Bitmap) BitmapRecord {
	if bitmap == nil {
		return BitmapRecord{Available: false, Error: "bitmap unavailable"}
	}
	buffer := bitmap.GetBuffer()
	sum := sha256.Sum256(buffer)
	return BitmapRecord{
		Available:     true,
		Rows:          bitmap.GetRows(),
		Width:         bitmap.GetWidth(),
		Pitch:         bitmap.GetPitch(),
		PixelMode:     bitmap.GetPixelMode(),
		PixelModeName: pixelModeName(bitmap.GetPixelMode()),
		BufferSize:    len(buffer),
		SHA256:        fmt.Sprintf("%x", sum),
	}
}

func realOutlinePointCount(points []api.Vector, contours []int) int {
	if len(contours) == 0 {
		return 0
	}
	count := contours[len(contours)-1] + 1
	if count < 0 {
		return 0
	}
	if count > len(points) {
		return len(points)
	}
	return count
}

func bbox(points []Point) *BBox {
	if len(points) == 0 {
		return nil
	}
	box := BBox{
		XMin: points[0].X,
		YMin: points[0].Y,
		XMax: points[0].X,
		YMax: points[0].Y,
	}
	for _, p := range points[1:] {
		if p.X < box.XMin {
			box.XMin = p.X
		}
		if p.Y < box.YMin {
			box.YMin = p.Y
		}
		if p.X > box.XMax {
			box.XMax = p.X
		}
		if p.Y > box.YMax {
			box.YMax = p.Y
		}
	}
	return &box
}

func resolveGoSelections(face api.Face, opts dumpOptions) ([]glyphSelection, []CharMapRecord) {
	order := make([]int, 0, len(opts.Glyphs)+len(opts.Chars))
	seen := make(map[int]bool)
	charsByGlyph := make(map[int][]string)

	for _, glyph := range opts.Glyphs {
		if !seen[glyph] {
			seen[glyph] = true
			order = append(order, glyph)
		}
	}

	charmap := make([]CharMapRecord, 0, len(opts.Chars))
	for _, ch := range opts.Chars {
		glyph, err := face.GetGlyphIndex(ch.Rune)
		record := CharMapRecord{Char: ch.Label}
		if err != nil {
			record.Error = err.Error()
			charmap = append(charmap, record)
			continue
		}
		record.GlyphIndex = glyph
		charmap = append(charmap, record)
		charsByGlyph[glyph] = append(charsByGlyph[glyph], ch.Label)
		if !seen[glyph] {
			seen[glyph] = true
			order = append(order, glyph)
		}
	}

	if len(order) == 0 {
		order = append(order, 0)
	}

	selections := make([]glyphSelection, 0, len(order))
	for _, glyph := range order {
		selections = append(selections, glyphSelection{
			GlyphIndex: glyph,
			Chars:      append([]string(nil), charsByGlyph[glyph]...),
		})
	}
	return selections, charmap
}

func newDump(engineName, engineVersion string, source SourceInfo, opts dumpOptions, face FaceInfo, charmap []CharMapRecord) *Dump {
	return &Dump{
		Schema: dumpSchema,
		Engine: EngineInfo{
			Name:    engineName,
			Version: engineVersion,
		},
		Source: source,
		Request: RequestInfo{
			FaceIndex:    opts.FaceIndex,
			PPEM:         sizeSpecStrings(opts.PPEMs),
			Glyphs:       append([]int(nil), opts.Glyphs...),
			Chars:        charSpecLabels(opts.Chars),
			LoadFlags:    strings.Join(loadFlagNames(opts.LoadFlags), ","),
			LoadFlagSets: multiValue(loadFlagNames(opts.LoadFlags)),
			RenderMode:   strings.Join(renderModeNames(opts.RenderModes), ","),
			RenderModes:  multiValue(renderModeNames(opts.RenderModes)),
			RequestPath:  opts.RequestPath,
			Corpus:       opts.Corpus,
		},
		Face:    face,
		Charmap: charmap,
	}
}

func writeDump(dump *Dump, outputPath string, stdout io.Writer) error {
	var w io.Writer = stdout
	var file *os.File
	if outputPath != "" && outputPath != "-" {
		var err error
		file, err = os.Create(outputPath)
		if err != nil {
			return err
		}
		defer file.Close()
		w = file
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(dump)
}

func openFileStream(path string) (*fileStream, func(), error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return &fileStream{file: file, size: info.Size()}, func() { _ = file.Close() }, nil
}

func buildSourceInfo(path string) (SourceInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return SourceInfo{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return SourceInfo{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return SourceInfo{}, err
	}

	return SourceInfo{
		Path:   path,
		Size:   info.Size(),
		SHA256: fmt.Sprintf("%x", hash.Sum(nil)),
	}, nil
}

type dumpRequestFile struct {
	Description string     `json:"description,omitempty"`
	FontPath    string     `json:"font,omitempty"`
	OutputPath  string     `json:"out,omitempty"`
	Corpus      string     `json:"corpus,omitempty"`
	FaceIndex   *int       `json:"face_index,omitempty"`
	PPEM        stringList `json:"ppem,omitempty"`
	Glyphs      intList    `json:"glyphs,omitempty"`
	GlyphRanges stringList `json:"glyph_ranges,omitempty"`
	Chars       stringList `json:"chars,omitempty"`
	LoadFlags   stringList `json:"load_flags,omitempty"`
	RenderMode  stringList `json:"render_mode,omitempty"`
	RenderModes stringList `json:"render_modes,omitempty"`
}

type stringList []string

type intList []int

func (l *intList) UnmarshalJSON(data []byte) error {
	var one int
	if err := json.Unmarshal(data, &one); err == nil {
		if one < 0 {
			return fmt.Errorf("invalid glyph %d", one)
		}
		*l = []int{one}
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err == nil {
		glyphs, err := parseGlyphList(raw)
		if err != nil {
			return err
		}
		*l = glyphs
		return nil
	}
	var many []int
	if err := json.Unmarshal(data, &many); err == nil {
		out := make([]int, len(many))
		for i, glyph := range many {
			if glyph < 0 {
				return fmt.Errorf("invalid glyph %d", glyph)
			}
			out[i] = glyph
		}
		*l = out
		return nil
	}
	var manyStrings []string
	if err := json.Unmarshal(data, &manyStrings); err != nil {
		return err
	}
	var out []int
	for _, item := range manyStrings {
		glyphs, err := parseGlyphList(item)
		if err != nil {
			return err
		}
		out = append(out, glyphs...)
	}
	*l = out
	return nil
}

func (l *stringList) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*l = splitRequestList(one)
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	out := make([]string, 0, len(many))
	for _, item := range many {
		out = append(out, splitRequestList(item)...)
	}
	*l = out
	return nil
}

func splitRequestList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func readDumpRequest(path string) (*dumpRequestFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open request: %w", err)
	}
	defer file.Close()

	var request dumpRequestFile
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return nil, fmt.Errorf("decode request: %w", err)
	}
	return &request, nil
}

func applyDumpRequest(opts *dumpOptions, request *dumpRequestFile, visited map[string]bool, ppemList, glyphList, charList, loadFlagList, renderModeList *string) {
	if request.FontPath != "" && !visited["font"] {
		opts.FontPath = request.FontPath
	}
	if request.OutputPath != "" && !visited["out"] {
		opts.OutputPath = request.OutputPath
	}
	if request.Corpus != "" && !visited["corpus"] {
		opts.Corpus = request.Corpus
	}
	if request.FaceIndex != nil && !visited["face"] {
		opts.FaceIndex = *request.FaceIndex
	}
	if len(request.PPEM) > 0 && !visited["ppem"] {
		*ppemList = strings.Join([]string(request.PPEM), ",")
	}
	if (len(request.Glyphs) > 0 || len(request.GlyphRanges) > 0) && !visited["glyphs"] {
		parts := make([]string, 0, len(request.Glyphs)+len(request.GlyphRanges))
		for _, glyph := range request.Glyphs {
			parts = append(parts, strconv.Itoa(glyph))
		}
		parts = append(parts, []string(request.GlyphRanges)...)
		*glyphList = strings.Join(parts, ",")
	}
	if len(request.Chars) > 0 && !visited["chars"] {
		*charList = strings.Join([]string(request.Chars), ",")
	}
	if len(request.LoadFlags) > 0 && !visited["load-flags"] {
		*loadFlagList = strings.Join([]string(request.LoadFlags), ",")
	}
	renderModes := request.RenderModes
	if len(renderModes) == 0 {
		renderModes = request.RenderMode
	}
	if len(renderModes) > 0 && !visited["render-mode"] {
		*renderModeList = strings.Join([]string(renderModes), ",")
	}
}

func visitedFlags(fs *flag.FlagSet) map[string]bool {
	visited := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}

func parsePPEMList(raw string) ([]SizeSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty -ppem list")
	}
	parts := strings.Split(raw, ",")
	ppems := make([]SizeSpec, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		sep := strings.IndexAny(part, "xX")
		if sep < 0 {
			size, err := strconv.Atoi(part)
			if err != nil || size <= 0 {
				return nil, fmt.Errorf("invalid ppem %q", part)
			}
			ppems = append(ppems, SizeSpec{X: size, Y: size})
			continue
		}
		x, errX := strconv.Atoi(part[:sep])
		y, errY := strconv.Atoi(part[sep+1:])
		if errX != nil || errY != nil || x <= 0 || y <= 0 {
			return nil, fmt.Errorf("invalid ppem %q", part)
		}
		ppems = append(ppems, SizeSpec{X: x, Y: y})
	}
	if len(ppems) == 0 {
		return nil, errors.New("empty -ppem list")
	}
	return ppems, nil
}

func parseGlyphList(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	var glyphs []int
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, errStart := strconv.Atoi(strings.TrimSpace(bounds[0]))
			end, errEnd := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if errStart != nil || errEnd != nil || start < 0 || end < start {
				return nil, fmt.Errorf("invalid glyph range %q", part)
			}
			for glyph := start; glyph <= end; glyph++ {
				glyphs = append(glyphs, glyph)
			}
			continue
		}
		glyph, err := strconv.Atoi(part)
		if err != nil || glyph < 0 {
			return nil, fmt.Errorf("invalid glyph %q", part)
		}
		glyphs = append(glyphs, glyph)
	}
	return glyphs, nil
}

func parseCharList(raw string) ([]CharSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	chars := make([]CharSpec, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		r, err := parseRune(part)
		if err != nil {
			return nil, err
		}
		chars = append(chars, CharSpec{Label: formatRune(r), Rune: r})
	}
	return chars, nil
}

func parseRune(raw string) (rune, error) {
	upper := strings.ToUpper(strings.TrimSpace(raw))
	if strings.HasPrefix(upper, "U+") {
		value, err := strconv.ParseInt(upper[2:], 16, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid codepoint %q", raw)
		}
		return rune(value), nil
	}
	if strings.HasPrefix(upper, "0X") {
		value, err := strconv.ParseInt(upper[2:], 16, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid codepoint %q", raw)
		}
		return rune(value), nil
	}
	if strings.HasPrefix(upper, "#") {
		value, err := strconv.ParseInt(upper[1:], 16, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid codepoint %q", raw)
		}
		return rune(value), nil
	}
	runes := []rune(raw)
	if len(runes) == 1 {
		return runes[0], nil
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid char %q", raw)
	}
	return rune(value), nil
}

func parseLoadFlagSets(raw string) ([]loadFlagSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "default"
	}
	parts := strings.Split(raw, ",")
	specs := make([]loadFlagSpec, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		spec, err := parseLoadFlagSet(part)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	if len(specs) == 0 {
		return nil, errors.New("empty -load-flags list")
	}
	return specs, nil
}

func parseLoadFlagSet(raw string) (loadFlagSpec, error) {
	tokens := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '+' || r == '|' || r == ' '
	})
	if len(tokens) == 0 {
		tokens = []string{"default"}
	}
	value := api.LoadDefault
	components := make([]string, 0, len(tokens))
	seen := make(map[string]bool)
	targetSeen := false
	for _, token := range tokens {
		name := canonicalLoadFlag(token)
		if name == "" || name == "default" {
			continue
		}
		if seen[name] {
			continue
		}
		flagValue, ok := loadFlagValues[name]
		if !ok {
			return loadFlagSpec{}, fmt.Errorf("unsupported load flag %q", token)
		}
		if strings.HasPrefix(name, "target-") {
			if targetSeen {
				return loadFlagSpec{}, fmt.Errorf("multiple load targets in %q", raw)
			}
			targetSeen = true
			value &^= api.LoadTargetMask
		}
		seen[name] = true
		value |= flagValue
		components = append(components, name)
	}
	if len(components) == 0 {
		components = []string{"default"}
	}
	return loadFlagSpec{
		Name:       strings.Join(components, "+"),
		Value:      value,
		Components: components,
	}, nil
}

var loadFlagValues = map[string]int{
	"no-hinting":                  api.LoadNoHinting,
	"no-scale":                    api.LoadNoScale,
	"render":                      api.LoadRender,
	"no-bitmap":                   api.LoadNoBitmap,
	"vertical-layout":             api.LoadVerticalLayout,
	"force-autohint":              api.LoadForceAutohint,
	"crop-bitmap":                 api.LoadCropBitmap,
	"pedantic":                    api.LoadPedantic,
	"ignore-global-advance-width": api.LoadIgnoreGlobalAdvanceWidth,
	"no-recurse":                  api.LoadNoRecurse,
	"ignore-transform":            api.LoadIgnoreTransform,
	"monochrome":                  api.LoadMonochrome,
	"linear-design":               api.LoadLinearDesign,
	"no-autohint":                 api.LoadNoAutohint,
	"color":                       api.LoadColor,
	"compute-metrics":             api.LoadComputeMetrics,
	"bitmap-metrics-only":         api.LoadBitmapMetricsOnly,
	"no-svg":                      api.LoadNoSVG,
	"target-normal":               api.LoadTargetNormal,
	"target-light":                api.LoadTargetLight,
	"target-mono":                 api.LoadTargetMono,
	"target-lcd":                  api.LoadTargetLCD,
	"target-lcd-v":                api.LoadTargetLCDV,
}

func canonicalLoadFlag(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.TrimPrefix(name, "ft_load_")
	name = strings.TrimPrefix(name, "load-")
	name = strings.ReplaceAll(name, "_", "-")
	switch name {
	case "", "default":
		return "default"
	case "nohinting":
		return "no-hinting"
	case "noscale":
		return "no-scale"
	case "nobitmap":
		return "no-bitmap"
	case "vertical":
		return "vertical-layout"
	case "force-autohint", "force-autohinting":
		return "force-autohint"
	case "ignore-global-advance", "ignore-global-advance-width":
		return "ignore-global-advance-width"
	case "norecurse":
		return "no-recurse"
	case "ignoretransform":
		return "ignore-transform"
	case "linear":
		return "linear-design"
	case "noautohint":
		return "no-autohint"
	case "bitmap-only", "bitmap-metrics":
		return "bitmap-metrics-only"
	case "nosvg":
		return "no-svg"
	case "normal", "target-normal":
		return "target-normal"
	case "light", "target-light":
		return "target-light"
	case "mono", "target-mono":
		return "target-mono"
	case "lcd", "target-lcd":
		return "target-lcd"
	case "lcd-v", "lcdv", "target-lcd-v", "target-lcdv":
		return "target-lcd-v"
	default:
		return name
	}
}

func parseRenderModeList(raw string) ([]renderModeSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "none"
	}
	parts := strings.Split(raw, ",")
	modes := make([]renderModeSpec, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		mode, err := parseRenderMode(part)
		if err != nil {
			return nil, err
		}
		modes = append(modes, mode)
	}
	if len(modes) == 0 {
		return nil, errors.New("empty -render-mode list")
	}
	return modes, nil
}

func parseRenderMode(raw string) (renderModeSpec, error) {
	switch canonicalRenderMode(raw) {
	case "none":
		return renderModeSpec{Name: "none", Value: api.RenderModeNone}, nil
	case "normal":
		return renderModeSpec{Name: "normal", Value: api.RenderModeNormal}, nil
	case "light":
		return renderModeSpec{Name: "light", Value: api.RenderModeLight}, nil
	case "mono":
		return renderModeSpec{Name: "mono", Value: api.RenderModeMono}, nil
	case "lcd":
		return renderModeSpec{Name: "lcd", Value: api.RenderModeLCD}, nil
	case "lcd-v":
		return renderModeSpec{Name: "lcd-v", Value: api.RenderModeLCDV}, nil
	default:
		return renderModeSpec{}, fmt.Errorf("unsupported render mode %q", raw)
	}
}

func canonicalRenderMode(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.TrimPrefix(name, "ft_render_mode_")
	name = strings.TrimPrefix(name, "render-")
	name = strings.ReplaceAll(name, "_", "-")
	switch name {
	case "", "none", "off":
		return "none"
	case "normal":
		return "normal"
	case "light":
		return "light"
	case "mono", "monochrome":
		return "mono"
	case "lcd":
		return "lcd"
	case "lcd-v", "lcdv", "vertical-lcd":
		return "lcd-v"
	default:
		return name
	}
}

func formatRune(r rune) string {
	if r <= 0xFFFF {
		return fmt.Sprintf("U+%04X", r)
	}
	return fmt.Sprintf("U+%X", r)
}

func sizeSpecStrings(ppems []SizeSpec) []string {
	out := make([]string, len(ppems))
	for i, ppem := range ppems {
		if ppem.X == ppem.Y {
			out[i] = strconv.Itoa(ppem.X)
		} else {
			out[i] = fmt.Sprintf("%dx%d", ppem.X, ppem.Y)
		}
	}
	return out
}

func charSpecLabels(chars []CharSpec) []string {
	out := make([]string, len(chars))
	for i, ch := range chars {
		out[i] = ch.Label
	}
	return out
}

func loadFlagNames(flags []loadFlagSpec) []string {
	out := make([]string, len(flags))
	for i, flag := range flags {
		out[i] = flag.Name
	}
	return out
}

func renderModeNames(modes []renderModeSpec) []string {
	out := make([]string, len(modes))
	for i, mode := range modes {
		out[i] = mode.Name
	}
	return out
}

func multiValue(values []string) []string {
	if len(values) <= 1 {
		return nil
	}
	return values
}

func pixelModeName(mode uint8) string {
	switch mode {
	case api.MODE_NONE:
		return "none"
	case api.MODE_MONO:
		return "mono"
	case api.MODE_GRAY:
		return "gray"
	case api.MODE_LCD:
		return "lcd"
	default:
		return fmt.Sprintf("unknown-%d", mode)
	}
}
