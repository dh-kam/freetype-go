package cff

import (
	"errors"
	"fmt"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

type charStringContext struct {
	stack       []float64
	x, y        float64
	outline     *core.Outline
	globalSubrs *Index
	localSubrs  *Index
	blendVector []float64
	nesting     int
	widthParsed bool
	vsIndex     int
}

func (c *charStringContext) push(v float64) {
	c.stack = append(c.stack, v)
}

func (c *charStringContext) pop() (float64, error) {
	if len(c.stack) == 0 {
		return 0, errors.New("charstring stack underflow")
	}
	v := c.stack[len(c.stack)-1]
	c.stack = c.stack[:len(c.stack)-1]
	return v, nil
}

func (c *charStringContext) requireStack(n int, op string) error {
	if len(c.stack) < n {
		return fmt.Errorf("stack underflow in %s", op)
	}
	return nil
}

func (c *charStringContext) clearStack(expected int) {
	if !c.widthParsed && len(c.stack) > expected {
		// First operand is width
		c.widthParsed = true
	}
	c.stack = c.stack[:0]
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
	ctx := &charStringContext{
		outline:     &core.Outline{},
		globalSubrs: globalSubrs,
		localSubrs:  localSubrs,
		blendVector: blendVector,
	}
	err := ctx.interpret(data)
	if err != nil {
		return nil, err
	}
	return ctx.outline, nil
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
			c.push(v)
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
			case 21: // rmoveto
				if err := c.requireStack(2, "rmoveto"); err != nil {
					return err
				}
				dy, _ := c.pop()
				dx, _ := c.pop()
				c.rmoveto(dx, dy)
				c.clearStack(0) // operands already popped
			case 22: // hmoveto
				if err := c.requireStack(1, "hmoveto"); err != nil {
					return err
				}
				dx, _ := c.pop()
				c.rmoveto(dx, 0)
				c.clearStack(0)
			case 4: // vmoveto
				if err := c.requireStack(1, "vmoveto"); err != nil {
					return err
				}
				dy, _ := c.pop()
				c.rmoveto(0, dy)
				c.clearStack(0)
			case 5: // lineto
				if len(c.stack) == 0 || len(c.stack)%2 != 0 {
					return errors.New("stack underflow in lineto")
				}
				for j := 0; j < len(c.stack); j += 2 {
					c.rlineto(c.stack[j], c.stack[j+1])
				}
				c.stack = c.stack[:0] // Not a width-clearing op
			case 6: // hlineto
				if len(c.stack) == 0 {
					return errors.New("stack underflow in hlineto")
				}
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
				if c.localSubrs == nil {
					return errors.New("callsubr called but no local subrs")
				}
				idx := int(operand) + calculateBias(int(c.localSubrs.Count))
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
				if c.globalSubrs == nil {
					return errors.New("callgsubr called but no global subrs")
				}
				idx := int(operand) + calculateBias(int(c.globalSubrs.Count))
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
				if !c.widthParsed && len(c.stack) > 0 {
					c.widthParsed = true
				}
				if len(c.outline.Points) > 0 {
					c.outline.Contours = append(c.outline.Contours, len(c.outline.Points)-1)
				}
				return nil
			case 16: // blend
				if len(c.blendVector) == 0 {
					return errors.New("blend operator but no blend vector")
				}
				operand, err := c.pop()
				if err != nil {
					return fmt.Errorf("stack underflow in blend")
				}
				n := int(operand)
				if n < 0 {
					return errors.New("negative operand count in blend")
				}
				k := len(c.blendVector)
				if len(c.stack) < n+n*k {
					return errors.New("stack underflow in blend")
				}

				baseIdx := len(c.stack) - n - n*k
				deltasIdx := len(c.stack) - n*k

				for i := 0; i < n; i++ {
					val := c.stack[baseIdx+i]
					for j := 0; j < k; j++ {
						val += c.stack[deltasIdx+i*k+j] * c.blendVector[j]
					}
					c.stack[baseIdx+i] = val
				}
				c.stack = c.stack[:baseIdx+n]
			default:
				// Ignore unknown operators for now
				c.stack = c.stack[:0]
			}
		}
	}
	return nil
}

func (c *charStringContext) rmoveto(dx, dy float64) {
	if len(c.outline.Points) > 0 {
		c.outline.Contours = append(c.outline.Contours, len(c.outline.Points)-1)
	}
	c.x += dx
	c.y += dy
	c.outline.Points = append(c.outline.Points, api.Vector{X: int32(c.x * 64), Y: int32(c.y * 64)})
	c.outline.Tags = append(c.outline.Tags, 1) // On-curve
}

func (c *charStringContext) rlineto(dx, dy float64) {
	c.x += dx
	c.y += dy
	c.outline.Points = append(c.outline.Points, api.Vector{X: int32(c.x * 64), Y: int32(c.y * 64)})
	c.outline.Tags = append(c.outline.Tags, 1) // On-curve
}

func (c *charStringContext) rrcurveto(dx1, dy1, dx2, dy2, dx3, dy3 float64) {
	x0, y0 := c.x, c.y
	x1, y1 := x0+dx1, y0+dy1
	x2, y2 := x1+dx2, y1+dy2
	x3, y3 := x2+dx3, y2+dy3

	// Flatten cubic Bezier to line segments
	const steps = 10
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		t2 := t * t
		t3 := t2 * t
		mt := 1.0 - t
		mt2 := mt * mt
		mt3 := mt2 * mt

		px := mt3*x0 + 3*mt2*t*x1 + 3*mt*t2*x2 + t3*x3
		py := mt3*y0 + 3*mt2*t*y1 + 3*mt*t2*y2 + t3*y3

		c.outline.Points = append(c.outline.Points, api.Vector{X: int32(px * 64), Y: int32(py * 64)})
		c.outline.Tags = append(c.outline.Tags, 1)
	}
	c.x, c.y = x3, y3
}
