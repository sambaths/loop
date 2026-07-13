.PHONY: build build-all test lint completions release clean install install-go go-version-check test-install-go test-install-make

MIN_GO_VERSION := 1.22
GO_INSTALL_DIR ?= /usr/local
GO_OS := $(shell uname -s | tr A-Z a-z)
GO_ARCH := $(shell uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build: go-version-check
	go build -ldflags="-X main.Version=$(VERSION) -X main.GOOS=$(GO_OS) -X main.GOARCH=$(GO_ARCH)" -o loop ./cmd/loop

INSTALL_DIR ?= $(HOME)/.local/bin

install: build
	mkdir -p "$(INSTALL_DIR)"
	cp loop "$(INSTALL_DIR)/loop"
	@echo "==> Installed loop $(VERSION) to $(INSTALL_DIR)/loop"

BUILD_PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

build-all: go-version-check
	@for p in $(BUILD_PLATFORMS); do \
		os=$$(echo $$p | cut -d/ -f1); \
		arch=$$(echo $$p | cut -d/ -f2); \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		out="loop-$$os-$$arch$$ext"; \
		echo "==> Building $$out ..."; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags="-X main.Version=$(VERSION) -X main.GOOS=$$os -X main.GOARCH=$$arch" -o "$$out" ./cmd/loop; \
	done
	@echo "==> Done. Built for:"; \
	for p in $(BUILD_PLATFORMS); do \
		os=$$(echo $$p | cut -d/ -f1); \
		arch=$$(echo $$p | cut -d/ -f2); \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		echo "  - loop-$$os-$$arch$$ext"; \
	done

go-version-check:
	@command -v go >/dev/null 2>&1 || { echo "Error: Go is not installed. Run 'make install-go' first."; exit 1; }
	@v=$$(go version | sed -n 's/.*go\([0-9]*\)\.\([0-9]*\).*/\1.\2/p'); \
	major=$$(echo "$$v" | cut -d. -f1); \
	minor=$$(echo "$$v" | cut -d. -f2); \
	if [ -z "$$major" ] || [ "$$major" -lt 1 ] || { [ "$$major" -eq 1 ] && [ "$$minor" -lt 22 ]; }; then \
		echo "Error: Go $(MIN_GO_VERSION)+ required (found: $$(go version | sed -n 's/.*go\([0-9]*\.[0-9]*\).*/\1/p'))"; \
		exit 1; \
	fi

test:
	go test ./...

lint:
	go vet ./...

release:
	@if [ -z "$(TAG)" ]; then \
		echo "Usage: make release TAG=v0.1.0"; \
		exit 1; \
	fi
	git tag -a "$(TAG)" -m "Release $(TAG)"
	git push origin "$(TAG)"

completions: build
	./loop completion bash > loop_completion.bash

clean:
	rm -f loop loop-*-* loop_completion.bash



test-install-go:
	@./test_install_go.sh

test-install-make:
	@./test_install_make.sh

install-go:
	@echo "==> Downloading Go $(MIN_GO_VERSION)+ for $(GO_OS)/$(GO_ARCH) ..."
	@GO_VERSION=$$(curl -sL 'https://go.dev/VERSION?m=text' | head -1 | sed 's/go//'); \
	if [ -z "$$GO_VERSION" ]; then \
		echo "Error: could not fetch latest Go version from go.dev"; \
		exit 1; \
	fi; \
	echo "==> Latest Go version: $$GO_VERSION"; \
	MAJOR=$$(echo "$$GO_VERSION" | cut -d. -f1); \
	MINOR=$$(echo "$$GO_VERSION" | cut -d. -f2); \
	if [ "$$MAJOR" -lt 1 ] || [ "$$MAJOR" -eq 1 -a "$$MINOR" -lt 22 ]; then \
		echo "Error: latest Go version $$GO_VERSION does not meet minimum $(MIN_GO_VERSION)"; \
		exit 1; \
	fi; \
	FILENAME="go$${GO_VERSION}.$(GO_OS)-$(GO_ARCH).tar.gz"; \
	URL="https://go.dev/dl/$$FILENAME"; \
	echo "==> Downloading $$URL ..."; \
	curl -sL "$$URL" -o /tmp/$$FILENAME; \
	echo "==> Extracting to $(GO_INSTALL_DIR) ..."; \
	if [ -w "$(GO_INSTALL_DIR)" ]; then \
		tar -C "$(GO_INSTALL_DIR)" -xzf /tmp/$$FILENAME; \
	else \
		sudo tar -C "$(GO_INSTALL_DIR)" -xzf /tmp/$$FILENAME; \
	fi; \
	rm /tmp/$$FILENAME; \
	echo "==> Installed Go $$GO_VERSION to $(GO_INSTALL_DIR)/go"; \
	echo "    Run 'export PATH=\$$PATH:$(GO_INSTALL_DIR)/go/bin' or add it to your shell profile."
