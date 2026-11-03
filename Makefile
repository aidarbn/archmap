.PHONY: build test lint

build:
	go build -o bin/archmap ./cmd/archmap

test:
	go test ./...

lint:
	go vet ./...
	golangci-lint run
