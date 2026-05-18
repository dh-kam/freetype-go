package layout

import (
	"encoding/binary"
	"testing"
)

func TestParseGSUB(t *testing.T) {
	data := make([]byte, 40)
	// Version 1.0
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	// Offsets
	binary.BigEndian.PutUint16(data[4:6], 10)  // ScriptList
	binary.BigEndian.PutUint16(data[6:8], 18)  // FeatureList
	binary.BigEndian.PutUint16(data[8:10], 26) // LookupList

	// ScriptList at 10: 1 script, tag 'dflt', offset 10 (from ScriptList start = 20)
	binary.BigEndian.PutUint16(data[10:12], 1)
	binary.BigEndian.PutUint32(data[12:16], 0x64666C74)
	binary.BigEndian.PutUint16(data[16:18], 10)

	// FeatureList at 18: 1 feature, tag 'test', offset 10 (from FeatureList start = 28)
	binary.BigEndian.PutUint16(data[18:20], 1)
	binary.BigEndian.PutUint32(data[20:24], 0x74657374)
	binary.BigEndian.PutUint16(data[24:26], 10)

	// LookupList at 26: 1 lookup, offset 4 (from LookupList start = 30)
	binary.BigEndian.PutUint16(data[26:28], 1)
	binary.BigEndian.PutUint16(data[28:30], 4)

	// LookupTable at 30
	binary.BigEndian.PutUint16(data[30:32], 1) // Type 1
	binary.BigEndian.PutUint16(data[32:34], 0) // Flag
	binary.BigEndian.PutUint16(data[34:36], 0) // SubtableCount

	gsub, err := ParseGSUB(data)
	if err != nil {
		t.Fatalf("ParseGSUB failed: %v", err)
	}

	if gsub.VersionMajor != 1 || gsub.VersionMinor != 0 {
		t.Errorf("expected version 1.0, got %d.%d", gsub.VersionMajor, gsub.VersionMinor)
	}

	if len(gsub.ScriptList.Scripts) != 1 {
		t.Errorf("expected 1 script, got %d", len(gsub.ScriptList.Scripts))
	}
	if gsub.ScriptList.Scripts[0].Tag != 0x64666C74 {
		t.Errorf("expected 'dflt' tag, got 0x%08X", gsub.ScriptList.Scripts[0].Tag)
	}

	if len(gsub.FeatureList.Features) != 1 {
		t.Errorf("expected 1 feature, got %d", len(gsub.FeatureList.Features))
	}
	if gsub.FeatureList.Features[0].Tag != 0x74657374 {
		t.Errorf("expected 'test' tag, got 0x%08X", gsub.FeatureList.Features[0].Tag)
	}

	if len(gsub.LookupList.Lookups) != 1 {
		t.Errorf("expected 1 lookup, got %d", len(gsub.LookupList.Lookups))
	}
}

func TestParseGPOS(t *testing.T) {
	data := make([]byte, 40)
	// Version 1.0
	binary.BigEndian.PutUint16(data[0:2], 1)
	binary.BigEndian.PutUint16(data[2:4], 0)
	// Offsets
	binary.BigEndian.PutUint16(data[4:6], 10)
	binary.BigEndian.PutUint16(data[6:8], 18)
	binary.BigEndian.PutUint16(data[8:10], 26)

	// ScriptList at 10
	binary.BigEndian.PutUint16(data[10:12], 1)
	binary.BigEndian.PutUint32(data[12:16], 0x64666C74)
	binary.BigEndian.PutUint16(data[16:18], 10)

	// FeatureList at 18
	binary.BigEndian.PutUint16(data[18:20], 1)
	binary.BigEndian.PutUint32(data[20:24], 0x74657374)
	binary.BigEndian.PutUint16(data[24:26], 10)

	// LookupList at 26
	binary.BigEndian.PutUint16(data[26:28], 1)
	binary.BigEndian.PutUint16(data[28:30], 4)

	// LookupTable at 30
	binary.BigEndian.PutUint16(data[30:32], 2) // Type 2 (Pair Adjustment)
	binary.BigEndian.PutUint16(data[32:34], 0)
	binary.BigEndian.PutUint16(data[34:36], 0)

	gpos, err := ParseGPOS(data)
	if err != nil {
		t.Fatalf("ParseGPOS failed: %v", err)
	}

	if gpos.VersionMajor != 1 || gpos.VersionMinor != 0 {
		t.Errorf("expected version 1.0, got %d.%d", gpos.VersionMajor, gpos.VersionMinor)
	}
}

