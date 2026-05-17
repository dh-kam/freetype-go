package helper

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"testing"

	"github.com/dh-kam/freetype-go/core"
)

func TestCompressGzip(t *testing.T) {
	original := []byte("Hello FreeType Go Gzip!")
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(original)
	w.Close()

	stream := core.NewMemoryStream(buf.Bytes())
	outStream, err := NewGzipStream(stream)
	if err != nil {
		t.Fatalf("NewGzipStream failed: %v", err)
	}

	outData := make([]byte, outStream.Size())
	if _, err := outStream.ReadAt(outData, 0); err != nil && err != io.EOF {
		t.Fatalf("ReadAt failed: %v", err)
	}

	if !bytes.Equal(original, outData) {
		t.Errorf("Mismatch. Expected %q, got %q", original, outData)
	}
}

func TestCompressBzip2(t *testing.T) {
	// bzip2 doesn't have a writer in standard library, so we will skip a full roundtrip
	// or mock with a pre-compressed known byte slice.
	// For now just ensure it compiles and returns error on invalid stream.
	stream := core.NewMemoryStream([]byte("invalid bzip2"))
	_, err := NewBzip2Stream(stream)
	if err == nil {
		t.Error("Expected error for invalid bzip2 stream")
	}
}

func TestCompressZlib(t *testing.T) {
	original := []byte("Hello FreeType Go Zlib!")
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(original)
	w.Close()

	stream := core.NewMemoryStream(buf.Bytes())
	outStream, err := NewZlibStream(stream)
	if err != nil {
		t.Fatalf("NewZlibStream failed: %v", err)
	}

	outData := make([]byte, outStream.Size())
	if _, err := outStream.ReadAt(outData, 0); err != nil && err != io.EOF {
		t.Fatalf("ReadAt failed: %v", err)
	}

	if !bytes.Equal(original, outData) {
		t.Errorf("Mismatch. Expected %q, got %q", original, outData)
	}
}

func TestCompressGzipRejectsOversizeOutput(t *testing.T) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	writeZeros(t, w, maxDecompressedStreamSize+1)
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	stream := core.NewMemoryStream(buf.Bytes())
	_, err := NewGzipStream(stream)
	if !errors.Is(err, errDecompressedDataTooLarge) {
		t.Fatalf("Expected oversize error, got %v", err)
	}
}

func TestCompressZlibRejectsOversizeOutput(t *testing.T) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	writeZeros(t, w, maxDecompressedStreamSize+1)
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	stream := core.NewMemoryStream(buf.Bytes())
	_, err := NewZlibStream(stream)
	if !errors.Is(err, errDecompressedDataTooLarge) {
		t.Fatalf("Expected oversize error, got %v", err)
	}
}

func writeZeros(t *testing.T, w io.Writer, n int64) {
	t.Helper()

	chunk := make([]byte, 32*1024)
	for n > 0 {
		size := int64(len(chunk))
		if n < size {
			size = n
		}
		if _, err := w.Write(chunk[:int(size)]); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		n -= size
	}
}
