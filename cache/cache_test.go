package cache

import (
	"testing"

	"github.com/dh-kam/freetype-go/api"
)

func TestCacheEviction(t *testing.T) {
	c := NewCache(2)

	c.Add("a", 1)
	c.Add("b", 2)
	if c.Len() != 2 {
		t.Errorf("expected length 2, got %d", c.Len())
	}

	c.Add("c", 3)
	if c.Len() != 2 {
		t.Errorf("expected length 2 after eviction, got %d", c.Len())
	}

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be evicted")
	}

	if v, ok := c.Get("b"); !ok || v.(int) != 2 {
		t.Error("expected 'b' to be present with value 2")
	}

	if v, ok := c.Get("c"); !ok || v.(int) != 3 {
		t.Error("expected 'c' to be present with value 3")
	}

	// Move 'b' to front
	c.Get("b")
	c.Add("d", 4)

	if _, ok := c.Get("c"); ok {
		t.Error("expected 'c' to be evicted after 'b' was accessed")
	}
}

func TestCacheZeroCapacityDoesNotStore(t *testing.T) {
	c := NewCache(0)
	c.Add("a", 1)
	if c.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", c.Len())
	}
	if _, ok := c.Get("a"); ok {
		t.Fatal("zero-capacity cache stored a value")
	}
}

type managerTestFace struct {
	setSizes []SizeRequest
	loads    int
	slot     api.GlyphSlot
}

func (f *managerTestFace) GetNumGlyphs() int { return 1 }

func (f *managerTestFace) SetPixelSizes(width, height int) error {
	f.setSizes = append(f.setSizes, SizeRequest{Width: width, Height: height})
	return nil
}

func (f *managerTestFace) LoadGlyph(glyphIndex int, loadFlags int) (api.GlyphSlot, error) {
	f.loads++
	return f.slot, nil
}

func (f *managerTestFace) GetGlyphSlot() api.GlyphSlot { return f.slot }
func (f *managerTestFace) GetUnitsPerEm() uint16       { return 1000 }

func (f *managerTestFace) GetGlyphIndex(char rune) (int, error) {
	return int(char), nil
}

func (f *managerTestFace) GetGlyphMetrics(glyphIndex int) (advance int32, lsb int32, err error) {
	return 640, 0, nil
}

func (f *managerTestFace) Shape(text string) ([]int, []api.Vector) {
	return nil, nil
}

type managerTestSlot struct {
	metrics api.GlyphMetrics
}

func (managerTestSlot) GetOutline() api.Outline { return nil }
func (managerTestSlot) SetOutline(api.Outline)  {}
func (managerTestSlot) GetBitmap() api.Bitmap   { return nil }
func (managerTestSlot) GetImage() *api.Image    { return nil }

func (s managerTestSlot) GetMetrics() (api.GlyphMetrics, bool) {
	return s.metrics, true
}

func TestManagerLookupFaceOrLoadCachesRequesterResult(t *testing.T) {
	m := NewManager(2, 2, 2)
	face := &managerTestFace{}
	loads := 0
	requester := func(id FaceID) (api.Face, error) {
		loads++
		return face, nil
	}

	first, err := m.LookupFaceOrLoad("font.ttf", requester)
	if err != nil {
		t.Fatalf("first LookupFaceOrLoad failed: %v", err)
	}
	second, err := m.LookupFaceOrLoad("font.ttf", requester)
	if err != nil {
		t.Fatalf("second LookupFaceOrLoad failed: %v", err)
	}
	if first != second || first != face {
		t.Fatal("cached face mismatch")
	}
	if loads != 1 {
		t.Fatalf("requester calls = %d, want 1", loads)
	}
}

func TestManagerLookupSizeOrCreateSelectsAndCachesSize(t *testing.T) {
	m := NewManager(2, 2, 2)
	face := &managerTestFace{}
	req := SizeRequest{FaceID: "font.ttf", Width: 12, Height: 18, HRes: 72, VRes: 72}

	first, err := m.LookupSizeOrCreate(req, face)
	if err != nil {
		t.Fatalf("first LookupSizeOrCreate failed: %v", err)
	}
	second, err := m.LookupSizeOrCreate(req, face)
	if err != nil {
		t.Fatalf("second LookupSizeOrCreate failed: %v", err)
	}
	if first != second {
		t.Fatal("cached size mismatch")
	}
	if first.Face != face || first.Request != req {
		t.Fatal("cached size did not retain face and request")
	}
	if len(face.setSizes) != 1 || face.setSizes[0].Width != 12 || face.setSizes[0].Height != 18 {
		t.Fatalf("SetPixelSizes calls = %+v, want one 12x18 call", face.setSizes)
	}
}

func TestManagerLookupGlyphOrLoadCachesSlotMetrics(t *testing.T) {
	m := NewManager(2, 2, 2)
	face := &managerTestFace{
		slot: managerTestSlot{metrics: api.GlyphMetrics{HoriAdvance: 768}},
	}
	req := GlyphRequest{SizeID: "font.ttf@12", GlyphIndex: 7, LoadFlags: api.LoadNoHinting}

	first, err := m.LookupGlyphOrLoad(req, face)
	if err != nil {
		t.Fatalf("first LookupGlyphOrLoad failed: %v", err)
	}
	second, err := m.LookupGlyphOrLoad(req, face)
	if err != nil {
		t.Fatalf("second LookupGlyphOrLoad failed: %v", err)
	}
	if first != second {
		t.Fatal("cached glyph mismatch")
	}
	if face.loads != 1 {
		t.Fatalf("LoadGlyph calls = %d, want 1", face.loads)
	}
	if !first.HasMetrics || first.Metrics.HoriAdvance != 768 || first.Advance.X != 768 {
		t.Fatalf("cached glyph metrics = %+v advance=%+v has=%v, want HoriAdvance 768", first.Metrics, first.Advance, first.HasMetrics)
	}
}
