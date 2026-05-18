package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	defaultSmokeCorpus     = "system-smoke"
	defaultSmokeLimit      = 4
	defaultSmokePPEM       = "12,16,24"
	defaultSmokeGlyphs     = "0"
	defaultSmokeChars      = "U+0020,U+0030,U+0041,U+0061"
	defaultSmokeLoadFlags  = "no-hinting"
	defaultSmokeRenderMode = "none"
)

type smokeOptions struct {
	FontPaths   []string
	FontDirs    []string
	RequestPath string
	OutputDir   string
	Corpus      string
	Limit       int
	Strict      bool
}

func runSmokeCommand(args []string, stdout, stderr io.Writer) error {
	opts, err := parseSmokeOptions(args, stdout)
	if err != nil {
		if isHelp(err) {
			return nil
		}
		return err
	}

	fonts, err := discoverFontFiles(opts.FontPaths, opts.FontDirs, opts.Limit)
	if err != nil {
		return err
	}
	if len(fonts) == 0 {
		fmt.Fprintf(stdout, "conformance smoke: SKIP (no local system fonts found in %d directories)\n", len(opts.FontDirs))
		if opts.Strict {
			return errors.New("no local system fonts found")
		}
		return nil
	}
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return err
	}

	successes := 0
	failures := 0
	for i, fontPath := range fonts {
		outputPath := smokeOutputPath(opts.OutputDir, fontPath, i)
		fmt.Fprintf(stdout, "[%d/%d] %s -> %s\n", i+1, len(fonts), fontPath, outputPath)

		dumpArgs := smokeDumpArgs(opts, fontPath, outputPath)
		parsed, err := parseDumpOptions("smoke", dumpArgs, stderr)
		if err == nil {
			var dump *Dump
			dump, err = buildGoDump(parsed)
			if err == nil {
				err = writeDump(dump, outputPath, stdout)
			}
		}
		if err != nil {
			failures++
			fmt.Fprintf(stdout, "  SKIP: %v\n", err)
			continue
		}
		successes++
		fmt.Fprintln(stdout, "  PASS")
	}

	fmt.Fprintf(stdout, "conformance smoke: %d dumps written, %d skipped\n", successes, failures)
	if opts.Strict && (successes == 0 || failures > 0) {
		return fmt.Errorf("conformance smoke failed: %d dumps written, %d skipped", successes, failures)
	}
	return nil
}

