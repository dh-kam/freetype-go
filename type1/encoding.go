package type1

// StandardEncoding returns the Adobe Type 1 StandardEncoding vector.
// Empty strings mark character codes with no glyph assigned.
func StandardEncoding() [256]string {
	return standardEncoding
}

// StandardEncodingGlyphName returns the StandardEncoding glyph name for code.
// It returns an empty string for out-of-range or unassigned codes.
func StandardEncodingGlyphName(code int) string {
	if code < 0 || code >= len(standardEncoding) {
		return ""
	}
	return standardEncoding[code]
}

// ISOLatin1Encoding returns the Adobe PostScript ISOLatin1Encoding vector.
// Empty strings mark character codes with no glyph assigned.
func ISOLatin1Encoding() [256]string {
	return isoLatin1Encoding
}

// ISOLatin1EncodingGlyphName returns the ISOLatin1Encoding glyph name for code.
// It returns an empty string for out-of-range or unassigned codes.
func ISOLatin1EncodingGlyphName(code int) string {
	if code < 0 || code >= len(isoLatin1Encoding) {
		return ""
	}
	return isoLatin1Encoding[code]
}

// ExpertEncoding returns the Adobe Type 1 ExpertEncoding vector.
// Empty strings mark character codes with no glyph assigned.
func ExpertEncoding() [256]string {
	return expertEncoding
}

// ExpertEncodingGlyphName returns the ExpertEncoding glyph name for code.
// It returns an empty string for out-of-range or unassigned codes.
func ExpertEncodingGlyphName(code int) string {
	if code < 0 || code >= len(expertEncoding) {
		return ""
	}
	return expertEncoding[code]
}

var standardEncoding = [256]string{
	32:  "space",
	33:  "exclam",
	34:  "quotedbl",
	35:  "numbersign",
	36:  "dollar",
	37:  "percent",
	38:  "ampersand",
	39:  "quoteright",
	40:  "parenleft",
	41:  "parenright",
	42:  "asterisk",
	43:  "plus",
	44:  "comma",
	45:  "hyphen",
	46:  "period",
	47:  "slash",
	48:  "zero",
	49:  "one",
	50:  "two",
	51:  "three",
	52:  "four",
	53:  "five",
	54:  "six",
	55:  "seven",
	56:  "eight",
	57:  "nine",
	58:  "colon",
	59:  "semicolon",
	60:  "less",
	61:  "equal",
	62:  "greater",
	63:  "question",
	64:  "at",
	65:  "A",
	66:  "B",
	67:  "C",
	68:  "D",
	69:  "E",
	70:  "F",
	71:  "G",
	72:  "H",
	73:  "I",
	74:  "J",
	75:  "K",
	76:  "L",
	77:  "M",
	78:  "N",
	79:  "O",
	80:  "P",
	81:  "Q",
	82:  "R",
	83:  "S",
	84:  "T",
	85:  "U",
	86:  "V",
	87:  "W",
	88:  "X",
	89:  "Y",
	90:  "Z",
	91:  "bracketleft",
	92:  "backslash",
	93:  "bracketright",
	94:  "asciicircum",
	95:  "underscore",
	96:  "quoteleft",
	97:  "a",
	98:  "b",
	99:  "c",
	100: "d",
	101: "e",
	102: "f",
	103: "g",
	104: "h",
	105: "i",
	106: "j",
	107: "k",
	108: "l",
	109: "m",
	110: "n",
	111: "o",
	112: "p",
	113: "q",
	114: "r",
	115: "s",
	116: "t",
	117: "u",
	118: "v",
	119: "w",
	120: "x",
	121: "y",
	122: "z",
	123: "braceleft",
	124: "bar",
	125: "braceright",
	126: "asciitilde",
	161: "exclamdown",
	162: "cent",
	163: "sterling",
	164: "fraction",
	165: "yen",
	166: "florin",
	167: "section",
	168: "currency",
	169: "quotesingle",
	170: "quotedblleft",
	171: "guillemotleft",
	172: "guilsinglleft",
	173: "guilsinglright",
	174: "fi",
	175: "fl",
	177: "endash",
	178: "dagger",
	179: "daggerdbl",
	180: "periodcentered",
	182: "paragraph",
	183: "bullet",
	184: "quotesinglbase",
	185: "quotedblbase",
	186: "quotedblright",
	187: "guillemotright",
	188: "ellipsis",
	189: "perthousand",
	191: "questiondown",
	193: "grave",
	194: "acute",
	195: "circumflex",
	196: "tilde",
	197: "macron",
	198: "breve",
	199: "dotaccent",
	200: "dieresis",
	202: "ring",
	203: "cedilla",
	205: "hungarumlaut",
	206: "ogonek",
	207: "caron",
	208: "emdash",
	225: "AE",
	227: "ordfeminine",
	232: "Lslash",
	233: "Oslash",
	234: "OE",
	235: "ordmasculine",
	241: "ae",
	245: "dotlessi",
	248: "lslash",
	249: "oslash",
	250: "oe",
	251: "germandbls",
}

