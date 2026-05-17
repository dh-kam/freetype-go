package truetype

import (
	"testing"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

func TestPointManipulation(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	// Setup Zone 1 with one point at (100, 100)
	ctx.Zones[1].Points = []api.Vector{{X: 100, Y: 100}}
	ctx.Zones[1].TouchedX = make([]bool, 1)
	ctx.Zones[1].TouchedY = make([]bool, 1)

	// SVTCA[x] (0x01)
	// PUSHB 0 (point index)
	// MDAP[1] (0x2F) - Move Direct Absolute Point with rounding
	ctx.Code = []byte{0x01, 0xB0, 0x00, 0x2F}

	err := ctx.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// 100 rounded to grid (64) is 128
	// (100 + 32) & ^63 = 132 & ^63 = 128
	expectedX := int32(128)
	if ctx.Zones[1].Points[0].X != expectedX {
		t.Fatalf("Expected X %d, got %d", expectedX, ctx.Zones[1].Points[0].X)
	}

	// Y should remain 100 because ProjVector and FreeVector were set to X-axis
	if ctx.Zones[1].Points[0].Y != 100 {
		t.Fatalf("Expected Y 100, got %d", ctx.Zones[1].Points[0].Y)
	}

	if !ctx.Zones[1].TouchedX[0] {
		t.Fatalf("Point should be touched on X")
	}
}

func TestMDRP(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	// Setup Zone 1 (Glyph zone)
	// Point 0 at (0, 0), Point 1 at (100, 0)
	ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 2)
	ctx.Zones[1].TouchedY = make([]bool, 2)

	ctx.ZP0 = 1
	ctx.ZP1 = 1
	ctx.RP0 = 0

	// SVTCA[x] (0x01)
	// PUSHB 1 (point index)
	// MDRP[round, gray] (0xC0 | 0x04 | 0x00 = 0xC4)
	ctx.Code = []byte{0x01, 0xB0, 0x01, 0xC4}

	err := ctx.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// 100 rounded to grid is 128
	expectedX := int32(128)
	if ctx.Zones[1].Points[1].X != expectedX {
		t.Fatalf("Expected X %d, got %d", expectedX, ctx.Zones[1].Points[1].X)
	}
	if !ctx.Zones[1].TouchedX[1] {
		t.Fatalf("Point 1 should be touched on X")
	}
}

func TestIUP(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	// Setup Zone 1
	// 3 points: 0: (0,0), 1: (50, 0), 2: (100, 0)
	// All in one contour
	ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 50, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 50, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 3)
	ctx.Zones[1].TouchedY = make([]bool, 3)
	ctx.Zones[1].Contours = []int{2}

	ctx.ZP2 = 1

	// Move point 0 to (0,0) [no change]
	// Move point 2 to (128, 0) [rounded 100 to 128]
	// Points 0 and 2 are touched. Point 1 is untouched.
	ctx.Zones[1].Points[2].X = 128
	ctx.Zones[1].TouchedX[0] = true
	ctx.Zones[1].TouchedX[2] = true

	// IUP[x] (0x31)
	ctx.Code = []byte{0x31}

	err := ctx.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Point 1 was at 50 (midway between 0 and 100).
	// Now it should be midway between 0 and 128, which is 64.
	expectedX := int32(64)
	if ctx.Zones[1].Points[1].X != expectedX {
		t.Fatalf("Expected X %d, got %d", expectedX, ctx.Zones[1].Points[1].X)
	}
}

func TestResetDoesNotClearAssignedMaps(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	functions := map[int32][]byte{
		1: {0x2D},
	}
	instructions := map[int32][]byte{
		2: {0xB0, 0x01},
	}
	ctx.Functions = functions
	ctx.Instructions = instructions

	ctx.Reset(sys)

	if got := functions[1]; len(got) != 1 || got[0] != 0x2D {
		t.Fatalf("Reset cleared caller-owned Functions map: %#v", functions)
	}
	if got := instructions[2]; len(got) != 2 || got[0] != 0xB0 || got[1] != 0x01 {
		t.Fatalf("Reset cleared caller-owned Instructions map: %#v", instructions)
	}
	if len(ctx.Functions) != 0 {
		t.Fatalf("Expected reset Functions map to be empty, got %#v", ctx.Functions)
	}
	if len(ctx.Instructions) != 0 {
		t.Fatalf("Expected reset Instructions map to be empty, got %#v", ctx.Instructions)
	}
}

func TestInterpreterBasic(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	// PUSHB[1] 10 (0xB0 0x0A)
	// PUSHB[1] 20 (0xB0 0x14)
	// ADD (0x60)
	ctx.Code = []byte{0xB0, 0x0A, 0xB0, 0x14, 0x60}

	err := ctx.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if ctx.StackTop != 1 {
		t.Fatalf("Expected stack top 1, got %d", ctx.StackTop)
	}

	val, err := ctx.pop()
	if err != nil {
		t.Fatalf("Pop failed: %v", err)
	}

	if val != 30 {
		t.Fatalf("Expected 30, got %d", val)
	}
}

