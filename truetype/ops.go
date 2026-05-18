package truetype

import (
	"fmt"
	stdmath "math"

	"github.com/dh-kam/freetype-go/api"
	ftmath "github.com/dh-kam/freetype-go/math"
)

const (
	outlineTagTouchX = 0x08
	outlineTagTouchY = 0x10
)

// push adds a value to the stack.
func (e *ExecutionEnv) push(val int32) error {
	if e.StackTop >= len(e.Stack) {
		return e.recordError(fmt.Errorf("%w: max stack elements %d", ErrStackOverflow, len(e.Stack)))
	}
	// BCE hint
	_ = e.Stack[e.StackTop]
	e.Stack[e.StackTop] = val
	e.StackTop++
	return nil
}

func (e *ExecutionEnv) checkCallDepth() error {
	if len(e.CallStack) >= e.effectiveMaxCallDepth() {
		return fmt.Errorf("%w: limit %d", ErrCallDepthExceeded, e.effectiveMaxCallDepth())
	}
	return nil
}

func (e *ExecutionEnv) checkLoopCallRepeat(count int32) error {
	if count > e.effectiveMaxLoopCallRepeat() {
		return fmt.Errorf("%w: limit %d", ErrLoopCallRepeatExceeded, e.effectiveMaxLoopCallRepeat())
	}
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
	return projectWithVector(p, e.GS.ProjVector)
}

func (e *ExecutionEnv) dualProject(p api.Vector) int32 {
	return projectWithVector(p, e.GS.DualVector)
}

func projectWithVector(p api.Vector, v api.Vector) int32 {
	dot := int64(p.X)*int64(v.X) + int64(p.Y)*int64(v.Y)
	return int32(divRound(dot, 0x4000))
}

// move moves a point by a distance along the FreeVector.
func (e *ExecutionEnv) move(p *api.Vector, distance int32) {
	if e.GS.FreeVector.X != 0 && !e.backwardSuppressesXMove() {
		p.X += int32(divRound(int64(distance)*int64(e.GS.FreeVector.X), 0x4000))
	}
	if e.GS.FreeVector.Y != 0 && !e.backwardSuppressesYMove() {
		p.Y += int32(divRound(int64(distance)*int64(e.GS.FreeVector.Y), 0x4000))
	}
}

func (e *ExecutionEnv) getProjectedDistance(p1, p2 api.Vector) int32 {
	return e.project(p2) - e.project(p1)
}

func (e *ExecutionEnv) getDualProjectedDistance(p1, p2 api.Vector) int32 {
	return e.dualProject(p2) - e.dualProject(p1)
}

func (e *ExecutionEnv) pointAlongFreedom(base api.Vector, distance int32) api.Vector {
	p := base
	e.move(&p, distance)
	return p
}

func (e *ExecutionEnv) moveProjected(p *api.Vector, distance int32) error {
	if e.backwardSuppressesAllMove() {
		return nil
	}
	denom := int64(e.GS.FreeVector.X)*int64(e.GS.ProjVector.X) + int64(e.GS.FreeVector.Y)*int64(e.GS.ProjVector.Y)
	if denom == 0 {
		return fmt.Errorf("freedom vector is perpendicular to projection vector")
	}
	num := int64(distance) * 0x4000 * 0x4000
	freeDistance := divRound(num, denom)
	if freeDistance > int64(^uint32(0)>>1) || freeDistance < -int64(^uint32(0)>>1)-1 {
		return fmt.Errorf("projected move overflows int32: %d", freeDistance)
	}
	e.move(p, int32(freeDistance))
	return nil
}

func (e *ExecutionEnv) backwardSuppressesXMove() bool {
	return e.GS.BackwardCompatibility && e.GS.FreeVector.X != 0
}

func (e *ExecutionEnv) backwardSuppressesYMove() bool {
	return e.GS.BackwardCompatibility && e.GS.FreeVector.Y != 0 && e.iupXCalled && e.iupYCalled
}

func (e *ExecutionEnv) backwardSuppressesAllMove() bool {
	if !e.GS.BackwardCompatibility {
		return false
	}
	xSuppressed := e.GS.FreeVector.X == 0 || e.backwardSuppressesXMove()
	ySuppressed := e.GS.FreeVector.Y == 0 || e.backwardSuppressesYMove()
	return xSuppressed && ySuppressed
}

func divRound(num, den int64) int64 {
	if den < 0 {
		num = -num
		den = -den
	}
	if num >= 0 {
		return (num + den/2) / den
	}
	return -((-num + den/2) / den)
}

func normalizeLineVector(p1, p2 api.Vector, perpendicular bool) (api.Vector, error) {
	dx := int64(p2.X - p1.X)
	dy := int64(p2.Y - p1.Y)
	if dx == 0 && dy == 0 {
		return api.Vector{}, fmt.Errorf("cannot set vector from coincident points")
	}
	length := stdmath.Hypot(float64(dx), float64(dy))
	x := int32(stdmath.Round(float64(dx) * 0x4000 / length))
	y := int32(stdmath.Round(float64(dy) * 0x4000 / length))
	if perpendicular {
		x, y = -y, x
	}
	return api.Vector{X: x, Y: y}, nil
}

func normalizeStackVector(x, y int32) (api.Vector, error) {
	if x == 0 && y == 0 {
		return api.Vector{}, fmt.Errorf("cannot set vector from zero stack vector")
	}
	length := stdmath.Hypot(float64(x), float64(y))
	return api.Vector{
		X: int32(stdmath.Round(float64(x) * 0x4000 / length)),
		Y: int32(stdmath.Round(float64(y) * 0x4000 / length)),
	}, nil
}

func stackF2Dot14(v int32) int32 {
	return int32(int16(v))
}

func zonePointer(val int32) (int, error) {
	if val != 0 && val != 1 {
		return 0, fmt.Errorf("invalid zone pointer: %d", val)
	}
	return int(val), nil
}

func (e *ExecutionEnv) scaleFUnits(value int32) int32 {
	if e.UnitsPerEm == 0 || e.PPEM <= 0 {
		return value
	}
	num := int64(value) * int64(e.PPEM) * 64
	return int32(divRound(num, int64(e.UnitsPerEm)))
}

func applyMinimumDistance(distance int32, minimum F26Dot6) int32 {
	minDist := int32(minimum)
	if minDist < 0 {
		minDist = -minDist
	}
	if distance >= 0 {
		if distance < minDist {
			return minDist
		}
		return distance
	}
	if distance > -minDist {
		return -minDist
	}
	return distance
}

