package type1

import "testing"

func TestCompanionMetricsGlyphNameByCode(t *testing.T) {
	metrics := &CompanionMetrics{}
	metrics.Encoding[65] = "A"

	name, ok := metrics.GlyphNameByCode(65)
	if !ok || name != "A" {
		t.Fatalf("GlyphNameByCode(65) = %q, %v; want A, true", name, ok)
	}

	for _, code := range []int{-1, 66, 256} {
		if name, ok := metrics.GlyphNameByCode(code); ok {
			t.Fatalf("GlyphNameByCode(%d) = %q, true; want missing", code, name)
		}
	}
}

func TestCompanionMetricsCodeByGlyphName(t *testing.T) {
	metrics := &CompanionMetrics{}
	metrics.Encoding[65] = "A"
	metrics.Encoding[66] = "B"
	metrics.Encoding[67] = "B"

	code, ok := metrics.CodeByGlyphName("B")
	if !ok || code != 66 {
		t.Fatalf("CodeByGlyphName(B) = %d, %v; want first encoded code 66, true", code, ok)
	}

	if code, ok := metrics.CodeByGlyphName("C"); ok {
		t.Fatalf("CodeByGlyphName(C) = %d, true; want missing", code)
	}
	if code, ok := metrics.CodeByGlyphName(""); ok {
		t.Fatalf("CodeByGlyphName(empty) = %d, true; want missing", code)
	}
}

func TestCompanionMetricsWidthPrefersAFMByGlyphName(t *testing.T) {
	metrics := &CompanionMetrics{
		AFM: &AFM{
			CharMetrics: []AFMCharMetric{
				{Code: 65, Name: "A", WidthX: 722, BBox: [4]float64{9, 0, 689, 674}},
			},
		},
		PFM: &PFM{
			FirstChar:    65,
			LastChar:     65,
			ExtentWidths: []uint16{600},
		},
	}
	metrics.Encoding[65] = "A"

	width, ok := metrics.WidthByGlyphName("A")
	if !ok || width != 722 {
		t.Fatalf("WidthByGlyphName(A) = %v, %v; want 722, true", width, ok)
	}

	width, ok = metrics.WidthByCode(65)
	if !ok || width != 722 {
		t.Fatalf("WidthByCode(65) = %v, %v; want 722, true", width, ok)
	}
}

func TestCompanionMetricsWidthFallsBackToPFMByEncoding(t *testing.T) {
	metrics := &CompanionMetrics{
		PFM: &PFM{
			FirstChar:    65,
			LastChar:     67,
			ExtentWidths: []uint16{600, 610, 620},
		},
	}
	metrics.Encoding[65] = "A"
	metrics.Encoding[66] = "B"
	metrics.Encoding[67] = "B"

	width, ok := metrics.WidthByGlyphName("B")
	if !ok || width != 610 {
		t.Fatalf("WidthByGlyphName(B) = %v, %v; want first encoded PFM width 610, true", width, ok)
	}

	width, ok = metrics.WidthByCode(67)
	if !ok || width != 620 {
		t.Fatalf("WidthByCode(67) = %v, %v; want PFM width 620, true", width, ok)
	}
}

func TestCompanionMetricsWidthByCodeFallsBackToAFMCode(t *testing.T) {
	metrics := &CompanionMetrics{
		AFM: &AFM{
			CharMetrics: []AFMCharMetric{
				{Code: 65, Name: "A.alt", WidthX: 610},
				{Code: 90, Name: "A", WidthX: 722},
				{Code: 66, Name: "B", WidthX: 667},
			},
		},
	}
	metrics.Encoding[65] = "A"

	width, ok := metrics.WidthByCode(65)
	if !ok || width != 722 {
		t.Fatalf("WidthByCode(65) = %v, %v; want encoded-name AFM width 722, true", width, ok)
	}

	width, ok = metrics.WidthByCode(66)
	if !ok || width != 667 {
		t.Fatalf("WidthByCode(66) = %v, %v; want code AFM width 667, true", width, ok)
	}
}

