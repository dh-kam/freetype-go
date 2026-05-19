package cff

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/type1"
)

const cffISOAdobeCharsetCount = 229

func (c *CFF) parseCFF1Metadata(stream api.Stream, tableOffset int64, topDict map[int][]float64) error {
	c.UnitsPerEm = 1000
	if bbox, ok, err := parseCFFFontBBox(topDict); err != nil {
		return err
	} else if ok {
		c.FontBBox = bbox
		c.HasFontBBox = true
	}

	glyphCount := int(c.CharStringsIndex.Count)
	charset, err := c.parseCFFCharset(stream, tableOffset, topDict, glyphCount, isCFFCIDKeyed(topDict))
	if err != nil {
		return err
	}
	c.Charset = charset
	c.GlyphIndexByName = buildCFFGlyphIndexByName(charset)

	encoding, codeToGID, codeToName, err := c.parseCFFEncoding(stream, tableOffset, topDict, charset, c.GlyphIndexByName)
	if err != nil {
		return err
	}
	c.Encoding = encoding
	c.GlyphIndexByCode = codeToGID
	c.GlyphNameByCode = codeToName
	return nil
}

func parseCFFFontBBox(topDict map[int][]float64) ([4]float64, bool, error) {
	operands, ok := topDict[opFontBBox]
	if !ok {
		return [4]float64{}, false, nil
	}
	if len(operands) < 4 {
		return [4]float64{}, false, errors.New("CFF FontBBox requires four operands")
	}
	var bbox [4]float64
	for i := range bbox {
		v := operands[i]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return [4]float64{}, false, errors.New("CFF FontBBox contains non-finite operand")
		}
		bbox[i] = v
	}
	return bbox, true, nil
}

func isCFFCIDKeyed(topDict map[int][]float64) bool {
	if _, ok := topDict[opROS]; ok {
		return true
	}
	_, ok := topDict[opFDArray]
	return ok
}

func (c *CFF) parseCFFCharset(stream api.Stream, tableOffset int64, topDict map[int][]float64, glyphCount int, isCID bool) ([]string, error) {
	if glyphCount <= 0 {
		return nil, nil
	}
	charsetOffset := int64(0)
	if operands, ok := topDict[opCharset]; ok && len(operands) > 0 {
		offset, err := dictUint(operands, 0, "CFF charset")
		if err != nil {
			return nil, err
		}
		charsetOffset = offset
	}

	switch charsetOffset {
	case 0:
		if isCID {
			return predefinedCFFCIDCharset(glyphCount), nil
		}
		return predefinedCFFCharset(cffStandardStrings[:cffISOAdobeCharsetCount], glyphCount), nil
	case 1:
		return predefinedCFFCharset(cffExpertCharsetNames, glyphCount), nil
	case 2:
		return predefinedCFFCharset(cffExpertSubsetCharsetNames, glyphCount), nil
	default:
		return c.parseCFFCustomCharset(stream, tableOffset+charsetOffset, glyphCount, isCID)
	}
}

func predefinedCFFCIDCharset(glyphCount int) []string {
	charset := make([]string, glyphCount)
	for i := range charset {
		charset[i] = fmt.Sprintf("cid%05d", i)
	}
	if glyphCount > 0 {
		charset[0] = ".notdef"
	}
	return charset
}

func predefinedCFFCharset(names []string, glyphCount int) []string {
	charset := make([]string, glyphCount)
	for i := range charset {
		if i < len(names) {
			charset[i] = names[i]
		} else {
			charset[i] = fmt.Sprintf("gid%d", i)
		}
	}
	if glyphCount > 0 && charset[0] == "" {
		charset[0] = ".notdef"
	}
	return charset
}

