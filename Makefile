APP_NAME := dumpify
CMD_PATH := ./cmd/dumpify
DIST_DIR := dist

GO := go
GOFLAGS := -trimpath
LDFLAGS := -s -w

.PHONY: all test clean build build-macos build-linux release help

all: build

help:
	@echo "Targets:"
	@echo "  build        Build host platform binary"
	@echo "  build-macos  Build macOS binaries (amd64, arm64)"
	@echo "  build-linux  Build Linux binaries (amd64, arm64)"
	@echo "  release      Build macOS + Linux binaries"
	@echo "  test         Run go tests"
	@echo "  clean        Remove dist artifacts"

clean:
	rm -rf $(DIST_DIR)

test:
	$(GO) test ./...

build:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME) $(CMD_PATH)

build-macos:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 $(CMD_PATH)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 $(CMD_PATH)

build-linux:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 $(CMD_PATH)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-linux-arm64 $(CMD_PATH)

release: build-macos build-linux
