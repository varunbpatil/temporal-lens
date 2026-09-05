SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help
.NOTPARALLEL:

.PHONY: help
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "; prev = "#"} /^[a-zA-Z/_-]+:.*?## / { split($$1, a, "/"); key = (a[2] != "") ? a[1] : "_"; if (key != prev) { if (prev != "#") printf "\n"; prev = key } printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

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
go/test: ## Run unit tests
	go test -race ./... -count=1

.PHONY: go/test-integration
go/test-integration: ## Run integration tests
	go test -race -tags=integration ./integration_tests/... -count=1

.PHONY: go/lint
go/lint: ## Run linter
	golangci-lint run --build-tags=all --fix

.PHONY: go/fmt
go/fmt: ## Format code
	golangci-lint fmt --build-tags=all

.PHONY: go/vet
go/vet: ## Run go vet
	go vet -tags=all ./...

.PHONY: go/fix
go/fix: ## Run go fix
	go fix -tags=all ./...

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

.PHONY: proto/fmt
proto/fmt: ## Format protobuf files
	buf format -w

.PHONY: proto/breaking
proto/breaking: ## Check for breaking changes
	buf breaking --against '.git#branch=main'

# ------------------------------------
#  React
# ------------------------------------

.PHONY: ui/install
ui/install: ## Install UI dependencies
	cd ui && npm install

.PHONY: ui/dev
ui/dev: ## Start UI dev server
	cd ui && npm run dev

.PHONY: ui/build
ui/build: ## Build UI for production
	cd ui && npm run build

.PHONY: ui/lint
ui/lint: ## Lint UI code
	cd ui && npm run lint

.PHONY: ui/fmt
ui/fmt: ## Format UI code
	cd ui && npm run fmt

