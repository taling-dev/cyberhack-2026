.DEFAULT_GOAL := help

# ─── Code generation ──────────────────────────────────────────────
.PHONY: gen
gen: ## Run buf lint + buf generate (proto → Go/TS/Python)
	buf lint
	buf format -w
	buf generate

# ─── Lint ─────────────────────────────────────────────────────────
.PHONY: lint
lint: ## Lint all languages
	@echo "==> Go"
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./apps/api/... ./apps/outbox-publisher/... || echo "  golangci-lint not installed, skipping"
	@echo "==> Proto"
	@command -v buf >/dev/null 2>&1 && buf lint || echo "  buf not installed, skipping"
	@echo "==> Frontend"
	@test -f apps/web/package.json && (cd apps/web && pnpm lint) || echo "  apps/web not initialized, skipping"
	@echo "==> Python"
	@test -f apps/ai-worker/pyproject.toml && (cd apps/ai-worker && ruff check .) || echo "  apps/ai-worker not initialized, skipping"

# ─── Build ────────────────────────────────────────────────────────
.PHONY: build
build: ## Build all services
	@echo "TODO: build api, web, ai-worker, outbox-publisher"

# ─── Test ─────────────────────────────────────────────────────────
.PHONY: test
test: ## Run all tests
	@echo "TODO: go test, vitest, pytest"

# ─── Local stack ──────────────────────────────────────────────────
.PHONY: up down stack-up stack-down stack-reset
up: stack-up
down: stack-down

stack-up: ## Start local platform stack (docker compose)
	docker compose up -d

stack-down: ## Stop local platform stack
	docker compose down

stack-reset: ## Reset local stack (down + remove volumes + up)
	docker compose down -v
	docker compose up -d

# ─── Database ─────────────────────────────────────────────────────
.PHONY: db-migrate db-rollback sqlc
db-migrate: ## Run Atlas migrations
	@echo "TODO: atlas migrate apply"

db-rollback: ## Rollback last migration
	@echo "TODO: atlas migrate down"

sqlc: ## Generate sqlc Go code
	@echo "TODO: sqlc generate"

# ─── Helm ─────────────────────────────────────────────────────────
.PHONY: helm-lint
helm-lint: ## Lint all Helm charts
	@echo "TODO: helm lint deploy/helm/*"

# ─── Demo ─────────────────────────────────────────────────────────
.PHONY: demo-reset demo-rehearse
demo-reset: ## Reset demo state (wipe lots/jobs/results, re-seed)
	@echo "TODO: truncate + re-seed"

demo-rehearse: ## Run full demo flow via Playwright
	@echo "TODO: playwright test --project=demo"

# ─── Help ─────────────────────────────────────────────────────────
.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'
