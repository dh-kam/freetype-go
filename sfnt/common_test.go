package sfnt

import (
	"github.com/dh-kam/freetype-go/api"
)

type mockStream struct {
	data []byte
}

func (m *mockStream) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 || off >= int64(len(m.data)) {
		return 0, nil
	}
	n = copy(p, m.data[off:])
	return n, nil
}

func (m *mockStream) Size() int64 {
	return int64(len(m.data))
}

type mockSystem struct{}

func (s *mockSystem) Math() api.MathEngine               { return nil }
func (s *mockSystem) Rasterizer() api.Rasterizer         { return nil }
func (s *mockSystem) Hinter() api.Hinter                 { return nil }
func (s *mockSystem) GetImageDecoder() api.ImageDecoder  { return nil }
func (s *mockSystem) SetImageDecoder(d api.ImageDecoder) {}
