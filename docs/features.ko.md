# FreeType-Go 기능 상세

이 문서는 현재 저장소에 구현되어 있는 영역을 요약합니다. FreeType 전체 호환성이나 안정적인 API 범위를 보장하는 문서는 아닙니다.

## 구현된 영역

### 1. 폰트 포맷 (Font Formats)
- **TrueType (.ttf)**: SFNT 디렉터리 파싱, 주요 테이블 파싱, 글리프 로딩, cmap/hmtx 처리, TrueType 명령어 실행 구조가 포함되어 있습니다.
- **OpenType/CFF (.otf)**: CFF 파싱 및 CharString 관련 지원이 있습니다. 지원 범위는 계속 변할 수 있습니다.
- **TrueType/OpenType 컬렉션(.ttc/.otc)**: SFNT 로더가 기본적으로 첫 face를 열거나 `sfnt.LoadFaceIndex`로 지정한 face를 열 수 있습니다.
- **WOFF/WOFF2 컨테이너**: WOFF 및 WOFF2 스트림을 SFNT 또는 TTC로 디코딩할 수 있으며, WOFF2는 변환된 `glyf`/`loca`, `hmtx`, collection directory 재구성을 포함합니다.
- **비트맵 폰트**: BDF, PCF, FNT 및 일부 내장 비트맵 테이블을 다룹니다.
- **컬러 폰트**: 일부 `COLR`/`CPAL` 및 `sbix` 데이터 처리를 위한 파서와 헬퍼가 있습니다.

### 2. 렌더링 엔진 (Rendering Engine)
- **고정소수점 수학 연산**: 26.6 및 16.16 형식의 주요 연산을 Go로 구현하고 패키지 테스트를 포함합니다.
- **Smooth 래스터라이저**: 외곽선을 `api.Bitmap` 대상으로 안티앨리어싱 렌더링합니다.
- **LCD 서브픽셀 렌더링**: 래스터라이저에 LCD 픽셀 모드와 필터링 지원이 있습니다.
- **Outline Stroker**: 벡터 경로 스트로킹 기능이 있습니다.

### 3. 타이포그래피 및 레이아웃 (Typography & Layout)
- **TrueType VM**: 힌팅 작업을 위한 바이트코드 인터프리터 코드가 있지만, 현재 완전한 VM 호환성을 주장하지는 않습니다.
- **가변 폰트(Variable Fonts)**: `fvar`, `gvar` 등 일부 OpenType 가변 폰트 테이블을 지원합니다.
- **OpenType 레이아웃**: GSUB/GPOS 파싱과 리가처, 문맥 치환, 커닝, 마크 배치 일부 처리를 포함합니다.

### 4. 시스템 유틸리티 (System Utilities)
- **LRU 캐시**: 뮤텍스로 보호되는 범용 LRU 캐시와 Face/Glyph 매니저 헬퍼.
- **Stream I/O**: 파일 및 메모리 읽기를 추상화한 인터페이스 제공.
- **에러 맵핑**: FreeType 스타일의 숫자 에러 값을 Go 코드로 표현합니다.