func TestCompanionMetricsReportsMissingData(t *testing.T) {
	metrics := &CompanionMetrics{
		AFM: &AFM{
			CharMetrics: []AFMCharMetric{
				{Code: 65, Name: "A", WidthX: 722, BBox: [4]float64{9, 0, 689, 674}},
			},
			KernPairs: []AFMKernPair{
				{Left: "A", Right: "V", X: -80},
			},
		},
		PFM: &PFM{
			FirstChar:    65,
			LastChar:     65,
			ExtentWidths: []uint16{600},
		},
	}
	metrics.Encoding[65] = "A"

	if width, ok := metrics.WidthByGlyphName("B"); ok {
		t.Fatalf("WidthByGlyphName(B) = %v, true; want missing", width)
	}
	if width, ok := metrics.WidthByCode(66); ok {
		t.Fatalf("WidthByCode(66) = %v, true; want missing", width)
	}
	if lsb, ok := metrics.LeftSideBearingByGlyphName("B"); ok {
		t.Fatalf("LeftSideBearingByGlyphName(B) = %v, true; want missing", lsb)
	}
	if lsb, ok := metrics.LeftSideBearingByCode(66); ok {
		t.Fatalf("LeftSideBearingByCode(66) = %v, true; want missing", lsb)
	}
	if x, ok := metrics.KernX("A", "W"); ok {
		t.Fatalf("KernX(A, W) = %v, true; want missing", x)
	}
	if x, ok := metrics.KernXByGlyphName("A", "W"); ok {
		t.Fatalf("KernXByGlyphName(A, W) = %v, true; want missing", x)
	}
	if x, ok := metrics.KernXByCode(65, 66); ok {
		t.Fatalf("KernXByCode(65, 66) = %v, true; want missing", x)
	}
}

func TestCompanionMetricsLeftSideBearingFromAFMBBox(t *testing.T) {
	metrics := &CompanionMetrics{
		AFM: &AFM{
			CharMetrics: []AFMCharMetric{
				{Code: 65, Name: "A", WidthX: 722, BBox: [4]float64{-15, -4, 690, 674}},
			},
		},
	}

	lsb, ok := metrics.LeftSideBearingByGlyphName("A")
	if !ok || lsb != -15 {
		t.Fatalf("LeftSideBearingByGlyphName(A) = %v, %v; want -15, true", lsb, ok)
	}
}

func TestCompanionMetricsLeftSideBearingByCode(t *testing.T) {
	metrics := &CompanionMetrics{
		AFM: &AFM{
			CharMetrics: []AFMCharMetric{
				{Code: 65, Name: "A.alt", WidthX: 610, BBox: [4]float64{40, 0, 600, 674}},
				{Code: 90, Name: "A", WidthX: 722, BBox: [4]float64{-15, -4, 690, 674}},
				{Code: 66, Name: "B", WidthX: 667, BBox: [4]float64{20, 0, 630, 674}},
			},
		},
	}
	metrics.Encoding[65] = "A"

	lsb, ok := metrics.LeftSideBearingByCode(65)
	if !ok || lsb != -15 {
		t.Fatalf("LeftSideBearingByCode(65) = %v, %v; want encoded-name lsb -15, true", lsb, ok)
	}

	lsb, ok = metrics.LeftSideBearingByCode(66)
	if !ok || lsb != 20 {
		t.Fatalf("LeftSideBearingByCode(66) = %v, %v; want code lsb 20, true", lsb, ok)
	}
}

func TestCompanionMetricsKernXUsesAFM(t *testing.T) {
	metrics := &CompanionMetrics{
		AFM: &AFM{
			KernPairs: []AFMKernPair{
				{Left: "A", Right: "V", X: -80},
				{Left: "A", Right: "V", X: -70},
				{Left: "T", Right: "o", X: -40.5},
			},
		},
	}
	metrics.Encoding[65] = "A"
	metrics.Encoding[86] = "V"
	metrics.Encoding[111] = "o"
	metrics.Encoding[84] = "T"

	x, ok := metrics.KernX("A", "V")
	if !ok || x != -80 {
		t.Fatalf("KernX(A, V) = %v, %v; want -80, true", x, ok)
	}

	x, ok = metrics.KernXByGlyphName("T", "o")
	if !ok || x != -40.5 {
		t.Fatalf("KernXByGlyphName(T, o) = %v, %v; want -40.5, true", x, ok)
	}

	x, ok = metrics.KernXByCode(65, 86)
	if !ok || x != -80 {
		t.Fatalf("KernXByCode(65, 86) = %v, %v; want -80, true", x, ok)
	}

	x, ok = metrics.KernXByCode(84, 111)
	if !ok || x != -40.5 {
		t.Fatalf("KernXByCode(84, 111) = %v, %v; want -40.5, true", x, ok)
	}
}

func TestCompanionMetricsNilReceiverReportsMissing(t *testing.T) {
	var metrics *CompanionMetrics

	checks := []struct {
		name   string
		lookup func() bool
	}{
		{"GlyphNameByCode", func() bool { _, ok := metrics.GlyphNameByCode(65); return ok }},
		{"CodeByGlyphName", func() bool { _, ok := metrics.CodeByGlyphName("A"); return ok }},
		{"WidthByGlyphName", func() bool { _, ok := metrics.WidthByGlyphName("A"); return ok }},
		{"WidthByCode", func() bool { _, ok := metrics.WidthByCode(65); return ok }},
		{"LeftSideBearingByGlyphName", func() bool { _, ok := metrics.LeftSideBearingByGlyphName("A"); return ok }},
		{"LeftSideBearingByCode", func() bool { _, ok := metrics.LeftSideBearingByCode(65); return ok }},
		{"KernX", func() bool { _, ok := metrics.KernX("A", "V"); return ok }},
		{"KernXByGlyphName", func() bool { _, ok := metrics.KernXByGlyphName("A", "V"); return ok }},
		{"KernXByCode", func() bool { _, ok := metrics.KernXByCode(65, 86); return ok }},
	}

	for _, tt := range checks {
		if tt.lookup() {
			t.Fatalf("%s on nil receiver reported data; want missing", tt.name)
		}
	}
}

