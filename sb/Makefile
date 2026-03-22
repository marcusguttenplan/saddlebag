VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build test lint install clean

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o bin/sb .

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run

install: build
	cp bin/sb /usr/local/bin/sb

clean:
	rm -rf bin/
