package cache

import (
	"container/list"
	"errors"
	"sync"

	"github.com/dh-kam/freetype-go/api"
)

var (
	ErrNilFace          = errors.New("nil face")
	ErrNilFaceRequester = errors.New("nil face requester")
)

// FaceID is a unique identifier for a font face.
type FaceID interface{}

// SizeRequest represents the parameters for a font size.
type SizeRequest struct {
	FaceID FaceID
	Width  int
	Height int
	HRes   int
	VRes   int
}

// GlyphRequest represents the parameters for a glyph.
type GlyphRequest struct {
	SizeID     interface{}
	GlyphIndex int
	LoadFlags  int
}

// Size represents a cached face size selection.
type Size struct {
	Request SizeRequest
	Face    api.Face
}

// Glyph represents a cached glyph object.
type Glyph struct {
	Outline    api.Outline
	Bitmap     api.Bitmap
	Image      *api.Image
	Advance    api.Vector
	Metrics    api.GlyphMetrics
	HasMetrics bool
}

// FaceRequester lazily resolves a face for a cache manager.
type FaceRequester func(id FaceID) (api.Face, error)

// Cache is a generic LRU cache.
type Cache struct {
	capacity int
	list     *list.List
	items    map[interface{}]*list.Element
	lock     sync.Mutex
}

type entry struct {
	key   interface{}
	value interface{}
}

func NewCache(capacity int) *Cache {
	return &Cache{
		capacity: capacity,
		list:     list.New(),
		items:    make(map[interface{}]*list.Element),
	}
}

func (c *Cache) Get(key interface{}) (interface{}, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if elem, ok := c.items[key]; ok {
		c.list.MoveToFront(elem)
		return elem.Value.(*entry).value, true
	}
	return nil, false
}

func (c *Cache) Add(key interface{}, value interface{}) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.capacity <= 0 {
		return
	}

	if elem, ok := c.items[key]; ok {
		c.list.MoveToFront(elem)
		elem.Value.(*entry).value = value
		return
	}

	if c.list.Len() >= c.capacity {
		c.removeOldest()
	}

	ent := &entry{key, value}
	elem := c.list.PushFront(ent)
	c.items[key] = elem
}

func (c *Cache) removeOldest() {
	elem := c.list.Back()
	if elem != nil {
		c.list.Remove(elem)
		delete(c.items, elem.Value.(*entry).key)
	}
}

func (c *Cache) Remove(key interface{}) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if elem, ok := c.items[key]; ok {
		c.list.Remove(elem)
		delete(c.items, key)
	}
}

func (c *Cache) Len() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.list.Len()
}

// Manager manages multiple caches for faces, sizes, and glyphs.
type Manager struct {
	faceCache  *Cache
	sizeCache  *Cache
	glyphCache *Cache
}

func NewManager(faceCap, sizeCap, glyphCap int) *Manager {
	return &Manager{
		faceCache:  NewCache(faceCap),
		sizeCache:  NewCache(sizeCap),
		glyphCache: NewCache(glyphCap),
	}
}

func (m *Manager) LookupFace(id FaceID) (api.Face, bool) {
	val, ok := m.faceCache.Get(id)
	if !ok {
		return nil, false
	}
	return val.(api.Face), true
}

func (m *Manager) AddFace(id FaceID, face api.Face) {
	m.faceCache.Add(id, face)
}

// LookupFaceOrLoad returns a cached face or loads and caches it through requester.
func (m *Manager) LookupFaceOrLoad(id FaceID, requester FaceRequester) (api.Face, error) {
	if face, ok := m.LookupFace(id); ok {
		return face, nil
	}
	if requester == nil {
		return nil, ErrNilFaceRequester
	}
	face, err := requester(id)
	if err != nil {
		return nil, err
	}
	m.AddFace(id, face)
	return face, nil
}

func (m *Manager) LookupSize(req SizeRequest) (*Size, bool) {
	val, ok := m.sizeCache.Get(req)
	if !ok {
		return nil, false
	}
	return val.(*Size), true
}

func (m *Manager) AddSize(req SizeRequest, size *Size) {
	m.sizeCache.Add(req, size)
}

// LookupSizeOrCreate selects a face size and caches the resulting request.
func (m *Manager) LookupSizeOrCreate(req SizeRequest, face api.Face) (*Size, error) {
	if size, ok := m.LookupSize(req); ok {
		return size, nil
	}
	if face == nil {
		return nil, ErrNilFace
	}
	if err := face.SetPixelSizes(req.Width, req.Height); err != nil {
		return nil, err
	}
	size := &Size{
		Request: req,
		Face:    face,
	}
	m.AddSize(req, size)
	return size, nil
}

func (m *Manager) LookupGlyph(req GlyphRequest) (*Glyph, bool) {
	val, ok := m.glyphCache.Get(req)
	if !ok {
		return nil, false
	}
	return val.(*Glyph), true
}

func (m *Manager) AddGlyph(req GlyphRequest, glyph *Glyph) {
	m.glyphCache.Add(req, glyph)
}

// LookupGlyphOrLoad returns a cached glyph or loads it from face and caches the slot data.
func (m *Manager) LookupGlyphOrLoad(req GlyphRequest, face api.Face) (*Glyph, error) {
	if glyph, ok := m.LookupGlyph(req); ok {
		return glyph, nil
	}
	if face == nil {
		return nil, ErrNilFace
	}
	slot, err := face.LoadGlyph(req.GlyphIndex, req.LoadFlags)
	if err != nil {
		return nil, err
	}
	glyph := glyphFromSlot(face, req.GlyphIndex, slot)
	m.AddGlyph(req, glyph)
	return glyph, nil
}

func glyphFromSlot(face api.Face, glyphIndex int, slot api.GlyphSlot) *Glyph {
	glyph := &Glyph{}
	if slot != nil {
		glyph.Outline = slot.GetOutline()
		glyph.Bitmap = slot.GetBitmap()
		glyph.Image = slot.GetImage()
		if metrics, ok := api.GetGlyphSlotMetrics(slot); ok {
			glyph.Metrics = metrics
			glyph.HasMetrics = true
			glyph.Advance.X = metrics.HoriAdvance
		}
	}
	if !glyph.HasMetrics && face != nil {
		if advance, _, err := face.GetGlyphMetrics(glyphIndex); err == nil {
			glyph.Advance.X = advance
		}
	}
	return glyph
}
