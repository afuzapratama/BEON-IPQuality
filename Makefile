# BEON-IPQuality Makefile

.PHONY: all build clean test run-api run-ingestor run-compiler run-judge

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Binary names
API_BINARY=beon-api
INGESTOR_BINARY=beon-ingestor
COMPILER_BINARY=beon-compiler
JUDGE_BINARY=beon-judge

# Directories
BUILD_DIR=./build
CMD_DIR=./cmd

# Build flags
LDFLAGS=-ldflags "-s -w"

all: build

## Build commands
build: build-api build-ingestor build-compiler build-judge

build-api:
	@echo "Building API server..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(API_BINARY) $(CMD_DIR)/api/main.go

build-ingestor:
	@echo "Building Ingestor service..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(INGESTOR_BINARY) $(CMD_DIR)/ingestor/main.go

build-compiler:
	@echo "Building MMDB Compiler..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(COMPILER_BINARY) $(CMD_DIR)/compiler/main.go

build-judge:
	@echo "Building Judge Node..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(JUDGE_BINARY) $(CMD_DIR)/judge/main.go

## Run commands
run-api:
	@echo "Running API server..."
	$(GOCMD) run $(CMD_DIR)/api/main.go

run-ingestor:
	@echo "Running Ingestor service..."
	$(GOCMD) run $(CMD_DIR)/ingestor/main.go

run-compiler:
	@echo "Running MMDB Compiler..."
	$(GOCMD) run $(CMD_DIR)/compiler/main.go

run-judge:
	@echo "Running Judge Node..."
	$(GOCMD) run $(CMD_DIR)/judge/main.go

## Test commands
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -tags=integration ./tests/...

## Dependency management
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download

deps-update:
	@echo "Updating dependencies..."
	$(GOMOD) tidy
	$(GOGET) -u ./...

## Code quality
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

lint:
	@echo "Running linter..."
	golangci-lint run ./...

vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

## Database
migrate:
	@echo "Running database migrations..."
	@$(GOCMD) run ./scripts/migrate.go up

migrate-down:
	@echo "Rolling back migrations..."
	@$(GOCMD) run ./scripts/migrate.go down

## Docker
docker-build:
	@echo "Building Docker images..."
	docker-compose build

docker-up:
	@echo "Starting Docker containers..."
	docker-compose up -d

docker-down:
	@echo "Stopping Docker containers..."
	docker-compose down

docker-logs:
	docker-compose logs -f

## Cleanup
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

## Help
help:
	@echo "BEON-IPQuality Makefile Commands:"
	@echo ""
	@echo "Build:"
	@echo "  make build           - Build all binaries"
	@echo "  make build-api       - Build API server"
	@echo "  make build-ingestor  - Build Ingestor service"
	@echo "  make build-compiler  - Build MMDB Compiler"
	@echo "  make build-judge     - Build Judge Node"
	@echo ""
	@echo "Run:"
	@echo "  make run-api         - Run API server"
	@echo "  make run-ingestor    - Run Ingestor service"
	@echo "  make run-compiler    - Run MMDB Compiler"
	@echo "  make run-judge       - Run Judge Node"
	@echo ""
	@echo "Test:"
	@echo "  make test            - Run unit tests"
	@echo "  make test-coverage   - Run tests with coverage"
	@echo "  make test-integration - Run integration tests"
	@echo ""
	@echo "Code Quality:"
	@echo "  make fmt             - Format code"
	@echo "  make lint            - Run linter"
	@echo "  make vet             - Run go vet"
	@echo ""
	@echo "Database:"
	@echo "  make migrate         - Run migrations"
	@echo "  make migrate-down    - Rollback migrations"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build    - Build Docker images"
	@echo "  make docker-up       - Start containers"
	@echo "  make docker-down     - Stop containers"
	@echo ""
	@echo "Other:"
	@echo "  make deps            - Download dependencies"
	@echo "  make clean           - Clean build artifacts"
