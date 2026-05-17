package core

import (
	"testing"
)

func TestNewSystem(t *testing.T) {
	sys := NewSystem()
	if sys == nil {
		t.Fatal("NewSystem() returned nil")
	}

	math := sys.Math()
	if math == nil {
		t.Error("sys.Math() returned nil")
	}

	if sys.Rasterizer() != nil {
		t.Error("sys.Rasterizer() should be nil")
	}

	if sys.Hinter() != nil {
		t.Error("sys.Hinter() should be nil")
	}
}