func (c *CFF) parseCFFCustomCharset(stream api.Stream, offset int64, glyphCount int, isCID bool) ([]string, error) {
	format, err := readByte(stream, offset)
	if err != nil {
		return nil, err
	}
	charset := make([]string, glyphCount)
	charset[0] = ".notdef"
	gid := 1
	p := offset + 1

	switch format {
	case 0:
		data, err := readData(stream, p, int64((glyphCount-1)*2))
		if err != nil {
			return nil, err
		}
		for gid < glyphCount {
			sid := int(binary.BigEndian.Uint16(data[(gid-1)*2 : gid*2]))
			name, err := c.charsetName(sid, isCID)
			if err != nil {
				return nil, err
			}
			charset[gid] = name
			gid++
		}
	case 1, 2:
		for gid < glyphCount {
			rangeSize := int64(3)
			if format == 2 {
				rangeSize = 4
			}
			data, err := readData(stream, p, rangeSize)
			if err != nil {
				return nil, err
			}
			firstSID := int(binary.BigEndian.Uint16(data[0:2]))
			nLeft := int(data[2])
			p += rangeSize
			if format == 2 {
				nLeft = int(binary.BigEndian.Uint16(data[2:4]))
			}
			for i := 0; i <= nLeft && gid < glyphCount; i++ {
				name, err := c.charsetName(firstSID+i, isCID)
				if err != nil {
					return nil, err
				}
				charset[gid] = name
				gid++
			}
		}
	default:
		return nil, fmt.Errorf("unsupported CFF charset format %d", format)
	}
	return charset, nil
}

func (c *CFF) charsetName(id int, isCID bool) (string, error) {
	if isCID {
		return fmt.Sprintf("cid%05d", id), nil
	}
	return c.stringForSID(id)
}

func (c *CFF) parseCFFEncoding(stream api.Stream, tableOffset int64, topDict map[int][]float64, charset []string, glyphIndexByName map[string]int) ([256]string, map[int]int, map[int]string, error) {
	var encoding [256]string
	codeToGID := make(map[int]int)
	codeToName := make(map[int]string)
	if len(charset) == 0 {
		return encoding, codeToGID, codeToName, nil
	}

	encodingOffset := int64(0)
	if operands, ok := topDict[opEncoding]; ok && len(operands) > 0 {
		offset, err := dictUint(operands, 0, "CFF Encoding")
		if err != nil {
			return encoding, nil, nil, err
		}
		encodingOffset = offset
	}
	switch encodingOffset {
	case 0, 1:
		fillCFFPredefinedEncoding(encodingOffset, glyphIndexByName, &encoding, codeToGID, codeToName)
		return encoding, codeToGID, codeToName, nil
	default:
		if err := c.parseCFFCustomEncoding(stream, tableOffset+encodingOffset, charset, glyphIndexByName, &encoding, codeToGID, codeToName); err != nil {
			return encoding, nil, nil, err
		}
		return encoding, codeToGID, codeToName, nil
	}
}

func fillCFFPredefinedEncoding(encodingOffset int64, glyphIndexByName map[string]int, encoding *[256]string, codeToGID map[int]int, codeToName map[int]string) {
	for code := 0; code < len(encoding); code++ {
		name := ""
		switch encodingOffset {
		case 0:
			name = type1.StandardEncodingGlyphName(code)
		case 1:
			name = type1.ExpertEncodingGlyphName(code)
		}
		if name == "" {
			continue
		}
		encoding[code] = name
		if gid, ok := glyphIndexByName[name]; ok {
			codeToGID[code] = gid
			codeToName[code] = name
		}
	}
}

