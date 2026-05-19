package sfnt

import (
	"encoding/binary"
	"testing"
)

func TestCMapFormat2Lookup(t *testing.T) {
	data := makeCMapFormat2Mixed()

	cmap, err := parseCMapSubtable(&mockStream{data: data}, 0)
	if err != nil {
		t.Fatalf("parseCMapSubtable failed: %v", err)
	}
	format2, ok := cmap.(*CMapFormat2)
	if !ok {
		t.Fatalf("expected *CMapFormat2, got %T", cmap)
	}
	if len(format2.SubHeaders) != 3 {
		t.Fatalf("expected 3 subHeaders, got %d", len(format2.SubHeaders))
	}
	if gid := cmap.Lookup(0x41); gid != 7 {
		t.Fatalf("expected single-byte GID 7 for 0x41, got %d", gid)
	}
	if gid := cmap.Lookup(0x81); gid != 0 {
		t.Fatalf("expected lead byte without low byte to return 0, got %d", gid)
	}
	if gid := cmap.Lookup(0x8140); gid != 22 {
		t.Fatalf("expected GID 22 for 0x8140, got %d", gid)
	}
	if gid := cmap.Lookup(0x8141); gid != 0 {
		t.Fatalf("expected zero glyph entry to stay missing, got %d", gid)
	}
	if gid := cmap.Lookup(0x8240); gid != 25 {
		t.Fatalf("expected shared sub-array with delta to map 0x8240 to GID 25, got %d", gid)
	}
	if gid := cmap.Lookup(0x823f); gid != 0 {
		t.Fatalf("expected low byte below firstCode to return 0, got %d", gid)
	}
	if gid := cmap.Lookup(0x8340); gid != 0 {
		t.Fatalf("expected non-lead high byte to return 0, got %d", gid)
	}
}

func TestParseCMapSelectsFormat2DBCSFallback(t *testing.T) {
	format2 := makeCMapFormat2Mixed()
	data := make([]byte, 12+len(format2))
	binary.BigEndian.PutUint16(data[2:4], 1)
	binary.BigEndian.PutUint16(data[4:6], 3)
	binary.BigEndian.PutUint16(data[6:8], 2)
	binary.BigEndian.PutUint32(data[8:12], 12)
	copy(data[12:], format2)

	cmap, err := parseCMap(&mockStream{data: data})
	if err != nil {
		t.Fatalf("parseCMap failed: %v", err)
	}
	if _, ok := cmap.(*CMapFormat2); !ok {
		t.Fatalf("expected *CMapFormat2 fallback, got %T", cmap)
	}
	if gid := cmap.Lookup(0x8140); gid != 22 {
		t.Fatalf("expected format 2 fallback to map 0x8140 to GID 22, got %d", gid)
	}
}

func TestCMapFormat6Lookup(t *testing.T) {
	data := make([]byte, 10+3*2)
	binary.BigEndian.PutUint16(data[0:2], 6)
	binary.BigEndian.PutUint16(data[2:4], uint16(len(data)))
	binary.BigEndian.PutUint16(data[6:8], 0x0020)
	binary.BigEndian.PutUint16(data[8:10], 3)
	binary.BigEndian.PutUint16(data[10:12], 0)
	binary.BigEndian.PutUint16(data[12:14], 41)
	binary.BigEndian.PutUint16(data[14:16], 42)

	cmap, err := parseCMapSubtable(&mockStream{data: data}, 0)
	if err != nil {
		t.Fatalf("parseCMapSubtable failed: %v", err)
	}
	if gid := cmap.Lookup(' '); gid != 0 {
		t.Fatalf("expected unmapped space to return 0, got %d", gid)
	}
	if gid := cmap.Lookup('!'); gid != 41 {
		t.Fatalf("expected GID 41 for '!', got %d", gid)
	}
	if gid := cmap.Lookup('"'); gid != 42 {
		t.Fatalf("expected GID 42 for '\"', got %d", gid)
	}
	if gid := cmap.Lookup('#'); gid != 0 {
		t.Fatalf("expected out-of-range codepoint to return 0, got %d", gid)
	}
}

