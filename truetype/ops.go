package truetype

import (
	"fmt"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/math"
)

// push adds a value to the stack.
func (e *ExecutionEnv) push(val int32) error {
	if e.StackTop >= len(e.Stack) {
		e.Stack = append(e.Stack, make([]int32, 1024)...)
	}
	// BCE hint
	_ = e.Stack[e.StackTop]
	e.Stack[e.StackTop] = val
	e.StackTop++
	return nil
}

// pop removes and returns the top value from the stack.
func (e *ExecutionEnv) pop() (int32, error) {
	if e.StackTop <= 0 {
		return 0, fmt.Errorf("stack underflow")
	}
	e.StackTop--
	return e.Stack[e.StackTop], nil
}

// skip moves the IP to the matching ELSE or EIF.
func (e *ExecutionEnv) skip(allowElse bool) error {
	nesting := 1
	for nesting > 0 {
		e.IP++
		if e.IP >= len(e.Code) {
			return fmt.Errorf("unexpected end of code while skipping")
		}
		op := e.Code[e.IP]
		switch {
		case op == 0x58: // IF
			nesting++
		case op == 0x59: // EIF
			nesting--
		case op == 0x1B: // ELSE
			if nesting == 1 && allowElse {
				nesting--
			}
		case op >= 0xB0 && op <= 0xB7: // PUSHB[1..8]
			n := int(op - 0xB0 + 1)
			e.IP += n
		case op >= 0xB8 && op <= 0xBF: // PUSHW[1..8]
			n := int(op - 0xB8 + 1)
			e.IP += n * 2
		case op == 0x40: // NPUSHB
			if e.IP+1 >= len(e.Code) {
				return fmt.Errorf("truncated NPUSHB")
			}
			n := int(e.Code[e.IP+1])
			e.IP += n + 1
		case op == 0x41: // NPUSHW
			if e.IP+1 >= len(e.Code) {
				return fmt.Errorf("truncated NPUSHW")
			}
			n := int(e.Code[e.IP+1])
			e.IP += n*2 + 1
		}
	}
	e.IP++
	return nil
}

// project returns the projection of a point onto the ProjVector.
func (e *ExecutionEnv) project(p api.Vector) int32 {
	res := (int64(p.X)*int64(e.GS.ProjVector.X) + int64(p.Y)*int64(e.GS.ProjVector.Y) + 0x2000) >> 14
	return int32(res)
}

// move moves a point by a distance along the FreeVector.
func (e *ExecutionEnv) move(p *api.Vector, distance int32) {
	if e.GS.FreeVector.X != 0 {
		p.X += int32((int64(distance)*int64(e.GS.FreeVector.X) + 0x2000) >> 14)
	}
	if e.GS.FreeVector.Y != 0 {
		p.Y += int32((int64(distance)*int64(e.GS.FreeVector.Y) + 0x2000) >> 14)
	}
}

func (e *ExecutionEnv) getProjectedDistance(p1, p2 api.Vector) int32 {
	return e.project(p2) - e.project(p1)
}

func (e *ExecutionEnv) touch(pIdx int, axis int, zoneIdx int) {
	zone := &e.Zones[zoneIdx]
	if axis == 0 { // y
		if pIdx >= 0 && pIdx < len(zone.TouchedY) {
			zone.TouchedY[pIdx] = true
		}
	} else { // x
		if pIdx >= 0 && pIdx < len(zone.TouchedX) {
			zone.TouchedX[pIdx] = true
		}
	}
}

func (e *ExecutionEnv) touchCurrent(pIdx int, zoneIdx int) {
	if e.GS.ProjVector.X != 0 {
		e.touch(pIdx, 1, zoneIdx)
	}
	if e.GS.ProjVector.Y != 0 {
		e.touch(pIdx, 0, zoneIdx)
	}
}

func (e *ExecutionEnv) untouch(pIdx int, axis int, zoneIdx int) {
	zone := &e.Zones[zoneIdx]
	if axis == 0 { // y
		if pIdx >= 0 && pIdx < len(zone.TouchedY) {
			zone.TouchedY[pIdx] = false
		}
	} else { // x
		if pIdx >= 0 && pIdx < len(zone.TouchedX) {
			zone.TouchedX[pIdx] = false
		}
	}
}

func (e *ExecutionEnv) untouchCurrent(pIdx int, zoneIdx int) {
	if e.GS.ProjVector.X != 0 {
		e.untouch(pIdx, 1, zoneIdx)
	}
	if e.GS.ProjVector.Y != 0 {
		e.untouch(pIdx, 0, zoneIdx)
	}
}

