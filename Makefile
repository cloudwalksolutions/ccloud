SHELL := /bin/bash
.DEFAULT_GOAL := help
-include .env

.PHONY: help
help:            ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build:           ## Build ./ccloud; signs in when GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET are set
	@echo "go build -o ccloud ./cmd/ccloud"
	@go build -o ccloud -ldflags "\
		-X github.com/cloudwalksolutions/ccloud/internal/cloud/cloudwalk.ClientID=$(GOOGLE_CLIENT_ID) \
		-X github.com/cloudwalksolutions/ccloud/internal/cloud/cloudwalk.ClientSecret=$(GOOGLE_CLIENT_SECRET)" \
		./cmd/ccloud

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