func TestCMapFormat8Lookup(t *testing.T) {
	data := make([]byte, cmapFormat8HeaderSize+2*12)
	binary.BigEndian.PutUint16(data[0:2], 8)
	binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
	setCMapFormat8Is32(data, 1)
	binary.BigEndian.PutUint32(data[12+cmapFormat8Is32Size:12+cmapFormat8Is32Size+4], 2)

	groupOffset := cmapFormat8HeaderSize
	binary.BigEndian.PutUint32(data[groupOffset:groupOffset+4], 'A')
	binary.BigEndian.PutUint32(data[groupOffset+4:groupOffset+8], 'C')
	binary.BigEndian.PutUint32(data[groupOffset+8:groupOffset+12], 10)

	groupOffset += 12
	binary.BigEndian.PutUint32(data[groupOffset:groupOffset+4], 0x10000)
	binary.BigEndian.PutUint32(data[groupOffset+4:groupOffset+8], 0x10001)
	binary.BigEndian.PutUint32(data[groupOffset+8:groupOffset+12], 200)

	cmap, err := parseCMapSubtable(&mockStream{data: data}, 0)
	if err != nil {
		t.Fatalf("parseCMapSubtable failed: %v", err)
	}
	if gid := cmap.Lookup('A'); gid != 10 {
		t.Fatalf("expected GID 10 for A, got %d", gid)
	}
	if gid := cmap.Lookup('C'); gid != 12 {
		t.Fatalf("expected GID 12 for C, got %d", gid)
	}
	if gid := cmap.Lookup('\U00010000'); gid != 200 {
		t.Fatalf("expected GID 200 for U+10000, got %d", gid)
	}
	if gid := cmap.Lookup('\U00010001'); gid != 201 {
		t.Fatalf("expected GID 201 for U+10001, got %d", gid)
	}
	if gid := cmap.Lookup('D'); gid != 0 {
		t.Fatalf("expected out-of-range codepoint to return 0, got %d", gid)
	}
}

func TestCMapFormat10Lookup(t *testing.T) {
	data := make([]byte, 20+2*2)
	binary.BigEndian.PutUint16(data[0:2], 10)
	binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
	binary.BigEndian.PutUint32(data[12:16], 0x1f600)
	binary.BigEndian.PutUint32(data[16:20], 2)
	binary.BigEndian.PutUint16(data[20:22], 77)
	binary.BigEndian.PutUint16(data[22:24], 78)

	cmap, err := parseCMapSubtable(&mockStream{data: data}, 0)
	if err != nil {
		t.Fatalf("parseCMapSubtable failed: %v", err)
	}
	if gid := cmap.Lookup('\U0001F600'); gid != 77 {
		t.Fatalf("expected GID 77 for U+1F600, got %d", gid)
	}
	if gid := cmap.Lookup('\U0001F601'); gid != 78 {
		t.Fatalf("expected GID 78 for U+1F601, got %d", gid)
	}
	if gid := cmap.Lookup('\U0001F602'); gid != 0 {
		t.Fatalf("expected out-of-range codepoint to return 0, got %d", gid)
	}
}

func TestCMapFormat12Lookup(t *testing.T) {
	data := make([]byte, 16+2*12)
	binary.BigEndian.PutUint16(data[0:2], 12)
	binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
	binary.BigEndian.PutUint32(data[12:16], 2)
	binary.BigEndian.PutUint32(data[16:20], 0x0041)
	binary.BigEndian.PutUint32(data[20:24], 0x0043)
	binary.BigEndian.PutUint32(data[24:28], 10)
	binary.BigEndian.PutUint32(data[28:32], 0x1f600)
	binary.BigEndian.PutUint32(data[32:36], 0x1f601)
	binary.BigEndian.PutUint32(data[36:40], 40)

	cmap, err := parseCMapSubtable(&mockStream{data: data}, 0)
	if err != nil {
		t.Fatalf("parseCMapSubtable failed: %v", err)
	}
	if gid := cmap.Lookup('A'); gid != 10 {
		t.Fatalf("expected GID 10 for 'A', got %d", gid)
	}
	if gid := cmap.Lookup('C'); gid != 12 {
		t.Fatalf("expected GID 12 for 'C', got %d", gid)
	}
	if gid := cmap.Lookup('\U0001F601'); gid != 41 {
		t.Fatalf("expected GID 41 for U+1F601, got %d", gid)
	}
	if gid := cmap.Lookup('D'); gid != 0 {
		t.Fatalf("expected out-of-range codepoint to return 0, got %d", gid)
	}
}

