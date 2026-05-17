package sfnt

import (
	"errors"
	"fmt"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/cff"
	"github.com/dh-kam/freetype-go/color"
	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/layout"
	"github.com/dh-kam/freetype-go/truetype"
	"github.com/dh-kam/freetype-go/var"
)

// loader implements api.Driver for SFNT formats.
type loader struct {
	sys api.FreetypeSystem
}

func NewLoader(sys api.FreetypeSystem) api.Driver {
	return &loader{sys: sys}
}

func (f *Face) getVM() *truetype.ExecutionEnv {
	vm := truetype.NewContext(f.sys)

	maxT := int(f.maxp.MaxTwilightPoints)
	maxS := int(f.maxp.MaxStorage)
	maxStk := int(f.maxp.MaxStackElements)

	if maxT < 16 {
		maxT = 16
	}
	if maxS < 64 {
		maxS = 64
	}
	if maxStk < 64 {
		maxStk = 64
	}

	vm.Prepare(maxT, maxS, maxStk, f.yPPEM, f.pointSize)
	return vm
}

func (l *loader) Handles(stream api.Stream) bool {
	if stream.Size() < 4 {
		return false
	}
	var buf [4]byte
	n, err := stream.ReadAt(buf[:], 0)
	if err != nil || n < 4 {
		return false
	}
	magic := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	return magic == 0x00010000 || magic == 0x4F54544F
}

