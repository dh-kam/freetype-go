package truetype

import (
	"fmt"
	"sync"

	"github.com/dh-kam/freetype-go/api"
)

// F26Dot6 represents a 26.6 fixed-point number.
type F26Dot6 int32

// Zone represents a TrueType zone (Twilight or Glyph).
type Zone struct {
	Points         []api.Vector
	OriginalPoints []api.Vector
	TouchedX       []bool
	TouchedY       []bool
	Contours       []int
}

// CallRecord stores the state to return to after a function call.
type CallRecord struct {
	ReturnIP   int
	ReturnCode []byte
	Repeat     int32
}

// GraphicsState contains standard TTF Graphics State variables.
type GraphicsState struct {
	AutoFlip              bool
	ControlValueCutIn     F26Dot6
	DeltaBase             int32
	DeltaShift            int32
	FreeVector            api.Vector
	ProjVector            api.Vector
	DualVector            api.Vector
	Loop                  int32
	MinimumDistance       F26Dot6
	RoundState            int32
	BackwardCompatibility bool // ClearType/Infinality subpixel pooling hacks
}

// ExecutionEnv represents the VM state for the TrueType interpreter.
type ExecutionEnv struct {
	GS           *GraphicsState
	Stack        []int32
	StackTop     int
	IP           int
	Code         []byte
	CallStack    []CallRecord
	Storage      []int32
	CVT          []int32
	Functions    map[int32][]byte
	Instructions map[int32][]byte

	Zones [2]Zone
	ZP0   int
	ZP1   int
	ZP2   int

	RP0      int
	RP1      int
	RP2      int
	RefPoint api.Vector

	System api.FreetypeSystem

	PPEM      int
	PointSize int32
}

var contextPool = sync.Pool{
	New: func() interface{} {
		return &ExecutionEnv{
			GS:           &GraphicsState{},
			Stack:        make([]int32, 4096),
			Storage:      make([]int32, 4096),
			CVT:          make([]int32, 4096),
			CallStack:    make([]CallRecord, 0, 128),
			Functions:    make(map[int32][]byte),
			Instructions: make(map[int32][]byte),
		}
	},
}

// NewContext returns a pooled ExecutionEnv or a new one.
func NewContext(sys api.FreetypeSystem) *ExecutionEnv {
	e := contextPool.Get().(*ExecutionEnv)
	e.Reset(sys)
	return e
}

// Free returns the context to the pool.
func (e *ExecutionEnv) Free() {
	contextPool.Put(e)
}

// Reset resets the ExecutionEnv to default values.
func (e *ExecutionEnv) Reset(sys api.FreetypeSystem) {
	e.GS.AutoFlip = true
	e.GS.ControlValueCutIn = 68
	e.GS.DeltaBase = 9
	e.GS.DeltaShift = 3
	e.GS.Loop = 1
	e.GS.MinimumDistance = 64
	e.GS.RoundState = 1
	e.GS.BackwardCompatibility = false
	e.GS.ProjVector = api.Vector{X: 0x4000, Y: 0}
	e.GS.FreeVector = api.Vector{X: 0x4000, Y: 0}
	e.GS.DualVector = api.Vector{X: 0x4000, Y: 0}

	e.StackTop = 0
	e.IP = 0
	e.CallStack = e.CallStack[:0]
	e.Functions = make(map[int32][]byte)
	e.Instructions = make(map[int32][]byte)

	e.ZP0 = 1
	e.ZP1 = 1
	e.ZP2 = 1
	e.RP0 = 0
	e.RP1 = 0
	e.RP2 = 0
	e.System = sys

	// Clear Zone 0 (Twilight Zone)
	for i := range e.Zones[0].Points {
		e.Zones[0].Points[i] = api.Vector{}
		e.Zones[0].OriginalPoints[i] = api.Vector{}
		e.Zones[0].TouchedX[i] = false
		e.Zones[0].TouchedY[i] = false
	}
}

// Prepare initializes the buffers and Twilight Zone based on the provided limits.
func (e *ExecutionEnv) Prepare(maxTwilightPoints, maxStorage, maxStack int, ppem int, pointSize int32) {
	e.PPEM = ppem
	e.PointSize = pointSize
	if maxTwilightPoints > len(e.Zones[0].Points) {
		e.Zones[0].Points = make([]api.Vector, maxTwilightPoints)
		e.Zones[0].OriginalPoints = make([]api.Vector, maxTwilightPoints)
		e.Zones[0].TouchedX = make([]bool, maxTwilightPoints)
		e.Zones[0].TouchedY = make([]bool, maxTwilightPoints)
	} else {
		e.Zones[0].Points = e.Zones[0].Points[:maxTwilightPoints]
		e.Zones[0].OriginalPoints = e.Zones[0].OriginalPoints[:maxTwilightPoints]
		e.Zones[0].TouchedX = e.Zones[0].TouchedX[:maxTwilightPoints]
		e.Zones[0].TouchedY = e.Zones[0].TouchedY[:maxTwilightPoints]
	}

	if maxStorage > len(e.Storage) {
		e.Storage = make([]int32, maxStorage)
	} else {
		e.Storage = e.Storage[:maxStorage]
	}
	// Storage should be cleared
	for i := range e.Storage {
		e.Storage[i] = 0
	}

	if maxStack > len(e.Stack) {
		e.Stack = make([]int32, maxStack)
	} else {
		e.Stack = e.Stack[:maxStack]
	}
	e.StackTop = 0
}

// Run executes the bytecode until the end or an error occurs.
func (e *ExecutionEnv) Run() error {
	for e.IP < len(e.Code) {
		opcode := e.Code[e.IP]
		if err := e.Step(opcode); err != nil {
			return fmt.Errorf("error at IP %d (opcode 0x%02x): %w", e.IP, opcode, err)
		}
	}
	return nil
}
