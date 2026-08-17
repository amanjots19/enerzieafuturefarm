SHELL := /bin/bash
GOBIN := $(shell go env GOPATH)/bin

# Coverage is measured over product code only. Two exclusions, both test
# infrastructure rather than shipped behaviour:
#   cmd/       — process wiring (signal handling, ListenAndServe), no branching
#                worth asserting
#   mongotest/ — the in-process fake MongoDB used BY tests. It has its own
#                suite, but its remaining lines are defensive wire-protocol
#                guards that a well-behaved driver never triggers.
# The gate is the total across the measured set; `make cover` also prints the
# per-package table so a single weak package stays visible.
COVER_PKGS ?= $(shell go list ./internal/... | grep -v '/internal/mongotest')
COVER_MIN  ?= 90

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show the available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: tidy
tidy: ## Sync go.mod / go.sum
	go mod tidy

.PHONY: fmt
fmt: ## Format the module in place
	gofmt -s -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## gofmt check + go vet + golangci-lint (must be clean)
	@unformatted=$$(gofmt -s -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "✗ these files are not gofmt'd:"; echo "$$unformatted"; \
		echo "  run: make fmt"; exit 1; \
	fi
	@echo "✓ gofmt"
	@go vet ./... && echo "✓ go vet"
	@if [ ! -x "$(GOBIN)/golangci-lint" ]; then \
		echo "✗ golangci-lint not found at $(GOBIN)"; \
		echo "  run: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; \
		exit 1; \
	fi
	@$(GOBIN)/golangci-lint run ./... && echo "✓ golangci-lint"

.PHONY: test
test: ## Run the full suite with the race detector
	go test ./... -race -count=1

.PHONY: cover
cover: ## Run tests and fail if coverage drops below COVER_MIN
	@go test $(COVER_PKGS) -covermode=atomic -coverprofile=coverage.out -count=1
	@echo
	@echo "per-package coverage:"
	@go test $(COVER_PKGS) -cover -count=1 | sed 's/^/  /'
	@echo
	@total=$$(go tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | tr -d '%'); \
	echo "total coverage: $$total% (minimum $(COVER_MIN)%)"; \
	if [ "$$(echo "$$total < $(COVER_MIN)" | bc -l)" -eq 1 ]; then \
		echo "✗ coverage below the $(COVER_MIN)% floor"; \
		echo "  uncovered lines:"; \
		go tool cover -func=coverage.out | awk '$$3 != "100.0%" && $$1 != "total:"' | sed 's/^/    /'; \
		exit 1; \
	fi
	@echo "✓ coverage gate passed"

.PHONY: cover-html
cover-html: ## Open the coverage report in a browser
	@go test $(COVER_PKGS) -covermode=atomic -coverprofile=coverage.out -count=1
	@go tool cover -html=coverage.out

.PHONY: check
check: lint cover ## What must pass before a task is done

.PHONY: run
run: ## Run the API locally (reads .env)
	@set -a && [ -f .env ] && . ./.env; set +a; go run ./cmd/api

.PHONY: seed
seed: ## Load the catalogue into MongoDB (idempotent; reads .env)
	@set -a && [ -f .env ] && . ./.env; set +a; go run ./cmd/seed

.PHONY: admin-password
admin-password: ## Generate a bcrypt hash for ADMIN_PASSWORD_HASH (reads password from stdin)
	@go run ./cmd/adminpass

.PHONY: build
build: ## Build the API binary into bin/
	go build -ldflags "-X main.version=$$(git rev-parse --short HEAD 2>/dev/null || echo dev)" \
		-o bin/api ./cmd/api

.PHONY: clean
clean: ## Remove build and coverage artefacts
	rm -rf bin coverage.out
