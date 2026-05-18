package type1

import "testing"

const type1FuzzMaxInput = 4096

func FuzzParseFont(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("not a Type 1 font"))
	f.Add([]byte("%!PS-AdobeFont-1.0: Missing 1.0\n/FontName /Missing def\n"))
	f.Add(type1RobustnessPFA(type1RobustnessValidPrivate()))
	f.Add(type1RobustnessPFB(type1RobustnessValidPrivate()))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > type1FuzzMaxInput {
			return
		}
		type1FuzzNoPanic(t, "ParseFont", func() {
			_, _ = ParseFont(data)
		})
	})
}

func FuzzExtractEexec(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("currentfile eexec"))
	f.Add([]byte("currentfile eexec abc"))
	f.Add(append([]byte("currentfile eexec\n"), type1RobustnessEncryptedEexec(type1RobustnessValidPrivate())...))
	f.Add(type1RobustnessPFA(type1RobustnessValidPrivate()))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > type1FuzzMaxInput {
			return
		}
		type1FuzzNoPanic(t, "ExtractEexec", func() {
			_, _ = ExtractEexec(data)
		})
	})
}

func FuzzDecodePFB(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("plain PFA bytes"))
	f.Add([]byte{0x80})
	f.Add([]byte{0x80, 1, 0x04, 0, 0, 0, 't', 'e'})
	f.Add(type1RobustnessPFB(type1RobustnessValidPrivate()))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > type1FuzzMaxInput {
			return
		}
		type1FuzzNoPanic(t, "DecodePFB", func() {
			_, _ = DecodePFB(data)
		})
	})
}

func FuzzDecodeCharString(f *testing.F) {
	f.Add([]byte{139, 248, 136, 13, 14})            // 0 500 hsbw endchar
	f.Add([]byte{139, 10, 14})                      // 0 callsubr endchar
	f.Add([]byte{12})                               // truncated escaped operator
	f.Add([]byte{247})                              // truncated positive number
	f.Add([]byte{255, 0, 0, 0})                     // truncated long number
	f.Add([]byte{2})                                // unsupported operator
	f.Add([]byte{0xde, 0xad, 0xbe, 0xef, 12, 0x22}) // small random bytes

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > type1FuzzMaxInput {
			return
		}
		type1FuzzNoPanic(t, "DecodeCharString", func() {
			_, _ = DecodeCharString(data, [][]byte{{11}})
		})
	})
}

func type1FuzzNoPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s panicked: %v", name, r)
		}
	}()
	fn()
}