func (l *loader) LoadFace(stream api.Stream) (api.Face, error) {
	f := &Face{
		stream:    stream,
		tables:    make(map[uint32]Table),
		sys:       l.sys,
		xPPEM:     24,
		yPPEM:     24,
		pointSize: 24,
	}

	if err := f.parseDirectory(); err != nil {
		return nil, fmt.Errorf("failed to parse SFNT directory: %v", err)
	}

	// Parse 'head' table
	headStream, err := f.GetTable("head")
	if err != nil {
		return nil, fmt.Errorf("required 'head' table not found: %v", err)
	}
	f.head, err = parseHead(headStream)
	if err != nil {
		return nil, fmt.Errorf("failed to parse 'head' table: %v", err)
	}

	// Parse 'maxp' table
	maxpStream, err := f.GetTable("maxp")
	if err != nil {
		return nil, fmt.Errorf("required 'maxp' table not found: %v", err)
	}
	f.maxp, err = parseMaxp(maxpStream)
	if err != nil {
		return nil, fmt.Errorf("failed to parse 'maxp' table: %v", err)
	}

	// Optional tables for hinting
	if s, err := f.GetTable("fpgm"); err == nil {
		f.fpgm, _ = parseFpgm(s)
	}
	if s, err := f.GetTable("prep"); err == nil {
		f.prep, _ = parsePrep(s)
	}
	if s, err := f.GetTable("cvt "); err == nil {
		f.cvt, _ = parseCvt(s)
	}

	f.funcs = make(map[int32][]byte)
	f.instrs = make(map[int32][]byte)

	vm := f.getVM()
	defer vm.Free()

	if len(f.cvt) > 0 {
		f.scaledCVT = make([]int32, len(f.cvt))
		copy(f.scaledCVT, f.cvt)
		vm.CVT = f.scaledCVT
	}

	// Run fpgm once at load time to populate functions/instructions
	if len(f.fpgm) > 0 {
		vm.Code = f.fpgm
		vm.IP = 0
		vm.Functions = f.funcs
		vm.Instructions = f.instrs
		_ = vm.Run()
	}

	// Run prep
	if len(f.prep) > 0 {
		vm.Code = f.prep
		vm.IP = 0
		vm.Functions = f.funcs
		vm.Instructions = f.instrs
		_ = vm.Run()
	}

	// Parse 'CFF ' table if present
	if cffStream, err := f.GetTable("CFF "); err == nil {
		f.cff, err = cff.ParseCFF(cffStream, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to parse 'CFF ' table: %v", err)
		}
	}

	f.glyphSlot = &GlyphSlot{}

	// Parse 'hhea' and 'hmtx' tables
	if hheaStream, err := f.GetTable("hhea"); err == nil {
		if hhea, err := parseHhea(hheaStream); err == nil {
			f.hhea = hhea
			if hmtxStream, err := f.GetTable("hmtx"); err == nil {
				if hmtx, err := parseHmtx(hmtxStream, f.GetNumGlyphs(), int(f.hhea.NumberOfHMetrics)); err == nil {
					f.hmtx = hmtx
				}
			}
		}
	}

	// Parse 'cmap' table
	if cmapStream, err := f.GetTable("cmap"); err == nil {
		if cmap, err := parseCMap(cmapStream); err == nil {
			f.cmap = cmap
		}
	}

	// Parse 'GSUB' table
	if gsubStream, err := f.GetTable("GSUB"); err == nil {
		if data, err := readStreamData(gsubStream); err == nil {
			f.gsub, _ = layout.ParseGSUB(data)
		}
	}

	// Parse 'GPOS' table
	if gposStream, err := f.GetTable("GPOS"); err == nil {
		if data, err := readStreamData(gposStream); err == nil {
			f.gpos, _ = layout.ParseGPOS(data)
		}
	}

	// Parse 'OS/2' table
	if s, err := f.GetTable("OS/2"); err == nil {
		f.os2, _ = parseOS2(s)
	}

	// Parse 'post' table
	if s, err := f.GetTable("post"); err == nil {
		f.post, _ = parsePost(s)
	}

	// Parse 'vhea' and 'vmtx' tables
	if vheaStream, err := f.GetTable("vhea"); err == nil {
		f.vhea, err = parseVhea(vheaStream)
		if err == nil {
			if vmtxStream, err := f.GetTable("vmtx"); err == nil {
				f.vmtx, err = parseVmtx(vmtxStream, f.GetNumGlyphs(), int(f.vhea.NumOfLongVerMetrics))
			}
		}
	}

	// Parse 'VORG' table
	if s, err := f.GetTable("VORG"); err == nil {
		f.vorg, _ = parseVORG(s)
	}

	if s, err := f.GetTable("gasp"); err == nil {
		f.gasp, _ = parseGasp(s)
	}
	if s, err := f.GetTable("VDMX"); err == nil {
		f.vdmx, _ = parseVDMX(s)
	}
	if s, err := f.GetTable("hdmx"); err == nil {
		f.hdmx, _ = parseHdmx(s, f.GetNumGlyphs())
	}
	if s, err := f.GetTable("LTSH"); err == nil {
		f.ltsh, _ = parseLTSH(s)
	}
	if s, err := f.GetTable("STAT"); err == nil {
		f.stat, _ = parseSTAT(s)
	}

	// Variation tables
	var fvar *ftvar.FvarTable
	var gvar *ftvar.GvarTable
	var hvar *ftvar.HVARTable
	var vvar *ftvar.VVARTable

	if s, err := f.GetTable("fvar"); err == nil {
		fvar, _ = ftvar.ParseFvar(s)
	}
	if s, err := f.GetTable("gvar"); err == nil {
		gvar, _ = ftvar.ParseGvar(s)
	}
	if s, err := f.GetTable("HVAR"); err == nil {
		hvar, _ = ftvar.ParseHVAR(s)
	}
	if s, err := f.GetTable("VVAR"); err == nil {
		vvar, _ = ftvar.ParseVVAR(s)
	}

	if fvar != nil {
		f.varEngine = ftvar.NewVariationEngine(fvar, gvar, hvar, vvar)
	}

	// Parse 'COLR' and 'CPAL' tables
	if s, err := f.GetTable("COLR"); err == nil {
		f.colr, _ = color.ParseCOLR(s)
	}
	if s, err := f.GetTable("CPAL"); err == nil {
		f.cpal, _ = color.ParseCPAL(s)
	}

	return f, nil
}

