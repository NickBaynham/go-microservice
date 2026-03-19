.PHONY: help setup check-go check-brew install-go install-swag env deps run build clean \
        docker-up docker-down docker-prod-up docker-prod-down \
        docker-test-up docker-test-down \
        certs certs-trust certs-check certs-clean \
        test test-unit test-integration test-integration-local docs

GOMIN        := 1.22
GOROOT_BREW  := $(shell brew --prefix go 2>/dev/null)/bin/go
APP          := go-microservice
CMD          := ./cmd/server
CERT_DIR     := certs
CERT_FILE    := $(CERT_DIR)/dev-cert.pem
KEY_FILE     := $(CERT_DIR)/dev-key.pem

## help: Show this help message
help:
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@grep -E '^## ' Makefile | sed 's/## /  /'
	@echo ""

# ── Setup ────────────────────────────────────────────────────────────────────

## setup: Full setup for new developers (install Go, swag, deps, env file, dev certs)
setup: check-brew install-go install-swag env deps certs docs
	@echo ""
	@echo "✅  Setup complete! Run 'make run' to start the HTTPS server."
	@echo "    Swagger UI will be at https://localhost:8443/swagger/index.html"
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

## install-swag: Install the swag CLI tool for generating Swagger docs
install-swag: check-go
	@echo "→ Installing swag CLI..."
	@which swag > /dev/null 2>&1 && echo "  ✔ swag already installed" || \
		(go install github.com/swaggo/swag/cmd/swag@latest && echo "  ✔ swag installed")

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

# ── Docs ─────────────────────────────────────────────────────────────────────

## docs: Generate Swagger documentation from code annotations
docs: install-swag
	@echo "→ Generating Swagger docs..."
	@swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal
	@echo "  ✔ Docs generated in ./docs/"
	@echo "  ✔ Start the server and visit https://localhost:8443/swagger/index.html"

# ── TLS / Certs ──────────────────────────────────────────────────────────────

## certs: Generate self-signed dev/test certificates (auto-skips if already exist)
certs:
	@echo "→ Checking dev TLS certificates..."
	@mkdir -p $(CERT_DIR)
	@if [ -f $(CERT_FILE) ] && [ -f $(KEY_FILE) ]; then \
		echo "  ✔ Dev certs already exist at $(CERT_DIR)/"; \
	else \
		openssl req -x509 -newkey ec \
			-pkeyopt ec_paramgen_curve:P-256 \
			-keyout $(KEY_FILE) \
			-out $(CERT_FILE) \
			-days 365 -nodes \
			-subj "/CN=localhost/O=go-microservice-dev" \
			-addext "subjectAltName=DNS:localhost,IP:127.0.0.1" 2>/dev/null; \
		echo "  ✔ Self-signed cert generated → $(CERT_DIR)/"; \
		echo "  ℹ️  Your browser will warn about this cert — that's expected in dev."; \
		echo "     To silence it, run: make certs-trust"; \
	fi

## certs-trust: Trust the dev cert in your macOS keychain (removes browser warning)
certs-trust:
	@echo "→ Trusting dev cert in macOS keychain..."
	@sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain $(CERT_FILE)
	@echo "  ✔ Dev cert trusted. Restart your browser."

## certs-check: Show cert expiry date
certs-check:
	@echo "→ Dev cert details:"
	@openssl x509 -in $(CERT_FILE) -noout -subject -dates 2>/dev/null || echo "  ❌  No cert found. Run 'make certs'"

## certs-clean: Remove dev certificates (will be regenerated on next run)
certs-clean:
	@echo "→ Removing dev certificates..."
	@rm -f $(CERT_FILE) $(KEY_FILE)
	@echo "  ✔ Done"

# ── Dev ──────────────────────────────────────────────────────────────────────

## run: Run the HTTPS server locally (self-signed cert, dev mode)
run: check-go env certs docs
	@echo "→ Starting $(APP) with HTTPS..."
	@echo "  ✔ API:        https://localhost:8443"
	@echo "  ✔ Swagger UI: https://localhost:8443/swagger/index.html"
	@go run $(CMD)

## build: Build the binary
build: check-go docs
	@echo "→ Building $(APP)..."
	@go build -ldflags="-s -w" -o bin/$(APP) $(CMD)
	@echo "  ✔ Binary available at bin/$(APP)"

## clean: Remove build artifacts and generated docs
clean:
	@echo "→ Cleaning..."
	@rm -rf bin/ docs/
	@echo "  ✔ Done"

# ── Docker ───────────────────────────────────────────────────────────────────

## docker-up: Start dev environment (HTTPS via self-signed cert)
docker-up: env certs docs
	@echo "→ Starting dev Docker services (HTTPS)..."
	@docker compose -f deployments/docker-compose.yml up --build -d
	@echo "  ✔ API:        https://localhost:8443"
	@echo "  ✔ Swagger UI: https://localhost:8443/swagger/index.html"
	@echo "  ✔ Health:     curl -k https://localhost:8443/health"

