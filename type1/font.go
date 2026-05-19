package type1

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/dh-kam/freetype-go/api"
)

// Font contains the core dictionaries needed to decode Type 1 glyph programs.
type Font struct {
	FontName         string
	FontMatrix       [6]float64
	FontBBox         [4]float64
	EncodingName     string
	Encoding         [256]string
	LenIV            int
	BlueValues       []float64
	OtherBlues       []float64
	FamilyBlues      []float64
	FamilyOtherBlues []float64
	StdHW            []float64
	StdVW            []float64
	StemSnapH        []float64
	StemSnapV        []float64
	BlueScale        float64
	HasBlueScale     bool
	BlueShift        int
	HasBlueShift     bool
	BlueFuzz         int
	HasBlueFuzz      bool
	ForceBold        bool
	HasForceBold     bool
	LanguageGroup    int
	HasLanguageGroup bool
	Subrs            [][]byte
	GlyphNames       []string
	CharStrings      map[string][]byte
}

// ParseFont parses a PFA/PFB Type 1 font far enough to expose dictionaries,
// encoding, decrypted Subrs, and decrypted CharStrings.
func ParseFont(data []byte) (*Font, error) {
	decoded, err := DecodePFB(data)
	if err != nil {
		return nil, err
	}
	clear, private, err := splitType1Sections(decoded)
	if err != nil {
		return nil, err
	}

	font := &Font{
		FontMatrix:   [6]float64{0.001, 0, 0, 0.001, 0, 0},
		EncodingName: "StandardEncoding",
		Encoding:     StandardEncoding(),
		LenIV:        4,
		CharStrings:  make(map[string][]byte),
	}
	parseType1ClearDict(clear, font)
	if err := parseType1PrivateDict(private, font); err != nil {
		return nil, err
	}
	return font, nil
}

// DecodeGlyph decodes a named glyph using the font's decrypted CharStrings and Subrs.
func (f *Font) DecodeGlyph(name string) (*CharStringResult, error) {
	if f == nil {
		return nil, errors.New("nil Type 1 font")
	}
	data, ok := f.CharStrings[name]
	if !ok {
		return nil, fmt.Errorf("Type 1 glyph %q not found", name)
	}
	return DecodeCharString(data, f.Subrs)
}

// DecodeGlyphMetrics decodes side bearing, advance, and SEAC metadata for a named glyph.
func (f *Font) DecodeGlyphMetrics(name string) (sideBearing, width api.Vector, seac *CharStringSEAC, err error) {
	if f == nil {
		return api.Vector{}, api.Vector{}, nil, errors.New("nil Type 1 font")
	}
	data, ok := f.CharStrings[name]
	if !ok {
		return api.Vector{}, api.Vector{}, nil, fmt.Errorf("Type 1 glyph %q not found", name)
	}
	return DecodeMetrics(data, f.Subrs)
}

// GlyphName returns the encoded glyph name for a character code.
func (f *Font) GlyphName(code int) string {
	if f == nil || code < 0 || code >= len(f.Encoding) {
		return ""
	}
	return f.Encoding[code]
}

func splitType1Sections(data []byte) ([]byte, []byte, error) {
	eexecStart, _ := findType1Eexec(data)
	if eexecStart < 0 {
		return nil, nil, errors.New("eexec not found")
	}
	private, err := ExtractEexec(data)
	if err != nil {
		return nil, nil, err
	}
	return data[:eexecStart], private, nil
}

func parseType1ClearDict(data []byte, font *Font) {
	tokens := Lexer(data)
	if value, ok := tokensAfterName(tokens, "FontName"); ok && len(value) > 0 {
		font.FontName = value[0].Value
	}
	if value, ok := tokensAfterName(tokens, "FontMatrix"); ok && len(value) >= 6 {
		if values, err := parseFloatTokens(value[:6], 6); err == nil {
			copy(font.FontMatrix[:], values)
		}
	}
	if value, ok := tokensAfterName(tokens, "FontBBox"); ok && len(value) >= 4 {
		if values, err := parseFloatTokens(value[:4], 4); err == nil {
			copy(font.FontBBox[:], values)
		}
	}
	parseType1Encoding(tokens, font)
}

func parseType1PrivateDict(data []byte, font *Font) error {
	if lenIV, ok, err := parseType1LenIV(data); err != nil {
		return err
	} else if ok {
		font.LenIV = lenIV
	}
	parseType1PrivateHints(data, font)

	subrs, err := parseType1Subrs(data, font.LenIV)
	if err != nil {
		return err
	}
	font.Subrs = subrs

	charStrings, glyphNames, err := parseType1CharStrings(data, font.LenIV)
	if err != nil {
		return err
	}
	font.CharStrings = charStrings
	font.GlyphNames = glyphNames
	return nil
}

