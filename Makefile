.PHONY: all build test fmt clean help

BINARY_NAME := clawcord
BUILD_DIR := build
CMD_DIR := cmd/$(BINARY_NAME)
GO ?= go

UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

ifeq ($(UNAME_S),Linux)
	PLATFORM := linux
	ifeq ($(UNAME_M),x86_64)
		ARCH := amd64
	else ifeq ($(UNAME_M),aarch64)
		ARCH := arm64
	else
		ARCH := $(UNAME_M)
	endif
else ifeq ($(UNAME_S),Darwin)
	PLATFORM := darwin
	ifeq ($(UNAME_M),x86_64)
		ARCH := amd64
	else ifeq ($(UNAME_M),arm64)
		ARCH := arm64
	else
		ARCH := $(UNAME_M)
	endif
else
	PLATFORM := $(UNAME_S)
	ARCH := $(UNAME_M)
endif

BINARY_PATH := $(BUILD_DIR)/$(BINARY_NAME)-$(PLATFORM)-$(ARCH)

all: build

## build: Build the clawcord binary for current platform
build:
	@echo "Building $(BINARY_NAME) for $(PLATFORM)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build -o $(BINARY_PATH) ./$(CMD_DIR)
	@ln -sf $(BINARY_NAME)-$(PLATFORM)-$(ARCH) $(BUILD_DIR)/$(BINARY_NAME)
	@echo "Build complete: $(BINARY_PATH)"

## test: Run all Go tests
test:
	@$(GO) test ./...

## fmt: Format Go code
fmt:
	@find . -name '*.go' -not -path './$(BUILD_DIR)/*' -not -path './bin/*' -print0 | xargs -0 gofmt -w

## clean: Remove build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

## help: Show this help
help:
	@echo "Available targets:"
	@echo "  make build"
	@echo "  make test"
	@echo "  make fmt"
	@echo "  make clean"
	@echo "  make help"
