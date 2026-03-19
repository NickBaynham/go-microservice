.PHONY: help setup check-go check-brew install-go env deps run build clean \
        docker-up docker-down docker-prod-up docker-prod-down \
        certs certs-check test

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

## setup: Full setup for new developers (install Go, deps, env file, dev certs)
setup: check-brew install-go env deps certs
	@echo ""
	@echo "✅  Setup complete! Run 'make run' to start the HTTPS server."
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
run: check-go env certs
	@echo "→ Starting $(APP) with HTTPS..."
	@echo "  ✔ API: https://localhost:8443"
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

# ── Docker ───────────────────────────────────────────────────────────────────

## docker-up: Start dev environment (HTTPS via self-signed cert)
docker-up: env certs
	@echo "→ Starting dev Docker services (HTTPS)..."
	@docker compose -f deployments/docker-compose.yml up --build -d
	@echo "  ✔ API at https://localhost:8443 (self-signed cert)"
	@echo "  ✔ Health: curl -k https://localhost:8443/health"

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