// Face implements api.Face for SFNT fonts.
type Face struct {
	stream    api.Stream
	tables    map[uint32]Table
	head      HeadTable
	maxp      MaxpTable
	hhea      HheaTable
	hmtx      HmtxTable
	os2       OS2Table
	post      PostTable
	vhea      VheaTable
	vmtx      VmtxTable
	vorg      VORGTable
	gasp      GaspTable
	hdmx      HdmxTable
	vdmx      VDMXTable
	ltsh      LTSHTable
	stat      STATTable
	colr      *color.COLR
	cpal      *color.CPAL
	cmap      CMap
	gsub      *layout.GSUB
	gpos      *layout.GPOS
	fpgm      []byte
	prep      []byte
	cvt       []int32
	scaledCVT []int32
	funcs     map[int32][]byte
	instrs    map[int32][]byte
	sys       api.FreetypeSystem
	cff       *cff.CFF
	glyphSlot *GlyphSlot
	xPPEM     int
	yPPEM     int
	pointSize int32
	varEngine *ftvar.VariationEngine
	sbix      *SbixTable
	cblc      *CBLCTable
	cbdt      *CBDTTable
}

type GlyphSlot struct {
	outline *core.Outline
	image   *api.Image
}

func (gs *GlyphSlot) GetOutline() api.Outline {
	if gs.outline == nil {
		return nil
	}
	return gs.outline
}

func (gs *GlyphSlot) SetOutline(outline api.Outline) {
	if o, ok := outline.(*core.Outline); ok {
		gs.outline = o
	}
}

func (gs *GlyphSlot) GetBitmap() api.Bitmap {
	return nil
}

func (gs *GlyphSlot) GetImage() *api.Image {
	return gs.image
}

func (f *Face) parseDirectory() error {
	// Read offset table (12 bytes)
	if f.stream.Size() < 12 {
		return errors.New("stream too short for SFNT offset table")
	}

	numTables, err := readUint16(f.stream, 4)
	if err != nil {
		return err
	}

	// Read table directory
	offset := int64(12)
	for i := 0; i < int(numTables); i++ {
		if f.stream.Size() < offset+16 {
			return errors.New("stream too short for table directory")
		}

		tag, err := readUint32(f.stream, offset)
		if err != nil {
			return err
		}
		checksum, err := readUint32(f.stream, offset+4)
		if err != nil {
			return err
		}
		tableOffset, err := readUint32(f.stream, offset+8)
		if err != nil {
			return err
		}
		length, err := readUint32(f.stream, offset+12)
		if err != nil {
			return err
		}

		f.tables[tag] = Table{
			Tag:      tag,
			Checksum: checksum,
			Offset:   tableOffset,
			Length:   length,
		}
		offset += 16
	}

	return nil
}

func (f *Face) GetTable(tagStr string) (api.Stream, error) {
	tag := stringToTag(tagStr)
	table, ok := f.tables[tag]
	if !ok {
		return nil, fmt.Errorf("table %s not found", tagStr)
	}

	return &tableStream{
		base:   f.stream,
		offset: int64(table.Offset),
		length: int64(table.Length),
	}, nil
}

func (f *Face) GetNumGlyphs() int {
	if f.cff != nil {
		return int(f.cff.CharStringsIndex.Count)
	}
	return int(f.maxp.NumGlyphs)
}

func (f *Face) GetUnitsPerEm() uint16 {
	return f.head.UnitsPerEm
}

func (f *Face) SetPixelSizes(width, height int) error {
	if width < 0 || height < 0 {
		return errors.New("pixel size must be non-negative")
	}
	if width == 0 {
		width = height
	}
	if height == 0 {
		height = width
	}
	if width <= 0 || height <= 0 {
		return errors.New("pixel size must be positive")
	}

	f.xPPEM = width
	f.yPPEM = height
	f.pointSize = int32(height)
	return nil
}