func (c *CFF) parseCFFCustomEncoding(stream api.Stream, offset int64, charset []string, glyphIndexByName map[string]int, encoding *[256]string, codeToGID map[int]int, codeToName map[int]string) error {
	formatByte, err := readByte(stream, offset)
	if err != nil {
		return err
	}
	format := formatByte & 0x7f
	hasSupplement := formatByte&0x80 != 0
	p := offset + 1
	gid := 1

	switch format {
	case 0:
		nCodes, err := readByte(stream, p)
		if err != nil {
			return err
		}
		p++
		codes, err := readData(stream, p, int64(nCodes))
		if err != nil {
			return err
		}
		p += int64(nCodes)
		for _, code := range codes {
			mapCFFCodeToGlyph(int(code), gid, charset, encoding, codeToGID, codeToName)
			gid++
		}
	case 1:
		nRanges, err := readByte(stream, p)
		if err != nil {
			return err
		}
		p++
		for i := 0; i < int(nRanges); i++ {
			rangeData, err := readData(stream, p, 2)
			if err != nil {
				return err
			}
			p += 2
			first := int(rangeData[0])
			nLeft := int(rangeData[1])
			for code := first; code <= first+nLeft; code++ {
				mapCFFCodeToGlyph(code, gid, charset, encoding, codeToGID, codeToName)
				gid++
			}
		}
	default:
		return fmt.Errorf("unsupported CFF Encoding format %d", format)
	}

	if !hasSupplement {
		return nil
	}
	nSups, err := readByte(stream, p)
	if err != nil {
		return err
	}
	p++
	for i := 0; i < int(nSups); i++ {
		supplement, err := readData(stream, p, 3)
		if err != nil {
			return err
		}
		p += 3
		code := int(supplement[0])
		sid := int(binary.BigEndian.Uint16(supplement[1:3]))
		name, err := c.stringForSID(sid)
		if err != nil {
			return err
		}
		encoding[code] = name
		if gid, ok := glyphIndexByName[name]; ok {
			codeToGID[code] = gid
			codeToName[code] = name
		} else {
			delete(codeToGID, code)
			delete(codeToName, code)
		}
	}
	return nil
}

func mapCFFCodeToGlyph(code, gid int, charset []string, encoding *[256]string, codeToGID map[int]int, codeToName map[int]string) {
	if code < 0 || code >= len(encoding) || gid <= 0 || gid >= len(charset) {
		return
	}
	name := charset[gid]
	if name == "" {
		return
	}
	encoding[code] = name
	codeToGID[code] = gid
	codeToName[code] = name
}

func buildCFFGlyphIndexByName(charset []string) map[string]int {
	index := make(map[string]int, len(charset))
	for gid, name := range charset {
		if name == "" {
			continue
		}
		if _, exists := index[name]; !exists {
			index[name] = gid
		}
	}
	return index
}

func (c *CFF) stringForSID(sid int) (string, error) {
	if sid < 0 {
		return "", fmt.Errorf("negative CFF SID %d", sid)
	}
	if sid < len(cffStandardStrings) {
		return cffStandardStrings[sid], nil
	}
	if c == nil {
		return "", fmt.Errorf("CFF SID %d missing String INDEX", sid)
	}
	data, err := c.StringIndex.Get(sid - len(cffStandardStrings))
	if err != nil {
		return "", fmt.Errorf("CFF SID %d out of range: %w", sid, err)
	}
	return string(data), nil
}

// GetUnitsPerEm reports the design grid used by standalone CFF Type1C data.
func (c *CFF) GetUnitsPerEm() uint16 {
	if c == nil {
		return 0
	}
	if c.UnitsPerEm != 0 {
		return c.UnitsPerEm
	}
	if c.Major == 1 {
		return 1000
	}
	return 0
}

func (c *CFF) GetFontBBox() ([4]float64, bool) {
	if c == nil || !c.HasFontBBox {
		return [4]float64{}, false
	}
	return c.FontBBox, true
}

func (c *CFF) GetNumGlyphs() int {
	if c == nil {
		return 0
	}
	return int(c.CharStringsIndex.Count)
}

func (c *CFF) GetGlyphIndex(char rune) (int, error) {
	if c == nil {
		return 0, errors.New("nil CFF face")
	}
	if char < 0 || char > 255 {
		return 0, nil
	}
	if gid, ok := c.GlyphIndexByCode[int(char)]; ok {
		return gid, nil
	}
	return 0, nil
}

