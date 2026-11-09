BINARY  := rustore-publisher
MODULE  := github.com/vyachin/rustore-publisher

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.Date=$(DATE)

.PHONY: all build test test-race lint fmt cover clean

all: build

build:
	mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/rustore-publisher

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run
	@if command -v actionlint >/dev/null 2>&1; then actionlint; else echo "actionlint не установлен: проверка workflow пропущена"; fi

fmt:
	@if command -v gofumpt >/dev/null 2>&1; then gofumpt -l -w cmd internal; else gofmt -l -w cmd internal; fi
	go mod tidy

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

clean:
	rm -rf bin dist coverage.out
