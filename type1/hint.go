package type1

import (
	"math"

	"github.com/dh-kam/freetype-go/core"
	ftmath "github.com/dh-kam/freetype-go/math"
)

// HintContext contains decoded Type 1 hint data scaled for the current face
// size. Scaled values are 26.6 pixel units.
type HintContext struct {
	Stems   []ScaledStem
	Private HintPrivateContext
}

// ScaledStem is a decoded charstring stem with design-space and scaled edges.
type ScaledStem struct {
	Kind              CharStringHintKind
	Operator          string
	Orientation       CharStringStemOrientation
	Position          float64
	Width             float64
	DesignMin         int32
	DesignMax         int32
	ScaledMin         int32
	ScaledMax         int32
	ScaledWidth       int32
	PrivateWidthMatch bool
}

// HintPrivateContext contains scaled Private dictionary hint metadata.
type HintPrivateContext struct {
	StdHW            []ScaledHintMetric
	StdVW            []ScaledHintMetric
	StemSnapH        []ScaledHintMetric
	StemSnapV        []ScaledHintMetric
	BlueValues       []ScaledHintZone
	OtherBlues       []ScaledHintZone
	FamilyBlues      []ScaledHintZone
	FamilyOtherBlues []ScaledHintZone
	BlueScale        float64
	HasBlueScale     bool
	BlueShift        int
	HasBlueShift     bool
	BlueFuzz         int
	HasBlueFuzz      bool
	ForceBold        bool
	HasForceBold     bool
	LanguageGroup    int
	HasLanguageGroup bool
}

// ScaledHintMetric is a Private dictionary width metric scaled to 26.6 pixels.
type ScaledHintMetric struct {
	Design float64
	Scaled int32
}

// ScaledHintZone is a Private dictionary blue zone scaled to 26.6 pixels.
type ScaledHintZone struct {
	DesignMin float64
	DesignMax float64
	ScaledMin int32
	ScaledMax int32
}

// BuildHintContext converts decoded charstring hints and Private dictionary
// metadata into size-specific data that can be inspected or applied
// conservatively.
func BuildHintContext(font *Font, result *CharStringResult, xScale, yScale int32) HintContext {
	context := HintContext{
		Private: buildType1PrivateHintContext(font, xScale, yScale),
	}
	if result == nil {
		return context
	}
	for _, hint := range result.Hints {
		for _, stem := range hint.Stems {
			scaled, ok := buildType1ScaledStem(hint, stem, context.Private, xScale, yScale)
			if ok {
				context.Stems = append(context.Stems, scaled)
			}
		}
	}
	return context
}

// ApplyHintContext snaps scaled outline points that already lie exactly on a
// scaled simple stem edge. Face loading uses a stricter design-space point
// capture before scaling, but this helper is useful for tests and inspection.
func ApplyHintContext(outline *core.Outline, context HintContext) {
	if outline == nil || len(outline.Points) == 0 {
		return
	}
	xEdges, yEdges := type1HintScaledEdgeMaps(context)
	for i := range outline.Points {
		if snap, ok := xEdges[outline.Points[i].X]; ok {
			outline.Points[i].X = snap
		}
		if snap, ok := yEdges[outline.Points[i].Y]; ok {
			outline.Points[i].Y = snap
		}
	}
}

func buildType1PrivateHintContext(font *Font, xScale, yScale int32) HintPrivateContext {
	if font == nil {
		return HintPrivateContext{}
	}
	return HintPrivateContext{
		StdHW:            scaleType1HintMetrics(font.StdHW, yScale),
		StdVW:            scaleType1HintMetrics(font.StdVW, xScale),
		StemSnapH:        scaleType1HintMetrics(font.StemSnapH, yScale),
		StemSnapV:        scaleType1HintMetrics(font.StemSnapV, xScale),
		BlueValues:       scaleType1HintZones(font.BlueValues, yScale),
		OtherBlues:       scaleType1HintZones(font.OtherBlues, yScale),
		FamilyBlues:      scaleType1HintZones(font.FamilyBlues, yScale),
		FamilyOtherBlues: scaleType1HintZones(font.FamilyOtherBlues, yScale),
		BlueScale:        font.BlueScale,
		HasBlueScale:     font.HasBlueScale,
		BlueShift:        font.BlueShift,
		HasBlueShift:     font.HasBlueShift,
		BlueFuzz:         font.BlueFuzz,
		HasBlueFuzz:      font.HasBlueFuzz,
		ForceBold:        font.ForceBold,
		HasForceBold:     font.HasForceBold,
		LanguageGroup:    font.LanguageGroup,
		HasLanguageGroup: font.HasLanguageGroup,
	}
}