func applyMinimumDistanceForSign(distance, original int32, minimum F26Dot6) int32 {
	minDist := int32(minimum)
	if minDist < 0 {
		minDist = -minDist
	}
	if original >= 0 {
		if distance < minDist {
			return minDist
		}
		return distance
	}
	if distance > -minDist {
		return -minDist
	}
	return distance
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func (z *Zone) hasUnscaledOriginalPoint(idx int) bool {
	return z != nil && idx >= 0 && idx < len(z.UnscaledOriginalPoints)
}

func (z *Zone) originalInterpolationPoints() []api.Vector {
	if z != nil && len(z.UnscaledOriginalPoints) >= len(z.OriginalPoints) {
		return z.UnscaledOriginalPoints
	}
	if z == nil {
		return nil
	}
	return z.OriginalPoints
}

func axisCoord(points []api.Vector, idx, axis int) int32 {
	if axis == 0 {
		return points[idx].Y
	}
	return points[idx].X
}

func setAxisCoord(points []api.Vector, idx, axis int, value int32) {
	if axis == 0 {
		points[idx].Y = value
		return
	}
	points[idx].X = value
}

func (e *ExecutionEnv) dualProjectUnscaledDistance(baseZone *Zone, baseIdx int, pointZone *Zone, pointIdx int, scale bool) int32 {
	if baseZone == nil || pointZone == nil {
		return 0
	}
	if !baseZone.hasUnscaledOriginalPoint(baseIdx) || !pointZone.hasUnscaledOriginalPoint(pointIdx) {
		return e.getDualProjectedDistance(baseZone.OriginalPoints[baseIdx], pointZone.OriginalPoints[pointIdx])
	}
	delta := api.Vector{
		X: pointZone.UnscaledOriginalPoints[pointIdx].X - baseZone.UnscaledOriginalPoints[baseIdx].X,
		Y: pointZone.UnscaledOriginalPoints[pointIdx].Y - baseZone.UnscaledOriginalPoints[baseIdx].Y,
	}
	if !scale {
		return projectWithVector(delta, e.GS.DualVector)
	}
	if e.XScale == e.YScale {
		return scaleUnscaledFUnit(projectWithVector(delta, e.GS.DualVector), e.XScale)
	}
	delta.X = scaleUnscaledFUnit(delta.X, e.XScale)
	delta.Y = scaleUnscaledFUnit(delta.Y, e.YScale)
	return projectWithVector(delta, e.GS.DualVector)
}

func scaleUnscaledFUnit(value, scale int32) int32 {
	return ftmath.MulFix(value<<6, scale)
}

func (e *ExecutionEnv) dualProjectHintDistance(baseZone *Zone, baseIdx int, pointZone *Zone, pointIdx int) int32 {
	return e.dualProjectUnscaledDistance(baseZone, baseIdx, pointZone, pointIdx, true)
}

func (e *ExecutionEnv) dualProjectIPDistance(baseZone *Zone, baseIdx int, pointZone *Zone, pointIdx int, twilight bool) int32 {
	if twilight {
		return e.getDualProjectedDistance(baseZone.OriginalPoints[baseIdx], pointZone.OriginalPoints[pointIdx])
	}
	return e.dualProjectUnscaledDistance(baseZone, baseIdx, pointZone, pointIdx, e.XScale != e.YScale)
}

func (e *ExecutionEnv) shiftReference(opcode byte) (int, int, int32, error) {
	refZoneIdx := e.ZP1
	refIdx := e.RP2
	if opcode&0x01 != 0 {
		refZoneIdx = e.ZP0
		refIdx = e.RP1
	}
	refZone := &e.Zones[refZoneIdx]
	if refIdx < 0 || refIdx >= len(refZone.Points) || refIdx >= len(refZone.OriginalPoints) {
		return 0, 0, 0, fmt.Errorf("reference point out of bounds: %d", refIdx)
	}
	shift := e.referencePointShift(refZone, refIdx)
	return refZoneIdx, refIdx, shift, nil
}

func (e *ExecutionEnv) referencePointShift(refZone *Zone, refIdx int) int32 {
	return e.project(refZone.Points[refIdx]) - e.dualProject(refZone.OriginalPoints[refIdx])
}

func contourBounds(zone *Zone, contour int32) (int, int, error) {
	if contour < 0 {
		return 0, 0, fmt.Errorf("contour index out of bounds: %d", contour)
	}
	if len(zone.Contours) == 0 {
		if contour != 0 {
			return 0, 0, fmt.Errorf("contour index out of bounds: %d", contour)
		}
		return 0, len(zone.Points), nil
	}
	if int(contour) >= len(zone.Contours) {
		return 0, 0, fmt.Errorf("contour index out of bounds: %d", contour)
	}
	start := 0
	if contour > 0 {
		start = zone.Contours[contour-1] + 1
	}
	limit := zone.Contours[contour] + 1
	if start < 0 || start > len(zone.Points) || limit < start || limit > len(zone.Points) {
		return 0, 0, fmt.Errorf("invalid contour bounds: contour=%d start=%d limit=%d", contour, start, limit)
	}
	return start, limit, nil
}

func realPointLimit(zone *Zone, zoneIdx int) int {
	if zoneIdx == 1 && len(zone.Contours) > 0 {
		limit := zone.Contours[len(zone.Contours)-1] + 1
		if limit >= 0 && limit < len(zone.Points) {
			return limit
		}
	}
	return len(zone.Points)
}

func (e *ExecutionEnv) deltaAmount(opcode byte, arg int32) (int32, bool) {
	ppemBase := e.GS.DeltaBase
	switch opcode {
	case 0x71, 0x74:
		ppemBase += 16
	case 0x72, 0x75:
		ppemBase += 32
	}
	targetPPEM := ppemBase + int32((uint32(arg)&0xF0)>>4)
	if int32(e.PPEM) != targetPPEM {
		return 0, false
	}

	steps := int32(uint32(arg)&0x0F) - 8
	if steps >= 0 {
		steps++
	}
	shift := e.GS.DeltaShift
	if shift < 0 {
		shift = 0
	}
	if shift > 6 {
		return 0, true
	}
	return steps << uint(6-shift), true
}

func intersectLines(a0, a1, b0, b1 api.Vector) (api.Vector, bool) {
	dax := float64(a1.X - a0.X)
	day := float64(a1.Y - a0.Y)
	dbx := float64(b1.X - b0.X)
	dby := float64(b1.Y - b0.Y)
	dx := float64(b0.X - a0.X)
	dy := float64(b0.Y - a0.Y)

	discriminant := dax*(-dby) + day*dbx
	dotproduct := dax*dbx + day*dby
	if 19*stdmath.Abs(discriminant) <= stdmath.Abs(dotproduct) {
		return api.Vector{
			X: int32((int64(a0.X) + int64(a1.X) + int64(b0.X) + int64(b1.X)) / 4),
			Y: int32((int64(a0.Y) + int64(a1.Y) + int64(b0.Y) + int64(b1.Y)) / 4),
		}, false
	}

	val := dx*(-dby) + dy*dbx
	return api.Vector{
		X: a0.X + int32(stdmath.Round(val*dax/discriminant)),
		Y: a0.Y + int32(stdmath.Round(val*day/discriminant)),
	}, true
}

func (e *ExecutionEnv) setSuperRound(gridPeriod, selector int32) {
	var period int32
	switch selector & 0xC0 {
	case 0:
		period = gridPeriod / 2
	case 0x40:
		period = gridPeriod
	case 0x80:
		period = gridPeriod * 2
	default:
		period = gridPeriod
	}

	var phase int32
	switch selector & 0x30 {
	case 0x10:
		phase = period / 4
	case 0x20:
		phase = period / 2
	case 0x30:
		phase = period * 3 / 4
	}

	var threshold int32
	if selector&0x0F == 0 {
		threshold = period - 1
	} else {
		threshold = ((selector & 0x0F) - 4) * period / 8
	}

	e.GS.SuperRoundPeriod = period >> 8
	e.GS.SuperRoundPhase = phase >> 8
	e.GS.SuperRoundThreshold = threshold >> 8
}

func (e *ExecutionEnv) roundSuper(value int32, useDivision bool) int32 {
	period := e.GS.SuperRoundPeriod
	if period <= 0 {
		return value
	}
	phase := e.GS.SuperRoundPhase
	threshold := e.GS.SuperRoundThreshold
	if value >= 0 {
		base := value + threshold - phase
		var rounded int32
		if useDivision {
			rounded = (base / period) * period
		} else {
			rounded = base & -period
		}
		rounded += phase
		if rounded < 0 {
			return phase
		}
		return rounded
	}

	base := threshold - phase - value
	var rounded int32
	if useDivision {
		rounded = -((base / period) * period)
	} else {
		rounded = -(base & -period)
	}
	rounded -= phase
	if rounded > 0 {
		return -phase
	}
	return rounded
}

func (e *ExecutionEnv) applySingleWidthDistance(distance int32, useOriginalBand bool) int32 {
	cutIn := int32(e.GS.SingleWidthCutIn)
	if cutIn <= 0 {
		return distance
	}
	width := int32(e.GS.SingleWidthValue)
	magnitude := distance
	if magnitude < 0 {
		magnitude = -magnitude
	}
	if useOriginalBand {
		if magnitude < width+cutIn && magnitude > width-cutIn {
			if distance >= 0 {
				return width
			}
			return -width
		}
		return distance
	}
	if abs32(magnitude-width) < cutIn {
		if distance >= 0 {
			return width
		}
		return -width
	}
	return distance
}

func (e *ExecutionEnv) effectiveInterpreterVersion() int32 {
	if e.InterpreterVersion > 0 {
		return e.InterpreterVersion
	}
	return 35
}

func (e *ExecutionEnv) isMonoRenderMode() bool {
	return e.RenderMode == api.RenderModeMono
}

func (e *ExecutionEnv) isGrayscaleRendering() bool {
	if e.Grayscale {
		return true
	}
	return e.RenderMode != api.RenderModeNone && e.RenderMode != api.RenderModeMono
}

func (e *ExecutionEnv) getInfo(selector int32) int32 {
	var res int32
	if selector&0x0001 != 0 {
		res = e.effectiveInterpreterVersion()
	}
	if selector&0x0002 != 0 && e.Rotated {
		res |= 1 << 8
	}
	if selector&0x0004 != 0 && e.Stretched {
		res |= 1 << 9
	}
	if selector&0x0008 != 0 && e.Variation {
		res |= 1 << 10
	}
	if selector&0x0020 != 0 && e.isGrayscaleRendering() {
		res |= 1 << 12
	}

	if e.effectiveInterpreterVersion() >= 40 && e.RenderMode != api.RenderModeNone && !e.isMonoRenderMode() {
		if selector&0x0040 != 0 {
			res |= 1 << 13
		}
		if selector&0x0100 != 0 && e.RenderMode == api.RenderModeLCDV {
			res |= 1 << 15
		}
		if selector&0x0400 != 0 {
			res |= 1 << 17
		}
		if selector&0x0800 != 0 {
			res |= 1 << 18
		}
		if selector&0x1000 != 0 &&
			e.RenderMode != api.RenderModeMono &&
			e.RenderMode != api.RenderModeLCD &&
			e.RenderMode != api.RenderModeLCDV {
			res |= 1 << 19
		}
	}

	return res
}

func (e *ExecutionEnv) touch(pIdx int, axis int, zoneIdx int) {
	zone := &e.Zones[zoneIdx]
	if axis == 0 { // y
		if pIdx >= 0 && pIdx < len(zone.TouchedY) {
			zone.TouchedY[pIdx] = true
		}
		if pIdx >= 0 && pIdx < len(zone.Tags) {
			zone.Tags[pIdx] |= outlineTagTouchY
		}
	} else { // x
		if pIdx >= 0 && pIdx < len(zone.TouchedX) {
			zone.TouchedX[pIdx] = true
		}
		if pIdx >= 0 && pIdx < len(zone.Tags) {
			zone.Tags[pIdx] |= outlineTagTouchX
		}
	}
}

func (e *ExecutionEnv) touchCurrent(pIdx int, zoneIdx int) {
	if e.GS.FreeVector.X != 0 {
		e.touch(pIdx, 1, zoneIdx)
	}
	if e.GS.FreeVector.Y != 0 {
		e.touch(pIdx, 0, zoneIdx)
	}
}

func (e *ExecutionEnv) untouch(pIdx int, axis int, zoneIdx int) {
	zone := &e.Zones[zoneIdx]
	if axis == 0 { // y
		if pIdx >= 0 && pIdx < len(zone.TouchedY) {
			zone.TouchedY[pIdx] = false
		}
		if pIdx >= 0 && pIdx < len(zone.Tags) {
			zone.Tags[pIdx] &^= outlineTagTouchY
		}
	} else { // x
		if pIdx >= 0 && pIdx < len(zone.TouchedX) {
			zone.TouchedX[pIdx] = false
		}
		if pIdx >= 0 && pIdx < len(zone.Tags) {
			zone.Tags[pIdx] &^= outlineTagTouchX
		}
	}
}

func (e *ExecutionEnv) untouchCurrent(pIdx int, zoneIdx int) {
	if e.GS.FreeVector.X != 0 {
		e.untouch(pIdx, 1, zoneIdx)
	}
	if e.GS.FreeVector.Y != 0 {
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

	for i := 0; i < len(touched)-1; i++ {
		e.interpolateUntouchedRange(touched[i]+1, touched[i+1]-1, touched[i], touched[i+1], axis, zone)
	}

	firstTouched := touched[0]
	lastTouched := touched[len(touched)-1]
	e.interpolateUntouchedRange(lastTouched+1, last, lastTouched, firstTouched, axis, zone)
	e.interpolateUntouchedRange(first, firstTouched-1, lastTouched, firstTouched, axis, zone)
}

func (e *ExecutionEnv) interpolateUntouchedRange(first, last, ref1, ref2, axis int, zone *Zone) {
	if first > last {
		return
	}

	orus := zone.originalInterpolationPoints()
	orus1 := axisCoord(orus, ref1, axis)
	orus2 := axisCoord(orus, ref2, axis)
	if orus1 > orus2 {
		orus1, orus2 = orus2, orus1
		ref1, ref2 = ref2, ref1
	}

	org1 := axisCoord(zone.OriginalPoints, ref1, axis)
	org2 := axisCoord(zone.OriginalPoints, ref2, axis)
	cur1 := axisCoord(zone.Points, ref1, axis)
	cur2 := axisCoord(zone.Points, ref2, axis)
	delta1 := cur1 - org1
	delta2 := cur2 - org2
	var scale int32
	scaleValid := false

	for i := first; i <= last; i++ {
		orig := axisCoord(zone.OriginalPoints, i, axis)

		var next int32
		switch {
		case orig <= org1:
			next = orig + delta1
		case orig >= org2:
			next = orig + delta2
		case cur1 == cur2 || orus1 == orus2:
			next = cur1
		default:
			if !scaleValid {
				scale = ftmath.DivFix(cur2-cur1, orus2-orus1)
				scaleValid = true
			}
			next = cur1 + ftmath.MulFix(axisCoord(orus, i, axis)-orus1, scale)
		}

		setAxisCoord(zone.Points, i, axis, next)
	}
}

// round rounds a 26.6 value according to the current RoundState.
func (e *ExecutionEnv) round(value int32) int32 {
	return e.roundWithCompensation(value, 0)
}

func (e *ExecutionEnv) roundWithCompensation(value, compensation int32) int32 {
	switch e.GS.RoundState {
	case 0: // Round to Half Grid
		if value >= 0 {
			rounded := ((value + compensation) & ^63) + 32
			if rounded < 0 {
				return 32
			}
			return rounded
		}
		rounded := -(((compensation - value) & ^63) + 32)
		if rounded > 0 {
			return -32
		}
		return rounded
	case 1: // Round to Grid
		if value >= 0 {
			rounded := (value + compensation + 32) & ^63
			if rounded < 0 {
				return 0
			}
			return rounded
		}
		rounded := -((compensation - value + 32) & ^63)
		if rounded > 0 {
			return 0
		}
		return rounded
	case 2: // Round to Double Grid
		if value >= 0 {
			rounded := (value + compensation + 16) & ^31
			if rounded < 0 {
				return 0
			}
			return rounded
		}
		rounded := -((compensation - value + 16) & ^31)
		if rounded > 0 {
			return 0
		}
		return rounded
	case 3: // Round Up To Grid
		if value >= 0 {
			rounded := (value + compensation + 63) & ^63
			if rounded < 0 {
				return 0
			}
			return rounded
		}
		rounded := -((compensation - value + 63) & ^63)
		if rounded > 0 {
			return 0
		}
		return rounded
	case 4: // Round Down To Grid
		if value >= 0 {
			rounded := (value + compensation) & ^63
			if rounded < 0 {
				return 0
			}
			return rounded
		}
		rounded := -((compensation - value) & ^63)
		if rounded > 0 {
			return 0
		}
		return rounded
	case 5: // Round Off
		return compensateDistance(value, compensation)
	case 6: // Super Round
		return e.roundSuperWithCompensation(value, compensation, false)
	case 7: // Super Round 45 degrees
		return e.roundSuperWithCompensation(value, compensation, true)
	default:
		return value
	}
}

func compensateDistance(value, compensation int32) int32 {
	if value >= 0 {
		next := value + compensation
		if next < 0 {
			return 0
		}
		return next
	}
	next := value - compensation
	if next > 0 {
		return 0
	}
	return next
}

func (e *ExecutionEnv) roundSuperWithCompensation(value, compensation int32, useDivision bool) int32 {
	period := e.GS.SuperRoundPeriod
	if period <= 0 {
		return value
	}
	phase := e.GS.SuperRoundPhase
	threshold := e.GS.SuperRoundThreshold
	if value >= 0 {
		base := value + threshold - phase + compensation
		var rounded int32
		if useDivision {
			rounded = (base / period) * period
		} else {
			rounded = base & -period
		}
		rounded += phase
		if rounded < 0 {
			return phase
		}
		return rounded
	}

	base := threshold - phase + compensation - value
	var rounded int32
	if useDivision {
		rounded = -((base / period) * period)
	} else {
		rounded = -(base & -period)
	}
	rounded -= phase
	if rounded > 0 {
		return -phase
	}
	return rounded
}

func (e *ExecutionEnv) engineCompensation(opcode byte) int32 {
	return int32(e.GS.Compensations[opcode&0x03])
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
		e.GS.DualVector = vec
		e.IP++

	case opcode == 0x02 || opcode == 0x03: // SPVTCA[y], SPVTCA[x]
		axis := int32(opcode & 0x01)
		if axis == 0 { // y
			e.GS.ProjVector = api.Vector{X: 0, Y: 0x4000}
		} else { // x
			e.GS.ProjVector = api.Vector{X: 0x4000, Y: 0}
		}
		e.GS.DualVector = e.GS.ProjVector
		e.IP++

	case opcode == 0x04 || opcode == 0x05: // SFVTCA[y], SFVTCA[x]
		axis := int32(opcode & 0x01)
		if axis == 0 { // y
			e.GS.FreeVector = api.Vector{X: 0, Y: 0x4000}
		} else { // x
			e.GS.FreeVector = api.Vector{X: 0x4000, Y: 0}
		}
		e.IP++

	case opcode == 0x06 || opcode == 0x07 || opcode == 0x08 || opcode == 0x09: // SPVTL[a], SFVTL[a]
		p1Idx, err := e.pop()
		if err != nil {
			return err
		}
		p2Idx, err := e.pop()
		if err != nil {
			return err
		}
		zone2 := &e.Zones[e.ZP2]
		zone1 := &e.Zones[e.ZP1]
		if p1Idx < 0 || int(p1Idx) >= len(zone2.Points) || p2Idx < 0 || int(p2Idx) >= len(zone1.Points) {
			return fmt.Errorf("index out of bounds in vector-to-line: p1=%d p2=%d", p1Idx, p2Idx)
		}
		vec, err := normalizeLineVector(zone2.Points[p1Idx], zone1.Points[p2Idx], opcode&0x01 != 0)
		if err != nil {
			return err
		}
		if opcode == 0x06 || opcode == 0x07 {
			e.GS.ProjVector = vec
			e.GS.DualVector = vec
		} else {
			e.GS.FreeVector = vec
		}
		e.IP++

	case opcode == 0x0A || opcode == 0x0B: // SPVFS, SFVFS
		y, err := e.pop()
		if err != nil {
			return err
		}
		x, err := e.pop()
		if err != nil {
			return err
		}
		vec, err := normalizeStackVector(stackF2Dot14(x), stackF2Dot14(y))
		if err != nil {
			return err
		}
		if opcode == 0x0A {
			e.GS.ProjVector = vec
			e.GS.DualVector = vec
		} else {
			e.GS.FreeVector = vec
		}
		e.IP++

	case opcode == 0x0C: // GPV
		if err := e.push(e.GS.ProjVector.X); err != nil {
			return err
		}
		if err := e.push(e.GS.ProjVector.Y); err != nil {
			return err
		}
		e.IP++

	case opcode == 0x0D: // GFV
		if err := e.push(e.GS.FreeVector.X); err != nil {
			return err
		}
		if err := e.push(e.GS.FreeVector.Y); err != nil {
			return err
		}
		e.IP++

	case opcode == 0x0E: // SFVTPV
		e.GS.FreeVector = e.GS.ProjVector
		e.IP++

	case opcode == 0x0F: // ISECT
		b1Idx, err := e.pop()
		if err != nil {
			return err
		}
		b0Idx, err := e.pop()
		if err != nil {
			return err
		}
		a1Idx, err := e.pop()
		if err != nil {
			return err
		}
		a0Idx, err := e.pop()
		if err != nil {
			return err
		}
		pIdx, err := e.pop()
		if err != nil {
			return err
		}
		zone2 := &e.Zones[e.ZP2]
		zone1 := &e.Zones[e.ZP1]
		zone0 := &e.Zones[e.ZP0]
		if pIdx < 0 || int(pIdx) >= len(zone2.Points) ||
			a0Idx < 0 || int(a0Idx) >= len(zone1.Points) ||
			a1Idx < 0 || int(a1Idx) >= len(zone1.Points) ||
			b0Idx < 0 || int(b0Idx) >= len(zone0.Points) ||
			b1Idx < 0 || int(b1Idx) >= len(zone0.Points) {
			return fmt.Errorf("index out of bounds in ISECT: p=%d a0=%d a1=%d b0=%d b1=%d", pIdx, a0Idx, a1Idx, b0Idx, b1Idx)
		}
		zone2.Points[pIdx], _ = intersectLines(zone1.Points[a0Idx], zone1.Points[a1Idx], zone0.Points[b0Idx], zone0.Points[b1Idx])
		e.touch(int(pIdx), 0, e.ZP2)
		e.touch(int(pIdx), 1, e.ZP2)
		e.IP++

	case opcode == 0x13: // SZP0
		val, err := e.pop()
		if err != nil {
			return err
		}
		zp, err := zonePointer(val)
		if err != nil {
			return err
		}
		e.ZP0 = zp
		e.IP++

	case opcode == 0x14: // SZP1
		val, err := e.pop()
		if err != nil {
			return err
		}
		zp, err := zonePointer(val)
		if err != nil {
			return err
		}
		e.ZP1 = zp
		e.IP++

	case opcode == 0x15: // SZP2
		val, err := e.pop()
		if err != nil {
			return err
		}
		zp, err := zonePointer(val)
		if err != nil {
			return err
		}
		e.ZP2 = zp
		e.IP++

	case opcode == 0x16: // SZPS
		val, err := e.pop()
		if err != nil {
			return err
		}
		zp, err := zonePointer(val)
		if err != nil {
			return err
		}
		e.ZP0 = zp
		e.ZP1 = zp
		e.ZP2 = zp
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
		if err := e.checkLoopCallRepeat(val); err != nil {
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
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.GS.ControlValueCutIn = F26Dot6(val)
		e.IP++

	case opcode == 0x1E: // SSWCI
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.GS.SingleWidthCutIn = F26Dot6(val)
		e.IP++

	case opcode == 0x1F: // SSW
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.GS.SingleWidthValue = F26Dot6(e.scaleFUnits(val))
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

	case opcode == 0x7A: // ROFF
		e.GS.RoundState = 5
		e.IP++

	case opcode == 0x7C: // RUTG
		e.GS.RoundState = 3
		e.IP++

	case opcode == 0x7D: // RDTG
		e.GS.RoundState = 4
		e.IP++

	case opcode == 0x76: // SROUND
		selector, err := e.pop()
		if err != nil {
			return err
		}
		e.setSuperRound(0x4000, selector)
		e.GS.RoundState = 6
		e.IP++

	case opcode == 0x77: // S45ROUND
		selector, err := e.pop()
		if err != nil {
			return err
		}
		e.setSuperRound(0x2D41, selector)
		e.GS.RoundState = 7
		e.IP++

	case opcode == 0x85: // SCANCTRL
		val, err := e.pop()
		if err != nil {
			return err
		}
		threshold := val & 0xFF
		switch threshold {
		case 0xFF:
			e.GS.ScanControl = true
		case 0:
			e.GS.ScanControl = false
		default:
			if val&0x0100 != 0 && int32(e.PPEM) <= threshold {
				e.GS.ScanControl = true
			}
			if val&0x0200 != 0 && e.Rotated {
				e.GS.ScanControl = true
			}
			if val&0x0400 != 0 && e.Stretched {
				e.GS.ScanControl = true
			}
			if val&0x0800 != 0 && int32(e.PPEM) > threshold {
				e.GS.ScanControl = false
			}
			if val&0x1000 != 0 && e.Rotated {
				e.GS.ScanControl = false
			}
			if val&0x2000 != 0 && e.Stretched {
				e.GS.ScanControl = false
			}
		}
		e.IP++

	case opcode == 0x8D: // SCANTYPE
		val, err := e.pop()
		if err != nil {
			return err
		}
		if val >= 0 {
			e.GS.ScanType = val & 0xFFFF
		}
		e.IP++

	case opcode == 0x86 || opcode == 0x87: // SDPVTL[a]
		p1Idx, err := e.pop()
		if err != nil {
			return err
		}
		p2Idx, err := e.pop()
		if err != nil {
			return err
		}
		zone2 := &e.Zones[e.ZP2]
		zone1 := &e.Zones[e.ZP1]
		if p1Idx < 0 || int(p1Idx) >= len(zone2.Points) || p2Idx < 0 || int(p2Idx) >= len(zone1.Points) {
			return fmt.Errorf("index out of bounds in SDPVTL: p1=%d p2=%d", p1Idx, p2Idx)
		}
		if int(p1Idx) >= len(zone2.OriginalPoints) || int(p2Idx) >= len(zone1.OriginalPoints) {
			return fmt.Errorf("original point index out of bounds in SDPVTL: p1=%d p2=%d", p1Idx, p2Idx)
		}
		perpendicular := opcode&0x01 != 0
		dualVec, dualErr := normalizeLineVector(zone2.OriginalPoints[p1Idx], zone1.OriginalPoints[p2Idx], perpendicular)
		if dualErr != nil {
			if zone2.OriginalPoints[p1Idx] != zone1.OriginalPoints[p2Idx] {
				return dualErr
			}
			dualVec = api.Vector{X: 0x4000, Y: 0}
			perpendicular = false
		}
		projVec, projErr := normalizeLineVector(zone2.Points[p1Idx], zone1.Points[p2Idx], perpendicular)
		if projErr != nil {
			if zone2.Points[p1Idx] != zone1.Points[p2Idx] {
				return projErr
			}
			projVec = api.Vector{X: 0x4000, Y: 0}
		}
		e.GS.DualVector = dualVec
		e.GS.ProjVector = projVec
		e.IP++

	case opcode == 0x8E: // INSTCTRL
		selector, err := e.pop()
		if err != nil {
			return err
		}
		value, err := e.pop()
		if err != nil {
			return err
		}
		if selector >= 1 && selector <= 3 {
			flag := int32(1 << uint(selector-1))
			if value == 0 || value == flag {
				e.GS.InstructControl &^= byte(flag)
				e.GS.InstructControl |= byte(value)
				if selector == 3 && e.effectiveInterpreterVersion() >= 40 {
					e.GS.BackwardCompatibility = value&flag == 0
				}
			}
		}
		e.IP++

	case opcode == 0x4D: // FLIPON
		e.GS.AutoFlip = true
		e.IP++

	case opcode == 0x4E: // FLIPOFF
		e.GS.AutoFlip = false
		e.IP++

	case opcode == 0x4F: // DEBUG
		if _, err := e.pop(); err != nil {
			return err
		}
		e.IP++

	case opcode == 0x7E || opcode == 0x7F: // SANGW, AA
		if _, err := e.pop(); err != nil {
			return err
		}
		e.IP++

	case opcode == 0x5E: // SDB
		val, err := e.pop()
		if err != nil {
			return err
		}
		e.GS.DeltaBase = val
		e.IP++

	case opcode == 0x5F: // SDS
		val, err := e.pop()
		if err != nil {
			return err
		}
		if val < 0 || val > 6 {
			return fmt.Errorf("invalid delta shift: %d", val)
		}
		e.GS.DeltaShift = val
		e.IP++

	case opcode == 0x88: // GETINFO
		selector, err := e.pop()
		if err != nil {
			return err
		}
		if err := e.push(e.getInfo(selector)); err != nil {
			return err
		}
		e.IP++

	case opcode == 0x4B: // MPPEM
		e.push(int32(e.PPEM))
		e.IP++

	case opcode == 0x4C: // MPS
		e.push(e.PointSize)
		e.IP++

	case opcode == 0x46 || opcode == 0x47: // GC[a]
		pIdx, err := e.pop()
		if err != nil {
			return err
		}
		zone := &e.Zones[e.ZP2]
		if opcode == 0x46 {
			if pIdx < 0 || int(pIdx) >= len(zone.Points) {
				return fmt.Errorf("point index out of bounds in GC[0]: %d", pIdx)
			}
			if err := e.push(e.project(zone.Points[pIdx])); err != nil {
				return err
			}
		} else {
			if pIdx < 0 || int(pIdx) >= len(zone.OriginalPoints) {
				return fmt.Errorf("point index out of bounds in GC[1]: %d", pIdx)
			}
			if err := e.push(e.dualProject(zone.OriginalPoints[pIdx])); err != nil {
				return err
			}
		}
		e.IP++

	case opcode == 0x48: // SCFS
		value, err := e.pop()
		if err != nil {
			return err
		}
		pIdx, err := e.pop()
		if err != nil {
			return err
		}
		zone := &e.Zones[e.ZP2]
		if pIdx < 0 || int(pIdx) >= len(zone.Points) {
			return fmt.Errorf("point index out of bounds in SCFS: %d", pIdx)
		}
		curDist := e.project(zone.Points[pIdx])
		if err := e.moveProjected(&zone.Points[pIdx], value-curDist); err != nil {
			return err
		}
		if e.ZP2 == 0 && int(pIdx) < len(zone.OriginalPoints) {
			zone.OriginalPoints[pIdx] = zone.Points[pIdx]
		}
		e.touchCurrent(int(pIdx), e.ZP2)
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

		var dist int32
		if e.ZP0 == 0 || e.ZP1 == 0 {
			dist = e.getDualProjectedDistance(zone0.OriginalPoints[p2Idx], zone1.OriginalPoints[p1Idx])
		} else {
			dist = e.dualProjectHintDistance(zone0, int(p2Idx), zone1, int(p1Idx))
		}
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
			if err := e.moveProjected(&p, targetDist-curDist); err != nil {
				return err
			}
			zone.Points[pIdx] = p
		}
		e.touchCurrent(int(pIdx), e.ZP0)
		e.RP0 = int(pIdx)
		e.RP1 = int(pIdx)
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
		if e.ZP0 == 0 {
			zone.OriginalPoints[pIdx] = e.pointAlongFreedom(api.Vector{}, targetDist)
			zone.Points[pIdx] = zone.OriginalPoints[pIdx]
		}
		originalDist := e.project(zone.OriginalPoints[pIdx])
		if opcode&0x01 != 0 && abs32(targetDist-originalDist) > int32(e.GS.ControlValueCutIn) {
			targetDist = originalDist
		}
		if opcode&0x01 != 0 { // round
			targetDist = e.round(targetDist)
		}
		p := zone.Points[pIdx]
		curDist := e.project(p)
		if err := e.moveProjected(&p, targetDist-curDist); err != nil {
			return err
		}
		zone.Points[pIdx] = p
		e.touchCurrent(int(pIdx), e.ZP0)
		e.RP0 = int(pIdx)
		e.RP1 = int(pIdx)
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

		var orgDist int32
		if e.ZP0 == 0 || e.ZP1 == 0 {
			orgDist = e.getDualProjectedDistance(zone0.OriginalPoints[e.RP0], zone1.OriginalPoints[pIdx])
		} else {
			orgDist = e.dualProjectHintDistance(zone0, e.RP0, zone1, int(pIdx))
		}
		orgDist = e.applySingleWidthDistance(orgDist, true)
		curDist := e.getProjectedDistance(zone0.Points[e.RP0], zone1.Points[pIdx])

		targetDist := orgDist
		if round {
			targetDist = e.roundWithCompensation(targetDist, e.engineCompensation(opcode))
		} else {
			targetDist = compensateDistance(targetDist, e.engineCompensation(opcode))
		}
		if minDist {
			targetDist = applyMinimumDistanceForSign(targetDist, orgDist, e.GS.MinimumDistance)
		}

		oldRP0 := e.RP0
		if err := e.moveProjected(&zone1.Points[pIdx], targetDist-curDist); err != nil {
			return err
		}
		e.touchCurrent(int(pIdx), e.ZP1)

		e.RP1 = oldRP0
		e.RP2 = int(pIdx)
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
		targetDist = e.applySingleWidthDistance(targetDist, false)
		if e.ZP1 == 0 {
			zone1.OriginalPoints[pIdx] = e.pointAlongFreedom(zone0.OriginalPoints[e.RP0], targetDist)
			zone1.Points[pIdx] = zone1.OriginalPoints[pIdx]
		}
		orgDist := e.getDualProjectedDistance(zone0.OriginalPoints[e.RP0], zone1.OriginalPoints[pIdx])
		if e.GS.AutoFlip && ((orgDist < 0 && targetDist > 0) || (orgDist > 0 && targetDist < 0)) {
			targetDist = -targetDist
		}
		if round && e.ZP0 == e.ZP1 && abs32(targetDist-orgDist) > int32(e.GS.ControlValueCutIn) {
			targetDist = orgDist
		}
		if round {
			targetDist = e.roundWithCompensation(targetDist, e.engineCompensation(opcode))
		} else {
			targetDist = compensateDistance(targetDist, e.engineCompensation(opcode))
		}
		if minDist {
			targetDist = applyMinimumDistanceForSign(targetDist, orgDist, e.GS.MinimumDistance)
		}

		curDist := e.getProjectedDistance(zone0.Points[e.RP0], zone1.Points[pIdx])

		oldRP0 := e.RP0
		if err := e.moveProjected(&zone1.Points[pIdx], targetDist-curDist); err != nil {
			return err
		}
		e.touchCurrent(int(pIdx), e.ZP1)

		e.RP1 = oldRP0
		e.RP2 = int(pIdx)
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

	case opcode == 0x27: // ALIGNPTS
		p2Idx, err := e.pop()
		if err != nil {
			return err
		}
		p1Idx, err := e.pop()
		if err != nil {
			return err
		}
		zone1 := &e.Zones[e.ZP1]
		zone0 := &e.Zones[e.ZP0]
		if p1Idx < 0 || int(p1Idx) >= len(zone1.Points) || p2Idx < 0 || int(p2Idx) >= len(zone0.Points) {
			return fmt.Errorf("index out of bounds in ALIGNPTS: p1=%d p2=%d", p1Idx, p2Idx)
		}
		distance := (e.project(zone0.Points[p2Idx]) - e.project(zone1.Points[p1Idx])) / 2
		if err := e.moveProjected(&zone1.Points[p1Idx], distance); err != nil {
			return err
		}
		if err := e.moveProjected(&zone0.Points[p2Idx], -distance); err != nil {
			return err
		}
		e.touchCurrent(int(p1Idx), e.ZP1)
		e.touchCurrent(int(p2Idx), e.ZP0)
		e.IP++

	case opcode == 0x32 || opcode == 0x33: // SHP[a]
		var refZone *Zone
		var refIdx int
		if opcode&0x01 == 0 {
			refZone = &e.Zones[e.ZP1]
			refIdx = e.RP2
		} else {
			refZone = &e.Zones[e.ZP0]
			refIdx = e.RP1
		}
		if refIdx < 0 || refIdx >= len(refZone.Points) || refIdx >= len(refZone.OriginalPoints) {
			return fmt.Errorf("reference point out of bounds in SHP: %d", refIdx)
		}
		shift := e.referencePointShift(refZone, refIdx)
		zone := &e.Zones[e.ZP2]
		for i := 0; i < int(e.GS.Loop); i++ {
			pIdx, err := e.pop()
			if err != nil {
				return err
			}
			if pIdx < 0 || int(pIdx) >= len(zone.Points) {
				return fmt.Errorf("point index out of bounds in SHP: %d", pIdx)
			}
			if err := e.moveProjected(&zone.Points[pIdx], shift); err != nil {
				return err
			}
			e.touchCurrent(int(pIdx), e.ZP2)
		}
		e.GS.Loop = 1
		e.IP++

	case opcode == 0x34 || opcode == 0x35: // SHC[a]
		contour, err := e.pop()
		if err != nil {
			return err
		}
		refZoneIdx, refIdx, shift, err := e.shiftReference(opcode)
		if err != nil {
			return fmt.Errorf("reference point out of bounds in SHC: %w", err)
		}
		zone := &e.Zones[e.ZP2]
		start, limit, err := contourBounds(zone, contour)
		if err != nil {
			return fmt.Errorf("invalid contour in SHC: %w", err)
		}
		for i := start; i < limit; i++ {
			if refZoneIdx == e.ZP2 && refIdx == i {
				continue
			}
			if err := e.moveProjected(&zone.Points[i], shift); err != nil {
				return err
			}
			e.touchCurrent(i, e.ZP2)
		}
		e.IP++

	case opcode == 0x36 || opcode == 0x37: // SHZ[a]
		zoneVal, err := e.pop()
		if err != nil {
			return err
		}
		zoneIdx, err := zonePointer(zoneVal)
		if err != nil {
			return err
		}
		refZoneIdx, refIdx, shift, err := e.shiftReference(opcode)
		if err != nil {
			return fmt.Errorf("reference point out of bounds in SHZ: %w", err)
		}
		zone := &e.Zones[zoneIdx]
		limit := realPointLimit(zone, zoneIdx)
		for i := 0; i < limit; i++ {
			if refZoneIdx == zoneIdx && refIdx == i {
				continue
			}
			if err := e.moveProjected(&zone.Points[i], shift); err != nil {
				return err
			}
		}
		e.IP++

	case opcode == 0x3C: // ALIGNRP
		zone1 := &e.Zones[e.ZP1]
		zone0 := &e.Zones[e.ZP0]
		if e.RP0 < 0 || e.RP0 >= len(zone0.Points) {
			return fmt.Errorf("reference point out of bounds in ALIGNRP: rp0=%d", e.RP0)
		}
		for i := 0; i < int(e.GS.Loop); i++ {
			pIdx, err := e.pop()
			if err != nil {
				return err
			}
			if pIdx < 0 || int(pIdx) >= len(zone1.Points) {
				return fmt.Errorf("point index out of bounds in ALIGNRP: %d", pIdx)
			}
			distance := e.project(zone0.Points[e.RP0]) - e.project(zone1.Points[pIdx])
			if err := e.moveProjected(&zone1.Points[pIdx], distance); err != nil {
				return err
			}
			e.touchCurrent(int(pIdx), e.ZP1)
		}
		e.GS.Loop = 1
		e.IP++

	case opcode == 0x38: // SHPIX
		dist, err := e.pop()
		if err != nil {
			return err
		}
		zone := &e.Zones[e.ZP2]
		inTwilight := e.ZP0 == 0 || e.ZP1 == 0 || e.ZP2 == 0
		for i := 0; i < int(e.GS.Loop); i++ {
			pIdx, err := e.pop()
			if err != nil {
				return err
			}
			if pIdx < 0 || int(pIdx) >= len(zone.Points) {
				continue
			}
			if e.GS.BackwardCompatibility {
				if !inTwilight && (e.iupXCalled && e.iupYCalled ||
					e.GS.FreeVector.Y == 0 ||
					int(pIdx) >= len(zone.Tags) ||
					zone.Tags[pIdx]&outlineTagTouchY == 0) {
					continue
				}
			}
			e.move(&zone.Points[pIdx], dist)
			e.touchCurrent(int(pIdx), e.ZP2)
		}
		e.GS.Loop = 1
		e.IP++

	case opcode == 0x3A || opcode == 0x3B: // MSIRP[a]
		distance, err := e.pop()
		if err != nil {
			return err
		}
		pIdx, err := e.pop()
		if err != nil {
			return err
		}
		zone1 := &e.Zones[e.ZP1]
		zone0 := &e.Zones[e.ZP0]
		if e.RP0 < 0 || e.RP0 >= len(zone0.Points) || pIdx < 0 || int(pIdx) >= len(zone1.Points) {
			return fmt.Errorf("index out of bounds in MSIRP: rp0=%d, p=%d", e.RP0, pIdx)
		}
		if e.ZP1 == 0 {
			zone1.OriginalPoints[pIdx] = e.pointAlongFreedom(zone0.OriginalPoints[e.RP0], distance)
			zone1.Points[pIdx] = zone1.OriginalPoints[pIdx]
		}
		curDist := e.getProjectedDistance(zone0.Points[e.RP0], zone1.Points[pIdx])
		oldRP0 := e.RP0
		if err := e.moveProjected(&zone1.Points[pIdx], distance-curDist); err != nil {
			return err
		}
		e.touchCurrent(int(pIdx), e.ZP1)
		e.RP1 = oldRP0
		e.RP2 = int(pIdx)
		if opcode&0x01 != 0 {
			e.RP0 = int(pIdx)
		}
		e.IP++

	case opcode == 0x5D || opcode == 0x71 || opcode == 0x72: // DELTAP1, DELTAP2, DELTAP3
		n, err := e.pop()
		if err != nil {
			return err
		}
		if n < 0 {
			return fmt.Errorf("negative DELTAP count: %d", n)
		}
		zone := &e.Zones[e.ZP0]
		for i := 0; i < int(n); i++ {
			pIdx, err := e.pop()
			if err != nil {
				return err
			}
			arg, err := e.pop()
			if err != nil {
				return err
			}
			if e.GS.BackwardCompatibility {
				if e.iupXCalled && e.iupYCalled ||
					e.GS.FreeVector.Y == 0 ||
					int(pIdx) >= len(zone.Tags) ||
					zone.Tags[pIdx]&outlineTagTouchY == 0 {
					continue
				}
			}
			amount, apply := e.deltaAmount(opcode, arg)
			if !apply || amount == 0 || pIdx < 0 || int(pIdx) >= len(zone.Points) {
				continue
			}
			e.move(&zone.Points[pIdx], amount)
			e.touchCurrent(int(pIdx), e.ZP0)
		}
		e.IP++

	case opcode == 0x73 || opcode == 0x74 || opcode == 0x75: // DELTAC1, DELTAC2, DELTAC3
		n, err := e.pop()
		if err != nil {
			return err
		}
		if n < 0 {
			return fmt.Errorf("negative DELTAC count: %d", n)
		}
		for i := 0; i < int(n); i++ {
			cvtIdx, err := e.pop()
			if err != nil {
				return err
			}
			arg, err := e.pop()
			if err != nil {
				return err
			}
			amount, apply := e.deltaAmount(opcode, arg)
			if !apply || amount == 0 || cvtIdx < 0 || int(cvtIdx) >= len(e.CVT) {
				continue
			}
			e.CVT[cvtIdx] += amount
		}
		e.IP++

	case opcode == 0x80: // FLIPPT
		if e.GS.BackwardCompatibility && e.iupXCalled && e.iupYCalled {
			for i := 0; i < int(e.GS.Loop); i++ {
				if _, err := e.pop(); err != nil {
					return err
				}
			}
			e.GS.Loop = 1
			e.IP++
			break
		}
		zone := &e.Zones[e.ZP0]
		for i := 0; i < int(e.GS.Loop); i++ {
			pIdx, err := e.pop()
			if err != nil {
				return err
			}
			if pIdx < 0 || int(pIdx) >= len(zone.Points) {
				return fmt.Errorf("point index out of bounds in FLIPPT: %d", pIdx)
			}
			if int(pIdx) < len(zone.Tags) {
				zone.Tags[pIdx] ^= 0x01
			}
		}
		e.GS.Loop = 1
		e.IP++

	case opcode == 0x81 || opcode == 0x82: // FLIPRGON, FLIPRGOFF
		high, err := e.pop()
		if err != nil {
			return err
		}
		low, err := e.pop()
		if err != nil {
			return err
		}
		if e.GS.BackwardCompatibility && e.iupXCalled && e.iupYCalled {
			e.IP++
			break
		}
		zone := &e.Zones[e.ZP0]
		if low < 0 || high < 0 || int(low) >= len(zone.Points) || int(high) >= len(zone.Points) {
			return fmt.Errorf("point range out of bounds in FLIPRG: low=%d high=%d", low, high)
		}
		if low <= high {
			for i := int(low); i <= int(high); i++ {
				if i >= len(zone.Tags) {
					continue
				}
				if opcode == 0x81 {
					zone.Tags[i] |= 0x01
				} else {
					zone.Tags[i] &^= 0x01
				}
			}
		}
		e.IP++

	case opcode == 0x39: // IP
		zone2 := &e.Zones[e.ZP2]
		zone1 := &e.Zones[e.ZP1]
		zone0 := &e.Zones[e.ZP0]
		if e.RP1 < 0 || e.RP1 >= len(zone0.Points) || e.RP1 >= len(zone0.OriginalPoints) ||
			e.RP2 < 0 || e.RP2 >= len(zone1.Points) || e.RP2 >= len(zone1.OriginalPoints) {
			return fmt.Errorf("reference index out of bounds in IP: rp1=%d, rp2=%d", e.RP1, e.RP2)
		}
		twilight := e.ZP0 == 0 || e.ZP1 == 0 || e.ZP2 == 0
		oldRange := e.dualProjectIPDistance(zone0, e.RP1, zone1, e.RP2, twilight)
		curRange := e.getProjectedDistance(zone0.Points[e.RP1], zone1.Points[e.RP2])

		for i := 0; i < int(e.GS.Loop); i++ {
			pIdx, err := e.pop()
			if err != nil {
				return err
			}

			if pIdx < 0 || int(pIdx) >= len(zone2.Points) || int(pIdx) >= len(zone2.OriginalPoints) {
				return fmt.Errorf("index out of bounds in IP: rp1=%d, rp2=%d, p=%d", e.RP1, e.RP2, pIdx)
			}

			orgDist := e.dualProjectIPDistance(zone0, e.RP1, zone2, int(pIdx), twilight)
			curDist := e.getProjectedDistance(zone0.Points[e.RP1], zone2.Points[pIdx])
			var newDist int32
			if orgDist == 0 {
				newDist = 0
			} else if oldRange == 0 {
				newDist = orgDist
			} else {
				newDist = int32(divRound(int64(orgDist)*int64(curRange), int64(oldRange)))
			}

			if err := e.moveProjected(&zone2.Points[pIdx], newDist-curDist); err != nil {
				return err
			}
			e.touchCurrent(int(pIdx), e.ZP2)
		}
		e.GS.Loop = 1
		e.IP++

	case opcode == 0x30 || opcode == 0x31: // IUP[y], IUP[x]
		axis := int(opcode & 0x01)
		if e.GS.BackwardCompatibility {
			if e.iupXCalled && e.iupYCalled {
				e.IP++
				return nil
			}
			if axis == 1 {
				e.iupXCalled = true
			} else {
				e.iupYCalled = true
			}
		}
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
		if opID < 0 || opID > 0xFF {
			return fmt.Errorf("instruction opcode out of bounds in IDEF: %d", opID)
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
		if err := e.checkCallDepth(); err != nil {
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
		if err := e.checkLoopCallRepeat(count); err != nil {
			return err
		}
		if err := e.checkCallDepth(); err != nil {
			return err
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

	case opcode == 0x24: // DEPTH
		if err := e.push(int32(e.StackTop)); err != nil {
			return err
		}
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

	case opcode == 0x62: // DIV
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
		e.push(ftmath.Div26(a, b))
		e.IP++

	case opcode == 0x63: // MUL
		b, err := e.pop()
		if err != nil {
			return err
		}
		a, err := e.pop()
		if err != nil {
			return err
		}
		e.push(ftmath.Mul26(a, b))
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
		e.push(e.roundWithCompensation(a, e.engineCompensation(opcode)))
		e.IP++

	case opcode >= 0x6C && opcode <= 0x6F: // NROUND
		a, err := e.pop()
		if err != nil {
			return err
		}
		e.push(compensateDistance(a, e.engineCompensation(opcode)))
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

	case opcode == 0x70: // WCVTF
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
		e.CVT[idx] = e.scaleFUnits(val)
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

	case opcode == 0x51: // LTEQ
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

	case opcode == 0x52: // GT
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

	case opcode == 0x53: // GTEQ
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

	case opcode == 0x54: // EQ
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

	case opcode == 0x55: // NEQ
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

	case opcode == 0x56 || opcode == 0x57: // ODD, EVEN
		a, err := e.pop()
		if err != nil {
			return err
		}
		rounded := e.round(a)
		if opcode == 0x56 {
			if rounded&127 == 64 {
				if err := e.push(1); err != nil {
					return err
				}
			} else {
				if err := e.push(0); err != nil {
					return err
				}
			}
		} else {
			if rounded&127 == 0 {
				if err := e.push(1); err != nil {
					return err
				}
			} else {
				if err := e.push(0); err != nil {
					return err
				}
			}
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
		if instruction, ok := e.Instructions[int32(opcode)]; ok {
			if err := e.checkCallDepth(); err != nil {
				return err
			}
			e.CallStack = append(e.CallStack, CallRecord{
				ReturnIP:   e.IP + 1,
				ReturnCode: e.Code,
				Repeat:     1,
			})
			e.Code = instruction
			e.IP = 0
			return nil
		}
		return fmt.Errorf("reserved or undefined opcode 0x%02x", opcode)
	}
	return nil
}