func TestMathOps(t *testing.T) {
	sys := core.NewSystem()

	tests := []struct {
		name     string
		code     []byte
		expected int32
	}{
		{"SUB", []byte{0xB0, 0x1E, 0xB0, 0x0A, 0x61}, 20},        // 30 - 10 = 20
		{"MUL", []byte{0xB0, 0x40, 0xB0, 0x40, 0x62}, 64},        // (64 * 64) / 64 = 64
		{"DIV", []byte{0xB0, 0x80, 0xB0, 0x40, 0x63}, 128},       // (128 * 64) / 64 = 128
		{"SWAP", []byte{0xB0, 0x0A, 0xB0, 0x14, 0x23, 0x61}, 10}, // 10, 20 -> 20, 10 -> 20 - 10 = 10
		{"PUSHW", []byte{0xB8, 0xFF, 0xF6}, -10},                 // PUSHW[1] -10 (0xFFF6)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(sys)
			ctx.Code = tt.code
			err := ctx.Run()
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			val, _ := ctx.pop()
			if val != tt.expected {
				t.Fatalf("Expected %d, got %d", tt.expected, val)
			}
		})
	}
}

func TestMemoryOps(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	// PUSHB 1 (index), PUSHB 42 (value), WS
	// PUSHB 1 (index), RS
	ctx.Code = []byte{0xB0, 0x01, 0xB0, 0x2A, 0x42, 0xB0, 0x01, 0x43}

	err := ctx.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	val, err := ctx.pop()
	if err != nil {
		t.Fatalf("Pop failed: %v", err)
	}

	if val != 42 {
		t.Fatalf("Expected 42, got %d", val)
	}
}

func TestControlFlow(t *testing.T) {
	sys := core.NewSystem()

	// IF is 0x58, ELSE is 0x1B, EIF is 0x59
	tests := []struct {
		name     string
		code     []byte
		expected int32
	}{
		{"IF_TRUE", []byte{0xB0, 0x01, 0x58, 0xB0, 0x0A, 0x1B, 0xB0, 0x14, 0x59}, 10},
		{"IF_FALSE", []byte{0xB0, 0x00, 0x58, 0xB0, 0x0A, 0x1B, 0xB0, 0x14, 0x59}, 20},
		{"NESTED_IF_TRUE", []byte{0xB0, 0x01, 0x58, 0xB0, 0x01, 0x58, 0xB0, 0x05, 0x59, 0x59}, 5},
		{"JMPR", []byte{0xB0, 0x03, 0x1C, 0xB0, 0x0A, 0xB0, 0x14}, 20}, // PUSH 3, JMPR (skips PUSH 10)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(sys)
			ctx.Code = tt.code
			err := ctx.Run()
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			val, _ := ctx.pop()
			if val != tt.expected {
				t.Fatalf("Expected %d, got %d", tt.expected, val)
			}
		})
	}
}

func TestSubroutines(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	// Define function 1: add 5 to the top of stack
	// PUSHB 1 (funcID)
	// FDEF (0x2C)
	//   PUSHB 5
	//   ADD
	// ENDF (0x2D)
	// PUSHB 10
	// PUSHB 1 (funcID)
	// CALL (0x2B)
	ctx.Code = []byte{
		0xB0, 0x01, 0x2C, // FDEF 1
		0xB0, 0x05, 0x60, // body: PUSH 5, ADD
		0x2D,       // ENDF
		0xB0, 0x0A, // PUSH 10
		0xB0, 0x01, 0x2B, // CALL 1
	}

	err := ctx.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	val, _ := ctx.pop()
	if val != 15 {
		t.Fatalf("Expected 15, got %d", val)
	}
}

func TestLoopCall(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	// Define function 1: add 1 to the top of stack
	// FDEF 1
	//   PUSHB 1
	//   ADD
	// ENDF
	// PUSHB 10 (initial value)
	// PUSHB 5 (count)
	// PUSHB 1 (funcID)
	// LOOPCALL
	ctx.Code = []byte{
		0xB0, 0x01, 0x2C,
		0xB0, 0x01, 0x60,
		0x2D,
		0xB0, 0x0A,
		0xB0, 0x05, 0xB0, 0x01, 0x2A,
	}

	err := ctx.Run()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	val, _ := ctx.pop()
	if val != 15 {
		t.Fatalf("Expected 15, got %d", val)
	}
}

func TestRelationalAndLogicalOps(t *testing.T) {
	sys := core.NewSystem()

	tests := []struct {
		name     string
		code     []byte
		expected int32
	}{
		{"LT_TRUE", []byte{0xB0, 0x0A, 0xB0, 0x14, 0x50}, 1},  // 10 < 20
		{"LT_FALSE", []byte{0xB0, 0x14, 0xB0, 0x0A, 0x50}, 0}, // 20 < 10
		{"GT_TRUE", []byte{0xB0, 0x14, 0xB0, 0x0A, 0x51}, 1},  // 20 > 10
		{"EQ_TRUE", []byte{0xB0, 0x0A, 0xB0, 0x0A, 0x52}, 1},  // 10 == 10
		{"AND_TRUE", []byte{0xB0, 0x01, 0xB0, 0x01, 0x5A}, 1},
		{"AND_FALSE", []byte{0xB0, 0x01, 0xB0, 0x00, 0x5A}, 0},
		{"OR_TRUE", []byte{0xB0, 0x01, 0xB0, 0x00, 0x5B}, 1},
		{"NOT_TRUE", []byte{0xB0, 0x00, 0x5C}, 1},
		{"NOT_FALSE", []byte{0xB0, 0x01, 0x5C}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(sys)
			ctx.Code = tt.code
			err := ctx.Run()
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			val, _ := ctx.pop()
			if val != tt.expected {
				t.Fatalf("Expected %d, got %d", tt.expected, val)
			}
		})
	}
}