func tokensAfterName(tokens []Token, name string) ([]Token, bool) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Type != "Name" || tokens[i].Value != name || i+1 >= len(tokens) {
			continue
		}
		next := tokens[i+1]
		if next.Type == "Delim" && next.Value == "[" {
			var out []Token
			depth := 1
			for j := i + 2; j < len(tokens); j++ {
				t := tokens[j]
				if t.Type == "Delim" && t.Value == "[" {
					depth++
					continue
				}
				if t.Type == "Delim" && t.Value == "]" {
					depth--
					if depth == 0 {
						return out, true
					}
					continue
				}
				if depth == 1 {
					out = append(out, t)
				}
			}
			return out, true
		}
		return []Token{next}, true
	}
	return nil, false
}

func parseFloatTokens(tokens []Token, count int) ([]float64, error) {
	out := make([]float64, count)
	for i := 0; i < count; i++ {
		if i >= len(tokens) {
			return out, errors.New("not enough array operands")
		}
		value, err := strconv.ParseFloat(tokens[i].Value, 64)
		if err != nil {
			return out, err
		}
		out[i] = value
	}
	return out, nil
}

func parseType1Encoding(tokens []Token, font *Font) {
	start := -1
	for i, t := range tokens {
		if t.Type == "Name" && t.Value == "Encoding" {
			start = i
			break
		}
	}
	if start < 0 {
		return
	}
	if start+1 < len(tokens) && (tokens[start+1].Type == "Name" || tokens[start+1].Type == "Operator") {
		if strings.HasSuffix(tokens[start+1].Value, "Encoding") {
			font.EncodingName = tokens[start+1].Value
			switch tokens[start+1].Value {
			case "StandardEncoding":
				font.Encoding = StandardEncoding()
			case "ISOLatin1Encoding":
				font.Encoding = ISOLatin1Encoding()
			case "ExpertEncoding":
				font.Encoding = ExpertEncoding()
			}
		}
	}
	if start+2 < len(tokens) && tokens[start+1].Type == "Number" && tokens[start+2].Type == "Operator" && tokens[start+2].Value == "array" {
		font.EncodingName = "CustomEncoding"
		font.Encoding = [256]string{}
	}
	for i := start + 1; i+3 < len(tokens); i++ {
		if tokens[i].Type != "Operator" || tokens[i].Value != "dup" {
			continue
		}
		code, err := strconv.Atoi(tokens[i+1].Value)
		if err != nil || code < 0 || code >= len(font.Encoding) {
			continue
		}
		if tokens[i+2].Type != "Name" {
			continue
		}
		if tokens[i+3].Type == "Operator" && tokens[i+3].Value == "put" {
			font.Encoding[code] = tokens[i+2].Value
		}
	}
}

func parseType1LenIV(data []byte) (int, bool, error) {
	pos := findPSToken(data, "/lenIV", 0)
	if pos < 0 {
		return 0, false, nil
	}
	tok, _, _, ok := readPSToken(data, pos+len("/lenIV"))
	if !ok {
		return 0, false, errors.New("truncated lenIV value")
	}
	lenIV, err := strconv.Atoi(tok)
	if err != nil {
		return 0, false, fmt.Errorf("invalid lenIV: %w", err)
	}
	return lenIV, true, nil
}

func parseType1Subrs(data []byte, lenIV int) ([][]byte, error) {
	start := findPSToken(data, "/Subrs", 0)
	if start < 0 {
		return nil, nil
	}
	limit := len(data)
	if charStrings := findPSToken(data, "/CharStrings", start+len("/Subrs")); charStrings >= 0 {
		limit = charStrings
	}

	declaredCount := 0
	if tok, _, _, ok := readPSToken(data, start+len("/Subrs")); ok {
		if count, err := strconv.Atoi(tok); err == nil && count > 0 {
			declaredCount = count
		}
	}
	subrs := make([][]byte, declaredCount)
	for pos := start; pos < limit; {
		tok, _, end, ok := readPSToken(data, pos)
		if !ok {
			break
		}
		pos = end
		if tok != "dup" {
			continue
		}
		idxTok, _, idxEnd, ok := readPSToken(data, pos)
		if !ok {
			break
		}
		lengthTok, _, lengthEnd, ok := readPSToken(data, idxEnd)
		if !ok {
			break
		}
		opTok, _, opEnd, ok := readPSToken(data, lengthEnd)
		if !ok || !isType1DataOperator(opTok) {
			pos = idxEnd
			continue
		}
		idx, err := strconv.Atoi(idxTok)
		if err != nil || idx < 0 {
			return nil, errors.New("invalid Type 1 Subrs index")
		}
		length, err := strconv.Atoi(lengthTok)
		if err != nil || length < 0 {
			return nil, errors.New("invalid Type 1 Subrs length")
		}
		raw, next, err := readType1BinaryData(data, opEnd, length)
		if err != nil {
			return nil, err
		}
		decoded, err := DecryptCharString(raw, lenIV)
		if err != nil {
			return nil, err
		}
		for idx >= len(subrs) {
			subrs = append(subrs, nil)
		}
		subrs[idx] = decoded
		pos = next
	}
	return subrs, nil
}

