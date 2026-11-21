.PHONY: build test lint install clean

build:
	go build -o bin/7geese-cli ./cmd/7geese-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/7geese-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/7geese-mcp ./cmd/7geese-mcp

install-mcp:
	go install ./cmd/7geese-mcp

build-all: build build-mcp
