SHELL := /bin/sh

DATABASE_URL ?= postgresql://secureledger:secureledger@localhost:5432/secureledger?sslmode=disable

.PHONY: help test test-race test-integration vet fmt check-fmt check run run-postgres reconcile-postgres build coverage postgres-up postgres-down compose-up compose-down

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Run unit and HTTP tests
	go test ./...

test-race: ## Run tests with the Go race detector
	go test -race ./...

test-integration: postgres-up ## Run PostgreSQL repository integration tests
	DATABASE_URL='$(DATABASE_URL)' go test -race -tags=integration ./internal/store/postgres

vet: ## Run Go static analysis
	go vet ./...

fmt: ## Format all Go source files
	gofmt -w $$(find . -name '*.go' -type f)

check-fmt: ## Fail when Go files are not formatted
	test -z "$$(gofmt -l .)"

check: check-fmt vet test-race build ## Run the local quality gate

run: ## Run with the in-memory repository
	SECURELEDGER_STORE=memory go run ./cmd/secureledger

run-postgres: postgres-up ## Run with PostgreSQL persistence
	SECURELEDGER_STORE=postgres SECURELEDGER_DATABASE_URL='$(DATABASE_URL)' go run ./cmd/secureledger

reconcile-postgres: ## Compare PostgreSQL balances with the complete journal
	SECURELEDGER_DATABASE_URL='$(DATABASE_URL)' go run ./cmd/secureledger-reconcile

build: ## Build deterministic local binaries
	mkdir -p bin
	go build -buildvcs=false -trimpath -ldflags="-s -w" -o bin/secureledger ./cmd/secureledger
	go build -buildvcs=false -trimpath -ldflags="-s -w" -o bin/secureledger-healthcheck ./cmd/secureledger-healthcheck
	go build -buildvcs=false -trimpath -ldflags="-s -w" -o bin/secureledger-reconcile ./cmd/secureledger-reconcile

coverage: ## Produce the unit-test coverage report
	packages="$$(go list ./... | grep -v '/internal/store/postgres$$')"; \
		go test -coverprofile=coverage.out $$packages
	go tool cover -func=coverage.out

postgres-up: ## Start the local PostgreSQL dependency and wait for readiness
	docker compose up -d --wait postgres

postgres-down: ## Stop local services without deleting database data
	docker compose down

compose-up: ## Build and start the complete PostgreSQL-backed service
	docker compose up -d --build --wait

compose-down: ## Stop the complete service
	docker compose down
