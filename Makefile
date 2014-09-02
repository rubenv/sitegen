DEPS = $(shell go list -f '{{range .TestImports}}{{.}} {{end}}' ./...)
PACKAGES = $(shell go list ./...)

all: deps format
	@mkdir -p bin/
	@bash --norc -i ./scripts/build.sh

deps:
	@echo "--> Installing build dependencies"
	@go get -d -v ./...
	@echo $(DEPS) | xargs -n1 go get -d

test: deps
	go list ./... | xargs -n1 go test

integ:
	go list ./... | INTEG_TESTS=yes xargs -n1 go test

format: deps
	@echo "--> Running go fmt"
	@go fmt $(PACKAGES)

.PHONY: all deps integ test
