package type1

import (
	"errors"
	"fmt"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

const (
	maxType1OperandStack = 24
	maxType1SubrNesting  = 10
)

// CharStringSEAC describes a Type 1 Standard Encoding accented character.
type CharStringSEAC struct {
	ASB        int32
	ADX        int32
	ADY        int32
	BaseChar   int32
	AccentChar int32
}

// CharStringHintKind describes the broad class of a decoded Type 1 hint record.
type CharStringHintKind string

const (
	CharStringHintStem       CharStringHintKind = "stem"
	CharStringHintCounter    CharStringHintKind = "counter"
	CharStringHintDotSection CharStringHintKind = "dotsection"
)

// CharStringStemOrientation describes whether a stem constrains x or y.
type CharStringStemOrientation string

const (
	CharStringStemHorizontal CharStringStemOrientation = "horizontal"
	CharStringStemVertical   CharStringStemOrientation = "vertical"
)

// CharStringStemHint is a single decoded Type 1 stem pair in font units.
type CharStringStemHint struct {
	Orientation CharStringStemOrientation
	Position    float64
	Width       float64
}

// CharStringHint records a Type 1 hint-related operator and the operands it consumed.
type CharStringHint struct {
	Kind     CharStringHintKind
	Operator string
	Operands []float64
	Stems    []CharStringStemHint
}

// CharStringSegmentKind describes a decoded Type 1 path segment.
type CharStringSegmentKind uint8

const (
	CharStringSegmentMove CharStringSegmentKind = iota
	CharStringSegmentLine
	CharStringSegmentCubic
	CharStringSegmentClose
)

// CharStringSegment preserves native Type 1 path commands in design 26.6 units.
//
// Move and line segments use Points[0]. Cubic segments use Points[0] and
// Points[1] as control points, and Points[2] as the endpoint. Close segments
// do not use Points.
type CharStringSegment struct {
	Kind   CharStringSegmentKind
	Points [3]api.Vector
}

// CharStringResult is the decoded result of a Type 1 charstring.
type CharStringResult struct {
	Outline     *core.Outline
	SideBearing api.Vector
	Width       api.Vector
	SEAC        *CharStringSEAC
	Hints       []CharStringHint
	Segments    []CharStringSegment
}

// DecryptCharString decrypts a Type 1 charstring using the standard key.
// lenIV is usually 4. A negative lenIV marks the charstring as unencrypted.
func DecryptCharString(data []byte, lenIV int) ([]byte, error) {
	if lenIV < 0 {
		out := append([]byte(nil), data...)
		return out, nil
	}
	decrypted := DecryptEexec(data, 4330)
	if len(decrypted) < lenIV {
		return nil, errors.New("charstring data shorter than lenIV")
	}
	out := append([]byte(nil), decrypted[lenIV:]...)
	return out, nil
}

// DecodeCharString decodes a decrypted Type 1 charstring into an outline.
// subrs are decrypted local Subrs indexed directly by callsubr.
func DecodeCharString(data []byte, subrs [][]byte) (*CharStringResult, error) {
	result := &CharStringResult{Outline: &core.Outline{}}
	ctx := &type1CharStringContext{
		result: result,
		subrs:  subrs,
	}
	if err := ctx.interpretTopLevel(data); err != nil {
		return nil, err
	}
	return result, nil
}

// DecodeMetrics decodes a Type 1 charstring far enough to expose side bearing,
// advance, and SEAC metadata. It uses the same interpreter as DecodeCharString.
func DecodeMetrics(data []byte, subrs [][]byte) (sideBearing, width api.Vector, seac *CharStringSEAC, err error) {
	result, err := DecodeCharString(data, subrs)
	if err != nil {
		return api.Vector{}, api.Vector{}, nil, err
	}
	return result.SideBearing, result.Width, result.SEAC, nil
}

type type1CharStringContext struct {
	stack            []float64
	x                float64
	y                float64
	result           *CharStringResult
	subrs            [][]byte
	nesting          int
	contourOpen      bool
	flexState        bool
	flexPoints       []type1FlexPoint
	otherSubrResults []float64
}

type type1FlexPoint struct {
	x float64
	y float64
}

type type1CharStringStop uint8

const (
	type1CharStringStopEOF type1CharStringStop = iota
	type1CharStringStopReturn
	type1CharStringStopEndChar
)

func (c *type1CharStringContext) interpretTopLevel(data []byte) error {
	stop, err := c.interpret(data)
	if err != nil {
		return err
	}
	switch stop {
	case type1CharStringStopEndChar:
		return nil
	case type1CharStringStopReturn:
		return errors.New("return in top-level type1 charstring")
	default:
		return errors.New("type1 charstring missing endchar")
	}
}

func (c *type1CharStringContext) interpret(data []byte) (type1CharStringStop, error) {
	if c.nesting > maxType1SubrNesting {
		return 0, errors.New("type1 charstring subroutine nesting too deep")
	}
	c.nesting++
	defer func() { c.nesting-- }()

	for i := 0; i < len(data); {
		b := data[i]
		if b >= 32 {
			if err := c.ensureOtherSubrResultsConsumed("operand"); err != nil {
				return 0, err
			}
			v, next, err := decodeType1CharStringNumber(data, i)
			if err != nil {
				return 0, err
			}
			if err := c.pushOperand(v); err != nil {
				return 0, err
			}
			i = next
			continue
		}
		i++

		op := int(b)
		if op == 12 {
			if i >= len(data) {
				return 0, errors.New("missing second byte for type1 charstring escape operator")
			}
			op = 1200 + int(data[i])
			i++
		}

		if op != 1217 {
			if err := c.ensureOtherSubrResultsConsumed(type1CharStringOperatorName(op)); err != nil {
				return 0, err
			}
		}

		switch op {
		case 1: // hstem
			if err := c.consumeStem("hstem", CharStringStemHorizontal); err != nil {
				return 0, err
			}
		case 3: // vstem
			if err := c.consumeStem("vstem", CharStringStemVertical); err != nil {
				return 0, err
			}
		case 4: // vmoveto
			args, err := c.consumeArgs(1, "vmoveto")
			if err != nil {
				return 0, err
			}
			c.rmoveto(0, args[0])
		case 5: // rlineto
			if len(c.stack) == 0 || len(c.stack)%2 != 0 {
				return 0, errors.New("invalid operand count in rlineto")
			}
			args := c.takeStack()
			for j := 0; j < len(args); j += 2 {
				if err := c.rlineto(args[j], args[j+1]); err != nil {
					return 0, err
				}
			}
		case 6: // hlineto
			args, err := c.consumeArgs(1, "hlineto")
			if err != nil {
				return 0, err
			}
			if err := c.rlineto(args[0], 0); err != nil {
				return 0, err
			}
		case 7: // vlineto
			args, err := c.consumeArgs(1, "vlineto")
			if err != nil {
				return 0, err
			}
			if err := c.rlineto(0, args[0]); err != nil {
				return 0, err
			}
		case 8: // rrcurveto
			if len(c.stack) == 0 || len(c.stack)%6 != 0 {
				return 0, errors.New("invalid operand count in rrcurveto")
			}
			args := c.takeStack()
			for j := 0; j < len(args); j += 6 {
				if err := c.rrcurveto(args[j], args[j+1], args[j+2], args[j+3], args[j+4], args[j+5]); err != nil {
					return 0, err
				}
			}
		case 9: // closepath
			c.closeContour(true)
			c.stack = c.stack[:0]
		case 10: // callsubr
			if len(c.stack) < 1 {
				return 0, errors.New("stack underflow in callsubr")
			}
			idx := int(c.stack[len(c.stack)-1])
			c.stack = c.stack[:len(c.stack)-1]
			if idx < 0 || idx >= len(c.subrs) {
				return 0, fmt.Errorf("type1 local subr %d out of range", idx)
			}
			stop, err := c.interpret(c.subrs[idx])
			if err != nil {
				return 0, err
			}
			switch stop {
			case type1CharStringStopReturn:
			case type1CharStringStopEndChar:
				return type1CharStringStopEndChar, nil
			default:
				return 0, fmt.Errorf("type1 local subr %d missing return", idx)
			}
		case 11: // return
			if len(c.otherSubrResults) != 0 {
				return 0, errors.New("pending OtherSubr pop results before return")
			}
			return type1CharStringStopReturn, nil
		case 13: // hsbw
			args, err := c.consumeArgs(2, "hsbw")
			if err != nil {
				return 0, err
			}
			c.setMetrics(args[0], 0, args[1], 0)
		case 14: // endchar
			if err := c.ensureTerminatorReady("endchar"); err != nil {
				return 0, err
			}
			c.closeContour(false)
			c.stack = c.stack[:0]
			return type1CharStringStopEndChar, nil
		case 21: // rmoveto
			args, err := c.consumeArgs(2, "rmoveto")
			if err != nil {
				return 0, err
			}
			c.rmoveto(args[0], args[1])
		case 22: // hmoveto
			args, err := c.consumeArgs(1, "hmoveto")
			if err != nil {
				return 0, err
			}
			c.rmoveto(args[0], 0)
		case 30: // vhcurveto
			args, err := c.consumeArgs(4, "vhcurveto")
			if err != nil {
				return 0, err
			}
			if err := c.rrcurveto(0, args[0], args[1], args[2], args[3], 0); err != nil {
				return 0, err
			}
		case 31: // hvcurveto
			args, err := c.consumeArgs(4, "hvcurveto")
			if err != nil {
				return 0, err
			}
			if err := c.rrcurveto(args[0], 0, args[1], args[2], 0, args[3]); err != nil {
				return 0, err
			}
		case 1200: // dotsection
			c.recordHint(CharStringHint{
				Kind:     CharStringHintDotSection,
				Operator: "dotsection",
				Operands: copyType1CharStringOperands(c.stack),
			})
			c.stack = c.stack[:0]
		case 1201: // vstem3
			if err := c.consumeTripleStem("vstem3", CharStringStemVertical); err != nil {
				return 0, err
			}
		case 1202: // hstem3
			if err := c.consumeTripleStem("hstem3", CharStringStemHorizontal); err != nil {
				return 0, err
			}
		case 1206: // seac
			args, err := c.consumeArgs(5, "seac")
			if err != nil {
				return 0, err
			}
			if err := c.ensureTerminatorReady("seac"); err != nil {
				return 0, err
			}
			c.result.SEAC = &CharStringSEAC{
				ASB:        int32(args[0]),
				ADX:        int32(args[1]),
				ADY:        int32(args[2]),
				BaseChar:   int32(args[3]),
				AccentChar: int32(args[4]),
			}
			return type1CharStringStopEndChar, nil
		case 1207: // sbw
			args, err := c.consumeArgs(4, "sbw")
			if err != nil {
				return 0, err
			}
			c.setMetrics(args[0], args[1], args[2], args[3])
		case 1212: // div
			if len(c.stack) < 2 {
				return 0, errors.New("stack underflow in div")
			}
			n := len(c.stack)
			if c.stack[n-1] == 0 {
				return 0, errors.New("division by zero in div")
			}
			c.stack[n-2] = c.stack[n-2] / c.stack[n-1]
			c.stack = c.stack[:n-1]
		case 1216: // callothersubr
			if err := c.callOtherSubr(); err != nil {
				return 0, err
			}
		case 1217: // pop
			if err := c.popOtherSubrResult(); err != nil {
				return 0, err
			}
		case 1233: // setcurrentpoint
			args, err := c.consumeArgs(2, "setcurrentpoint")
			if err != nil {
				return 0, err
			}
			c.x = args[0]
			c.y = args[1]
			c.flexState = false
			c.flexPoints = c.flexPoints[:0]
		default:
			return 0, fmt.Errorf("unsupported type1 charstring operator %d", op)
		}
	}
	return type1CharStringStopEOF, nil
}

func decodeType1CharStringNumber(data []byte, i int) (float64, int, error) {
	b := data[i]
	switch {
	case b >= 32 && b <= 246:
		return float64(int(b) - 139), i + 1, nil
	case b >= 247 && b <= 250:
		if i+1 >= len(data) {
			return 0, 0, errors.New("unexpected EOF reading positive type1 charstring number")
		}
		return float64((int(b)-247)*256 + int(data[i+1]) + 108), i + 2, nil
	case b >= 251 && b <= 254:
		if i+1 >= len(data) {
			return 0, 0, errors.New("unexpected EOF reading negative type1 charstring number")
		}
		return float64(-(int(b)-251)*256 - int(data[i+1]) - 108), i + 2, nil
	case b == 255:
		if i+4 >= len(data) {
			return 0, 0, errors.New("unexpected EOF reading long type1 charstring number")
		}
		v := int32(uint32(data[i+1])<<24 | uint32(data[i+2])<<16 | uint32(data[i+3])<<8 | uint32(data[i+4]))
		return float64(v), i + 5, nil
	default:
		return 0, 0, fmt.Errorf("invalid type1 charstring number byte %d", b)
	}
}

func (c *type1CharStringContext) takeStack() []float64 {
	args := append([]float64(nil), c.stack...)
	c.stack = c.stack[:0]
	return args
}

func (c *type1CharStringContext) pushOperand(v float64) error {
	if len(c.stack) >= maxType1OperandStack {
		return errors.New("type1 charstring operand stack overflow")
	}
	c.stack = append(c.stack, v)
	return nil
}

func (c *type1CharStringContext) consumeArgs(count int, op string) ([]float64, error) {
	if len(c.stack) < count {
		return nil, fmt.Errorf("stack underflow in %s", op)
	}
	if len(c.stack) != count {
		return nil, fmt.Errorf("invalid operand count in %s", op)
	}
	return c.takeStack(), nil
}

func (c *type1CharStringContext) consumeStem(op string, orientation CharStringStemOrientation) error {
	if len(c.stack) == 0 || len(c.stack)%2 != 0 {
		return fmt.Errorf("invalid operand count in %s", op)
	}
	c.recordStemHint(CharStringHintStem, op, orientation, c.takeStack())
	return nil
}

func (c *type1CharStringContext) consumeTripleStem(op string, orientation CharStringStemOrientation) error {
	if len(c.stack) != 6 {
		return fmt.Errorf("invalid operand count in %s", op)
	}
	c.recordStemHint(CharStringHintCounter, op, orientation, c.takeStack())
	return nil
}

func (c *type1CharStringContext) recordStemHint(kind CharStringHintKind, op string, orientation CharStringStemOrientation, operands []float64) {
	stems := make([]CharStringStemHint, 0, len(operands)/2)
	for i := 0; i+1 < len(operands); i += 2 {
		stems = append(stems, CharStringStemHint{
			Orientation: orientation,
			Position:    operands[i],
			Width:       operands[i+1],
		})
	}
	c.recordHint(CharStringHint{
		Kind:     kind,
		Operator: op,
		Operands: copyType1CharStringOperands(operands),
		Stems:    stems,
	})
}

func (c *type1CharStringContext) recordHint(hint CharStringHint) {
	c.result.Hints = append(c.result.Hints, hint)
}

func copyType1CharStringOperands(operands []float64) []float64 {
	if len(operands) == 0 {
		return nil
	}
	return append([]float64(nil), operands...)
}

func (c *type1CharStringContext) setMetrics(sbx, sby, wx, wy float64) {
	c.result.SideBearing = type1CharStringPoint(sbx, sby)
	c.result.Width = type1CharStringPoint(wx, wy)
	c.x = sbx
	c.y = sby
}

func (c *type1CharStringContext) rmoveto(dx, dy float64) {
	if c.flexState {
		c.x += dx
		c.y += dy
		return
	}
	c.closeContour(false)
	c.x += dx
	c.y += dy
	p := type1CharStringPoint(c.x, c.y)
	c.result.Outline.Points = append(c.result.Outline.Points, p)
	c.result.Outline.Tags = append(c.result.Outline.Tags, 1)
	c.recordSegment(CharStringSegment{
		Kind:   CharStringSegmentMove,
		Points: [3]api.Vector{p},
	})
	c.contourOpen = true
}

func (c *type1CharStringContext) rlineto(dx, dy float64) error {
	if !c.contourOpen {
		return errors.New("lineto without current contour")
	}
	c.x += dx
	c.y += dy
	p := type1CharStringPoint(c.x, c.y)
	c.result.Outline.Points = append(c.result.Outline.Points, p)
	c.result.Outline.Tags = append(c.result.Outline.Tags, 1)
	c.recordSegment(CharStringSegment{
		Kind:   CharStringSegmentLine,
		Points: [3]api.Vector{p},
	})
	return nil
}

func (c *type1CharStringContext) rrcurveto(dx1, dy1, dx2, dy2, dx3, dy3 float64) error {
	if !c.contourOpen {
		return errors.New("curveto without current contour")
	}
	x0, y0 := c.x, c.y
	x1, y1 := x0+dx1, y0+dy1
	x2, y2 := x1+dx2, y1+dy2
	x3, y3 := x2+dx3, y2+dy3
	c.recordSegment(CharStringSegment{
		Kind: CharStringSegmentCubic,
		Points: [3]api.Vector{
			type1CharStringPoint(x1, y1),
			type1CharStringPoint(x2, y2),
			type1CharStringPoint(x3, y3),
		},
	})
	c.result.Outline.Points = append(c.result.Outline.Points,
		type1CharStringPoint(x1, y1),
		type1CharStringPoint(x2, y2),
		type1CharStringPoint(x3, y3),
	)
	c.result.Outline.Tags = append(c.result.Outline.Tags, 2, 2, 1)
	c.x = x3
	c.y = y3
	return nil
}

func (c *type1CharStringContext) callOtherSubr() error {
	if len(c.stack) < 2 {
		return errors.New("stack underflow in callothersubr")
	}
	subrNo, err := type1CharStringIntOperand(c.stack[len(c.stack)-1], "subroutine number", "callothersubr")
	if err != nil {
		return err
	}
	argCount, err := type1CharStringIntOperand(c.stack[len(c.stack)-2], "argument count", "callothersubr")
	if err != nil {
		return err
	}
	if argCount < 0 || argCount > len(c.stack)-2 {
		return errors.New("stack underflow in callothersubr")
	}
	if len(c.otherSubrResults) != 0 {
		return errors.New("pending OtherSubr pop results before callothersubr")
	}
	argsStart := len(c.stack) - 2 - argCount
	args := append([]float64(nil), c.stack[argsStart:len(c.stack)-2]...)
	c.stack = c.stack[:argsStart]
	c.otherSubrResults = c.otherSubrResults[:0]

	switch subrNo {
	case 0: // end flex feature
		if argCount != 3 {
			return errors.New("invalid operand count in flex end")
		}
		if !c.flexState || len(c.flexPoints) != 7 {
			return errors.New("unexpected flex end")
		}
		if err := c.drawFlex(); err != nil {
			return err
		}
		c.otherSubrResults = append(c.otherSubrResults, c.x, c.y)
	case 1: // start flex feature
		if argCount != 0 {
			return errors.New("invalid operand count in flex start")
		}
		c.flexState = true
		c.flexPoints = c.flexPoints[:0]
		if !c.contourOpen {
			p := type1CharStringPoint(c.x, c.y)
			c.result.Outline.Points = append(c.result.Outline.Points, p)
			c.result.Outline.Tags = append(c.result.Outline.Tags, 1)
			c.recordSegment(CharStringSegment{
				Kind:   CharStringSegmentMove,
				Points: [3]api.Vector{p},
			})
			c.contourOpen = true
		}
	case 2: // add flex vector
		if argCount != 0 {
			return errors.New("invalid operand count in flex vector")
		}
		if !c.flexState {
			return errors.New("missing flex start")
		}
		if len(c.flexPoints) >= 7 {
			return errors.New("too many flex vectors")
		}
		c.flexPoints = append(c.flexPoints, type1FlexPoint{x: c.x, y: c.y})
	case 3: // change hints
		if argCount != 1 {
			return errors.New("invalid operand count in change hints")
		}
		c.otherSubrResults = append(c.otherSubrResults, args[0])
	case 12, 13: // counter control hints
		c.recordHint(CharStringHint{
			Kind:     CharStringHintCounter,
			Operator: fmt.Sprintf("othersubr%d", subrNo),
			Operands: copyType1CharStringOperands(args),
		})
		c.stack = c.stack[:0]
	default:
		return fmt.Errorf("unsupported type1 callothersubr %d", subrNo)
	}
	return nil
}

func type1CharStringIntOperand(v float64, name, op string) (int, error) {
	i := int(v)
	if float64(i) != v {
		return 0, fmt.Errorf("invalid %s in %s", name, op)
	}
	return i, nil
}

func (c *type1CharStringContext) popOtherSubrResult() error {
	if len(c.otherSubrResults) == 0 {
		return errors.New("unsupported type1 pop")
	}
	v := c.otherSubrResults[0]
	c.otherSubrResults = c.otherSubrResults[1:]
	if err := c.pushOperand(v); err != nil {
		return err
	}
	return nil
}

func (c *type1CharStringContext) ensureTerminatorReady(op string) error {
	if c.flexState {
		return fmt.Errorf("unterminated flex sequence before %s", op)
	}
	return c.ensureOtherSubrResultsConsumed(op)
}

func (c *type1CharStringContext) ensureOtherSubrResultsConsumed(next string) error {
	if len(c.otherSubrResults) != 0 {
		return fmt.Errorf("pending OtherSubr pop results before %s", next)
	}
	return nil
}

func type1CharStringOperatorName(op int) string {
	switch op {
	case 1:
		return "hstem"
	case 3:
		return "vstem"
	case 4:
		return "vmoveto"
	case 5:
		return "rlineto"
	case 6:
		return "hlineto"
	case 7:
		return "vlineto"
	case 8:
		return "rrcurveto"
	case 9:
		return "closepath"
	case 10:
		return "callsubr"
	case 11:
		return "return"
	case 13:
		return "hsbw"
	case 14:
		return "endchar"
	case 21:
		return "rmoveto"
	case 22:
		return "hmoveto"
	case 30:
		return "vhcurveto"
	case 31:
		return "hvcurveto"
	case 1200:
		return "dotsection"
	case 1201:
		return "vstem3"
	case 1202:
		return "hstem3"
	case 1206:
		return "seac"
	case 1207:
		return "sbw"
	case 1212:
		return "div"
	case 1216:
		return "callothersubr"
	case 1217:
		return "pop"
	case 1233:
		return "setcurrentpoint"
	default:
		return fmt.Sprintf("operator %d", op)
	}
}

func (c *type1CharStringContext) drawFlex() error {
	p := c.flexPoints
	if len(p) != 7 {
		return errors.New("invalid flex vector count")
	}
	c.x = p[0].x
	c.y = p[0].y
	if !c.contourOpen {
		p0 := type1CharStringPoint(c.x, c.y)
		c.result.Outline.Points = append(c.result.Outline.Points, p0)
		c.result.Outline.Tags = append(c.result.Outline.Tags, 1)
		c.recordSegment(CharStringSegment{
			Kind:   CharStringSegmentMove,
			Points: [3]api.Vector{p0},
		})
		c.contourOpen = true
	}
	if err := c.rrcurveto(
		p[1].x-p[0].x, p[1].y-p[0].y,
		p[2].x-p[1].x, p[2].y-p[1].y,
		p[3].x-p[2].x, p[3].y-p[2].y,
	); err != nil {
		return err
	}
	if err := c.rrcurveto(
		p[4].x-p[3].x, p[4].y-p[3].y,
		p[5].x-p[4].x, p[5].y-p[4].y,
		p[6].x-p[5].x, p[6].y-p[5].y,
	); err != nil {
		return err
	}
	return nil
}

func (c *type1CharStringContext) closeContour(recordSegment bool) {
	if !c.contourOpen || len(c.result.Outline.Points) == 0 {
		return
	}
	end := len(c.result.Outline.Points) - 1
	contours := c.result.Outline.Contours
	if len(contours) == 0 || contours[len(contours)-1] != end {
		c.result.Outline.Contours = append(c.result.Outline.Contours, end)
	}
	if recordSegment {
		c.recordSegment(CharStringSegment{Kind: CharStringSegmentClose})
	}
	c.contourOpen = false
}

func (c *type1CharStringContext) recordSegment(segment CharStringSegment) {
	c.result.Segments = append(c.result.Segments, segment)
}

func type1CharStringPoint(x, y float64) api.Vector {
	return api.Vector{X: int32(x * 64), Y: int32(y * 64)}
}
