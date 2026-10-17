PREFIX ?= $(HOME)/.local
BIN    := $(PREFIX)/bin/ember

.PHONY: build install uninstall test check fmt vet lint race vuln cover preview gallery demo docs site-media

# pinned tools, run through go run (nothing to install)
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.8.1
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.4.0

# a static binary (no libc to drift under it), stamped with its version
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -X main.version=$(VERSION)' -o ember ./cmd/ember

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

# everything CI checks, in order: run this before a commit
check: fmt
	go vet ./...
	go vet -tags render ./...
	go run $(STATICCHECK) ./...
	go run $(STATICCHECK) -tags render ./...
	go test -count=1 ./...
	go test -race -count=1 ./...

fmt:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...
	go vet -tags render ./...

lint:
	go run $(STATICCHECK) ./...
	go run $(STATICCHECK) -tags render ./...

race:
	go test -race -count=1 ./...

vuln:
	go run $(GOVULNCHECK) ./...

# coverage per function, and the total, in ./preview/cover.out
cover:
	mkdir -p preview
	go test -coverprofile=preview/cover.out ./...
	go tool cover -func=preview/cover.out | tail -1

# The render tools (gallery, previews, demos) are tests behind the render tag:
# go test ./... skips them, these targets always run them afresh (-count=1).

# renders every state of the island to PNGs in ./preview
preview:
	mkdir -p preview
	EMBER_PREVIEW=$(CURDIR)/preview go test -tags render -count=1 -run 'TestPreview|TestFilmstrip' ./internal/island

# every state of the island over a made-up desktop, as stills, in ./preview
gallery:
	mkdir -p preview
	EMBER_PREVIEW=$(CURDIR)/preview EMBER_EDGE_STATS=$(XDG_RUNTIME_DIR)/edge/stats.json go test -tags render -count=1 -run TestGallery ./internal/island

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
	EMBER_DEMO=$(CURDIR)/docs/demo go test -tags render -count=1 -run 'TestDemo$$|TestBanner' -timeout 30m ./internal/island

# the landing page's frames: the same scenes on the page's own dark ground, in docs/site
site-media:
	rm -rf preview/site && mkdir -p preview/site docs/site
	EMBER_GROUND=site EMBER_PREVIEW=$(CURDIR)/preview/site EMBER_DEMO=$(CURDIR)/preview/site go test -tags render -count=1 -run 'TestGallery|TestDemo$$' -timeout 30m ./internal/island
	cp preview/site/*.webp docs/site/
	for f in preview/site/site-*.png; do ffmpeg -loglevel error -y -i $$f -c:v libwebp -quality 92 docs/site/$$(basename $$f .png).webp; done
