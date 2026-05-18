package type1

import (
	"encoding/hex"
	"errors"
	"unicode"
)

// DecodePFB decodes a .pfb file into a continuous byte slice.
// If the data is not PFB, it returns the data unchanged (assuming PFA).
func DecodePFB(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}
	if data[0] != 0x80 {
		return data, nil
	}

	var result []byte
	i := 0
	for i < len(data) {
		if data[i] != 0x80 {
			return nil, errors.New("invalid PFB block header")
		}
		if i+2 > len(data) {
			return nil, errors.New("truncated PFB block header")
		}
		blockType := data[i+1]
		if blockType == 3 {
			break // EOF
		}
		if blockType != 1 && blockType != 2 {
			return nil, errors.New("unsupported PFB block type")
		}
		if i+6 > len(data) {
			return nil, errors.New("truncated PFB block header")
		}

		length := int(data[i+2]) | int(data[i+3])<<8 | int(data[i+4])<<16 | int(data[i+5])<<24
		i += 6
		if i+length > len(data) {
			return nil, errors.New("PFB block length exceeds data")
		}

		result = append(result, data[i:i+length]...)
		i += length
	}
	return result, nil
}

// DecryptEexec decrypts Type 1 eexec or charstring data.
func DecryptEexec(data []byte, r uint16) []byte {
	const c1 = 52845
	const c2 = 22719

	result := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		cipher := data[i]
		plain := cipher ^ byte(r>>8)
		result[i] = plain
		r = (uint16(cipher)+r)*c1 + c2
	}
	return result
}

// Token represents a PostScript token.
type Token struct {
	Type  string
	Value string
}

// Lexer tokenizes raw PostScript data.
func Lexer(data []byte) []Token {
	var tokens []Token
	i := 0
	for i < len(data) {
		c := data[i]
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}
		if c == '%' {
			// Skip comment
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				i++
			}
			continue
		}
		if c == '/' {
			i++
			start := i
			for i < len(data) && !unicode.IsSpace(rune(data[i])) && !isDelim(data[i]) {
				i++
			}
			tokens = append(tokens, Token{Type: "Name", Value: string(data[start:i])})
			continue
		}
		if c == '[' || c == ']' || c == '{' || c == '}' {
			tokens = append(tokens, Token{Type: "Delim", Value: string(c)})
			i++
			continue
		}
		if c == '(' {
			// String
			start := i
			i++
			depth := 1
			for i < len(data) && depth > 0 {
				if data[i] == '\\' {
					i++
					if i < len(data) {
						i++
					}
					continue
				}
				if data[i] == '(' {
					depth++
				} else if data[i] == ')' {
					depth--
				}
				i++
			}
			tokens = append(tokens, Token{Type: "String", Value: string(data[start:i])})
			continue
		}
		if c == '<' {
			// Hex string or dictionary
			if i+1 < len(data) && data[i+1] == '<' {
				tokens = append(tokens, Token{Type: "DictStart", Value: "<<"})
				i += 2
			} else {
				start := i
				i++
				for i < len(data) && data[i] != '>' {
					i++
				}
				if i < len(data) {
					i++
				}
				tokens = append(tokens, Token{Type: "HexString", Value: string(data[start:i])})
			}
			continue
		}
		if c == '>' && i+1 < len(data) && data[i+1] == '>' {
			tokens = append(tokens, Token{Type: "DictEnd", Value: ">>"})
			i += 2
			continue
		}
		if isDelim(c) {
			tokens = append(tokens, Token{Type: "Delim", Value: string(c)})
			i++
			continue
		}

		// Number or Operator
		start := i
		for i < len(data) && !unicode.IsSpace(rune(data[i])) && !isDelim(data[i]) {
			i++
		}
		val := string(data[start:i])
		// Very basic check if it's a number
		if isNumber(val) {
			tokens = append(tokens, Token{Type: "Number", Value: val})
		} else {
			tokens = append(tokens, Token{Type: "Operator", Value: val})
		}
	}
	return tokens
}

func isDelim(c byte) bool {
	return c == '/' || c == '[' || c == ']' || c == '{' || c == '}' || c == '(' || c == ')' || c == '<' || c == '>' || c == '%'
}

func isNumber(s string) bool {
	if len(s) == 0 {
		return false
	}
	digits := 0
	dots := 0
	for i, c := range s {
		if (c == '+' || c == '-') && i == 0 {
			continue
		}
		if c == '.' {
			dots++
			if dots > 1 {
				return false
			}
			continue
		}
		if !unicode.IsDigit(c) {
			return false
		}
		digits++
	}
	return digits > 0
}

// ParseDicts basic PostScript dictionary parser returning map of properties.
func ParseDicts(tokens []Token) map[string][]Token {
	dicts := make(map[string][]Token)
	var currentKey string
	var currentArray []Token
	inArray := 0

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.Type == "Name" {
			if inArray == 0 {
				currentKey = t.Value
				currentArray = nil
			} else {
				currentArray = append(currentArray, t)
			}
		} else if t.Type == "Delim" && (t.Value == "[" || t.Value == "{") {
			inArray++
		} else if t.Type == "Delim" && (t.Value == "]" || t.Value == "}") {
			if inArray == 0 {
				currentKey = ""
				currentArray = nil
				continue
			}
			inArray--
			if inArray == 0 && currentKey != "" {
				dicts[currentKey] = currentArray
				currentKey = ""
			}
		} else if inArray > 0 {
			currentArray = append(currentArray, t)
		} else if t.Type == "Number" || t.Type == "String" || t.Type == "HexString" {
			if currentKey != "" {
				dicts[currentKey] = []Token{t}
				currentKey = ""
			}
		}
	}
	return dicts
}

// ExtractEexec extracts and decrypts the eexec section.
func ExtractEexec(data []byte) ([]byte, error) {
	_, eexecEnd := findType1Eexec(data)
	if eexecEnd == -1 {
		return nil, errors.New("eexec not found")
	}

	startIdx := eexecEnd
	// skip whitespace
	for startIdx < len(data) && unicode.IsSpace(rune(data[startIdx])) {
		startIdx++
	}

	encData := data[startIdx:]

	// Determine if it's hex or binary.
	// PFB is usually binary, PFA is hex.
	// If the first few bytes are valid hex characters, it might be hex.
	isHex := true
	for i := 0; i < 8 && i < len(encData); i++ {
		c := encData[i]
		if !unicode.IsSpace(rune(c)) && !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')) {
			isHex = false
			break
		}
	}

	var rawBytes []byte
	if isHex {
		// Clean hex string (remove whitespaces)
		var hexBytes []byte
		zeroCount := 0
		for i := 0; i < len(encData); i++ {
			c := encData[i]
			if unicode.IsSpace(rune(c)) {
				continue
			}
			if c == '0' {
				zeroCount++
				if zeroCount == 16 {
					hexBytes = hexBytes[:len(hexBytes)-15]
					break
				}
			} else {
				zeroCount = 0
			}
			hexBytes = append(hexBytes, c)
			// Stop if we hit 512 zeros (cleartomark) but usually PFA stops at end.
		}
		decoded, err := hex.DecodeString(string(hexBytes))
		if err == nil {
			rawBytes = decoded
		} else {
			// fallback to binary if hex decoding fails
			rawBytes = encData
		}
	} else {
		rawBytes = encData
	}

	decrypted := DecryptEexec(rawBytes, 55665)
	if len(decrypted) < 4 {
		return nil, errors.New("eexec data too short")
	}
	// The first 4 bytes are padding
	return decrypted[4:], nil
}
