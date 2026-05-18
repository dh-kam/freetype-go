package type1

import (
	"math"
	"strconv"
	"strings"
)

func parseType1PrivateHints(data []byte, font *Font) {
	tokens := type1PrivateTokens(data)

	if values, ok := type1PrivateFloatArray(tokens, "BlueValues"); ok {
		font.BlueValues = values
	}
	if values, ok := type1PrivateFloatArray(tokens, "OtherBlues"); ok {
		font.OtherBlues = values
	}
	if values, ok := type1PrivateFloatArray(tokens, "FamilyBlues"); ok {
		font.FamilyBlues = values
	}
	if values, ok := type1PrivateFloatArray(tokens, "FamilyOtherBlues"); ok {
		font.FamilyOtherBlues = values
	}
	if values, ok := type1PrivateFloatArray(tokens, "StdHW"); ok {
		font.StdHW = values
	}
	if values, ok := type1PrivateFloatArray(tokens, "StdVW"); ok {
		font.StdVW = values
	}
	if values, ok := type1PrivateFloatArray(tokens, "StemSnapH"); ok {
		font.StemSnapH = values
	}
	if values, ok := type1PrivateFloatArray(tokens, "StemSnapV"); ok {
		font.StemSnapV = values
	}
	if value, ok := type1PrivateFloatScalar(tokens, "BlueScale"); ok {
		font.BlueScale = value
		font.HasBlueScale = true
	}
	if value, ok := type1PrivateIntScalar(tokens, "BlueShift"); ok {
		font.BlueShift = value
		font.HasBlueShift = true
	}
	if value, ok := type1PrivateIntScalar(tokens, "BlueFuzz"); ok {
		font.BlueFuzz = value
		font.HasBlueFuzz = true
	}
	if value, ok := type1PrivateBoolScalar(tokens, "ForceBold"); ok {
		font.ForceBold = value
		font.HasForceBold = true
	}
	if value, ok := type1PrivateIntScalar(tokens, "LanguageGroup"); ok {
		font.LanguageGroup = value
		font.HasLanguageGroup = true
	}
}

func type1PrivateTokens(data []byte) []Token {
	var tokens []Token
	var previousRaw string
	for pos := 0; pos < len(data); {
		raw, _, end, ok := readPSToken(data, pos)
		if !ok {
			break
		}
		pos = end
		tokens = append(tokens, type1PrivateToken(raw))
		if isType1DataOperator(raw) {
			length, err := strconv.Atoi(previousRaw)
			if err == nil && length >= 0 {
				if _, next, err := readType1BinaryData(data, pos, length); err == nil {
					pos = next
					previousRaw = ""
					continue
				}
			}
		}
		previousRaw = raw
	}
	return tokens
}

func type1PrivateToken(raw string) Token {
	switch raw {
	case "[", "]", "{", "}":
		return Token{Type: "Delim", Value: raw}
	}
	if strings.HasPrefix(raw, "/") {
		return Token{Type: "Name", Value: strings.TrimPrefix(raw, "/")}
	}
	if strings.HasPrefix(raw, "(") {
		return Token{Type: "String", Value: raw}
	}
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return Token{Type: "Number", Value: raw}
	}
	return Token{Type: "Operator", Value: raw}
}

func type1PrivateFloatArray(tokens []Token, name string) ([]float64, bool) {
	array, ok := type1PrivateArrayTokensAfterName(tokens, name)
	if !ok {
		return nil, false
	}
	values, err := parseFloatTokens(array, len(array))
	if err != nil {
		return nil, false
	}
	for _, value := range values {
		if !type1PrivateFiniteFloat(value) {
			return nil, false
		}
	}
	return values, true
}

func type1PrivateFloatScalar(tokens []Token, name string) (float64, bool) {
	token, ok := type1PrivateScalarTokenAfterName(tokens, name)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseFloat(token.Value, 64)
	if err != nil || !type1PrivateFiniteFloat(value) {
		return 0, false
	}
	return value, true
}

func type1PrivateIntScalar(tokens []Token, name string) (int, bool) {
	token, ok := type1PrivateScalarTokenAfterName(tokens, name)
	if !ok {
		return 0, false
	}
	if value, err := strconv.Atoi(token.Value); err == nil {
		return value, true
	}
	floatValue, err := strconv.ParseFloat(token.Value, 64)
	if err != nil || !type1PrivateFiniteFloat(floatValue) || math.Trunc(floatValue) != floatValue {
		return 0, false
	}
	value := int(floatValue)
	if float64(value) != floatValue {
		return 0, false
	}
	return value, true
}

func type1PrivateBoolScalar(tokens []Token, name string) (bool, bool) {
	token, ok := type1PrivateScalarTokenAfterName(tokens, name)
	if !ok {
		return false, false
	}
	switch strings.ToLower(token.Value) {
	case "true", "1":
		return true, true
	case "false", "0":
		return false, true
	default:
		return false, false
	}
}

func type1PrivateArrayTokensAfterName(tokens []Token, name string) ([]Token, bool) {
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].Type != "Name" || tokens[i].Value != name {
			continue
		}
		open := tokens[i+1]
		if open.Type != "Delim" {
			return nil, false
		}
		closeValue := ""
		switch open.Value {
		case "[":
			closeValue = "]"
		case "{":
			closeValue = "}"
		default:
			return nil, false
		}

		depth := 1
		var out []Token
		for j := i + 2; j < len(tokens); j++ {
			token := tokens[j]
			if token.Type == "Delim" && token.Value == open.Value {
				depth++
				continue
			}
			if token.Type == "Delim" && token.Value == closeValue {
				depth--
				if depth == 0 {
					return out, true
				}
				continue
			}
			if depth == 1 {
				out = append(out, token)
			}
		}
		return nil, false
	}
	return nil, false
}

func type1PrivateScalarTokenAfterName(tokens []Token, name string) (Token, bool) {
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].Type != "Name" || tokens[i].Value != name {
			continue
		}
		token := tokens[i+1]
		if token.Type == "Delim" {
			return Token{}, false
		}
		return token, true
	}
	return Token{}, false
}

func type1PrivateFiniteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