func parseType1CharStrings(data []byte, lenIV int) (map[string][]byte, []string, error) {
	start := findPSToken(data, "/CharStrings", 0)
	if start < 0 {
		return nil, nil, errors.New("Type 1 CharStrings dictionary not found")
	}
	charStrings := make(map[string][]byte)
	var glyphNames []string
	for pos := start + len("/CharStrings"); pos < len(data); {
		tok, _, end, ok := readPSToken(data, pos)
		if !ok {
			break
		}
		pos = end
		if tok == "end" && len(charStrings) > 0 {
			break
		}
		if !strings.HasPrefix(tok, "/") || tok == "/CharStrings" {
			continue
		}
		lengthTok, _, lengthEnd, ok := readPSToken(data, pos)
		if !ok {
			break
		}
		opTok, _, opEnd, ok := readPSToken(data, lengthEnd)
		if !ok || !isType1DataOperator(opTok) {
			pos = lengthEnd
			continue
		}
		length, err := strconv.Atoi(lengthTok)
		if err != nil || length < 0 {
			return nil, nil, errors.New("invalid Type 1 CharStrings length")
		}
		raw, next, err := readType1BinaryData(data, opEnd, length)
		if err != nil {
			return nil, nil, err
		}
		decoded, err := DecryptCharString(raw, lenIV)
		if err != nil {
			return nil, nil, err
		}
		name := strings.TrimPrefix(tok, "/")
		if _, exists := charStrings[name]; !exists {
			charStrings[name] = decoded
			glyphNames = append(glyphNames, name)
		}
		pos = next
	}
	if len(charStrings) == 0 {
		return nil, nil, errors.New("Type 1 CharStrings dictionary is empty")
	}
	return charStrings, glyphNames, nil
}

func isType1DataOperator(tok string) bool {
	return tok == "RD" || tok == "-|"
}

func readType1BinaryData(data []byte, pos, length int) ([]byte, int, error) {
	start, end, err := skipType1BinaryData(data, pos, length)
	if err != nil {
		return nil, 0, err
	}
	return data[start:end], end, nil
}

func skipType1BinaryData(data []byte, pos, length int) (int, int, error) {
	if length < 0 {
		return 0, 0, errors.New("negative Type 1 binary length")
	}
	start := pos
	if start < len(data) && isPSSpace(data[start]) {
		start++
	}
	end := start + length
	if end < start || end > len(data) {
		return 0, 0, errors.New("Type 1 binary data exceeds input")
	}
	return start, end, nil
}

func findType1Eexec(data []byte) (int, int) {
	for pos := 0; pos < len(data); {
		tok, start, end, ok := readPSToken(data, pos)
		if !ok {
			return -1, -1
		}
		if tok == "eexec" {
			return start, end
		}
		pos = end
	}
	return -1, -1
}

func findPSToken(data []byte, target string, pos int) int {
	var prevToken string
	for pos < len(data) {
		tok, start, end, ok := readPSToken(data, pos)
		if !ok {
			return -1
		}
		if tok == target {
			return start
		}
		pos = end
		if isType1DataOperator(tok) {
			if length, err := strconv.Atoi(prevToken); err == nil {
				if payloadStart, _, err := skipType1BinaryData(data, pos, length); err == nil {
					pos = payloadStart + length
				}
			}
		}
		prevToken = tok
	}
	return -1
}

func readPSToken(data []byte, pos int) (token string, start, end int, ok bool) {
	pos = skipPSSpaceAndComments(data, pos)
	if pos >= len(data) {
		return "", pos, pos, false
	}
	start = pos
	c := data[pos]
	switch c {
	case '[', ']', '{', '}':
		return string(c), start, pos + 1, true
	case '(':
		pos++
		depth := 1
		for pos < len(data) && depth > 0 {
			if data[pos] == '\\' {
				pos += 2
				continue
			}
			if data[pos] == '(' {
				depth++
			} else if data[pos] == ')' {
				depth--
			}
			pos++
		}
		return string(data[start:pos]), start, pos, true
	}

	if c == '/' {
		pos++
		for pos < len(data) && !isPSSpace(data[pos]) && !isPSDelimiter(data[pos]) {
			pos++
		}
		return string(data[start:pos]), start, pos, true
	}

	for pos < len(data) && !isPSSpace(data[pos]) && !isPSDelimiter(data[pos]) {
		pos++
	}
	if pos == start {
		return string(data[start : start+1]), start, start + 1, true
	}
	return string(data[start:pos]), start, pos, true
}

func skipPSSpaceAndComments(data []byte, pos int) int {
	for pos < len(data) {
		if isPSSpace(data[pos]) {
			pos++
			continue
		}
		if data[pos] == '%' {
			for pos < len(data) && data[pos] != '\n' && data[pos] != '\r' {
				pos++
			}
			continue
		}
		break
	}
	return pos
}

func isPSSpace(b byte) bool {
	return unicode.IsSpace(rune(b))
}

func isPSDelimiter(b byte) bool {
	return b == '[' || b == ']' || b == '{' || b == '}' || b == '(' || b == ')' || b == '<' || b == '>' || b == '%'
}
