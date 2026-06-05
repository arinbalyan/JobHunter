# ────────────────────────────────────────────────────
# JobHunter — Makefile
# ────────────────────────────────────────────────────

.PHONY: dev build test vet doctor scrape send send-dry cleanup followup inbox migrate syncsecrets all

# ────────────────────────────────────────────────────
# Development
# ────────────────────────────────────────────────────

dev: ## Start Docker services and run doctor
	docker compose up -d
	@echo "Waiting for database to be healthy..."
	@sleep 3
	go run ./cmd/doctor/

# ────────────────────────────────────────────────────
# Build & Test
# ────────────────────────────────────────────────────

build: ## Build all packages
	go build ./...

test: ## Run tests with race detector
	go test -race ./tests/... -count=1

vet: ## Run go vet
	go vet ./...

all: test vet build ## Run tests, vet, and build

# ────────────────────────────────────────────────────
# Workflow commands
# ────────────────────────────────────────────────────

doctor: ## Run diagnostic checks
	go run ./cmd/doctor/

scrape: ## Scrape jobs
	go run ./cmd/scrape/

send: ## Send pending emails
	go run ./cmd/send/

send-dry: ## Dry-run email sending
	go run ./cmd/send/ --dry-run

cleanup: ## Cleanup old jobs
	go run ./cmd/cleanup/

followup: ## Queue follow-up reminders
	go run ./cmd/followup/

inbox: ## Show inbox telemetry
	go run ./cmd/inbox/

migrate: ## Run database migrations
	go run ./cmd/migrate/

syncsecrets: ## Sync .env to GitHub Secrets
	go run ./cmd/syncsecrets/
