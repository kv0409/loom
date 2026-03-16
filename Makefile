BINARY=loom
VERSION=0.1.16
COMMIT=$(shell git rev-parse --short HEAD)

.PHONY: build install clean vet tidy

build:
	go build -ldflags "-X main.version=$(VERSION) -X main.commitHash=$(COMMIT)" -o $(BINARY) ./cmd/loom

install: build
	GOBIN=$(HOME)/go/bin go install -ldflags "-X main.version=$(VERSION) -X main.commitHash=$(COMMIT)" ./cmd/loom

clean:
	rm -f $(BINARY)
	go clean

vet:
	go vet ./...

tidy:
	go mod tidy
