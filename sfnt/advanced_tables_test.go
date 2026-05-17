package sfnt

import (
	"encoding/binary"
	"testing"
)

func TestParseOS2(t *testing.T) {
	data := make([]byte, 100)
	binary.BigEndian.PutUint16(data[0:2], 3)      // version
	binary.BigEndian.PutUint16(data[4:6], 400)    // usWeightClass
	binary.BigEndian.PutUint16(data[64:66], 0x20) // usFirstCharIndex
	binary.BigEndian.PutUint16(data[66:68], 0xFF) // usLastCharIndex
	binary.BigEndian.PutUint16(data[74:76], 1000) // usWinAscent
	binary.BigEndian.PutUint16(data[76:78], 200)  // usWinDescent

	s := &mockStream{data: data}
	os2, err := parseOS2(s)
	if err != nil {
		t.Fatalf("failed to parse OS/2: %v", err)
	}

	if os2.Version != 3 {
		t.Errorf("expected version 3, got %d", os2.Version)
	}
	if os2.UsWeightClass != 400 {
		t.Errorf("expected weight class 400, got %d", os2.UsWeightClass)
	}
	if os2.UsWinAscent != 1000 {
		t.Errorf("expected win ascent 1000, got %d", os2.UsWinAscent)
	}
}

func TestParseVmtx(t *testing.T) {
	// 2 long metrics (4 bytes each) + 2 top side bearings (2 bytes each)
	data := make([]byte, 2*4+2*2)

	// Metric 0: AH=1000, TSB=100
	binary.BigEndian.PutUint16(data[0:2], 1000)
	binary.BigEndian.PutUint16(data[2:4], uint16(int16(100)))

	// Metric 1: AH=1100, TSB=110
	binary.BigEndian.PutUint16(data[4:6], 1100)
	binary.BigEndian.PutUint16(data[6:8], uint16(int16(110)))

	// TSBs for glyphs 2 and 3
	binary.BigEndian.PutUint16(data[8:10], uint16(int16(120)))
	binary.BigEndian.PutUint16(data[10:12], uint16(int16(130)))

	s := &mockStream{data: data}
	vmtx, err := parseVmtx(s, 4, 2)
	if err != nil {
		t.Fatalf("failed to parse vmtx: %v", err)
	}

	if len(vmtx.VMetrics) != 2 {
		t.Errorf("expected 2 long metrics, got %d", len(vmtx.VMetrics))
	}
	if vmtx.VMetrics[0].AdvanceHeight != 1000 {
		t.Errorf("expected AH 1000, got %d", vmtx.VMetrics[0].AdvanceHeight)
	}
	if vmtx.VMetrics[1].TopSideBearing != 110 {
		t.Errorf("expected TSB 110, got %d", vmtx.VMetrics[1].TopSideBearing)
	}
	if len(vmtx.TopSideBearings) != 2 {
		t.Errorf("expected 2 additional TSBs, got %d", len(vmtx.TopSideBearings))
	}
	if vmtx.TopSideBearings[0] != 120 {
		t.Errorf("expected TSB 120, got %d", vmtx.TopSideBearings[0])
	}
}

func TestParseVORG(t *testing.T) {
	data := make([]byte, 8+2*4)
	binary.BigEndian.PutUint16(data[0:2], 1)                   // major
	binary.BigEndian.PutUint16(data[2:4], 0)                   // minor
	binary.BigEndian.PutUint16(data[4:6], uint16(int16(1000))) // defaultVertOriginY
	binary.BigEndian.PutUint16(data[6:8], 2)                   // numVertOriginYMetrics

	// Metric 0: GID 5, Y 1200
	binary.BigEndian.PutUint16(data[8:10], 5)
	binary.BigEndian.PutUint16(data[10:12], uint16(int16(1200)))

	// Metric 1: GID 10, Y 1300
	binary.BigEndian.PutUint16(data[12:14], 10)
	binary.BigEndian.PutUint16(data[14:16], uint16(int16(1300)))

	s := &mockStream{data: data}
	vorg, err := parseVORG(s)
	if err != nil {
		t.Fatalf("failed to parse VORG: %v", err)
	}

	if vorg.MajorVersion != 1 {
		t.Errorf("expected major version 1, got %d", vorg.MajorVersion)
	}
	if vorg.DefaultVertOriginY != 1000 {
		t.Errorf("expected default Y 1000, got %d", vorg.DefaultVertOriginY)
	}
	if len(vorg.VertOriginYMetrics) != 2 {
		t.Errorf("expected 2 metrics, got %d", len(vorg.VertOriginYMetrics))
	}
	if vorg.VertOriginYMetrics[0].GlyphIndex != 5 || vorg.VertOriginYMetrics[0].VertOriginY != 1200 {
		t.Errorf("metric 0 mismatch")
	}
}