func (e *ExecutionEnv) interpolateUntouched(axis int) {
	zone := &e.Zones[e.ZP2]
	if len(zone.Contours) == 0 {
		return
	}

	startPoint := 0
	for _, endPoint := range zone.Contours {
		if endPoint >= len(zone.Points) {
			continue
		}
		e.interpolateContour(startPoint, endPoint, axis, zone)
		startPoint = endPoint + 1
	}
}

func (e *ExecutionEnv) interpolateContour(first, last int, axis int, zone *Zone) {
	if last < first || last >= len(zone.Points) {
		return
	}

	touched := make([]int, 0)
	if axis == 0 { // Y
		_ = zone.TouchedY[last]
		for i := first; i <= last; i++ {
			if zone.TouchedY[i] {
				touched = append(touched, i)
			}
		}
	} else { // X
		_ = zone.TouchedX[last]
		for i := first; i <= last; i++ {
			if zone.TouchedX[i] {
				touched = append(touched, i)
			}
		}
	}

	if len(touched) == 0 {
		return
	}

	if len(touched) == 1 {
		idx := touched[0]
		var shift int32
		if axis == 0 {
			_ = zone.Points[last]
			_ = zone.OriginalPoints[last]
			shift = zone.Points[idx].Y - zone.OriginalPoints[idx].Y
			for i := first; i <= last; i++ {
				if i != idx {
					zone.Points[i].Y = zone.OriginalPoints[i].Y + shift
				}
			}
		} else {
			_ = zone.Points[last]
			_ = zone.OriginalPoints[last]
			shift = zone.Points[idx].X - zone.OriginalPoints[idx].X
			for i := first; i <= last; i++ {
				if i != idx {
					zone.Points[i].X = zone.OriginalPoints[i].X + shift
				}
			}
		}
		return
	}

	firstTouched := touched[0]
	lastTouched := touched[len(touched)-1]

	if axis == 0 { // Y
		_ = zone.Points[last]
		_ = zone.OriginalPoints[last]
		shiftFirst := zone.Points[firstTouched].Y - zone.OriginalPoints[firstTouched].Y
		for i := first; i < firstTouched; i++ {
			zone.Points[i].Y = zone.OriginalPoints[i].Y + shiftFirst
		}
		shiftLast := zone.Points[lastTouched].Y - zone.OriginalPoints[lastTouched].Y
		for i := lastTouched + 1; i <= last; i++ {
			zone.Points[i].Y = zone.OriginalPoints[i].Y + shiftLast
		}
	} else { // X
		_ = zone.Points[last]
		_ = zone.OriginalPoints[last]
		shiftFirst := zone.Points[firstTouched].X - zone.OriginalPoints[firstTouched].X
		for i := first; i < firstTouched; i++ {
			zone.Points[i].X = zone.OriginalPoints[i].X + shiftFirst
		}
		shiftLast := zone.Points[lastTouched].X - zone.OriginalPoints[lastTouched].X
		for i := lastTouched + 1; i <= last; i++ {
			zone.Points[i].X = zone.OriginalPoints[i].X + shiftLast
		}
	}

	for i := 0; i < len(touched)-1; i++ {
		p1 := touched[i]
		p2 := touched[i+1]

		if p2 <= p1+1 {
			continue
		}

		if axis == 0 { // Y
			_ = zone.Points[p2-1]
			_ = zone.OriginalPoints[p2-1]
			orig1 := zone.OriginalPoints[p1].Y
			orig2 := zone.OriginalPoints[p2].Y
			cur1 := zone.Points[p1].Y
			cur2 := zone.Points[p2].Y

			if orig1 == orig2 {
				for j := p1 + 1; j < p2; j++ {
					zone.Points[j].Y = cur1
				}
			} else {
				for j := p1 + 1; j < p2; j++ {
					origJ := zone.OriginalPoints[j].Y
					ratio := int64(origJ-orig1) * int64(cur2-cur1)
					zone.Points[j].Y = cur1 + int32(ratio/int64(orig2-orig1))
				}
			}
		} else { // X
			_ = zone.Points[p2-1]
			_ = zone.OriginalPoints[p2-1]
			orig1 := zone.OriginalPoints[p1].X
			orig2 := zone.OriginalPoints[p2].X
			cur1 := zone.Points[p1].X
			cur2 := zone.Points[p2].X

			if orig1 == orig2 {
				for j := p1 + 1; j < p2; j++ {
					zone.Points[j].X = cur1
				}
			} else {
				for j := p1 + 1; j < p2; j++ {
					origJ := zone.OriginalPoints[j].X
					ratio := int64(origJ-orig1) * int64(cur2-cur1)
					zone.Points[j].X = cur1 + int32(ratio/int64(orig2-orig1))
				}
			}
		}
	}
}