func (f *Face) GetGlyphIndex(char rune) (int, error) {
	if f.cmap == nil {
		return 0, errors.New("no cmap table found")
	}
	return int(f.cmap.Lookup(char)), nil
}

func (f *Face) GetGlyphMetrics(glyphIndex int) (advance int32, lsb int32, err error) {
	if glyphIndex < 0 || glyphIndex >= f.GetNumGlyphs() {
		return 0, 0, fmt.Errorf("glyph index %d out of range", glyphIndex)
	}

	numHMetrics := int(f.hhea.NumberOfHMetrics)
	if numHMetrics == 0 {
		return 0, 0, errors.New("no horizontal metrics found")
	}
	if len(f.hmtx.HMetrics) == 0 {
		return 0, 0, errors.New("hmtx table not found")
	}
	if numHMetrics > len(f.hmtx.HMetrics) {
		return 0, 0, fmt.Errorf("hmtx table has %d long metrics, hhea requires %d", len(f.hmtx.HMetrics), numHMetrics)
	}

	if glyphIndex < numHMetrics {
		advance = int32(f.hmtx.HMetrics[glyphIndex].AdvanceWidth)
		lsb = int32(f.hmtx.HMetrics[glyphIndex].LeftSideBearing)
	} else {
		advance = int32(f.hmtx.HMetrics[numHMetrics-1].AdvanceWidth)
		lsbIndex := glyphIndex - numHMetrics
		if lsbIndex < len(f.hmtx.LeftSideBearings) {
			lsb = int32(f.hmtx.LeftSideBearings[lsbIndex])
		} else {
			lsb = 0
		}
	}

	if f.varEngine != nil {
		advance += f.varEngine.GetAdvanceDelta(glyphIndex)
		lsb += f.varEngine.GetLSBDelta(glyphIndex)
	}

	return advance, lsb, nil
}

func (f *Face) Shape(text string) ([]int, []api.Vector) {
	glyphs := make([]int, 0, len(text))
	for _, r := range text {
		gid, _ := f.GetGlyphIndex(r)
		glyphs = append(glyphs, gid)
	}

	if f.gsub != nil {
		glyphs = f.gsub.Substitute(glyphs)
	}

	positions := make([]api.Vector, len(glyphs))
	// Basic positioning using hmtx
	for i, gid := range glyphs {
		advance, _, _ := f.GetGlyphMetrics(gid)
		positions[i].X = advance
	}

	if f.gpos != nil {
		adjustments := f.gpos.Position(glyphs)
		for i := range positions {
			positions[i].X += adjustments[i].X
			positions[i].Y += adjustments[i].Y
		}
	}

	return glyphs, positions
}

const (
	ARG_1_AND_2_ARE_WORDS    = 0x0001
	ARGS_ARE_XY_VALUES       = 0x0002
	ROUND_XY_TO_GRID         = 0x0004
	WE_HAVE_A_SCALE          = 0x0008
	MORE_COMPONENTS          = 0x0020
	WE_HAVE_AN_X_AND_Y_SCALE = 0x0040
	WE_HAVE_A_TWO_BY_TWO     = 0x0080
	WE_HAVE_INSTRUCTIONS     = 0x0100
	USE_MY_METRICS           = 0x0200
	OVERLAP_COMPOUND         = 0x0400
)

