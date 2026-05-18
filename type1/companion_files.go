package type1

import (
	"errors"
	"fmt"
	"io"
)

// ReadAFM reads and parses an Adobe Font Metrics companion file.
//
// The returned AFM can be attached to a Face with SetAFM.
func ReadAFM(r io.Reader) (*AFM, error) {
	if r == nil {
		return nil, errors.New("type1: nil AFM reader")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseAFM(data)
}

// ReadPFM reads and parses a Windows Printer Font Metrics companion file.
//
// The returned PFM can be attached to a Face with SetPFM.
func ReadPFM(r io.Reader) (*PFM, error) {
	if r == nil {
		return nil, errors.New("type1: nil PFM reader")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParsePFM(data)
}

// ReadCompanionMetrics reads optional AFM and PFM companion metric files.
//
// A nil AFM or PFM reader means that companion file is absent and is not an
// error. Non-nil readers are parsed with ReadAFM and ReadPFM. The returned
// metrics use encoding for glyph-name lookups and can be queried directly.
func ReadCompanionMetrics(afm io.Reader, pfm io.Reader, encoding [256]string) (*CompanionMetrics, error) {
	metrics := &CompanionMetrics{Encoding: encoding}

	if afm != nil {
		parsed, err := ReadAFM(afm)
		if err != nil {
			return nil, fmt.Errorf("type1: read AFM companion metrics: %w", err)
		}
		metrics.AFM = parsed
	}
	if pfm != nil {
		parsed, err := ReadPFM(pfm)
		if err != nil {
			return nil, fmt.Errorf("type1: read PFM companion metrics: %w", err)
		}
		metrics.PFM = parsed
	}

	return metrics, nil
}
