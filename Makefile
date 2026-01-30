.PHONY: help install build run dev test test-coverage lint fmt clean docker-build docker-up docker-down docker-logs

# Variables
BINARY_NAME=cts-core
BUILD_DIR=bin
CMD_DIR=cmd/cts-core
CONFIG_FILE=conf/config.yaml

# Default target
help:
	@echo "CTS-Core Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  help           - Show this help message"
	@echo "  install        - Install Go dependencies"
	@echo "  build          - Build the binary"
	@echo "  run            - Run the application"
	@echo "  dev            - Build and run"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  lint           - Run golangci-lint"
	@echo "  fmt            - Format code with go fmt"
	@echo "  clean          - Remove build artifacts and logs"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-up      - Start Docker Compose"
	@echo "  docker-down    - Stop Docker Compose"
	@echo "  docker-logs    - View Docker logs"
	@echo "  db-ping        - Check MySQL database connection"
	@echo "  db-test        - Run database integration tests"
	@echo "  hsm-test       - Run HSM integration tests"

# Install dependencies
install:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies installed."

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)/main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME) -config $(CONFIG_FILE)

# Build and run (development)
dev: build
	@echo "Starting development mode..."
	@./$(BUILD_DIR)/$(BINARY_NAME) -config $(CONFIG_FILE)

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -cover ./...
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run linter
lint:
	@echo "Running golangci-lint..."
	@golangci-lint run || echo "golangci-lint not installed. Run: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$$(go env GOPATH)/bin"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Code formatted."

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@rm -f logs/*.log logs/*.log.*
	@echo "Clean complete."

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	@docker build -t $(BINARY_NAME):latest .
	@echo "Docker image built: $(BINARY_NAME):latest"

# Start Docker Compose
docker-up:
	@echo "Starting Docker Compose..."
	@docker compose up -d
	@echo "Docker Compose started."

# Stop Docker Compose
docker-down:
	@echo "Stopping Docker Compose..."
	@docker compose down
	@echo "Docker Compose stopped."

# View Docker logs
docker-logs:
	@docker compose logs -f $(BINARY_NAME)

# Database operations
.PHONY: db-ping db-test hsm-test

# Ping database
db-ping:
	@echo "Pinging MySQL database..."
	@mysql -h 127.0.0.1 -P 3306 -u root -proot -e "SELECT 1 AS 'Connection OK';" ct_system || echo "Failed to connect. Check MySQL credentials in conf/config.yaml"

# Run database tests
db-test:
	@echo "Running database integration tests..."
	@echo "Note: Tests requiring MySQL will be skipped if MYSQL_HOST is not set"
	@go test -v ./internal/db/...

# Run HSM integration tests
hsm-test:
	@echo "Running HSM integration tests..."
	@echo "Note: Requires HSM service at https://192.168.50.4:8443"
	@HSM_URL=https://192.168.50.4:8443 go test -v -tags=integration ./internal/hsm/...
