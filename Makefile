SHELL := /bin/bash
.PHONY: all build test clean install lint fmt coverage run help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOLINT=golangci-lint

# Binary output names
SERVER_OUTPUT=bin/godns
CLIENT_OUTPUT=bin/godns-client
CONFIG_OUTPUT=config.yaml

# Source directories
SERVER_SRC=./cmd/godns
CLIENT_SRC=./cmd/godns-client

# Default build target - builds both server and client
all: build

# Help target - displays available make commands
help:
	@echo "GoDNS Makefile"
	@echo "Available commands:"
	@echo "  make build     - Build the server and client"
	@echo "  make server    - Build only the server"
	@echo "  make client    - Build only the client"
	@echo "  make test      - Run tests"
	@echo "  make coverage  - Run tests with coverage report"
	@echo "  make lint      - Run linter"
	@echo "  make fmt       - Format code"
	@echo "  make clean     - Remove build artifacts"
	@echo "  make install   - Install dependencies"
	@echo "  make run       - Run the server"
	@echo "  make config    - Generate default config file"
	@echo "  make help      - Display this help message"

# Build target - builds the server and client
build: server client
	@echo "Build completed successfully!"

# Build the server
server: 
	@echo "Building GoDNS server..."
	@mkdir -p bin
	$(GOBUILD) -o $(SERVER_OUTPUT) $(SERVER_SRC)
	@echo "Server built successfully: $(SERVER_OUTPUT)"

# Build the client
client:
	@echo "Building GoDNS client..."
	@mkdir -p bin
	$(GOBUILD) -o $(CLIENT_OUTPUT) $(CLIENT_SRC)
	@echo "Client built successfully: $(CLIENT_OUTPUT)"

# Test target - runs tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Clean target - cleans build artifacts
clean:
	@echo "Cleaning build artifacts..."
	$(GOCLEAN)
	rm -rf bin/
	rm -f coverage.out
	@echo "Clean completed successfully!"

# Install dependencies
install:
	@echo "Installing dependencies..."
	$(GOMOD) download
	@if ! command -v golangci-lint &> /dev/null; then \
		echo "Installing golangci-lint..." && \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin; \
	fi
	@echo "Dependencies installed successfully!"

# Lint the code
lint:
	@echo "Linting code..."
	@if ! command -v $(GOLINT) &> /dev/null; then \
		echo "golangci-lint not found. Please run 'make install' first."; \
		exit 1; \
	fi
	$(GOLINT) run ./...

# Format the code
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

# Generate coverage report
coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

# Generate default config file
config:
	@echo "Generating default config..."
	@cat > $(CONFIG_OUTPUT) << 'EOF'
server:
  address: "0.0.0.0"
  port: 53
  cache_ttl: 3600

upstream:
  servers:
    - "8.8.8.8:53"
    - "1.1.1.1:53"
  timeout: 5

plugins:
  file:
    enabled: true
    path: "/etc/godns/hosts"
    refresh_interval: 60
  
  memory:
    enabled: true
    entries:
      - hostname: "example.local"
        ips: ["192.168.1.10", "192.168.1.11"]
    
rest:
  enabled: true
  address: "0.0.0.0"
  port: 8080

logging:
  level: "info"
  format: "text"
EOF
	@echo "Config file created: $(CONFIG_OUTPUT)"

# Run the server
run: server config
	@echo "Running GoDNS server..."
	$(SERVER_OUTPUT) -config $(CONFIG_OUTPUT)

