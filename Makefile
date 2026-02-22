BINARY     := tote
MODULE     := github.com/ppiankov/tote
VERSION    ?= dev
LDFLAGS    := -s -w -X $(MODULE)/internal/version.Version=$(VERSION)
GOFLAGS    := -race

.PHONY: build test lint fmt clean deps vet docker-build generate helm-lint

build:
	go build -ldflags="$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)/

test:
	go test $(GOFLAGS) ./...

lint:
	go vet ./...
	golangci-lint run

fmt:
	gofmt -w .
	@command -v goimports >/dev/null 2>&1 && goimports -w . || true

vet:
	go vet ./...

clean:
	rm -rf bin/ dist/

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(BINARY):$(VERSION) .

generate:
	controller-gen object paths=./api/v1alpha1/ output:dir=./api/v1alpha1/
	controller-gen crd paths=./api/v1alpha1/ output:crd:dir=./config/crd/
	cp config/crd/tote.dev_salvagerecords.yaml charts/tote/crds/

helm-lint:
	helm lint charts/tote/
	helm template tote charts/tote/
	helm template tote charts/tote/ --set tls.enabled=true,tls.secretName=tote-tls
	helm template tote charts/tote/ --set dashboard.enabled=true
	helm template tote charts/tote/ --set pdb.enabled=true,networkPolicy.enabled=true

deps:
	go mod download
	go mod tidy
