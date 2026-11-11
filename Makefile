SHELL := /bin/bash
.DEFAULT_GOAL := help

.PHONY: help
help:            ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build:           ## Build ./ccloud (no OIDC client: sign-in needs a release build)
	go build -o ccloud ./cmd/ccloud

.PHONY: lint
lint:            ## golangci-lint
	golangci-lint run ./...

.PHONY: unit
unit:            ## Unit tests with the race detector
	go test -race ./...

.PHONY: coverage
coverage:        ## The coverage gate CI enforces
	go test -count=1 ./... -coverprofile=cover.out -covermode=atomic -coverpkg=./...
	go run github.com/vladopajic/go-test-coverage/v2@v2.23.0 --config=./.testcoverage.yml

.PHONY: check
check: lint build unit coverage  ## The local gate