func TestParseGasp(t *testing.T) {
	data := make([]byte, 4+2*4)
	binary.BigEndian.PutUint16(data[0:2], 1) // version
	binary.BigEndian.PutUint16(data[2:4], 2) // numRanges

	// Range 0: MaxPPEM=8, Behavior=2
	binary.BigEndian.PutUint16(data[4:6], 8)
	binary.BigEndian.PutUint16(data[6:8], 2)

	// Range 1: MaxPPEM=65535, Behavior=15
	binary.BigEndian.PutUint16(data[8:10], 65535)
	binary.BigEndian.PutUint16(data[10:12], 15)

	s := &mockStream{data: data}
	gasp, err := parseGasp(s)
	if err != nil {
		t.Fatalf("failed to parse gasp: %v", err)
	}

	if gasp.Version != 1 {
		t.Errorf("expected version 1, got %d", gasp.Version)
	}
	if gasp.NumRanges != 2 {
		t.Errorf("expected 2 ranges, got %d", gasp.NumRanges)
	}
	if len(gasp.GaspRanges) != 2 {
		t.Errorf("expected length 2, got %d", len(gasp.GaspRanges))
	}
	if gasp.GaspRanges[0].RangeMaxPPEM != 8 || gasp.GaspRanges[0].RangeGaspBehavior != 2 {
		t.Errorf("range 0 mismatch")
	}
	if gasp.GaspRanges[1].RangeMaxPPEM != 65535 || gasp.GaspRanges[1].RangeGaspBehavior != 15 {
		t.Errorf("range 1 mismatch")
	}
}

func TestParseHdmxRejectsInvalidRecordLayout(t *testing.T) {
	data := make([]byte, 8)
	binary.BigEndian.PutUint16(data[0:2], 0)      // version
	binary.BigEndian.PutUint16(data[2:4], 0xFFFF) // negative int16 record count
	binary.BigEndian.PutUint32(data[4:8], 4)

	if _, err := parseHdmx(&mockStream{data: data}, 2); err == nil {
		t.Fatal("expected invalid hdmx record count to fail")
	}

	binary.BigEndian.PutUint16(data[2:4], 1)
	binary.BigEndian.PutUint32(data[4:8], 3) // smaller than pixelSize+maxWidth+2 glyph widths
	if _, err := parseHdmx(&mockStream{data: data}, 2); err == nil {
		t.Fatal("expected invalid hdmx record size to fail")
	}
}

func TestParseLTSHRejectsShortYpels(t *testing.T) {
	data := make([]byte, 5)
	binary.BigEndian.PutUint16(data[0:2], 0)
	binary.BigEndian.PutUint16(data[2:4], 2)
	data[4] = 8

	if _, err := parseLTSH(&mockStream{data: data}); err == nil {
		t.Fatal("expected short LTSH ypels to fail")
	}
}

func TestParseSTAT(t *testing.T) {
	data := make([]byte, 20)
	binary.BigEndian.PutUint16(data[0:2], 1)    // MajorVersion
	binary.BigEndian.PutUint16(data[2:4], 2)    // MinorVersion
	binary.BigEndian.PutUint16(data[4:6], 8)    // DesignAxisSize
	binary.BigEndian.PutUint16(data[6:8], 2)    // DesignAxisCount
	binary.BigEndian.PutUint32(data[8:12], 20)  // DesignAxesOffset
	binary.BigEndian.PutUint16(data[12:14], 3)  // AxisValueCount
	binary.BigEndian.PutUint32(data[14:18], 40) // AxisValueOffset
	binary.BigEndian.PutUint16(data[18:20], 2)  // ElidedFallbackNameID

	s := &mockStream{data: data}
	stat, err := parseSTAT(s)
	if err != nil {
		t.Fatalf("failed to parse STAT: %v", err)
	}

	if stat.MajorVersion != 1 || stat.MinorVersion != 2 {
		t.Errorf("expected version 1.2, got %d.%d", stat.MajorVersion, stat.MinorVersion)
	}
	if stat.DesignAxisCount != 2 {
		t.Errorf("expected 2 axes, got %d", stat.DesignAxisCount)
	}
	if stat.AxisValueCount != 3 {
		t.Errorf("expected 3 axis values, got %d", stat.AxisValueCount)
	}
	if stat.ElidedFallbackNameID != 2 {
		t.Errorf("expected fallback name ID 2, got %d", stat.ElidedFallbackNameID)
	}
}
