//go:build !cgo || !freetype_conformance

package main

import "errors"

func buildFreeTypeDump(opts dumpOptions) (*Dump, error) {
	return nil, errors.New("FreeType reference dumper is not built; rerun with CGO_ENABLED=1, -tags freetype_conformance, and freetype2 pkg-config support")
}
