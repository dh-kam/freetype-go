package cache

import (
	"testing"
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
