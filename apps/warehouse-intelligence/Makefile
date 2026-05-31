# Warehouse Intelligence System - Makefile

.PHONY: help setup install test lint format run docker-build docker-run clean deploy

.DEFAULT_GOAL := help

# Colors for output
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

help: ## Display this help message
	@echo "$(BLUE)Warehouse Intelligence System - Available Commands$(NC)"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "$(GREEN)%-20s$(NC) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(YELLOW)Examples:$(NC)"
	@echo "  make setup          # Initial setup"
	@echo "  make test           # Run all tests"
	@echo "  make run-local      # Run locally"
	@echo "  make docker-build   # Build Docker image"
	@echo ""

# ============================================================================
# Development Setup
# ============================================================================

setup: ## Initial setup (creates venv, installs dependencies)
	@echo "$(BLUE)Setting up Warehouse Intelligence System...$(NC)"
	python3 -m venv venv
	. venv/bin/activate && pip install --upgrade pip setuptools wheel
	. venv/bin/activate && pip install -r requirements.txt
	. venv/bin/activate && pip install -e ".[dev,api]"
	@echo "$(GREEN)✓ Setup complete$(NC)"
	@echo "Run: source venv/bin/activate"

install: ## Install dependencies
	@echo "$(BLUE)Installing dependencies...$(NC)"
	pip install -r requirements.txt
	pip install -e ".[dev,api,production]"
	@echo "$(GREEN)✓ Dependencies installed$(NC)"

install-dev: ## Install development dependencies
	@echo "$(BLUE)Installing development dependencies...$(NC)"
	pip install -e ".[dev]"
	@echo "$(GREEN)✓ Dev dependencies installed$(NC)"

# ============================================================================
# Testing
# ============================================================================

test: ## Run all tests
	@echo "$(BLUE)Running tests...$(NC)"
	pytest tests/ -v --tb=short
	@echo "$(GREEN)✓ Tests completed$(NC)"

test-verbose: ## Run tests with verbose output and coverage
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	pytest tests/ -vv --cov=src --cov-report=html --cov-report=term
	@echo "$(GREEN)✓ Coverage report generated: htmlcov/index.html$(NC)"

test-slotting: ## Test Smart Slotting module only
	@echo "$(BLUE)Testing Smart Slotting...$(NC)"
	pytest tests/test_warehouse_intelligence.py::TestSmartSlotting -v

test-hazard: ## Test Hazard Segregation module only
	@echo "$(BLUE)Testing Hazard Segregation...$(NC)"
	pytest tests/test_warehouse_intelligence.py::TestHazardSegregation -v

test-coldchain: ## Test Cold-Chain Monitoring module only
	@echo "$(BLUE)Testing Cold-Chain...$(NC)"
	pytest tests/test_warehouse_intelligence.py::TestColdChainMonitoring -v

test-performance: ## Run performance benchmarks
	@echo "$(BLUE)Running performance tests...$(NC)"
	pytest tests/test_warehouse_intelligence.py::TestPerformance -v

# ============================================================================
# Code Quality
# ============================================================================

lint: ## Run linting checks
	@echo "$(BLUE)Running linting...$(NC)"
	flake8 src/ tests/ --max-line-length=100 --ignore=E203,W503
	@echo "$(GREEN)✓ Linting passed$(NC)"

format: ## Format code with Black
	@echo "$(BLUE)Formatting code...$(NC)"
	black src/ tests/ --line-length=100
	@echo "$(GREEN)✓ Code formatted$(NC)"

type-check: ## Run type checking with mypy
	@echo "$(BLUE)Running type checks...$(NC)"
	mypy src/ --ignore-missing-imports
	@echo "$(GREEN)✓ Type checks passed$(NC)"

quality: lint type-check ## Run all quality checks
	@echo "$(GREEN)✓ All quality checks passed$(NC)"

# ============================================================================
# Running
# ============================================================================

run-local: ## Run locally in development mode
	@echo "$(BLUE)Starting local development server...$(NC)"
	@echo "WAREHOUSE_ENV=development"
	WAREHOUSE_ENV=development python -m warehouse_intelligence.api

run-demo-slotting: ## Run Smart Slotting demo
	@echo "$(BLUE)Running Smart Slotting demo...$(NC)"
	python src/smart_slotting.py

run-demo-hazard: ## Run Hazard Segregation demo
	@echo "$(BLUE)Running Hazard Segregation demo...$(NC)"
	python src/hazard_segregation.py

run-demo-coldchain: ## Run Cold-Chain Monitoring demo
	@echo "$(BLUE)Running Cold-Chain Monitoring demo...$(NC)"
	python src/cold_chain_monitoring.py

# ============================================================================
# Database
# ============================================================================

db-init: ## Initialize database schema
	@echo "$(BLUE)Initializing database...$(NC)"
	python scripts/init_db.py
	@echo "$(GREEN)✓ Database initialized$(NC)"

db-migrate: ## Run database migrations
	@echo "$(BLUE)Running migrations...$(NC)"
	alembic upgrade head
	@echo "$(GREEN)✓ Migrations completed$(NC)"

db-seed: ## Seed database with sample data
	@echo "$(BLUE)Seeding database...$(NC)"
	python scripts/seed_data.py
	@echo "$(GREEN)✓ Database seeded$(NC)"

db-reset: ## Reset database (warning: destructive)
	@echo "$(RED)⚠ WARNING: This will delete all data$(NC)"
	@echo "Press Ctrl+C to cancel, or wait 5 seconds to continue..."
	sleep 5
	python scripts/reset_db.py
	@echo "$(GREEN)✓ Database reset$(NC)"

