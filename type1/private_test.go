package type1

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func TestParseFontPrivateHintMetadata(t *testing.T) {
	private := privateHintTestPrivate(`
/BlueValues [-20 0 500 520] def
/OtherBlues [-220 -200] def
/FamilyBlues [-15 0 490 510] def
/FamilyOtherBlues [-210 -190] def
/StdHW [70] def
/StdVW [80] def
/StemSnapH [70 90] def
/StemSnapV [80 100 120] def
/BlueScale 0.039625 def
/BlueShift 7 def
/BlueFuzz 1 def
/ForceBold true def
/LanguageGroup 1 def
`, []byte{14})

	font, err := ParseFont(privateHintTestPFA(private))
	if err != nil {
		t.Fatalf("ParseFont failed: %v", err)
	}
	privateHintWantFloats(t, "BlueValues", font.BlueValues, []float64{-20, 0, 500, 520})
	privateHintWantFloats(t, "OtherBlues", font.OtherBlues, []float64{-220, -200})
	privateHintWantFloats(t, "FamilyBlues", font.FamilyBlues, []float64{-15, 0, 490, 510})
	privateHintWantFloats(t, "FamilyOtherBlues", font.FamilyOtherBlues, []float64{-210, -190})
	privateHintWantFloats(t, "StdHW", font.StdHW, []float64{70})
	privateHintWantFloats(t, "StdVW", font.StdVW, []float64{80})
	privateHintWantFloats(t, "StemSnapH", font.StemSnapH, []float64{70, 90})
	privateHintWantFloats(t, "StemSnapV", font.StemSnapV, []float64{80, 100, 120})
	if !font.HasBlueScale || math.Abs(font.BlueScale-0.039625) > 1e-12 {
		t.Fatalf("BlueScale = (%v, %v), want present 0.039625", font.BlueScale, font.HasBlueScale)
	}
	if !font.HasBlueShift || font.BlueShift != 7 {
		t.Fatalf("BlueShift = (%d, %v), want present 7", font.BlueShift, font.HasBlueShift)
	}
	if !font.HasBlueFuzz || font.BlueFuzz != 1 {
		t.Fatalf("BlueFuzz = (%d, %v), want present 1", font.BlueFuzz, font.HasBlueFuzz)
	}
	if !font.HasForceBold || !font.ForceBold {
		t.Fatalf("ForceBold = (%v, %v), want present true", font.ForceBold, font.HasForceBold)
	}
	if !font.HasLanguageGroup || font.LanguageGroup != 1 {
		t.Fatalf("LanguageGroup = (%d, %v), want present 1", font.LanguageGroup, font.HasLanguageGroup)
	}
}

func TestParseFontPrivateHintsIgnoreMalformedOptionalValues(t *testing.T) {
	private := privateHintTestPrivate(`
/BlueValues [0 nope 10] def
/StdHW 70 def
/BlueScale nope def
/BlueShift 7.5 def
/BlueFuzz bad def
/ForceBold maybe def
/LanguageGroup one def
/StemSnapV [80 90] def
`, []byte{14})

	font, err := ParseFont(privateHintTestPFA(private))
	if err != nil {
		t.Fatalf("ParseFont failed: %v", err)
	}
	if font.BlueValues != nil {
		t.Fatalf("BlueValues = %v, want malformed value ignored", font.BlueValues)
	}
	if font.StdHW != nil {
		t.Fatalf("StdHW = %v, want malformed scalar form ignored", font.StdHW)
	}
	if font.HasBlueScale || font.HasBlueShift || font.HasBlueFuzz || font.HasForceBold || font.HasLanguageGroup {
		t.Fatalf("malformed scalar hint fields were marked present: %+v", font)
	}
	privateHintWantFloats(t, "StemSnapV", font.StemSnapV, []float64{80, 90})
	if _, ok := font.CharStrings["A"]; !ok {
		t.Fatal("valid CharStrings data was not parsed")
	}
}

func TestParseFontPrivateHintsSkipCharStringBinaryData(t *testing.T) {
	charString := []byte("/BlueValues [1 2] def")
	private := privateHintTestPrivate("", charString)

	font, err := ParseFont(privateHintTestPFA(private))
	if err != nil {
		t.Fatalf("ParseFont failed: %v", err)
	}
	if font.BlueValues != nil {
		t.Fatalf("BlueValues = %v, want metadata inside charstring bytes ignored", font.BlueValues)
	}
	if !bytes.Equal(font.CharStrings["A"], charString) {
		t.Fatalf("A charstring = %q, want %q", font.CharStrings["A"], charString)
	}
}

func privateHintTestPrivate(metadata string, charString []byte) []byte {
	private := []byte("/Private 32 dict dup begin\n/lenIV -1 def\n")
	private = append(private, metadata...)
	private = append(private, []byte(fmt.Sprintf("/CharStrings 1 dict dup begin\n/A %d RD ", len(charString)))...)
	private = append(private, charString...)
	private = append(private, []byte(" ND\nend\nend\n")...)
	return private
}

func privateHintTestPFA(private []byte) []byte {
	pfa := []byte(`%!PS-AdobeFont-1.0: PrivateHints 1.0
/FontName /PrivateHints def
currentfile eexec
`)
	pfa = append(pfa, []byte(hex.EncodeToString(privateHintEncryptedEexec(private)))...)
	pfa = append(pfa, []byte("\n0000000000000000\ncleartomark\n")...)
	return pfa
}

func privateHintEncryptedEexec(private []byte) []byte {
	plain := append([]byte{1, 2, 3, 4}, private...)
	return privateHintEncryptType1Bytes(plain, 55665)
}

func privateHintEncryptType1Bytes(data []byte, seed uint16) []byte {
	const c1 = 52845
	const c2 = 22719

	out := make([]byte, len(data))
	r := seed
	for i, plain := range data {
		cipher := plain ^ byte(r>>8)
		out[i] = cipher
		r = (uint16(cipher)+r)*c1 + c2
	}
	return out
}

func privateHintWantFloats(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}
