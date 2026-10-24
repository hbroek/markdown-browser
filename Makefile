WAILS ?= $(shell command -v wails 2>/dev/null || echo $(HOME)/go/bin/wails)

# Keep the macOS deployment target in line with Info.plist (silences linker warnings).
export CGO_CFLAGS  += -mmacosx-version-min=13.0
export CGO_LDFLAGS += -mmacosx-version-min=13.0

.PHONY: dev build test windows linux

dev:
	$(WAILS) dev

build:
	$(WAILS) build

test:
	go vet ./...
	go test ./...

# Cross-compiling works from any OS for Windows; Linux needs a Linux host
# with libgtk-3-dev and libwebkit2gtk-4.1-dev (see README).
windows:
	$(WAILS) build -platform windows/amd64

linux:
	$(WAILS) build -tags webkit2_41
