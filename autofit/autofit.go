package autofit

import (
	"github.com/dh-kam/freetype-go/api"
)

// Edge represents a collection of segments sharing a similar coordinate.
type Edge struct {
	Pos    int32
	Points []int // indices of points forming this edge
}

// Stem represents a pair of parallel edges.
type Stem struct {
	E1, E2 int // indices in the edges slice
	Width  int32
}

// HintingMetrics holds global font metrics calculated for auto-hinting.
type HintingMetrics struct {
	StandardWidth  int32
	StandardHeight int32
	BlueZones      []int32
}

// HintingData holds point-level hinting data for a specific glyph.
type HintingData struct {
	Edges []Edge
	Stems []Stem
}

// AutoHinter implements api.Hinter using a heuristic grid-fitting algorithm.
type AutoHinter struct {
	system  api.FreetypeSystem
	metrics HintingMetrics
}

// NewAutoHinter creates a new auto-hinter instance.
func NewAutoHinter(sys api.FreetypeSystem) api.Hinter {
	return &AutoHinter{
		system: sys,
		metrics: HintingMetrics{
			StandardWidth:  64, // 1 pixel default
			StandardHeight: 64, // 1 pixel default
		},
	}
}

func abs(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}

// Hint applies auto-hinting to the given outline.
// It detects horizontal and vertical edges, identifies stems, and snaps them to the pixel grid.
func (h *AutoHinter) Hint(outline api.Outline, size int32) error {
	points := outline.GetPoints()
	contours := outline.GetContours()
	if len(points) == 0 {
		return nil
	}

	// 1. Process Y-axis (Horizontal edges/stems)
	hEdges := h.detectEdges(points, contours, false)
	hStems := h.detectStems(hEdges)
	h.applySnapping(points, hEdges, hStems, false)

	// 2. Process X-axis (Vertical edges/stems)
	vEdges := h.detectEdges(points, contours, true)
	vStems := h.detectStems(vEdges)
	h.applySnapping(points, vEdges, vStems, true)

	return nil
}

// detectEdges identifies nearly horizontal or vertical segments.
func (h *AutoHinter) detectEdges(points []api.Vector, contours []int, vertical bool) []Edge {
	var edges []Edge
	start := 0
	for _, end := range contours {
		for i := start; i <= end; i++ {
			next := i + 1
			if i == end {
				next = start
			}

			p1 := points[i]
			p2 := points[next]

			if vertical {
				if abs(p1.X-p2.X) < 8 { // Nearly vertical (within 1/8th of a pixel)
					h.addEdge(&edges, (p1.X+p2.X)/2, i, next)
				}
			} else {
				if abs(p1.Y-p2.Y) < 8 { // Nearly horizontal
					h.addEdge(&edges, (p1.Y+p2.Y)/2, i, next)
				}
			}
		}
		start = end + 1
	}
	return edges
}

func (h *AutoHinter) addEdge(edges *[]Edge, pos int32, p1, p2 int) {
	for i := range *edges {
		if abs((*edges)[i].Pos-pos) < 32 { // Within 0.5 pixel
			(*edges)[i].Points = append((*edges)[i].Points, p1, p2)
			return
		}
	}
	*edges = append(*edges, Edge{Pos: pos, Points: []int{p1, p2}})
}

// detectStems identifies pairs of parallel edges that are likely to be stems.
func (h *AutoHinter) detectStems(edges []Edge) []Stem {
	var stems []Stem
	for i := 0; i < len(edges); i++ {
		for j := i + 1; j < len(edges); j++ {
			width := abs(edges[i].Pos - edges[j].Pos)
			// Heuristic: stems are usually between 0.5 and 4 pixels wide (32 to 256 units)
			if width >= 32 && width <= 256 {
				stems = append(stems, Stem{E1: i, E2: j, Width: width})
			}
		}
	}
	return stems
}

// applySnapping calculates and applies the snapped positions for edges and stems.
func (h *AutoHinter) applySnapping(points []api.Vector, edges []Edge, stems []Stem, vertical bool) {
	snappedEdges := make(map[int]int32)
	edgeUsedInStem := make(map[int]bool)

	// 1. Snap stems first to maintain consistent width
	for _, s := range stems {
		if edgeUsedInStem[s.E1] || edgeUsedInStem[s.E2] {
			continue
		}

		e1 := edges[s.E1]
		e2 := edges[s.E2]

		// Round first edge to grid
		newPos1 := (e1.Pos + 32) & ^63

		// Snap width to nearest standard width or pixel boundary
		stdWidth := h.metrics.StandardHeight
		if vertical {
			stdWidth = h.metrics.StandardWidth
		}

		snappedWidth := s.Width
		if abs(s.Width-stdWidth) < 32 {
			snappedWidth = stdWidth
		} else {
			snappedWidth = (s.Width + 32) & ^63
		}

		if snappedWidth < 64 {
			snappedWidth = 64
		}

		var newPos2 int32
		if e2.Pos > e1.Pos {
			newPos2 = newPos1 + snappedWidth
		} else {
			newPos2 = newPos1 - snappedWidth
		}

		snappedEdges[s.E1] = newPos1
		snappedEdges[s.E2] = newPos2
		edgeUsedInStem[s.E1] = true
		edgeUsedInStem[s.E2] = true
	}

	// 2. Snap remaining edges
	for i, e := range edges {
		if !edgeUsedInStem[i] {
			snappedEdges[i] = (e.Pos + 32) & ^63
		}
	}

	// 3. Apply changes to points
	pointMoved := make(map[int]bool)
	for i, newPos := range snappedEdges {
		for _, pIdx := range edges[i].Points {
			if pointMoved[pIdx] {
				continue
			}
			if vertical {
				points[pIdx].X = newPos
			} else {
				points[pIdx].Y = newPos
			}
			pointMoved[pIdx] = true
		}
	}
}
