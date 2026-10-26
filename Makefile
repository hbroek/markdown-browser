WAILS ?= $(shell command -v wails 2>/dev/null || echo $(HOME)/go/bin/wails)

# Keep the macOS deployment target in line with Info.plist (silences linker warnings).
export CGO_CFLAGS  += -mmacosx-version-min=13.0
export CGO_LDFLAGS += -mmacosx-version-min=13.0

.PHONY: dev build test windows linux sign notarize

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

# macOS release signing. IDENTITY is the "Developer ID Application" certificate;
# NOTARY_PROFILE is a keychain profile created once with:
#   xcrun notarytool store-credentials markdown-browser --apple-id <id> --team-id <team>
IDENTITY ?= Developer ID Application
NOTARY_PROFILE ?= markdown-browser
APP := build/bin/markdown-browser.app

# Sign with the hardened runtime (required for notarization).
sign: build
	codesign --force --deep --options runtime --timestamp --sign "$(IDENTITY)" $(APP)
	codesign --verify --deep --strict --verbose=2 $(APP)

# Submit to Apple, wait, then staple the ticket so it works offline.
notarize: sign
	ditto -c -k --keepParent $(APP) build/bin/notarize.zip
	xcrun notarytool submit build/bin/notarize.zip --keychain-profile "$(NOTARY_PROFILE)" --wait
	xcrun stapler staple $(APP)
	spctl --assess --type execute --verbose $(APP)
	rm build/bin/notarize.zip
