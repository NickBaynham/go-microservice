# go-microservice

A production-ready Go REST API microservice for user management, featuring JWT authentication, MongoDB integration, and Docker support.

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
├── deployments/         # Docker Compose
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
make run     # starts the server at http://localhost:8080
```

That's it! `make setup` handles everything — no need to manually install Go or copy `.env`.

### All available Make commands

| Command | Description |
|---|---|
| `make setup` | Full onboarding — installs Go, creates .env, pulls deps |
| `make run` | Start the server locally |
| `make build` | Compile binary to `bin/` |
| `make test` | Run all tests |
| `make docker-up` | Start app + MongoDB via Docker Compose |
| `make docker-down` | Stop Docker services |
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

### Register
```bash
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice","email":"alice@example.com","password":"secret123"}'
```

### Login
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"secret123"}'
```

### Get current user
```bash
curl http://localhost:8080/me \
  -H "Authorization: Bearer <token>"
```

### Health check
```bash
curl http://localhost:8080/health
```

## Environment Variables

| Variable          | Default                    | Description            |
|-------------------|----------------------------|------------------------|
| PORT              | 8080                       | HTTP port              |
| MONGO_URI         | mongodb://localhost:27017  | MongoDB connection URI |
| MONGO_DB          | userservice                | Database name          |
| JWT_SECRET        | change-me-in-production    | JWT signing secret     |
| JWT_EXPIRE_HOURS  | 24                         | Token expiry in hours  |