func TestCMapFormat13Lookup(t *testing.T) {
	data := make([]byte, 16+12)
	binary.BigEndian.PutUint16(data[0:2], 13)
	binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
	binary.BigEndian.PutUint32(data[12:16], 1)
	binary.BigEndian.PutUint32(data[16:20], 0xe000)
	binary.BigEndian.PutUint32(data[20:24], 0xe002)
	binary.BigEndian.PutUint32(data[24:28], 123)

	cmap, err := parseCMapSubtable(&mockStream{data: data}, 0)
	if err != nil {
		t.Fatalf("parseCMapSubtable failed: %v", err)
	}
	for _, r := range []rune{0xe000, 0xe001, 0xe002} {
		if gid := cmap.Lookup(r); gid != 123 {
			t.Fatalf("expected GID 123 for U+%04X, got %d", r, gid)
		}
	}
	if gid := cmap.Lookup(0xe003); gid != 0 {
		t.Fatalf("expected out-of-range codepoint to return 0, got %d", gid)
	}
}

func TestCMapFormat14LookupVariation(t *testing.T) {
	data := make([]byte, 54)
	binary.BigEndian.PutUint16(data[0:2], 14)
	binary.BigEndian.PutUint32(data[2:6], uint32(len(data)))
	binary.BigEndian.PutUint32(data[6:10], 2)

	putCMapUint24(data[10:13], 0xfe0e)
	binary.BigEndian.PutUint32(data[13:17], 32)
	binary.BigEndian.PutUint32(data[17:21], 0)

	putCMapUint24(data[21:24], 0xe0100)
	binary.BigEndian.PutUint32(data[24:28], 0)
	binary.BigEndian.PutUint32(data[28:32], 40)

	binary.BigEndian.PutUint32(data[32:36], 1)
	putCMapUint24(data[36:39], 'A')
	data[39] = 1

	binary.BigEndian.PutUint32(data[40:44], 2)
	putCMapUint24(data[44:47], 0x4e00)
	binary.BigEndian.PutUint16(data[47:49], 320)
	putCMapUint24(data[49:52], 0x4e01)
	binary.BigEndian.PutUint16(data[52:54], 321)

	cmap, err := parseCMapSubtable(&mockStream{data: data}, 0)
	if err != nil {
		t.Fatalf("parseCMapSubtable failed: %v", err)
	}
	format14, ok := cmap.(*CMapFormat14)
	if !ok {
		t.Fatalf("expected *CMapFormat14, got %T", cmap)
	}
	if gid := format14.Lookup('A'); gid != 0 {
		t.Fatalf("expected format 14 base lookup to return 0, got %d", gid)
	}
	if gid, useDefault, ok := format14.LookupVariation('A', 0xfe0e); !ok || !useDefault || gid != 0 {
		t.Fatalf("expected default UVS for A+FE0E, got gid=%d useDefault=%v ok=%v", gid, useDefault, ok)
	}
	if _, _, ok := format14.LookupVariation('C', 0xfe0e); ok {
		t.Fatal("expected C+FE0E to be absent")
	}
	if gid, useDefault, ok := format14.LookupVariation(0x4e01, 0xe0100); !ok || useDefault || gid != 321 {
		t.Fatalf("expected non-default UVS gid 321, got gid=%d useDefault=%v ok=%v", gid, useDefault, ok)
	}
}

