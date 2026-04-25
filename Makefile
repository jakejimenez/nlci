BINARY     := nlci
APPLE_BIN  := nlci-apple
BUILD_DIR  := bin
GO_CMD     := cmd/nlci
APPLE_DIR  := apple

APPLE_INSTALL_DIR := $(HOME)/.config/nlci/bin

.PHONY: all build build-apple install install-apple clean test lint help

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

## install: Install nlci to /usr/local/bin
install: build
	install -d /usr/local/bin
	install -m 755 $(BUILD_DIR)/$(BINARY) /usr/local/bin/$(BINARY)
	@echo "Installed: /usr/local/bin/$(BINARY)"

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

## help: Show this help message
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