## docker-down: Stop dev Docker services
docker-down:
	@echo "→ Stopping dev Docker services..."
	@docker compose -f deployments/docker-compose.yml down
	@echo "  ✔ Done"

## docker-prod-up: Start production environment (Let's Encrypt via Nginx)
docker-prod-up: env
	@echo "→ Starting production Docker services..."
	@echo "  ⚠️  Make sure you have run 'make letsencrypt' first!"
	@docker compose -f deployments/docker-compose.prod.yml up --build -d
	@echo "  ✔ Services running with Let's Encrypt SSL"
	@echo "  ℹ️  Swagger UI is disabled in production"

## docker-prod-down: Stop production Docker services
docker-prod-down:
	@echo "→ Stopping production Docker services..."
	@docker compose -f deployments/docker-compose.prod.yml down
	@echo "  ✔ Done"

# ── Production / Let's Encrypt ───────────────────────────────────────────────

## letsencrypt: Obtain a Let's Encrypt certificate (run once on your server)
## Usage: make letsencrypt DOMAIN=api.example.com EMAIL=you@example.com
letsencrypt:
	@if [ -z "$(DOMAIN)" ] || [ -z "$(EMAIL)" ]; then \
		echo "❌  Usage: make letsencrypt DOMAIN=api.example.com EMAIL=you@example.com"; \
		exit 1; \
	fi
	@echo "→ Obtaining Let's Encrypt certificate for $(DOMAIN)..."
	@docker compose -f deployments/docker-compose.prod.yml up -d nginx
	@docker compose -f deployments/docker-compose.prod.yml run --rm certbot \
		certonly --webroot \
		--webroot-path=/var/www/certbot \
		--email $(EMAIL) \
		--agree-tos \
		--no-eff-email \
		-d $(DOMAIN)
	@echo ""
	@echo "  ✔ Certificate obtained for $(DOMAIN)"
	@echo "  Next: update deployments/nginx/conf.d/app.conf with your domain"
	@echo "        then run: make docker-prod-up"

# ── Testing ───────────────────────────────────────────────────────────────────

## test-unit: Run unit tests only (no server required)
test-unit: check-go
	@echo "→ Running unit tests..."
	@go test ./internal/... -v -count=1 -race
	@echo "  ✔ Unit tests passed"

## test-integration-local: Run integration tests against your local running server
## Requires: make run or make docker-up to be running first
test-integration-local: check-go
	@echo "→ Running integration tests against local server (https://localhost:8443)..."
	@TEST_HOST=localhost \
	 TEST_PORT=8443 \
	 TEST_SCHEME=https \
	 TEST_SKIP_TLS_VERIFY=true \
	 MONGO_URI=mongodb://localhost:27017 \
	 MONGO_DB=userservice \
	 go test ./tests/integration/... -v -count=1 -timeout 60s
	@echo "  ✔ Integration tests passed"

## test-integration: Run integration tests in isolated Docker environment (pipeline-safe)
test-integration: check-go certs
	@echo "→ Starting isolated test environment..."
	@docker compose -f deployments/docker-compose.test.yml up -d --build --wait
	@echo "→ Running integration tests..."
	@TEST_HOST=localhost \
	 TEST_PORT=9443 \
	 TEST_SCHEME=https \
	 TEST_SKIP_TLS_VERIFY=true \
	 MONGO_URI=mongodb://localhost:27117 \
	 MONGO_DB=userservice_test \
	 go test ./tests/integration/... -v -count=1 -timeout 120s; \
	 EXIT_CODE=$$?; \
	 docker compose -f deployments/docker-compose.test.yml down; \
	 exit $$EXIT_CODE

## test: Run unit tests + integration tests in Docker (full pipeline suite)
test: test-unit test-integration
	@echo ""
	@echo "✅  All tests passed!"
	@echo ""

## docker-test-up: Start isolated test environment (for debugging test failures)
docker-test-up: certs
	@echo "→ Starting test Docker environment on port 9443..."
	@docker compose -f deployments/docker-compose.test.yml up -d --build --wait
	@echo "  ✔ Test environment ready"
	@echo "  ✔ API:   https://localhost:9443"
	@echo "  ✔ Mongo: localhost:27117"
	@echo "  ✔ Run tests: MONGO_URI=mongodb://localhost:27117 MONGO_DB=userservice_test make test-integration-local TEST_PORT=9443"

## docker-test-down: Stop isolated test environment
docker-test-down:
	@echo "→ Stopping test Docker environment..."
	@docker compose -f deployments/docker-compose.test.yml down
	@echo "  ✔ Done"