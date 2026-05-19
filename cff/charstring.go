package cff

import (
	"errors"
	"fmt"
	"math"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

const (
	maxCFF1ArgumentStack = 48
	maxCFF2ArgumentStack = 513
)

type charStringContext struct {
	stack        []float64
	x, y         float64
	outline      *core.Outline
	segments     []charStringSegment
	globalSubrs  *Index
	localSubrs   *Index
	blendVector  []float64
	zeroBlend    []float64
	blendVectors map[int][]float64
	varStore     *VariationStore
	varCoords    []float64
	nesting      int
	widthParsed  bool
	hintCount    int
	vsIndex      int
	maxStack     int
	transient    [32]float64
	randomSeed   uint32
}

type charStringSegmentKind uint8

const (
	charStringMoveSegment charStringSegmentKind = iota
	charStringLineSegment
	charStringCubicSegment
)

type charStringSegment struct {
	kind  charStringSegmentKind
	from  api.Vector
	ctrl1 api.Vector
	ctrl2 api.Vector
	to    api.Vector
}

type charStringResult struct {
	outline  *core.Outline
	segments []charStringSegment
}

type charStringDecodeOptions struct {
	globalSubrs     *Index
	localSubrs      *Index
	blendVector     []float64
	variationStore  *VariationStore
	variationCoords []float64
	defaultVSIndex  int
	maxStack        int
}

func (c *charStringContext) stackLimit() int {
	if c.maxStack > 0 {
		return c.maxStack
	}
	return maxCFF1ArgumentStack
}

func (c *charStringContext) push(v float64) error {
	if len(c.stack) >= c.stackLimit() {
		return errors.New("charstring operand stack overflow")
	}
	c.stack = append(c.stack, v)
	return nil
}

func (c *charStringContext) pop() (float64, error) {
	if len(c.stack) == 0 {
		return 0, errors.New("charstring stack underflow")
	}
	v := c.stack[len(c.stack)-1]
	c.stack = c.stack[:len(c.stack)-1]
	return v, nil
}

func (c *charStringContext) removeFirstOperand() {
	copy(c.stack, c.stack[1:])
	c.stack = c.stack[:len(c.stack)-1]
}

func (c *charStringContext) markWidthParsed() {
	if !c.widthParsed {
		c.widthParsed = true
	}
}

func (c *charStringContext) consumeFixedArgs(expected int, op string) error {
	if len(c.stack) < expected {
		return fmt.Errorf("stack underflow in %s", op)
	}
	if !c.widthParsed {
		switch len(c.stack) {
		case expected:
			c.widthParsed = true
		case expected + 1:
			c.removeFirstOperand()
			c.widthParsed = true
		default:
			return fmt.Errorf("invalid operand count in %s", op)
		}
		return nil
	}
	if len(c.stack) != expected {
		return fmt.Errorf("invalid operand count in %s", op)
	}
	return nil
}

func (c *charStringContext) consumeStemHints(op string, allowEmpty bool) error {
	operands := len(c.stack)
	if !c.widthParsed {
		if operands%2 == 1 {
			c.removeFirstOperand()
			operands--
		}
		c.widthParsed = true
	}
	if operands == 0 {
		if allowEmpty {
			c.stack = c.stack[:0]
			return nil
		}
		return fmt.Errorf("stack underflow in %s", op)
	}
	if operands%2 != 0 {
		return fmt.Errorf("invalid operand count in %s", op)
	}
	c.hintCount += operands / 2
	c.stack = c.stack[:0]
	return nil
}

func (c *charStringContext) unaryStackOp(op string, fn func(float64) (float64, error)) error {
	if len(c.stack) < 1 {
		return fmt.Errorf("stack underflow in %s", op)
	}
	top := len(c.stack) - 1
	result, err := fn(c.stack[top])
	if err != nil {
		return err
	}
	c.stack[top] = result
	return nil
}

func (c *charStringContext) binaryStackOp(op string, fn func(float64, float64) (float64, error)) error {
	if len(c.stack) < 2 {
		return fmt.Errorf("stack underflow in %s", op)
	}
	left := len(c.stack) - 2
	result, err := fn(c.stack[left], c.stack[left+1])
	if err != nil {
		return err
	}
	c.stack[left] = result
	c.stack = c.stack[:left+1]
	return nil
}

func boolNumber(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func charStringIntOperand(v float64, name, op string) (int, error) {
	i := int(v)
	if float64(i) != v {
		return 0, fmt.Errorf("invalid %s in %s", name, op)
	}
	return i, nil
}

func calculateBias(count int) int {
	if count < 1240 {
		return 107
	} else if count < 33900 {
		return 1131
	}
	return 32768
}

func DecodeCharString(data []byte, globalSubrs *Index, localSubrs *Index, blendVector []float64) (*core.Outline, error) {
	result, err := decodeCharString(data, charStringDecodeOptions{
		globalSubrs: globalSubrs,
		localSubrs:  localSubrs,
		blendVector: blendVector,
	})
	if err != nil {
		return nil, err
	}
	return result.outline, nil
}

func decodeCharString(data []byte, opts charStringDecodeOptions) (*charStringResult, error) {
	ctx := &charStringContext{
		outline:     &core.Outline{},
		globalSubrs: opts.globalSubrs,
		localSubrs:  opts.localSubrs,
		blendVector: opts.blendVector,
		varStore:    opts.variationStore,
		varCoords:   opts.variationCoords,
		vsIndex:     opts.defaultVSIndex,
		maxStack:    opts.maxStack,
	}
	err := ctx.interpret(data)
	if err != nil {
		return nil, err
	}
	return &charStringResult{outline: ctx.outline, segments: ctx.segments}, nil
}

func (c *charStringContext) interpret(data []byte) error {
	if c.nesting > 10 {
		return errors.New("charstring subroutine nesting too deep")
	}
	c.nesting++
	defer func() { c.nesting-- }()

	i := 0
	for i < len(data) {
		b := data[i]
		i++

		if b >= 32 || b == 28 {
			// Number
			var v float64
			if b == 28 {
				if i+2 > len(data) {
					return errors.New("unexpected EOF reading short charstring number")
				}
				v = float64(int16(uint16(data[i])<<8 | uint16(data[i+1])))
				i += 2
			} else if b >= 32 && b <= 246 {
				v = float64(int(b) - 139)
			} else if b >= 247 && b <= 250 {
				if i >= len(data) {
					return errors.New("unexpected EOF reading positive charstring number")
				}
				b2 := data[i]
				i++
				v = float64((int(b)-247)*256 + int(b2) + 108)
			} else if b >= 251 && b <= 254 {
				if i >= len(data) {
					return errors.New("unexpected EOF reading negative charstring number")
				}
				b2 := data[i]
				i++
				v = float64(-(int(b)-251)*256 - int(b2) - 108)
			} else if b == 255 {
				// 16.16 fixed point
				if i+4 > len(data) {
					return errors.New("unexpected EOF reading fixed charstring number")
				}
				v = float64(int16(uint16(data[i])<<8|uint16(data[i+1]))) + float64(uint16(data[i+2])<<8|uint16(data[i+3]))/65536.0
				i += 4
			}
			if err := c.push(v); err != nil {
				return err
			}
		} else {
			// Operator
			op := int(b)
			if op == 12 {
				// Escape
				if i >= len(data) {
					return errors.New("missing second byte for charstring escape operator")
				}
				op = 1200 + int(data[i])
				i++
			}

			switch op {
			case 1: // hstem
				if err := c.consumeStemHints("hstem", false); err != nil {
					return err
				}
			case 3: // vstem
				if err := c.consumeStemHints("vstem", false); err != nil {
					return err
				}
			case 21: // rmoveto
				if err := c.consumeFixedArgs(2, "rmoveto"); err != nil {
					return err
				}
				c.rmoveto(c.stack[0], c.stack[1])
				c.stack = c.stack[:0]
			case 22: // hmoveto
				if err := c.consumeFixedArgs(1, "hmoveto"); err != nil {
					return err
				}
				c.rmoveto(c.stack[0], 0)
				c.stack = c.stack[:0]
			case 4: // vmoveto
				if err := c.consumeFixedArgs(1, "vmoveto"); err != nil {
					return err
				}
				c.rmoveto(0, c.stack[0])
				c.stack = c.stack[:0]
			case 5: // lineto
				if len(c.stack) == 0 || len(c.stack)%2 != 0 {
					return errors.New("stack underflow in lineto")
				}
				c.markWidthParsed()
				for j := 0; j < len(c.stack); j += 2 {
					c.rlineto(c.stack[j], c.stack[j+1])
				}
				c.stack = c.stack[:0]
			case 6: // hlineto
				if len(c.stack) == 0 {
					return errors.New("stack underflow in hlineto")
				}
				c.markWidthParsed()
				for j, v := range c.stack {
					if j%2 == 0 {
						c.rlineto(v, 0)
					} else {
						c.rlineto(0, v)
					}
				}
				c.stack = c.stack[:0]
			case 7: // vlineto
				if len(c.stack) == 0 {
					return errors.New("stack underflow in vlineto")
				}
				c.markWidthParsed()
				for j, v := range c.stack {
					if j%2 == 0 {
						c.rlineto(0, v)
					} else {
						c.rlineto(v, 0)
					}
				}
				c.stack = c.stack[:0]
			case 8: // rrcurveto
				if len(c.stack) == 0 || len(c.stack)%6 != 0 {
					return errors.New("stack underflow in rrcurveto")
				}
				c.markWidthParsed()
				for j := 0; j < len(c.stack); j += 6 {
					dx1, dy1 := c.stack[j], c.stack[j+1]
					dx2, dy2 := c.stack[j+2], c.stack[j+3]
					dx3, dy3 := c.stack[j+4], c.stack[j+5]
					c.rrcurveto(dx1, dy1, dx2, dy2, dx3, dy3)
				}
				c.stack = c.stack[:0]
			case 10: // callsubr
				operand, err := c.pop()
				if err != nil {
					return fmt.Errorf("stack underflow in callsubr")
				}
				subrIndex, err := charStringIntOperand(operand, "subroutine index", "callsubr")
				if err != nil {
					return err
				}
				if c.localSubrs == nil {
					return errors.New("callsubr called but no local subrs")
				}
				idx := subrIndex + calculateBias(int(c.localSubrs.Count))
				subrData, err := c.localSubrs.Get(idx)
				if err != nil {
					return fmt.Errorf("failed to get local subr %d: %v", idx, err)
				}
				if err := c.interpret(subrData); err != nil {
					return err
				}
			case 29: // callgsubr
				operand, err := c.pop()
				if err != nil {
					return fmt.Errorf("stack underflow in callgsubr")
				}
				subrIndex, err := charStringIntOperand(operand, "subroutine index", "callgsubr")
				if err != nil {
					return err
				}
				if c.globalSubrs == nil {
					return errors.New("callgsubr called but no global subrs")
				}
				idx := subrIndex + calculateBias(int(c.globalSubrs.Count))
				subrData, err := c.globalSubrs.Get(idx)
				if err != nil {
					return fmt.Errorf("failed to get global subr %d: %v", idx, err)
				}
				if err := c.interpret(subrData); err != nil {
					return err
				}
			case 11: // return
				return nil
			case 14: // endchar
				if err := c.consumeEndcharArgs(); err != nil {
					return err
				}
				if len(c.outline.Points) > 0 {
					c.outline.Contours = append(c.outline.Contours, len(c.outline.Points)-1)
				}
				return nil
			case 15: // vsindex (CFF2)
				if err := c.vsindex(); err != nil {
					return err
				}
			case 18: // hstemhm
				if err := c.consumeStemHints("hstemhm", false); err != nil {
					return err
				}
			case 19, 20: // hintmask, cntrmask
				maskOp := "hintmask"
				if op == 20 {
					maskOp = "cntrmask"
				}
				if err := c.consumeStemHints(maskOp, true); err != nil {
					return err
				}
				maskBytes := (c.hintCount + 7) / 8
				if i+maskBytes > len(data) {
					return fmt.Errorf("unexpected EOF reading %s", maskOp)
				}
				i += maskBytes
			case 23: // vstemhm
				if err := c.consumeStemHints("vstemhm", false); err != nil {
					return err
				}
			case 24: // rcurveline
				if err := c.rcurveline(); err != nil {
					return err
				}
			case 25: // rlinecurve
				if err := c.rlinecurve(); err != nil {
					return err
				}
			case 26: // vvcurveto
				if err := c.vvcurveto(); err != nil {
					return err
				}
			case 27: // hhcurveto
				if err := c.hhcurveto(); err != nil {
					return err
				}
			case 1203: // and
				if err := c.binaryStackOp("and", func(a, b float64) (float64, error) {
					return boolNumber(a != 0 && b != 0), nil
				}); err != nil {
					return err
				}
			case 1204: // or
				if err := c.binaryStackOp("or", func(a, b float64) (float64, error) {
					return boolNumber(a != 0 || b != 0), nil
				}); err != nil {
					return err
				}
			case 1205: // not
				if err := c.unaryStackOp("not", func(v float64) (float64, error) {
					return boolNumber(v == 0), nil
				}); err != nil {
					return err
				}
			case 1209: // abs
				if err := c.unaryStackOp("abs", func(v float64) (float64, error) {
					return math.Abs(v), nil
				}); err != nil {
					return err
				}
			case 1210: // add
				if err := c.binaryStackOp("add", func(a, b float64) (float64, error) {
					return a + b, nil
				}); err != nil {
					return err
				}
			case 1211: // sub
				if err := c.binaryStackOp("sub", func(a, b float64) (float64, error) {
					return a - b, nil
				}); err != nil {
					return err
				}
			case 1212: // div
				if err := c.binaryStackOp("div", func(a, b float64) (float64, error) {
					if b == 0 {
						return 0, errors.New("division by zero in div")
					}
					return a / b, nil
				}); err != nil {
					return err
				}
			case 1214: // neg
				if err := c.unaryStackOp("neg", func(v float64) (float64, error) {
					return -v, nil
				}); err != nil {
					return err
				}
			case 1215: // eq
				if err := c.binaryStackOp("eq", func(a, b float64) (float64, error) {
					return boolNumber(a == b), nil
				}); err != nil {
					return err
				}
			case 1218: // drop
				if err := c.drop(); err != nil {
					return err
				}
			case 1220: // put
				if err := c.put(); err != nil {
					return err
				}
			case 1221: // get
				if err := c.get(); err != nil {
					return err
				}
			case 1222: // ifelse
				if err := c.ifelse(); err != nil {
					return err
				}
			case 1223: // random
				if err := c.push(c.random()); err != nil {
					return err
				}
			case 1224: // mul
				if err := c.binaryStackOp("mul", func(a, b float64) (float64, error) {
					return a * b, nil
				}); err != nil {
					return err
				}
			case 1226: // sqrt
				if err := c.unaryStackOp("sqrt", func(v float64) (float64, error) {
					if v < 0 {
						return 0, errors.New("negative operand in sqrt")
					}
					return math.Sqrt(v), nil
				}); err != nil {
					return err
				}
			case 1227: // dup
				if err := c.dup(); err != nil {
					return err
				}
			case 1228: // exch
				if err := c.exch(); err != nil {
					return err
				}
			case 1229: // index
				if err := c.index(); err != nil {
					return err
				}
			case 1230: // roll
				if err := c.roll(); err != nil {
					return err
				}
			case 16: // blend
				blendVector, err := c.currentBlendVector()
				if err != nil {
					return err
				}
				operand, err := c.pop()
				if err != nil {
					return fmt.Errorf("stack underflow in blend")
				}
				n, err := charStringIntOperand(operand, "operand count", "blend")
				if err != nil {
					return err
				}
				if n < 0 {
					return errors.New("negative operand count in blend")
				}
				k := len(blendVector)
				if len(c.stack) < n+n*k {
					return errors.New("stack underflow in blend")
				}

				baseIdx := len(c.stack) - n - n*k
				deltasIdx := len(c.stack) - n*k

				for i := 0; i < n; i++ {
					val := c.stack[baseIdx+i]
					for j := 0; j < k; j++ {
						val += c.stack[deltasIdx+i*k+j] * blendVector[j]
					}
					c.stack[baseIdx+i] = val
				}
				c.stack = c.stack[:baseIdx+n]
			case 30: // vhcurveto
				if err := c.alternatingCurve(false, "vhcurveto"); err != nil {
					return err
				}
			case 31: // hvcurveto
				if err := c.alternatingCurve(true, "hvcurveto"); err != nil {
					return err
				}
			case 1234: // hflex
				if err := c.hflex(); err != nil {
					return err
				}
			case 1235: // flex
				if err := c.flex(); err != nil {
					return err
				}
			case 1236: // hflex1
				if err := c.hflex1(); err != nil {
					return err
				}
			case 1237: // flex1
				if err := c.flex1(); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported charstring operator %d", op)
			}
		}
	}
	return nil
}

func (c *charStringContext) vsindex() error {
	if len(c.stack) < 1 {
		return errors.New("stack underflow in vsindex")
	}
	idx, err := charStringIntOperand(c.stack[len(c.stack)-1], "variation store index", "vsindex")
	if err != nil {
		return err
	}
	if idx < 0 {
		return errors.New("negative variation store index in vsindex")
	}
	if c.varStore != nil {
		if _, ok := c.varStore.ActiveRegionCount(idx); !ok {
			return fmt.Errorf("variation store index %d out of range in vsindex", idx)
		}
	}
	c.stack = c.stack[:len(c.stack)-1]
	c.vsIndex = idx
	return nil
}

func (c *charStringContext) currentBlendVector() ([]float64, error) {
	if len(c.blendVector) > 0 {
		return c.blendVector, nil
	}
	if c.varStore == nil {
		return nil, errors.New("blend operator but no blend vector")
	}
	if c.blendVectors != nil {
		if vector, ok := c.blendVectors[c.vsIndex]; ok {
			return vector, nil
		}
	}
	if vector, ok := c.varStore.BlendVector(c.vsIndex, c.varCoords); ok {
		if c.blendVectors == nil {
			c.blendVectors = make(map[int][]float64)
		}
		c.blendVectors[c.vsIndex] = vector
		return vector, nil
	}
	k, ok := c.varStore.ActiveRegionCount(c.vsIndex)
	if !ok {
		return nil, fmt.Errorf("variation store index %d out of range in blend", c.vsIndex)
	}
	if cap(c.zeroBlend) < k {
		c.zeroBlend = make([]float64, k)
	}
	return c.zeroBlend[:k], nil
}

func (c *charStringContext) drop() error {
	if len(c.stack) < 1 {
		return errors.New("stack underflow in drop")
	}
	c.stack = c.stack[:len(c.stack)-1]
	return nil
}

func (c *charStringContext) dup() error {
	if len(c.stack) < 1 {
		return errors.New("stack underflow in dup")
	}
	return c.push(c.stack[len(c.stack)-1])
}

func (c *charStringContext) exch() error {
	if len(c.stack) < 2 {
		return errors.New("stack underflow in exch")
	}
	top := len(c.stack) - 1
	c.stack[top], c.stack[top-1] = c.stack[top-1], c.stack[top]
	return nil
}

func (c *charStringContext) index() error {
	if len(c.stack) < 2 {
		return errors.New("stack underflow in index")
	}
	idx, err := charStringIntOperand(c.stack[len(c.stack)-1], "stack index", "index")
	if err != nil {
		return err
	}
	c.stack = c.stack[:len(c.stack)-1]
	if idx < 0 {
		idx = 0
	}
	if idx >= len(c.stack) {
		idx = len(c.stack) - 1
	}
	return c.push(c.stack[len(c.stack)-1-idx])
}

func (c *charStringContext) roll() error {
	if len(c.stack) < 2 {
		return errors.New("stack underflow in roll")
	}
	j, err := charStringIntOperand(c.stack[len(c.stack)-1], "roll shift", "roll")
	if err != nil {
		return err
	}
	n, err := charStringIntOperand(c.stack[len(c.stack)-2], "roll count", "roll")
	if err != nil {
		return err
	}
	c.stack = c.stack[:len(c.stack)-2]
	if n < 0 || n > len(c.stack) {
		return errors.New("invalid operand count in roll")
	}
	if n <= 1 {
		return nil
	}

	shift := j % n
	if shift < 0 {
		shift += n
	}
	if shift == 0 {
		return nil
	}

	start := len(c.stack) - n
	rolled := append([]float64(nil), c.stack[start+n-shift:]...)
	rolled = append(rolled, c.stack[start:start+n-shift]...)
	copy(c.stack[start:], rolled)
	return nil
}

func (c *charStringContext) put() error {
	if len(c.stack) < 2 {
		return errors.New("stack underflow in put")
	}
	idx, err := charStringIntOperand(c.stack[len(c.stack)-1], "transient array index", "put")
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(c.transient) {
		return errors.New("transient array index out of range in put")
	}
	c.transient[idx] = c.stack[len(c.stack)-2]
	c.stack = c.stack[:len(c.stack)-2]
	return nil
}

func (c *charStringContext) get() error {
	if len(c.stack) < 1 {
		return errors.New("stack underflow in get")
	}
	idx, err := charStringIntOperand(c.stack[len(c.stack)-1], "transient array index", "get")
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(c.transient) {
		return errors.New("transient array index out of range in get")
	}
	c.stack[len(c.stack)-1] = c.transient[idx]
	return nil
}

func (c *charStringContext) ifelse() error {
	if len(c.stack) < 4 {
		return errors.New("stack underflow in ifelse")
	}
	base := len(c.stack) - 4
	s1, s2 := c.stack[base], c.stack[base+1]
	v1, v2 := c.stack[base+2], c.stack[base+3]
	if v1 <= v2 {
		c.stack[base] = s1
	} else {
		c.stack[base] = s2
	}
	c.stack = c.stack[:base+1]
	return nil
}

func (c *charStringContext) random() float64 {
	if c.randomSeed == 0 {
		c.randomSeed = 1
	}
	c.randomSeed = c.randomSeed*1103515245 + 12345
	return float64((c.randomSeed>>1)+1) / float64(uint32(1)<<31)
}

func (c *charStringContext) consumeEndcharArgs() error {
	args := len(c.stack)
	if !c.widthParsed {
		switch args {
		case 0:
			c.widthParsed = true
		case 1:
			c.removeFirstOperand()
			c.widthParsed = true
		case 4:
			c.widthParsed = true
		case 5:
			c.removeFirstOperand()
			c.widthParsed = true
		default:
			return errors.New("invalid operand count in endchar")
		}
	} else if args != 0 && args != 4 {
		return errors.New("invalid operand count in endchar")
	}
	c.stack = c.stack[:0]
	return nil
}

func (c *charStringContext) rmoveto(dx, dy float64) {
	if len(c.outline.Points) > 0 {
		c.outline.Contours = append(c.outline.Contours, len(c.outline.Points)-1)
	}
	c.x += dx
	c.y += dy
	to := charStringPoint(c.x, c.y)
	c.outline.Points = append(c.outline.Points, to)
	c.outline.Tags = append(c.outline.Tags, 1) // On-curve
	c.segments = append(c.segments, charStringSegment{
		kind: charStringMoveSegment,
		to:   to,
	})
}

func (c *charStringContext) rlineto(dx, dy float64) {
	from := charStringPoint(c.x, c.y)
	c.x += dx
	c.y += dy
	to := charStringPoint(c.x, c.y)
	c.outline.Points = append(c.outline.Points, to)
	c.outline.Tags = append(c.outline.Tags, 1) // On-curve
	c.segments = append(c.segments, charStringSegment{
		kind: charStringLineSegment,
		from: from,
		to:   to,
	})
}

func (c *charStringContext) rcurveline() error {
	if len(c.stack) < 8 || (len(c.stack)-2)%6 != 0 {
		return errors.New("stack underflow in rcurveline")
	}
	c.markWidthParsed()
	curveEnd := len(c.stack) - 2
	for j := 0; j < curveEnd; j += 6 {
		c.rrcurveto(c.stack[j], c.stack[j+1], c.stack[j+2], c.stack[j+3], c.stack[j+4], c.stack[j+5])
	}
	c.rlineto(c.stack[curveEnd], c.stack[curveEnd+1])
	c.stack = c.stack[:0]
	return nil
}

func (c *charStringContext) rlinecurve() error {
	if len(c.stack) < 8 || (len(c.stack)-6)%2 != 0 {
		return errors.New("stack underflow in rlinecurve")
	}
	c.markWidthParsed()
	curveStart := len(c.stack) - 6
	for j := 0; j < curveStart; j += 2 {
		c.rlineto(c.stack[j], c.stack[j+1])
	}
	c.rrcurveto(
		c.stack[curveStart], c.stack[curveStart+1],
		c.stack[curveStart+2], c.stack[curveStart+3],
		c.stack[curveStart+4], c.stack[curveStart+5],
	)
	c.stack = c.stack[:0]
	return nil
}

func (c *charStringContext) vvcurveto() error {
	if len(c.stack) < 4 {
		return errors.New("stack underflow in vvcurveto")
	}
	c.markWidthParsed()
	j := 0
	dx1 := 0.0
	if len(c.stack)%4 == 1 {
		dx1 = c.stack[0]
		j = 1
	}
	if (len(c.stack)-j)%4 != 0 {
		return errors.New("invalid operand count in vvcurveto")
	}
	for j < len(c.stack) {
		dy1 := c.stack[j]
		dx2, dy2 := c.stack[j+1], c.stack[j+2]
		dy3 := c.stack[j+3]
		c.rrcurveto(dx1, dy1, dx2, dy2, 0, dy3)
		dx1 = 0
		j += 4
	}
	c.stack = c.stack[:0]
	return nil
}

func (c *charStringContext) hhcurveto() error {
	if len(c.stack) < 4 {
		return errors.New("stack underflow in hhcurveto")
	}
	c.markWidthParsed()
	j := 0
	dy1 := 0.0
	if len(c.stack)%4 == 1 {
		dy1 = c.stack[0]
		j = 1
	}
	if (len(c.stack)-j)%4 != 0 {
		return errors.New("invalid operand count in hhcurveto")
	}
	for j < len(c.stack) {
		dx1 := c.stack[j]
		dx2, dy2 := c.stack[j+1], c.stack[j+2]
		dx3 := c.stack[j+3]
		c.rrcurveto(dx1, dy1, dx2, dy2, dx3, 0)
		dy1 = 0
		j += 4
	}
	c.stack = c.stack[:0]
	return nil
}

func (c *charStringContext) alternatingCurve(horizontal bool, op string) error {
	if len(c.stack) < 4 || (len(c.stack)%4 != 0 && len(c.stack)%4 != 1) {
		return fmt.Errorf("stack underflow in %s", op)
	}
	c.markWidthParsed()
	j := 0
	for j < len(c.stack) {
		remaining := len(c.stack) - j
		if remaining < 4 {
			return fmt.Errorf("invalid operand count in %s", op)
		}

		extraFinal := remaining == 5
		if horizontal {
			dx1 := c.stack[j]
			dx2, dy2 := c.stack[j+1], c.stack[j+2]
			dx3, dy3 := 0.0, c.stack[j+3]
			j += 4
			if extraFinal {
				dx3 = c.stack[j]
				j++
			}
			c.rrcurveto(dx1, 0, dx2, dy2, dx3, dy3)
		} else {
			dy1 := c.stack[j]
			dx2, dy2 := c.stack[j+1], c.stack[j+2]
			dx3, dy3 := c.stack[j+3], 0.0
			j += 4
			if extraFinal {
				dy3 = c.stack[j]
				j++
			}
			c.rrcurveto(0, dy1, dx2, dy2, dx3, dy3)
		}
		horizontal = !horizontal
	}
	c.stack = c.stack[:0]
	return nil
}

func (c *charStringContext) hflex() error {
	if len(c.stack) != 7 {
		return errors.New("invalid operand count in hflex")
	}
	c.markWidthParsed()
	c.rrcurveto(c.stack[0], 0, c.stack[1], c.stack[2], c.stack[3], 0)
	c.rrcurveto(c.stack[4], 0, c.stack[5], -c.stack[2], c.stack[6], 0)
	c.stack = c.stack[:0]
	return nil
}

func (c *charStringContext) flex() error {
	if len(c.stack) != 13 {
		return errors.New("invalid operand count in flex")
	}
	c.markWidthParsed()
	c.rrcurveto(c.stack[0], c.stack[1], c.stack[2], c.stack[3], c.stack[4], c.stack[5])
	c.rrcurveto(c.stack[6], c.stack[7], c.stack[8], c.stack[9], c.stack[10], c.stack[11])
	c.stack = c.stack[:0]
	return nil
}

func (c *charStringContext) hflex1() error {
	if len(c.stack) != 9 {
		return errors.New("invalid operand count in hflex1")
	}
	c.markWidthParsed()
	dy6 := -(c.stack[1] + c.stack[3] + c.stack[7])
	c.rrcurveto(c.stack[0], c.stack[1], c.stack[2], c.stack[3], c.stack[4], 0)
	c.rrcurveto(c.stack[5], 0, c.stack[6], c.stack[7], c.stack[8], dy6)
	c.stack = c.stack[:0]
	return nil
}

func (c *charStringContext) flex1() error {
	if len(c.stack) != 11 {
		return errors.New("invalid operand count in flex1")
	}
	c.markWidthParsed()
	dx := c.stack[0] + c.stack[2] + c.stack[4] + c.stack[6] + c.stack[8]
	dy := c.stack[1] + c.stack[3] + c.stack[5] + c.stack[7] + c.stack[9]
	dx6, dy6 := -dx, c.stack[10]
	if absFloat64(dx) > absFloat64(dy) {
		dx6, dy6 = c.stack[10], -dy
	}
	c.rrcurveto(c.stack[0], c.stack[1], c.stack[2], c.stack[3], c.stack[4], c.stack[5])
	c.rrcurveto(c.stack[6], c.stack[7], c.stack[8], c.stack[9], dx6, dy6)
	c.stack = c.stack[:0]
	return nil
}

func absFloat64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func (c *charStringContext) rrcurveto(dx1, dy1, dx2, dy2, dx3, dy3 float64) {
	x0, y0 := c.x, c.y
	x1, y1 := x0+dx1, y0+dy1
	x2, y2 := x1+dx2, y1+dy2
	x3, y3 := x2+dx3, y2+dy3
	c.segments = append(c.segments, charStringSegment{
		kind:  charStringCubicSegment,
		from:  charStringPoint(x0, y0),
		ctrl1: charStringPoint(x1, y1),
		ctrl2: charStringPoint(x2, y2),
		to:    charStringPoint(x3, y3),
	})

	c.outline.Points = append(c.outline.Points,
		charStringPoint(x1, y1),
		charStringPoint(x2, y2),
		charStringPoint(x3, y3),
	)
	c.outline.Tags = append(c.outline.Tags, 2, 2, 1)
	c.x, c.y = x3, y3
}

func charStringPoint(x, y float64) api.Vector {
	return api.Vector{X: int32(x * 64), Y: int32(y * 64)}
}
