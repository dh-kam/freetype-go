package cff

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

// Index represents a CFF INDEX structure.
type Index struct {
	Count   uint16
	OffSize uint8
	Offsets []uint32
	Data    []byte
}

// Get returns the i-th object in the INDEX.
func (idx *Index) Get(i int) ([]byte, error) {
	if i < 0 || i >= int(idx.Count) {
		return nil, errors.New("index out of range")
	}
	start := idx.Offsets[i] - 1
	end := idx.Offsets[i+1] - 1
	if start > end || end > uint32(len(idx.Data)) {
		return nil, errors.New("invalid offset in index")
	}
	return idx.Data[start:end], nil
}

// CFF represents the Compact Font Format table.
type CFF struct {
	Major   uint8
	Minor   uint8
	HdrSize uint8
	OffSize uint8 // Absolute offset size

	TopDictSize uint16
	MaxStack    int

	NameIndex        Index
	TopDictIndex     Index
	StringIndex      Index
	GlobalSubrIndex  Index
	CharStringsIndex Index
	LocalSubrIndex   Index
	FDArrayIndex     Index

	TopDict   map[int][]float64
	FontDicts []FontDict
	FDSelect  *FDSelect

	VariationStore *VariationStore
	Stream         api.Stream

	variationCoords []float64
}

// FontDict represents a CFF/CFF2 Font DICT and its corresponding Private DICT.
type FontDict struct {
	Dict        map[int][]float64
	PrivateDict PrivateDict
}

// PrivateDict stores the subset of Private DICT data needed by the CFF loader.
type PrivateDict struct {
	Dict           map[int][]float64
	LocalSubrIndex Index
	VSIndex        int
}

// VariationRegionAxis stores one axis coordinate triple for a variation region.
type VariationRegionAxis struct {
	Start float64
	Peak  float64
	End   float64
}

// VariationRegion stores the per-axis support for one variation region.
type VariationRegion struct {
	Axes []VariationRegionAxis
}

// ItemVariationData stores a parsed ItemVariationData subtable.
type ItemVariationData struct {
	ItemCount      uint16
	LongWords      bool
	WordDeltaCount uint16
	RegionIndexes  []uint16
	DeltaSets      [][]int32
}

// VariationStore stores the CFF2 VariationStore wrapper and parsed
// ItemVariationStore data needed for blend-vector calculation.
type VariationStore struct {
	Offset      int64
	Length      uint16
	Data        []byte
	AxisCount   uint16
	RegionCount uint16

	Regions                   []VariationRegion
	ItemVariationData         []ItemVariationData
	ItemVariationRegionCounts []uint16
}

// ActiveRegionCount returns the number of blend deltas selected by vsindex.
func (vs *VariationStore) ActiveRegionCount(index int) (int, bool) {
	if vs == nil || index < 0 {
		return 0, false
	}
	if index < len(vs.ItemVariationData) {
		return len(vs.ItemVariationData[index].RegionIndexes), true
	}
	if index >= len(vs.ItemVariationRegionCounts) {
		return 0, false
	}
	return int(vs.ItemVariationRegionCounts[index]), true
}

// RegionScalar returns the scalar for a global region at normalized coordinates.
func (vs *VariationStore) RegionScalar(regionIndex int, coords []float64) (float64, bool) {
	if vs == nil || regionIndex < 0 || regionIndex >= len(vs.Regions) {
		return 0, false
	}
	return variationRegionScalar(vs.Regions[regionIndex], coords), true
}

// BlendVector returns the vsindex-selected blend vector at normalized coordinates.
func (vs *VariationStore) BlendVector(index int, coords []float64) ([]float64, bool) {
	if vs == nil || index < 0 {
		return nil, false
	}
	if index >= len(vs.ItemVariationData) {
		k, ok := vs.ActiveRegionCount(index)
		if !ok {
			return nil, false
		}
		return make([]float64, k), true
	}

	data := vs.ItemVariationData[index]
	vector := make([]float64, len(data.RegionIndexes))
	for i, regionIndex := range data.RegionIndexes {
		scalar, ok := vs.RegionScalar(int(regionIndex), coords)
		if !ok {
			return nil, false
		}
		vector[i] = scalar
	}
	return vector, true
}

// FDSelect maps glyph IDs to CFF/CFF2 Font DICT indices.
type FDSelect struct {
	Format  uint8
	GlyphFD []uint16
}

