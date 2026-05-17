package cache

import (
	"container/list"
	"sync"

	"github.com/dh-kam/freetype-go/api"
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

// Glyph represents a cached glyph object.
type Glyph struct {
	Outline api.Outline
	Advance api.Vector
}

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
