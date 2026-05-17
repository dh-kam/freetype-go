package truetype

import (
	"errors"
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
	if ctx.RP0 != 0 || ctx.RP1 != 0 {
		t.Fatalf("MDAP should set RP0/RP1 to point 0, got RP0=%d RP1=%d", ctx.RP0, ctx.RP1)
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
	if ctx.RP0 != 0 || ctx.RP1 != 0 || ctx.RP2 != 1 {
		t.Fatalf("MDRP should set RP1=old RP0 and RP2=p without changing RP0, got RP0=%d RP1=%d RP2=%d", ctx.RP0, ctx.RP1, ctx.RP2)
	}
}

func TestMDRPSetRP0(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 2)
	ctx.Zones[1].TouchedY = make([]bool, 2)
	ctx.ZP0 = 1
	ctx.ZP1 = 1
	ctx.RP0 = 0

	ctx.Code = []byte{0x01, 0xB0, 0x01, 0xD4} // MDRP[set-rp0, round]
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if ctx.RP0 != 1 || ctx.RP1 != 0 || ctx.RP2 != 1 {
		t.Fatalf("MDRP should set RP0=p, RP1=old RP0, RP2=p; got RP0=%d RP1=%d RP2=%d", ctx.RP0, ctx.RP1, ctx.RP2)
	}
}