func TestParseCMapCombinesBaseAndVariation(t *testing.T) {
	base := makeCMapFormat4Range('A', 'B', -64)
	variation := make([]byte, 49)
	binary.BigEndian.PutUint16(variation[0:2], 14)
	binary.BigEndian.PutUint32(variation[2:6], uint32(len(variation)))
	binary.BigEndian.PutUint32(variation[6:10], 2)

	putCMapUint24(variation[10:13], 0xfe0e)
	binary.BigEndian.PutUint32(variation[13:17], 32)
	binary.BigEndian.PutUint32(variation[17:21], 0)

	putCMapUint24(variation[21:24], 0xfe0f)
	binary.BigEndian.PutUint32(variation[24:28], 0)
	binary.BigEndian.PutUint32(variation[28:32], 40)

	binary.BigEndian.PutUint32(variation[32:36], 1)
	putCMapUint24(variation[36:39], 'A')
	variation[39] = 0

	binary.BigEndian.PutUint32(variation[40:44], 1)
	putCMapUint24(variation[44:47], 'B')
	binary.BigEndian.PutUint16(variation[47:49], 222)

	data := make([]byte, 20+len(base)+len(variation))
	binary.BigEndian.PutUint16(data[2:4], 2)
	binary.BigEndian.PutUint16(data[4:6], 3)
	binary.BigEndian.PutUint16(data[6:8], 1)
	binary.BigEndian.PutUint32(data[8:12], 20)
	binary.BigEndian.PutUint16(data[12:14], 0)
	binary.BigEndian.PutUint16(data[14:16], 5)
	binary.BigEndian.PutUint32(data[16:20], uint32(20+len(base)))
	copy(data[20:], base)
	copy(data[20+len(base):], variation)

	cmap, err := parseCMap(&mockStream{data: data})
	if err != nil {
		t.Fatalf("parseCMap failed: %v", err)
	}
	if gid := cmap.Lookup('A'); gid != 1 {
		t.Fatalf("expected base gid 1 for A, got %d", gid)
	}
	if gid := cmap.Lookup('B'); gid != 2 {
		t.Fatalf("expected base gid 2 for B, got %d", gid)
	}
	variationCMap, ok := cmap.(CMapVariation)
	if !ok {
		t.Fatalf("expected cmap to implement CMapVariation, got %T", cmap)
	}
	if gid, useDefault, ok := variationCMap.LookupVariation('A', 0xfe0e); !ok || !useDefault || gid != 1 {
		t.Fatalf("expected default UVS to resolve base gid 1, got gid=%d useDefault=%v ok=%v", gid, useDefault, ok)
	}
	if gid, useDefault, ok := variationCMap.LookupVariation('B', 0xfe0f); !ok || useDefault || gid != 222 {
		t.Fatalf("expected non-default UVS gid 222, got gid=%d useDefault=%v ok=%v", gid, useDefault, ok)
	}
}

