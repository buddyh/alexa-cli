BINARY_NAME=alexacli
POLLER_BINARY_NAME=alexa-poller
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

.PHONY: build build-poller build-tools install clean test

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/alexa

build-poller:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(POLLER_BINARY_NAME) ./cmd/alexa-poller

build-tools: build build-poller

install: build
	cp bin/$(BINARY_NAME) /usr/local/bin/

clean:
	rm -rf bin/

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

lint:
	golangci-lint run

# Cross-compilation
build-all:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/alexa
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/alexa
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/alexa
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/alexa
