BIN_CLI = bbclip
DATE = $(shell date +%Y%m%d%H)
GIT_HASH = g$(shell git rev-parse --short HEAD || echo "local")
PKG = main
GOFLAGS = -trimpath
VERSION := $(shell git describe --tags --abbrev=0 || echo "dev")

build:
	go mod tidy
	go build $(GOFLAGS) -ldflags="\
		-s -w -X '$(PKG).version=$(VERSION)' \
		-X '$(PKG).commit=$(GIT_HASH)'" \
		-o ${BIN_CLI}

install-local:
	rsync -azP ${BIN_CLI} ~/.local/bin/
	go clean
	rm ${BIN_CLI}

install:
	rsync -azP ${BIN_CLI} /usr/bin/
	go clean

clean:
	go clean
	rm ${BIN_CLI}

.PHONY: test

test:
	go test ./...

test-verbose:
	go test ./... -v
