.PHONY: help build test clean proto docker run

# Default target
help:
	@echo "Available targets:"
	@echo "  build       - Build the project binaries"
	@echo "  test        - Run tests"
	@echo "  clean       - Clean build artifacts"
	@echo "  proto       - Generate Go code from protobuf definitions"
	@echo "  docker      - Build Docker image"
	@echo "  run-server  - Run the task queue server"
	@echo "  run-worker  - Run a worker"
	@echo "  fmt         - Format Go code"
	@echo "  lint        - Run linters"

# Build the project
build:
	@echo "Building task queue server..."
	@go build -o bin/taskqueue-server cmd/server/main.go
	@echo "Building task queue worker..."
	@go build -o bin/taskqueue-worker cmd/worker/main.go
	@echo "Build complete!"

# Run tests
test:
	@echo "Running tests..."
	@go test -v -race -cover ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf internal/api/grpc/pb/
	@go clean
	@echo "Clean complete!"

# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	@mkdir -p internal/api/grpc/pb
	@protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/proto/taskqueue.proto
	@echo "Protobuf code generated!"

# Build Docker image
docker:
	@echo "Building Docker image..."
	@docker build -t taskqueue:latest .
	@echo "Docker image built!"

# Run server
run-server:
	@echo "Running task queue server..."
	@go run cmd/server/main.go

# Run worker
run-worker:
	@echo "Running task queue worker..."
	@go run cmd/worker/main.go

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Format complete!"

# Run linters
lint:
	@echo "Running linters..."
	@go vet ./...
	@echo "Lint complete!"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies installed!"

# Run all checks
check: fmt lint test
	@echo "All checks passed!"
