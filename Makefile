BINARY     := tote
MODULE     := github.com/ppiankov/tote
VERSION    ?= dev
LDFLAGS    := -s -w -X $(MODULE)/internal/version.Version=$(VERSION)
GOFLAGS    := -race

.PHONY: build test lint fmt clean deps vet

build:
	go build -ldflags="$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)/

test:
	go test $(GOFLAGS) ./...

lint:
	golangci-lint run --timeout=5m

fmt:
	gofmt -w .
	goimports -w .

vet:
	go vet ./...

clean:
	rm -rf bin/ dist/

deps:
	go mod download
	go mod tidy