func (f *Face) LoadGlyph(glyphIndex int, loadFlags int) (api.GlyphSlot, error) {
	var imagePayload []byte

	if f.sbix != nil {
		payload, err := f.sbix.GetImage(glyphIndex, 0xFFFF) // Request max size
		if err == nil && payload != nil {
			imagePayload = payload
		}
	}
	if imagePayload == nil && f.cblc != nil && f.cbdt != nil {
		payload, err := GetCBLCImage(*f.cblc, *f.cbdt, glyphIndex)
		if err == nil && payload != nil {
			imagePayload = payload
		}
	}

	var decodedImage *api.Image
	if imagePayload != nil {
		decoder := f.sys.GetImageDecoder()
		if decoder != nil {
			img, err := decoder.Decode(imagePayload)
			if err == nil {
				decodedImage = img
			}
		}
	}

	if f.cff != nil {
		outline, err := f.cff.LoadGlyphOutline(glyphIndex)
		if err != nil {
			if decodedImage != nil {
				slot := &GlyphSlot{image: decodedImage}
				f.glyphSlot = slot
				return slot, nil
			}
			return nil, err
		}
		slot := &GlyphSlot{outline: outline, image: decodedImage}
		f.glyphSlot = slot
		return slot, nil
	}
	outline, err := f.loadGlyphInternal(glyphIndex)
	if err != nil {
		if decodedImage != nil {
			slot := &GlyphSlot{image: decodedImage}
			f.glyphSlot = slot
			return slot, nil
		}
		return nil, err
	}
	slot := &GlyphSlot{outline: outline, image: decodedImage}
	f.glyphSlot = slot
	return slot, nil
}

func (f *Face) loadGlyphInternal(glyphIndex int) (*core.Outline, error) {
	if glyphIndex < 0 || glyphIndex >= f.GetNumGlyphs() {
		return nil, fmt.Errorf("glyph index %d out of range", glyphIndex)
	}

	locaStream, err := f.GetTable("loca")
	if err != nil {
		return nil, err
	}

	var offset, length uint32
	if f.head.IndexToLocFormat == 0 {
		// Short format (uint16)
		o1, err := readUint16(locaStream, int64(glyphIndex*2))
		if err != nil {
			return nil, err
		}
		o2, err := readUint16(locaStream, int64((glyphIndex+1)*2))
		if err != nil {
			return nil, err
		}
		offset = uint32(o1) * 2
		length = uint32(o2)*2 - offset
	} else {
		// Long format (uint32)
		o1, err := readUint32(locaStream, int64(glyphIndex*4))
		if err != nil {
			return nil, err
		}
		o2, err := readUint32(locaStream, int64((glyphIndex+1)*4))
		if err != nil {
			return nil, err
		}
		offset = o1
		length = o2 - offset
	}

	if length == 0 {
		// Empty glyph
		return &core.Outline{}, nil
	}

	glyfStream, err := f.GetTable("glyf")
	if err != nil {
		return nil, err
	}

	return f.parseGlyph(glyfStream, int64(offset), int64(length), glyphIndex)
}

func (f *Face) parseGlyph(s api.Stream, offset int64, length int64, glyphIndex int) (*core.Outline, error) {
	numberOfContours, err := readInt16(s, offset)
	if err != nil {
		return nil, err
	}

	if numberOfContours >= 0 {
		return f.parseSimpleGlyph(s, offset, numberOfContours, glyphIndex)
	} else {
		return f.parseCompositeGlyph(s, offset, glyphIndex)
	}
}

func (f *Face) getPhantomPoints(glyphIndex int) []api.Vector {
	advance, _, _ := f.GetGlyphMetrics(glyphIndex)

	advanceHeight := int32(f.GetUnitsPerEm())
	var topOrigin int32 = 0

	if f.vhea.NumOfLongVerMetrics > 0 {
		numVMetrics := int(f.vhea.NumOfLongVerMetrics)
		var tsb int16
		if glyphIndex < numVMetrics {
			advanceHeight = int32(f.vmtx.VMetrics[glyphIndex].AdvanceHeight)
			tsb = f.vmtx.VMetrics[glyphIndex].TopSideBearing
		} else {
			advanceHeight = int32(f.vmtx.VMetrics[numVMetrics-1].AdvanceHeight)
			tsbIndex := glyphIndex - numVMetrics
			if tsbIndex >= 0 && tsbIndex < len(f.vmtx.TopSideBearings) {
				tsb = f.vmtx.TopSideBearings[tsbIndex]
			}
		}
		topOrigin = int32(tsb)
	}

	return []api.Vector{
		{X: 0, Y: 0},
		{X: advance << 6, Y: 0},
		{X: 0, Y: topOrigin << 6},
		{X: 0, Y: advanceHeight << 6},
	}
}