// FDIndex returns the Font DICT index for glyphIndex.
func (s *FDSelect) FDIndex(glyphIndex int) (int, error) {
	if s == nil {
		return 0, nil
	}
	if glyphIndex < 0 || glyphIndex >= len(s.GlyphFD) {
		return 0, errors.New("glyph index out of range in FDSelect")
	}
	return int(s.GlyphFD[glyphIndex]), nil
}

const (
	opCharStrings    = 17
	opPrivate        = 18
	opLocalSubrs     = 19
	opVariationStore = 24
	opCFF2MaxStack   = 25
	opFDArray        = 12<<8 | 36
	opFDSelect       = 12<<8 | 37
	opPrivateVSIndex = 22
)

// ParseCFF parses the CFF table from the given stream and offset.
func ParseCFF(stream api.Stream, offset int64) (*CFF, error) {
	cff := &CFF{Stream: stream}

	// Read Header
	major, err := readByte(stream, offset)
	if err != nil {
		return nil, err
	}
	minor, err := readByte(stream, offset+1)
	if err != nil {
		return nil, err
	}
	hdrSize, err := readByte(stream, offset+2)
	if err != nil {
		return nil, err
	}

	cff.Major = major
	cff.Minor = minor
	cff.HdrSize = hdrSize

	if major == 2 {
		return parseCFF2(stream, offset, cff)
	}

	if hdrSize < 4 {
		return nil, fmt.Errorf("invalid CFF header size %d", hdrSize)
	}
	offSize, err := readByte(stream, offset+3)
	if err != nil {
		return nil, err
	}
	cff.OffSize = offSize

	curr := offset + int64(hdrSize)

	// Name INDEX
	nameIndex, next, err := parseIndex(stream, curr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Name INDEX: %v", err)
	}
	cff.NameIndex = *nameIndex
	curr = next

	// Top DICT INDEX
	topDictIndex, next, err := parseIndex(stream, curr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Top DICT INDEX: %v", err)
	}
	cff.TopDictIndex = *topDictIndex
	curr = next

	// String INDEX
	stringIndex, next, err := parseIndex(stream, curr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse String INDEX: %v", err)
	}
	cff.StringIndex = *stringIndex
	curr = next

	// Global Subr INDEX
	globalSubrIndex, _, err := parseIndex(stream, curr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Global Subr INDEX: %v", err)
	}
	cff.GlobalSubrIndex = *globalSubrIndex

	// Parse the first Top DICT
	if cff.TopDictIndex.Count > 0 {
		data, err := cff.TopDictIndex.Get(0)
		if err != nil {
			return nil, fmt.Errorf("failed to get first Top DICT: %v", err)
		}
		topDict, err := ParseDict(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Top DICT: %v", err)
		}
		cff.TopDict = topDict

		// Parse CharStrings INDEX
		if csOff, ok := topDict[opCharStrings]; ok && len(csOff) > 0 {
			charStringsOffset, err := dictUint(csOff, 0, "CharStrings")
			if err != nil {
				return nil, err
			}
			csIdx, _, err := parseIndex(stream, offset+charStringsOffset)
			if err != nil {
				return nil, fmt.Errorf("failed to parse CharStrings INDEX: %v", err)
			}
			cff.CharStringsIndex = *csIdx
		}

		// Parse Private DICT and Local Subrs (Simplified)
		if fdArrayOps, ok := topDict[opFDArray]; ok && len(fdArrayOps) > 0 {
			if err := cff.parseCFF1FDArrayAndSelect(stream, offset, topDict); err != nil {
				return nil, err
			}
		} else if priv, ok := topDict[opPrivate]; ok && len(priv) >= 2 {
			privateDict, err := parsePrivateDict(stream, offset, priv, 2)
			if err != nil {
				return nil, err
			}
			cff.LocalSubrIndex = privateDict.LocalSubrIndex
			cff.FontDicts = []FontDict{{PrivateDict: privateDict}}
		}
	}

	return cff, nil
}