func (c *CFF) LookupGlyphIndexByCode(code int) (int, bool) {
	if c == nil {
		return 0, false
	}
	gid, ok := c.GlyphIndexByCode[code]
	return gid, ok
}

func (c *CFF) LookupGlyphNameByCode(code int) (string, bool) {
	if c == nil {
		return "", false
	}
	name, ok := c.GlyphNameByCode[code]
	return name, ok
}

func (c *CFF) GetGlyphNameByCharCode(code int) (string, bool) {
	return c.LookupGlyphNameByCode(code)
}

func (c *CFF) LookupGlyphIndexByName(name string) (int, bool) {
	if c == nil {
		return 0, false
	}
	gid, ok := c.GlyphIndexByName[name]
	return gid, ok
}

func (c *CFF) GetGlyphName(glyphIndex int) (string, bool) {
	return c.GlyphName(glyphIndex)
}

func (c *CFF) GlyphName(glyphIndex int) (string, bool) {
	if c == nil || glyphIndex < 0 || glyphIndex >= len(c.Charset) {
		return "", false
	}
	name := c.Charset[glyphIndex]
	return name, name != ""
}

// cffStandardStrings is the CFF standard SID string set from Adobe Technical
// Note #5176, Appendix A.
var cffStandardStrings = [...]string{
	".notdef",
	"space",
	"exclam",
	"quotedbl",
	"numbersign",
	"dollar",
	"percent",
	"ampersand",
	"quoteright",
	"parenleft",
	"parenright",
	"asterisk",
	"plus",
	"comma",
	"hyphen",
	"period",
	"slash",
	"zero",
	"one",
	"two",
	"three",
	"four",
	"five",
	"six",
	"seven",
	"eight",
	"nine",
	"colon",
	"semicolon",
	"less",
	"equal",
	"greater",
	"question",
	"at",
	"A",
	"B",
	"C",
	"D",
	"E",
	"F",
	"G",
	"H",
	"I",
	"J",
	"K",
	"L",
	"M",
	"N",
	"O",
	"P",
	"Q",
	"R",
	"S",
	"T",
	"U",
	"V",
	"W",
	"X",
	"Y",
	"Z",
	"bracketleft",
	"backslash",
	"bracketright",
	"asciicircum",
	"underscore",
	"quoteleft",
	"a",
	"b",
	"c",
	"d",
	"e",
	"f",
	"g",
	"h",
	"i",
	"j",
	"k",
	"l",
	"m",
	"n",
	"o",
	"p",
	"q",
	"r",
	"s",
	"t",
	"u",
	"v",
	"w",
	"x",
	"y",
	"z",
	"braceleft",
	"bar",
	"braceright",
	"asciitilde",
	"exclamdown",
	"cent",
	"sterling",
	"fraction",
	"yen",
	"florin",
	"section",
	"currency",
	"quotesingle",
	"quotedblleft",
	"guillemotleft",
	"guilsinglleft",
	"guilsinglright",
	"fi",
	"fl",
	"endash",
	"dagger",
	"daggerdbl",
	"periodcentered",
	"paragraph",
	"bullet",
	"quotesinglbase",
	"quotedblbase",
	"quotedblright",
	"guillemotright",
	"ellipsis",
	"perthousand",
	"questiondown",
	"grave",
	"acute",
	"circumflex",
	"tilde",
	"macron",
	"breve",
	"dotaccent",
	"dieresis",
	"ring",
	"cedilla",
	"hungarumlaut",
	"ogonek",
	"caron",
	"emdash",
	"AE",
	"ordfeminine",
	"Lslash",
	"Oslash",
	"OE",
	"ordmasculine",
	"ae",
	"dotlessi",
	"lslash",
	"oslash",
	"oe",
	"germandbls",
	"onesuperior",
	"logicalnot",
	"mu",
	"trademark",
	"Eth",
	"onehalf",
	"plusminus",
	"Thorn",
	"onequarter",
	"divide",
	"brokenbar",
	"degree",
	"thorn",
	"threequarters",
	"twosuperior",
	"registered",
	"minus",
	"eth",
	"multiply",
	"threesuperior",
	"copyright",
	"Aacute",
	"Acircumflex",
	"Adieresis",
	"Agrave",
	"Aring",
	"Atilde",
	"Ccedilla",
	"Eacute",
	"Ecircumflex",
	"Edieresis",
	"Egrave",
	"Iacute",
	"Icircumflex",
	"Idieresis",
	"Igrave",
	"Ntilde",
	"Oacute",
	"Ocircumflex",
	"Odieresis",
	"Ograve",
	"Otilde",
	"Scaron",
	"Uacute",
	"Ucircumflex",
	"Udieresis",
	"Ugrave",
	"Yacute",
	"Ydieresis",
	"Zcaron",
	"aacute",
	"acircumflex",
	"adieresis",
	"agrave",
	"aring",
	"atilde",
	"ccedilla",
	"eacute",
	"ecircumflex",
	"edieresis",
	"egrave",
	"iacute",
	"icircumflex",
	"idieresis",
	"igrave",
	"ntilde",
	"oacute",
	"ocircumflex",
	"odieresis",
	"ograve",
	"otilde",
	"scaron",
	"uacute",
	"ucircumflex",
	"udieresis",
	"ugrave",
	"yacute",
	"ydieresis",
	"zcaron",
	"exclamsmall",
	"Hungarumlautsmall",
	"dollaroldstyle",
	"dollarsuperior",
	"ampersandsmall",
	"Acutesmall",
	"parenleftsuperior",
	"parenrightsuperior",
	"twodotenleader",
	"onedotenleader",
	"zerooldstyle",
	"oneoldstyle",
	"twooldstyle",
	"threeoldstyle",
	"fouroldstyle",
	"fiveoldstyle",
	"sixoldstyle",
	"sevenoldstyle",
	"eightoldstyle",
	"nineoldstyle",
	"commasuperior",
	"threequartersemdash",
	"periodsuperior",
	"questionsmall",
	"asuperior",
	"bsuperior",
	"centsuperior",
	"dsuperior",
	"esuperior",
	"isuperior",
	"lsuperior",
	"msuperior",
	"nsuperior",
	"osuperior",
	"rsuperior",
	"ssuperior",
	"tsuperior",
	"ff",
	"ffi",
	"ffl",
	"parenleftinferior",
	"parenrightinferior",
	"Circumflexsmall",
	"hyphensuperior",
	"Gravesmall",
	"Asmall",
	"Bsmall",
	"Csmall",
	"Dsmall",
	"Esmall",
	"Fsmall",
	"Gsmall",
	"Hsmall",
	"Ismall",
	"Jsmall",
	"Ksmall",
	"Lsmall",
	"Msmall",
	"Nsmall",
	"Osmall",
	"Psmall",
	"Qsmall",
	"Rsmall",
	"Ssmall",
	"Tsmall",
	"Usmall",
	"Vsmall",
	"Wsmall",
	"Xsmall",
	"Ysmall",
	"Zsmall",
	"colonmonetary",
	"onefitted",
	"rupiah",
	"Tildesmall",
	"exclamdownsmall",
	"centoldstyle",
	"Lslashsmall",
	"Scaronsmall",
	"Zcaronsmall",
	"Dieresissmall",
	"Brevesmall",
	"Caronsmall",
	"Dotaccentsmall",
	"Macronsmall",
	"figuredash",
	"hypheninferior",
	"Ogoneksmall",
	"Ringsmall",
	"Cedillasmall",
	"questiondownsmall",
	"oneeighth",
	"threeeighths",
	"fiveeighths",
	"seveneighths",
	"onethird",
	"twothirds",
	"zerosuperior",
	"foursuperior",
	"fivesuperior",
	"sixsuperior",
	"sevensuperior",
	"eightsuperior",
	"ninesuperior",
	"zeroinferior",
	"oneinferior",
	"twoinferior",
	"threeinferior",
	"fourinferior",
	"fiveinferior",
	"sixinferior",
	"seveninferior",
	"eightinferior",
	"nineinferior",
	"centinferior",
	"dollarinferior",
	"periodinferior",
	"commainferior",
	"Agravesmall",
	"Aacutesmall",
	"Acircumflexsmall",
	"Atildesmall",
	"Adieresissmall",
	"Aringsmall",
	"AEsmall",
	"Ccedillasmall",
	"Egravesmall",
	"Eacutesmall",
	"Ecircumflexsmall",
	"Edieresissmall",
	"Igravesmall",
	"Iacutesmall",
	"Icircumflexsmall",
	"Idieresissmall",
	"Ethsmall",
	"Ntildesmall",
	"Ogravesmall",
	"Oacutesmall",
	"Ocircumflexsmall",
	"Otildesmall",
	"Odieresissmall",
	"OEsmall",
	"Oslashsmall",
	"Ugravesmall",
	"Uacutesmall",
	"Ucircumflexsmall",
	"Udieresissmall",
	"Yacutesmall",
	"Thornsmall",
	"Ydieresissmall",
	"001.000",
	"001.001",
	"001.002",
	"001.003",
	"Black",
	"Bold",
	"Book",
	"Light",
	"Medium",
	"Regular",
	"Roman",
	"Semibold",
}

