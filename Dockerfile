# ---- Build stage ----
# Target linux/amd64 at build time (CI/Makefile pass --platform) so Fargate x86 matches the image.
# Avoid FROM --platform=... here — BuildKit warns on constant platforms; use `docker build --platform`.
FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /user-service ./cmd/server

# ---- Runtime stage (Alpine: wget for ECS container health checks) ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /user-service /user-service

EXPOSE 8080 8443

ENTRYPOINT ["/user-service"]
