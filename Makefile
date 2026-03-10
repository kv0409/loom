BINARY=loom
VERSION=0.1.0

.PHONY: build install clean vet tidy

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/loom

install: build
	cp $(BINARY) $(GOPATH)/bin/ || cp $(BINARY) ~/go/bin/

clean:
	rm -f $(BINARY)
	go clean

vet:
	go vet ./...

tidy:
	go mod tidy
