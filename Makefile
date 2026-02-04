.PHONY: build test lint fmt clean help

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build    Build the gean binary"
	@echo "  test     Run tests with race detector"
	@echo "  lint     Run linters (vet + staticcheck)"
	@echo "  fmt      Format code"
	@echo "  clean    Remove build artifacts"

build:
	@mkdir -p bin
	@echo "Building gean..."
	@go build -ldflags "-X main.version=$(VERSION)" -o bin/gean ./cmd/gean
	@echo "Done: bin/gean"

test:
	go test -race ./...

lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

fmt:
	go fmt ./...

clean:
	rm -rf bin
	go clean
