# User Service

A production-ready Go REST API microservice for user management, featuring JWT authentication, MongoDB integration, and Docker support.

## Project Structure

```
user-service/
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
- Go 1.22+
- Docker & Docker Compose (for containerized setup)

### Run locally

```bash
# 1. Clone and enter the directory
cd user-service

# 2. Copy environment file
cp .env.example .env

# 3. Install dependencies
go mod tidy

# 4. Start MongoDB (or update MONGO_URI in .env)
docker run -d -p 27017:27017 mongo:7

# 5. Run the server
go run ./cmd/server
```

### Run with Docker Compose

```bash
cp .env.example .env
docker compose -f deployments/docker-compose.yml up --build
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
