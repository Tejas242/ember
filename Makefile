PREFIX ?= $(HOME)/.local
BIN    := $(PREFIX)/bin/ember

.PHONY: build install uninstall test preview gallery demo docs site-media

# a static binary (no libc to drift under it), stamped with its version
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -X main.version=$(VERSION)' -o ember .

# builds, installs the binary, then sets up the service, Claude Code's hooks and the config
install: build
	install -Dm755 ember $(BIN)
	$(BIN) install

uninstall:
	$(BIN) uninstall
	rm -f $(BIN)

test:
	go vet ./...
	go test ./...

# renders every state of the island to PNGs in ./preview
preview:
	mkdir -p preview
	EMBER_PREVIEW=$(CURDIR)/preview go test -run 'TestPreview|TestFilmstrip' ./island

# every state of the island over a made-up desktop, as stills, in ./preview
gallery:
	mkdir -p preview
	EMBER_PREVIEW=$(CURDIR)/preview go test -run TestGallery ./island

# the README's pictures: stills, animated demos (needs ffmpeg with libwebp) and the banner
docs: gallery
	cp preview/gallery-pills.png docs/pills.png
	cp preview/gallery-cards.png docs/cards.png
	cp preview/gallery-requests.png docs/requests.png
	cp preview/gallery-card.png docs/card.png
	cp preview/gallery-request.png docs/request.png
	$(MAKE) demo

demo:
	mkdir -p docs/demo
	EMBER_DEMO=$(CURDIR)/docs/demo go test -run 'TestDemo$$|TestBanner' -timeout 30m ./island

# the landing page's frames: the same scenes on the page's own dark ground, in docs/site
site-media:
	rm -rf preview/site && mkdir -p preview/site docs/site
	EMBER_GROUND=site EMBER_PREVIEW=$(CURDIR)/preview/site EMBER_DEMO=$(CURDIR)/preview/site go test -run 'TestGallery|TestDemo$$' -timeout 30m ./island
	cp preview/site/*.webp docs/site/
	for f in preview/site/site-*.png; do ffmpeg -loglevel error -y -i $$f -c:v libwebp -quality 92 docs/site/$$(basename $$f .png).webp; done
