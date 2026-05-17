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

.PHONY: all clean help fmt vet bench benchmem test test-harness run $(OS_LIST) $(ARCH_LIST) $(BUILD_VARIANTS) $(OS_ARCH_PAIRS) $(OS_VARIANT_PAIRS) $(ARCH_VARIANT_PAIRS) $(FULL_SELECTORS)

all: $(FULL_TARGETS)

$(OUTPUT_DIR):
	@mkdir -p $(OUTPUT_DIR)

clean:
	@rm -rf $(OUTPUT_DIR)

test:
	$(GO) test ./...

fmt:
	$(GOFMT) -w $(GO_FILES)

vet:
	$(GO) vet ./...

bench:
	$(GO) test -bench=. ./...

benchmem:
	$(GO) test -bench=. -benchmem ./...

test-harness:
	$(GO) test -tags freetype_harness ./harness

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
	@echo "make test-harness: run cgo FreeType parity tests"
	@echo "make run: run $(GO_PKG)"
	@echo "make <os>|<arch>|<variant>|<os>-<arch>|<os>-<variant>|<arch>-<variant>|<os>-<arch>-<variant>"

%:
	@echo "Unknown target '$@'"
	@echo "Run 'make help' for valid patterns"
	@exit 1
