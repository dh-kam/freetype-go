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

func TestParseVheaVersion11FullHeader(t *testing.T) {
	data := make([]byte, 36)
	binary.BigEndian.PutUint32(data[0:4], 0x00011000)
	putTestInt16(data[4:6], 500)
	putTestInt16(data[6:8], -500)
	putTestInt16(data[8:10], 0)
	binary.BigEndian.PutUint16(data[10:12], 2079)
	putTestInt16(data[12:14], -342)
	putTestInt16(data[14:16], -333)
	putTestInt16(data[16:18], 2036)
	putTestInt16(data[18:20], 0)
	putTestInt16(data[20:22], 1)
	putTestInt16(data[22:24], 12)
	putTestInt16(data[24:26], 1)
	putTestInt16(data[26:28], -1)
	putTestInt16(data[28:30], 2)
	putTestInt16(data[30:32], -2)
	putTestInt16(data[32:34], 0)
	binary.BigEndian.PutUint16(data[34:36], 258)

	vhea, err := parseVhea(&mockStream{data: data})
	if err != nil {
		t.Fatalf("parseVhea failed: %v", err)
	}
	if vhea.Version != 0x00011000 || vhea.Ascent != 500 || vhea.Descent != -500 {
		t.Fatalf("unexpected vhea header: %+v", vhea)
	}
	if vhea.AdvanceHeightMax != 2079 || vhea.MinTopSideBearing != -342 || vhea.MinBottomSideBearing != -333 || vhea.YMaxExtent != 2036 {
		t.Fatalf("unexpected vhea vertical metrics: %+v", vhea)
	}
	if vhea.CaretSlopeRise != 0 || vhea.CaretSlopeRun != 1 || vhea.CaretOffset != 12 {
		t.Fatalf("unexpected vhea caret fields: %+v", vhea)
	}
	if vhea.Reserved != [4]int16{1, -1, 2, -2} {
		t.Fatalf("unexpected vhea reserved fields: %+v", vhea.Reserved)
	}
	if vhea.MetricDataFormat != 0 || vhea.NumOfLongVerMetrics != 258 {
		t.Fatalf("unexpected vhea metric format/count: %+v", vhea)
	}
}

func putTestInt16(dst []byte, v int16) {
	binary.BigEndian.PutUint16(dst, uint16(v))
}

