PREFIX ?= $(HOME)/.local
BIN    := $(PREFIX)/bin/ember

.PHONY: build install uninstall test preview docs

build:
	go build -trimpath -ldflags='-s -w' -o ember .

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
