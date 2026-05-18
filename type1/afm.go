package type1

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// AFM contains the companion metrics parsed from an Adobe Font Metrics file.
type AFM struct {
	FontName     string
	FullName     string
	FamilyName   string
	Weight       string
	ItalicAngle  float64
	IsFixedPitch bool
	FontBBox     [4]float64
	CharMetrics  []AFMCharMetric
	KernPairs    []AFMKernPair

	glyphMetricByName map[string]int
	glyphMetricByCode map[int]int
	kernPairIndex     map[afmKernKey]int
}

// AFMCharMetric contains one StartCharMetrics character metric entry.
type AFMCharMetric struct {
	Code   int
	Name   string
	WidthX float64
	BBox   [4]float64
}

// AFMKernPair contains one StartKernPairs KPX kerning entry.
type AFMKernPair struct {
	Left  string
	Right string
	X     float64
}

type afmKernKey struct {
	left  string
	right string
}

// ParseAFM parses the common AFM records needed for Type 1 companion metrics.
func ParseAFM(data []byte) (*AFM, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)

	afm := &AFM{}
	lineNo := 0
	inCharMetrics := false
	inKernPairs := false

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "FontName":
			afm.FontName = afmStringValue(line, "FontName")
		case "FullName":
			afm.FullName = afmStringValue(line, "FullName")
		case "FamilyName":
			afm.FamilyName = afmStringValue(line, "FamilyName")
		case "Weight":
			afm.Weight = afmStringValue(line, "Weight")
		case "ItalicAngle":
			value, err := afmRequiredFloat(fields, 1, lineNo, "ItalicAngle")
			if err != nil {
				return nil, err
			}
			afm.ItalicAngle = value
		case "IsFixedPitch":
			value, err := afmRequiredBool(fields, 1, lineNo, "IsFixedPitch")
			if err != nil {
				return nil, err
			}
			afm.IsFixedPitch = value
		case "FontBBox":
			bbox, err := afmRequiredBBox(fields, 1, lineNo, "FontBBox")
			if err != nil {
				return nil, err
			}
			afm.FontBBox = bbox
		case "StartCharMetrics":
			if _, err := afmRequiredInt(fields, 1, lineNo, "StartCharMetrics"); err != nil {
				return nil, err
			}
			inCharMetrics = true
		case "EndCharMetrics":
			inCharMetrics = false
		case "StartKernPairs":
			if _, err := afmRequiredInt(fields, 1, lineNo, "StartKernPairs"); err != nil {
				return nil, err
			}
			inKernPairs = true
		case "EndKernPairs":
			inKernPairs = false
		case "C":
			if !inCharMetrics {
				continue
			}
			metric, err := parseAFMCharMetric(line, lineNo)
			if err != nil {
				return nil, err
			}
			afm.CharMetrics = append(afm.CharMetrics, metric)
		case "KPX":
			if !inKernPairs {
				continue
			}
			pair, err := parseAFMKernPair(fields, lineNo)
			if err != nil {
				return nil, err
			}
			afm.KernPairs = append(afm.KernPairs, pair)
		default:
			// AFM files contain many optional records. Unknown records are ignored.
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	afm.buildIndexes()
	return afm, nil
}

// GlyphMetricByName returns the first character metric declared for name.
func (afm *AFM) GlyphMetricByName(name string) (AFMCharMetric, bool) {
	if afm == nil || name == "" {
		return AFMCharMetric{}, false
	}
	if afm.glyphMetricByName != nil {
		if index, ok := afm.glyphMetricByName[name]; ok && index >= 0 && index < len(afm.CharMetrics) {
			metric := afm.CharMetrics[index]
			if metric.Name == name {
				return metric, true
			}
		}
	}
	for _, metric := range afm.CharMetrics {
		if metric.Name == name {
			return metric, true
		}
	}
	return AFMCharMetric{}, false
}

// GlyphMetricByCode returns the first character metric declared for code.
func (afm *AFM) GlyphMetricByCode(code int) (AFMCharMetric, bool) {
	if afm == nil {
		return AFMCharMetric{}, false
	}
	if afm.glyphMetricByCode != nil {
		if index, ok := afm.glyphMetricByCode[code]; ok && index >= 0 && index < len(afm.CharMetrics) {
			metric := afm.CharMetrics[index]
			if metric.Code == code {
				return metric, true
			}
		}
	}
	for _, metric := range afm.CharMetrics {
		if metric.Code == code {
			return metric, true
		}
	}
	return AFMCharMetric{}, false
}

// KernX returns the first KPX adjustment declared for the glyph pair.
func (afm *AFM) KernX(left, right string) (float64, bool) {
	if afm == nil {
		return 0, false
	}
	key := afmKernKey{left: left, right: right}
	if afm.kernPairIndex != nil {
		if index, ok := afm.kernPairIndex[key]; ok && index >= 0 && index < len(afm.KernPairs) {
			pair := afm.KernPairs[index]
			if pair.Left == left && pair.Right == right {
				return pair.X, true
			}
		}
	}
	for _, pair := range afm.KernPairs {
		if pair.Left == left && pair.Right == right {
			return pair.X, true
		}
	}
	return 0, false
}

