package math

import (
	"math"

	"github.com/dh-kam/freetype-go/api"
)

// MulFix computes (a * b) / 0x10000 with rounding.
func MulFix(a, b int32) int32 {
	ret := int64(a) * int64(b)
	ret += 0x8000 + (ret >> 63)
	return int32(ret >> 16)
}

// DivFix computes (a * 0x10000) / b with rounding.
func DivFix(a, b int32) int32 {
	if b == 0 {
		if a >= 0 {
			return 0x7FFFFFFF
		}
		return -0x7FFFFFFF
	}

	var s int32 = 1
	if a < 0 {
		a = -a
		s = -s
	}
	if b < 0 {
		b = -b
		s = -s
	}

	q := (int64(a) << 16)
	r := q % int64(b)
	q = q / int64(b)

	if 2*r >= int64(b) {
		q++
	}

	res := int32(q)
	if s < 0 {
		res = -res
	}
	return res
}

// Cos returns the cosine of an angle (in 16.16 fixed-point radians) as 16.16 fixed-point.
func Cos(angle int32) int32 {
	rad := float64(angle) / 65536.0
	return int32(math.Round(math.Cos(rad) * 65536.0))
}

// Sin returns the sine of an angle (in 16.16 fixed-point radians) as 16.16 fixed-point.
func Sin(angle int32) int32 {
	rad := float64(angle) / 65536.0
	return int32(math.Round(math.Sin(rad) * 65536.0))
}

// Mul26 computes (a * b + 32) / 64 for 26.6 math.
func Mul26(a, b int32) int32 {
	return int32((int64(a)*int64(b) + 32) >> 6)
}

// Div26 computes (a * 64) / b for 26.6 math.
func Div26(a, b int32) int32 {
	if b == 0 {
		return 0
	}
	return int32((int64(a) << 6) / int64(b))
}

type mathEngine struct{}

// NewMathEngine returns a new instance of the MathEngine implementation.
func NewMathEngine() api.MathEngine {
	return &mathEngine{}
}

// MulFix computes (a * b) / 0x10000 with rounding.
func (e *mathEngine) MulFix(a, b int32) int32 {
	return MulFix(a, b)
}

// DivFix computes (a * 0x10000) / b with rounding.
func (e *mathEngine) DivFix(a, b int32) int32 {
	return DivFix(a, b)
}

// Cos returns the cosine of an angle (in 16.16 fixed-point radians) as 16.16 fixed-point.
func (e *mathEngine) Cos(angle int32) int32 {
	return Cos(angle)
}

// Sin returns the sine of an angle (in 16.16 fixed-point radians) as 16.16 fixed-point.
func (e *mathEngine) Sin(angle int32) int32 {
	return Sin(angle)
}