func TestCMapMalformedSubtablesRejected(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "format 4 segment arrays exceed length",
			data: func() []byte {
				data := make([]byte, 16)
				binary.BigEndian.PutUint16(data[0:2], 4)
				binary.BigEndian.PutUint16(data[2:4], uint16(len(data)))
				binary.BigEndian.PutUint16(data[6:8], 2)
				return data
			}(),
		},
		{
			name: "format 2 subHeaderKeys exceed length",
			data: func() []byte {
				data := make([]byte, cmapFormat2MinSize)
				binary.BigEndian.PutUint16(data[0:2], 2)
				binary.BigEndian.PutUint16(data[2:4], uint16(len(data)))
				binary.BigEndian.PutUint16(data[6:8], 8)
				return data
			}(),
		},
		{
			name: "format 2 subHeaderKey not multiple of eight",
			data: func() []byte {
				data := make([]byte, cmapFormat2MinSize)
				binary.BigEndian.PutUint16(data[0:2], 2)
				binary.BigEndian.PutUint16(data[2:4], uint16(len(data)))
				binary.BigEndian.PutUint16(data[6:8], 2)
				return data
			}(),
		},
		{
			name: "format 2 subHeader byte range overflows",
			data: func() []byte {
				data := make([]byte, cmapFormat2MinSize)
				binary.BigEndian.PutUint16(data[0:2], 2)
				binary.BigEndian.PutUint16(data[2:4], uint16(len(data)))
				binary.BigEndian.PutUint16(data[cmapFormat2HeaderSize:cmapFormat2HeaderSize+2], 250)
				binary.BigEndian.PutUint16(data[cmapFormat2HeaderSize+2:cmapFormat2HeaderSize+4], 7)
				return data
			}(),
		},
		{
			name: "format 2 idRangeOffset not word aligned",
			data: func() []byte {
				data := make([]byte, cmapFormat2MinSize+2)
				binary.BigEndian.PutUint16(data[0:2], 2)
				binary.BigEndian.PutUint16(data[2:4], uint16(len(data)))
				binary.BigEndian.PutUint16(data[cmapFormat2HeaderSize+2:cmapFormat2HeaderSize+4], 1)
				binary.BigEndian.PutUint16(data[cmapFormat2HeaderSize+6:cmapFormat2HeaderSize+8], 3)
				return data
			}(),
		},
		{
			name: "format 2 glyph range exceeds length",
			data: func() []byte {
				data := make([]byte, cmapFormat2MinSize+2)
				binary.BigEndian.PutUint16(data[0:2], 2)
				binary.BigEndian.PutUint16(data[2:4], uint16(len(data)))
				binary.BigEndian.PutUint16(data[cmapFormat2HeaderSize+2:cmapFormat2HeaderSize+4], 2)
				binary.BigEndian.PutUint16(data[cmapFormat2HeaderSize+6:cmapFormat2HeaderSize+8], 2)
				return data
			}(),
		},
		{
			name: "format 6 glyph array exceeds length",
			data: func() []byte {
				data := make([]byte, 12)
				binary.BigEndian.PutUint16(data[0:2], 6)
				binary.BigEndian.PutUint16(data[2:4], uint16(len(data)))
				binary.BigEndian.PutUint16(data[8:10], 2)
				return data
			}(),
		},
		{
			name: "format 8 header shorter than minimum",
			data: func() []byte {
				data := make([]byte, 20)
				binary.BigEndian.PutUint16(data[0:2], 8)
				binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
				return data
			}(),
		},
		{
			name: "format 8 groups exceed length",
			data: func() []byte {
				data := make([]byte, cmapFormat8HeaderSize)
				binary.BigEndian.PutUint16(data[0:2], 8)
				binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
				binary.BigEndian.PutUint32(data[12+cmapFormat8Is32Size:12+cmapFormat8Is32Size+4], 1)
				return data
			}(),
		},
		{
			name: "format 8 supplementary group missing is32 marker",
			data: func() []byte {
				data := make([]byte, cmapFormat8HeaderSize+12)
				binary.BigEndian.PutUint16(data[0:2], 8)
				binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
				binary.BigEndian.PutUint32(data[12+cmapFormat8Is32Size:12+cmapFormat8Is32Size+4], 1)
				binary.BigEndian.PutUint32(data[cmapFormat8HeaderSize:cmapFormat8HeaderSize+4], 0x10000)
				binary.BigEndian.PutUint32(data[cmapFormat8HeaderSize+4:cmapFormat8HeaderSize+8], 0x10000)
				binary.BigEndian.PutUint32(data[cmapFormat8HeaderSize+8:cmapFormat8HeaderSize+12], 1)
				return data
			}(),
		},
		{
			name: "format 10 glyph array exceeds length",
			data: func() []byte {
				data := make([]byte, 20)
				binary.BigEndian.PutUint16(data[0:2], 10)
				binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
				binary.BigEndian.PutUint32(data[16:20], 1)
				return data
			}(),
		},
		{
			name: "format 12 groups exceed length",
			data: func() []byte {
				data := make([]byte, 16)
				binary.BigEndian.PutUint16(data[0:2], 12)
				binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
				binary.BigEndian.PutUint32(data[12:16], 1)
				return data
			}(),
		},
		{
			name: "format 13 descending range",
			data: func() []byte {
				data := make([]byte, 28)
				binary.BigEndian.PutUint16(data[0:2], 13)
				binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
				binary.BigEndian.PutUint32(data[12:16], 1)
				binary.BigEndian.PutUint32(data[16:20], 0x20)
				binary.BigEndian.PutUint32(data[20:24], 0x10)
				binary.BigEndian.PutUint32(data[24:28], 3)
				return data
			}(),
		},
		{
			name: "format 14 selector records exceed length",
			data: func() []byte {
				data := make([]byte, 10)
				binary.BigEndian.PutUint16(data[0:2], 14)
				binary.BigEndian.PutUint32(data[2:6], uint32(len(data)))
				binary.BigEndian.PutUint32(data[6:10], 1)
				return data
			}(),
		},
		{
			name: "format 14 default UVS offset outside length",
			data: func() []byte {
				data := make([]byte, 21)
				binary.BigEndian.PutUint16(data[0:2], 14)
				binary.BigEndian.PutUint32(data[2:6], uint32(len(data)))
				binary.BigEndian.PutUint32(data[6:10], 1)
				putCMapUint24(data[10:13], 0xfe0e)
				binary.BigEndian.PutUint32(data[13:17], uint32(len(data)))
				return data
			}(),
		},
		{
			name: "format 14 non-default UVS mappings exceed length",
			data: func() []byte {
				data := make([]byte, 25)
				binary.BigEndian.PutUint16(data[0:2], 14)
				binary.BigEndian.PutUint32(data[2:6], uint32(len(data)))
				binary.BigEndian.PutUint32(data[6:10], 1)
				putCMapUint24(data[10:13], 0xfe0e)
				binary.BigEndian.PutUint32(data[17:21], 21)
				binary.BigEndian.PutUint32(data[21:25], 1)
				return data
			}(),
		},
		{
			name: "format 14 non-default UVS value exceeds Unicode range",
			data: func() []byte {
				data := make([]byte, 30)
				binary.BigEndian.PutUint16(data[0:2], 14)
				binary.BigEndian.PutUint32(data[2:6], uint32(len(data)))
				binary.BigEndian.PutUint32(data[6:10], 1)
				putCMapUint24(data[10:13], 0xfe0e)
				binary.BigEndian.PutUint32(data[17:21], 21)
				binary.BigEndian.PutUint32(data[21:25], 1)
				putCMapUint24(data[25:28], 0x110000)
				binary.BigEndian.PutUint16(data[28:30], 1)
				return data
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseCMapSubtable(&mockStream{data: tt.data}, 0); err == nil {
				t.Fatal("expected malformed cmap subtable to fail")
			}
		})
	}
}