var cffExpertCharsetNames = []string{
	".notdef",
	"space",
	"exclamsmall",
	"Hungarumlautsmall",
	"dollaroldstyle",
	"dollarsuperior",
	"ampersandsmall",
	"Acutesmall",
	"parenleftsuperior",
	"parenrightsuperior",
	"twodotenleader",
	"onedotenleader",
	"comma",
	"hyphen",
	"period",
	"fraction",
	"zerooldstyle",
	"oneoldstyle",
	"twooldstyle",
	"threeoldstyle",
	"fouroldstyle",
	"fiveoldstyle",
	"sixoldstyle",
	"sevenoldstyle",
	"eightoldstyle",
	"nineoldstyle",
	"colon",
	"semicolon",
	"commasuperior",
	"threequartersemdash",
	"periodsuperior",
	"questionsmall",
	"asuperior",
	"bsuperior",
	"centsuperior",
	"dsuperior",
	"esuperior",
	"isuperior",
	"lsuperior",
	"msuperior",
	"nsuperior",
	"osuperior",
	"rsuperior",
	"ssuperior",
	"tsuperior",
	"ff",
	"fi",
	"fl",
	"ffi",
	"ffl",
	"parenleftinferior",
	"parenrightinferior",
	"Circumflexsmall",
	"hyphensuperior",
	"Gravesmall",
	"Asmall",
	"Bsmall",
	"Csmall",
	"Dsmall",
	"Esmall",
	"Fsmall",
	"Gsmall",
	"Hsmall",
	"Ismall",
	"Jsmall",
	"Ksmall",
	"Lsmall",
	"Msmall",
	"Nsmall",
	"Osmall",
	"Psmall",
	"Qsmall",
	"Rsmall",
	"Ssmall",
	"Tsmall",
	"Usmall",
	"Vsmall",
	"Wsmall",
	"Xsmall",
	"Ysmall",
	"Zsmall",
	"colonmonetary",
	"onefitted",
	"rupiah",
	"Tildesmall",
	"exclamdownsmall",
	"centoldstyle",
	"Lslashsmall",
	"Scaronsmall",
	"Zcaronsmall",
	"Dieresissmall",
	"Brevesmall",
	"Caronsmall",
	"Dotaccentsmall",
	"Macronsmall",
	"figuredash",
	"hypheninferior",
	"Ogoneksmall",
	"Ringsmall",
	"Cedillasmall",
	"onequarter",
	"onehalf",
	"threequarters",
	"questiondownsmall",
	"oneeighth",
	"threeeighths",
	"fiveeighths",
	"seveneighths",
	"onethird",
	"twothirds",
	"zerosuperior",
	"onesuperior",
	"twosuperior",
	"threesuperior",
	"foursuperior",
	"fivesuperior",
	"sixsuperior",
	"sevensuperior",
	"eightsuperior",
	"ninesuperior",
	"zeroinferior",
	"oneinferior",
	"twoinferior",
	"threeinferior",
	"fourinferior",
	"fiveinferior",
	"sixinferior",
	"seveninferior",
	"eightinferior",
	"nineinferior",
	"centinferior",
	"dollarinferior",
	"periodinferior",
	"commainferior",
	"Agravesmall",
	"Aacutesmall",
	"Acircumflexsmall",
	"Atildesmall",
	"Adieresissmall",
	"Aringsmall",
	"AEsmall",
	"Ccedillasmall",
	"Egravesmall",
	"Eacutesmall",
	"Ecircumflexsmall",
	"Edieresissmall",
	"Igravesmall",
	"Iacutesmall",
	"Icircumflexsmall",
	"Idieresissmall",
	"Ethsmall",
	"Ntildesmall",
	"Ogravesmall",
	"Oacutesmall",
	"Ocircumflexsmall",
	"Otildesmall",
	"Odieresissmall",
	"OEsmall",
	"Oslashsmall",
	"Ugravesmall",
	"Uacutesmall",
	"Ucircumflexsmall",
	"Udieresissmall",
	"Yacutesmall",
	"Thornsmall",
	"Ydieresissmall",
}

