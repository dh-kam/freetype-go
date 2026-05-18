package type1

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestSegmentUtilitiesCountBoundsAndRecords(t *testing.T) {
	segments := segmentUtilityFixture()

	counts := SegmentKindCounts(segments)
	for _, kind := range []CharStringSegmentKind{
		CharStringSegmentMove,
		CharStringSegmentLine,
		CharStringSegmentCubic,
		CharStringSegmentClose,
	} {
		if got, want := counts[kind], 1; got != want {
			t.Fatalf("count[%s] = %d, want %d", kind, got, want)
		}
	}

	min, max, ok := SegmentBounds(segments)
	if !ok {
		t.Fatal("SegmentBounds returned ok=false")
	}
	if min != (api.Vector{}) {
		t.Fatalf("min = %+v, want zero", min)
	}
	// The cubic control points reach y=128, but the curve's true extremum is
	// y=96 in 26.6 design units.
	if want := (api.Vector{X: 128, Y: 96}); max != want {
		t.Fatalf("max = %+v, want %+v", max, want)
	}

	_, _, ok = SegmentBounds(nil)
	if ok {
		t.Fatal("SegmentBounds(nil) returned ok=true")
	}

	records := SegmentsToRecords(segments)
	if got, want := len(records), len(segments); got != want {
		t.Fatalf("record count = %d, want %d", got, want)
	}
	if records[2].Kind != "cubic" || records[2].Units != CharStringSegmentUnits {
		t.Fatalf("cubic record metadata = %+v", records[2])
	}
	if got, want := records[2].Points, []SegmentPoint{{X: 64, Y: 128}, {X: 128, Y: 128}, {X: 128, Y: 0}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cubic record points = %+v, want %+v", got, want)
	}
	if _, err := json.Marshal(records); err != nil {
		t.Fatalf("records did not marshal to JSON: %v", err)
	}
}

func TestSegmentsToPolylineFlattensCubicAndPreservesEndpoints(t *testing.T) {
	segments := segmentUtilityFixture()

	got := SegmentsToPolyline(segments, 4)
	want := []api.Vector{
		{},
		{X: 64, Y: 0},
		{X: 74, Y: 72},
		{X: 96, Y: 96},
		{X: 118, Y: 72},
		{X: 128, Y: 0},
		{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("polyline = %+v, want %+v", got, want)
	}
	if gotEndpoint, wantEndpoint := got[len(got)-2], segments[2].Points[2]; gotEndpoint != wantEndpoint {
		t.Fatalf("flattened cubic endpoint = %+v, want exact endpoint %+v", gotEndpoint, wantEndpoint)
	}
	if gotClose, wantClose := got[len(got)-1], segments[0].Points[0]; gotClose != wantClose {
		t.Fatalf("close endpoint = %+v, want contour start %+v", gotClose, wantClose)
	}
}

func segmentUtilityFixture() []CharStringSegment {
	return []CharStringSegment{
		{Kind: CharStringSegmentMove, Points: [3]api.Vector{{}}},
		{Kind: CharStringSegmentLine, Points: [3]api.Vector{{X: 64, Y: 0}}},
		{Kind: CharStringSegmentCubic, Points: [3]api.Vector{
			{X: 64, Y: 128},
			{X: 128, Y: 128},
			{X: 128, Y: 0},
		}},
		{Kind: CharStringSegmentClose},
	}
}
