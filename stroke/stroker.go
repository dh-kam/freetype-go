package stroke

import (
	"math"
	"sort"

	"github.com/dh-kam/freetype-go/api"
	ftgomath "github.com/dh-kam/freetype-go/math"
)

type LineCap int

const (
	LineCapButt LineCap = iota
	LineCapRound
	LineCapSquare
)

type LineJoin int

const (
	LineJoinRound LineJoin = iota
	LineJoinBevel
	LineJoinMiter
)

type Stroker struct {
	radius     int32
	lineCap    LineCap
	lineJoin   LineJoin
	miterLimit float64
}

type Outline struct {
	Points   []api.Vector
	Tags     []byte
	Contours []int
}

func (o *Outline) GetPoints() []api.Vector { return o.Points }
func (o *Outline) GetTags() []byte         { return o.Tags }
func (o *Outline) GetContours() []int      { return o.Contours }
func (o *Outline) Scale(xScale, yScale int32) {
	for i := range o.Points {
		o.Points[i].X = int32((int64(o.Points[i].X) * int64(xScale)) >> 16)
		o.Points[i].Y = int32((int64(o.Points[i].Y) * int64(yScale)) >> 16)
	}
}

func (o *Outline) Translate(x, y int32) {
	for i := range o.Points {
		o.Points[i].X += x
		o.Points[i].Y += y
	}
}

func (o *Outline) Transform(m *api.Matrix) {
	if m == nil {
		return
	}
	for i := range o.Points {
		x := o.Points[i].X
		y := o.Points[i].Y
		o.Points[i].X = int32((int64(x)*int64(m.XX) + int64(y)*int64(m.XY)) >> 16)
		o.Points[i].Y = int32((int64(x)*int64(m.YX) + int64(y)*int64(m.YY)) >> 16)
	}
}

func NewStroker() *Stroker {
	return &Stroker{
		radius:     64,
		lineCap:    LineCapButt,
		lineJoin:   LineJoinMiter,
		miterLimit: 10.0,
	}
}

func (s *Stroker) SetRadius(radius int32) { s.radius = radius }
func (s *Stroker) SetCaps(cap LineCap)    { s.lineCap = cap }
func (s *Stroker) SetJoins(join LineJoin) { s.lineJoin = join }

func (s *Stroker) Stroke(outline api.Outline, radius int32) (*Outline, error) {
	if outline == nil {
		return &Outline{}, nil
	}

	s.radius = radius
	points := outline.GetPoints()
	contours := outline.GetContours()

	result := &Outline{}

	startIdx := 0
	for _, endIdx := range contours {
		if endIdx < startIdx {
			continue
		}
		cPoints := points[startIdx : endIdx+1]
		s.strokeContour(cPoints, result)
		startIdx = endIdx + 1
	}

	result = clipOutline(result)
	return result, nil
}

