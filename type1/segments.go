package type1

import (
	"math"

	"github.com/dh-kam/freetype-go/api"
)

// CharStringSegmentUnits names the coordinate system used by
// CharStringResult.Segments.
const CharStringSegmentUnits = "26.6 design units"

// SegmentPoint is a JSON-friendly point in Type 1 native segment coordinates.
// Coordinates are 26.6 design units.
type SegmentPoint struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
}

// SegmentRecord is a JSON-friendly representation of a CharStringSegment.
// Points contain one endpoint for move/line segments and three points for
// cubic segments: control 1, control 2, endpoint. Close segments have no
// points. Coordinates are 26.6 design units.
type SegmentRecord struct {
	Kind   string         `json:"kind"`
	Units  string         `json:"units"`
	Points []SegmentPoint `json:"points,omitempty"`
}

// String returns the stable inspection name for a native Type 1 segment kind.
func (k CharStringSegmentKind) String() string {
	switch k {
	case CharStringSegmentMove:
		return "move"
	case CharStringSegmentLine:
		return "line"
	case CharStringSegmentCubic:
		return "cubic"
	case CharStringSegmentClose:
		return "close"
	default:
		return "unknown"
	}
}

// SegmentKindCounts counts native Type 1 segment kinds in a segment stream.
func SegmentKindCounts(segments []CharStringSegment) map[CharStringSegmentKind]int {
	counts := make(map[CharStringSegmentKind]int)
	for _, segment := range segments {
		counts[segment.Kind]++
	}
	return counts
}

// SegmentBounds returns geometric bounds for native Type 1 segments in 26.6
// design units. Cubic extrema are included, and returned integer bounds are
// rounded outward. ok is false when no point-bearing segments are present.
func SegmentBounds(segments []CharStringSegment) (min, max api.Vector, ok bool) {
	var bounds segmentBounds
	var current, contourStart api.Vector
	haveCurrent := false

	for _, segment := range segments {
		switch segment.Kind {
		case CharStringSegmentMove:
			p := segment.Points[0]
			bounds.addPoint(p)
			current = p
			contourStart = p
			haveCurrent = true
		case CharStringSegmentLine:
			p := segment.Points[0]
			if haveCurrent {
				bounds.addPoint(current)
			} else {
				contourStart = p
				haveCurrent = true
			}
			bounds.addPoint(p)
			current = p
		case CharStringSegmentCubic:
			p1, p2, p3 := segment.Points[0], segment.Points[1], segment.Points[2]
			if haveCurrent {
				bounds.addCubic(current, p1, p2, p3)
			} else {
				bounds.addPoint(p1)
				bounds.addPoint(p2)
				bounds.addPoint(p3)
				contourStart = p3
				haveCurrent = true
			}
			current = p3
		case CharStringSegmentClose:
			if haveCurrent {
				bounds.addPoint(current)
				bounds.addPoint(contourStart)
				current = contourStart
			}
		}
	}

	if !bounds.ok {
		return api.Vector{}, api.Vector{}, false
	}
	return bounds.minVector(), bounds.maxVector(), true
}

// SegmentsToPolyline flattens native Type 1 segments to points in 26.6 design
// units. cubicSteps controls the number of line endpoints emitted per cubic;
// values less than one are treated as one. Cubic endpoints are copied exactly.
func SegmentsToPolyline(segments []CharStringSegment, cubicSteps int) []api.Vector {
	if cubicSteps < 1 {
		cubicSteps = 1
	}

	polyline := make([]api.Vector, 0, len(segments))
	var current, contourStart api.Vector
	haveCurrent := false

	for _, segment := range segments {
		switch segment.Kind {
		case CharStringSegmentMove:
			p := segment.Points[0]
			polyline = append(polyline, p)
			current = p
			contourStart = p
			haveCurrent = true
		case CharStringSegmentLine:
			p := segment.Points[0]
			polyline = append(polyline, p)
			if !haveCurrent {
				contourStart = p
				haveCurrent = true
			}
			current = p
		case CharStringSegmentCubic:
			p1, p2, p3 := segment.Points[0], segment.Points[1], segment.Points[2]
			if !haveCurrent {
				polyline = append(polyline, p3)
				contourStart = p3
				current = p3
				haveCurrent = true
				break
			}
			for step := 1; step <= cubicSteps; step++ {
				if step == cubicSteps {
					polyline = append(polyline, p3)
					continue
				}
				t := float64(step) / float64(cubicSteps)
				x, y := cubicPoint(current, p1, p2, p3, t)
				polyline = append(polyline, api.Vector{
					X: roundFixed(x),
					Y: roundFixed(y),
				})
			}
			current = p3
		case CharStringSegmentClose:
			if haveCurrent {
				if current != contourStart {
					polyline = append(polyline, contourStart)
				}
				current = contourStart
			}
		}
	}

	return polyline
}

