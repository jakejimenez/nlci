BINARY     := nlci
APPLE_BIN  := nlci-apple
BUILD_DIR  := bin
GO_CMD     := cmd/nlci
APPLE_DIR  := apple
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

APPLE_INSTALL_DIR := $(HOME)/.config/nlci/bin

.PHONY: all build build-apple install install-apple release clean test lint help

all: build

## build: Compile the Go nlci binary
build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) ./$(GO_CMD)
	@echo "Built: $(BUILD_DIR)/$(BINARY)"

## build-apple: Compile the Swift nlci-apple binary (requires Xcode + macOS 26)
build-apple:
	@mkdir -p $(BUILD_DIR)
	cd $(APPLE_DIR) && swift build -c release
	cp $(APPLE_DIR)/.build/release/$(APPLE_BIN) $(BUILD_DIR)/$(APPLE_BIN)
	@echo "Built: $(BUILD_DIR)/$(APPLE_BIN)"

## install: Install nlci to ~/.local/bin (no sudo required)
install: build
	@mkdir -p $(HOME)/.local/bin
	install -m 755 $(BUILD_DIR)/$(BINARY) $(HOME)/.local/bin/$(BINARY)
	@echo "Installed: $(HOME)/.local/bin/$(BINARY)"

## install-apple: Install nlci-apple to ~/.config/nlci/bin/
install-apple: build-apple
	@mkdir -p $(APPLE_INSTALL_DIR)
	install -m 755 $(BUILD_DIR)/$(APPLE_BIN) $(APPLE_INSTALL_DIR)/$(APPLE_BIN)
	@echo "Installed: $(APPLE_INSTALL_DIR)/$(APPLE_BIN)"

## install-all: Build and install both binaries
install-all: install install-apple

## test: Run Go tests
test:
	go test ./...

## lint: Run go vet
lint:
	go vet ./...

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)
	cd $(APPLE_DIR) && swift package clean

## release: Build a release tarball (Go binary + Swift binary if present) for distribution
## Usage: VERSION=v0.1.0 make release
release: build
	@echo "Building release tarball for $(VERSION)..."
	@mkdir -p dist
	@cp $(BUILD_DIR)/$(BINARY) dist/
	@[ -f $(BUILD_DIR)/$(APPLE_BIN) ] && cp $(BUILD_DIR)/$(APPLE_BIN) dist/ || true
	@cp README.md dist/
	@if [ -f dist/$(APPLE_BIN) ]; then \
		tar -czf dist/nlci-$(VERSION)-darwin-arm64.tar.gz -C dist $(BINARY) $(APPLE_BIN) README.md; \
	else \
		tar -czf dist/nlci-$(VERSION)-darwin-arm64.tar.gz -C dist $(BINARY) README.md; \
	fi
	@echo "Release: dist/nlci-$(VERSION)-darwin-arm64.tar.gz"
	@shasum -a 256 dist/nlci-$(VERSION)-darwin-arm64.tar.gz

## help: Show this help message
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
