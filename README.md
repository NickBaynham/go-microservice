# go-microservice

A production-ready Go REST API microservice for user management, featuring JWT authentication, MongoDB integration, Docker support, and TLS encryption (self-signed for dev/test, Let's Encrypt for production).

## Project Structure

```
go-microservice/
├── Makefile             # Developer commands
├── cmd/server/          # Entry point
├── internal/
│   ├── auth/            # JWT helpers
│   ├── config/          # Environment config
│   ├── handlers/        # HTTP handlers
│   ├── middleware/       # Auth & role middleware
│   ├── models/          # Request/response models
│   └── repository/      # MongoDB data layer
├── certs/               # Dev/test self-signed certs (git-ignored)
├── deployments/
│   ├── docker-compose.yml       # Dev stack
│   ├── docker-compose.prod.yml  # Production stack (Nginx + certbot)
│   └── nginx/conf.d/app.conf    # Nginx SSL config
├── internal/tls/        # TLS config loader
├── Dockerfile
├── .env.example
└── go.mod
```

## API Endpoints

### Public
| Method | Path              | Description          |
|--------|-------------------|----------------------|
| GET    | /health           | Health check         |
| POST   | /auth/register    | Register a new user  |
| POST   | /auth/login       | Login & get JWT      |

### Protected (JWT required)
| Method | Path         | Description             |
|--------|--------------|-------------------------|
| GET    | /me          | Get current user        |
| PUT    | /users/:id   | Update user (self/admin) |

### Admin only (JWT + role=admin required)
| Method | Path         | Description    |
|--------|--------------|----------------|
| GET    | /users       | List all users |
| GET    | /users/:id   | Get user by ID |
| DELETE | /users/:id   | Delete user    |

## Getting Started

### Prerequisites
- [Homebrew](https://brew.sh) (macOS) — used to install Go
- Docker & Docker Compose (for containerized setup)

### Quickstart (new developers)

```bash
git clone <repo-url>
cd go-microservice
make setup   # installs Go, creates .env, downloads dependencies
make run     # starts HTTPS server at https://localhost:8443
```

That's it! `make setup` handles everything — Go installation, `.env`, dependencies, and self-signed dev certificates.

### All available Make commands

| Command | Description |
|---|---|
| `make setup` | Full onboarding — installs Go, creates .env, pulls deps, generates certs |
| `make run` | Start HTTPS server locally on :8443 (self-signed cert) |
| `make build` | Compile binary to `bin/` |
| `make test` | Run all tests |
| `make certs` | Generate self-signed dev/test certificates |
| `make certs-trust` | Trust dev cert in macOS keychain (removes browser warning) |
| `make certs-check` | Show dev cert expiry date |
| `make docker-up` | Start dev stack (HTTPS on :8443) |
| `make docker-down` | Stop dev Docker services |
| `make docker-prod-up` | Start production stack (Nginx + Let's Encrypt) |
| `make docker-prod-down` | Stop production Docker services |
| `make letsencrypt` | Obtain Let's Encrypt cert (requires DOMAIN= and EMAIL=) |
| `make clean` | Remove build artifacts |
| `make help` | List all commands |

### Run with Docker Compose

```bash
make docker-up
```

### ⚠️ GoLand users

GoLand sets `GOROOT` automatically and may point to an old Go installation. If you see `cannot find GOROOT directory`, update it in:

**Settings → Build, Execution, Deployment → Go → GOROOT**

Set it to the output of:
```bash
brew --prefix go
```

## Example Usage

> Note: Use `-k` with curl in dev to skip self-signed cert verification. In production, omit `-k`.

### Register
```bash
curl -k -X POST https://localhost:8443/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice","email":"alice@example.com","password":"secret123"}'
```

### Login
```bash
curl -k -X POST https://localhost:8443/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"secret123"}'
```

### Get current user
```bash
curl -k https://localhost:8443/me \
  -H "Authorization: Bearer <token>"
```

### Health check
```bash
curl -k https://localhost:8443/health
```

## Environment Variables

| Variable         | Default                   | Description                                      |
|------------------|---------------------------|--------------------------------------------------|
| PORT             | 8080                      | Internal HTTP port                               |
| TLS_PORT         | 8443                      | HTTPS port (dev/test)                            |
| ENV              | development               | `development`, `test`, or `production`           |
| MONGO_URI        | mongodb://localhost:27017 | MongoDB connection URI                           |
| MONGO_DB         | userservice               | Database name                                    |
| JWT_SECRET       | change-me-in-production   | JWT signing secret                               |
| JWT_EXPIRE_HOURS | 24                        | Token expiry in hours                            |
| TLS_CERT         | _(auto in dev)_           | Path to cert PEM (required in production)        |
| TLS_KEY          | _(auto in dev)_           | Path to key PEM (required in production)         |

## TLS / SSL

This service uses **self-signed certificates** in dev/test and **Let's Encrypt** in production.

### Development & Test (self-signed)

Certs are auto-generated to `./certs/` on first run. No manual steps needed:

```bash
make run        # generates certs automatically, starts HTTPS on :8443
```

Your browser will show a security warning — that's expected. To silence it on macOS:

```bash
make certs-trust   # adds the dev cert to your macOS keychain
```

Test with curl (skip cert verification in dev):
```bash
curl -k https://localhost:8443/health
```

### Production (Let's Encrypt via Nginx)

1. Point your domain's DNS A record to your server IP.

2. Update `deployments/nginx/conf.d/app.conf` — replace `YOUR_DOMAIN_HERE` with your domain.

3. Obtain the certificate (run once on your server):
```bash
make letsencrypt DOMAIN=api.example.com EMAIL=you@example.com
```

4. Start the production stack:
```bash
make docker-prod-up
```

Certbot runs as a sidecar container and **auto-renews** the certificate every 12 hours.

### How it works

| Environment | TLS handled by | Certificate |
|---|---|---|
| `development` | Go directly | Self-signed (auto-generated) |
| `test` | Go directly | Self-signed (auto-generated) |
| `production` | Nginx reverse proxy | Let's Encrypt (certbot) |

In production, Nginx terminates SSL on port 443 and proxies plain HTTP to the Go app internally on port 8080 — so your Go code never changes between environments.

## Swagger / API Documentation

Swagger docs are auto-generated from annotations in the handler code using [swaggo/swag](https://github.com/swaggo/swag).

### Generate & view docs

```bash
make docs   # generates ./docs/ from code annotations
make run    # start the server
```

Then open in your browser:
```
https://localhost:8443/swagger/index.html
```

You can authorize with a JWT directly in the UI — click the **Authorize** button and enter `Bearer <your_token>`.

> Swagger UI is automatically **disabled in production** (`ENV=production`).

### Regenerate after changes

Any time you add or modify a handler, regenerate the docs:

```bash
make docs
```

Or it runs automatically as part of `make run` and `make build`.