// SegmentsToRecords converts native Type 1 segments to JSON-friendly records.
// Coordinates are preserved as 26.6 design units.
func SegmentsToRecords(segments []CharStringSegment) []SegmentRecord {
	records := make([]SegmentRecord, 0, len(segments))
	for _, segment := range segments {
		record := SegmentRecord{
			Kind:  segment.Kind.String(),
			Units: CharStringSegmentUnits,
		}
		switch segment.Kind {
		case CharStringSegmentMove, CharStringSegmentLine:
			record.Points = []SegmentPoint{newSegmentPoint(segment.Points[0])}
		case CharStringSegmentCubic:
			record.Points = []SegmentPoint{
				newSegmentPoint(segment.Points[0]),
				newSegmentPoint(segment.Points[1]),
				newSegmentPoint(segment.Points[2]),
			}
		}
		records = append(records, record)
	}
	return records
}

type segmentBounds struct {
	minX, minY float64
	maxX, maxY float64
	ok         bool
}

func (b *segmentBounds) addPoint(p api.Vector) {
	b.addFloat(float64(p.X), float64(p.Y))
}

func (b *segmentBounds) addFloat(x, y float64) {
	if !b.ok {
		b.minX, b.maxX = x, x
		b.minY, b.maxY = y, y
		b.ok = true
		return
	}
	b.minX = math.Min(b.minX, x)
	b.minY = math.Min(b.minY, y)
	b.maxX = math.Max(b.maxX, x)
	b.maxY = math.Max(b.maxY, y)
}

func (b *segmentBounds) addCubic(p0, p1, p2, p3 api.Vector) {
	b.addPoint(p0)
	b.addPoint(p3)
	for _, t := range cubicExtrema(float64(p0.X), float64(p1.X), float64(p2.X), float64(p3.X)) {
		x, y := cubicPoint(p0, p1, p2, p3, t)
		b.addFloat(x, y)
	}
	for _, t := range cubicExtrema(float64(p0.Y), float64(p1.Y), float64(p2.Y), float64(p3.Y)) {
		x, y := cubicPoint(p0, p1, p2, p3, t)
		b.addFloat(x, y)
	}
}

func (b segmentBounds) minVector() api.Vector {
	return api.Vector{
		X: floorFixed(b.minX),
		Y: floorFixed(b.minY),
	}
}

func (b segmentBounds) maxVector() api.Vector {
	return api.Vector{
		X: ceilFixed(b.maxX),
		Y: ceilFixed(b.maxY),
	}
}

func newSegmentPoint(p api.Vector) SegmentPoint {
	return SegmentPoint{X: p.X, Y: p.Y}
}

func cubicExtrema(p0, p1, p2, p3 float64) []float64 {
	const epsilon = 1e-9

	a := -p0 + 3*p1 - 3*p2 + p3
	b := 2 * (p0 - 2*p1 + p2)
	c := p1 - p0

	if math.Abs(a) < epsilon {
		if math.Abs(b) < epsilon {
			return nil
		}
		t := -c / b
		if t > 0 && t < 1 {
			return []float64{t}
		}
		return nil
	}

	discriminant := b*b - 4*a*c
	if discriminant < -epsilon {
		return nil
	}
	if discriminant < 0 {
		discriminant = 0
	}

	sqrtDiscriminant := math.Sqrt(discriminant)
	t1 := (-b + sqrtDiscriminant) / (2 * a)
	t2 := (-b - sqrtDiscriminant) / (2 * a)

	roots := make([]float64, 0, 2)
	if t1 > 0 && t1 < 1 {
		roots = append(roots, t1)
	}
	if math.Abs(t2-t1) > epsilon && t2 > 0 && t2 < 1 {
		roots = append(roots, t2)
	}
	return roots
}

func cubicPoint(p0, p1, p2, p3 api.Vector, t float64) (x, y float64) {
	return cubicCoordinate(float64(p0.X), float64(p1.X), float64(p2.X), float64(p3.X), t),
		cubicCoordinate(float64(p0.Y), float64(p1.Y), float64(p2.Y), float64(p3.Y), t)
}

func cubicCoordinate(p0, p1, p2, p3, t float64) float64 {
	mt := 1 - t
	return mt*mt*mt*p0 + 3*mt*mt*t*p1 + 3*mt*t*t*p2 + t*t*t*p3
}

func floorFixed(v float64) int32 {
	return int32(math.Floor(v + 1e-9))
}

func ceilFixed(v float64) int32 {
	return int32(math.Ceil(v - 1e-9))
}

func roundFixed(v float64) int32 {
	return int32(math.Round(v))
}
