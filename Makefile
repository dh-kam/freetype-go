GO ?= go
GOFMT ?= gofmt
GO_PKG ?= ./cmd/ftgo
OS_LIST ?= linux darwin windows
ARCH_LIST ?= amd64 arm64
BUILD_VARIANTS ?= debug release

OUTPUT_DIR ?= dist
APP_NAME ?= ftgo

GO_DEBUG_FLAGS ?= -trimpath -gcflags="all=-N -l"
GO_RELEASE_FLAGS ?= -trimpath -ldflags="-s -w"
CGO_DEBUG_ENABLED ?= 0
CGO_RELEASE_ENABLED ?= 0
FUZZTIME ?= 5s
FUZZ_PKGS ?= ./fuzz
COVERAGE_OUT ?= coverage.out
CONFORMANCE_TOOL ?= ./tools/conformance
CONFORMANCE_FONT ?=
CONFORMANCE_FONTDIR ?=
CONFORMANCE_REQUEST ?=
CONFORMANCE_REQUESTS ?= $(CONFORMANCE_REQUEST)
CONFORMANCE_OUT ?= conformance-go.json
CONFORMANCE_REF_OUT ?= conformance-freetype.json
CONFORMANCE_OUT_DIR ?= conformance-out
CONFORMANCE_REF_DIR ?= $(CONFORMANCE_OUT_DIR)
CONFORMANCE_CANDIDATE_DIR ?= $(CONFORMANCE_OUT_DIR)
CONFORMANCE_REF_SUFFIX ?= .freetype.json
CONFORMANCE_CANDIDATE_SUFFIX ?= .go.json
CONFORMANCE_BATCH_FLAGS ?=
CONFORMANCE_COMPARE_FLAGS ?=
CONFORMANCE_REF ?=
CONFORMANCE_CANDIDATE ?= $(CONFORMANCE_OUT)
CONFORMANCE_PPEM ?= 12,16,24
CONFORMANCE_GLYPHS ?= 0
CONFORMANCE_CHARS ?= U+0020,U+0030,U+0041,U+0061
CONFORMANCE_LOAD_FLAGS ?= no-hinting
CONFORMANCE_RENDER_MODE ?= none
CONFORMANCE_METRIC_TOLERANCE ?= 0
CONFORMANCE_POINT_TOLERANCE ?= 0
CONFORMANCE_ALLOW_MISSING_BITMAP ?= 0
CONFORMANCE_ALLOW_MISSING_SLOT_METRICS ?= 0

