BINARY := agentic-pdf
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)

.PHONY: build install test cross clean sync-viewer

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(BINARY) ./cmd/agentic-pdf

install:
	go install -ldflags "-X main.version=$(VERSION)" ./cmd/agentic-pdf

test:
	go test ./...

# Cross-compile static binaries into dist/
cross:
	for os_arch in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do \
		os=$${os_arch%/*}; arch=$${os_arch#*/}; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "-s -w -X main.version=$(VERSION)" \
			-o dist/$(BINARY)-$$os-$$arch$(if $(filter windows,$$os),.exe,) ./cmd/agentic-pdf; \
	done

# The local viewer and the GitHub Pages viewer share the same HTML.
# (The Pages site lives in docs/: index = spec guide, viewer/ = viewer.)
sync-viewer:
	cp internal/viewer/viewer.html docs/viewer/index.html

clean:
	rm -rf bin dist