func parseSmokeOptions(args []string, output io.Writer) (smokeOptions, error) {
	var fontPaths stringValues
	var fontDirs stringValues
	opts := smokeOptions{
		OutputDir: "conformance-smoke",
		Corpus:    defaultSmokeCorpus,
		Limit:     defaultSmokeLimit,
	}

	fs := flag.NewFlagSet("smoke", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.Var(&fontPaths, "font", "font file to smoke; repeatable; defaults also honor FTGO_CONFORMANCE_FONT or CONFORMANCE_FONT")
	fs.Var(&fontDirs, "font-dir", "font directory to scan; repeatable; defaults to common system font locations")
	fs.Var(&fontDirs, "fontdir", "alias for -font-dir")
	fs.StringVar(&opts.RequestPath, "request", "", "optional request JSON file; explicit discovered font/out/corpus override request fields")
	fs.StringVar(&opts.OutputDir, "out-dir", opts.OutputDir, "directory for discovered Go JSON dumps")
	fs.StringVar(&opts.Corpus, "corpus", opts.Corpus, "corpus label recorded in each dump")
	fs.IntVar(&opts.Limit, "limit", opts.Limit, "maximum discovered fonts to dump")
	fs.BoolVar(&opts.Strict, "strict", false, "fail when no dump is written or any discovered font is skipped")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.OutputDir == "" {
		return opts, errors.New("missing required -out-dir")
	}
	if opts.Limit <= 0 {
		return opts, errors.New("-limit must be positive")
	}
	opts.FontPaths = cleanPathList([]string(fontPaths))
	opts.FontDirs = []string(fontDirs)
	if len(opts.FontPaths) == 0 && len(opts.FontDirs) == 0 {
		opts.FontPaths = defaultSystemFontFiles()
		opts.FontDirs = defaultSystemFontDirs()
	}
	opts.FontPaths = cleanPathList(opts.FontPaths)
	opts.FontDirs = cleanPathList(opts.FontDirs)
	return opts, nil
}

func smokeDumpArgs(opts smokeOptions, fontPath, outputPath string) []string {
	args := []string{
		"-font", fontPath,
		"-out", outputPath,
		"-corpus", opts.Corpus,
	}
	if opts.RequestPath != "" {
		args = append(args, "-request", opts.RequestPath)
		return args
	}
	return append(args,
		"-ppem", defaultSmokePPEM,
		"-glyphs", defaultSmokeGlyphs,
		"-chars", defaultSmokeChars,
		"-load-flags", defaultSmokeLoadFlags,
		"-render-mode", defaultSmokeRenderMode,
	)
}

func discoverFontFiles(fontPaths, dirs []string, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, errors.New("limit must be positive")
	}
	seen := make(map[string]bool)
	var fonts []string
	addFont := func(path string) {
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		seen[clean] = true
		fonts = append(fonts, clean)
	}
	for _, path := range cleanPathList(fontPaths) {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || !isFontPath(path) {
			continue
		}
		addFont(path)
	}
	for _, dir := range cleanPathList(dirs) {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d == nil || d.IsDir() || !isFontPath(path) {
				return nil
			}
			addFont(path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(fonts)
	if len(fonts) > limit {
		fonts = fonts[:limit]
	}
	return fonts, nil
}

func defaultSystemFontDirs() []string {
	var dirs []string
	dirs = append(dirs, splitPathEnv("FTGO_CONFORMANCE_FONTDIR")...)
	dirs = append(dirs, splitPathEnv("CONFORMANCE_FONTDIR")...)
	switch runtime.GOOS {
	case "darwin":
		dirs = append(dirs, "/System/Library/Fonts", "/Library/Fonts")
		if home, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs, filepath.Join(home, "Library", "Fonts"))
		}
	case "windows":
		if windir := os.Getenv("WINDIR"); windir != "" {
			dirs = append(dirs, filepath.Join(windir, "Fonts"))
		}
	default:
		dirs = append(dirs, "/usr/share/fonts", "/usr/local/share/fonts")
		if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
			dirs = append(dirs, filepath.Join(dataHome, "fonts"))
		}
		for _, dataDir := range splitPathEnvDefault("XDG_DATA_DIRS", "/usr/local/share:/usr/share") {
			dirs = append(dirs, filepath.Join(dataDir, "fonts"))
		}
		if home, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs,
				filepath.Join(home, ".local", "share", "fonts"),
				filepath.Join(home, ".fonts"),
			)
		}
	}
	return cleanPathList(dirs)
}

func defaultSystemFontFiles() []string {
	var files []string
	files = append(files, splitPathEnv("FTGO_CONFORMANCE_FONT")...)
	files = append(files, splitPathEnv("CONFORMANCE_FONT")...)
	files = append(files, fontconfigFontPaths()...)
	return cleanPathList(files)
}

func splitPathEnvDefault(name, fallback string) []string {
	value := os.Getenv(name)
	if value == "" {
		value = fallback
	}
	if value == "" {
		return nil
	}
	return filepath.SplitList(value)
}

func splitPathEnv(name string) []string {
	value := os.Getenv(name)
	if value == "" {
		return nil
	}
	return filepath.SplitList(value)
}

func fontconfigFontPaths() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	out, err := exec.Command("fc-list", "-f", "%{file}\n").Output()
	if err == nil {
		return splitLines(string(out))
	}
	out, err = exec.Command("fc-match", "-f", "%{file}\n").Output()
	if err != nil {
		return nil
	}
	return splitLines(string(out))
}

func splitLines(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func cleanPathList(paths []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

func isFontPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".otf", ".ttc", ".otc":
		return true
	default:
		return false
	}
}

func smokeOutputPath(outputDir, fontPath string, index int) string {
	base := filepath.Base(fontPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(outputDir, fmt.Sprintf("%02d-%s.go.json", index+1, sanitizeFileStem(name)))
}

func sanitizeFileStem(name string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "font"
	}
	return out
}