// WidthXByName returns the horizontal width for the first metric declared for name.
func (afm *AFM) WidthXByName(name string) (float64, bool) {
	metric, ok := afm.GlyphMetricByName(name)
	if !ok {
		return 0, false
	}
	return metric.WidthX, true
}

// WidthXByCode returns the horizontal width for the first metric declared for code.
func (afm *AFM) WidthXByCode(code int) (float64, bool) {
	metric, ok := afm.GlyphMetricByCode(code)
	if !ok {
		return 0, false
	}
	return metric.WidthX, true
}

func (afm *AFM) buildIndexes() {
	afm.glyphMetricByName = make(map[string]int, len(afm.CharMetrics))
	afm.glyphMetricByCode = make(map[int]int, len(afm.CharMetrics))
	afm.kernPairIndex = make(map[afmKernKey]int, len(afm.KernPairs))

	for i, metric := range afm.CharMetrics {
		if metric.Name != "" {
			if _, ok := afm.glyphMetricByName[metric.Name]; !ok {
				afm.glyphMetricByName[metric.Name] = i
			}
		}
		if _, ok := afm.glyphMetricByCode[metric.Code]; !ok {
			afm.glyphMetricByCode[metric.Code] = i
		}
	}
	for i, pair := range afm.KernPairs {
		key := afmKernKey{left: pair.Left, right: pair.Right}
		if _, ok := afm.kernPairIndex[key]; !ok {
			afm.kernPairIndex[key] = i
		}
	}
}

func afmStringValue(line, key string) string {
	if len(line) <= len(key) {
		return ""
	}
	return strings.TrimSpace(line[len(key):])
}

func parseAFMCharMetric(line string, lineNo int) (AFMCharMetric, error) {
	metric := AFMCharMetric{Code: -1}
	for _, segment := range strings.Split(line, ";") {
		fields := strings.Fields(strings.TrimSpace(segment))
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "C":
			code, err := afmRequiredInt(fields, 1, lineNo, "C")
			if err != nil {
				return AFMCharMetric{}, err
			}
			metric.Code = code
		case "WX", "W0X":
			width, err := afmRequiredFloat(fields, 1, lineNo, fields[0])
			if err != nil {
				return AFMCharMetric{}, err
			}
			metric.WidthX = width
		case "N":
			if len(fields) > 1 {
				metric.Name = fields[1]
			}
		case "B":
			bbox, err := afmRequiredBBox(fields, 1, lineNo, "B")
			if err != nil {
				return AFMCharMetric{}, err
			}
			metric.BBox = bbox
		default:
			// Ligature and direction-specific fields are intentionally skipped.
		}
	}
	return metric, nil
}

func parseAFMKernPair(fields []string, lineNo int) (AFMKernPair, error) {
	if len(fields) < 4 {
		return AFMKernPair{}, afmLineError(lineNo, "KPX requires left glyph, right glyph, and x adjustment")
	}
	x, err := afmRequiredFloat(fields, 3, lineNo, "KPX")
	if err != nil {
		return AFMKernPair{}, err
	}
	return AFMKernPair{
		Left:  fields[1],
		Right: fields[2],
		X:     x,
	}, nil
}

func afmRequiredBBox(fields []string, start int, lineNo int, record string) ([4]float64, error) {
	var bbox [4]float64
	if len(fields) < start+4 {
		return bbox, afmLineError(lineNo, "%s requires 4 numeric values", record)
	}
	for i := 0; i < 4; i++ {
		value, err := afmParseFloat(fields[start+i], lineNo, record)
		if err != nil {
			return bbox, err
		}
		bbox[i] = value
	}
	return bbox, nil
}

func afmRequiredBool(fields []string, index int, lineNo int, record string) (bool, error) {
	if len(fields) <= index {
		return false, afmLineError(lineNo, "%s requires a boolean value", record)
	}
	value, err := strconv.ParseBool(fields[index])
	if err != nil {
		return false, afmLineError(lineNo, "invalid %s value %q", record, fields[index])
	}
	return value, nil
}

func afmRequiredFloat(fields []string, index int, lineNo int, record string) (float64, error) {
	if len(fields) <= index {
		return 0, afmLineError(lineNo, "%s requires a numeric value", record)
	}
	return afmParseFloat(fields[index], lineNo, record)
}

func afmRequiredInt(fields []string, index int, lineNo int, record string) (int, error) {
	if len(fields) <= index {
		return 0, afmLineError(lineNo, "%s requires an integer value", record)
	}
	value, err := strconv.Atoi(fields[index])
	if err != nil {
		return 0, afmLineError(lineNo, "invalid %s integer %q", record, fields[index])
	}
	return value, nil
}

func afmParseFloat(raw string, lineNo int, record string) (float64, error) {
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, afmLineError(lineNo, "invalid %s number %q", record, raw)
	}
	return value, nil
}

func afmLineError(lineNo int, format string, args ...any) error {
	return fmt.Errorf("AFM line %d: "+format, append([]any{lineNo}, args...)...)
}