func TestMIAPSetsReferencePoints(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.CVT = []int32{128}
	ctx.Zones[1].Points = []api.Vector{{X: 100, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 100, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 1)
	ctx.Zones[1].TouchedY = make([]bool, 1)
	ctx.ZP0 = 1

	ctx.Code = []byte{0x01, 0xB0, 0x00, 0xB0, 0x00, 0x3E} // p=0, cvt=0, MIAP[0]
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if ctx.Zones[1].Points[0].X != 128 {
		t.Fatalf("expected point moved to CVT value 128, got %d", ctx.Zones[1].Points[0].X)
	}
	if ctx.RP0 != 0 || ctx.RP1 != 0 {
		t.Fatalf("MIAP should set RP0/RP1 to point 0, got RP0=%d RP1=%d", ctx.RP0, ctx.RP1)
	}
}

func TestMIAPUsesOriginalPositionWhenCVTExceedsCutIn(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.CVT = []int32{1000}
	ctx.Zones[1].Points = []api.Vector{{X: 100, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 100, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 1)
	ctx.Zones[1].TouchedY = make([]bool, 1)
	ctx.ZP0 = 1

	ctx.Code = []byte{0x01, 0xB0, 0x00, 0xB0, 0x00, 0x3F} // p=0, cvt=0, MIAP[1]
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if ctx.Zones[1].Points[0].X != 128 {
		t.Fatalf("expected original position rounded to 128 after cut-in miss, got %d", ctx.Zones[1].Points[0].X)
	}
}

func TestGCUsesCurrentAndOriginalPointCoordinates(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.Zones[1].Points = []api.Vector{{X: 140, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 100, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 1)
	ctx.Zones[1].TouchedY = make([]bool, 1)
	ctx.ZP2 = 1

	ctx.Code = []byte{
		0x01,       // SVTCA[x]
		0xB0, 0x00, // point 0
		0x46,       // GC[0], current
		0xB0, 0x00, // point 0
		0x47, // GC[1], original
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if ctx.StackTop != 2 {
		t.Fatalf("StackTop = %d, want 2", ctx.StackTop)
	}
	if got := ctx.Stack[0]; got != 140 {
		t.Fatalf("GC[0] = %d, want 140", got)
	}
	if got := ctx.Stack[1]; got != 100 {
		t.Fatalf("GC[1] = %d, want 100", got)
	}
}

func TestSCFSMovesPointToProjectedCoordinate(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.Zones[1].Points = []api.Vector{{X: 40, Y: 20}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 40, Y: 20}}
	ctx.Zones[1].TouchedX = make([]bool, 1)
	ctx.Zones[1].TouchedY = make([]bool, 1)
	ctx.ZP2 = 1

	ctx.Code = []byte{
		0x01,       // SVTCA[x]
		0xB0, 0x00, // point 0
		0xB0, 0x80, // target x = 128
		0x48, // SCFS
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got := ctx.Zones[1].Points[0].X; got != 128 {
		t.Fatalf("SCFS moved X to %d, want 128", got)
	}
	if got := ctx.Zones[1].Points[0].Y; got != 20 {
		t.Fatalf("SCFS changed Y to %d, want 20", got)
	}
	if !ctx.Zones[1].TouchedX[0] || ctx.Zones[1].TouchedY[0] {
		t.Fatalf("SCFS touch flags = X:%v Y:%v, want X only", ctx.Zones[1].TouchedX[0], ctx.Zones[1].TouchedY[0])
	}
}

func TestMIRPUsesOriginalDistanceWhenCVTExceedsCutIn(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.CVT = []int32{1000}
	ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 2)
	ctx.Zones[1].TouchedY = make([]bool, 2)
	ctx.ZP0 = 1
	ctx.ZP1 = 1
	ctx.RP0 = 0

	ctx.Code = []byte{0x01, 0xB0, 0x01, 0xB0, 0x00, 0xE4} // p=1, cvt=0, MIRP[round/cut-in]
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if ctx.Zones[1].Points[1].X != 128 {
		t.Fatalf("expected original distance rounded to 128 after cut-in miss, got %d", ctx.Zones[1].Points[1].X)
	}
}

func TestMSIRPMovesPointAndUpdatesReferencePoints(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 50, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 50, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 2)
	ctx.Zones[1].TouchedY = make([]bool, 2)
	ctx.ZP0 = 1
	ctx.ZP1 = 1
	ctx.RP0 = 0

	ctx.Code = []byte{
		0x01,       // SVTCA[x]
		0xB0, 0x01, // point 1
		0xB0, 0x80, // distance 128
		0x3B, // MSIRP[1]
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got := ctx.Zones[1].Points[1].X; got != 128 {
		t.Fatalf("MSIRP moved X to %d, want 128", got)
	}
	if ctx.RP0 != 1 || ctx.RP1 != 0 || ctx.RP2 != 1 {
		t.Fatalf("MSIRP refs got RP0=%d RP1=%d RP2=%d, want 1,0,1", ctx.RP0, ctx.RP1, ctx.RP2)
	}
	if !ctx.Zones[1].TouchedX[1] {
		t.Fatalf("MSIRP should touch point 1 on X")
	}
}

func TestSHPShiftsByReferencePointDelta(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.Zones[1].Points = []api.Vector{{X: 128, Y: 0}, {X: 50, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 100, Y: 0}, {X: 50, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 2)
	ctx.Zones[1].TouchedY = make([]bool, 2)
	ctx.ZP1 = 1
	ctx.ZP2 = 1
	ctx.RP2 = 0

	ctx.Code = []byte{
		0x01,       // SVTCA[x]
		0xB0, 0x01, // point 1
		0x32, // SHP[0], reference rp2 in zp1
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got := ctx.Zones[1].Points[1].X; got != 78 {
		t.Fatalf("SHP moved X to %d, want 78", got)
	}
	if !ctx.Zones[1].TouchedX[1] {
		t.Fatalf("SHP should touch point 1 on X")
	}
	if ctx.GS.Loop != 1 {
		t.Fatalf("SHP should reset loop to 1, got %d", ctx.GS.Loop)
	}
}

func TestStackDepthDebugAndStateOpcodes(t *testing.T) {
	sys := core.NewSystem()

	t.Run("DEPTH", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.Code = []byte{
			0xB1, 0x0A, 0x14, // two stack values
			0x24, // DEPTH
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if ctx.StackTop != 3 || ctx.Stack[2] != 2 {
			t.Fatalf("DEPTH stack = %v top=%d, want depth 2 on top", ctx.Stack[:ctx.StackTop], ctx.StackTop)
		}
	})

	t.Run("DEBUG", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.Code = []byte{0xB0, 0x63, 0x4F}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if ctx.StackTop != 0 {
			t.Fatalf("DEBUG should consume its argument, stack top=%d", ctx.StackTop)
		}
	})

	t.Run("rounding controls", func(t *testing.T) {
		tests := []struct {
			name string
			code []byte
			want int32
		}{
			{"ROFF", []byte{0x7A, 0xB0, 0x41, 0x68}, 65},
			{"RUTG", []byte{0x7C, 0xB0, 0x41, 0x68}, 128},
			{"RDTG", []byte{0x7D, 0xB0, 0x7F, 0x68}, 64},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ctx := NewContext(sys)
				ctx.Code = tt.code
				if err := ctx.Run(); err != nil {
					t.Fatalf("Run failed: %v", err)
				}
				if got := ctx.Stack[ctx.StackTop-1]; got != tt.want {
					t.Fatalf("rounded value = %d, want %d", got, tt.want)
				}
			})
		}
	})
}

func TestSHCAndSHZShiftByReferencePointDelta(t *testing.T) {
	sys := core.NewSystem()

	t.Run("SHC", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 50, Y: 0}, {X: 132, Y: 0}}
		ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 50, Y: 0}, {X: 100, Y: 0}}
		ctx.Zones[1].TouchedX = make([]bool, 3)
		ctx.Zones[1].TouchedY = make([]bool, 3)
		ctx.Zones[1].Contours = []int{1, 2}
		ctx.ZP1 = 1
		ctx.ZP2 = 1
		ctx.RP2 = 2

		ctx.Code = []byte{
			0x01,       // SVTCA[x]
			0xB0, 0x00, // contour 0
			0x34, // SHC[0]
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if ctx.Zones[1].Points[0].X != 32 || ctx.Zones[1].Points[1].X != 82 || ctx.Zones[1].Points[2].X != 132 {
			t.Fatalf("SHC points = %#v, want x shifts on contour 0 only", ctx.Zones[1].Points)
		}
		if !ctx.Zones[1].TouchedX[0] || !ctx.Zones[1].TouchedX[1] || ctx.Zones[1].TouchedX[2] {
			t.Fatalf("SHC touched X = %#v, want contour points touched only", ctx.Zones[1].TouchedX)
		}
	})

	t.Run("SHZ", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 50, Y: 0}, {X: 132, Y: 0}, {X: 500, Y: 0}}
		ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 50, Y: 0}, {X: 100, Y: 0}, {X: 500, Y: 0}}
		ctx.Zones[1].TouchedX = make([]bool, 4)
		ctx.Zones[1].TouchedY = make([]bool, 4)
		ctx.Zones[1].Contours = []int{2}
		ctx.ZP1 = 1
		ctx.RP2 = 2

		ctx.Code = []byte{
			0x01,       // SVTCA[x]
			0xB0, 0x01, // zone 1
			0x36, // SHZ[0]
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if got := ctx.Zones[1].Points; got[0].X != 32 || got[1].X != 82 || got[2].X != 132 || got[3].X != 500 {
			t.Fatalf("SHZ points = %#v, want real non-reference points shifted", got)
		}
		if ctx.Zones[1].TouchedX[0] || ctx.Zones[1].TouchedX[1] || ctx.Zones[1].TouchedX[2] {
			t.Fatalf("SHZ should not touch shifted points, touched X=%#v", ctx.Zones[1].TouchedX)
		}
	})
}