func TestParseVDMX(t *testing.T) {
	data := make([]byte, 28)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 1)
	binary.BigEndian.PutUint16(data[4:6], 1)
	data[6] = 1
	data[7] = 1
	data[8] = 0
	data[9] = 1
	binary.BigEndian.PutUint16(data[10:12], 12)
	binary.BigEndian.PutUint16(data[12:14], 2)
	data[14] = 8
	data[15] = 16
	binary.BigEndian.PutUint16(data[16:18], 9)
	binary.BigEndian.PutUint16(data[18:20], uint16(int16(700)))
	binary.BigEndian.PutUint16(data[20:22], 0xff38)
	binary.BigEndian.PutUint16(data[22:24], 16)
	binary.BigEndian.PutUint16(data[24:26], uint16(int16(900)))
	binary.BigEndian.PutUint16(data[26:28], 0xff06)

	vdmx, err := parseVDMX(&mockStream{data: data})
	if err != nil {
		t.Fatalf("parseVDMX failed: %v", err)
	}
	if vdmx.Version != 1 || vdmx.NumVDMXGroups != 1 || vdmx.NumRatios != 1 {
		t.Fatalf("unexpected VDMX header: %+v", vdmx)
	}
	if len(vdmx.RatioRanges) != 1 || vdmx.RatioRanges[0].XRatio != 1 || vdmx.RatioRanges[0].YEndRatio != 1 {
		t.Fatalf("unexpected VDMX ratio ranges: %+v", vdmx.RatioRanges)
	}
	if len(vdmx.GroupOffsets) != 1 || vdmx.GroupOffsets[0] != 12 {
		t.Fatalf("unexpected VDMX group offsets: %+v", vdmx.GroupOffsets)
	}
	if len(vdmx.Groups) != 1 || vdmx.Groups[0].StartSize != 8 || vdmx.Groups[0].EndSize != 16 {
		t.Fatalf("unexpected VDMX groups: %+v", vdmx.Groups)
	}
	if len(vdmx.Groups[0].Records) != 2 {
		t.Fatalf("expected 2 VDMX records, got %d", len(vdmx.Groups[0].Records))
	}
	if record := vdmx.Groups[0].Records[1]; record.YPelHeight != 16 || record.YMax != 900 || record.YMin != -250 {
		t.Fatalf("unexpected VDMX record: %+v", record)
	}
}

