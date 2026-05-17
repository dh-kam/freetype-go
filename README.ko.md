# FreeType-Go (한글)

**FreeType 2.13.2** 폰트 엔진에서 영감을 받은 순수 Go 기반 포팅 작업입니다. 폰트 파싱, 글리프 로딩, 래스터라이징, 레이아웃 실험 및 관련 유틸리티가 하위 패키지로 나뉘어 있습니다.

[![Go Reference](https://pkg.go.dev/badge/github.com/dh-kam/freetype-go.svg)](https://pkg.go.dev/github.com/dh-kam/freetype-go)
[![License: FTL](https://img.shields.io/badge/License-FTL-blue.svg)](https://www.freetype.org/license.html)

## 상태

- **CGO 없는 구조**: C FreeType 라이브러리에 링크하지 않고 Go 코드로 빌드하는 것을 목표로 합니다.
- **실험적인 API**: 필요한 하위 패키지를 직접 import해서 사용하세요. 루트 패키지는 현재 `go list .` 및 pkg.go.dev 발견을 위한 문서만 제공합니다.
- **부분적인 FreeType 범위**: SFNT/OpenType 파싱, TrueType/CFF 관련 코드, 래스터라이징, 일부 비트맵/컬러/가변 폰트 테이블, 초기 레이아웃 지원이 포함되어 있습니다. 아직 FreeType의 완전한 대체재라고 보기는 어렵습니다.
- **검증 범위는 확장 중**: 여러 패키지에 테스트가 있지만, 현재 비트 단위 렌더링 동일성이나 전체 사양 지원을 보장하지는 않습니다.

## 빠른 시작

### 설치
```bash
go get github.com/dh-kam/freetype-go
```

### 기본 사용법
```go
package main

import (
	"github.com/dh-kam/freetype-go/core"
	"github.com/dh-kam/freetype-go/raster"
	"github.com/dh-kam/freetype-go/sfnt"
	"os"
)

func main() {
	f, _ := os.Open("font.ttf")
	defer f.Close()

	// 1. 폰트 로드
	stream, _ := core.NewFileStream(f)
	loader := sfnt.NewLoader(core.NewSystem())
	face, _ := loader.LoadFace(stream)

	// 2. 글자 찾기 및 로드
	face.SetPixelSizes(64, 64)
	glyphIndex, _ := face.GetGlyphIndex('A')
	slot, _ := face.LoadGlyph(glyphIndex, 0)

	// 3. 비트맵으로 렌더링
	outline := slot.GetOutline()
	bitmap := core.NewBitmap(64, 64)
	rasterizer := raster.NewSmoothRasterizer()
	rasterizer.Render(outline, bitmap)
}
```

## 문서
- [상세 기능 목록](docs/features.ko.md)
- [아키텍처 가이드](docs/architecture.md)
- [CLI ASCII 렌더러](cmd/ftgo)
- [웹 데모 (WASM)](https://dh-kam.github.com/freetype-go/)

## 라이선스
본 프로젝트는 오리지널 FreeType 프로젝트에 대한 존중을 담아 **FreeType License (FTL)** 하에 배포됩니다. 자세한 내용은 [LICENSE](LICENSE) 파일을 참조하세요.

---
*Maintained by dh-kam and the Gemini CLI Panel.*
