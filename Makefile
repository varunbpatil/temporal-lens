SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help
.NOTPARALLEL:

.PHONY: help
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z/_-]+:.*?## / { printf "\033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ------------------------------------
#  Go
# ------------------------------------

.PHONY: go/build
go/build: ## Build the application
	go build -o bin/temporal-lens .

.PHONY: go/run
go/run: ## Run the application
	go run .

.PHONY: go/test
go/test: ## Run all tests
	go test -race ./... -count=1

.PHONY: go/lint
go/lint: ## Run linter
	golangci-lint run

.PHONY: go/fmt
go/fmt: ## Format code
	golangci-lint fmt

.PHONY: go/vet
go/vet: ## Run go vet
	go vet ./...

.PHONY: go/fix
go/fix: ## Run go fix
	go fix ./...

.PHONY: go/tidy
go/tidy: ## Tidy and verify go.mod
	go mod tidy
	go mod verify

# ------------------------------------
#  Protobuf
# ------------------------------------

.PHONY: proto/generate
proto/generate: ## Generate protobuf code
	NODE_OPTIONS="--disable-warning=ExperimentalWarning" buf generate

.PHONY: proto/lint
proto/lint: ## Lint protobuf files
	buf lint

.PHONY: proto/breaking
proto/breaking: ## Check for breaking changes
	buf breaking --against '.git#branch=main'

# ------------------------------------
#  React
# ------------------------------------

