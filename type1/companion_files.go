package type1

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FontPathStream is an optional interface for streams that can report the
// filesystem path of the Type 1 font data they expose.
//
// LoadFace uses this path, when present and non-empty, to discover adjacent
// same-stem AFM and PFM companion metrics. Plain memory streams and streams
// that do not implement this interface keep the existing no-auto-attach
// behavior.
type FontPathStream interface {
	Type1FontPath() string
}

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
// metrics use encoding for glyph-name lookups and can be queried directly or
// attached to a Face with SetCompanionMetrics.
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

// ReadCompanionMetricsFiles reads optional AFM and PFM companion metric files
// from explicit filesystem paths.
//
// An empty path means the corresponding companion file is absent and is not an
// error. Non-empty paths are opened and parsed as supplied; this helper does
// not discover or infer companion files from a Type 1 font path. The returned
// metrics use encoding for glyph-name lookups and can be queried directly or
// attached to a Face with SetCompanionMetrics.
func ReadCompanionMetricsFiles(afmPath, pfmPath string, encoding [256]string) (*CompanionMetrics, error) {
	metrics := &CompanionMetrics{Encoding: encoding}

	if afmPath != "" {
		parsed, err := readAFMFile(afmPath)
		if err != nil {
			return nil, err
		}
		metrics.AFM = parsed
	}
	if pfmPath != "" {
		parsed, err := readPFMFile(pfmPath)
		if err != nil {
			return nil, err
		}
		metrics.PFM = parsed
	}

	return metrics, nil
}

// DiscoverCompanionMetricsFiles finds optional AFM and PFM companion metric
// files adjacent to a Type 1 font path.
//
// Discovery probes files with the same stem as fontPath and .afm/.AFM and
// .pfm/.PFM extensions. Missing companion files are not errors.
func DiscoverCompanionMetricsFiles(fontPath string) (afmPath, pfmPath string, err error) {
	if fontPath == "" {
		return "", "", errors.New("type1: empty Type 1 font path")
	}

	stem := strings.TrimSuffix(fontPath, filepath.Ext(fontPath))
	afmPath, err = discoverCompanionMetricFile(stem, ".afm")
	if err != nil {
		return "", "", err
	}
	pfmPath, err = discoverCompanionMetricFile(stem, ".pfm")
	if err != nil {
		return "", "", err
	}
	return afmPath, pfmPath, nil
}

// ReadCompanionMetricsForFont reads optional AFM and PFM companion metric files
// discovered next to fontPath.
//
// The returned metrics use encoding for glyph-name lookups and can be queried
// directly or attached to a Face with SetCompanionMetrics.
func ReadCompanionMetricsForFont(fontPath string, encoding [256]string) (*CompanionMetrics, error) {
	afmPath, pfmPath, err := DiscoverCompanionMetricsFiles(fontPath)
	if err != nil {
		return nil, err
	}
	return ReadCompanionMetricsFiles(afmPath, pfmPath, encoding)
}

func readCompanionMetricsForStream(stream interface{}, encoding [256]string) (*CompanionMetrics, bool, error) {
	pathStream, ok := stream.(FontPathStream)
	if !ok {
		return nil, false, nil
	}
	fontPath := pathStream.Type1FontPath()
	if fontPath == "" {
		return nil, false, nil
	}

	metrics, err := ReadCompanionMetricsForFont(fontPath, encoding)
	if err != nil {
		return nil, true, err
	}
	return metrics, true, nil
}

// SetCompanionMetricsFiles reads optional AFM and PFM companion metric files
// from explicit filesystem paths and attaches them to the face.
//
// Empty paths have the same meaning as ReadCompanionMetricsFiles: the
// corresponding companion file is absent and is not an error. Paths are opened
// exactly as supplied; this helper does not discover companion files from a
// Type 1 font path or probe alternate extensions.
func (f *Face) SetCompanionMetricsFiles(afmPath, pfmPath string, encoding [256]string) error {
	metrics, err := ReadCompanionMetricsFiles(afmPath, pfmPath, encoding)
	if err != nil {
		return err
	}
	f.SetCompanionMetrics(metrics)
	return nil
}

// SetCompanionMetricsForFont reads optional AFM and PFM companion metric files
// discovered next to fontPath and attaches them to the face.
func (f *Face) SetCompanionMetricsForFont(fontPath string) error {
	if f == nil || f.font == nil {
		return errors.New("type1: nil Type 1 face")
	}
	metrics, err := ReadCompanionMetricsForFont(fontPath, f.font.Encoding)
	if err != nil {
		return err
	}
	f.SetCompanionMetrics(metrics)
	return nil
}

func readAFMFile(path string) (*AFM, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("type1: open AFM companion metrics %q: %w", path, err)
	}
	defer f.Close()

	afm, err := ReadAFM(f)
	if err != nil {
		return nil, fmt.Errorf("type1: read AFM companion metrics %q: %w", path, err)
	}
	return afm, nil
}

func readPFMFile(path string) (*PFM, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("type1: open PFM companion metrics %q: %w", path, err)
	}
	defer f.Close()

	pfm, err := ReadPFM(f)
	if err != nil {
		return nil, fmt.Errorf("type1: read PFM companion metrics %q: %w", path, err)
	}
	return pfm, nil
}

func discoverCompanionMetricFile(stem, ext string) (string, error) {
	for _, path := range companionMetricFileCandidates(stem, ext) {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("type1: probe companion metrics %q: %w", path, err)
		}
	}
	return "", nil
}

func companionMetricFileCandidates(stem, ext string) []string {
	upperExt := strings.ToUpper(ext)
	if upperExt == ext {
		return []string{stem + ext}
	}
	return []string{stem + ext, stem + upperExt}
}