// round rounds a 26.6 value according to the current RoundState.
func (e *ExecutionEnv) round(value int32) int32 {
	switch e.GS.RoundState {
	case 0: // Round to Half Grid
		if value >= 0 {
			return (value & ^63) + 32
		} else {
			return -((-value & ^63) + 32)
		}
	case 1: // Round to Grid
		if value >= 0 {
			return (value + 32) & ^63
		} else {
			return -((-value + 32) & ^63)
		}
	case 2: // Round to Double Grid
		if value >= 0 {
			return (value + 16) & ^31
		} else {
			return -((-value + 16) & ^31)
		}
	default:
		return value
	}
}

// Step executes a single opcode and updates the instruction pointer.
func (e *ExecutionEnv) Step(opcode byte) error {
	switch {
	case opcode == 0x00 || opcode == 0x01: // SVTCA[y], SVTCA[x]
		axis := int32(opcode & 0x01)
		var vec api.Vector
		if axis == 0 { // y
			vec = api.Vector{X: 0, Y: 0x4000}
		} else { // x
			vec = api.Vector{X: 0x4000, Y: 0}
		}
		e.GS.ProjVector = vec
		e.GS.FreeVector = vec
		e.IP++

	case opcode == 0x02 || opcode == 0x03: // SPVTCA[y], SPVTCA[x]
		axis := int32(opcode & 0x01)
		if axis == 0 { // y
			e.GS.ProjVector = api.Vector{X: 0, Y: 0x4000}
		} else { // x
			e.GS.ProjVector = api.Vector{X: 0x4000, Y: 0}
		}
		e.IP++

	case opcode == 0x04 || opcode == 0x05: // SFVTCA[y], SFVTCA[x]
		axis := int32(opcode & 0x01)
		if axis == 0 { // y
			e.GS.FreeVector = api.Vector{X: 0, Y: 0x4000}
		} else { // x
			e.GS.FreeVector = api.Vector{X: 0x4000, Y: 0}
		}
		e.IP++

	case opcode == 0x13: // SZP0
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.ZP0 = int(val)
		e.IP++

	case opcode == 0x14: // SZP1
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.ZP1 = int(val)
		e.IP++

	case opcode == 0x15: // SZP2
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.ZP2 = int(val)
		e.IP++

	case opcode == 0x10: // SRP0
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.RP0 = int(val)
		e.IP++

	case opcode == 0x11: // SRP1
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.RP1 = int(val)
		e.IP++

	case opcode == 0x12: // SRP2
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.RP2 = int(val)
		e.IP++

	case opcode == 0x17: // SLOOP
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.GS.Loop = val
		e.IP++

	case opcode == 0x1A: // SMD
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.GS.MinimumDistance = F26Dot6(val)
		e.IP++

	case opcode == 0x1D: // SCVTCI
		_, err := e.pop() // Control Value Cut-In
		if err != nil {
			return err
		}
		e.IP++

	case opcode == 0x1E: // SSWCI
		_, err := e.pop() // Single Width Cut-In
		if err != nil {
			return err
		}
		e.IP++

	case opcode == 0x1F: // SSW
		_, err := e.pop() // Single Width
		if err != nil {
			return err
		}
		e.IP++

	case opcode == 0x18: // RTG
		e.GS.RoundState = 1
		e.IP++

	case opcode == 0x19: // RTHG
		e.GS.RoundState = 0
		e.IP++

	case opcode == 0x3D: // RTDG
		e.GS.RoundState = 2
		e.IP++

	case opcode == 0x76: // SROUND
		_, err := e.pop()
		if err != nil {
			return err
		}
		e.IP++

	case opcode == 0x77: // S45ROUND
		_, err := e.pop()
		if err != nil {
			return err
		}
		e.IP++

	case opcode == 0x85: // SCANCTRL
		_, err := e.pop()
		if err != nil {
			return err
		}
		e.IP++

	case opcode == 0x8D: // SCANTYPE
		_, err := e.pop()
		if err != nil {
			return err
		}
		e.IP++

	case opcode == 0x8E: // INSTCTRL
		_, err := e.pop() // selector
		if err != nil {
			return err
		}
		_, err = e.pop() // value
		if err != nil {
			return err
		}
		e.IP++

	case opcode == 0x91: // GETINFO
		selector, err := e.pop()
		if err != nil {
			return err
		}
		var res int32
		if selector&0x01 != 0 {
			res = 35 // Version 35
		}
		e.push(res)
		e.IP++

	case opcode == 0x4B: // MPPEM
		e.push(int32(e.PPEM))
		e.IP++

	case opcode == 0x4C: // MPS
		e.push(e.PointSize)
		e.IP++

	case opcode == 0x49: // MD[0]
		p1Idx, err := e.pop()
		if err != nil {
			return err
		}
		p2Idx, err := e.pop()
		if err != nil {
			return err
		}
		zone1 := &e.Zones[e.ZP1]
		zone0 := &e.Zones[e.ZP0]

		if p1Idx < 0 || int(p1Idx) >= len(zone1.OriginalPoints) || p2Idx < 0 || int(p2Idx) >= len(zone0.OriginalPoints) {
			return fmt.Errorf("index out of bounds in MD[0]: p1=%d, p2=%d", p1Idx, p2Idx)
		}

		dist := e.getProjectedDistance(zone0.OriginalPoints[p2Idx], zone1.OriginalPoints[p1Idx])
		e.push(dist)
		e.IP++

	case opcode == 0x4A: // MD[1]
		p1Idx, err := e.pop()
		if err != nil {
			return err
		}
		p2Idx, err := e.pop()
		if err != nil {
			return err
		}
		zone1 := &e.Zones[e.ZP1]
		zone0 := &e.Zones[e.ZP0]

		if p1Idx < 0 || int(p1Idx) >= len(zone1.Points) || p2Idx < 0 || int(p2Idx) >= len(zone0.Points) {
			return fmt.Errorf("index out of bounds in MD[1]: p1=%d, p2=%d", p1Idx, p2Idx)
		}

		dist := e.getProjectedDistance(zone0.Points[p2Idx], zone1.Points[p1Idx])
		e.push(dist)
		e.IP++

	case opcode == 0x2E || opcode == 0x2F: // MDAP[r]
		pIdx, err := e.pop()
		if err != nil {
			return err
		}
		zone := &e.Zones[e.ZP0]
		if pIdx < 0 || int(pIdx) >= len(zone.Points) {
			return fmt.Errorf("point index out of bounds: %d", pIdx)
		}
		p := zone.Points[pIdx]
		curDist := e.project(p)
		if opcode&0x01 != 0 { // round
			targetDist := e.round(curDist)
			e.move(&p, targetDist-curDist)
			zone.Points[pIdx] = p
		}
		e.touchCurrent(int(pIdx), e.ZP0)
		e.IP++

	case opcode == 0x3E || opcode == 0x3F: // MIAP[r]
		cvtIdx, err := e.pop()
		if err != nil {
			return err
		}
		pIdx, err := e.pop()
		if err != nil {
			return err
		}
		if cvtIdx < 0 || int(cvtIdx) >= len(e.CVT) {
			return fmt.Errorf("CVT index out of bounds: %d", cvtIdx)
		}
		zone := &e.Zones[e.ZP0]
		if pIdx < 0 || int(pIdx) >= len(zone.Points) {
			return fmt.Errorf("point index out of bounds: %d", pIdx)
		}
		targetDist := e.CVT[cvtIdx]
		if opcode&0x01 != 0 { // round
			targetDist = e.round(targetDist)
		}
		p := zone.Points[pIdx]
		curDist := e.project(p)
		e.move(&p, targetDist-curDist)
		zone.Points[pIdx] = p
		e.touchCurrent(int(pIdx), e.ZP0)
		e.IP++

	case opcode >= 0xC0 && opcode <= 0xDF: // MDRP[abcde]
		pIdx, err := e.pop()
		if err != nil {
			return err
		}
		setRP0 := opcode&0x10 != 0
		minDist := opcode&0x08 != 0
		round := opcode&0x04 != 0

		zone1 := &e.Zones[e.ZP1]
		zone0 := &e.Zones[e.ZP0]

		if e.RP0 < 0 || e.RP0 >= len(zone0.Points) || pIdx < 0 || int(pIdx) >= len(zone1.Points) {
			return fmt.Errorf("index out of bounds in MDRP: rp0=%d, p=%d", e.RP0, pIdx)
		}

		orgDist := e.getProjectedDistance(zone0.OriginalPoints[e.RP0], zone1.OriginalPoints[pIdx])
		curDist := e.getProjectedDistance(zone0.Points[e.RP0], zone1.Points[pIdx])

		targetDist := orgDist
		if round {
			targetDist = e.round(targetDist)
		}
		if minDist && targetDist < int32(e.GS.MinimumDistance) {
			targetDist = int32(e.GS.MinimumDistance)
		}

		e.move(&zone1.Points[pIdx], targetDist-curDist)
		e.touchCurrent(int(pIdx), e.ZP1)

		if setRP0 {
			e.RP0 = int(pIdx)
		}
		e.IP++

	case opcode >= 0xE0 && opcode <= 0xFF: // MIRP[abcde]
		cvtIdx, err := e.pop()
		if err != nil {
			return err
		}
		pIdx, err := e.pop()
		if err != nil {
			return err
		}
		setRP0 := opcode&0x10 != 0
		minDist := opcode&0x08 != 0
		round := opcode&0x04 != 0

		if cvtIdx < 0 || int(cvtIdx) >= len(e.CVT) {
			return fmt.Errorf("CVT index out of bounds: %d", cvtIdx)
		}

		zone1 := &e.Zones[e.ZP1]
		zone0 := &e.Zones[e.ZP0]

		if e.RP0 < 0 || e.RP0 >= len(zone0.Points) || pIdx < 0 || int(pIdx) >= len(zone1.Points) {
			return fmt.Errorf("index out of bounds in MIRP: rp0=%d, p=%d", e.RP0, pIdx)
		}

		targetDist := e.CVT[cvtIdx]
		if round {
			targetDist = e.round(targetDist)
		}
		if minDist && targetDist < int32(e.GS.MinimumDistance) {
			targetDist = int32(e.GS.MinimumDistance)
		}

		curDist := e.getProjectedDistance(zone0.Points[e.RP0], zone1.Points[pIdx])

		e.move(&zone1.Points[pIdx], targetDist-curDist)
		e.touchCurrent(int(pIdx), e.ZP1)

		if setRP0 {
			e.RP0 = int(pIdx)
		}
		e.IP++

	case opcode == 0x58: // IF
		cond, err := e.pop()
		if err != nil {
			return err
		}
		if cond == 0 {
			if err := e.skip(true); err != nil {
				return err
			}
		} else {
			e.IP++
		}

	case opcode == 0x1B: // ELSE
		if err := e.skip(false); err != nil {
			return err
		}

	case opcode == 0x59: // EIF
		e.IP++

	case opcode == 0x1C: // JMPR
		offset, err := e.pop()
		if err != nil {
			return err
		}
		e.IP += int(offset)

	case opcode == 0x78: // JROT
		offset, err := e.pop()
		if err != nil {
			return err
		}
		cond, err := e.pop()
		if err != nil {
			return err
		}
		if cond != 0 {
			e.IP += int(offset)
		} else {
			e.IP++
		}

	case opcode == 0x79: // JAF
		offset, err := e.pop()
		if err != nil {
			return err
		}
		cond, err := e.pop()
		if err != nil {
			return err
		}
		if cond == 0 {
			e.IP += int(offset)
		} else {
			e.IP++
		}

	case opcode == 0x29: // UTP
		pIdx, err := e.pop()
		if err != nil {
			return err
		}
		e.untouchCurrent(int(pIdx), e.ZP0)
		e.IP++

	case opcode == 0x38: // SHPIX
		dist, err := e.pop()
		if err != nil {
			return err
		}
		// If BackwardCompatibility is true and we're altering on the X-axis, skip SHPIX
		// to prevent ruining subpixel rendering (ClearType/Infinality hack).
		if e.GS.BackwardCompatibility && e.GS.FreeVector.X != 0 {
			for i := 0; i < int(e.GS.Loop); i++ {
				if _, err := e.pop(); err != nil {
					return err
				}
			}
		} else {
			zone := &e.Zones[e.ZP2]
			for i := 0; i < int(e.GS.Loop); i++ {
				pIdx, err := e.pop()
				if err != nil {
					return err
				}
				if pIdx >= 0 && int(pIdx) < len(zone.Points) {
					e.move(&zone.Points[pIdx], dist)
					e.touchCurrent(int(pIdx), e.ZP2)
				}
			}
		}
		e.GS.Loop = 1
		e.IP++

	case opcode == 0x5D || opcode == 0x71 || opcode == 0x72: // DELTAP1, DELTAP2, DELTAP3
		n, err := e.pop()
		if err != nil {
			return err
		}
		for i := 0; i < int(n); i++ {
			pIdx, err := e.pop()
			if err != nil {
				return err
			}
			arg, err := e.pop()
			if err != nil {
				return err
			}
			// ClearType/Infinality hack for DELTAP
			if e.GS.BackwardCompatibility && e.GS.FreeVector.X != 0 {
				continue // Ignore delta on X-axis
			}
			// Basic framework: normally would compute delta from arg and apply to pIdx
			_ = pIdx
			_ = arg
		}
		e.IP++

	case opcode == 0x39: // IP
		for i := 0; i < int(e.GS.Loop); i++ {
			pIdx, err := e.pop()
			if err != nil {
				return err
			}
			zone2 := &e.Zones[e.ZP2]
			zone1 := &e.Zones[e.ZP1]
			zone0 := &e.Zones[e.ZP0]

			if e.RP1 < 0 || e.RP1 >= len(zone0.Points) || e.RP2 < 0 || e.RP2 >= len(zone1.Points) || pIdx < 0 || int(pIdx) >= len(zone2.Points) {
				return fmt.Errorf("index out of bounds in IP: rp1=%d, rp2=%d, p=%d", e.RP1, e.RP2, pIdx)
			}

			// Original positions projected onto ProjVector
			o1 := e.project(zone0.OriginalPoints[e.RP1])
			o2 := e.project(zone1.OriginalPoints[e.RP2])
			op := e.project(zone2.OriginalPoints[pIdx])

			// Current positions projected onto ProjVector
			c1 := e.project(zone0.Points[e.RP1])
			c2 := e.project(zone1.Points[e.RP2])

			var targetDist int32
			if o1 == o2 {
				targetDist = c1
			} else {
				// Linear interpolation: cp = c1 + (op - o1) * (c2 - c1) / (o2 - o1)
				ratio := int64(op-o1) * int64(c2-c1)
				targetDist = c1 + int32(ratio/int64(o2-o1))
			}

			cp := e.project(zone2.Points[pIdx])
			e.move(&zone2.Points[pIdx], targetDist-cp)
			e.touchCurrent(int(pIdx), e.ZP2)
		}
		e.GS.Loop = 1
		e.IP++

	case opcode == 0x30 || opcode == 0x31: // IUP[y], IUP[x]
		axis := int(opcode & 0x01)
		e.interpolateUntouched(axis)
		e.IP++

	case opcode >= 0xB0 && opcode <= 0xB7: // PUSHB[1..8]
		n := int(opcode - 0xB0 + 1)
		e.IP++
		for i := 0; i < n; i++ {
			if e.IP >= len(e.Code) {
				return fmt.Errorf("unexpected end of code in PUSHB")
			}
			if err := e.push(int32(e.Code[e.IP])); err != nil {
				return err
			}
			e.IP++
		}

	case opcode >= 0xB8 && opcode <= 0xBF: // PUSHW[1..8]
		n := int(opcode - 0xB8 + 1)
		e.IP++
		for i := 0; i < n; i++ {
			if e.IP+1 >= len(e.Code) {
				return fmt.Errorf("unexpected end of code in PUSHW")
			}
			high := int32(e.Code[e.IP])
			low := int32(e.Code[e.IP+1])
			val := (high << 8) | low
			if val >= 0x8000 {
				val -= 0x10000
			}
			if err := e.push(val); err != nil {
				return err
			}
			e.IP += 2
		}

	case opcode == 0x40: // NPUSHB
		e.IP++
		if e.IP >= len(e.Code) {
			return fmt.Errorf("unexpected end of code in NPUSHB")
		}
		n := int(e.Code[e.IP])
		e.IP++
		for i := 0; i < n; i++ {
			if e.IP >= len(e.Code) {
				return fmt.Errorf("unexpected end of code in NPUSHB data")
			}
			if err := e.push(int32(e.Code[e.IP])); err != nil {
				return err
			}
			e.IP++
		}

	case opcode == 0x41: // NPUSHW
		e.IP++
		if e.IP >= len(e.Code) {
			return fmt.Errorf("unexpected end of code in NPUSHW")
		}
		n := int(e.Code[e.IP])
		e.IP++
		for i := 0; i < n; i++ {
			if e.IP+1 >= len(e.Code) {
				return fmt.Errorf("unexpected end of code in NPUSHW data")
			}
			high := int32(e.Code[e.IP])
			low := int32(e.Code[e.IP+1])
			val := (high << 8) | low
			if val >= 0x8000 {
				val -= 0x10000
			}
			if err := e.push(val); err != nil {
				return err
			}
			e.IP += 2
		}

	case opcode == 0x2C: // FDEF
		funcID, err := e.pop()
		if err != nil {
			return err
		}
		startIP := e.IP + 1
		nesting := 1
		for nesting > 0 {
			e.IP++
			if e.IP >= len(e.Code) {
				return fmt.Errorf("unexpected end of code while skipping FDEF")
			}
			op := e.Code[e.IP]
			switch {
			case op == 0x2C: // FDEF
				return fmt.Errorf("nested FDEF is not allowed")
			case op == 0x2D: // ENDF
				nesting--
			case op == 0x89: // IDEF
				return fmt.Errorf("nested IDEF is not allowed")
			case op >= 0xB0 && op <= 0xB7: // PUSHB
				e.IP += int(op - 0xB0 + 1)
			case op >= 0xB8 && op <= 0xBF: // PUSHW
				e.IP += int(op-0xB8+1) * 2
			case op == 0x40: // NPUSHB
				if e.IP+1 >= len(e.Code) {
					return fmt.Errorf("truncated NPUSHB")
				}
				e.IP += int(e.Code[e.IP+1]) + 1
			case op == 0x41: // NPUSHW
				if e.IP+1 >= len(e.Code) {
					return fmt.Errorf("truncated NPUSHW")
				}
				e.IP += int(e.Code[e.IP+1])*2 + 1
			}
		}
		e.Functions[funcID] = e.Code[startIP : e.IP+1]
		e.IP++

	case opcode == 0x89: // IDEF
		opID, err := e.pop()
		if err != nil {
			return err
		}
		startIP := e.IP + 1
		nesting := 1
		for nesting > 0 {
			e.IP++
			if e.IP >= len(e.Code) {
				return fmt.Errorf("unexpected end of code while skipping IDEF")
			}
			op := e.Code[e.IP]
			switch {
			case op == 0x2C: // FDEF
				return fmt.Errorf("nested FDEF is not allowed")
			case op == 0x2D: // ENDF
				nesting--
			case op == 0x89: // IDEF
				return fmt.Errorf("nested IDEF is not allowed")
			case op >= 0xB0 && op <= 0xB7: // PUSHB
				e.IP += int(op - 0xB0 + 1)
			case op >= 0xB8 && op <= 0xBF: // PUSHW
				e.IP += int(op-0xB8+1) * 2
			case op == 0x40: // NPUSHB
				if e.IP+1 >= len(e.Code) {
					return fmt.Errorf("truncated NPUSHB")
				}
				e.IP += int(e.Code[e.IP+1]) + 1
			case op == 0x41: // NPUSHW
				if e.IP+1 >= len(e.Code) {
					return fmt.Errorf("truncated NPUSHW")
				}
				e.IP += int(e.Code[e.IP+1])*2 + 1
			}
		}
		e.Instructions[opID] = e.Code[startIP : e.IP+1]
		e.IP++

	case opcode == 0x2B: // CALL
		funcID, err := e.pop()
		if err != nil {
			return err
		}
		fnCode, ok := e.Functions[funcID]
		if !ok {
			return fmt.Errorf("undefined function %d", funcID)
		}
		e.CallStack = append(e.CallStack, CallRecord{
			ReturnIP:   e.IP + 1,
			ReturnCode: e.Code,
			Repeat:     1,
		})
		e.Code = fnCode
		e.IP = 0

	case opcode == 0x2A: // LOOPCALL
		funcID, err := e.pop()
		if err != nil {
			return err
		}
		count, err := e.pop()
		if err != nil {
			return err
		}
		if count <= 0 {
			e.IP++
			return nil
		}
		fnCode, ok := e.Functions[funcID]
		if !ok {
			return fmt.Errorf("undefined function %d", funcID)
		}
		e.CallStack = append(e.CallStack, CallRecord{
			ReturnIP:   e.IP + 1,
			ReturnCode: e.Code,
			Repeat:     count,
		})
		e.Code = fnCode
		e.IP = 0

	case opcode == 0x2D: // ENDF
		if len(e.CallStack) == 0 {
			e.IP = len(e.Code) // Ensure Run() finishes
			return nil
		}
		lastIdx := len(e.CallStack) - 1
		if e.CallStack[lastIdx].Repeat > 1 {
			e.CallStack[lastIdx].Repeat--
			e.IP = 0
			return nil
		}
		res := e.CallStack[lastIdx]
		e.CallStack = e.CallStack[:lastIdx]
		e.IP = res.ReturnIP
		e.Code = res.ReturnCode

	case opcode == 0x20: // DUP
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.push(val)
		e.push(val)
		e.IP++

	case opcode == 0x21: // POP
		if _, err := e.pop(); err != nil {
			return err
		}
		e.IP++

	case opcode == 0x22: // CLEAR
		e.StackTop = 0
		e.IP++

	case opcode == 0x23: // SWAP
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		e.push(b)
		e.push(a)
		e.IP++

	case opcode == 0x25: // CINDEX
		k, err := e.pop()
		if err != nil {
			return err
		}
		if k <= 0 || int(k) > e.StackTop {
			return fmt.Errorf("stack index out of bounds in CINDEX: %d", k)
		}
		val := e.Stack[e.StackTop-int(k)]
		e.push(val)
		e.IP++

	case opcode == 0x26: // MINDEX
		k, err := e.pop()
		if err != nil {
			return err
		}
		if k <= 0 || int(k) > e.StackTop {
			return fmt.Errorf("stack index out of bounds in MINDEX: %d", k)
		}
		idx := e.StackTop - int(k)
		val := e.Stack[idx]
		copy(e.Stack[idx:e.StackTop-1], e.Stack[idx+1:e.StackTop])
		e.Stack[e.StackTop-1] = val
		e.IP++

	case opcode == 0x60: // ADD
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		e.push(a + b)
		e.IP++

	case opcode == 0x61: // SUB
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		e.push(a - b)
		e.IP++

	case opcode == 0x62: // MUL
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		e.push(math.Mul26(a, b))
		e.IP++

	case opcode == 0x63: // DIV
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		if b == 0 {
			return fmt.Errorf("division by zero")
		}
		e.push(math.Div26(a, b))
		e.IP++

	case opcode == 0x64: // ABS
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a < 0 {
			e.push(-a)
		} else {
			e.push(a)
		}
		e.IP++

	case opcode == 0x65: // NEG
		a, err := e.pop()
		if err != nil {
			return err
		}
		e.push(-a)
		e.IP++

	case opcode == 0x66: // FLOOR
		a, err := e.pop()
		if err != nil {
			return err
		}
		e.push(a & ^63)
		e.IP++

	case opcode == 0x67: // CEIL
		a, err := e.pop()
		if err != nil {
			return err
		}
		e.push((a + 63) & ^63)
		e.IP++

	case opcode >= 0x68 && opcode <= 0x6B: // ROUND
		a, err := e.pop()
		if err != nil {
			return err
		}
		e.push(e.round(a))
		e.IP++

	case opcode >= 0x6C && opcode <= 0x6F: // NROUND
		a, err := e.pop()
		if err != nil {
			return err
		}
		e.push(a)
		e.IP++

	case opcode == 0x8A: // ROLL
		a, err := e.pop()
		if err != nil {
			return err
		}
		b, err := e.pop()
		if err != nil {
			return err
		}
		c, err := e.pop()
		if err != nil {
			return err
		}
		e.push(b)
		e.push(a)
		e.push(c)
		e.IP++

	case opcode == 0x8B: // MAX
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a > b {
			e.push(a)
		} else {
			e.push(b)
		}
		e.IP++

	case opcode == 0x8C: // MIN
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a < b {
			e.push(a)
		} else {
			e.push(b)
		}
		e.IP++

	case opcode == 0x42: // WS
		val, err := e.pop()
		if err != nil {
			return err
		}
		idx, err := e.pop()
		if err != nil {
			return err
		}
		if idx < 0 || int(idx) >= len(e.Storage) {
			return fmt.Errorf("storage index out of bounds: %d", idx)
		}
		e.Storage[idx] = val
		e.IP++

	case opcode == 0x43: // RS
		idx, err := e.pop()
		if err != nil {
			return err
		}
		if idx < 0 || int(idx) >= len(e.Storage) {
			return fmt.Errorf("storage index out of bounds: %d", idx)
		}
		if err := e.push(e.Storage[idx]); err != nil {
			return err
		}
		e.IP++

	case opcode == 0x44: // WCVTP
		val, err := e.pop()
		if err != nil {
			return err
		}
		idx, err := e.pop()
		if err != nil {
			return err
		}
		if idx < 0 || int(idx) >= len(e.CVT) {
			return fmt.Errorf("CVT index out of bounds: %d", idx)
		}
		e.CVT[idx] = val
		e.IP++

	case opcode == 0x45: // RCVT
		idx, err := e.pop()
		if err != nil {
			return err
		}
		if idx < 0 || int(idx) >= len(e.CVT) {
			return fmt.Errorf("CVT index out of bounds: %d", idx)
		}
		if err := e.push(e.CVT[idx]); err != nil {
			return err
		}
		e.IP++

	case opcode == 0x50: // LT
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a < b {
			e.push(1)
		} else {
			e.push(0)
		}
		e.IP++

	case opcode == 0x51: // GT
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a > b {
			e.push(1)
		} else {
			e.push(0)
		}
		e.IP++

	case opcode == 0x52: // EQ
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a == b {
			e.push(1)
		} else {
			e.push(0)
		}
		e.IP++

	case opcode == 0x53: // NEQ
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a != b {
			e.push(1)
		} else {
			e.push(0)
		}
		e.IP++

	case opcode == 0x54: // GTEQ
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a >= b {
			e.push(1)
		} else {
			e.push(0)
		}
		e.IP++

	case opcode == 0x55: // LTEQ
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a <= b {
			e.push(1)
		} else {
			e.push(0)
		}
		e.IP++

	case opcode == 0x5A: // AND
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a != 0 && b != 0 {
			e.push(1)
		} else {
			e.push(0)
		}
		e.IP++

	case opcode == 0x5B: // OR
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a != 0 || b != 0 {
			e.push(1)
		} else {
			e.push(0)
		}
		e.IP++

	case opcode == 0x5C: // NOT
		a, err := e.pop()
		if err != nil {
			return err
		}
		if a == 0 {
			e.push(1)
		} else {
			e.push(0)
		}
		e.IP++

	default:
		return fmt.Errorf("unsupported opcode 0x%02x", opcode)
	}
	return nil
}