func (c *CFF) parseCFF1FDArrayAndSelect(stream api.Stream, offset int64, topDict map[int][]float64) error {
	fdArrayOffset, err := requiredDictUint(topDict, opFDArray, "CFF FDArray")
	if err != nil {
		return err
	}
	fdArray, _, err := parseIndex(stream, offset+fdArrayOffset)
	if err != nil {
		return fmt.Errorf("failed to parse CFF FDArray INDEX: %v", err)
	}
	if fdArray.Count == 0 {
		return errors.New("CFF FDArray INDEX is empty")
	}
	c.FDArrayIndex = *fdArray
	c.FontDicts = make([]FontDict, fdArray.Count)
	for i := 0; i < int(fdArray.Count); i++ {
		fdData, err := fdArray.Get(i)
		if err != nil {
			return fmt.Errorf("failed to get CFF Font DICT %d: %v", i, err)
		}
		fdDict, err := ParseDict(fdData)
		if err != nil {
			return fmt.Errorf("failed to parse CFF Font DICT %d: %v", i, err)
		}
		fontDict := FontDict{Dict: fdDict}
		if priv, ok := fdDict[opPrivate]; ok && len(priv) >= 2 {
			privateDict, err := parsePrivateDict(stream, offset, priv, 2)
			if err != nil {
				return fmt.Errorf("failed to parse CFF Private DICT %d: %v", i, err)
			}
			fontDict.PrivateDict = privateDict
		}
		c.FontDicts[i] = fontDict
	}

	if fdSelectOps, ok := topDict[opFDSelect]; ok && len(fdSelectOps) > 0 {
		fdSelectOffset, err := dictUint(fdSelectOps, 0, "CFF FDSelect")
		if err != nil {
			return err
		}
		fdSelect, err := parseFDSelect(stream, offset+fdSelectOffset, int(c.CharStringsIndex.Count), int(c.FDArrayIndex.Count))
		if err != nil {
			return fmt.Errorf("failed to parse CFF FDSelect: %v", err)
		}
		c.FDSelect = fdSelect
	} else if c.FDArrayIndex.Count > 1 {
		return errors.New("CFF FDSelect missing for multiple Font DICTs")
	}

	return nil
}

func parseCFF2(stream api.Stream, offset int64, cff *CFF) (*CFF, error) {
	if cff.HdrSize < 5 {
		return nil, fmt.Errorf("invalid CFF2 header size %d", cff.HdrSize)
	}
	topDictSize, err := readUint16(stream, offset+3)
	if err != nil {
		return nil, err
	}
	cff.TopDictSize = topDictSize

	topDictData, err := readData(stream, offset+int64(cff.HdrSize), int64(topDictSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read CFF2 Top DICT: %v", err)
	}
	topDict, err := ParseDict(topDictData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CFF2 Top DICT: %v", err)
	}
	cff.TopDict = topDict
	maxStack, err := parseCFF2MaxStack(topDict)
	if err != nil {
		return nil, err
	}
	cff.MaxStack = maxStack

	globalSubrOffset := offset + int64(cff.HdrSize) + int64(topDictSize)
	globalSubrs, _, err := parseIndex32(stream, globalSubrOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CFF2 Global Subr INDEX: %v", err)
	}
	cff.GlobalSubrIndex = *globalSubrs

	charStringsOffset, err := requiredDictUint(topDict, opCharStrings, "CFF2 CharStrings")
	if err != nil {
		return nil, err
	}
	charStrings, _, err := parseIndex32(stream, offset+charStringsOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CFF2 CharStrings INDEX: %v", err)
	}
	cff.CharStringsIndex = *charStrings

	fdArrayOffset, err := requiredDictUint(topDict, opFDArray, "CFF2 FDArray")
	if err != nil {
		return nil, err
	}
	fdArray, _, err := parseIndex32(stream, offset+fdArrayOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CFF2 FDArray INDEX: %v", err)
	}
	cff.FDArrayIndex = *fdArray
	if fdArray.Count == 0 {
		return nil, errors.New("CFF2 FDArray INDEX is empty")
	}

	cff.FontDicts = make([]FontDict, fdArray.Count)
	for i := 0; i < int(fdArray.Count); i++ {
		fdData, err := fdArray.Get(i)
		if err != nil {
			return nil, fmt.Errorf("failed to get CFF2 Font DICT %d: %v", i, err)
		}
		fdDict, err := ParseDict(fdData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CFF2 Font DICT %d: %v", i, err)
		}
		priv, ok := fdDict[opPrivate]
		if !ok {
			return nil, fmt.Errorf("CFF2 Font DICT %d missing Private DICT", i)
		}
		privateDict, err := parsePrivateDict(stream, offset, priv, 4)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CFF2 Private DICT %d: %v", i, err)
		}
		cff.FontDicts[i] = FontDict{
			Dict:        fdDict,
			PrivateDict: privateDict,
		}
	}

	if vstore, ok := topDict[opVariationStore]; ok && len(vstore) > 0 {
		vstoreOffset, err := dictUint(vstore, 0, "CFF2 VariationStore")
		if err != nil {
			return nil, err
		}
		variationStore, err := parseVariationStore(stream, offset+vstoreOffset)
		if err != nil {
			return nil, fmt.Errorf("failed to parse CFF2 VariationStore: %v", err)
		}
		cff.VariationStore = variationStore
	}

	if fdSelectOps, ok := topDict[opFDSelect]; ok && len(fdSelectOps) > 0 {
		fdSelectOffset, err := dictUint(fdSelectOps, 0, "CFF2 FDSelect")
		if err != nil {
			return nil, err
		}
		fdSelect, err := parseFDSelect(stream, offset+fdSelectOffset, int(cff.CharStringsIndex.Count), int(cff.FDArrayIndex.Count))
		if err != nil {
			return nil, fmt.Errorf("failed to parse CFF2 FDSelect: %v", err)
		}
		cff.FDSelect = fdSelect
	} else if cff.FDArrayIndex.Count > 1 {
		return nil, errors.New("CFF2 FDSelect missing for multiple Font DICTs")
	}

	return cff, nil
}

