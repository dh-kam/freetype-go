package core

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestMemoryStreamReadAtAndSize(t *testing.T) {
	stream := NewMemoryStream([]byte("abc"))
	buf := make([]byte, 2)

	n, err := stream.ReadAt(buf, 1)
	if err != nil {
		t.Fatalf("ReadAt full read failed: %v", err)
	}
	if n != 2 || string(buf) != "bc" {
		t.Fatalf("ReadAt full read = n %d buf %q, want 2 bc", n, string(buf))
	}

	n, err = stream.ReadAt(buf, 2)
	if !errors.Is(err, io.EOF) || n != 1 || buf[0] != 'c' {
		t.Fatalf("partial ReadAt = n %d err %v buf %q, want 1 EOF c", n, err, string(buf))
	}
	if _, err := stream.ReadAt(buf, -1); err == nil {
		t.Fatal("negative offset returned nil error")
	}
	if _, err := stream.ReadAt(buf, 3); !errors.Is(err, io.EOF) {
		t.Fatalf("past-end ReadAt err = %v, want EOF", err)
	}
	if stream.Size() != 3 {
		t.Fatalf("Size = %d, want 3", stream.Size())
	}
}

func TestFileStreamReadAtAndSize(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "font-*.bin")
	if err != nil {
		t.Fatalf("CreateTemp failed: %v", err)
	}
	if _, err := file.WriteString("font"); err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek failed: %v", err)
	}

	stream, err := NewFileStream(file)
	if err != nil {
		t.Fatalf("NewFileStream failed: %v", err)
	}
	if stream.Size() != 4 {
		t.Fatalf("Size = %d, want 4", stream.Size())
	}

	buf := make([]byte, 2)
	n, err := stream.ReadAt(buf, 1)
	if err != nil || n != 2 || string(buf) != "on" {
		t.Fatalf("ReadAt = n %d err %v buf %q, want 2 nil on", n, err, string(buf))
	}
}

func TestOutlineScaleTranslateAndAccessors(t *testing.T) {
	outline := &Outline{
		Points:   []api.Vector{{X: 64, Y: -128}, {X: -32, Y: 96}},
		Tags:     []byte{1, 0},
		Contours: []int{1},
	}

	if len(outline.GetPoints()) != 2 || len(outline.GetTags()) != 2 || len(outline.GetContours()) != 1 {
		t.Fatal("outline accessors returned unexpected lengths")
	}

	outline.Scale(2<<16, 1<<15)
	outline.Translate(10, -20)

	want := []api.Vector{{X: 138, Y: -84}, {X: -54, Y: 28}}
	for i, got := range outline.Points {
		if got != want[i] {
			t.Fatalf("point %d = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestSystemImageDecoderRoundTrip(t *testing.T) {
	sys := NewSystemWithServices(nil, nil, nil)
	decoder := coreTestImageDecoder{}

	if sys.GetImageDecoder() != nil {
		t.Fatal("new system returned image decoder")
	}
	sys.SetImageDecoder(decoder)
	if sys.GetImageDecoder() != decoder {
		t.Fatal("GetImageDecoder did not return configured decoder")
	}
}

type coreTestImageDecoder struct{}

func (coreTestImageDecoder) Decode(data []byte) (*api.Image, error) {
	return &api.Image{Width: len(data), Height: 1, Pixels: data}, nil
}