func TestParseVheaRejectsShortHeader(t *testing.T) {
	if _, err := parseVhea(&mockStream{data: make([]byte, 35)}); err == nil {
		t.Fatal("expected short vhea table to fail")
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

func TestParseJstfScriptLangSysAndPriority(t *testing.T) {
	data := make([]byte, 112)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint32(data[6:10], stringToTag("arab"))
	binary.BigEndian.PutUint16(data[10:12], 12)

	script := 12
	binary.BigEndian.PutUint16(data[script:script+2], 12)
	binary.BigEndian.PutUint16(data[script+2:script+4], 20)
	binary.BigEndian.PutUint16(data[script+4:script+6], 1)
	binary.BigEndian.PutUint32(data[script+6:script+10], stringToTag("FAR "))
	binary.BigEndian.PutUint16(data[script+10:script+12], 68)

	extenders := script + 12
	binary.BigEndian.PutUint16(data[extenders:extenders+2], 2)
	binary.BigEndian.PutUint16(data[extenders+2:extenders+4], 0x01d3)
	binary.BigEndian.PutUint16(data[extenders+4:extenders+6], 0x01d4)

	defaultLangSys := script + 20
	binary.BigEndian.PutUint16(data[defaultLangSys:defaultLangSys+2], 1)
	binary.BigEndian.PutUint16(data[defaultLangSys+2:defaultLangSys+4], 4)

	defaultPriority := defaultLangSys + 4
	binary.BigEndian.PutUint16(data[defaultPriority:defaultPriority+2], 20)
	binary.BigEndian.PutUint16(data[defaultPriority+18:defaultPriority+20], 26)

	modList := defaultPriority + 20
	binary.BigEndian.PutUint16(data[modList:modList+2], 2)
	binary.BigEndian.PutUint16(data[modList+2:modList+4], 46)
	binary.BigEndian.PutUint16(data[modList+4:modList+6], 99)

	jstfMax := defaultPriority + 26
	binary.BigEndian.PutUint16(data[jstfMax:jstfMax+2], 1)
	binary.BigEndian.PutUint16(data[jstfMax+2:jstfMax+4], 8)
	lookup := jstfMax + 8
	binary.BigEndian.PutUint16(data[lookup:lookup+2], 1)
	binary.BigEndian.PutUint16(data[lookup+2:lookup+4], 0)
	binary.BigEndian.PutUint16(data[lookup+4:lookup+6], 0)

	farsiLangSys := script + 68
	binary.BigEndian.PutUint16(data[farsiLangSys:farsiLangSys+2], 1)
	binary.BigEndian.PutUint16(data[farsiLangSys+2:farsiLangSys+4], 4)
	farsiPriority := farsiLangSys + 4
	binary.BigEndian.PutUint16(data[farsiPriority+14:farsiPriority+16], 20)
	farsiModList := farsiPriority + 20
	binary.BigEndian.PutUint16(data[farsiModList:farsiModList+2], 1)
	binary.BigEndian.PutUint16(data[farsiModList+2:farsiModList+4], 108)

	jstf, err := ParseJstf(&mockStream{data: data})
	if err != nil {
		t.Fatalf("ParseJstf failed: %v", err)
	}
	if jstf.JstfScriptCount != 1 || len(jstf.JstfScripts) != 1 {
		t.Fatalf("unexpected JSTF script records: %+v", jstf)
	}
	record := jstf.JstfScripts[0]
	if record.JstfScriptTag != stringToTag("arab") || record.JstfScript == nil {
		t.Fatalf("unexpected JSTF script record: %+v", record)
	}
	if got := record.JstfScript.ExtenderGlyphs; len(got) != 2 || got[0] != 0x01d3 || got[1] != 0x01d4 {
		t.Fatalf("unexpected extender glyphs: %+v", got)
	}
	defaultPriorityTable := record.JstfScript.DefJstfLangSys.JstfPriorities[0]
	if got := defaultPriorityTable.GsubShrinkageEnable.LookupIndices; len(got) != 2 || got[0] != 46 || got[1] != 99 {
		t.Fatalf("unexpected shrinkage mod list: %+v", got)
	}
	if defaultPriorityTable.ExtensionJstfMax == nil || defaultPriorityTable.ExtensionJstfMax.LookupOffsets[0] != 8 {
		t.Fatalf("unexpected JstfMax: %+v", defaultPriorityTable.ExtensionJstfMax)
	}
	langRecord := record.JstfScript.JstfLangSysRecords[0]
	if langRecord.JstfLangSysTag != stringToTag("FAR ") || langRecord.JstfLangSys == nil {
		t.Fatalf("unexpected langsys record: %+v", langRecord)
	}
	if got := langRecord.JstfLangSys.JstfPriorities[0].GposExtensionEnable.LookupIndices; len(got) != 1 || got[0] != 108 {
		t.Fatalf("unexpected extension mod list: %+v", got)
	}
}

func TestParseJstfRejectsMalformedOffsets(t *testing.T) {
	data := make([]byte, 12)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint32(data[6:10], stringToTag("latn"))
	binary.BigEndian.PutUint16(data[10:12], 100)

	if _, err := ParseJstf(&mockStream{data: data}); err == nil {
		t.Fatal("expected malformed JSTF script offset to fail")
	}
}

func TestParseHdmxAndLookupWidth(t *testing.T) {
	data := make([]byte, 24)
	binary.BigEndian.PutUint16(data[0:2], 0)
	binary.BigEndian.PutUint16(data[2:4], 2)
	binary.BigEndian.PutUint32(data[4:8], 8)
	data[8] = 12
	data[9] = 9
	copy(data[10:13], []byte{4, 5, 6})
	data[16] = 14
	data[17] = 11
	copy(data[18:21], []byte{7, 8, 9})

	hdmx, err := parseHdmx(&mockStream{data: data}, 3)
	if err != nil {
		t.Fatalf("parseHdmx failed: %v", err)
	}
	if len(hdmx.Records) != 2 || hdmx.Records[1].PixelSize != 14 {
		t.Fatalf("unexpected hdmx records: %+v", hdmx.Records)
	}
	if width, ok := hdmx.Width(2, 14); !ok || width != 9 {
		t.Fatalf("hdmx width got %d ok=%v, want 9 true", width, ok)
	}
	if _, ok := hdmx.Width(3, 14); ok {
		t.Fatal("expected out-of-range glyph lookup to fail")
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