var expertEncoding = [256]string{
	32:  "space",
	33:  "exclamsmall",
	34:  "Hungarumlautsmall",
	36:  "dollaroldstyle",
	37:  "dollarsuperior",
	38:  "ampersandsmall",
	39:  "Acutesmall",
	40:  "parenleftsuperior",
	41:  "parenrightsuperior",
	42:  "twodotenleader",
	43:  "onedotenleader",
	44:  "comma",
	45:  "hyphen",
	46:  "period",
	47:  "fraction",
	48:  "zerooldstyle",
	49:  "oneoldstyle",
	50:  "twooldstyle",
	51:  "threeoldstyle",
	52:  "fouroldstyle",
	53:  "fiveoldstyle",
	54:  "sixoldstyle",
	55:  "sevenoldstyle",
	56:  "eightoldstyle",
	57:  "nineoldstyle",
	58:  "colon",
	59:  "semicolon",
	60:  "commasuperior",
	61:  "threequartersemdash",
	62:  "periodsuperior",
	63:  "questionsmall",
	65:  "asuperior",
	66:  "bsuperior",
	67:  "centsuperior",
	68:  "dsuperior",
	69:  "esuperior",
	73:  "isuperior",
	76:  "lsuperior",
	77:  "msuperior",
	78:  "nsuperior",
	79:  "osuperior",
	82:  "rsuperior",
	83:  "ssuperior",
	84:  "tsuperior",
	86:  "ff",
	87:  "fi",
	88:  "fl",
	89:  "ffi",
	90:  "ffl",
	91:  "parenleftinferior",
	93:  "parenrightinferior",
	94:  "Circumflexsmall",
	95:  "hyphensuperior",
	96:  "Gravesmall",
	97:  "Asmall",
	98:  "Bsmall",
	99:  "Csmall",
	100: "Dsmall",
	101: "Esmall",
	102: "Fsmall",
	103: "Gsmall",
	104: "Hsmall",
	105: "Ismall",
	106: "Jsmall",
	107: "Ksmall",
	108: "Lsmall",
	109: "Msmall",
	110: "Nsmall",
	111: "Osmall",
	112: "Psmall",
	113: "Qsmall",
	114: "Rsmall",
	115: "Ssmall",
	116: "Tsmall",
	117: "Usmall",
	118: "Vsmall",
	119: "Wsmall",
	120: "Xsmall",
	121: "Ysmall",
	122: "Zsmall",
	123: "colonmonetary",
	124: "onefitted",
	125: "rupiah",
	126: "Tildesmall",
	161: "exclamdownsmall",
	162: "centoldstyle",
	163: "Lslashsmall",
	166: "Scaronsmall",
	167: "Zcaronsmall",
	168: "Dieresissmall",
	169: "Brevesmall",
	170: "Caronsmall",
	172: "Dotaccentsmall",
	175: "Macronsmall",
	178: "figuredash",
	179: "hypheninferior",
	182: "Ogoneksmall",
	183: "Ringsmall",
	184: "Cedillasmall",
	188: "onequarter",
	189: "onehalf",
	190: "threequarters",
	191: "questiondownsmall",
	192: "oneeighth",
	193: "threeeighths",
	194: "fiveeighths",
	195: "seveneighths",
	196: "onethird",
	197: "twothirds",
	200: "zerosuperior",
	201: "onesuperior",
	202: "twosuperior",
	203: "threesuperior",
	204: "foursuperior",
	205: "fivesuperior",
	206: "sixsuperior",
	207: "sevensuperior",
	208: "eightsuperior",
	209: "ninesuperior",
	210: "zeroinferior",
	211: "oneinferior",
	212: "twoinferior",
	213: "threeinferior",
	214: "fourinferior",
	215: "fiveinferior",
	216: "sixinferior",
	217: "seveninferior",
	218: "eightinferior",
	219: "nineinferior",
	220: "centinferior",
	221: "dollarinferior",
	222: "periodinferior",
	223: "commainferior",
	224: "Agravesmall",
	225: "Aacutesmall",
	226: "Acircumflexsmall",
	227: "Atildesmall",
	228: "Adieresissmall",
	229: "Aringsmall",
	230: "AEsmall",
	231: "Ccedillasmall",
	232: "Egravesmall",
	233: "Eacutesmall",
	234: "Ecircumflexsmall",
	235: "Edieresissmall",
	236: "Igravesmall",
	237: "Iacutesmall",
	238: "Icircumflexsmall",
	239: "Idieresissmall",
	240: "Ethsmall",
	241: "Ntildesmall",
	242: "Ogravesmall",
	243: "Oacutesmall",
	244: "Ocircumflexsmall",
	245: "Otildesmall",
	246: "Odieresissmall",
	247: "OEsmall",
	248: "Oslashsmall",
	249: "Ugravesmall",
	250: "Uacutesmall",
	251: "Ucircumflexsmall",
	252: "Udieresissmall",
	253: "Yacutesmall",
	254: "Thornsmall",
	255: "Ydieresissmall",
}