func TestDeltaInstructions(t *testing.T) {
	sys := core.NewSystem()

	pointOps := []struct {
		name   string
		opcode byte
		ppem   int
	}{
		{"DELTAP1", 0x5D, 9},
		{"DELTAP2", 0x71, 25},
		{"DELTAP3", 0x72, 41},
	}
	for _, tt := range pointOps {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(sys)
			ctx.PPEM = tt.ppem
			ctx.Zones[1].Points = []api.Vector{{X: 10, Y: 20}}
			ctx.Zones[1].OriginalPoints = []api.Vector{{X: 10, Y: 20}}
			ctx.Zones[1].TouchedX = make([]bool, 1)
			ctx.Zones[1].TouchedY = make([]bool, 1)
			ctx.ZP0 = 1
			ctx.Code = []byte{
				0x01,                   // SVTCA[x]
				0xB2, 0x08, 0x00, 0x01, // arg, point, count
				tt.opcode,
			}
			if err := ctx.Run(); err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			if got := ctx.Zones[1].Points[0].X; got != 18 {
				t.Fatalf("%s moved X to %d, want 18", tt.name, got)
			}
			if !ctx.Zones[1].TouchedX[0] {
				t.Fatalf("%s should touch moved point on X", tt.name)
			}
		})
	}

	cvtOps := []struct {
		name   string
		opcode byte
		ppem   int
	}{
		{"DELTAC1", 0x73, 9},
		{"DELTAC2", 0x74, 25},
		{"DELTAC3", 0x75, 41},
	}
	for _, tt := range cvtOps {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(sys)
			ctx.PPEM = tt.ppem
			ctx.CVT = []int32{100}
			ctx.Code = []byte{
				0xB2, 0x08, 0x00, 0x01, // arg, CVT index, count
				tt.opcode,
			}
			if err := ctx.Run(); err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			if got := ctx.CVT[0]; got != 108 {
				t.Fatalf("%s CVT[0] = %d, want 108", tt.name, got)
			}
		})
	}

	t.Run("SDB_SDS", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.PPEM = 11
		ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}}
		ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}}
		ctx.Zones[1].TouchedX = make([]bool, 1)
		ctx.Zones[1].TouchedY = make([]bool, 1)
		ctx.ZP0 = 1
		ctx.Code = []byte{
			0xB0, 0x0B, 0x5E, // delta base = 11
			0xB0, 0x02, 0x5F, // delta shift = 2
			0xB2, 0x09, 0x00, 0x01, // arg, point, count
			0x5D, // DELTAP1
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if ctx.GS.DeltaBase != 11 || ctx.GS.DeltaShift != 2 {
			t.Fatalf("delta graphics state = base %d shift %d, want 11/2", ctx.GS.DeltaBase, ctx.GS.DeltaShift)
		}
		if got := ctx.Zones[1].Points[0].X; got != 32 {
			t.Fatalf("SDB/SDS delta moved X to %d, want 32", got)
		}
	})
}

