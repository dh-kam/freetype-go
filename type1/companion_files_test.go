package type1

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadAFMParsesReader(t *testing.T) {
	afm, err := ReadAFM(strings.NewReader(`
StartFontMetrics 4.1
FontName Reader-Regular
StartCharMetrics 1
C 65 ; WX 722 ; N A ; B 9 0 689 674 ;
EndCharMetrics
EndFontMetrics
`))
	if err != nil {
		t.Fatalf("ReadAFM failed: %v", err)
	}
	if afm.FontName != "Reader-Regular" {
		t.Fatalf("FontName = %q, want Reader-Regular", afm.FontName)
	}
	if width, ok := afm.WidthXByName("A"); !ok || width != 722 {
		t.Fatalf("WidthXByName(A) = %v, %v; want 722, true", width, ok)
	}
}

func TestReadPFMParsesReader(t *testing.T) {
	data, _, _ := testPFM()

	pfm, err := ReadPFM(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ReadPFM failed: %v", err)
	}
	if pfm.FaceName != "Demo Sans" {
		t.Fatalf("FaceName = %q, want Demo Sans", pfm.FaceName)
	}
	if width, ok := pfm.WidthByCode('C'); !ok || width != 620 {
		t.Fatalf("WidthByCode('C') = %d, %v; want 620, true", width, ok)
	}
}

func TestReadAFMReadPFMRejectNilReaders(t *testing.T) {
	if afm, err := ReadAFM(nil); err == nil || afm != nil {
		t.Fatalf("ReadAFM(nil) = %v, %v; want nil AFM and error", afm, err)
	}
	if pfm, err := ReadPFM(nil); err == nil || pfm != nil {
		t.Fatalf("ReadPFM(nil) = %v, %v; want nil PFM and error", pfm, err)
	}
}

func TestReadCompanionMetricsParsesProvidedReaders(t *testing.T) {
	pfmData, _, _ := testPFM()
	var encoding [256]string
	encoding[65] = "A"
	encoding[67] = "C"

	metrics, err := ReadCompanionMetrics(strings.NewReader(`
StartFontMetrics 4.1
FontName Reader-Regular
StartCharMetrics 1
C 65 ; WX 722 ; N A ; B 9 0 689 674 ;
EndCharMetrics
EndFontMetrics
`), bytes.NewReader(pfmData), encoding)
	if err != nil {
		t.Fatalf("ReadCompanionMetrics failed: %v", err)
	}
	if metrics.AFM == nil {
		t.Fatal("AFM is nil, want parsed AFM")
	}
	if metrics.PFM == nil {
		t.Fatal("PFM is nil, want parsed PFM")
	}
	if name, ok := metrics.GlyphNameByCode(65); !ok || name != "A" {
		t.Fatalf("GlyphNameByCode(65) = %q, %v; want A, true", name, ok)
	}
	if width, ok := metrics.WidthByCode(65); !ok || width != 722 {
		t.Fatalf("WidthByCode(65) = %v, %v; want AFM width 722, true", width, ok)
	}
	if width, ok := metrics.WidthByCode(67); !ok || width != 620 {
		t.Fatalf("WidthByCode(67) = %v, %v; want PFM width 620, true", width, ok)
	}
}

func TestReadCompanionMetricsOptionalReaders(t *testing.T) {
	pfmData, _, _ := testPFM()
	var encoding [256]string
	encoding[65] = "A"

	tests := []struct {
		name    string
		afm     io.Reader
		pfm     io.Reader
		wantAFM bool
		wantPFM bool
	}{
		{
			name: "no companions",
		},
		{
			name: "only AFM",
			afm: strings.NewReader(`
StartFontMetrics 4.1
FontName Reader-Regular
StartCharMetrics 1
C 65 ; WX 722 ; N A ; B 9 0 689 674 ;
EndCharMetrics
EndFontMetrics
`),
			wantAFM: true,
		},
		{
			name:    "only PFM",
			pfm:     bytes.NewReader(pfmData),
			wantPFM: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, err := ReadCompanionMetrics(tt.afm, tt.pfm, encoding)
			if err != nil {
				t.Fatalf("ReadCompanionMetrics failed: %v", err)
			}
			if (metrics.AFM != nil) != tt.wantAFM {
				t.Fatalf("AFM present = %v, want %v", metrics.AFM != nil, tt.wantAFM)
			}
			if (metrics.PFM != nil) != tt.wantPFM {
				t.Fatalf("PFM present = %v, want %v", metrics.PFM != nil, tt.wantPFM)
			}
			if name, ok := metrics.GlyphNameByCode(65); !ok || name != "A" {
				t.Fatalf("GlyphNameByCode(65) = %q, %v; want A, true", name, ok)
			}
		})
	}
}

func TestReadAFMReadPFMPropagateReaderErrors(t *testing.T) {
	readErr := errors.New("read failed")
	tests := []struct {
		name string
		read func() error
	}{
		{
			name: "afm",
			read: func() error {
				_, err := ReadAFM(companionErrorReader{err: readErr})
				return err
			},
		},
		{
			name: "pfm",
			read: func() error {
				_, err := ReadPFM(companionErrorReader{err: readErr})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.read(); !errors.Is(err, readErr) {
				t.Fatalf("error = %v, want %v", err, readErr)
			}
		})
	}
}

func TestReadCompanionMetricsPropagatesReaderErrors(t *testing.T) {
	readErr := errors.New("read failed")
	var encoding [256]string

	tests := []struct {
		name string
		afm  io.Reader
		pfm  io.Reader
	}{
		{
			name: "afm",
			afm:  companionErrorReader{err: readErr},
		},
		{
			name: "pfm",
			pfm:  companionErrorReader{err: readErr},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ReadCompanionMetrics(tt.afm, tt.pfm, encoding)
			if !errors.Is(err, readErr) {
				t.Fatalf("error = %v, want %v", err, readErr)
			}
		})
	}
}

type companionErrorReader struct {
	err error
}

func (r companionErrorReader) Read([]byte) (int, error) {
	return 0, r.err
}
