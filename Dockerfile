# Multi-stage build for GoTask

# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build binaries
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/bin/taskqueue-server cmd/server/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/bin/taskqueue-worker cmd/worker/main.go

# Runtime stage - Server
FROM alpine:latest AS server

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy server binary
COPY --from=builder /app/bin/taskqueue-server .

# Expose ports
EXPOSE 8080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run server
ENTRYPOINT ["./taskqueue-server"]

# Runtime stage - Worker
FROM alpine:latest AS worker

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy worker binary
COPY --from=builder /app/bin/taskqueue-worker .

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ps aux | grep taskqueue-worker || exit 1

# Run worker
ENTRYPOINT ["./taskqueue-worker"]
