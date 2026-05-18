package type1

import (
	"encoding/hex"
	"testing"
)

func TestParserRobustnessRejectsMalformedPFBBlockLengths(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "truncated length field", data: []byte{0x80, 1, 0x01, 0x00, 0x00}},
		{name: "ascii length exceeds payload", data: []byte{0x80, 1, 0x04, 0x00, 0x00, 0x00, 'a', 'b'}},
		{name: "binary length exceeds payload", data: []byte{0x80, 2, 0x02, 0x00, 0x00, 0x00, 0x00}},
		{name: "second block length exceeds payload", data: append(type1RobustnessPFBBlock(1, []byte("ok")), 0x80, 2, 3, 0, 0, 0, 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type1AssertRejectsWithoutPanic(t, "DecodePFB "+tt.name, func() error {
				_, err := DecodePFB(tt.data)
				return err
			})
		})
	}
}

func TestParserRobustnessRejectsMissingEexec(t *testing.T) {
	data := []byte("%!PS-AdobeFont-1.0: Missing 1.0\n/FontName /Missing def\n")

	tests := []struct {
		name string
		fn   func() error
	}{
		{name: "ExtractEexec", fn: func() error {
			_, err := ExtractEexec(data)
			return err
		}},
		{name: "ParseFont", fn: func() error {
			_, err := ParseFont(data)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type1AssertRejectsWithoutPanic(t, tt.name, tt.fn)
		})
	}
}

func TestParserRobustnessRejectsInvalidEexecData(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{name: "short odd hex", fn: func() error {
			_, err := ExtractEexec([]byte("currentfile eexec abc"))
			return err
		}},
		{name: "short binary", fn: func() error {
			_, err := ExtractEexec([]byte("currentfile eexec \x00\x01\x02"))
			return err
		}},
		{name: "garbled hex private dict", fn: func() error {
			_, err := ParseFont([]byte("%!PS-AdobeFont-1.0: Bad 1.0\n/FontName /Bad def\ncurrentfile eexec\n00000000\n"))
			return err
		}},
		{name: "garbled binary private dict", fn: func() error {
			data := append([]byte("%!PS-AdobeFont-1.0: Bad 1.0\n/FontName /Bad def\ncurrentfile eexec\n"), 0x00, 0x01, 0x02, 0x03)
			_, err := ParseFont(data)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type1AssertRejectsWithoutPanic(t, tt.name, tt.fn)
		})
	}
}

func TestParserRobustnessRejectsMalformedRDLengths(t *testing.T) {
	tests := []struct {
		name    string
		private []byte
	}{
		{
			name:    "negative charstring length",
			private: []byte("/Private 8 dict dup begin\n/lenIV -1 def\n/CharStrings 1 dict dup begin\n/A -1 RD abc ND\nend\nend\n"),
		},
		{
			name:    "non-numeric charstring length",
			private: []byte("/Private 8 dict dup begin\n/lenIV -1 def\n/CharStrings 1 dict dup begin\n/A bogus RD abc ND\nend\nend\n"),
		},
		{
			name:    "charstring length exceeds input",
			private: []byte("/Private 8 dict dup begin\n/lenIV -1 def\n/CharStrings 1 dict dup begin\n/A 12 RD abc"),
		},
		{
			name:    "subr negative length",
			private: []byte("/Private 8 dict dup begin\n/lenIV -1 def\n/Subrs 1 array\ndup 0 -1 RD abc NP\n/CharStrings 1 dict dup begin\n/A 1 RD \x0e ND\nend\nend\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type1AssertRejectsWithoutPanic(t, "ParseFont "+tt.name, func() error {
				_, err := ParseFont(type1RobustnessPFA(tt.private))
				return err
			})
		})
	}
}

func TestParserRobustnessRejectsNilAndShortData(t *testing.T) {
	tests := []struct {
		name string
		fn   func() error
	}{
		{name: "DecodePFB nil", fn: func() error {
			_, err := DecodePFB(nil)
			return err
		}},
		{name: "DecodePFB short PFB header", fn: func() error {
			_, err := DecodePFB([]byte{0x80})
			return err
		}},
		{name: "ParseFont nil", fn: func() error {
			_, err := ParseFont(nil)
			return err
		}},
		{name: "ParseFont short PFB", fn: func() error {
			_, err := ParseFont([]byte{0x80, 1, 1, 0, 0, 0})
			return err
		}},
		{name: "ExtractEexec nil", fn: func() error {
			_, err := ExtractEexec(nil)
			return err
		}},
		{name: "ExtractEexec no payload", fn: func() error {
			_, err := ExtractEexec([]byte("currentfile eexec"))
			return err
		}},
		{name: "DecodeCharString truncated escape", fn: func() error {
			_, err := DecodeCharString([]byte{12}, nil)
			return err
		}},
		{name: "DecodeCharString truncated positive number", fn: func() error {
			_, err := DecodeCharString([]byte{247}, nil)
			return err
		}},
		{name: "DecodeCharString truncated negative number", fn: func() error {
			_, err := DecodeCharString([]byte{251}, nil)
			return err
		}},
		{name: "DecodeCharString truncated long number", fn: func() error {
			_, err := DecodeCharString([]byte{255, 0, 0, 0}, nil)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type1AssertRejectsWithoutPanic(t, tt.name, tt.fn)
		})
	}
}

func type1AssertRejectsWithoutPanic(t *testing.T, name string, fn func() error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s panicked: %v", name, r)
		}
	}()
	if err := fn(); err == nil {
		t.Fatalf("%s unexpectedly succeeded", name)
	}
}

func type1RobustnessValidPrivate() []byte {
	return []byte("/Private 8 dict dup begin\n/lenIV -1 def\n/CharStrings 1 dict dup begin\n/A 1 RD \x0e ND\nend\nend\n")
}

func type1RobustnessPFA(private []byte) []byte {
	out := []byte("%!PS-AdobeFont-1.0: Robust 1.0\n/FontName /Robust def\ncurrentfile eexec\n")
	out = append(out, []byte(hex.EncodeToString(type1RobustnessEncryptedEexec(private)))...)
	out = append(out, []byte("\n0000000000000000\ncleartomark\n")...)
	return out
}

func type1RobustnessPFB(private []byte) []byte {
	clear := []byte("%!PS-AdobeFont-1.0: Robust 1.0\n/FontName /Robust def\ncurrentfile eexec\n")
	return type1RobustnessPFBBlocks(
		type1RobustnessPFBBlock(1, clear),
		type1RobustnessPFBBlock(2, type1RobustnessEncryptedEexec(private)),
		type1RobustnessPFBBlock(1, []byte("\n0000000000000000\ncleartomark\n")),
	)
}

func type1RobustnessPFBBlocks(blocks ...[]byte) []byte {
	var out []byte
	for _, block := range blocks {
		out = append(out, block...)
	}
	out = append(out, 0x80, 3)
	return out
}

func type1RobustnessPFBBlock(blockType byte, data []byte) []byte {
	out := []byte{0x80, blockType, byte(len(data)), byte(len(data) >> 8), byte(len(data) >> 16), byte(len(data) >> 24)}
	out = append(out, data...)
	return out
}

func type1RobustnessEncryptedEexec(private []byte) []byte {
	plain := append([]byte{1, 2, 3, 4}, private...)
	return type1RobustnessEncryptBytes(plain, 55665)
}

func type1RobustnessEncryptBytes(data []byte, seed uint16) []byte {
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
