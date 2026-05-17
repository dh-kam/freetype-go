package fuzz

import (
	"encoding/binary"
	"testing"

	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/helper"
)

const maxDecodedFuzzFontSize = 64 << 20

func FuzzDecodeWOFFWrappedStreams(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("not a wrapped font"))
	f.Add(minimalWOFFFuzzSeed())
	f.Add(minimalWOFF2FuzzSeed())

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip("keeping WOFF fuzz smoke inputs bounded")
		}

		out, err := helper.DecodeWOFFIfNeeded(core.NewMemoryStream(data))
		if err != nil {
			return
		}
		if out == nil {
			t.Fatal("DecodeWOFFIfNeeded returned nil stream without error")
		}
		if out.Size() > maxDecodedFuzzFontSize {
			t.Fatalf("decoded stream too large: %d", out.Size())
		}
	})
}

func minimalWOFFFuzzSeed() []byte {
	data := make([]byte, 44)
	binary.BigEndian.PutUint32(data[0:4], 0x774f4646) // wOFF
	binary.BigEndian.PutUint32(data[4:8], 0x00010000)
	binary.BigEndian.PutUint32(data[8:12], uint32(len(data)))
	binary.BigEndian.PutUint16(data[12:14], 0)
	binary.BigEndian.PutUint32(data[16:20], 12)
	return data
}

func minimalWOFF2FuzzSeed() []byte {
	data := make([]byte, 48)
	binary.BigEndian.PutUint32(data[0:4], 0x774f4632) // wOF2
	binary.BigEndian.PutUint32(data[4:8], 0x00010000)
	binary.BigEndian.PutUint32(data[8:12], uint32(len(data)))
	binary.BigEndian.PutUint16(data[12:14], 0)
	binary.BigEndian.PutUint32(data[16:20], 12)
	binary.BigEndian.PutUint32(data[20:24], 0)
	return data
}
