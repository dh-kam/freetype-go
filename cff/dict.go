package cff

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseDict parses a CFF DICT structure.
func ParseDict(data []byte) (map[int][]float64, error) {
	dict := make(map[int][]float64)
	var stack []float64

	i := 0
	for i < len(data) {
		b0 := data[i]
		i++

		if b0 <= 25 {
			// Operator
			var op int
			if b0 == 12 {
				if i >= len(data) {
					return nil, fmt.Errorf("missing second byte for operator 12")
				}
				b1 := data[i]
				i++
				op = 12<<8 | int(b1)
			} else {
				op = int(b0)
			}
			dict[op] = append([]float64(nil), stack...)
			stack = stack[:0]
		} else if b0 == 28 {
			// 16-bit integer
			if i+1 >= len(data) {
				return nil, fmt.Errorf("unexpected EOF reading 16-bit int")
			}
			val := int16(uint16(data[i])<<8 | uint16(data[i+1]))
			var err error
			stack, err = pushDictOperand(stack, float64(val))
			if err != nil {
				return nil, err
			}
			i += 2
		} else if b0 == 29 {
			// 32-bit integer
			if i+3 >= len(data) {
				return nil, fmt.Errorf("unexpected EOF reading 32-bit int")
			}
			val := int32(uint32(data[i])<<24 | uint32(data[i+1])<<16 | uint32(data[i+2])<<8 | uint32(data[i+3]))
			var err error
			stack, err = pushDictOperand(stack, float64(val))
			if err != nil {
				return nil, err
			}
			i += 4
		} else if b0 == 30 {
			// Real number (BCD)
			var sb strings.Builder
			done := false
			for !done {
				if i >= len(data) {
					return nil, fmt.Errorf("unexpected EOF reading BCD")
				}
				b := data[i]
				i++
				for _, nibble := range []byte{b >> 4, b & 0x0f} {
					switch nibble {
					case 0x0, 0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9:
						sb.WriteByte('0' + nibble)
					case 0xa:
						sb.WriteByte('.')
					case 0xb:
						sb.WriteByte('E')
					case 0xc:
						sb.WriteString("E-")
					case 0xd:
						// Reserved
						return nil, fmt.Errorf("invalid BCD nibble 0xd")
					case 0xe:
						sb.WriteByte('-')
					case 0xf:
						done = true
						break
					}
					if done {
						break
					}
				}
			}
			val, err := strconv.ParseFloat(sb.String(), 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse BCD float %q: %v", sb.String(), err)
			}
			stack, err = pushDictOperand(stack, val)
			if err != nil {
				return nil, err
			}
		} else if b0 >= 32 && b0 <= 246 {
			var err error
			stack, err = pushDictOperand(stack, float64(int(b0)-139))
			if err != nil {
				return nil, err
			}
		} else if b0 >= 247 && b0 <= 250 {
			if i >= len(data) {
				return nil, fmt.Errorf("unexpected EOF reading short int")
			}
			b1 := data[i]
			i++
			val := (int(b0)-247)*256 + int(b1) + 108
			var err error
			stack, err = pushDictOperand(stack, float64(val))
			if err != nil {
				return nil, err
			}
		} else if b0 >= 251 && b0 <= 254 {
			if i >= len(data) {
				return nil, fmt.Errorf("unexpected EOF reading short int")
			}
			b1 := data[i]
			i++
			val := -(int(b0)-251)*256 - int(b1) - 108
			var err error
			stack, err = pushDictOperand(stack, float64(val))
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("invalid DICT byte %d", b0)
		}
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unterminated DICT operands")
	}

	return dict, nil
}

func pushDictOperand(stack []float64, v float64) ([]float64, error) {
	if len(stack) >= maxCFF2ArgumentStack {
		return nil, fmt.Errorf("DICT operand stack overflow")
	}
	return append(stack, v), nil
}
