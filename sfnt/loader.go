package sfnt

import (
	"errors"
	"fmt"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/cff"
	"github.com/dh-kam/freetype-go/color"
	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/layout"
	ftmath "github.com/dh-kam/freetype-go/math"
	"github.com/dh-kam/freetype-go/raster"
	"github.com/dh-kam/freetype-go/truetype"
	"github.com/dh-kam/freetype-go/var"
)

const tagTTCF = 0x74746366

const (
	outlineTagHasScanMode = 0x04
	outlineTagScanMode    = 0xE0
)

// loader implements api.Driver for SFNT formats.
type loader struct {
	sys api.FreetypeSystem
}

func NewLoader(sys api.FreetypeSystem) api.Driver {
	return &loader{sys: sys}
}

// LoadFaceIndex loads a face from an SFNT stream or collection.
// Non-collection fonts only support faceIndex 0.
func LoadFaceIndex(sys api.FreetypeSystem, stream api.Stream, faceIndex int) (api.Face, error) {
	return (&loader{sys: sys}).LoadFaceIndex(stream, faceIndex)
}

// NumFaces returns the number of faces in an SFNT stream or collection.
func NumFaces(stream api.Stream) (int, error) {
	return sfntFaceCount(stream)
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
	f.configureVM(vm, api.LoadDefault)
	return vm
}

func (f *Face) configureVM(vm *truetype.ExecutionEnv, loadFlags int) {
	if vm == nil {
		return
	}
	vm.UnitsPerEm = f.GetUnitsPerEm()
	vm.XScale = f.xScale
	vm.YScale = f.yScale
	vm.InterpreterVersion = 40
	vm.Variation = f.varEngine != nil
	vm.RenderMode = renderModeForLoadFlags(loadFlags)
	vm.Grayscale = vm.RenderMode != api.RenderModeNone && vm.RenderMode != api.RenderModeMono
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
	return magic == 0x00010000 || magic == 0x4F54544F || magic == tagTTCF
}

func (l *loader) LoadFace(stream api.Stream) (api.Face, error) {
	return l.LoadFaceIndex(stream, 0)
}

