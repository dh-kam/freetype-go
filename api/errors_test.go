package api

import (
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestErrorToCodeMapsStandardErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want FT_Err
	}{
		{name: "nil", err: nil, want: FT_Err_Ok},
		{name: "eof", err: io.EOF, want: FT_Err_Invalid_File_Format},
		{name: "unexpected-eof", err: io.ErrUnexpectedEOF, want: FT_Err_Invalid_File_Format},
		{name: "not-exist", err: os.ErrNotExist, want: FT_Err_Cannot_Open_Resource},
		{name: "permission", err: os.ErrPermission, want: FT_Err_Cannot_Open_Resource},
		{name: "fallback", err: errors.New("bad input"), want: FT_Err_Invalid_Argument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorToCode(tt.err); got != tt.want {
				t.Fatalf("ErrorToCode(%v) = %#x, want %#x", tt.err, got, tt.want)
			}
		})
	}
}

func TestCodedErrorPreservesCodeAndUnwrap(t *testing.T) {
	base := errors.New("missing driver")
	err := NewError(FT_Err_Missing_Module, base)

	if got := ErrorToCode(err); got != FT_Err_Missing_Module {
		t.Fatalf("ErrorToCode(coded) = %#x, want %#x", got, FT_Err_Missing_Module)
	}
	if !errors.Is(err, base) {
		t.Fatalf("coded error does not unwrap base error")
	}
	if err.Error() != base.Error() {
		t.Fatalf("Error() = %q, want %q", err.Error(), base.Error())
	}
}

func TestErrorToCodeFindsWrappedCodedError(t *testing.T) {
	err := fmt.Errorf("load failed: %w", NewError(FT_Err_Invalid_Glyph_Index, errors.New("glyph")))
	if got := ErrorToCode(err); got != FT_Err_Invalid_Glyph_Index {
		t.Fatalf("ErrorToCode(wrapped coded) = %#x, want %#x", got, FT_Err_Invalid_Glyph_Index)
	}
}

func TestOptionalGlyphSlotMetricsProvider(t *testing.T) {
	metrics := GlyphMetrics{Width: 64, HoriAdvance: 128}
	slot := metricsSlot{metrics: metrics}

	got, ok := GetGlyphSlotMetrics(slot)
	if !ok {
		t.Fatal("GetGlyphSlotMetrics returned !ok")
	}
	if got != metrics {
		t.Fatalf("metrics = %+v, want %+v", got, metrics)
	}
	if _, ok := GetGlyphSlotMetrics(nil); ok {
		t.Fatal("nil slot returned metrics")
	}
	if _, ok := GetGlyphSlotMetrics(emptySlot{}); ok {
		t.Fatal("slot without provider returned metrics")
	}
}

type emptySlot struct{}

func (emptySlot) GetOutline() Outline        { return nil }
func (emptySlot) SetOutline(outline Outline) {}
func (emptySlot) GetBitmap() Bitmap          { return nil }
func (emptySlot) GetImage() *Image           { return nil }

type metricsSlot struct {
	emptySlot
	metrics GlyphMetrics
}

func (s metricsSlot) GetMetrics() (GlyphMetrics, bool) {
	return s.metrics, true
}