func TestControlScanAndInstructionStateOpcodes(t *testing.T) {
	sys := core.NewSystem()

	t.Run("SCVTCI_SSWCI_SSW", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.PPEM = 20
		ctx.UnitsPerEm = 1000
		ctx.Code = []byte{
			0xB0, 0x20, 0x1D, // control value cut-in = 32
			0xB0, 0x10, 0x1E, // single width cut-in = 16
			0xB0, 0x64, 0x1F, // single width = 100 FUnits -> 128 F26.6
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if ctx.GS.ControlValueCutIn != 32 {
			t.Fatalf("control value cut-in = %d, want 32", ctx.GS.ControlValueCutIn)
		}
		if ctx.GS.SingleWidthCutIn != 16 {
			t.Fatalf("single width cut-in = %d, want 16", ctx.GS.SingleWidthCutIn)
		}
		if ctx.GS.SingleWidthValue != 128 {
			t.Fatalf("single width value = %d, want 128", ctx.GS.SingleWidthValue)
		}
	})

	t.Run("SCANCTRL_SCANTYPE", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.PPEM = 10
		ctx.Rotated = true
		ctx.Code = []byte{
			0xB8, 0x01, 0x0C, 0x85, // enable if ppem <= 12
			0xB8, 0x10, 0x01, 0x85, // disable if rotated
			0xB0, 0x02, 0x8D, // scan type = 2
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if ctx.GS.ScanControl {
			t.Fatalf("scan control = true, want false after rotated disable")
		}
		if ctx.GS.ScanType != 2 {
			t.Fatalf("scan type = %d, want 2", ctx.GS.ScanType)
		}
	})

	t.Run("INSTCTRL", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.InterpreterVersion = 40
		ctx.Code = []byte{
			0xB1, 0x04, 0x03, 0x8E, // value 4, selector 3
			0xB1, 0x00, 0x03, 0x8E, // value 0, selector 3
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if ctx.GS.InstructControl&0x04 != 0 {
			t.Fatalf("instruction control bit 2 should be cleared, got %#x", ctx.GS.InstructControl)
		}
		if !ctx.GS.BackwardCompatibility {
			t.Fatalf("INSTCTRL selector 3 value 0 should enable backward compatibility in v40 mode")
		}
	})
}

func TestSuperRoundOpcodesAffectRoundInstruction(t *testing.T) {
	sys := core.NewSystem()

	tests := []struct {
		name string
		code []byte
		want int32
	}{
		{
			name: "SROUND",
			code: []byte{
				0xB0, 0x00, 0x76, // selector 0
				0xB0, 0x11, 0x68, // ROUND 17
			},
			want: 32,
		},
		{
			name: "S45ROUND",
			code: []byte{
				0xB0, 0x00, 0x77, // selector 0
				0xB0, 0x11, 0x68, // ROUND 17
			},
			want: 22,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(sys)
			ctx.Code = tt.code
			if err := ctx.Run(); err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			if got := ctx.Stack[ctx.StackTop-1]; got != tt.want {
				t.Fatalf("rounded value = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSingleWidthCutInAffectsRelativeMoves(t *testing.T) {
	sys := core.NewSystem()

	t.Run("MDRP", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.GS.SingleWidthCutIn = 10
		ctx.GS.SingleWidthValue = 128
		ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 124, Y: 0}}
		ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 124, Y: 0}}
		ctx.Zones[1].TouchedX = make([]bool, 2)
		ctx.Zones[1].TouchedY = make([]bool, 2)
		ctx.ZP0 = 1
		ctx.ZP1 = 1
		ctx.RP0 = 0
		ctx.Code = []byte{
			0x01,       // SVTCA[x]
			0xB0, 0x01, // point 1
			0xC0, // MDRP without round/min distance
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if got := ctx.Zones[1].Points[1].X; got != 128 {
			t.Fatalf("MDRP single-width X = %d, want 128", got)
		}
	})

	t.Run("MIRP", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.GS.SingleWidthCutIn = 10
		ctx.GS.SingleWidthValue = 128
		ctx.CVT = []int32{124}
		ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
		ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
		ctx.Zones[1].TouchedX = make([]bool, 2)
		ctx.Zones[1].TouchedY = make([]bool, 2)
		ctx.ZP0 = 1
		ctx.ZP1 = 1
		ctx.RP0 = 0
		ctx.Code = []byte{
			0x01,             // SVTCA[x]
			0xB1, 0x01, 0x00, // point, CVT
			0xE0, // MIRP without round/min distance
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if got := ctx.Zones[1].Points[1].X; got != 128 {
			t.Fatalf("MIRP single-width X = %d, want 128", got)
		}
	})
}

