package helper

import (
	"compress/bzip2"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"

	"github.com/dh-kam/freetype-go/api"
	"github.com/dh-kam/freetype-go/core"
)

const (
	maxDecompressedStreamSize = 64 << 20
	maxDecodedFontSize        = 64 << 20
	maxCompressedFontDataSize = 64 << 20
)

var errDecompressedDataTooLarge = errors.New("decompressed data too large")

type readerAtStream struct {
	api.Stream
}

func (s readerAtStream) ReadAt(p []byte, off int64) (n int, err error) {
	return s.Stream.ReadAt(p, off)
}

func streamToReader(s api.Stream) io.Reader {
	return io.NewSectionReader(readerAtStream{s}, 0, s.Size())
}

func readAllLimited(r io.Reader, limit int64) ([]byte, error) {
	lr := &io.LimitedReader{R: r, N: limit + 1}
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errDecompressedDataTooLarge
	}
	return data, nil
}

// NewGzipStream wraps the input stream and transparently decompresses it using gzip.
func NewGzipStream(in api.Stream) (api.Stream, error) {
	r, err := gzip.NewReader(streamToReader(in))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	data, err := readAllLimited(r, maxDecompressedStreamSize)
	if err != nil {
		return nil, err
	}
	return core.NewMemoryStream(data), nil
}

// NewBzip2Stream wraps the input stream and transparently decompresses it using bzip2.
func NewBzip2Stream(in api.Stream) (api.Stream, error) {
	r := bzip2.NewReader(streamToReader(in))
	data, err := readAllLimited(r, maxDecompressedStreamSize)
	if err != nil {
		return nil, err
	}
	return core.NewMemoryStream(data), nil
}

// NewZlibStream wraps the input stream and transparently decompresses it using zlib.
func NewZlibStream(in api.Stream) (api.Stream, error) {
	r, err := zlib.NewReader(streamToReader(in))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	data, err := readAllLimited(r, maxDecompressedStreamSize)
	if err != nil {
		return nil, err
	}
	return core.NewMemoryStream(data), nil
}
