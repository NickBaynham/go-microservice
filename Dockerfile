# ---- Build stage ----
# Pin linux/amd64 so CI (x86 runners) and ECS Fargate x86 use the same image. On Apple Silicon,
# Docker may emulate amd64 during build; use buildx for multi-arch if you need arm64 too.
FROM --platform=linux/amd64 golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /user-service ./cmd/server

# ---- Runtime stage (Alpine: wget for ECS container health checks) ----
FROM --platform=linux/amd64 alpine:3.21

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /user-service /user-service

EXPOSE 8080 8443

ENTRYPOINT ["/user-service"]