func scaleType1HintMetrics(values []float64, scale int32) []ScaledHintMetric {
	if len(values) == 0 {
		return nil
	}
	metrics := make([]ScaledHintMetric, 0, len(values))
	for _, value := range values {
		design, ok := type1HintDesignTo26Dot6(value)
		if !ok {
			continue
		}
		metrics = append(metrics, ScaledHintMetric{
			Design: value,
			Scaled: type1HintScale26Dot6(design, scale),
		})
	}
	return metrics
}

func scaleType1HintZones(values []float64, scale int32) []ScaledHintZone {
	if len(values) < 2 {
		return nil
	}
	zones := make([]ScaledHintZone, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		minDesign := values[i]
		maxDesign := values[i+1]
		min, okMin := type1HintDesignTo26Dot6(minDesign)
		max, okMax := type1HintDesignTo26Dot6(maxDesign)
		if !okMin || !okMax {
			continue
		}
		scaledMin := type1HintScale26Dot6(min, scale)
		scaledMax := type1HintScale26Dot6(max, scale)
		if minDesign > maxDesign {
			minDesign, maxDesign = maxDesign, minDesign
		}
		if scaledMin > scaledMax {
			scaledMin, scaledMax = scaledMax, scaledMin
		}
		zones = append(zones, ScaledHintZone{
			DesignMin: minDesign,
			DesignMax: maxDesign,
			ScaledMin: scaledMin,
			ScaledMax: scaledMax,
		})
	}
	return zones
}

func buildType1ScaledStem(hint CharStringHint, stem CharStringStemHint, private HintPrivateContext, xScale, yScale int32) (ScaledStem, bool) {
	if !type1HintFinite(stem.Position) || !type1HintFinite(stem.Width) || !type1HintFinite(stem.Position+stem.Width) {
		return ScaledStem{}, false
	}
	edge0, ok0 := type1HintDesignTo26Dot6(stem.Position)
	edge1, ok1 := type1HintDesignTo26Dot6(stem.Position + stem.Width)
	if !ok0 || !ok1 {
		return ScaledStem{}, false
	}
	designMin, designMax := edge0, edge1
	if designMin > designMax {
		designMin, designMax = designMax, designMin
	}
	scale := yScale
	if stem.Orientation == CharStringStemVertical {
		scale = xScale
	}
	scaledMin := type1HintScale26Dot6(designMin, scale)
	scaledMax := type1HintScale26Dot6(designMax, scale)
	if scaledMin > scaledMax {
		scaledMin, scaledMax = scaledMax, scaledMin
	}
	return ScaledStem{
		Kind:              hint.Kind,
		Operator:          hint.Operator,
		Orientation:       stem.Orientation,
		Position:          stem.Position,
		Width:             stem.Width,
		DesignMin:         designMin,
		DesignMax:         designMax,
		ScaledMin:         scaledMin,
		ScaledMax:         scaledMax,
		ScaledWidth:       scaledMax - scaledMin,
		PrivateWidthMatch: type1HintPrivateWidthMatch(private, stem.Orientation, stem.Width),
	}, true
}

type type1HintPointSnap struct {
	index int
	x     int32
	y     int32
	hasX  bool
	hasY  bool
}

func type1HintPointSnaps(outline *core.Outline, context HintContext) []type1HintPointSnap {
	if outline == nil || len(outline.Points) == 0 {
		return nil
	}
	xEdges, yEdges := type1HintDesignEdgeMaps(context)
	if len(xEdges) == 0 && len(yEdges) == 0 {
		return nil
	}
	snaps := make([]type1HintPointSnap, 0, len(outline.Points))
	for i, point := range outline.Points {
		snap := type1HintPointSnap{index: i}
		if x, ok := xEdges[point.X]; ok {
			snap.x = x
			snap.hasX = true
		}
		if y, ok := yEdges[point.Y]; ok {
			snap.y = y
			snap.hasY = true
		}
		if snap.hasX || snap.hasY {
			snaps = append(snaps, snap)
		}
	}
	return snaps
}

func applyType1HintPointSnaps(outline *core.Outline, snaps []type1HintPointSnap) {
	if outline == nil || len(snaps) == 0 {
		return
	}
	for _, snap := range snaps {
		if snap.index < 0 || snap.index >= len(outline.Points) {
			continue
		}
		if snap.hasX {
			outline.Points[snap.index].X = snap.x
		}
		if snap.hasY {
			outline.Points[snap.index].Y = snap.y
		}
	}
}