# ============================================================================
# Docker
# ============================================================================

docker-build: ## Build Docker image
	@echo "$(BLUE)Building Docker image...$(NC)"
	docker build -t warehouse-intelligence:latest \
		-t warehouse-intelligence:1.0.0 \
		-f Dockerfile .
	@echo "$(GREEN)✓ Docker image built$(NC)"

docker-run: ## Run Docker container
	@echo "$(BLUE)Running Docker container...$(NC)"
	docker run -p 8000:8000 \
		-e WAREHOUSE_ENV=development \
		-e DB_HOST=host.docker.internal \
		warehouse-intelligence:latest

docker-compose-up: ## Start all services with Docker Compose
	@echo "$(BLUE)Starting Docker Compose services...$(NC)"
	docker-compose up -d
	@echo "$(GREEN)✓ Services started$(NC)"
	docker-compose logs -f warehouse-intelligence

docker-compose-down: ## Stop all Docker Compose services
	@echo "$(BLUE)Stopping Docker Compose services...$(NC)"
	docker-compose down
	@echo "$(GREEN)✓ Services stopped$(NC)"

docker-logs: ## View Docker container logs
	@echo "$(BLUE)Showing Docker logs...$(NC)"
	docker logs -f warehouse-intelligence

# ============================================================================
# Deployment
# ============================================================================

deploy-dev: ## Deploy to development environment
	@echo "$(BLUE)Deploying to development...$(NC)"
	docker build -t warehouse-intelligence:dev .
	# Add your deployment commands here
	@echo "$(GREEN)✓ Deployment to dev complete$(NC)"

deploy-staging: ## Deploy to staging environment
	@echo "$(BLUE)Deploying to staging...$(NC)"
	@echo "Prerequisites: kubectl context set to staging"
	kubectl apply -f deploy/k8s/staging/ -n warehouse-intelligence
	@echo "$(GREEN)✓ Deployment to staging complete$(NC)"

deploy-prod: ## Deploy to production environment
	@echo "$(RED)⚠ Deploying to PRODUCTION$(NC)"
	@echo "Prerequisites: kubectl context set to production"
	@echo "Press Ctrl+C to cancel, or wait 5 seconds to continue..."
	sleep 5
	kubectl apply -f deploy/k8s/production/ -n warehouse-intelligence
	@echo "$(GREEN)✓ Deployment to production complete$(NC)"

# ============================================================================
# Cleanup
# ============================================================================

clean: ## Remove generated files and caches
	@echo "$(BLUE)Cleaning up...$(NC)"
	find . -type d -name __pycache__ -exec rm -rf {} + 2>/dev/null || true
	find . -type f -name "*.pyc" -delete
	find . -type d -name ".pytest_cache" -exec rm -rf {} + 2>/dev/null || true
	find . -type d -name ".mypy_cache" -exec rm -rf {} + 2>/dev/null || true
	find . -type d -name "*.egg-info" -exec rm -rf {} + 2>/dev/null || true
	rm -rf htmlcov/ .coverage
	@echo "$(GREEN)✓ Cleanup complete$(NC)"

clean-docker: ## Remove Docker images and containers
	@echo "$(BLUE)Removing Docker images...$(NC)"
	docker rmi warehouse-intelligence:latest warehouse-intelligence:1.0.0 || true
	@echo "$(GREEN)✓ Docker cleanup complete$(NC)"

# ============================================================================
# Documentation
# ============================================================================

docs-build: ## Build documentation
	@echo "$(BLUE)Building documentation...$(NC)"
	cd docs && make html
	@echo "$(GREEN)✓ Documentation built: docs/_build/html/index.html$(NC)"

docs-serve: ## Serve documentation locally
	@echo "$(BLUE)Serving documentation on http://localhost:8888$(NC)"
	python -m http.server 8888 -d docs/_build/html

# ============================================================================
# Monitoring & Health
# ============================================================================

health: ## Check system health
	@echo "$(BLUE)Checking system health...$(NC)"
	@echo "API Status: $$(curl -s http://localhost:8000/health || echo 'OFFLINE')"
	@echo "Database: $$(psql -h localhost -U postgres -d warehouse_dev -c 'SELECT 1' >/dev/null 2>&1 && echo 'OK' || echo 'OFFLINE')"
	docker ps -a --filter "name=warehouse" --format "table {{.Names}}\t{{.Status}}"

logs: ## View application logs
	@echo "$(BLUE)Showing recent logs...$(NC)"
	tail -f logs/warehouse_intelligence.log

# ============================================================================
# Version & Info
# ============================================================================

version: ## Show version info
	@echo "Warehouse Intelligence System v1.0.0"
	@python --version
	@pip show warehouse-intelligence 2>/dev/null || echo "Not installed"

info: ## Show system information
	@echo "$(BLUE)System Information$(NC)"
	@echo "Python: $$(python --version)"
	@echo "Pip: $$(pip --version)"
	@echo "OS: $$(uname -s)"
	@echo ""
	@echo "$(BLUE)Project Structure$(NC)"
	@echo "Source files: $$(find src -name '*.py' | wc -l)"
	@echo "Test files: $$(find tests -name '*.py' | wc -l)"
	@echo "Lines of code: $$(find src -name '*.py' -exec wc -l {} + | tail -1 | awk '{print $$1}')"

# ============================================================================
# Shortcuts
# ============================================================================

dev: setup test run-local ## Full development workflow

check: quality test ## Run quality checks and tests

all: clean install test lint format ## Do everything

.PHONY: $(MAKECMDGOALS)