# collect all go files recursively for dependency tracking
rwildcard = $(wildcard $(1)/$(2)) $(foreach d,$(wildcard $(1)/*),$(call rwildcard,$d,$(2)))
GO_FILES := $(call rwildcard,.,*.go)

artifact = $(OUTPUT_DIR)/$(APP_NAME)-$(1)-$(2)-$(3)$(if $(filter windows,$(1)),.exe,)

define build_target
$(call artifact,$(1),$(2),$(3)): $(GO_FILES) | $(OUTPUT_DIR)
	@mkdir -p $(OUTPUT_DIR)
	GOOS=$(1) GOARCH=$(2) CGO_ENABLED=$(if $(filter release,$(3)),$(CGO_RELEASE_ENABLED),$(CGO_DEBUG_ENABLED)) \
	$(GO) build \
	$(if $(filter release,$(3)),$(GO_RELEASE_FLAGS),$(GO_DEBUG_FLAGS)) \
	-o $$@ $(GO_PKG)
endef

token1 = $(word 1,$(subst -, ,$(1)))
token2 = $(word 2,$(subst -, ,$(1)))
token3 = $(word 3,$(subst -, ,$(1)))

OS_ARCH_PAIRS := $(foreach os,$(OS_LIST),$(foreach arch,$(ARCH_LIST),$(os)-$(arch)))
OS_VARIANT_PAIRS := $(foreach os,$(OS_LIST),$(foreach var,$(BUILD_VARIANTS),$(os)-$(var)))
ARCH_VARIANT_PAIRS := $(foreach arch,$(ARCH_LIST),$(foreach var,$(BUILD_VARIANTS),$(arch)-$(var)))
FULL_SELECTORS := $(foreach os,$(OS_LIST),$(foreach arch,$(ARCH_LIST),$(foreach var,$(BUILD_VARIANTS),$(os)-$(arch)-$(var))))
FULL_TARGETS := $(foreach os,$(OS_LIST),$(foreach arch,$(ARCH_LIST),$(foreach var,$(BUILD_VARIANTS),$(call artifact,$(os),$(arch),$(var)))))

define all_for_os
$(foreach arch,$(ARCH_LIST),$(foreach var,$(BUILD_VARIANTS),$(call artifact,$(1),$(arch),$(var))))
endef

define all_for_arch
$(foreach os,$(OS_LIST),$(foreach var,$(BUILD_VARIANTS),$(call artifact,$(os),$(1),$(var))))
endef

define all_for_variant
$(foreach os,$(OS_LIST),$(foreach arch,$(ARCH_LIST),$(call artifact,$(os),$(arch),$(1))))
endef

define all_for_os_arch
$(foreach var,$(BUILD_VARIANTS),$(call artifact,$(1),$(2),$(var)))
endef

define all_for_os_variant
$(foreach arch,$(ARCH_LIST),$(call artifact,$(1),$(arch),$(2)))
endef

define all_for_arch_variant
$(foreach os,$(OS_LIST),$(call artifact,$(os),$(1),$(2)))
endef

.PHONY: all clean help fmt vet bench benchmem test test-race fuzz-smoke coverage test-harness conformance-help conformance-dump conformance-ftdump conformance-corpus conformance-batch conformance-ftbatch conformance-compare conformance-batch-compare run $(OS_LIST) $(ARCH_LIST) $(BUILD_VARIANTS) $(OS_ARCH_PAIRS) $(OS_VARIANT_PAIRS) $(ARCH_VARIANT_PAIRS) $(FULL_SELECTORS)

all: $(FULL_TARGETS)

$(OUTPUT_DIR):
	@mkdir -p $(OUTPUT_DIR)

clean:
	@rm -rf $(OUTPUT_DIR)

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

fmt:
	$(GOFMT) -w $(GO_FILES)

vet:
	$(GO) vet ./...

fuzz-smoke:
	@set -e; \
	for pkg in $(FUZZ_PKGS); do \
		echo "==> fuzz $$pkg"; \
		$(GO) test $$pkg -run=^$$ -fuzz=Fuzz -fuzztime=$(FUZZTIME); \
	done

coverage:
	$(GO) test -coverprofile=$(COVERAGE_OUT) ./...
	$(GO) tool cover -func=$(COVERAGE_OUT)

bench:
	$(GO) test -bench=. ./...

benchmem:
	$(GO) test -bench=. -benchmem ./...

test-harness:
	$(GO) test -tags freetype_harness ./harness

conformance-help:
	$(GO) run $(CONFORMANCE_TOOL)

conformance-dump:
	@test -n "$(CONFORMANCE_FONT)$(CONFORMANCE_REQUEST)" || { echo "CONFORMANCE_FONT or CONFORMANCE_REQUEST is required"; exit 2; }
	$(GO) run $(CONFORMANCE_TOOL) dump \
		$(if $(CONFORMANCE_REQUEST),-request "$(CONFORMANCE_REQUEST)",) \
		$(if $(CONFORMANCE_FONT),-font "$(CONFORMANCE_FONT)",) \
		-out "$(CONFORMANCE_OUT)" \
		$(if $(CONFORMANCE_REQUEST),,-ppem "$(CONFORMANCE_PPEM)") \
		$(if $(CONFORMANCE_REQUEST),,-glyphs "$(CONFORMANCE_GLYPHS)") \
		$(if $(CONFORMANCE_REQUEST),,-chars "$(CONFORMANCE_CHARS)") \
		$(if $(CONFORMANCE_REQUEST),,-load-flags "$(CONFORMANCE_LOAD_FLAGS)") \
		$(if $(CONFORMANCE_REQUEST),,-render-mode "$(CONFORMANCE_RENDER_MODE)")

conformance-ftdump:
	@test -n "$(CONFORMANCE_FONT)$(CONFORMANCE_REQUEST)" || { echo "CONFORMANCE_FONT or CONFORMANCE_REQUEST is required"; exit 2; }
	CGO_ENABLED=1 $(GO) run -tags freetype_conformance $(CONFORMANCE_TOOL) ftdump \
		$(if $(CONFORMANCE_REQUEST),-request "$(CONFORMANCE_REQUEST)",) \
		$(if $(CONFORMANCE_FONT),-font "$(CONFORMANCE_FONT)",) \
		-out "$(CONFORMANCE_REF_OUT)" \
		$(if $(CONFORMANCE_REQUEST),,-ppem "$(CONFORMANCE_PPEM)") \
		$(if $(CONFORMANCE_REQUEST),,-glyphs "$(CONFORMANCE_GLYPHS)") \
		$(if $(CONFORMANCE_REQUEST),,-chars "$(CONFORMANCE_CHARS)") \
		$(if $(CONFORMANCE_REQUEST),,-load-flags "$(CONFORMANCE_LOAD_FLAGS)") \
		$(if $(CONFORMANCE_REQUEST),,-render-mode "$(CONFORMANCE_RENDER_MODE)")

conformance-corpus:
	@test -n "$(CONFORMANCE_FONTDIR)" || { echo "CONFORMANCE_FONTDIR is required"; exit 2; }
	@mkdir -p "$(CONFORMANCE_OUT_DIR)"
	@set -e; \
	find "$(CONFORMANCE_FONTDIR)" -type f \( -name '*.ttf' -o -name '*.otf' -o -name '*.ttc' -o -name '*.otc' \) | sort | while read -r font; do \
		name=$$(basename "$$font"); \
		echo "==> $$font"; \
		$(GO) run $(CONFORMANCE_TOOL) dump \
			$(if $(CONFORMANCE_REQUEST),-request "$(CONFORMANCE_REQUEST)",) \
			-font "$$font" \
			-out "$(CONFORMANCE_OUT_DIR)/$$name.go.json" \
			$(if $(CONFORMANCE_REQUEST),,-ppem "$(CONFORMANCE_PPEM)") \
			$(if $(CONFORMANCE_REQUEST),,-glyphs "$(CONFORMANCE_GLYPHS)") \
			$(if $(CONFORMANCE_REQUEST),,-chars "$(CONFORMANCE_CHARS)") \
			$(if $(CONFORMANCE_REQUEST),,-load-flags "$(CONFORMANCE_LOAD_FLAGS)") \
			$(if $(CONFORMANCE_REQUEST),,-render-mode "$(CONFORMANCE_RENDER_MODE)"); \
	done

conformance-batch:
	@test -n "$(CONFORMANCE_REQUESTS)" || { echo "CONFORMANCE_REQUESTS is required"; exit 2; }
	$(GO) run $(CONFORMANCE_TOOL) batch \
		-requests "$(CONFORMANCE_REQUESTS)" \
		-out-dir "$(CONFORMANCE_OUT_DIR)" \
		-engine go \
		$(if $(CONFORMANCE_FONT),-font "$(CONFORMANCE_FONT)",) \
		$(CONFORMANCE_BATCH_FLAGS)

conformance-ftbatch:
	@test -n "$(CONFORMANCE_REQUESTS)" || { echo "CONFORMANCE_REQUESTS is required"; exit 2; }
	CGO_ENABLED=1 $(GO) run -tags freetype_conformance $(CONFORMANCE_TOOL) batch \
		-requests "$(CONFORMANCE_REQUESTS)" \
		-out-dir "$(CONFORMANCE_OUT_DIR)" \
		-engine freetype \
		$(if $(CONFORMANCE_FONT),-font "$(CONFORMANCE_FONT)",) \
		$(CONFORMANCE_BATCH_FLAGS)

conformance-compare:
	@test -n "$(CONFORMANCE_REF)" || { echo "CONFORMANCE_REF is required"; exit 2; }
	@test -n "$(CONFORMANCE_CANDIDATE)" || { echo "CONFORMANCE_CANDIDATE is required"; exit 2; }
	$(GO) run $(CONFORMANCE_TOOL) compare \
		-reference "$(CONFORMANCE_REF)" \
		-candidate "$(CONFORMANCE_CANDIDATE)" \
		-metric-tolerance "$(CONFORMANCE_METRIC_TOLERANCE)" \
		-point-tolerance "$(CONFORMANCE_POINT_TOLERANCE)" \
		$(if $(filter 1 true yes,$(CONFORMANCE_ALLOW_MISSING_BITMAP)),-allow-missing-bitmap,) \
		$(if $(filter 1 true yes,$(CONFORMANCE_ALLOW_MISSING_SLOT_METRICS)),-allow-missing-slot-metrics,)

conformance-batch-compare:
	@test -n "$(CONFORMANCE_REQUESTS)" || { echo "CONFORMANCE_REQUESTS is required"; exit 2; }
	$(GO) run $(CONFORMANCE_TOOL) batch-compare \
		-requests "$(CONFORMANCE_REQUESTS)" \
		-reference-dir "$(CONFORMANCE_REF_DIR)" \
		-candidate-dir "$(CONFORMANCE_CANDIDATE_DIR)" \
		-reference-suffix "$(CONFORMANCE_REF_SUFFIX)" \
		-candidate-suffix "$(CONFORMANCE_CANDIDATE_SUFFIX)" \
		-metric-tolerance "$(CONFORMANCE_METRIC_TOLERANCE)" \
		-point-tolerance "$(CONFORMANCE_POINT_TOLERANCE)" \
		$(if $(filter 1 true yes,$(CONFORMANCE_ALLOW_MISSING_BITMAP)),-allow-missing-bitmap,) \
		$(if $(filter 1 true yes,$(CONFORMANCE_ALLOW_MISSING_SLOT_METRICS)),-allow-missing-slot-metrics,) \
		$(CONFORMANCE_COMPARE_FLAGS)

run:
	$(GO) run $(GO_PKG)

$(OS_LIST):
	@$(MAKE) $(call all_for_os,$@)

$(ARCH_LIST):
	@$(MAKE) $(call all_for_arch,$@)

$(BUILD_VARIANTS):
	@$(MAKE) $(call all_for_variant,$@)

$(OS_ARCH_PAIRS):
	@$(MAKE) $(call all_for_os_arch,$(call token1,$@),$(call token2,$@))

$(OS_VARIANT_PAIRS):
	@$(MAKE) $(call all_for_os_variant,$(call token1,$@),$(call token2,$@))

$(ARCH_VARIANT_PAIRS):
	@$(MAKE) $(call all_for_arch_variant,$(call token1,$@),$(call token2,$@))

$(FULL_SELECTORS):
	@$(MAKE) $(call artifact,$(call token1,$@),$(call token2,$@),$(call token3,$@))

$(foreach os,$(OS_LIST),$(foreach arch,$(ARCH_LIST),$(foreach var,$(BUILD_VARIANTS),$(eval $(call build_target,$(os),$(arch),$(var))))))

help:
	@echo "make all: build all targets"
	@echo "make clean: remove $(OUTPUT_DIR)"
	@echo "make fmt: format Go sources"
	@echo "make vet: run go vet"
	@echo "make bench: run benchmarks"
	@echo "make benchmem: run benchmarks with memory stats"
	@echo "make test: run default Go tests"
	@echo "make test-race: run Go tests with the race detector"
	@echo "make fuzz-smoke: run short Go fuzzing smoke tests"
	@echo "make coverage: write coverage profile to $(COVERAGE_OUT)"
	@echo "make test-harness: run cgo FreeType parity tests"
	@echo "make conformance-help: show conformance harness usage"
	@echo "make conformance-dump CONFORMANCE_FONT=font.ttf: write Go JSON dump to $(CONFORMANCE_OUT)"
	@echo "make conformance-ftdump CONFORMANCE_FONT=font.ttf: write FreeType JSON dump to $(CONFORMANCE_REF_OUT)"
	@echo "make conformance-corpus CONFORMANCE_FONTDIR=fonts/: dump every TTF/OTF/TTC/OTC in a directory"
	@echo "make conformance-batch CONFORMANCE_REQUESTS='testdata/conformance/*.json' CONFORMANCE_FONT=font.ttf: dump request corpus with Go"
	@echo "make conformance-ftbatch CONFORMANCE_REQUESTS='testdata/conformance/*.json' CONFORMANCE_FONT=font.ttf: dump request corpus with FreeType"
	@echo "make conformance-compare CONFORMANCE_REF=ref.json CONFORMANCE_CANDIDATE=go.json: compare dumps"
	@echo "make conformance-batch-compare CONFORMANCE_REQUESTS='testdata/conformance/*.json': compare per-request dumps"
	@echo "make run: run $(GO_PKG)"
	@echo "make <os>|<arch>|<variant>|<os>-<arch>|<os>-<variant>|<arch>-<variant>|<os>-<arch>-<variant>"

%:
	@echo "Unknown target '$@'"
	@echo "Run 'make help' for valid patterns"
	@exit 1
