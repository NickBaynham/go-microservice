# ---- Build stage ----
FROM --platform=linux/arm64 golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /user-service ./cmd/server

# ---- Runtime stage (Alpine: wget for ECS container health checks) ----
FROM --platform=linux/arm64 alpine:3.21

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /user-service /user-service

EXPOSE 8080 8443

ENTRYPOINT ["/user-service"]