func parsePrivateDict(stream api.Stream, tableOffset int64, operands []float64, indexCountSize int) (PrivateDict, error) {
	privSize, err := dictUint(operands, 0, "Private DICT size")
	if err != nil {
		return PrivateDict{}, err
	}
	privOff, err := dictUint(operands, 1, "Private DICT offset")
	if err != nil {
		return PrivateDict{}, err
	}
	privateDict := PrivateDict{VSIndex: 0}
	if privSize == 0 {
		privateDict.Dict = map[int][]float64{}
		return privateDict, nil
	}

	privData, err := readData(stream, tableOffset+privOff, privSize)
	if err != nil {
		return PrivateDict{}, fmt.Errorf("failed to read Private DICT: %v", err)
	}
	privDict, err := ParseDict(privData)
	if err != nil {
		return PrivateDict{}, fmt.Errorf("failed to parse Private DICT: %v", err)
	}
	privateDict.Dict = privDict

	if vsIndex, ok := privDict[opPrivateVSIndex]; ok && len(vsIndex) > 0 {
		v, err := dictUint(vsIndex, 0, "Private DICT vsindex")
		if err != nil {
			return PrivateDict{}, err
		}
		privateDict.VSIndex = int(v)
	}

	if subrsOff, ok := privDict[opLocalSubrs]; ok && len(subrsOff) > 0 {
		localSubrOffset, err := dictUint(subrsOff, 0, "Local Subr INDEX")
		if err != nil {
			return PrivateDict{}, err
		}
		lsIdx, _, err := parseIndexWithCountSize(stream, tableOffset+privOff+localSubrOffset, indexCountSize)
		if err != nil {
			return PrivateDict{}, fmt.Errorf("failed to parse Local Subr INDEX: %v", err)
		}
		privateDict.LocalSubrIndex = *lsIdx
	}

	return privateDict, nil
}

// SetVariationCoordinates sets normalized variation coordinates for CFF2 glyph loading.
func (c *CFF) SetVariationCoordinates(coords []float64) {
	c.variationCoords = append(c.variationCoords[:0], coords...)
}

// VariationCoordinates returns the normalized variation coordinates used by LoadGlyphOutline.
func (c *CFF) VariationCoordinates() []float64 {
	return append([]float64(nil), c.variationCoords...)
}

// LoadGlyphOutlineAt loads a glyph outline at normalized variation coordinates.
func (c *CFF) LoadGlyphOutlineAt(glyphIndex int, coords []float64) (*core.Outline, error) {
	return c.loadGlyphOutline(glyphIndex, coords)
}

func (c *CFF) LoadGlyphOutline(glyphIndex int) (*core.Outline, error) {
	return c.loadGlyphOutline(glyphIndex, c.variationCoords)
}