var isoLatin1Encoding = func() [256]string {
	enc := standardEncoding
	enc[45] = "minus"
	for i := 128; i < len(enc); i++ {
		enc[i] = ""
	}

	enc[144] = "dotlessi"
	enc[145] = "grave"
	enc[146] = "acute"
	enc[147] = "circumflex"
	enc[148] = "tilde"
	enc[149] = "macron"
	enc[150] = "breve"
	enc[151] = "dotaccent"
	enc[152] = "dieresis"
	enc[154] = "ring"
	enc[155] = "cedilla"
	enc[157] = "hungarumlaut"
	enc[158] = "ogonek"
	enc[159] = "caron"
	enc[160] = "space"
	enc[161] = "exclamdown"
	enc[162] = "cent"
	enc[163] = "sterling"
	enc[164] = "currency"
	enc[165] = "yen"
	enc[166] = "brokenbar"
	enc[167] = "section"
	enc[168] = "dieresis"
	enc[169] = "copyright"
	enc[170] = "ordfeminine"
	enc[171] = "guillemotleft"
	enc[172] = "logicalnot"
	enc[173] = "hyphen"
	enc[174] = "registered"
	enc[175] = "macron"
	enc[176] = "degree"
	enc[177] = "plusminus"
	enc[178] = "twosuperior"
	enc[179] = "threesuperior"
	enc[180] = "acute"
	enc[181] = "mu"
	enc[182] = "paragraph"
	enc[183] = "periodcentered"
	enc[184] = "cedilla"
	enc[185] = "onesuperior"
	enc[186] = "ordmasculine"
	enc[187] = "guillemotright"
	enc[188] = "onequarter"
	enc[189] = "onehalf"
	enc[190] = "threequarters"
	enc[191] = "questiondown"
	enc[192] = "Agrave"
	enc[193] = "Aacute"
	enc[194] = "Acircumflex"
	enc[195] = "Atilde"
	enc[196] = "Adieresis"
	enc[197] = "Aring"
	enc[198] = "AE"
	enc[199] = "Ccedilla"
	enc[200] = "Egrave"
	enc[201] = "Eacute"
	enc[202] = "Ecircumflex"
	enc[203] = "Edieresis"
	enc[204] = "Igrave"
	enc[205] = "Iacute"
	enc[206] = "Icircumflex"
	enc[207] = "Idieresis"
	enc[208] = "Eth"
	enc[209] = "Ntilde"
	enc[210] = "Ograve"
	enc[211] = "Oacute"
	enc[212] = "Ocircumflex"
	enc[213] = "Otilde"
	enc[214] = "Odieresis"
	enc[215] = "multiply"
	enc[216] = "Oslash"
	enc[217] = "Ugrave"
	enc[218] = "Uacute"
	enc[219] = "Ucircumflex"
	enc[220] = "Udieresis"
	enc[221] = "Yacute"
	enc[222] = "Thorn"
	enc[223] = "germandbls"
	enc[224] = "agrave"
	enc[225] = "aacute"
	enc[226] = "acircumflex"
	enc[227] = "atilde"
	enc[228] = "adieresis"
	enc[229] = "aring"
	enc[230] = "ae"
	enc[231] = "ccedilla"
	enc[232] = "egrave"
	enc[233] = "eacute"
	enc[234] = "ecircumflex"
	enc[235] = "edieresis"
	enc[236] = "igrave"
	enc[237] = "iacute"
	enc[238] = "icircumflex"
	enc[239] = "idieresis"
	enc[240] = "eth"
	enc[241] = "ntilde"
	enc[242] = "ograve"
	enc[243] = "oacute"
	enc[244] = "ocircumflex"
	enc[245] = "otilde"
	enc[246] = "odieresis"
	enc[247] = "divide"
	enc[248] = "oslash"
	enc[249] = "ugrave"
	enc[250] = "uacute"
	enc[251] = "ucircumflex"
	enc[252] = "udieresis"
	enc[253] = "yacute"
	enc[254] = "thorn"
	enc[255] = "ydieresis"
	return enc
}()