func TestFlipOpcodesUseZoneTagsWhenAvailable(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)
	ctx.Zones[1].Points = []api.Vector{{}, {}, {}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{}, {}, {}}
	ctx.Zones[1].TouchedX = make([]bool, 3)
	ctx.Zones[1].TouchedY = make([]bool, 3)
	ctx.Zones[1].Tags = []byte{1, 0, 1}
	ctx.ZP0 = 1

	ctx.Code = []byte{
		0xB0, 0x02, 0x17, // loop = 2
		0xB1, 0x01, 0x02, // points 1, 2
		0x80,             // FLIPPT
		0xB1, 0x01, 0x02, // low 1, high 2
		0x81,             // FLIPRGON
		0xB1, 0x00, 0x01, // low 0, high 1
		0x82, // FLIPRGOFF
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got, want := ctx.Zones[1].Tags, []byte{0, 0, 1}; got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("tags = %#v, want %#v", got, want)
	}
	if ctx.GS.Loop != 1 {
		t.Fatalf("FLIPPT should reset loop to 1, got %d", ctx.GS.Loop)
	}
	if ctx.Zones[1].TouchedX[0] || ctx.Zones[1].TouchedX[1] || ctx.Zones[1].TouchedX[2] {
		t.Fatalf("FLIP opcodes should not touch points, touched X=%#v", ctx.Zones[1].TouchedX)
	}
}

func TestFlipOpcodesPreserveNonOnCurveTagBits(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)
	ctx.Zones[1].Points = []api.Vector{{}, {}, {}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{}, {}, {}}
	ctx.Zones[1].TouchedX = make([]bool, 3)
	ctx.Zones[1].TouchedY = make([]bool, 3)
	ctx.Zones[1].Tags = []byte{0x02, 0x03, 0x04}
	ctx.ZP0 = 1

	ctx.Code = []byte{
		0xB0, 0x00, 0x80, // flip point 0: 0x02 -> 0x03
		0xB1, 0x00, 0x01, 0x81, // set on-curve for 0..1
		0xB1, 0x01, 0x02, 0x82, // clear on-curve for 1..2
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	want := []byte{0x03, 0x02, 0x04}
	for i := range want {
		if ctx.Zones[1].Tags[i] != want[i] {
			t.Fatalf("tag[%d] = %#x, want %#x; all tags %#v", i, ctx.Zones[1].Tags[i], want[i], ctx.Zones[1].Tags)
		}
	}
}

func TestISECTMovesPointToLineIntersection(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)
	ctx.Zones[0].Points = []api.Vector{{X: 50, Y: -50}, {X: 50, Y: 50}}
	ctx.Zones[0].OriginalPoints = append([]api.Vector(nil), ctx.Zones[0].Points...)
	ctx.Zones[0].TouchedX = make([]bool, 2)
	ctx.Zones[0].TouchedY = make([]bool, 2)
	ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}, {X: 0, Y: 0}}
	ctx.Zones[1].OriginalPoints = append([]api.Vector(nil), ctx.Zones[1].Points...)
	ctx.Zones[1].TouchedX = make([]bool, 3)
	ctx.Zones[1].TouchedY = make([]bool, 3)
	ctx.ZP0 = 0
	ctx.ZP1 = 1
	ctx.ZP2 = 1

	ctx.Code = []byte{
		0xB4, 0x02, 0x00, 0x01, 0x00, 0x01, // p, a0, a1, b0, b1
		0x0F, // ISECT
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got := ctx.Zones[1].Points[2]; got != (api.Vector{X: 50, Y: 0}) {
		t.Fatalf("ISECT point = %#v, want (50,0)", got)
	}
	if !ctx.Zones[1].TouchedX[2] || !ctx.Zones[1].TouchedY[2] {
		t.Fatalf("ISECT should touch both axes, touched X/Y=%v/%v", ctx.Zones[1].TouchedX[2], ctx.Zones[1].TouchedY[2])
	}
}

