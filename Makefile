BINARY     := bin/ari
MODULE     := github.com/xb/ari
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -ldflags "-s -w -X main.version=$(VERSION)"
GO         := go

.PHONY: all build dev test lint sqlc migrate-new ui-dev ui-build clean help test-e2e-api test-e2e-web test-e2e

## all: Build the binary (default target)
all: build

## build: Build frontend then compile Go binary to bin/ari
build: ui-build
	$(GO) build $(LDFLAGS) -o $(BINARY) ./cmd/ari

## dev: Run the server in development mode
dev:
	ARI_ENV=development $(GO) run $(LDFLAGS) ./cmd/ari run

## test: Run all Go tests with race detection
test:
	$(GO) test -race -count=1 ./...

## lint: Run go vet and staticcheck
lint:
	$(GO) vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

## sqlc: Regenerate sqlc code from SQL queries
sqlc:
	sqlc generate

## migrate-new: Create a new goose migration (usage: make migrate-new NAME=create_users)
migrate-new:
	@test -n "$(NAME)" || (echo "Usage: make migrate-new NAME=description" && exit 1)
	goose -dir internal/database/migrations create $(NAME) sql

## ui-dev: Run the frontend dev server
ui-dev:
	cd web && npm run dev

## ui-build: Build the frontend for production
ui-build:
	cd web && npm run build

## test-e2e-api: Run Go API E2E journey tests
test-e2e-api:
	$(GO) test -v -count=1 -run TestE2E ./cmd/ari/

## test-e2e-web: Run Playwright web E2E tests
test-e2e-web:
	cd web && npx playwright test

## test-e2e: Run all E2E tests (API + Web)
test-e2e: test-e2e-api test-e2e-web

## clean: Remove build artifacts and data directory
clean:
	rm -rf bin/ data/

## help: Show this help message
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