func (c *CFF) loadGlyphOutline(glyphIndex int, variationCoords []float64) (*core.Outline, error) {
	data, err := c.CharStringsIndex.Get(glyphIndex)
	if err != nil {
		return nil, err
	}

	localSubrs := &c.LocalSubrIndex
	opts := charStringDecodeOptions{
		globalSubrs: &c.GlobalSubrIndex,
		localSubrs:  localSubrs,
	}
	if c.Major == 2 {
		opts.maxStack = c.MaxStack
		if opts.maxStack == 0 {
			opts.maxStack = defaultCFF2MaxStack
		}
		fdIndex := 0
		if c.FDSelect != nil {
			fdIndex, err = c.FDSelect.FDIndex(glyphIndex)
			if err != nil {
				return nil, err
			}
		}
		if fdIndex < 0 || fdIndex >= len(c.FontDicts) {
			return nil, fmt.Errorf("CFF2 FD index %d out of range", fdIndex)
		}
		privateDict := &c.FontDicts[fdIndex].PrivateDict
		opts.localSubrs = &privateDict.LocalSubrIndex
		opts.defaultVSIndex = privateDict.VSIndex
		opts.variationStore = c.VariationStore
		opts.variationCoords = variationCoords
	} else if c.FDArrayIndex.Count > 0 {
		fdIndex := 0
		if c.FDSelect != nil {
			fdIndex, err = c.FDSelect.FDIndex(glyphIndex)
			if err != nil {
				return nil, err
			}
		}
		if fdIndex < 0 || fdIndex >= len(c.FontDicts) {
			return nil, fmt.Errorf("CFF FD index %d out of range", fdIndex)
		}
		opts.localSubrs = &c.FontDicts[fdIndex].PrivateDict.LocalSubrIndex
	}

	result, err := decodeCharString(data, opts)
	if err != nil {
		return nil, err
	}
	return result.outline, nil
}

func parseIndex(stream api.Stream, offset int64) (*Index, int64, error) {
	return parseIndexWithCountSize(stream, offset, 2)
}

func parseIndex32(stream api.Stream, offset int64) (*Index, int64, error) {
	return parseIndexWithCountSize(stream, offset, 4)
}

func parseIndexWithCountSize(stream api.Stream, offset int64, countSize int) (*Index, int64, error) {
	var count uint32
	var err error
	switch countSize {
	case 2:
		var c uint16
		c, err = readUint16(stream, offset)
		count = uint32(c)
	case 4:
		count, err = readUint32(stream, offset)
	default:
		return nil, 0, fmt.Errorf("invalid INDEX count size %d", countSize)
	}
	if err != nil {
		return nil, 0, err
	}

	if count == 0 {
		return &Index{Count: 0}, offset + int64(countSize), nil
	}
	if count > uint32(^uint16(0)) {
		return nil, 0, fmt.Errorf("INDEX count %d exceeds supported maximum", count)
	}

	offSize, err := readByte(stream, offset+int64(countSize))
	if err != nil {
		return nil, 0, err
	}

	if offSize < 1 || offSize > 4 {
		return nil, 0, fmt.Errorf("invalid offSize %d in INDEX", offSize)
	}

	offsets := make([]uint32, int(count)+1)
	for i := 0; i <= int(count); i++ {
		off, err := readOffset(stream, offset+int64(countSize)+1+int64(i)*int64(offSize), offSize)
		if err != nil {
			return nil, 0, err
		}
		if off == 0 {
			return nil, 0, fmt.Errorf("invalid zero offset %d in INDEX", i)
		}
		if i > 0 && off < offsets[i-1] {
			return nil, 0, fmt.Errorf("non-monotonic offset %d in INDEX", i)
		}
		offsets[i] = off
	}

	dataSize := offsets[count] - 1
	dataOffset := offset + int64(countSize) + 1 + int64(count+1)*int64(offSize)
	data, err := readData(stream, dataOffset, int64(dataSize))
	if err != nil {
		return nil, 0, err
	}

	return &Index{
		Count:   uint16(count),
		OffSize: offSize,
		Offsets: offsets,
		Data:    data,
	}, dataOffset + int64(dataSize), nil
}

func requiredDictUint(dict map[int][]float64, op int, name string) (int64, error) {
	operands, ok := dict[op]
	if !ok || len(operands) == 0 {
		return 0, fmt.Errorf("%s offset missing", name)
	}
	return dictUint(operands, 0, name)
}