func clipOutline(outline *Outline) *Outline {
	if len(outline.Points) < 3 {
		return outline
	}

	type Edge struct {
		p1, p2 api.Vector
	}
	var edges []Edge

	startIdx := 0
	for _, endIdx := range outline.Contours {
		if endIdx < startIdx {
			continue
		}
		for i := startIdx; i <= endIdx; i++ {
			p1 := outline.Points[i]
			p2 := outline.Points[startIdx]
			if i < endIdx {
				p2 = outline.Points[i+1]
			}
			if p1 != p2 {
				edges = append(edges, Edge{p1, p2})
			}
		}
		startIdx = endIdx + 1
	}

	if len(edges) == 0 {
		return outline
	}

	type IntPoint struct {
		t float64
		p api.Vector
	}
	edgeInts := make([][]IntPoint, len(edges))
	for i := range edges {
		edgeInts[i] = append(edgeInts[i], IntPoint{0.0, edges[i].p1})
		edgeInts[i] = append(edgeInts[i], IntPoint{1.0, edges[i].p2})
	}

	for i := 0; i < len(edges); i++ {
		for j := i + 1; j < len(edges); j++ {
			e1 := edges[i]
			e2 := edges[j]

			P := e1.p1
			E := api.Vector{X: e1.p2.X - e1.p1.X, Y: e1.p2.Y - e1.p1.Y}
			Q := e2.p1
			F := api.Vector{X: e2.p2.X - e2.p1.X, Y: e2.p2.Y - e2.p1.Y}

			denom := ftgomath.Mul26(E.Y, F.X) - ftgomath.Mul26(E.X, F.Y)
			if denom == 0 {
				continue
			}
			t_num := ftgomath.Mul26(Q.Y-P.Y, F.X) - ftgomath.Mul26(Q.X-P.X, F.Y)
			u_num := ftgomath.Mul26(Q.Y-P.Y, E.X) - ftgomath.Mul26(Q.X-P.X, E.Y)

			t := float64(t_num) / float64(denom)
			u := float64(u_num) / float64(denom)

			if t > 1e-5 && t < 1.0-1e-5 && u > 1e-5 && u < 1.0-1e-5 {
				ix := P.X + int32(float64(E.X)*t)
				iy := P.Y + int32(float64(E.Y)*t)
				pt := api.Vector{X: ix, Y: iy}
				edgeInts[i] = append(edgeInts[i], IntPoint{t, pt})
				edgeInts[j] = append(edgeInts[j], IntPoint{u, pt})
			}
		}
	}

	var allPoints []api.Vector
	getPointID := func(p api.Vector) int {
		for i, ap := range allPoints {
			if math.Abs(float64(ap.X-p.X)) <= 1 && math.Abs(float64(ap.Y-p.Y)) <= 1 {
				return i
			}
		}
		allPoints = append(allPoints, p)
		return len(allPoints) - 1
	}

	type SubSegment struct {
		p1, p2 int
	}
	var subSegments []SubSegment

	for i := range edges {
		ints := edgeInts[i]
		sort.Slice(ints, func(a, b int) bool {
			return ints[a].t < ints[b].t
		})
		var pIDs []int
		for _, in := range ints {
			id := getPointID(in.p)
			if len(pIDs) == 0 || pIDs[len(pIDs)-1] != id {
				pIDs = append(pIDs, id)
			}
		}
		for k := 0; k < len(pIDs)-1; k++ {
			subSegments = append(subSegments, SubSegment{pIDs[k], pIDs[k+1]})
		}
	}

	isLeft := func(p1, p2 api.Vector, x, y int32) int32 {
		dx1 := p2.X - p1.X
		dy1 := y - p1.Y
		dx2 := x - p1.X
		dy2 := p2.Y - p1.Y
		return ftgomath.Mul26(dx1, dy1) - ftgomath.Mul26(dx2, dy2)
	}

	calcWinding := func(x, y int32) int {
		winding := 0
		for _, e := range edges {
			e1y := e.p1.Y
			e2y := e.p2.Y
			if e1y <= y {
				if e2y > y {
					if isLeft(e.p1, e.p2, x, y) > 0 {
						winding++
					}
				}
			} else {
				if e2y <= y {
					if isLeft(e.p1, e.p2, x, y) < 0 {
						winding--
					}
				}
			}
		}
		return winding
	}

	var validSegments []SubSegment
	for _, seg := range subSegments {
		p1 := allPoints[seg.p1]
		p2 := allPoints[seg.p2]

		midX := (p1.X + p2.X) / 2
		midY := (p1.Y + p2.Y) / 2

		dx := p2.X - p1.X
		dy := p2.Y - p1.Y
		l := math.Hypot(float64(dx), float64(dy))
		if l == 0 {
			continue
		}
		nx := -float64(dy) / l
		ny := float64(dx) / l

		eps := 4.0 // 4/64th of a pixel offset
		pLeftX := midX + int32(nx*eps)
		pLeftY := midY + int32(ny*eps)
		pRightX := midX - int32(nx*eps)
		pRightY := midY - int32(ny*eps)

		wLeft := calcWinding(pLeftX, pLeftY)
		wRight := calcWinding(pRightX, pRightY)

		if wLeft != 0 && wRight == 0 {
			validSegments = append(validSegments, seg)
		} else if wLeft == 0 && wRight != 0 {
			validSegments = append(validSegments, SubSegment{seg.p2, seg.p1})
		}
	}

	result := &Outline{}
	used := make([]bool, len(validSegments))

	for {
		startSeg := -1
		for i, u := range used {
			if !u {
				startSeg = i
				break
			}
		}
		if startSeg == -1 {
			break
		}

		currSeg := startSeg
		used[currSeg] = true
		startID := validSegments[currSeg].p1
		currID := validSegments[currSeg].p2

		contourPoints := []api.Vector{allPoints[startID]}

		for {
			nextSeg := -1
			for i, u := range used {
				if !u && validSegments[i].p1 == currID {
					nextSeg = i
					break
				}
			}
			if nextSeg == -1 {
				break
			}
			used[nextSeg] = true
			contourPoints = append(contourPoints, allPoints[validSegments[nextSeg].p1])
			currID = validSegments[nextSeg].p2
			if currID == startID {
				break
			}
		}

		if len(contourPoints) >= 3 {
			newStartIdx := len(result.Points)
			result.Points = append(result.Points, contourPoints...)
			for i := newStartIdx; i < len(result.Points); i++ {
				result.Tags = append(result.Tags, 1)
			}
			result.Contours = append(result.Contours, len(result.Points)-1)
		}
	}

	return result
}

