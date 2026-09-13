SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help
.NOTPARALLEL:

# ------------------------------------
#  Vars
# ------------------------------------

VERSION                ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT                 ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME             ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS                := -ldflags "-X github.com/varunbpatil/temporal-lens/version.Version=$(VERSION) -X github.com/varunbpatil/temporal-lens/version.GitCommit=$(COMMIT) -X github.com/varunbpatil/temporal-lens/version.BuildTime=$(BUILD_TIME)"
WAIT_FOR               ?=
WAIT_FOR_RETRY_SECONDS ?= 1

# ------------------------------------
#  Help
# ------------------------------------

.PHONY: help
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "; prev = "#"} /^[a-zA-Z/_-]+:.*?## / { split($$1, a, "/"); key = (a[2] != "") ? a[1] : "_"; if (key != prev) { if (prev != "#") printf "\n"; prev = key } printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ------------------------------------
#  Go
# ------------------------------------

.PHONY: go/build
go/build: ## Build the application
	go build $(LDFLAGS) -o bin/temporal-lens ./cmd/workflows

.PHONY: go/build-ui
go/build-ui: ## Build the application with embedded UI assets
	go build -tags=ui $(LDFLAGS) -o bin/temporal-lens ./cmd/workflows

.PHONY: go/run
go/run: ## Run the application
	go run ./cmd/workflows

.PHONY: go/seed
go/seed: ## Seed Temporal with deterministic UI test workflows
	go run ./cmd/temporal-seed $(ARGS)

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
	golangci-lint fmt

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

.PHONY: go/mocks
go/mocks: ## Generate Go interface mocks
	mockgen -destination=mocks/workflows.go -package=mocks github.com/varunbpatil/temporal-lens/domains/workflows/ports Mapper,WorkflowService,WorkflowSource,WorkflowRepository

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
	cd ui && npm ci

.PHONY: ui/dev
ui/dev: ## Start UI dev server
	cd ui && npm run dev

.PHONY: ui/build
ui/build: ## Build UI for production
	cd ui && npm run build

.PHONY: ui/lint
ui/lint: ## Lint UI code
	cd ui && npm run lint

.PHONY: ui/lint-fix
ui/lint-fix: ## Fix UI lint issues, including suggestions
	cd ui && npm run lint-fix

.PHONY: ui/fmt
ui/fmt: ## Format UI code
	cd ui && npm run fmt

.PHONY: ui/fmt-check
ui/fmt-check: ## Check UI formatting
	cd ui && npm run fmt-check

# ------------------------------------
#  Docker
# ------------------------------------

.PHONY: docker/build
docker/build: ## Build Docker image
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME) -t temporal-lens .

.PHONY: docker/run
docker/run: ## Run the Docker image with environment variables from .env
	docker_env=(); \
	while IFS= read -r name; do docker_env+=(--env "$$name"); done < <(sed -nE 's/^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)=.*/\1/p' .env); \
	docker run --net host "$${docker_env[@]}" temporal-lens

# ------------------------------------
#  Local dependencies
# ------------------------------------

.PHONY: wait
wait: # Wait for WAIT_FOR, an HTTP(S) URL or host:port
	@WAIT_FOR="$(WAIT_FOR)" WAIT_FOR_RETRY_SECONDS="$(WAIT_FOR_RETRY_SECONDS)" scripts/wait.sh