func TestGETINFOAndAutoFlip(t *testing.T) {
	sys := core.NewSystem()

	t.Run("GETINFO", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.Code = []byte{0xB0, 0x01, 0x88}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if got := ctx.Stack[ctx.StackTop-1]; got != 35 {
			t.Fatalf("GETINFO version = %d, want 35", got)
		}
	})

	t.Run("GETINFO_capability_bits", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.InterpreterVersion = 40
		ctx.Rotated = true
		ctx.Stretched = true
		ctx.Variation = true
		ctx.Grayscale = true
		ctx.RenderMode = api.RenderModeLCDV
		ctx.Code = []byte{
			0xB8, 0x1D, 0x6F, // selector bits 0,1,2,3,5,6,8,10,11,12
			0x88,
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		want := int32(40 | 1<<8 | 1<<9 | 1<<10 | 1<<12 | 1<<13 | 1<<15 | 1<<17 | 1<<18)
		if got := ctx.Stack[ctx.StackTop-1]; got != want {
			t.Fatalf("GETINFO capabilities = %#x, want %#x", got, want)
		}
	})

	t.Run("FLIPOFF_affects_MIRP_CVT_sign", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.CVT = []int32{-100}
		ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
		ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
		ctx.Zones[1].TouchedX = make([]bool, 2)
		ctx.Zones[1].TouchedY = make([]bool, 2)
		ctx.ZP0 = 1
		ctx.ZP1 = 1
		ctx.RP0 = 0
		ctx.Code = []byte{
			0x01,             // SVTCA[x]
			0x4E,             // FLIPOFF
			0xB1, 0x01, 0x00, // point, CVT
			0xE0, // MIRP without rounding or minimum distance
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if got := ctx.Zones[1].Points[1].X; got != -100 {
			t.Fatalf("FLIPOFF MIRP moved X to %d, want -100", got)
		}
	})
}

func TestObsoleteNoopOpcodesAndUnknownFallback(t *testing.T) {
	sys := core.NewSystem()

	t.Run("SANGW_AA", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.Code = []byte{
			0xB0, 0x11, 0x7E,
			0xB0, 0x22, 0x7F,
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if ctx.StackTop != 0 {
			t.Fatalf("obsolete no-op opcodes should consume arguments, stack top=%d", ctx.StackTop)
		}
	})

	t.Run("UNKNOWN", func(t *testing.T) {
		for _, opcode := range []byte{0x83, 0x91} {
			ctx := NewContext(sys)
			ctx.Code = []byte{opcode}
			if err := ctx.Run(); err == nil {
				t.Fatalf("unknown opcode 0x%02x should fail", opcode)
			}
		}
	})
}

func TestMinimumDistancePreservesSign(t *testing.T) {
	if got := applyMinimumDistance(-10, 64); got != -64 {
		t.Fatalf("negative distance got %d, want -64", got)
	}
	if got := applyMinimumDistance(10, 64); got != 64 {
		t.Fatalf("positive distance got %d, want 64", got)
	}
	if got := applyMinimumDistance(-90, 64); got != -90 {
		t.Fatalf("large negative distance got %d, want -90", got)
	}
}

func TestVectorToLineSetsProjectionAndFreedomVectors(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 2)
	ctx.Zones[1].TouchedY = make([]bool, 2)
	ctx.ZP1 = 1
	ctx.ZP2 = 1

	ctx.Code = []byte{
		0xB0, 0x01, // p2
		0xB0, 0x00, // p1
		0x06,       // SPVTL[0], parallel
		0xB0, 0x01, // p2
		0xB0, 0x00, // p1
		0x09, // SFVTL[1], perpendicular
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got := ctx.GS.ProjVector; got != (api.Vector{X: 0x4000, Y: 0}) {
		t.Fatalf("ProjVector = %#v, want x-axis", got)
	}
	if got := ctx.GS.FreeVector; got != (api.Vector{X: 0, Y: 0x4000}) {
		t.Fatalf("FreeVector = %#v, want y-axis", got)
	}
}

func TestStackVectorOps(t *testing.T) {
	sys := core.NewSystem()

	t.Run("SPVFS_GPV", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.Code = []byte{
			0xB9, 0x00, 0x00, 0x40, 0x00, // x=0, y=1.0
			0x0A, // SPVFS
			0x0C, // GPV
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if got := ctx.GS.ProjVector; got != (api.Vector{X: 0, Y: 0x4000}) {
			t.Fatalf("ProjVector = %#v, want y-axis", got)
		}
		if got := ctx.GS.DualVector; got != (api.Vector{X: 0, Y: 0x4000}) {
			t.Fatalf("DualVector = %#v, want y-axis", got)
		}
		if ctx.StackTop != 2 || ctx.Stack[0] != 0 || ctx.Stack[1] != 0x4000 {
			t.Fatalf("GPV stack = %v top=%d, want [0 16384]", ctx.Stack[:ctx.StackTop], ctx.StackTop)
		}
	})

	t.Run("SFVFS_GFV", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.Code = []byte{
			0xB9, 0x00, 0x00, 0x40, 0x00, // x=0, y=1.0
			0x0B, // SFVFS
			0x0D, // GFV
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if got := ctx.GS.FreeVector; got != (api.Vector{X: 0, Y: 0x4000}) {
			t.Fatalf("FreeVector = %#v, want y-axis", got)
		}
		if ctx.StackTop != 2 || ctx.Stack[0] != 0 || ctx.Stack[1] != 0x4000 {
			t.Fatalf("GFV stack = %v top=%d, want [0 16384]", ctx.Stack[:ctx.StackTop], ctx.StackTop)
		}
	})

	t.Run("SFVTPV", func(t *testing.T) {
		ctx := NewContext(sys)
		ctx.Code = []byte{
			0xB9, 0x00, 0x00, 0x40, 0x00, // x=0, y=1.0
			0x0A, // SPVFS
			0x0E, // SFVTPV
		}
		if err := ctx.Run(); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if got := ctx.GS.FreeVector; got != (api.Vector{X: 0, Y: 0x4000}) {
			t.Fatalf("FreeVector = %#v, want projection y-axis", got)
		}
	})
}