func (s *Stroker) strokeContour(points []api.Vector, result *Outline) {
	if len(points) < 2 {
		return
	}

	isClosed := points[0] == points[len(points)-1]

	var leftPoints []api.Vector
	var rightPoints []api.Vector

	type segment struct {
		p1, p2 api.Vector
		nx, ny float64 // Normal
	}

	var segments []segment
	for i := 0; i < len(points)-1; i++ {
		p1, p2 := points[i], points[i+1]
		dx, dy := float64(p2.X-p1.X), float64(p2.Y-p1.Y)
		l := math.Hypot(dx, dy)
		if l == 0 {
			continue
		}
		segments = append(segments, segment{
			p1: p1, p2: p2,
			nx: -dy / l, ny: dx / l,
		})
	}

	if len(segments) == 0 {
		return
	}

	r := float64(s.radius)

	for i := 0; i < len(segments); i++ {
		seg := segments[i]
		offX, offY := int32(seg.nx*r), int32(seg.ny*r)

		lp1 := api.Vector{X: seg.p1.X + offX, Y: seg.p1.Y + offY}
		lp2 := api.Vector{X: seg.p2.X + offX, Y: seg.p2.Y + offY}
		rp1 := api.Vector{X: seg.p1.X - offX, Y: seg.p1.Y - offY}
		rp2 := api.Vector{X: seg.p2.X - offX, Y: seg.p2.Y - offY}

		if i > 0 {
			prev := segments[i-1]
			s.addJoin(&leftPoints, prev.p2, prev.nx, prev.ny, seg.nx, seg.ny, true)
			s.addJoin(&rightPoints, prev.p2, -prev.nx, -prev.ny, -seg.nx, -seg.ny, false)
		} else if !isClosed {
			s.addCap(&leftPoints, &rightPoints, seg.p1, seg.nx, seg.ny, true)
		}

		leftPoints = append(leftPoints, lp1, lp2)
		rightPoints = append(rightPoints, rp1, rp2)
	}

	if !isClosed {
		last := segments[len(segments)-1]
		s.addCap(&leftPoints, &rightPoints, last.p2, last.nx, last.ny, false)
	} else {
		first, last := segments[0], segments[len(segments)-1]
		s.addJoin(&leftPoints, first.p1, last.nx, last.ny, first.nx, first.ny, true)
		s.addJoin(&rightPoints, first.p1, -last.nx, -last.ny, -first.nx, -first.ny, false)
	}

	newStartIdx := len(result.Points)
	result.Points = append(result.Points, leftPoints...)
	for i := len(rightPoints) - 1; i >= 0; i-- {
		result.Points = append(result.Points, rightPoints[i])
	}

	for i := newStartIdx; i < len(result.Points); i++ {
		result.Tags = append(result.Tags, 1)
	}
	result.Contours = append(result.Contours, len(result.Points)-1)
}

func (s *Stroker) addJoin(pts *[]api.Vector, p api.Vector, nx1, ny1, nx2, ny2 float64, left bool) {
	dot := nx1*nx2 + ny1*ny2
	if dot > 0.999 {
		return
	}

	r := float64(s.radius)

	switch s.lineJoin {
	case LineJoinBevel:
	case LineJoinMiter:
	case LineJoinRound:
		numSteps := 3
		angle1 := math.Atan2(ny1, nx1)
		angle2 := math.Atan2(ny2, nx2)
		da := angle2 - angle1
		if da > math.Pi {
			da -= 2 * math.Pi
		}
		if da < -math.Pi {
			da += 2 * math.Pi
		}

		for i := 1; i < numSteps; i++ {
			a := angle1 + da*float64(i)/float64(numSteps)
			*pts = append(*pts, api.Vector{
				X: p.X + int32(math.Cos(a)*r),
				Y: p.Y + int32(math.Sin(a)*r),
			})
		}
	}
}

func (s *Stroker) addCap(leftPts, rightPts *[]api.Vector, p api.Vector, nx, ny float64, start bool) {
	r := float64(s.radius)
	if s.lineCap == LineCapRound {
		numSteps := 5
		baseAngle := math.Atan2(ny, nx)
		startAngle := baseAngle
		da := math.Pi
		if start {
			startAngle = baseAngle + math.Pi
		}

		for i := 1; i < numSteps; i++ {
			a := startAngle + da*float64(i)/float64(numSteps)
			v := api.Vector{
				X: p.X + int32(math.Cos(a)*r),
				Y: p.Y + int32(math.Sin(a)*r),
			}
			if start {
				*leftPts = append(*leftPts, v)
			} else {
				*rightPts = append(*rightPts, v)
			}
		}
	}
}
