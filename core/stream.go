package core

import (
	"errors"
	"io"
	"os"

	"github.com/dh-kam/freetype-go/api"
)

// MemoryStream implements api.Stream using a byte slice.
type MemoryStream struct {
	data []byte
}

func NewMemoryStream(data []byte) *MemoryStream {
	return &MemoryStream{data: data}
}

func (s *MemoryStream) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	if off >= int64(len(s.data)) {
		return 0, io.EOF
	}
	n := copy(p, s.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (s *MemoryStream) Size() int64 {
	return int64(len(s.data))
}

// FileStream implements api.Stream using an os.File.
type FileStream struct {
	file *os.File
	size int64
}

func NewFileStream(file *os.File) (*FileStream, error) {
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return &FileStream{
		file: file,
		size: stat.Size(),
	}, nil
}

func (s *FileStream) ReadAt(p []byte, off int64) (int, error) {
	return s.file.ReadAt(p, off)
}

func (s *FileStream) Size() int64 {
	return s.size
}

var _ api.Stream = (*MemoryStream)(nil)
var _ api.Stream = (*FileStream)(nil)