func type1HintDesignEdgeMaps(context HintContext) (map[int32]int32, map[int32]int32) {
	var xEdges map[int32]int32
	var yEdges map[int32]int32
	for _, stem := range context.Stems {
		minSnap, maxSnap, ok := type1HintStemSnaps(stem, context.Private)
		if !ok {
			continue
		}
		if stem.Orientation == CharStringStemVertical {
			if xEdges == nil {
				xEdges = make(map[int32]int32)
			}
			xEdges[stem.DesignMin] = minSnap
			xEdges[stem.DesignMax] = maxSnap
			continue
		}
		if yEdges == nil {
			yEdges = make(map[int32]int32)
		}
		yEdges[stem.DesignMin] = minSnap
		yEdges[stem.DesignMax] = maxSnap
	}
	return xEdges, yEdges
}

func type1HintScaledEdgeMaps(context HintContext) (map[int32]int32, map[int32]int32) {
	var xEdges map[int32]int32
	var yEdges map[int32]int32
	for _, stem := range context.Stems {
		minSnap, maxSnap, ok := type1HintStemSnaps(stem, context.Private)
		if !ok {
			continue
		}
		if stem.Orientation == CharStringStemVertical {
			if xEdges == nil {
				xEdges = make(map[int32]int32)
			}
			xEdges[stem.ScaledMin] = minSnap
			xEdges[stem.ScaledMax] = maxSnap
			continue
		}
		if yEdges == nil {
			yEdges = make(map[int32]int32)
		}
		yEdges[stem.ScaledMin] = minSnap
		yEdges[stem.ScaledMax] = maxSnap
	}
	return xEdges, yEdges
}

func type1HintStemSnaps(stem ScaledStem, private HintPrivateContext) (int32, int32, bool) {
	if stem.Kind != CharStringHintStem || stem.DesignMin == stem.DesignMax || stem.ScaledMin == stem.ScaledMax {
		return 0, 0, false
	}
	if type1HintHasPrivateWidths(private, stem.Orientation) && !stem.PrivateWidthMatch {
		return 0, 0, false
	}
	minSnap := type1HintRoundToPixel(stem.ScaledMin)
	maxSnap := type1HintRoundToPixel(stem.ScaledMax)
	if minSnap == maxSnap {
		return 0, 0, false
	}
	if !type1HintSnapDistanceOK(stem.ScaledMin, minSnap) || !type1HintSnapDistanceOK(stem.ScaledMax, maxSnap) {
		return 0, 0, false
	}
	return minSnap, maxSnap, true
}

func type1HintPrivateWidthMatch(private HintPrivateContext, orientation CharStringStemOrientation, width float64) bool {
	standard, snap := type1HintPrivateWidthSlices(private, orientation)
	return type1HintWidthMatchesMetric(standard, width) || type1HintWidthMatchesMetric(snap, width)
}

func type1HintHasPrivateWidths(private HintPrivateContext, orientation CharStringStemOrientation) bool {
	standard, snap := type1HintPrivateWidthSlices(private, orientation)
	return len(standard) > 0 || len(snap) > 0
}

func type1HintPrivateWidthSlices(private HintPrivateContext, orientation CharStringStemOrientation) ([]ScaledHintMetric, []ScaledHintMetric) {
	if orientation == CharStringStemVertical {
		return private.StdVW, private.StemSnapV
	}
	return private.StdHW, private.StemSnapH
}

func type1HintWidthMatchesMetric(metrics []ScaledHintMetric, width float64) bool {
	for _, metric := range metrics {
		if math.Abs(math.Abs(width)-math.Abs(metric.Design)) <= 1e-6 {
			return true
		}
	}
	return false
}

func type1HintDesignTo26Dot6(value float64) (int32, bool) {
	if !type1HintFinite(value) {
		return 0, false
	}
	scaled := value * 64
	const (
		minInt32 = -1 << 31
		maxInt32 = 1<<31 - 1
	)
	if scaled < minInt32 || scaled > maxInt32 {
		return 0, false
	}
	return int32(scaled), true
}

func type1HintScale26Dot6(value, scale int32) int32 {
	return ftmath.MulFix(type1Design26Dot6ToUnits(value), scale)
}

func type1HintRoundToPixel(value int32) int32 {
	return int32(math.Round(float64(value)/64) * 64)
}

func type1HintSnapDistanceOK(value, snap int32) bool {
	diff := int64(value) - int64(snap)
	if diff < 0 {
		diff = -diff
	}
	return diff <= 32
}

func type1HintFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