var cffExpertSubsetCharsetNames = []string{
	".notdef",
	"space",
	"dollaroldstyle",
	"dollarsuperior",
	"parenleftsuperior",
	"parenrightsuperior",
	"twodotenleader",
	"onedotenleader",
	"comma",
	"hyphen",
	"period",
	"fraction",
	"zerooldstyle",
	"oneoldstyle",
	"twooldstyle",
	"threeoldstyle",
	"fouroldstyle",
	"fiveoldstyle",
	"sixoldstyle",
	"sevenoldstyle",
	"eightoldstyle",
	"nineoldstyle",
	"colon",
	"semicolon",
	"commasuperior",
	"threequartersemdash",
	"periodsuperior",
	"asuperior",
	"bsuperior",
	"centsuperior",
	"dsuperior",
	"esuperior",
	"isuperior",
	"lsuperior",
	"msuperior",
	"nsuperior",
	"osuperior",
	"rsuperior",
	"ssuperior",
	"tsuperior",
	"ff",
	"fi",
	"fl",
	"ffi",
	"ffl",
	"parenleftinferior",
	"parenrightinferior",
	"hyphensuperior",
	"colonmonetary",
	"onefitted",
	"rupiah",
	"centoldstyle",
	"figuredash",
	"hypheninferior",
	"onequarter",
	"onehalf",
	"threequarters",
	"oneeighth",
	"threeeighths",
	"fiveeighths",
	"seveneighths",
	"onethird",
	"twothirds",
	"zerosuperior",
	"onesuperior",
	"twosuperior",
	"threesuperior",
	"foursuperior",
	"fivesuperior",
	"sixsuperior",
	"sevensuperior",
	"eightsuperior",
	"ninesuperior",
	"zeroinferior",
	"oneinferior",
	"twoinferior",
	"threeinferior",
	"fourinferior",
	"fiveinferior",
	"sixinferior",
	"seveninferior",
	"eightinferior",
	"nineinferior",
	"centinferior",
	"dollarinferior",
	"periodinferior",
	"commainferior",
}
