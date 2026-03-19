.PHONY: help setup check-go check-brew install-go env deps run build clean docker-up docker-down test

GOMIN := 1.22
GOROOT_BREW := $(shell brew --prefix go 2>/dev/null)/bin/go
APP := go-microservice
CMD := ./cmd/server

## help: Show this help message
help:
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@grep -E '^## ' Makefile | sed 's/## /  /'
	@echo ""

## setup: Full setup for new developers (install Go, deps, env file)
setup: check-brew install-go env deps
	@echo ""
	@echo "✅  Setup complete! Run 'make run' to start the server."
	@echo ""

## check-brew: Ensure Homebrew is installed
check-brew:
	@echo "→ Checking Homebrew..."
	@which brew > /dev/null 2>&1 || (echo "❌  Homebrew not found. Install it from https://brew.sh" && exit 1)
	@echo "  ✔ Homebrew found"

## check-go: Ensure Go meets minimum version requirement
check-go:
	@echo "→ Checking Go version..."
	@which go > /dev/null 2>&1 || (echo "❌  Go not found. Run 'make install-go'" && exit 1)
	@GOVER=$$(go version | awk '{print $$3}' | sed 's/go//'); \
	MAJOR=$$(echo $$GOVER | cut -d. -f1); \
	MINOR=$$(echo $$GOVER | cut -d. -f2); \
	REQMINOR=$$(echo $(GOMIN) | cut -d. -f2); \
	if [ "$$MAJOR" -lt 1 ] || [ "$$MINOR" -lt $$REQMINOR ]; then \
		echo "❌  Go $$GOVER found but $(GOMIN)+ required. Run 'make install-go'"; \
		exit 1; \
	fi
	@echo "  ✔ Go $$(go version | awk '{print $$3}') found"

## install-go: Install or upgrade Go via Homebrew
install-go: check-brew
	@echo "→ Installing/upgrading Go..."
	@brew install go 2>/dev/null || brew upgrade go
	@echo "  ✔ Go installed: $$($(GOROOT_BREW) version)"
	@echo ""
	@echo "  ⚠️  Add Go to your PATH if not already set:"
	@echo "     echo 'export PATH=\"$$(brew --prefix go)/bin:\$$PATH\"' >> ~/.zshrc"
	@echo "     source ~/.zshrc"
	@echo ""
	@echo "  ⚠️  If using GoLand, update GOROOT in:"
	@echo "     Settings → Build, Execution, Deployment → Go → GOROOT"
	@echo "     Set to: $$(brew --prefix go)"

## env: Create .env from .env.example if it doesn't exist
env:
	@echo "→ Setting up .env file..."
	@if [ -f .env ]; then \
		echo "  ✔ .env already exists, skipping"; \
	else \
		cp .env.example .env; \
		echo "  ✔ .env created from .env.example"; \
		echo "  ⚠️  Remember to update JWT_SECRET before going to production!"; \
	fi

## deps: Download Go dependencies
deps: check-go
	@echo "→ Downloading dependencies..."
	@go mod tidy
	@echo "  ✔ Dependencies ready"

## run: Run the server locally
run: check-go env
	@echo "→ Starting $(APP)..."
	@go run $(CMD)

## build: Build the binary
build: check-go
	@echo "→ Building $(APP)..."
	@go build -ldflags="-s -w" -o bin/$(APP) $(CMD)
	@echo "  ✔ Binary available at bin/$(APP)"

## test: Run tests
test: check-go
	@echo "→ Running tests..."
	@go test ./... -v

## clean: Remove build artifacts
clean:
	@echo "→ Cleaning..."
	@rm -rf bin/
	@echo "  ✔ Done"

## docker-up: Start all services with Docker Compose
docker-up: env
	@echo "→ Starting Docker services..."
	@docker compose -f deployments/docker-compose.yml up --build -d
	@echo "  ✔ Services running. API at http://localhost:8080"
	@echo "  ✔ Health check: curl http://localhost:8080/health"

## docker-down: Stop all Docker services
docker-down:
	@echo "→ Stopping Docker services..."
	@docker compose -f deployments/docker-compose.yml down
	@echo "  ✔ Done"