func TestParseVDMXRejectsMalformedGroupBounds(t *testing.T) {
	data := make([]byte, 16)
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 1)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint16(data[10:12], 12)
	binary.BigEndian.PutUint16(data[12:14], 1)
	data[14] = 8
	data[15] = 8

	if _, err := parseVDMX(&mockStream{data: data}); err == nil {
		t.Fatal("expected short VDMX group to fail")
	}
}

func TestParsePostVersion2(t *testing.T) {
	data := make([]byte, 32+2+2*2+1+5+1+3)
	binary.BigEndian.PutUint32(data[0:4], 0x00020000)
	binary.BigEndian.PutUint32(data[4:8], 0xffff0000)
	binary.BigEndian.PutUint16(data[8:10], 0xffb5)
	binary.BigEndian.PutUint16(data[10:12], 50)
	binary.BigEndian.PutUint32(data[12:16], 1)
	binary.BigEndian.PutUint16(data[32:34], 2)
	binary.BigEndian.PutUint16(data[34:36], 258)
	binary.BigEndian.PutUint16(data[36:38], 259)
	data[38] = 5
	copy(data[39:44], "A.alt")
	data[44] = 3
	copy(data[45:48], "foo")

	post, err := parsePost(&mockStream{data: data})
	if err != nil {
		t.Fatalf("parsePost failed: %v", err)
	}
	if post.Version != 0x00020000 {
		t.Fatalf("expected post version 2.0, got %#x", post.Version)
	}
	if post.ItalicAngle != -65536 || post.UnderlinePosition != -75 || post.UnderlineThickness != 50 {
		t.Fatalf("unexpected post metrics: %+v", post)
	}
	if post.IsFixedPitch != 1 {
		t.Fatalf("expected fixed pitch flag, got %d", post.IsFixedPitch)
	}
	if len(post.GlyphNameID) != 2 || post.GlyphNameID[0] != 258 || post.GlyphNameID[1] != 259 {
		t.Fatalf("unexpected glyph name IDs: %+v", post.GlyphNameID)
	}
	if len(post.Names) != 2 || post.Names[0] != "A.alt" || post.Names[1] != "foo" {
		t.Fatalf("unexpected post names: %+v", post.Names)
	}
	if got, ok := post.GlyphName(0); !ok || got != "A.alt" {
		t.Fatalf("post glyph 0 name = %q, %v; want A.alt, true", got, ok)
	}
	if got, ok := post.GlyphName(1); !ok || got != "foo" {
		t.Fatalf("post glyph 1 name = %q, %v; want foo, true", got, ok)
	}
	standard := PostTable{Version: 0x00010000}
	if got, ok := standard.GlyphName(36); !ok || got != "A" {
		t.Fatalf("standard post glyph 36 name = %q, %v; want A, true", got, ok)
	}
}

