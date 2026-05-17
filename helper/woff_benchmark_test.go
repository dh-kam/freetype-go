package helper

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/dh-kam/freetype-go/core"
)

var (
	benchmarkWOFF2Size int64
	benchmarkWOFF2Err  error
)

func BenchmarkDecodeWOFF2Synthetic(b *testing.B) {
	b.Run("transformed_glyf_loca", func(b *testing.B) {
		data := benchmarkWOFF2WithTransformedGlyfLoca(b, testWOFF2TransformedSimpleGlyf())
		stream := core.NewMemoryStream(data)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := DecodeWOFF2(stream)
			if err != nil {
				b.Fatalf("DecodeWOFF2 failed: %v", err)
			}
			benchmarkWOFF2Size = out.Size()
		}
	})

	b.Run("small_oversize_guard", func(b *testing.B) {
		stream := core.NewMemoryStream(benchmarkWOFF2OversizeHeader())

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := DecodeWOFF2(stream)
			if err == nil {
				b.Fatal("DecodeWOFF2 unexpectedly accepted oversized header")
			}
			if !errors.Is(err, errDecompressedDataTooLarge) {
				benchmarkWOFF2Err = err
			}
		}
	})
}

func benchmarkWOFF2WithTransformedGlyfLoca(tb testing.TB, transformedGlyf []byte) []byte {
	tb.Helper()

	var directory bytes.Buffer
	directory.WriteByte(byte(10)) // glyf, transform version 0
	directory.Write(testBase128(20))
	directory.Write(testBase128(uint32(len(transformedGlyf))))
	directory.WriteByte(byte(11)) // loca, transform version 0
	directory.Write(testBase128(4))
	directory.Write(testBase128(0))

	var compressed bytes.Buffer
	bw := brotli.NewWriter(&compressed)
	if _, err := bw.Write(transformedGlyf); err != nil {
		tb.Fatalf("brotli write failed: %v", err)
	}
	if err := bw.Close(); err != nil {
		tb.Fatalf("brotli close failed: %v", err)
	}

	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], 0x00010000)
	binary.BigEndian.PutUint32(header[8:12], uint32(48+directory.Len()+compressed.Len()))
	binary.BigEndian.PutUint16(header[12:14], 2)
	binary.BigEndian.PutUint32(header[16:20], 68)
	binary.BigEndian.PutUint32(header[20:24], uint32(compressed.Len()))

	out := append(header, directory.Bytes()...)
	out = append(out, compressed.Bytes()...)
	return out
}

func benchmarkWOFF2OversizeHeader() []byte {
	header := make([]byte, 48)
	binary.BigEndian.PutUint32(header[0:4], 0x774F4632) // wOF2
	binary.BigEndian.PutUint32(header[4:8], 0x00010000)
	binary.BigEndian.PutUint32(header[8:12], 48)
	binary.BigEndian.PutUint32(header[16:20], maxDecodedFontSize+1)
	return header
}
