BINARY := agentic-pdf
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)

.PHONY: build install test cross clean sync-viewer sync-embedded-demos

build: sync-embedded-demos
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(BINARY) ./cmd/agentic-pdf

install: sync-embedded-demos
	go install -ldflags "-X main.version=$(VERSION)" ./cmd/agentic-pdf

test:
	go test ./...

# Cross-compile static binaries into dist/
cross: sync-embedded-demos
	for os_arch in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do \
		os=$${os_arch%/*}; arch=$${os_arch#*/}; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "-s -w -X main.version=$(VERSION)" \
			-o dist/$(BINARY)-$$os-$$arch$(if $(filter windows,$$os),.exe,) ./cmd/agentic-pdf; \
	done

# The local viewer binary embeds internal/viewer/viewer.html + demo PDFs.
# docs/demo/ is the source of truth for generated samples; sync-embedded-demos
# refreshes the binary's embedded copies, and sync-viewer publishes the viewer
# HTML into docs/ for GitHub Pages.
sync-embedded-demos:
	cp docs/demo/*.agent.pdf internal/viewer/demo/

sync-viewer:
	cp internal/viewer/viewer.html docs/viewer/index.html

clean:
	rm -rf bin dist
