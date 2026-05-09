.PHONY: build test lint install clean

build:
	go build -o bin/7geese-pp-cli ./cmd/7geese-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/7geese-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/7geese-pp-mcp ./cmd/7geese-pp-mcp

install-mcp:
	go install ./cmd/7geese-pp-mcp

build-all: build build-mcp
