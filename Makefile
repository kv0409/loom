BINARY=loom
VERSION=0.1.0

.PHONY: build install clean vet tidy

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/loom

install: build
	GOBIN=$(HOME)/go/bin go install ./cmd/loom

clean:
	rm -f $(BINARY)
	go clean

vet:
	go vet ./...

tidy:
	go mod tidy
