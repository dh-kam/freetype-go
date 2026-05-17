package truetype

import (
	"errors"
	"fmt"
	"sync"

	"github.com/dh-kam/freetype-go/api"
)

// F26Dot6 represents a 26.6 fixed-point number.
type F26Dot6 int32

const (
	defaultInstructionBudget = 1_000_000
	defaultMaxStackElements  = 4096
	maxStackElementsHardCap  = 65536
	defaultMaxCallDepth      = 64
	maxCallDepthHardCap      = 1024
	defaultLoopCallRepeat    = 65535
)

var (
	ErrInstructionBudgetExceeded = errors.New("truetype: instruction budget exceeded")
	ErrStackOverflow             = errors.New("truetype: stack overflow")
	ErrCallDepthExceeded         = errors.New("truetype: call depth exceeded")
	ErrLoopCallRepeatExceeded    = errors.New("truetype: loop call repeat count exceeded")
)

// Zone represents a TrueType zone (Twilight or Glyph).
type Zone struct {
	Points         []api.Vector
	OriginalPoints []api.Vector
	TouchedX       []bool
	TouchedY       []bool
	Contours       []int
	Tags           []byte
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
	SingleWidthCutIn      F26Dot6
	SingleWidthValue      F26Dot6
	SuperRoundPeriod      int32
	SuperRoundPhase       int32
	SuperRoundThreshold   int32
	InstructControl       byte
	ScanControl           bool
	ScanType              int32
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

	PPEM       int
	PointSize  int32
	UnitsPerEm uint16

	InterpreterVersion int32
	Rotated            bool
	Stretched          bool
	Variation          bool
	Grayscale          bool
	RenderMode         api.RenderMode

	InstructionBudget int
	MaxCallDepth      int
	MaxLoopCallRepeat int32

	steps      int
	pendingErr error
}

var contextPool = sync.Pool{
	New: func() interface{} {
		return &ExecutionEnv{
			GS:           &GraphicsState{},
			Stack:        make([]int32, defaultMaxStackElements),
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
	e.GS.SingleWidthCutIn = 0
	e.GS.SingleWidthValue = 0
	e.GS.SuperRoundPeriod = 64
	e.GS.SuperRoundPhase = 0
	e.GS.SuperRoundThreshold = 0
	e.GS.InstructControl = 0
	e.GS.ScanControl = false
	e.GS.ScanType = 0
	e.GS.BackwardCompatibility = false
	e.GS.ProjVector = api.Vector{X: 0x4000, Y: 0}
	e.GS.FreeVector = api.Vector{X: 0x4000, Y: 0}
	e.GS.DualVector = api.Vector{X: 0x4000, Y: 0}

	e.StackTop = 0
	if len(e.Stack) != defaultMaxStackElements {
		if cap(e.Stack) >= defaultMaxStackElements {
			e.Stack = e.Stack[:defaultMaxStackElements]
		} else {
			e.Stack = make([]int32, defaultMaxStackElements)
		}
	}
	e.IP = 0
	e.CallStack = e.CallStack[:0]
	e.Functions = make(map[int32][]byte)
	e.Instructions = make(map[int32][]byte)
	e.InstructionBudget = defaultInstructionBudget
	e.MaxCallDepth = defaultMaxCallDepth
	e.MaxLoopCallRepeat = defaultLoopCallRepeat
	e.UnitsPerEm = 0
	e.InterpreterVersion = 35
	e.Rotated = false
	e.Stretched = false
	e.Variation = false
	e.Grayscale = false
	e.RenderMode = api.RenderModeNone
	e.steps = 0
	e.pendingErr = nil

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
		if i < len(e.Zones[0].Tags) {
			e.Zones[0].Tags[i] = 0
		}
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
		e.Zones[0].Tags = make([]byte, maxTwilightPoints)
	} else {
		e.Zones[0].Points = e.Zones[0].Points[:maxTwilightPoints]
		e.Zones[0].OriginalPoints = e.Zones[0].OriginalPoints[:maxTwilightPoints]
		e.Zones[0].TouchedX = e.Zones[0].TouchedX[:maxTwilightPoints]
		e.Zones[0].TouchedY = e.Zones[0].TouchedY[:maxTwilightPoints]
		if len(e.Zones[0].Tags) >= maxTwilightPoints {
			e.Zones[0].Tags = e.Zones[0].Tags[:maxTwilightPoints]
		} else {
			e.Zones[0].Tags = make([]byte, maxTwilightPoints)
		}
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

	stackLimit := sanitizeMaxStack(maxStack)
	if stackLimit > len(e.Stack) {
		e.Stack = make([]int32, stackLimit)
	} else {
		e.Stack = e.Stack[:stackLimit]
	}
	e.StackTop = 0
}

func sanitizeMaxStack(maxStack int) int {
	if maxStack <= 0 || maxStack > maxStackElementsHardCap {
		return defaultMaxStackElements
	}
	return maxStack
}

func (e *ExecutionEnv) effectiveInstructionBudget() int {
	if e.InstructionBudget <= 0 {
		return defaultInstructionBudget
	}
	return e.InstructionBudget
}

func (e *ExecutionEnv) effectiveMaxCallDepth() int {
	if e.MaxCallDepth <= 0 || e.MaxCallDepth > maxCallDepthHardCap {
		return defaultMaxCallDepth
	}
	return e.MaxCallDepth
}

func (e *ExecutionEnv) effectiveMaxLoopCallRepeat() int32 {
	if e.MaxLoopCallRepeat <= 0 {
		return defaultLoopCallRepeat
	}
	return e.MaxLoopCallRepeat
}

func (e *ExecutionEnv) recordError(err error) error {
	if e.pendingErr == nil {
		e.pendingErr = err
	}
	return err
}

func (e *ExecutionEnv) consumeStep() error {
	e.steps++
	if e.steps > e.effectiveInstructionBudget() {
		return fmt.Errorf("%w: limit %d", ErrInstructionBudgetExceeded, e.effectiveInstructionBudget())
	}
	return nil
}

// Run executes the bytecode until the end or an error occurs.
func (e *ExecutionEnv) Run() error {
	e.steps = 0
	e.pendingErr = nil
	for e.IP < len(e.Code) {
		if e.IP < 0 {
			return fmt.Errorf("instruction pointer out of bounds: %d", e.IP)
		}
		opcode := e.Code[e.IP]
		if err := e.consumeStep(); err != nil {
			return fmt.Errorf("error at IP %d (opcode 0x%02x): %w", e.IP, opcode, err)
		}
		startIP := e.IP
		if err := e.Step(opcode); err != nil {
			return fmt.Errorf("error at IP %d (opcode 0x%02x): %w", startIP, opcode, err)
		}
		if e.pendingErr != nil {
			return fmt.Errorf("error at IP %d (opcode 0x%02x): %w", startIP, opcode, e.pendingErr)
		}
	}
	return nil
}