func TestSDPVTLUsesOriginalForDualAndCurrentForProjection(t *testing.T) {
	sys := core.NewSystem()

	tests := []struct {
		name     string
		opcode   byte
		wantProj api.Vector
		wantDual api.Vector
	}{
		{"parallel", 0x86, api.Vector{X: 0x4000, Y: 0}, api.Vector{X: 0, Y: 0x4000}},
		{"perpendicular", 0x87, api.Vector{X: 0, Y: 0x4000}, api.Vector{X: -0x4000, Y: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(sys)
			ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
			ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 0, Y: 100}}
			ctx.Zones[1].TouchedX = make([]bool, 2)
			ctx.Zones[1].TouchedY = make([]bool, 2)
			ctx.ZP1 = 1
			ctx.ZP2 = 1

			ctx.Code = []byte{
				0xB1, 0x01, 0x00, // p2=1, p1=0
				tt.opcode,
			}
			if err := ctx.Run(); err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			if got := ctx.GS.ProjVector; got != tt.wantProj {
				t.Fatalf("ProjVector = %#v, want %#v", got, tt.wantProj)
			}
			if got := ctx.GS.DualVector; got != tt.wantDual {
				t.Fatalf("DualVector = %#v, want %#v", got, tt.wantDual)
			}
		})
	}
}