func parseCFF2MaxStack(dict map[int][]float64) (int, error) {
	operands, ok := dict[opCFF2MaxStack]
	if !ok {
		return defaultCFF2MaxStack, nil
	}
	if len(operands) != 1 {
		return 0, errors.New("invalid CFF2 maxstack operand count")
	}
	v, err := dictUint(operands, 0, "CFF2 maxstack")
	if err != nil {
		return 0, err
	}
	if v < defaultCFF2MaxStack || v > maxCFF2ArgumentStack {
		return 0, errors.New("CFF2 maxstack out of range")
	}
	return int(v), nil
}

func dictUint(operands []float64, index int, name string) (int64, error) {
	if index < 0 || index >= len(operands) {
		return 0, fmt.Errorf("%s operand %d missing", name, index)
	}
	v := operands[index]
	if v < 0 {
		return 0, fmt.Errorf("%s is negative", name)
	}
	iv := int64(v)
	if float64(iv) != v {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return iv, nil
}

func parseVariationStore(stream api.Stream, offset int64) (*VariationStore, error) {
	length, err := readUint16(stream, offset)
	if err != nil {
		return nil, err
	}
	data, err := readData(stream, offset+2, int64(length))
	if err != nil {
		return nil, err
	}

	vs := &VariationStore{
		Offset: offset,
		Length: length,
		Data:   data,
	}
	if err := parseItemVariationStore(vs); err != nil {
		return nil, err
	}
	return vs, nil
}

func parseItemVariationStore(vs *VariationStore) error {
	data := vs.Data
	if len(data) < 8 {
		return io.ErrUnexpectedEOF
	}
	format := binary.BigEndian.Uint16(data[0:2])
	if format != 1 {
		return fmt.Errorf("unsupported ItemVariationStore format %d", format)
	}
	regionListOffset := int(binary.BigEndian.Uint32(data[2:6]))
	itemVariationDataCount := int(binary.BigEndian.Uint16(data[6:8]))
	itemOffsetsEnd := 8 + itemVariationDataCount*4
	if itemOffsetsEnd > len(data) {
		return io.ErrUnexpectedEOF
	}
	if regionListOffset+4 > len(data) {
		return io.ErrUnexpectedEOF
	}

	axisCount := int(binary.BigEndian.Uint16(data[regionListOffset : regionListOffset+2]))
	regionCount := int(binary.BigEndian.Uint16(data[regionListOffset+2 : regionListOffset+4]))
	regionListEnd := regionListOffset + 4 + axisCount*regionCount*6
	if regionListEnd > len(data) {
		return io.ErrUnexpectedEOF
	}
	vs.AxisCount = uint16(axisCount)
	vs.RegionCount = uint16(regionCount)
	vs.Regions = make([]VariationRegion, regionCount)
	p := regionListOffset + 4
	for i := 0; i < regionCount; i++ {
		region := VariationRegion{Axes: make([]VariationRegionAxis, axisCount)}
		for j := 0; j < axisCount; j++ {
			region.Axes[j] = VariationRegionAxis{
				Start: f2Dot14(data[p : p+2]),
				Peak:  f2Dot14(data[p+2 : p+4]),
				End:   f2Dot14(data[p+4 : p+6]),
			}
			p += 6
		}
		vs.Regions[i] = region
	}

	vs.ItemVariationData = make([]ItemVariationData, itemVariationDataCount)
	vs.ItemVariationRegionCounts = make([]uint16, itemVariationDataCount)
	for i := 0; i < itemVariationDataCount; i++ {
		itemOffset := int(binary.BigEndian.Uint32(data[8+i*4 : 12+i*4]))
		if itemOffset == 0 {
			continue
		}
		if itemOffset+6 > len(data) {
			return io.ErrUnexpectedEOF
		}
		itemData, err := parseItemVariationData(data[itemOffset:], regionCount)
		if err != nil {
			return fmt.Errorf("failed to parse ItemVariationData %d: %v", i, err)
		}
		vs.ItemVariationData[i] = itemData
		vs.ItemVariationRegionCounts[i] = uint16(len(itemData.RegionIndexes))
	}
	return nil
}

func parseItemVariationData(data []byte, regionCount int) (ItemVariationData, error) {
	if len(data) < 6 {
		return ItemVariationData{}, io.ErrUnexpectedEOF
	}
	itemCount := int(binary.BigEndian.Uint16(data[0:2]))
	wordDeltaPacked := binary.BigEndian.Uint16(data[2:4])
	regionIndexCount := int(binary.BigEndian.Uint16(data[4:6]))
	longWords := wordDeltaPacked&0x8000 != 0
	wordDeltaCount := int(wordDeltaPacked & 0x7fff)
	if wordDeltaCount > regionIndexCount {
		return ItemVariationData{}, fmt.Errorf("wordDeltaCount %d exceeds regionIndexCount %d", wordDeltaCount, regionIndexCount)
	}

	regionIndexesEnd := 6 + regionIndexCount*2
	if regionIndexesEnd > len(data) {
		return ItemVariationData{}, io.ErrUnexpectedEOF
	}
	itemData := ItemVariationData{
		ItemCount:      uint16(itemCount),
		LongWords:      longWords,
		WordDeltaCount: uint16(wordDeltaCount),
		RegionIndexes:  make([]uint16, regionIndexCount),
	}
	for j := 0; j < regionIndexCount; j++ {
		regionIndex := binary.BigEndian.Uint16(data[6+j*2 : 8+j*2])
		if int(regionIndex) >= regionCount {
			return ItemVariationData{}, fmt.Errorf("VariationStore region index %d out of range", regionIndex)
		}
		itemData.RegionIndexes[j] = regionIndex
	}

	wordBytes := 2
	shortBytes := 1
	if longWords {
		wordBytes = 4
		shortBytes = 2
	}
	rowSize := wordDeltaCount*wordBytes + (regionIndexCount-wordDeltaCount)*shortBytes
	deltaDataEnd := regionIndexesEnd + itemCount*rowSize
	if deltaDataEnd > len(data) {
		return ItemVariationData{}, io.ErrUnexpectedEOF
	}

	p := regionIndexesEnd
	itemData.DeltaSets = make([][]int32, itemCount)
	for row := 0; row < itemCount; row++ {
		deltas := make([]int32, regionIndexCount)
		for col := 0; col < regionIndexCount; col++ {
			if col < wordDeltaCount {
				if longWords {
					deltas[col] = int32(binary.BigEndian.Uint32(data[p : p+4]))
					p += 4
				} else {
					deltas[col] = int32(int16(binary.BigEndian.Uint16(data[p : p+2])))
					p += 2
				}
			} else {
				if longWords {
					deltas[col] = int32(int16(binary.BigEndian.Uint16(data[p : p+2])))
					p += 2
				} else {
					deltas[col] = int32(int8(data[p]))
					p++
				}
			}
		}
		itemData.DeltaSets[row] = deltas
	}

	return itemData, nil
}

func variationRegionScalar(region VariationRegion, coords []float64) float64 {
	scalar := 1.0
	for i, axis := range region.Axes {
		axisScalar := 1.0
		coord := 0.0
		if i < len(coords) {
			coord = coords[i]
		}

		switch {
		case axis.Start > axis.Peak || axis.Peak > axis.End:
			axisScalar = 1
		case axis.Start < 0 && axis.End > 0 && axis.Peak != 0:
			axisScalar = 1
		case axis.Peak == 0:
			axisScalar = 1
		case coord < axis.Start || coord > axis.End:
			return 0
		case coord == axis.Peak:
			axisScalar = 1
		case coord < axis.Peak:
			axisScalar = (coord - axis.Start) / (axis.Peak - axis.Start)
		default:
			axisScalar = (axis.End - coord) / (axis.End - axis.Peak)
		}
		scalar *= axisScalar
	}
	return scalar
}

func f2Dot14(data []byte) float64 {
	return float64(int16(binary.BigEndian.Uint16(data))) / 16384.0
}

func parseFDSelect(stream api.Stream, offset int64, glyphCount, fdCount int) (*FDSelect, error) {
	format, err := readByte(stream, offset)
	if err != nil {
		return nil, err
	}

	fdSelect := &FDSelect{Format: format, GlyphFD: make([]uint16, glyphCount)}
	switch format {
	case 0:
		data, err := readData(stream, offset+1, int64(glyphCount))
		if err != nil {
			return nil, err
		}
		for i, fd := range data {
			if int(fd) >= fdCount {
				return nil, fmt.Errorf("FDSelect FD index %d out of range", fd)
			}
			fdSelect.GlyphFD[i] = uint16(fd)
		}
	case 3:
		numRanges, err := readUint16(stream, offset+1)
		if err != nil {
			return nil, err
		}
		size := int64(1 + 2 + int(numRanges)*3 + 2)
		data, err := readData(stream, offset, size)
		if err != nil {
			return nil, err
		}
		ranges := make([]fdSelectRange, numRanges)
		p := 3
		for i := range ranges {
			ranges[i].first = int(binary.BigEndian.Uint16(data[p : p+2]))
			ranges[i].fdIndex = int(data[p+2])
			p += 3
		}
		sentinel := int(binary.BigEndian.Uint16(data[p : p+2]))
		if err := fillFDSelectRanges(fdSelect.GlyphFD, ranges, sentinel, glyphCount, fdCount); err != nil {
			return nil, err
		}
	case 4:
		numRanges, err := readUint32(stream, offset+1)
		if err != nil {
			return nil, err
		}
		if numRanges > uint32(^uint16(0)) {
			return nil, fmt.Errorf("FDSelect range count %d exceeds supported maximum", numRanges)
		}
		size := int64(1 + 4 + int(numRanges)*6 + 4)
		data, err := readData(stream, offset, size)
		if err != nil {
			return nil, err
		}
		ranges := make([]fdSelectRange, int(numRanges))
		p := 5
		for i := range ranges {
			ranges[i].first = int(binary.BigEndian.Uint32(data[p : p+4]))
			ranges[i].fdIndex = int(binary.BigEndian.Uint16(data[p+4 : p+6]))
			p += 6
		}
		sentinel := int(binary.BigEndian.Uint32(data[p : p+4]))
		if err := fillFDSelectRanges(fdSelect.GlyphFD, ranges, sentinel, glyphCount, fdCount); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported FDSelect format %d", format)
	}

	return fdSelect, nil
}

type fdSelectRange struct {
	first   int
	fdIndex int
}

func fillFDSelectRanges(glyphFD []uint16, ranges []fdSelectRange, sentinel, glyphCount, fdCount int) error {
	if len(ranges) == 0 {
		return errors.New("FDSelect has no ranges")
	}
	if ranges[0].first != 0 {
		return errors.New("FDSelect first range must start at glyph 0")
	}
	if sentinel != glyphCount {
		return fmt.Errorf("FDSelect sentinel %d does not match glyph count %d", sentinel, glyphCount)
	}
	for i, r := range ranges {
		if r.fdIndex < 0 || r.fdIndex >= fdCount {
			return fmt.Errorf("FDSelect FD index %d out of range", r.fdIndex)
		}
		end := sentinel
		if i+1 < len(ranges) {
			end = ranges[i+1].first
			if end <= r.first {
				return errors.New("FDSelect ranges are not increasing")
			}
		}
		if r.first < 0 || r.first > end || end > glyphCount {
			return errors.New("FDSelect range outside glyph count")
		}
		for glyphID := r.first; glyphID < end; glyphID++ {
			glyphFD[glyphID] = uint16(r.fdIndex)
		}
	}
	return nil
}

// --- Helpers ---

func readData(s api.Stream, off, size int64) ([]byte, error) {
	if off < 0 || size < 0 {
		return nil, fmt.Errorf("invalid negative read bounds")
	}
	if off > s.Size() || size > s.Size()-off {
		return nil, io.ErrUnexpectedEOF
	}
	data := make([]byte, size)
	n, err := s.ReadAt(data, off)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n != len(data) {
		return nil, io.ErrUnexpectedEOF
	}
	return data, nil
}

func readByte(s api.Stream, off int64) (byte, error) {
	var buf [1]byte
	_, err := s.ReadAt(buf[:], off)
	if err != nil {
		return 0, err
	}
	return buf[0], nil
}

func readUint16(s api.Stream, off int64) (uint16, error) {
	var buf [2]byte
	_, err := s.ReadAt(buf[:], off)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf[:]), nil
}

func readUint32(s api.Stream, off int64) (uint32, error) {
	var buf [4]byte
	_, err := s.ReadAt(buf[:], off)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}

func readOffset(s api.Stream, off int64, offSize uint8) (uint32, error) {
	switch offSize {
	case 1:
		v, err := readByte(s, off)
		return uint32(v), err
	case 2:
		v, err := readUint16(s, off)
		return uint32(v), err
	case 3:
		var buf [3]byte
		_, err := s.ReadAt(buf[:], off)
		if err != nil {
			return 0, err
		}
		return uint32(buf[0])<<16 | uint32(buf[1])<<8 | uint32(buf[2]), nil
	case 4:
		return readUint32(s, off)
	default:
		return 0, fmt.Errorf("invalid offSize %d", offSize)
	}
}