func (l *loader) LoadFaceIndex(stream api.Stream, faceIndex int) (api.Face, error) {
	directoryOffset, err := sfntDirectoryOffset(stream, faceIndex)
	if err != nil {
		return nil, err
	}

	f := &Face{
		stream:          stream,
		directoryOffset: directoryOffset,
		tables:          make(map[uint32]Table),
		sys:             l.sys,
		xPPEM:           24,
		yPPEM:           24,
		xScale:          1 << 16,
		yScale:          1 << 16,
		pointSize:       24,
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

	// Variation tables are needed before CVT scaling/prep so cvar deltas and
	// VM variation state are visible during size setup.
	var fvar *ftvar.FvarTable
	var gvar *ftvar.GvarTable
	var hvar *ftvar.HVARTable
	var vvar *ftvar.VVARTable
	var avar *ftvar.AvarTable
	var cvar *ftvar.CvarTable
	var mvar *ftvar.MVARTable

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
		if s, err := f.GetTable("avar"); err == nil {
			avar, _ = ftvar.ParseAvar(s)
		}
		if s, err := f.GetTable("cvar"); err == nil {
			cvar, _ = ftvar.ParseCvar(s, len(fvar.Axes))
		}
		if s, err := f.GetTable("MVAR"); err == nil {
			mvar, _ = ftvar.ParseMVAR(s)
		}

		f.varEngine = ftvar.NewVariationEngine(fvar, gvar, hvar, vvar)
		f.varEngine.SetAvar(avar)
		f.varEngine.SetCvar(cvar)
		f.varEngine.SetMVAR(mvar)
	}

	f.funcs = make(map[int32][]byte)
	f.instrs = make(map[int32][]byte)
	f.recomputeSizeMetrics()

	vm := f.getVM()
	defer vm.Free()

	if err := f.scaleCVT(); err != nil {
		return nil, fmt.Errorf("failed to apply CVT variations: %w", err)
	}
	vm.CVT = f.scaledCVT

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
		_ = f.runPrep()
	}

	// Parse 'CFF ' table if present
	if cffStream, err := f.GetTable("CFF "); err == nil {
		f.cff, err = cff.ParseCFF(cffStream, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to parse 'CFF ' table: %v", err)
		}
	} else if cff2Stream, err := f.GetTable("CFF2"); err == nil {
		f.cff, err = cff.ParseCFF(cff2Stream, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to parse 'CFF2' table: %v", err)
		}
		f.syncCFFVariationCoordinates()
	}

	f.glyphSlot = &GlyphSlot{}

	// Parse 'hhea' and 'hmtx' tables
	if hheaStream, err := f.GetTable("hhea"); err == nil {
		if hhea, err := parseHhea(hheaStream); err == nil {
			f.hhea = hhea
			f.baseHhea = hhea
			f.hasBaseHhea = true
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

	// Parse 'GDEF' table before GSUB/GPOS so lookup flags can use glyph classes
	// and mark filtering data.
	if gdefStream, err := f.GetTable("GDEF"); err == nil {
		if data, err := readStreamData(gdefStream); err == nil {
			f.gdef, _ = layout.ParseGDEF(data)
		}
	}

	// Parse 'GSUB' table
	if gsubStream, err := f.GetTable("GSUB"); err == nil {
		if data, err := readStreamData(gsubStream); err == nil {
			f.gsub, _ = layout.ParseGSUB(data)
			if f.gsub != nil {
				f.gsub.GDEF = f.gdef
			}
		}
	}

	// Parse 'GPOS' table
	if gposStream, err := f.GetTable("GPOS"); err == nil {
		if data, err := readStreamData(gposStream); err == nil {
			f.gpos, _ = layout.ParseGPOS(data)
			if f.gpos != nil {
				f.gpos.GDEF = f.gdef
			}
		}
	}

	// Parse 'OS/2' table
	if s, err := f.GetTable("OS/2"); err == nil {
		if os2, err := parseOS2(s); err == nil {
			f.os2 = os2
			f.baseOS2 = os2
			f.hasBaseOS2 = true
		}
	}

	// Parse 'post' table
	if s, err := f.GetTable("post"); err == nil {
		if post, err := parsePost(s); err == nil {
			f.post = post
			f.basePost = post
			f.hasBasePost = true
		}
	}

	// Parse 'vhea' and 'vmtx' tables
	if vheaStream, err := f.GetTable("vhea"); err == nil {
		f.vhea, err = parseVhea(vheaStream)
		if err == nil {
			f.baseVhea = f.vhea
			f.hasBaseVhea = true
			if vmtxStream, err := f.GetTable("vmtx"); err == nil {
				f.vmtx, err = parseVmtx(vmtxStream, f.GetNumGlyphs(), int(f.vhea.NumOfLongVerMetrics))
			}
		}
	}

	// Parse 'VORG' table
	if s, err := f.GetTable("VORG"); err == nil {
		if vorg, err := parseVORG(s); err == nil {
			f.vorg = vorg
			f.hasVORG = true
		}
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

	if s, err := f.GetTable("sbix"); err == nil {
		if sbix, err := parseSbix(s); err == nil {
			f.sbix = &sbix
		}
	}
	if s, err := f.GetTable("CBLC"); err == nil {
		if cblc, err := parseCBLC(s); err == nil {
			f.cblc = &cblc
		}
	} else if s, err := f.GetTable("EBLC"); err == nil {
		if eblc, err := parseCBLC(s); err == nil {
			f.cblc = &eblc
		}
	}
	if s, err := f.GetTable("CBDT"); err == nil {
		if cbdt, err := parseCBDT(s); err == nil {
			f.cbdt = &cbdt
		}
	} else if s, err := f.GetTable("EBDT"); err == nil {
		if ebdt, err := parseCBDT(s); err == nil {
			f.cbdt = &ebdt
		}
	}

	f.applyMVARMetricDeltas()

	// Parse 'COLR' and 'CPAL' tables
	if s, err := f.GetTable("COLR"); err == nil {
		f.colr, _ = color.ParseCOLR(s)
	}
	if s, err := f.GetTable("CPAL"); err == nil {
		f.cpal, _ = color.ParseCPAL(s)
	}

	return f, nil
}

func sfntFaceCount(stream api.Stream) (int, error) {
	if stream.Size() < 4 {
		return 0, errors.New("stream too short for SFNT signature")
	}
	magic, err := readUint32(stream, 0)
	if err != nil {
		return 0, err
	}
	if magic != tagTTCF {
		return 1, nil
	}
	if stream.Size() < 16 {
		return 0, errors.New("TTC header too short")
	}
	version, err := readUint32(stream, 4)
	if err != nil {
		return 0, err
	}
	if version != 0x00010000 && version != 0x00020000 {
		return 0, fmt.Errorf("unsupported TTC version 0x%08x", version)
	}
	numFonts, err := readUint32(stream, 8)
	if err != nil {
		return 0, err
	}
	if numFonts == 0 {
		return 0, errors.New("TTC collection has no fonts")
	}
	if uint64(numFonts) > uint64(int(^uint(0)>>1)) {
		return 0, errors.New("TTC collection has too many fonts")
	}
	if uint64(12)+uint64(numFonts)*4 > uint64(stream.Size()) {
		return 0, errors.New("TTC offset table too short")
	}
	return int(numFonts), nil
}

func sfntDirectoryOffset(stream api.Stream, faceIndex int) (int64, error) {
	if faceIndex < 0 {
		return 0, errors.New("negative face index")
	}
	magic, err := readUint32(stream, 0)
	if err != nil {
		return 0, err
	}
	if magic != tagTTCF {
		if faceIndex != 0 {
			return 0, errors.New("face index out of range")
		}
		return 0, nil
	}

	numFaces, err := sfntFaceCount(stream)
	if err != nil {
		return 0, err
	}
	if faceIndex >= numFaces {
		return 0, errors.New("face index out of range")
	}
	faceOffset, err := readUint32(stream, 12+int64(faceIndex)*4)
	if err != nil {
		return 0, err
	}
	if uint64(faceOffset)+12 > uint64(stream.Size()) {
		return 0, errors.New("TTC face offset out of bounds")
	}
	return int64(faceOffset), nil
}

// Face implements api.Face for SFNT fonts.
type Face struct {
	stream          api.Stream
	directoryOffset int64
	tables          map[uint32]Table
	head            HeadTable
	maxp            MaxpTable
	hhea            HheaTable
	baseHhea        HheaTable
	hasBaseHhea     bool
	hmtx            HmtxTable
	os2             OS2Table
	baseOS2         OS2Table
	hasBaseOS2      bool
	post            PostTable
	basePost        PostTable
	hasBasePost     bool
	vhea            VheaTable
	baseVhea        VheaTable
	hasBaseVhea     bool
	vmtx            VmtxTable
	vorg            VORGTable
	hasVORG         bool
	gasp            GaspTable
	hdmx            HdmxTable
	vdmx            VDMXTable
	ltsh            LTSHTable
	stat            STATTable
	colr            *color.COLR
	cpal            *color.CPAL
	cmap            CMap
	gdef            *layout.GDEF
	gsub            *layout.GSUB
	gpos            *layout.GPOS
	fpgm            []byte
	prep            []byte
	cvt             []int32
	scaledCVT       []int32
	funcs           map[int32][]byte
	instrs          map[int32][]byte
	prepGS          truetype.GraphicsState
	hasPrepGS       bool
	sys             api.FreetypeSystem
	cff             *cff.CFF
	glyphSlot       *GlyphSlot
	xPPEM           int
	yPPEM           int
	xScale          int32
	yScale          int32
	pointSize       int32
	varEngine       *ftvar.VariationEngine
	sbix            *SbixTable
	cblc            *CBLCTable
	cbdt            *CBDTTable
	loadedMetrics   map[int]glyphMetrics26Dot6
}

// UsesCFFOutlines reports whether this SFNT face renders glyph outlines from CFF/CFF2 data.
func (f *Face) UsesCFFOutlines() bool {
	return f != nil && f.cff != nil
}

type GlyphSlot struct {
	outline        *core.Outline
	bitmap         api.Bitmap
	image          *api.Image
	metrics        glyphMetrics26Dot6
	hasMetrics     bool
	slotMetrics    api.GlyphMetrics
	hasSlotMetrics bool
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
	return gs.bitmap
}

func (gs *GlyphSlot) GetImage() *api.Image {
	return gs.image
}

func (gs *GlyphSlot) GetMetrics() (api.GlyphMetrics, bool) {
	if gs == nil || !gs.hasSlotMetrics {
		return api.GlyphMetrics{}, false
	}
	return gs.slotMetrics, true
}

func (f *Face) parseDirectory() error {
	// Read offset table (12 bytes)
	base := f.directoryOffset
	if base < 0 || f.stream.Size() < base+12 {
		return errors.New("stream too short for SFNT offset table")
	}

	numTables, err := readUint16(f.stream, base+4)
	if err != nil {
		return err
	}

	// Read table directory
	offset := base + 12
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

func (f *Face) GetGlyphName(glyphIndex int) (string, bool) {
	if f == nil || glyphIndex < 0 || glyphIndex >= f.GetNumGlyphs() {
		return "", false
	}
	if f.cff != nil {
		return f.cff.GlyphName(glyphIndex)
	}
	return f.post.GlyphName(glyphIndex)
}

func (f *Face) recomputeSizeMetrics() {
	f.pointSize = int32(f.yPPEM)
	f.xScale = f.computeScale(f.xPPEM)
	f.yScale = f.computeScale(f.yPPEM)
}

func (f *Face) computeScale(ppem int) int32 {
	unitsPerEm := f.GetUnitsPerEm()
	if unitsPerEm == 0 || ppem <= 0 {
		return 1 << 16
	}
	return ftmath.DivFix(int32(ppem), int32(unitsPerEm))
}

func (f *Face) scaleCVT() error {
	if len(f.cvt) == 0 {
		f.scaledCVT = nil
		return nil
	}
	cvt := make([]int32, len(f.cvt))
	copy(cvt, f.cvt)
	if f.varEngine != nil {
		if err := f.varEngine.ApplyCVTDeltas(cvt); err != nil {
			return err
		}
	}
	f.scaledCVT = make([]int32, len(f.cvt))
	for i, v := range cvt {
		f.scaledCVT[i] = f.scaleFUnitsY(v)
	}
	return nil
}

func (f *Face) runPrep() error {
	f.hasPrepGS = false
	if len(f.prep) == 0 {
		return nil
	}
	if f.funcs == nil {
		f.funcs = make(map[int32][]byte)
	}
	if f.instrs == nil {
		f.instrs = make(map[int32][]byte)
	}

	vm := f.getVM()
	defer vm.Free()

	vm.Code = f.prep
	vm.IP = 0
	vm.Functions = f.funcs
	vm.Instructions = f.instrs
	vm.CVT = f.scaledCVT
	if err := vm.Run(); err != nil {
		return err
	}
	f.prepGS = *vm.GS
	f.hasPrepGS = true
	return nil
}

func (f *Face) scaleFUnitsX(v int32) int32 {
	return scaleTTFUnits(v, f.xPPEM, f.GetUnitsPerEm())
}

func (f *Face) scaleFUnitsY(v int32) int32 {
	return scaleTTFUnits(v, f.yPPEM, f.GetUnitsPerEm())
}

func (f *Face) scaleFUnitsXForLoadFlags(v int32, loadFlags int) int32 {
	if loadFlags&api.LoadNoScale != 0 {
		return v << 6
	}
	return f.scaleFUnitsX(v)
}

func (f *Face) scaleFUnitsYForLoadFlags(v int32, loadFlags int) int32 {
	if loadFlags&api.LoadNoScale != 0 {
		return v << 6
	}
	return f.scaleFUnitsY(v)
}

func (f *Face) scale26Dot6X(v int32) int32 {
	return ftmath.MulFix(v, f.xScale)
}

func (f *Face) scale26Dot6Y(v int32) int32 {
	return ftmath.MulFix(v, f.yScale)
}

func (f *Face) scale26Dot6XForLoadFlags(v int32, loadFlags int) int32 {
	if loadFlags&api.LoadNoScale != 0 {
		return v
	}
	return f.scale26Dot6X(v)
}

func (f *Face) scale26Dot6YForLoadFlags(v int32, loadFlags int) int32 {
	if loadFlags&api.LoadNoScale != 0 {
		return v
	}
	return f.scale26Dot6Y(v)
}

func shouldHintGlyph(loadFlags int) bool {
	return loadFlags&(api.LoadNoHinting|api.LoadNoScale) == 0
}

func shouldGridFitSlotMetrics(loadFlags int) bool {
	return shouldHintGlyph(loadFlags)
}

func useMinimalSubpixelHinting(loadFlags int) bool {
	mode := renderModeForLoadFlags(loadFlags)
	return shouldHintGlyph(loadFlags) && mode != api.RenderModeNone && mode != api.RenderModeMono
}

func (f *Face) scaleOutline(outline *core.Outline) {
	if outline == nil {
		return
	}
	for i := range outline.Points {
		outline.Points[i].X = f.scaleTTOutlineCoordX(outline.Points[i].X)
		outline.Points[i].Y = f.scaleTTOutlineCoordY(outline.Points[i].Y)
	}
}

func (f *Face) scaleTTOutlineCoordX(v int32) int32 {
	return scaleTTOutlineCoord(v, f.xPPEM, f.GetUnitsPerEm())
}

func (f *Face) scaleTTOutlineCoordY(v int32) int32 {
	return scaleTTOutlineCoord(v, f.yPPEM, f.GetUnitsPerEm())
}

func scaleTTOutlineCoord(v int32, ppem int, unitsPerEm uint16) int32 {
	if unitsPerEm == 0 || ppem <= 0 {
		return v
	}
	// FreeType's TrueType loader scales integer design-unit outline points with
	// size->metrics.[xy]_scale, i.e. a 16.16 scale for 26.6 pixel output.
	if v%64 == 0 {
		return scaleTTFUnits(v>>6, ppem, unitsPerEm)
	}
	return scaleFractionalTTOutlineCoord(v, ppem, unitsPerEm)
}

func scaleTTFUnits(v int32, ppem int, unitsPerEm uint16) int32 {
	if unitsPerEm == 0 || ppem <= 0 {
		return v << 6
	}
	return ftmath.MulFix(v, ftmath.DivFix(int32(ppem<<6), int32(unitsPerEm)))
}

func scaleFractionalTTOutlineCoord(v int32, ppem int, unitsPerEm uint16) int32 {
	num := int64(v) * int64(ppem)
	den := int64(unitsPerEm)
	if num >= 0 {
		return int32((num + den/2) / den)
	}
	return -int32((-num + den/2) / den)
}

func (f *Face) scaleCFFOutline(outline *core.Outline) {
	if outline == nil {
		return
	}
	// OpenType CFF glyph coordinates are already in the SFNT design space.
	// FreeType scales them with head.unitsPerEm; applying the CFF FontMatrix
	// here double-scales fonts whose Top DICT matrix is 1/unitsPerEm.
	xScale := f.computeCFFSizeScale(f.xPPEM)
	yScale := f.computeCFFSizeScale(f.yPPEM)
	for i := range outline.Points {
		outline.Points[i].X = ftmath.MulFix(cffDesignUnit(outline.Points[i].X), xScale)
		outline.Points[i].Y = ftmath.MulFix(cffDesignUnit(outline.Points[i].Y), yScale)
	}
}

func (f *Face) computeCFFSizeScale(ppem int) int32 {
	unitsPerEm := f.GetUnitsPerEm()
	if unitsPerEm == 0 || ppem <= 0 {
		return 1 << 16
	}
	return ftmath.DivFix(int32(ppem)<<6, int32(unitsPerEm))
}

func cffDesignUnit(v int32) int32 {
	return v / 64
}

func copyTTOrusPoints(outline *core.Outline) []api.Vector {
	if outline == nil || len(outline.Points) == 0 {
		return nil
	}
	points := make([]api.Vector, len(outline.Points))
	for i, p := range outline.Points {
		points[i] = api.Vector{X: p.X >> 6, Y: p.Y >> 6}
	}
	return points
}

func cloneVectors(points []api.Vector) []api.Vector {
	if len(points) == 0 {
		return nil
	}
	return append([]api.Vector(nil), points...)
}

func (f *Face) runGlyphProgram(outline *core.Outline, instructions []byte, loadFlags int, unscaledOriginalPoints []api.Vector) error {
	if outline == nil || len(instructions) == 0 {
		return nil
	}

	points := make([]api.Vector, len(outline.Points))
	copy(points, outline.Points)

	cvt := make([]int32, len(f.scaledCVT))
	copy(cvt, f.scaledCVT)

	vm := f.getVM()
	defer vm.Free()
	if f.hasPrepGS {
		*vm.GS = f.prepGS
	}
	f.configureVM(vm, loadFlags)
	vm.GS.BackwardCompatibility = vm.InterpreterVersion >= 40 && useMinimalSubpixelHinting(loadFlags)
	if vm.GS.BackwardCompatibility {
		vm.GS.ProjVector = api.Vector{X: 0x4000, Y: 0}
		vm.GS.FreeVector = vm.GS.ProjVector
		vm.GS.DualVector = vm.GS.ProjVector
	}

	vm.Functions = f.funcs
	vm.Instructions = f.instrs
	vm.CVT = cvt
	vm.Zones[1] = truetype.Zone{
		Points:                 points,
		OriginalPoints:         make([]api.Vector, len(outline.Points)),
		UnscaledOriginalPoints: nil,
		TouchedX:               make([]bool, len(outline.Points)),
		TouchedY:               make([]bool, len(outline.Points)),
		Contours:               outline.Contours,
		Tags:                   append([]byte{}, outline.Tags...),
	}
	copy(vm.Zones[1].OriginalPoints, outline.Points)
	if len(unscaledOriginalPoints) == len(outline.Points) {
		vm.Zones[1].UnscaledOriginalPoints = cloneVectors(unscaledOriginalPoints)
	}

	vm.Code = instructions
	vm.IP = 0
	vm.ZP0 = 1
	vm.ZP1 = 1
	vm.ZP2 = 1
	vm.RP0 = 0
	vm.RP1 = 0
	vm.RP2 = 0

	if err := vm.Run(); err != nil {
		return err
	}

	applyScanModeTag(vm.Zones[1].Tags[:clampRealPointCount(realPointCount(outline), outline)], vm.GS.ScanControl, vm.GS.ScanType)
	copy(outline.Points, points)
	if len(vm.Zones[1].Tags) == len(outline.Tags) {
		copy(outline.Tags, vm.Zones[1].Tags)
	}
	copy(f.scaledCVT, cvt)
	return nil
}

func applyScanModeTag(tags []byte, scanControl bool, scanType int32) {
	if len(tags) == 0 || !scanControl {
		return
	}
	tags[0] &^= outlineTagHasScanMode | outlineTagScanMode
	tags[0] |= outlineTagHasScanMode | byte(scanType&7)<<5
}

// SetVariationNormalizedCoordinates sets normalized variation coordinates in [-1, 1].
func (f *Face) SetVariationNormalizedCoordinates(coords []float32) error {
	if f.varEngine == nil {
		return errors.New("font has no variation axes")
	}
	f.varEngine.SetNormalizedCoordinates(coords)
	f.loadedMetrics = nil
	return f.refreshVariationDependentState()
}

// SetVariationDesignCoordinates sets design-space variation coordinates.
func (f *Face) SetVariationDesignCoordinates(coords []ftvar.Fixed) error {
	if f.varEngine == nil {
		return errors.New("font has no variation axes")
	}
	f.varEngine.SetDesignCoordinates(coords)
	f.loadedMetrics = nil
	return f.refreshVariationDependentState()
}

func (f *Face) refreshVariationDependentState() error {
	f.syncCFFVariationCoordinates()
	if err := f.scaleCVT(); err != nil {
		return fmt.Errorf("failed to apply CVT variations: %w", err)
	}
	if err := f.runPrep(); err != nil {
		return fmt.Errorf("failed to run prep program: %w", err)
	}
	f.applyMVARMetricDeltas()
	return nil
}

func (f *Face) syncCFFVariationCoordinates() {
	if f == nil || f.cff == nil || f.varEngine == nil {
		return
	}
	f.cff.SetVariationCoordinates(f.normalizedVariationCoordinates64())
}

func (f *Face) normalizedVariationCoordinates64() []float64 {
	if f == nil || f.varEngine == nil {
		return nil
	}
	coords := make([]float64, len(f.varEngine.Coords))
	for i, coord := range f.varEngine.Coords {
		coords[i] = float64(coord)
	}
	return coords
}

func (f *Face) applyMVARMetricDeltas() {
	if f.varEngine == nil {
		return
	}
	if f.hasBaseHhea {
		hhea := f.baseHhea
		hhea.CaretSlopeRise = f.mvarInt16("hcrs", hhea.CaretSlopeRise)
		hhea.CaretSlopeRun = f.mvarInt16("hcrn", hhea.CaretSlopeRun)
		hhea.CaretOffset = f.mvarInt16("hcof", hhea.CaretOffset)
		f.hhea = hhea
	}
	if f.hasBaseVhea {
		vhea := f.baseVhea
		vhea.Ascent = f.mvarInt16("vasc", vhea.Ascent)
		vhea.Descent = f.mvarInt16("vdsc", vhea.Descent)
		vhea.LineGap = f.mvarInt16("vlgp", vhea.LineGap)
		vhea.CaretSlopeRise = f.mvarInt16("vcrs", vhea.CaretSlopeRise)
		vhea.CaretSlopeRun = f.mvarInt16("vcrn", vhea.CaretSlopeRun)
		vhea.CaretOffset = f.mvarInt16("vcof", vhea.CaretOffset)
		f.vhea = vhea
	}
	if f.hasBaseOS2 {
		os2 := f.baseOS2
		os2.STypoAscender = f.mvarInt16("hasc", os2.STypoAscender)
		os2.STypoDescender = f.mvarInt16("hdsc", os2.STypoDescender)
		os2.STypoLineGap = f.mvarInt16("hlgp", os2.STypoLineGap)
		os2.UsWinAscent = f.mvarUint16("hcla", os2.UsWinAscent)
		os2.UsWinDescent = f.mvarUint16("hcld", os2.UsWinDescent)
		os2.SxHeight = f.mvarInt16("xhgt", os2.SxHeight)
		os2.SCapHeight = f.mvarInt16("cpht", os2.SCapHeight)
		os2.YSubscriptXSize = f.mvarInt16("sbxs", os2.YSubscriptXSize)
		os2.YSubscriptYSize = f.mvarInt16("sbys", os2.YSubscriptYSize)
		os2.YSubscriptXOffset = f.mvarInt16("sbxo", os2.YSubscriptXOffset)
		os2.YSubscriptYOffset = f.mvarInt16("sbyo", os2.YSubscriptYOffset)
		os2.YSuperscriptXSize = f.mvarInt16("spxs", os2.YSuperscriptXSize)
		os2.YSuperscriptYSize = f.mvarInt16("spys", os2.YSuperscriptYSize)
		os2.YSuperscriptXOffset = f.mvarInt16("spxo", os2.YSuperscriptXOffset)
		os2.YSuperscriptYOffset = f.mvarInt16("spyo", os2.YSuperscriptYOffset)
		os2.YStrikeoutSize = f.mvarInt16("strs", os2.YStrikeoutSize)
		os2.YStrikeoutPosition = f.mvarInt16("stro", os2.YStrikeoutPosition)
		f.os2 = os2
	}
	if f.hasBasePost {
		post := f.basePost
		post.UnderlineThickness = f.mvarInt16("unds", post.UnderlineThickness)
		post.UnderlinePosition = f.mvarInt16("undo", post.UnderlinePosition)
		f.post = post
	}
}

func (f *Face) mvarInt16(tag string, value int16) int16 {
	return clampInt16Metric(int32(value) + f.varEngine.GetMetricDelta(stringToTag(tag)))
}

func (f *Face) mvarUint16(tag string, value uint16) uint16 {
	return clampUint16Metric(int32(value) + f.varEngine.GetMetricDelta(stringToTag(tag)))
}

func clampInt16Metric(value int32) int16 {
	if value < -32768 {
		return -32768
	}
	if value > 32767 {
		return 32767
	}
	return int16(value)
}

func clampUint16Metric(value int32) uint16 {
	if value < 0 {
		return 0
	}
	if value > 65535 {
		return 65535
	}
	return uint16(value)
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
	if width > 1<<25 || height > 1<<25 {
		return errors.New("pixel size too large")
	}

	f.xPPEM = width
	f.yPPEM = height
	f.recomputeSizeMetrics()
	f.loadedMetrics = nil
	f.syncCFFVariationCoordinates()
	if err := f.scaleCVT(); err != nil {
		return fmt.Errorf("failed to apply CVT variations: %w", err)
	}
	if err := f.runPrep(); err != nil {
		return fmt.Errorf("failed to run prep program: %w", err)
	}
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
	if metrics, ok := f.loadedMetrics[glyphIndex]; ok {
		return metrics.advance, metrics.lsb, nil
	}
	advance, lsb, err = f.getGlyphMetricsFUnits(glyphIndex)
	if err != nil {
		return 0, 0, err
	}
	return f.scaleFUnitsX(advance), f.scaleFUnitsX(lsb), nil
}

func (f *Face) getGlyphMetricsFUnits(glyphIndex int) (advance int32, lsb int32, err error) {
	if glyphIndex < 0 || glyphIndex >= f.GetNumGlyphs() {
		return 0, 0, fmt.Errorf("glyph index %d out of range", glyphIndex)
	}
	if metricsGlyphIndex, ok := f.compositeMetricsGlyphIndex(glyphIndex, make(map[int]bool)); ok {
		glyphIndex = metricsGlyphIndex
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

func (f *Face) compositeMetricsGlyphIndex(glyphIndex int, active map[int]bool) (int, bool) {
	if glyphIndex < 0 || glyphIndex >= f.GetNumGlyphs() {
		return 0, false
	}
	if active[glyphIndex] {
		return 0, false
	}
	active[glyphIndex] = true
	defer delete(active, glyphIndex)

	glyphStream, err := f.glyphDataStream(glyphIndex)
	if err != nil || glyphStream == nil || glyphStream.Size() < glyphHeaderSize {
		return 0, false
	}
	numberOfContours, err := readInt16(glyphStream, 0)
	if err != nil || numberOfContours >= 0 {
		return 0, false
	}

	currOffset := int64(10)
	selectedGlyph := -1
	for {
		flags, err := readUint16(glyphStream, currOffset)
		if err != nil {
			return 0, false
		}
		currOffset += 2

		subGlyphIndex, err := readUint16(glyphStream, currOffset)
		if err != nil {
			return 0, false
		}
		currOffset += 2

		if flags&ARG_1_AND_2_ARE_WORDS != 0 {
			currOffset += 4
		} else {
			currOffset += 2
		}

		switch {
		case flags&WE_HAVE_A_SCALE != 0:
			currOffset += 2
		case flags&WE_HAVE_AN_X_AND_Y_SCALE != 0:
			currOffset += 4
		case flags&WE_HAVE_A_TWO_BY_TWO != 0:
			currOffset += 8
		}

		if flags&USE_MY_METRICS != 0 {
			selectedGlyph = int(subGlyphIndex)
		}
		if flags&MORE_COMPONENTS == 0 {
			break
		}
	}

	if selectedGlyph < 0 || selectedGlyph >= f.GetNumGlyphs() {
		return 0, false
	}
	if nestedGlyph, ok := f.compositeMetricsGlyphIndex(selectedGlyph, active); ok {
		return nestedGlyph, true
	}
	return selectedGlyph, true
}

func (f *Face) glyphDataStream(glyphIndex int) (api.Stream, error) {
	if glyphIndex < 0 || glyphIndex >= f.GetNumGlyphs() {
		return nil, fmt.Errorf("glyph index %d out of range", glyphIndex)
	}
	locaStream, err := f.GetTable("loca")
	if err != nil {
		return nil, err
	}

	var offset, length uint32
	if f.head.IndexToLocFormat == 0 {
		o1, err := readUint16(locaStream, int64(glyphIndex*2))
		if err != nil {
			return nil, err
		}
		o2, err := readUint16(locaStream, int64((glyphIndex+1)*2))
		if err != nil {
			return nil, err
		}
		if o2 < o1 {
			return nil, fmt.Errorf("loca offsets are not monotonic for glyph %d", glyphIndex)
		}
		offset = uint32(o1) * 2
		length = uint32(o2)*2 - offset
	} else if f.head.IndexToLocFormat == 1 {
		o1, err := readUint32(locaStream, int64(glyphIndex*4))
		if err != nil {
			return nil, err
		}
		o2, err := readUint32(locaStream, int64((glyphIndex+1)*4))
		if err != nil {
			return nil, err
		}
		if o2 < o1 {
			return nil, fmt.Errorf("loca offsets are not monotonic for glyph %d", glyphIndex)
		}
		offset = o1
		length = o2 - offset
	} else {
		return nil, fmt.Errorf("unsupported indexToLocFormat %d", f.head.IndexToLocFormat)
	}

	glyfStream, err := f.GetTable("glyf")
	if err != nil {
		return nil, err
	}
	if int64(offset) > glyfStream.Size() || int64(length) > glyfStream.Size()-int64(offset) {
		return nil, fmt.Errorf("glyph %d range [%d,%d) exceeds glyf table length %d", glyphIndex, offset, offset+length, glyfStream.Size())
	}
	return &tableStream{
		base:   glyfStream,
		offset: int64(offset),
		length: int64(length),
	}, nil
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
			positions[i].X += f.scaleFUnitsX(adjustments[i].X)
			positions[i].Y += f.scaleFUnitsY(adjustments[i].Y)
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

	OVERLAP_SIMPLE = 0x40
)

const (
	glyphHeaderSize             int64 = 10
	maxCompositeGlyphDepth            = 64
	maxCompositeGlyphComponents       = 1024
)

type glyphLoadContext struct {
	active         map[int]bool
	depth          int
	componentCount int
}

type glyphBBox struct {
	xMin int32
	yMin int32
	xMax int32
	yMax int32
}

type glyphMetrics26Dot6 struct {
	advance int32
	lsb     int32
}

type glyphLoadResult struct {
	outline          *core.Outline
	realPointCount   int
	metricPointStart int
	metricPointEnd   int
	metrics          glyphMetrics26Dot6
	hasMetrics       bool
}

func (f *Face) LoadGlyph(glyphIndex int, loadFlags int) (api.GlyphSlot, error) {
	var imagePayload []byte

	if loadFlags&(api.LoadNoBitmap|api.LoadNoScale) == 0 {
		if f.sbix != nil {
			payload, err := f.sbix.GetImage(glyphIndex, embeddedBitmapRequestPPEM(f.yPPEM))
			if err == nil && payload != nil {
				imagePayload = payload
			}
		}
		if imagePayload == nil && f.cblc != nil && f.cbdt != nil {
			payload, err := GetCBLCImageAtPPEM(*f.cblc, *f.cbdt, glyphIndex, embeddedBitmapRequestPPEM(f.yPPEM))
			if err == nil && payload != nil {
				imagePayload = payload
			}
		}
	}

	var decodedImage *api.Image
	if imagePayload != nil && f.sys != nil {
		decoder := f.sys.GetImageDecoder()
		if decoder != nil {
			img, err := decoder.Decode(imagePayload)
			if err == nil {
				decodedImage = img
			}
		}
	}

	if f.cff != nil {
		f.syncCFFVariationCoordinates()
		outline, err := f.cff.LoadGlyphOutlineAt(glyphIndex, f.normalizedVariationCoordinates64())
		if err != nil {
			if decodedImage != nil {
				slot := &GlyphSlot{image: decodedImage}
				f.glyphSlot = slot
				return slot, nil
			}
			return nil, err
		}
		if loadFlags&api.LoadNoScale == 0 {
			f.scaleCFFOutline(outline)
		}
		bitmap, err := f.renderGlyphBitmap(outline, loadFlags)
		if err != nil {
			return nil, err
		}
		metrics, hasMetrics := f.glyphMetricsForLoadFlags(glyphIndex, loadFlags)
		syntheticVertAdvance := int32(0)
		if !f.hasExplicitVerticalMetrics() {
			syntheticVertAdvance = f.syntheticVerticalAdvance(loadFlags)
		}
		slotMetrics, hasSlotMetrics := glyphSlotMetricsFromOutline(outline, realPointCount(outline), metrics, hasMetrics, syntheticVertAdvance)
		if syntheticVertAdvance == 0 {
			f.applyCFFVerticalSlotMetrics(glyphIndex, loadFlags, &slotMetrics)
		}
		slot := &GlyphSlot{outline: outline, bitmap: bitmap, image: decodedImage, slotMetrics: slotMetrics, hasSlotMetrics: hasSlotMetrics}
		f.glyphSlot = slot
		return slot, nil
	}
	result, err := f.loadGlyphInternal(glyphIndex, loadFlags)
	if err != nil {
		if decodedImage != nil {
			slot := &GlyphSlot{image: decodedImage}
			f.glyphSlot = slot
			return slot, nil
		}
		return nil, err
	}
	if !result.hasMetrics {
		if metrics, ok := f.glyphMetricsForLoadFlags(glyphIndex, loadFlags); ok {
			result.metrics = metrics
			result.hasMetrics = true
		}
	}
	slotMetrics, hasSlotMetrics := glyphSlotMetricsFromOutline(result.outline, result.realPointCount, result.metrics, result.hasMetrics, 0)
	if !f.hasExplicitVerticalMetrics() {
		f.applySyntheticVerticalSlotMetrics(loadFlags, &slotMetrics)
	}
	if shouldGridFitSlotMetrics(loadFlags) {
		gridFitSlotMetricsFromOutline(result.outline, result.realPointCount, &slotMetrics)
		if result.hasMetrics {
			result.metrics.advance = roundToPixel26Dot6(slotMetrics.HoriAdvance)
			result.metrics.lsb = slotMetrics.HoriBearingX
			slotMetrics.HoriAdvance = result.metrics.advance
		}
	}
	outline := stripGlyphPhantomsAndApplyOrigin(result.outline, result.realPointCount)
	bitmap, err := f.renderGlyphBitmap(outline, loadFlags)
	if err != nil {
		return nil, err
	}
	slot := &GlyphSlot{
		outline:        outline,
		bitmap:         bitmap,
		image:          decodedImage,
		metrics:        result.metrics,
		hasMetrics:     result.hasMetrics,
		slotMetrics:    slotMetrics,
		hasSlotMetrics: hasSlotMetrics,
	}
	if result.hasMetrics && loadFlags&api.LoadNoScale == 0 {
		f.rememberLoadedMetrics(glyphIndex, result.metrics)
	}
	f.glyphSlot = slot
	return slot, nil
}

func (f *Face) glyphMetricsForLoadFlags(glyphIndex int, loadFlags int) (glyphMetrics26Dot6, bool) {
	advance, lsb, err := f.getGlyphMetricsFUnits(glyphIndex)
	if err != nil {
		return glyphMetrics26Dot6{}, false
	}
	if loadFlags&api.LoadNoScale != 0 {
		return glyphMetrics26Dot6{advance: advance << 6, lsb: lsb << 6}, true
	}
	return glyphMetrics26Dot6{advance: f.scaleFUnitsX(advance), lsb: f.scaleFUnitsX(lsb)}, true
}

func embeddedBitmapRequestPPEM(ppem int) uint16 {
	if ppem <= 0 {
		return 0
	}
	if ppem > 0xffff {
		return 0xffff
	}
	return uint16(ppem)
}

func (f *Face) renderGlyphBitmap(outline *core.Outline, loadFlags int) (api.Bitmap, error) {
	if loadFlags&api.LoadRender == 0 {
		return nil, nil
	}

	mode := renderModeForLoadFlags(loadFlags)
	renderOutline, bitmap, _, ok := core.PrepareBitmapForOutline(outline, realPointCount(outline), mode)
	if !ok {
		return nil, nil
	}
	if renderOutline == nil {
		return bitmap, nil
	}

	var rast api.Rasterizer
	if f.sys != nil {
		rast = f.sys.Rasterizer()
	}
	if rast == nil {
		rast = raster.NewSmoothRasterizer()
	}
	if err := rast.Render(renderOutline, bitmap); err != nil {
		return nil, err
	}
	return bitmap, nil
}

func renderModeForLoadFlags(loadFlags int) api.RenderMode {
	if loadFlags&api.LoadMonochrome != 0 {
		return api.RenderModeMono
	}
	switch loadFlags & api.LoadTargetMask {
	case api.LoadTargetLight:
		return api.RenderModeLight
	case api.LoadTargetMono:
		return api.RenderModeMono
	case api.LoadTargetLCD:
		return api.RenderModeLCD
	case api.LoadTargetLCDV:
		return api.RenderModeLCDV
	}
	if loadFlags&api.LoadRender != 0 {
		return api.RenderModeNormal
	}
	return api.RenderModeNormal
}

func (f *Face) hasExplicitVerticalMetrics() bool {
	return f.vhea.NumOfLongVerMetrics > 0 && len(f.vmtx.VMetrics) > 0
}

func (f *Face) syntheticVerticalAdvance(loadFlags int) int32 {
	advance := f.syntheticVerticalAdvanceFUnits()
	if advance == 0 {
		return 0
	}
	if loadFlags&api.LoadNoScale != 0 {
		return advance << 6
	}
	return f.scaleFUnitsY(advance)
}

func (f *Face) syntheticVerticalAdvanceFUnits() int32 {
	if f.hasBaseOS2 {
		return absInt32(int32(f.os2.STypoAscender) - int32(f.os2.STypoDescender))
	}
	if f.hasBaseHhea && (f.hhea.Ascender != 0 || f.hhea.Descender != 0) {
		return absInt32(int32(f.hhea.Ascender) - int32(f.hhea.Descender))
	}
	return int32(f.GetUnitsPerEm())
}

func absInt32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func glyphSlotMetricsFromOutline(outline *core.Outline, realPoints int, metrics glyphMetrics26Dot6, hasMetrics bool, syntheticVertAdvance int32) (api.GlyphMetrics, bool) {
	if outline == nil {
		return api.GlyphMetrics{}, false
	}

	minX, minY, maxX, maxY, hasBounds := outlineBounds(outline, realPoints)
	slotMetrics := api.GlyphMetrics{}
	if hasBounds {
		slotMetrics.Width = maxX - minX
		slotMetrics.Height = maxY - minY
		slotMetrics.HoriBearingX = minX
		slotMetrics.HoriBearingY = maxY
	}
	if hasMetrics {
		slotMetrics.HoriBearingX = metrics.lsb
		slotMetrics.HoriAdvance = metrics.advance
	}
	if syntheticVertAdvance != 0 {
		slotMetrics.VertBearingX = slotMetrics.HoriBearingX - slotMetrics.HoriAdvance/2
		slotMetrics.VertBearingY = (syntheticVertAdvance - slotMetrics.Height) / 2
		slotMetrics.VertAdvance = syntheticVertAdvance
		return slotMetrics, true
	}
	if phantoms, ok := copyGlyphPhantoms(outline, realPoints); ok {
		slotMetrics.VertBearingX = minX - phantoms[2].X
		slotMetrics.VertBearingY = phantoms[2].Y - maxY
		slotMetrics.VertAdvance = phantoms[2].Y - phantoms[3].Y
	}
	return slotMetrics, true
}

func gridFitSlotMetricsFromOutline(outline *core.Outline, realPoints int, slotMetrics *api.GlyphMetrics) {
	if outline == nil || slotMetrics == nil {
		return
	}
	minX, minY, maxX, maxY, ok := outlineBounds(outline, realPoints)
	if !ok {
		if slotMetrics.HoriAdvance != 0 {
			slotMetrics.HoriAdvance = roundToPixel26Dot6(slotMetrics.HoriAdvance)
		}
		slotMetrics.VertBearingX = floorToPixel26Dot6(slotMetrics.VertBearingX)
		slotMetrics.VertBearingY = floorToPixel26Dot6(slotMetrics.VertBearingY)
		slotMetrics.VertAdvance = roundToPixel26Dot6(slotMetrics.VertAdvance)
		return
	}
	slotMetrics.VertBearingX = floorToPixel26Dot6(slotMetrics.VertBearingX)
	slotMetrics.VertBearingY = floorToPixel26Dot6(slotMetrics.VertBearingY)
	slotMetrics.VertAdvance = roundToPixel26Dot6(slotMetrics.VertAdvance)
	fittedMinX := floorToPixel26Dot6(minX)
	fittedMinY := floorToPixel26Dot6(minY)
	fittedMaxX := ceilToPixel26Dot6(maxX)
	fittedMaxY := ceilToPixel26Dot6(maxY)
	slotMetrics.Width = fittedMaxX - fittedMinX
	slotMetrics.Height = fittedMaxY - fittedMinY
	slotMetrics.HoriBearingX = fittedMinX
	slotMetrics.HoriBearingY = fittedMaxY
	slotMetrics.HoriAdvance = roundToPixel26Dot6(slotMetrics.HoriAdvance)
}

func floorToPixel26Dot6(v int32) int32 {
	return v &^ 63
}

func ceilToPixel26Dot6(v int32) int32 {
	return (v + 63) &^ 63
}

func roundToPixel26Dot6(v int32) int32 {
	return floorToPixel26Dot6(v + 32)
}

func (f *Face) applyCFFVerticalSlotMetrics(glyphIndex int, loadFlags int, slotMetrics *api.GlyphMetrics) {
	if slotMetrics == nil {
		return
	}
	advanceHeight, topSideBearing, ok := f.verticalMetricsFUnits(glyphIndex)
	if !ok {
		return
	}
	slotMetrics.VertBearingX = slotMetrics.HoriBearingX - slotMetrics.HoriAdvance/2
	slotMetrics.VertAdvance = f.scaleFUnitsYForLoadFlags(advanceHeight, loadFlags)
	if f.hasVORG {
		originY := f.vorgOriginYFUnits(glyphIndex)
		slotMetrics.VertBearingY = f.scaleFUnitsYForLoadFlags(originY, loadFlags) - slotMetrics.HoriBearingY
		return
	}
	slotMetrics.VertBearingY = f.scaleFUnitsYForLoadFlags(topSideBearing, loadFlags)
}

func (f *Face) applySyntheticVerticalSlotMetrics(loadFlags int, slotMetrics *api.GlyphMetrics) {
	if slotMetrics == nil {
		return
	}
	advance := f.syntheticVerticalAdvanceFUnits()
	if advance == 0 {
		return
	}
	height := ceil26Dot6(slotMetrics.Height)
	if loadFlags&api.LoadNoScale == 0 {
		// FreeType's TrueType loader reverses scaled 26.6 bbox height with
		// size->metrics.y_scale, which is 64 times the f.yScale used for
		// fUnit-to-26.6 scaling here.
		if yScale := f.yScale << 6; yScale != 0 {
			height = ftmath.DivFix(slotMetrics.Height, yScale)
		}
	}
	topSideBearing := (advance - height) / 2
	slotMetrics.VertBearingX = slotMetrics.HoriBearingX - slotMetrics.HoriAdvance/2
	slotMetrics.VertBearingY = f.scaleFUnitsYForLoadFlags(topSideBearing, loadFlags)
	slotMetrics.VertAdvance = f.scaleFUnitsYForLoadFlags(advance, loadFlags)
}

func outlineBounds(outline *core.Outline, realPoints int) (minX, minY, maxX, maxY int32, ok bool) {
	realPoints = clampRealPointCount(realPoints, outline)
	if realPoints == 0 {
		return 0, 0, 0, 0, false
	}
	minX = outline.Points[0].X
	minY = outline.Points[0].Y
	maxX = minX
	maxY = minY
	for i := 1; i < realPoints; i++ {
		p := outline.Points[i]
		if p.X < minX {
			minX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return minX, minY, maxX, maxY, true
}

func floor26Dot6(v int32) int32 {
	if v >= 0 {
		return v >> 6
	}
	return -int32((int64(-v) + 63) >> 6)
}

func ceil26Dot6(v int32) int32 {
	return -floor26Dot6(-v)
}

func (f *Face) loadGlyphInternal(glyphIndex int, loadFlags int) (*glyphLoadResult, error) {
	return f.loadGlyphInternalWithContext(glyphIndex, loadFlags, &glyphLoadContext{
		active: make(map[int]bool),
	})
}

func (f *Face) loadGlyphInternalWithContext(glyphIndex int, loadFlags int, ctx *glyphLoadContext) (*glyphLoadResult, error) {
	if glyphIndex < 0 || glyphIndex >= f.GetNumGlyphs() {
		return nil, fmt.Errorf("glyph index %d out of range", glyphIndex)
	}
	if ctx == nil {
		ctx = &glyphLoadContext{active: make(map[int]bool)}
	}
	if ctx.active == nil {
		ctx.active = make(map[int]bool)
	}
	if ctx.depth > maxCompositeGlyphDepth {
		return nil, fmt.Errorf("composite glyph depth exceeded at glyph %d", glyphIndex)
	}
	if ctx.active[glyphIndex] {
		return nil, fmt.Errorf("composite glyph cycle references glyph %d", glyphIndex)
	}
	ctx.active[glyphIndex] = true
	defer delete(ctx.active, glyphIndex)

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
		if o2 < o1 {
			return nil, fmt.Errorf("loca offsets are not monotonic for glyph %d", glyphIndex)
		}
		offset = uint32(o1) * 2
		length = uint32(o2)*2 - offset
	} else if f.head.IndexToLocFormat == 1 {
		// Long format (uint32)
		o1, err := readUint32(locaStream, int64(glyphIndex*4))
		if err != nil {
			return nil, err
		}
		o2, err := readUint32(locaStream, int64((glyphIndex+1)*4))
		if err != nil {
			return nil, err
		}
		if o2 < o1 {
			return nil, fmt.Errorf("loca offsets are not monotonic for glyph %d", glyphIndex)
		}
		offset = o1
		length = o2 - offset
	} else {
		return nil, fmt.Errorf("unsupported indexToLocFormat %d", f.head.IndexToLocFormat)
	}

	glyfStream, err := f.GetTable("glyf")
	if err != nil {
		return nil, err
	}
	if int64(offset) > glyfStream.Size() || int64(length) > glyfStream.Size()-int64(offset) {
		return nil, fmt.Errorf("glyph %d range [%d,%d) exceeds glyf table length %d", glyphIndex, offset, offset+length, glyfStream.Size())
	}

	if length == 0 {
		// Empty glyph
		return &glyphLoadResult{outline: &core.Outline{}}, nil
	}

	return f.parseGlyph(glyfStream, int64(offset), int64(length), glyphIndex, loadFlags, ctx)
}

func (f *Face) parseGlyph(s api.Stream, offset int64, length int64, glyphIndex int, loadFlags int, ctx *glyphLoadContext) (*glyphLoadResult, error) {
	if offset < 0 || length < 0 || offset > s.Size() || length > s.Size()-offset {
		return nil, fmt.Errorf("glyph %d range [%d,%d) exceeds stream length %d", glyphIndex, offset, offset+length, s.Size())
	}
	if length < glyphHeaderSize {
		return nil, fmt.Errorf("glyph %d is too short: %d bytes", glyphIndex, length)
	}

	glyphStream := &tableStream{
		base:   s,
		offset: offset,
		length: length,
	}

	numberOfContours, err := readInt16(glyphStream, 0)
	if err != nil {
		return nil, err
	}
	bbox, err := readGlyphBBox(glyphStream, 0)
	if err != nil {
		return nil, err
	}

	if numberOfContours >= 0 {
		return f.parseSimpleGlyph(glyphStream, 0, numberOfContours, glyphIndex, loadFlags, bbox)
	} else {
		return f.parseCompositeGlyph(glyphStream, 0, glyphIndex, loadFlags, ctx, bbox)
	}
}

func readGlyphBBox(s api.Stream, offset int64) (glyphBBox, error) {
	xMin, err := readInt16(s, offset+2)
	if err != nil {
		return glyphBBox{}, err
	}
	yMin, err := readInt16(s, offset+4)
	if err != nil {
		return glyphBBox{}, err
	}
	xMax, err := readInt16(s, offset+6)
	if err != nil {
		return glyphBBox{}, err
	}
	yMax, err := readInt16(s, offset+8)
	if err != nil {
		return glyphBBox{}, err
	}
	return glyphBBox{xMin: int32(xMin), yMin: int32(yMin), xMax: int32(xMax), yMax: int32(yMax)}, nil
}

func (f *Face) getPhantomPoints(glyphIndex int, bbox glyphBBox, loadFlags int) []api.Vector {
	points := f.getPhantomPointsFUnits(glyphIndex, bbox)
	if loadFlags&api.LoadNoScale != 0 {
		return points
	}
	for i := range points {
		points[i].X = f.scale26Dot6X(points[i].X)
		points[i].Y = f.scale26Dot6Y(points[i].Y)
	}
	return points
}

func (f *Face) getPhantomPointsFUnits(glyphIndex int, bbox glyphBBox) []api.Vector {
	advance, lsb, err := f.getGlyphMetricsFUnits(glyphIndex)
	if err != nil {
		advance = 0
		lsb = bbox.xMin
	}
	leftOrigin := bbox.xMin - lsb

	advanceHeight := int32(f.GetUnitsPerEm())
	topOrigin := bbox.yMax
	verticalOriginX := (leftOrigin << 6) + (advance<<6)/2

	if verticalAdvance, topSideBearing, ok := f.verticalMetricsFUnits(glyphIndex); ok {
		advanceHeight = verticalAdvance
		topOrigin = bbox.yMax + topSideBearing
	} else {
		advanceHeight = f.syntheticVerticalAdvanceFUnits()
		topSideBearing := (advanceHeight - (bbox.yMax - bbox.yMin)) / 2
		topOrigin = bbox.yMax + topSideBearing
	}

	return []api.Vector{
		{X: leftOrigin << 6, Y: 0},
		{X: (leftOrigin + advance) << 6, Y: 0},
		{X: verticalOriginX, Y: topOrigin << 6},
		{X: verticalOriginX, Y: (topOrigin - advanceHeight) << 6},
	}
}

func (f *Face) verticalMetricsFUnits(glyphIndex int) (advanceHeight int32, topSideBearing int32, ok bool) {
	if !f.hasExplicitVerticalMetrics() {
		return 0, 0, false
	}
	numVMetrics := int(f.vhea.NumOfLongVerMetrics)
	if numVMetrics <= 0 || numVMetrics > len(f.vmtx.VMetrics) {
		return 0, 0, false
	}
	if glyphIndex < numVMetrics {
		advanceHeight = int32(f.vmtx.VMetrics[glyphIndex].AdvanceHeight)
		topSideBearing = int32(f.vmtx.VMetrics[glyphIndex].TopSideBearing)
	} else {
		advanceHeight = int32(f.vmtx.VMetrics[numVMetrics-1].AdvanceHeight)
		tsbIndex := glyphIndex - numVMetrics
		if tsbIndex >= 0 && tsbIndex < len(f.vmtx.TopSideBearings) {
			topSideBearing = int32(f.vmtx.TopSideBearings[tsbIndex])
		}
	}
	if f.varEngine != nil {
		advanceHeight += f.varEngine.GetAdvanceHeightDelta(glyphIndex)
		topSideBearing += f.varEngine.GetTSBDelta(glyphIndex)
	}
	return advanceHeight, topSideBearing, true
}

func (f *Face) vorgOriginYFUnits(glyphIndex int) int32 {
	originY := int32(f.vorg.DefaultVertOriginY)
	for _, metric := range f.vorg.VertOriginYMetrics {
		if int(metric.GlyphIndex) == glyphIndex {
			originY = int32(metric.VertOriginY)
			break
		}
	}
	if f.varEngine != nil {
		originY += f.varEngine.GetVOrgDelta(glyphIndex)
	}
	return originY
}

func (f *Face) applyGlyphVariation(glyphIndex int, outline *core.Outline) error {
	if f.varEngine == nil || outline == nil {
		return nil
	}
	return f.varEngine.ApplyVariation(glyphIndex, outline)
}

func (f *Face) glyphVariationDeltas(glyphIndex int, points []api.Vector, contours []int) ([]api.Vector, error) {
	if f.varEngine == nil {
		return make([]api.Vector, len(points)), nil
	}
	return f.varEngine.GetGlyphDeltas(glyphIndex, points, contours)
}

func (f *Face) parseSimpleGlyph(s api.Stream, offset int64, numberOfContours int16, glyphIndex int, loadFlags int, bbox glyphBBox) (*glyphLoadResult, error) {
	if numberOfContours <= 0 {
		return &glyphLoadResult{outline: &core.Outline{}}, nil
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

	instructionLengthOffset := offset + headerSize + int64(numberOfContours)*2
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
	if numPoints > 0 && flags[0]&OVERLAP_SIMPLE != 0 {
		outline.Flags |= core.OutlineOverlap
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
	phantomPoints := f.getPhantomPointsFUnits(glyphIndex, bbox)
	outline.Points = append(outline.Points, phantomPoints...)
	outline.Tags = append(outline.Tags, 0, 0, 0, 0)

	if err := f.applyGlyphVariation(glyphIndex, outline); err != nil {
		return nil, err
	}
	unscaledOriginalPoints := copyTTOrusPoints(outline)
	if loadFlags&api.LoadNoScale == 0 {
		f.scaleOutline(outline)
	}

	if len(instructions) > 0 && shouldHintGlyph(loadFlags) {
		_ = f.runGlyphProgram(outline, instructions, loadFlags, unscaledOriginalPoints)
	}

	result := &glyphLoadResult{
		outline:          outline,
		realPointCount:   numPoints,
		metricPointStart: 0,
		metricPointEnd:   numPoints,
	}
	updateGlyphMetricsFromPhantoms(result)
	return result, nil
}

func (f *Face) parseCompositeGlyph(s api.Stream, offset int64, glyphIndex int, loadFlags int, ctx *glyphLoadContext, bbox glyphBBox) (*glyphLoadResult, error) {
	currOffset := offset + 10 // skip header (numberOfContours + bounding box)

	if ctx == nil {
		ctx = &glyphLoadContext{active: make(map[int]bool)}
	}

	componentTotal, err := countCompositeGlyphComponents(s, currOffset)
	if err != nil {
		return nil, err
	}
	variationPoints := make([]api.Vector, componentTotal+4)
	componentDeltas, err := f.glyphVariationDeltas(glyphIndex, variationPoints, nil)
	if err != nil {
		return nil, err
	}

	var finalOutline *core.Outline
	var instructions []byte
	var metricPhantoms []api.Vector
	metricPointStart := -1
	metricPointEnd := -1
	componentIndex := 0

	for {
		ctx.componentCount++
		if ctx.componentCount > maxCompositeGlyphComponents {
			return nil, fmt.Errorf("composite glyph component limit exceeded at glyph %d", glyphIndex)
		}

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

		ctx.depth++
		subResult, err := f.loadGlyphInternalWithContext(int(subGlyphIndex), loadFlags, ctx)
		ctx.depth--
		if err != nil {
			return nil, err
		}
		subOutline := subResult.outline
		if subOutline == nil {
			subOutline = &core.Outline{}
		}
		componentMetricPhantoms, hasComponentMetricPhantoms := copyGlyphPhantoms(subOutline, subResult.realPointCount)

		// Apply transform and initial translation
		var dx, dy int32
		if flags&ARGS_ARE_XY_VALUES != 0 {
			dx = f.scaleFUnitsXForLoadFlags(arg1, loadFlags)
			dy = f.scaleFUnitsYForLoadFlags(arg2, loadFlags)
		}

		for i := range subOutline.Points {
			x := subOutline.Points[i].X
			y := subOutline.Points[i].Y

			// (x * xx + y * xy) >> 14
			nx := int32((int64(x)*int64(xx) + int64(y)*int64(xy)) >> 14)
			ny := int32((int64(x)*int64(yx) + int64(y)*int64(yy)) >> 14)

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
		if componentIndex < len(componentDeltas) {
			delta := componentDeltas[componentIndex]
			vdx := f.scale26Dot6XForLoadFlags(delta.X, loadFlags)
			vdy := f.scale26Dot6YForLoadFlags(delta.Y, loadFlags)
			for i := range subOutline.Points {
				subOutline.Points[i].X += vdx
				subOutline.Points[i].Y += vdy
			}
		}

		mergePointStart := 0
		if finalOutline != nil {
			mergePointStart = len(finalOutline.Points)
		}
		if flags&USE_MY_METRICS != 0 && subResult.hasMetrics {
			if hasComponentMetricPhantoms {
				metricPhantoms = componentMetricPhantoms
				metricPointStart = mergePointStart + subResult.metricPointStart
				metricPointEnd = mergePointStart + subResult.metricPointEnd
			}
		}

		// Strip phantom points from subOutline before merging
		realSubPoints := clampRealPointCount(subResult.realPointCount, subOutline)
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
			finalOutline.Flags |= subOutline.Flags
		}
		if componentIndex == 0 && flags&OVERLAP_COMPOUND != 0 {
			finalOutline.Flags |= core.OutlineOverlap
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
		componentIndex++
	}

	if finalOutline == nil {
		finalOutline = &core.Outline{}
	}

	// Append phantom points for the composite glyph
	finalRealPointCount := realPointCount(finalOutline)
	phantomPoints := metricPhantoms
	if len(phantomPoints) != 4 {
		phantomPoints = f.getPhantomPoints(glyphIndex, bbox, loadFlags)
		metricPointStart = 0
		metricPointEnd = finalRealPointCount
	}
	if len(componentDeltas) >= componentTotal+4 {
		for i := 0; i < 4; i++ {
			delta := componentDeltas[componentTotal+i]
			phantomPoints[i].X += f.scale26Dot6XForLoadFlags(delta.X, loadFlags)
			phantomPoints[i].Y += f.scale26Dot6YForLoadFlags(delta.Y, loadFlags)
		}
	}
	finalOutline.Points = append(finalOutline.Points, phantomPoints...)
	finalOutline.Tags = append(finalOutline.Tags, 0, 0, 0, 0)

	if len(instructions) > 0 && shouldHintGlyph(loadFlags) {
		_ = f.runGlyphProgram(finalOutline, instructions, loadFlags, nil)
	}

	result := &glyphLoadResult{
		outline:          finalOutline,
		realPointCount:   finalRealPointCount,
		metricPointStart: metricPointStart,
		metricPointEnd:   metricPointEnd,
	}
	updateGlyphMetricsFromPhantoms(result)
	return result, nil
}

func countCompositeGlyphComponents(s api.Stream, offset int64) (int, error) {
	count := 0
	currOffset := offset
	for {
		flags, err := readUint16(s, currOffset)
		if err != nil {
			return 0, err
		}
		currOffset += 2
		if _, err := readUint16(s, currOffset); err != nil {
			return 0, err
		}
		currOffset += 2

		if flags&ARG_1_AND_2_ARE_WORDS != 0 {
			currOffset += 4
		} else {
			currOffset += 2
		}

		switch {
		case flags&WE_HAVE_A_SCALE != 0:
			currOffset += 2
		case flags&WE_HAVE_AN_X_AND_Y_SCALE != 0:
			currOffset += 4
		case flags&WE_HAVE_A_TWO_BY_TWO != 0:
			currOffset += 8
		}

		count++
		if flags&MORE_COMPONENTS == 0 {
			return count, nil
		}
	}
}

func (f *Face) rememberLoadedMetrics(glyphIndex int, metrics glyphMetrics26Dot6) {
	if f.loadedMetrics == nil {
		f.loadedMetrics = make(map[int]glyphMetrics26Dot6)
	}
	f.loadedMetrics[glyphIndex] = metrics
}

func stripGlyphPhantoms(outline *core.Outline, realPointCount int) *core.Outline {
	if outline == nil {
		return nil
	}
	realPointCount = clampRealPointCount(realPointCount, outline)
	outline.Points = outline.Points[:realPointCount]
	outline.Tags = outline.Tags[:realPointCount]
	return outline
}

func stripGlyphPhantomsAndApplyOrigin(outline *core.Outline, realPointCount int) *core.Outline {
	if outline == nil {
		return nil
	}
	phantoms, ok := copyGlyphPhantoms(outline, realPointCount)
	outline = stripGlyphPhantoms(outline, realPointCount)
	if !ok {
		return outline
	}
	translateOutline(outline, -phantoms[0].X, 0)
	return outline
}

func translateOutline(outline *core.Outline, dx, dy int32) {
	if outline == nil || (dx == 0 && dy == 0) {
		return
	}
	for i := range outline.Points {
		outline.Points[i].X += dx
		outline.Points[i].Y += dy
	}
}

func realPointCount(outline *core.Outline) int {
	if outline == nil || len(outline.Contours) == 0 {
		return 0
	}
	n := outline.Contours[len(outline.Contours)-1] + 1
	return clampRealPointCount(n, outline)
}

func clampRealPointCount(n int, outline *core.Outline) int {
	if outline == nil || n < 0 {
		return 0
	}
	if n > len(outline.Points) {
		return len(outline.Points)
	}
	if n > len(outline.Tags) {
		return len(outline.Tags)
	}
	return n
}

func copyGlyphPhantoms(outline *core.Outline, realPointCount int) ([]api.Vector, bool) {
	if outline == nil {
		return nil, false
	}
	realPointCount = clampRealPointCount(realPointCount, outline)
	if len(outline.Points)-realPointCount < 4 {
		return nil, false
	}
	phantoms := make([]api.Vector, 4)
	copy(phantoms, outline.Points[realPointCount:realPointCount+4])
	return phantoms, true
}

func updateGlyphMetricsFromPhantoms(result *glyphLoadResult) {
	if result == nil || result.outline == nil {
		return
	}
	phantoms, ok := copyGlyphPhantoms(result.outline, result.realPointCount)
	if !ok {
		return
	}
	minX, ok := outlineMinXRange(result.outline, 0, result.realPointCount)
	if !ok {
		return
	}
	result.metrics = glyphMetrics26Dot6{
		advance: phantoms[1].X - phantoms[0].X,
		lsb:     minX - phantoms[0].X,
	}
	result.hasMetrics = true
}

func outlineMinXRange(outline *core.Outline, start, end int) (int32, bool) {
	if outline == nil || start < 0 || end <= start || start >= len(outline.Points) {
		return 0, false
	}
	if end > len(outline.Points) {
		end = len(outline.Points)
	}
	minX := outline.Points[start].X
	for i := start + 1; i < end; i++ {
		if outline.Points[i].X < minX {
			minX = outline.Points[i].X
		}
	}
	return minX, true
}

func (f *Face) GetGlyphSlot() api.GlyphSlot {
	return f.glyphSlot
}

func (f *Face) GetColorLayers(glyphIndex int) ([]color.Layer, error) {
	return f.GetColorLayersForPalette(glyphIndex, 0)
}

func (f *Face) GetColorLayersForPalette(glyphIndex int, paletteIndex int) ([]color.Layer, error) {
	if f == nil || f.colr == nil {
		return nil, nil
	}
	if glyphIndex < 0 || glyphIndex > 0xffff {
		return nil, errors.New("invalid glyph index")
	}
	record, ok := f.colr.BaseGlyphRecords[uint16(glyphIndex)]
	if !ok {
		return nil, nil
	}

	end := int(record.FirstLayerIndex) + int(record.NumLayers)
	if end > len(f.colr.LayerRecords) {
		return nil, errors.New("invalid layer index in COLR table")
	}

	layers := make([]color.Layer, int(record.NumLayers))
	for i := 0; i < int(record.NumLayers); i++ {
		lr := f.colr.LayerRecords[int(record.FirstLayerIndex)+i]
		layerColor := color.RGBA{R: 0, G: 0, B: 0, A: 255}
		if lr.PaletteIndex != 0xffff && f.cpal != nil {
			if c, ok := f.cpal.Color(paletteIndex, lr.PaletteIndex); ok {
				layerColor = c
			}
		}
		layers[i] = color.Layer{
			GlyphID:      lr.GlyphID,
			Color:        layerColor,
			PaletteIndex: lr.PaletteIndex,
		}
	}
	return layers, nil
}

var _ api.Driver = (*loader)(nil)
var _ api.Face = (*Face)(nil)
var _ api.GlyphSlotMetricsProvider = (*GlyphSlot)(nil)