func TestParsePostRejectsShortHeader(t *testing.T) {
	if _, err := parsePost(&mockStream{data: make([]byte, 31)}); err == nil {
		t.Fatal("expected short post table to fail")
	}
}

func makeCMapFormat4Range(start rune, end rune, delta int16) []byte {
	segCount := 2
	length := 16 + segCount*8
	data := make([]byte, length)

	binary.BigEndian.PutUint16(data[0:2], 4)
	binary.BigEndian.PutUint16(data[2:4], uint16(length))
	binary.BigEndian.PutUint16(data[6:8], uint16(segCount*2))
	binary.BigEndian.PutUint16(data[8:10], 2)
	binary.BigEndian.PutUint16(data[10:12], 1)
	binary.BigEndian.PutUint16(data[12:14], 2)
	binary.BigEndian.PutUint16(data[14:16], uint16(start))
	binary.BigEndian.PutUint16(data[16:18], 0xffff)
	binary.BigEndian.PutUint16(data[20:22], uint16(start))
	binary.BigEndian.PutUint16(data[22:24], 0xffff)
	binary.BigEndian.PutUint16(data[24:26], uint16(delta))
	binary.BigEndian.PutUint16(data[26:28], 1)
	binary.BigEndian.PutUint16(data[28:30], 0)
	binary.BigEndian.PutUint16(data[30:32], 0)
	if end != start {
		binary.BigEndian.PutUint16(data[14:16], uint16(end))
	}
	return data
}

func makeCMapFormat2Mixed() []byte {
	subHeaderCount := 3
	glyphArrayOffset := cmapFormat2HeaderSize + subHeaderCount*8
	singleByteGlyphsOffset := glyphArrayOffset
	doubleByteGlyphsOffset := singleByteGlyphsOffset + 256*2
	data := make([]byte, doubleByteGlyphsOffset+2*2)

	binary.BigEndian.PutUint16(data[0:2], 2)
	binary.BigEndian.PutUint16(data[2:4], uint16(len(data)))
	binary.BigEndian.PutUint16(data[6+0x81*2:6+0x81*2+2], 8)
	binary.BigEndian.PutUint16(data[6+0x82*2:6+0x82*2+2], 16)

	putCMapFormat2SubHeader(data, 0, 0, 256, 0, singleByteGlyphsOffset)
	putCMapFormat2SubHeader(data, 1, 0x40, 2, 2, doubleByteGlyphsOffset)
	putCMapFormat2SubHeader(data, 2, 0x40, 2, 5, doubleByteGlyphsOffset)

	binary.BigEndian.PutUint16(data[singleByteGlyphsOffset+0x41*2:singleByteGlyphsOffset+0x41*2+2], 7)
	binary.BigEndian.PutUint16(data[doubleByteGlyphsOffset:doubleByteGlyphsOffset+2], 20)
	return data
}

func putCMapFormat2SubHeader(data []byte, index int, firstCode uint16, entryCount uint16, idDelta int16, glyphStartOffset int) {
	subHeaderOffset := cmapFormat2HeaderSize + index*8
	idRangeOffsetWordOffset := subHeaderOffset + 6
	binary.BigEndian.PutUint16(data[subHeaderOffset:subHeaderOffset+2], firstCode)
	binary.BigEndian.PutUint16(data[subHeaderOffset+2:subHeaderOffset+4], entryCount)
	binary.BigEndian.PutUint16(data[subHeaderOffset+4:subHeaderOffset+6], uint16(idDelta))
	binary.BigEndian.PutUint16(data[subHeaderOffset+6:subHeaderOffset+8], uint16(glyphStartOffset-idRangeOffsetWordOffset))
}

func putCMapUint24(dst []byte, v uint32) {
	dst[0] = byte(v >> 16)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v)
}

func setCMapFormat8Is32(data []byte, word uint16) {
	data[12+int(word)/8] |= byte(1 << (7 - (word % 8)))
}