func (f *Face) parseSimpleGlyph(s api.Stream, offset int64, numberOfContours int16, glyphIndex int) (*core.Outline, error) {
	if numberOfContours <= 0 {
		return &core.Outline{}, nil
	}
	headerSize := int64(10) // numberOfContours + 4 * int16 bounding box

	endPtsOfContours := make([]uint16, numberOfContours)
	for i := 0; i < int(numberOfContours); i++ {
		val, err := readUint16(s, offset+headerSize+int64(i*2))
		if err != nil {
			return nil, err
		}
		endPtsOfContours[i] = val
	}

	lastPointIndex := int(endPtsOfContours[numberOfContours-1])
	numPoints := lastPointIndex + 1

	instructionLengthOffset := offset + headerSize + int64(numberOfContours*2)
	instructionLength, err := readUint16(s, instructionLengthOffset)
	if err != nil {
		return nil, err
	}

	// Read instructions
	instructions := make([]byte, instructionLength)
	if instructionLength > 0 {
		if err := readExactAt(s, instructions, instructionLengthOffset+2); err != nil {
			return nil, err
		}
	}

	flagsOffset := instructionLengthOffset + 2 + int64(instructionLength)

	flags := make([]byte, numPoints)
	currOffset := flagsOffset
	for i := 0; i < numPoints; {
		flag, err := readByte(s, currOffset)
		if err != nil {
			return nil, err
		}
		currOffset++
		flags[i] = flag
		i++
		if flag&0x08 != 0 { // Repeat flag
			repeatCount, err := readByte(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset++
			for j := 0; j < int(repeatCount) && i < numPoints; j++ {
				flags[i] = flag
				i++
			}
		}
	}

	xCoords := make([]int32, numPoints)
	var lastX int32 = 0
	for i := 0; i < numPoints; i++ {
		flag := flags[i]
		if flag&0x02 != 0 { // X Short Vector
			val, err := readByte(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset++
			if flag&0x10 != 0 { // X Is Same or Positive X Short Vector
				lastX += int32(val)
			} else {
				lastX -= int32(val)
			}
		} else {
			if flag&0x10 != 0 { // X Is Same
				// lastX remains same
			} else {
				val, err := readInt16(s, currOffset)
				if err != nil {
					return nil, err
				}
				currOffset += 2
				lastX += int32(val)
			}
		}
		xCoords[i] = lastX
	}

	yCoords := make([]int32, numPoints)
	var lastY int32 = 0
	for i := 0; i < numPoints; i++ {
		flag := flags[i]
		if flag&0x04 != 0 { // Y Short Vector
			val, err := readByte(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset++
			if flag&0x20 != 0 { // Y Is Same or Positive Y Short Vector
				lastY += int32(val)
			} else {
				lastY -= int32(val)
			}
		} else {
			if flag&0x20 != 0 { // Y Is Same
				// lastY remains same
			} else {
				val, err := readInt16(s, currOffset)
				if err != nil {
					return nil, err
				}
				currOffset += 2
				lastY += int32(val)
			}
		}
		yCoords[i] = lastY
	}

	// Convert to api.Outline
	outline := &core.Outline{
		Points:   make([]api.Vector, numPoints),
		Tags:     make([]byte, numPoints),
		Contours: make([]int, numberOfContours),
	}

	for i := 0; i < numPoints; i++ {
		// TrueType units are converted to 26.6 fixed point (shifted by 6)
		outline.Points[i] = api.Vector{X: xCoords[i] << 6, Y: yCoords[i] << 6}
		outline.Tags[i] = flags[i] & 0x01 // Bit 0 is onCurve
	}

	for i := 0; i < int(numberOfContours); i++ {
		outline.Contours[i] = int(endPtsOfContours[i])
	}

	// Append phantom points
	phantomPoints := f.getPhantomPoints(glyphIndex)
	outline.Points = append(outline.Points, phantomPoints...)
	outline.Tags = append(outline.Tags, 0, 0, 0, 0)

	if len(instructions) > 0 {
		vm := f.getVM()
		defer vm.Free()

		vm.Functions = f.funcs
		vm.Instructions = f.instrs
		vm.CVT = f.scaledCVT

		// Prepare VM Zone 1 with glyph points (including phantom points)
		vm.Zones[1] = truetype.Zone{
			Points:         outline.Points,
			OriginalPoints: make([]api.Vector, len(outline.Points)),
			TouchedX:       make([]bool, len(outline.Points)),
			TouchedY:       make([]bool, len(outline.Points)),
			Contours:       outline.Contours,
		}
		copy(vm.Zones[1].OriginalPoints, outline.Points)

		// Set default VM state for glyph execution
		vm.Code = instructions
		vm.IP = 0
		vm.ZP0 = 1
		vm.ZP1 = 1
		vm.ZP2 = 1
		vm.RP0 = 0
		vm.RP1 = 0
		vm.RP2 = 0

		_ = vm.Run()
	}

	return outline, nil
}

func (f *Face) parseCompositeGlyph(s api.Stream, offset int64, glyphIndex int) (*core.Outline, error) {
	currOffset := offset + 10 // skip header (numberOfContours + bounding box)

	var finalOutline *core.Outline
	var instructions []byte

	for {
		flags, err := readUint16(s, currOffset)
		if err != nil {
			return nil, err
		}
		currOffset += 2

		subGlyphIndex, err := readUint16(s, currOffset)
		if err != nil {
			return nil, err
		}
		currOffset += 2

		var arg1, arg2 int32
		if flags&ARG_1_AND_2_ARE_WORDS != 0 {
			v1, err := readInt16(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset += 2
			v2, err := readInt16(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset += 2
			arg1 = int32(v1)
			arg2 = int32(v2)
		} else {
			v1, err := readByte(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset++
			v2, err := readByte(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset++
			if flags&ARGS_ARE_XY_VALUES != 0 {
				arg1 = int32(int8(v1))
				arg2 = int32(int8(v2))
			} else {
				arg1 = int32(v1)
				arg2 = int32(v2)
			}
		}

		var xx, xy, yx, yy int32
		xx = 1 << 14 // 1.0 in 2.14
		yy = 1 << 14
		xy = 0
		yx = 0

		if flags&WE_HAVE_A_SCALE != 0 {
			scale, err := readInt16(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset += 2
			xx = int32(scale)
			yy = int32(scale)
		} else if flags&WE_HAVE_AN_X_AND_Y_SCALE != 0 {
			xscale, err := readInt16(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset += 2
			yscale, err := readInt16(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset += 2
			xx = int32(xscale)
			yy = int32(yscale)
		} else if flags&WE_HAVE_A_TWO_BY_TWO != 0 {
			xscale, err := readInt16(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset += 2
			scale01, err := readInt16(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset += 2
			scale10, err := readInt16(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset += 2
			yscale, err := readInt16(s, currOffset)
			if err != nil {
				return nil, err
			}
			currOffset += 2
			xx = int32(xscale)
			xy = int32(scale01)
			yx = int32(scale10)
			yy = int32(yscale)
		}

		subOutline, err := f.loadGlyphInternal(int(subGlyphIndex))
		if err != nil {
			return nil, err
		}

		// Apply transform and initial translation
		var dx, dy int32
		if flags&ARGS_ARE_XY_VALUES != 0 {
			dx = arg1 << 6
			dy = arg2 << 6
		}

		for i := range subOutline.Points {
			x := subOutline.Points[i].X
			y := subOutline.Points[i].Y

			// (x * xx + y * xy) >> 14
			nx := (x*xx + y*xy) >> 14
			ny := (x*yx + y*yy) >> 14

			subOutline.Points[i].X = nx + dx
			subOutline.Points[i].Y = ny + dy
		}

		if flags&ARGS_ARE_XY_VALUES == 0 {
			if finalOutline != nil && int(arg1) < len(finalOutline.Points) && int(arg2) < len(subOutline.Points) {
				p1 := finalOutline.Points[arg1]
				p2 := subOutline.Points[arg2]
				matchDx := p1.X - p2.X
				matchDy := p1.Y - p2.Y
				for i := range subOutline.Points {
					subOutline.Points[i].X += matchDx
					subOutline.Points[i].Y += matchDy
				}
			}
		}

		// Strip phantom points from subOutline before merging
		realSubPoints := len(subOutline.Points)
		if len(subOutline.Contours) > 0 {
			realSubPoints = subOutline.Contours[len(subOutline.Contours)-1] + 1
		} else {
			realSubPoints = 0
		}
		subOutline.Points = subOutline.Points[:realSubPoints]
		subOutline.Tags = subOutline.Tags[:realSubPoints]

		// Merge into finalOutline
		if finalOutline == nil {
			finalOutline = subOutline
		} else {
			numPoints := len(finalOutline.Points)
			finalOutline.Points = append(finalOutline.Points, subOutline.Points...)
			finalOutline.Tags = append(finalOutline.Tags, subOutline.Tags...)
			for _, c := range subOutline.Contours {
				finalOutline.Contours = append(finalOutline.Contours, c+numPoints)
			}
		}

		if flags&MORE_COMPONENTS == 0 {
			if flags&WE_HAVE_INSTRUCTIONS != 0 {
				numInstr, err := readUint16(s, currOffset)
				if err != nil {
					return nil, err
				}
				currOffset += 2
				instructions = make([]byte, numInstr)
				if err := readExactAt(s, instructions, currOffset); err != nil {
					return nil, err
				}
			}
			break
		}
	}

	if finalOutline == nil {
		finalOutline = &core.Outline{}
	}

	// Append phantom points for the composite glyph
	phantomPoints := f.getPhantomPoints(glyphIndex)
	finalOutline.Points = append(finalOutline.Points, phantomPoints...)
	finalOutline.Tags = append(finalOutline.Tags, 0, 0, 0, 0)

	if len(instructions) > 0 {
		vm := f.getVM()
		defer vm.Free()

		vm.Functions = f.funcs
		vm.Instructions = f.instrs
		vm.CVT = f.scaledCVT

		// Prepare VM Zone 1 with glyph points
		vm.Zones[1] = truetype.Zone{
			Points:         finalOutline.Points,
			OriginalPoints: make([]api.Vector, len(finalOutline.Points)),
			TouchedX:       make([]bool, len(finalOutline.Points)),
			TouchedY:       make([]bool, len(finalOutline.Points)),
			Contours:       finalOutline.Contours,
		}
		copy(vm.Zones[1].OriginalPoints, finalOutline.Points)

		// Set default VM state for glyph execution
		vm.Code = instructions
		vm.IP = 0
		vm.ZP0 = 1
		vm.ZP1 = 1
		vm.ZP2 = 1
		vm.RP0 = 0
		vm.RP1 = 0
		vm.RP2 = 0

		_ = vm.Run()
	}

	return finalOutline, nil
}

func (f *Face) GetGlyphSlot() api.GlyphSlot {
	return f.glyphSlot
}

func (f *Face) GetColorLayers(glyphIndex int) ([]color.Layer, error) {
	if f.colr == nil {
		return nil, nil
	}
	return f.colr.GetLayers(uint16(glyphIndex), f.cpal, 0)
}

var _ api.Driver = (*loader)(nil)
var _ api.Face = (*Face)(nil)
