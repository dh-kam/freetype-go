//go:build cgo && freetype_harness

package harness

/*
#cgo pkg-config: freetype2
#include <ft2build.h>
#include FT_FREETYPE_H
*/
import "C"
import (
	"github.com/dh-kam/freetype-go/math"
)

// CompareMulFix compares the Go implementation of MulFix with the C implementation.
func CompareMulFix(a, b int32) (goRes, cRes int32, equal bool) {
	engine := math.NewMathEngine()
	goRes = engine.MulFix(a, b)
	cRes = int32(C.FT_MulFix(C.long(a), C.long(b)))
	equal = (goRes == cRes)
	return
}

// CompareDivFix compares the Go implementation of DivFix with the C implementation.
func CompareDivFix(a, b int32) (goRes, cRes int32, equal bool) {
	engine := math.NewMathEngine()
	goRes = engine.DivFix(a, b)
	cRes = int32(C.FT_DivFix(C.long(a), C.long(b)))
	equal = (goRes == cRes)
	return
}
