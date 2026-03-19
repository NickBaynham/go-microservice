# ---- Build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /user-service ./cmd/server

# ---- Runtime stage ----
FROM scratch

COPY --from=builder /user-service /user-service
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 8443

ENTRYPOINT ["/user-service"]