func TestCompanionMetricsNilCompanionsReportMissingMetrics(t *testing.T) {
	metrics := &CompanionMetrics{}
	metrics.Encoding[65] = "A"
	metrics.Encoding[86] = "V"

	if name, ok := metrics.GlyphNameByCode(65); !ok || name != "A" {
		t.Fatalf("GlyphNameByCode(65) = %q, %v; want A, true", name, ok)
	}
	if code, ok := metrics.CodeByGlyphName("A"); !ok || code != 65 {
		t.Fatalf("CodeByGlyphName(A) = %d, %v; want 65, true", code, ok)
	}

	checks := []struct {
		name   string
		lookup func() bool
	}{
		{"WidthByGlyphName", func() bool { _, ok := metrics.WidthByGlyphName("A"); return ok }},
		{"WidthByCode", func() bool { _, ok := metrics.WidthByCode(65); return ok }},
		{"LeftSideBearingByGlyphName", func() bool { _, ok := metrics.LeftSideBearingByGlyphName("A"); return ok }},
		{"LeftSideBearingByCode", func() bool { _, ok := metrics.LeftSideBearingByCode(65); return ok }},
		{"KernX", func() bool { _, ok := metrics.KernX("A", "V"); return ok }},
		{"KernXByGlyphName", func() bool { _, ok := metrics.KernXByGlyphName("A", "V"); return ok }},
		{"KernXByCode", func() bool { _, ok := metrics.KernXByCode(65, 86); return ok }},
	}

	for _, tt := range checks {
		if tt.lookup() {
			t.Fatalf("%s with nil AFM/PFM reported metrics; want missing", tt.name)
		}
	}
}

func TestCompanionMetricsNilAFMUsesPFMWidthsOnly(t *testing.T) {
	metrics := &CompanionMetrics{
		PFM: &PFM{
			FirstChar:    65,
			LastChar:     66,
			ExtentWidths: []uint16{600, 610},
		},
	}
	metrics.Encoding[65] = "A"
	metrics.Encoding[66] = "B"

	width, ok := metrics.WidthByGlyphName("A")
	if !ok || width != 600 {
		t.Fatalf("WidthByGlyphName(A) with nil AFM = %v, %v; want 600, true", width, ok)
	}

	width, ok = metrics.WidthByCode(66)
	if !ok || width != 610 {
		t.Fatalf("WidthByCode(66) with nil AFM = %v, %v; want 610, true", width, ok)
	}

	if lsb, ok := metrics.LeftSideBearingByCode(65); ok {
		t.Fatalf("LeftSideBearingByCode(65) with nil AFM = %v, true; want missing", lsb)
	}
	if x, ok := metrics.KernXByCode(65, 66); ok {
		t.Fatalf("KernXByCode(65, 66) with nil AFM = %v, true; want missing", x)
	}
}

func TestCompanionMetricsNilPFMUsesAFMOnly(t *testing.T) {
	metrics := &CompanionMetrics{
		AFM: &AFM{
			CharMetrics: []AFMCharMetric{
				{Code: 65, Name: "A", WidthX: 722, BBox: [4]float64{9, 0, 689, 674}},
			},
			KernPairs: []AFMKernPair{
				{Left: "A", Right: "V", X: -80},
			},
		},
	}
	metrics.Encoding[65] = "A"
	metrics.Encoding[86] = "V"

	width, ok := metrics.WidthByGlyphName("A")
	if !ok || width != 722 {
		t.Fatalf("WidthByGlyphName(A) with nil PFM = %v, %v; want 722, true", width, ok)
	}

	width, ok = metrics.WidthByCode(65)
	if !ok || width != 722 {
		t.Fatalf("WidthByCode(65) with nil PFM = %v, %v; want 722, true", width, ok)
	}

	lsb, ok := metrics.LeftSideBearingByCode(65)
	if !ok || lsb != 9 {
		t.Fatalf("LeftSideBearingByCode(65) with nil PFM = %v, %v; want 9, true", lsb, ok)
	}

	x, ok := metrics.KernXByCode(65, 86)
	if !ok || x != -80 {
		t.Fatalf("KernXByCode(65, 86) with nil PFM = %v, %v; want -80, true", x, ok)
	}
}