func TestValueRecordSizeAndNilParse(t *testing.T) {
	format := uint16(0x0005) // XPlacement + XAdvance
	if got := ValueRecordSize(format); got != 4 {
		t.Fatalf("expected ValueRecord size 4, got %d", got)
	}

	vr, size := ParseValueRecord(nil, format)
	if size != 4 {
		t.Fatalf("expected nil ParseValueRecord size 4, got %d", size)
	}
	if vr != (ValueRecord{}) {
		t.Fatalf("expected empty ValueRecord for nil data, got %+v", vr)
	}

	data := []byte{0x00, 0x09}
	vr, size = ParseValueRecord(data, format)
	if size != 4 {
		t.Fatalf("expected short ParseValueRecord size 4, got %d", size)
	}
	if vr.XPlacement != 9 || vr.XAdvance != 0 {
		t.Fatalf("unexpected short ValueRecord parse: %+v", vr)
	}
}

func TestParseLookupTableWithMarkFilteringSet(t *testing.T) {
	data := make([]byte, 10)
	binary.BigEndian.PutUint16(data[0:2], 4)
	binary.BigEndian.PutUint16(data[2:4], lookupFlagUseMarkFilteringSet)
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint16(data[6:8], 10)
	binary.BigEndian.PutUint16(data[8:10], 3)

	lookup, err := ParseLookupTable(data, 0)
	if err != nil {
		t.Fatalf("ParseLookupTable failed: %v", err)
	}
	if lookup.MarkFilteringSet != 3 {
		t.Fatalf("expected mark filtering set 3, got %d", lookup.MarkFilteringSet)
	}
}

func TestLayoutParsersRejectTruncatedCountRecords(t *testing.T) {
	tests := map[string]func() error{
		"script list": func() error {
			_, err := ParseScriptList([]byte{
				0x00, 0x01,
				0x64, 0x66,
			}, 0)
			return err
		},
		"feature list": func() error {
			_, err := ParseFeatureList([]byte{
				0x00, 0x01,
				0x74, 0x65, 0x73,
			}, 0)
			return err
		},
		"lookup list": func() error {
			_, err := ParseLookupList([]byte{0x00, 0x02, 0x00, 0x04}, 0)
			return err
		},
		"lookup table": func() error {
			_, err := ParseLookupTable([]byte{
				0x00, 0x01,
				0x00, 0x00,
				0x00, 0x01,
			}, 0)
			return err
		},
		"coverage format 1": func() error {
			_, err := ParseCoverage([]byte{
				0x00, 0x01,
				0x00, 0x02,
				0x00, 0x05,
			}, 0)
			return err
		},
		"coverage format 2": func() error {
			_, err := ParseCoverage([]byte{
				0x00, 0x02,
				0x00, 0x01,
				0x00, 0x01,
				0x00, 0x02,
			}, 0)
			return err
		},
		"class definition format 1": func() error {
			_, err := ParseClassDef([]byte{
				0x00, 0x01,
				0x00, 0x14,
				0x00, 0x02,
				0x00, 0x01,
			}, 0)
			return err
		},
		"class definition format 2": func() error {
			_, err := ParseClassDef([]byte{
				0x00, 0x02,
				0x00, 0x01,
				0x00, 0x14,
				0x00, 0x15,
			}, 0)
			return err
		},
	}

	for name, fn := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("parser panicked: %v", r)
				}
			}()
			if err := fn(); err == nil {
				t.Fatalf("expected malformed table error")
			}
		})
	}
}
