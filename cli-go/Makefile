BINARY := clarity
MODULE := github.com/piyush-gambhir/clarity-cli/cli-go
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w -X $(MODULE)/internal/build.Version=$(VERSION) -X $(MODULE)/internal/build.Commit=$(COMMIT) -X $(MODULE)/internal/build.Date=$(DATE)
INSTALL_DIR ?= $(shell go env GOPATH)/bin

.PHONY: build install test vet fmt tidy docs
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .
install:
	mkdir -p "$(INSTALL_DIR)"
	go build -ldflags "$(LDFLAGS)" -o "$(INSTALL_DIR)/$(BINARY)$(shell go env GOEXE)" .
test:
	go test -race -count=1 ./...
vet:
	go vet ./...
fmt:
	gofmt -w .
tidy:
	go mod tidy
docs:
	go run ./tools/gendocs > ../docs/commands.md