func TestZonePointerOps(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.Code = []byte{
		0xB0, 0x00, 0x16, // SZPS: zp0/zp1/zp2 = twilight
		0xB0, 0x01, 0x13, // SZP0: glyph
		0xB0, 0x00, 0x14, // SZP1: twilight
		0xB0, 0x01, 0x15, // SZP2: glyph
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if ctx.ZP0 != 1 || ctx.ZP1 != 0 || ctx.ZP2 != 1 {
		t.Fatalf("zone pointers = %d/%d/%d, want 1/0/1", ctx.ZP0, ctx.ZP1, ctx.ZP2)
	}
}

func TestAlignPoints(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.Zones[1].Points = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 0, Y: 0}, {X: 100, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 2)
	ctx.Zones[1].TouchedY = make([]bool, 2)
	ctx.ZP0 = 1
	ctx.ZP1 = 1

	ctx.Code = []byte{
		0xB1, 0x01, 0x00, // p1=1, p2=0
		0x27, // ALIGNPTS
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if ctx.Zones[1].Points[0].X != 50 || ctx.Zones[1].Points[1].X != 50 {
		t.Fatalf("aligned X positions = %d/%d, want 50/50", ctx.Zones[1].Points[0].X, ctx.Zones[1].Points[1].X)
	}
	if !ctx.Zones[1].TouchedX[0] || !ctx.Zones[1].TouchedX[1] {
		t.Fatalf("ALIGNPTS should touch both points on X")
	}
}

func TestAlignReferencePoint(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.Zones[1].Points = []api.Vector{{X: 100, Y: 0}, {X: 40, Y: 0}, {X: 160, Y: 0}}
	ctx.Zones[1].OriginalPoints = []api.Vector{{X: 100, Y: 0}, {X: 40, Y: 0}, {X: 160, Y: 0}}
	ctx.Zones[1].TouchedX = make([]bool, 3)
	ctx.Zones[1].TouchedY = make([]bool, 3)
	ctx.ZP0 = 1
	ctx.ZP1 = 1
	ctx.RP0 = 0

	ctx.Code = []byte{
		0xB0, 0x02, 0x17, // SLOOP 2
		0xB1, 0x01, 0x02, // points 1 and 2
		0x3C, // ALIGNRP
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if ctx.Zones[1].Points[1].X != 100 || ctx.Zones[1].Points[2].X != 100 {
		t.Fatalf("aligned X positions = %d/%d, want 100/100", ctx.Zones[1].Points[1].X, ctx.Zones[1].Points[2].X)
	}
	if !ctx.Zones[1].TouchedX[1] || !ctx.Zones[1].TouchedX[2] {
		t.Fatalf("ALIGNRP should touch aligned points on X")
	}
	if ctx.GS.Loop != 1 {
		t.Fatalf("ALIGNRP should reset loop to 1, got %d", ctx.GS.Loop)
	}
}

func TestOddEven(t *testing.T) {
	sys := core.NewSystem()

	tests := []struct {
		name     string
		code     []byte
		expected int32
	}{
		{"ODD_TRUE", []byte{0xB0, 0x40, 0x56}, 1},
		{"ODD_FALSE", []byte{0xB0, 0x80, 0x56}, 0},
		{"EVEN_TRUE", []byte{0xB0, 0x80, 0x57}, 1},
		{"EVEN_FALSE", []byte{0xB0, 0x40, 0x57}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewContext(sys)
			ctx.Code = tt.code
			if err := ctx.Run(); err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			got, err := ctx.pop()
			if err != nil {
				t.Fatalf("Pop failed: %v", err)
			}
			if got != tt.expected {
				t.Fatalf("result = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestWCVTFScalesFontUnitsWhenUnitsPerEmIsKnown(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.CVT = []int32{0}
	ctx.PPEM = 20
	ctx.UnitsPerEm = 1000

	ctx.Code = []byte{
		0xB0, 0x00, // CVT index
		0xB0, 0x64, // 100 font units
		0x70, // WCVTF
	}
	if err := ctx.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if got := ctx.CVT[0]; got != 128 {
		t.Fatalf("WCVTF CVT[0] = %d, want 128", got)
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

func TestInstructionBudgetStopsInfiniteJump(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)
	ctx.InstructionBudget = 8

	// PUSHW -3; JMPR jumps back to the PUSHW opcode forever without a budget.
	ctx.Code = []byte{0xB8, 0xFF, 0xFD, 0x1C}

	err := ctx.Run()
	if !errors.Is(err, ErrInstructionBudgetExceeded) {
		t.Fatalf("Run error = %v, want %v", err, ErrInstructionBudgetExceeded)
	}
}

func TestCallDepthLimitStopsRecursiveCall(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)
	ctx.MaxCallDepth = 2

	// Function 1 calls itself recursively.
	ctx.Code = []byte{
		0xB0, 0x01, 0x2C, // FDEF 1
		0xB0, 0x01, 0x2B, // CALL 1
		0x2D,             // ENDF
		0xB0, 0x01, 0x2B, // CALL 1
	}

	err := ctx.Run()
	if !errors.Is(err, ErrCallDepthExceeded) {
		t.Fatalf("Run error = %v, want %v", err, ErrCallDepthExceeded)
	}
}

func TestLoopCallRepeatLimit(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)
	ctx.MaxLoopCallRepeat = 2

	ctx.Code = []byte{
		0xB0, 0x01, 0x2C, // FDEF 1
		0x2D,       // ENDF
		0xB0, 0x03, // count
		0xB0, 0x01, // funcID
		0x2A, // LOOPCALL
	}

	err := ctx.Run()
	if !errors.Is(err, ErrLoopCallRepeatExceeded) {
		t.Fatalf("Run error = %v, want %v", err, ErrLoopCallRepeatExceeded)
	}
}

func TestStackOverflowUsesHardCap(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)
	ctx.Prepare(0, 0, 2, 0, 0)
	ctx.Code = []byte{0xB2, 0x01, 0x02, 0x03} // PUSHB[3]

	err := ctx.Run()
	if !errors.Is(err, ErrStackOverflow) {
		t.Fatalf("Run error = %v, want %v", err, ErrStackOverflow)
	}
	if ctx.StackTop != 2 {
		t.Fatalf("StackTop = %d, want hard cap 2", ctx.StackTop)
	}
}

func TestPrepareUsesSafeDefaultForInvalidStackLimit(t *testing.T) {
	sys := core.NewSystem()
	ctx := NewContext(sys)

	ctx.Prepare(0, 0, 0, 0, 0)
	if len(ctx.Stack) != defaultMaxStackElements {
		t.Fatalf("stack len for zero maxStack = %d, want %d", len(ctx.Stack), defaultMaxStackElements)
	}

	ctx.Prepare(0, 0, maxStackElementsHardCap+1, 0, 0)
	if len(ctx.Stack) != defaultMaxStackElements {
		t.Fatalf("stack len for oversized maxStack = %d, want %d", len(ctx.Stack), defaultMaxStackElements)
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
