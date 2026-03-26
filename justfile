# Load .env file
set dotenv-load

# List all recipes
default:
    @just --list

# Generate Swagger documentation
swagger:
    #!/usr/bin/env sh
    if ! command -v swag > /dev/null; then
        echo "Installing swag..."
        mkdir -p bin
        GOBIN="$PWD/bin" go install github.com/swaggo/swag/cmd/swag@latest
        export PATH="$PWD/bin:$PATH"
    fi
    swag init -g cmd/api/main.go

# Run the application
run: swagger
    go run cmd/api/main.go

# Run the application with hot reload using air
dev: swagger
    #!/usr/bin/env sh
    if ! command -v air > /dev/null; then
        echo "Installing air for hot reload..."
        go install github.com/air-verse/air@latest
    fi
    air

# Run the sync service
run-sync:
    go run cmd/sync/main.go

# Run both API and sync service (in separate terminals)
run-all: swagger
    @echo "Starting API and Sync Service..."
    @echo "API will run on port 8080"
    @echo "Sync Service will run in background"
    @echo ""
    @echo "To run both services, use:"
    @echo "  Terminal 1: just run"
    @echo "  Terminal 2: just run-sync"
    @echo ""
    @echo "Or use tmux:"
    @echo "  tmux new-session -d -s rmi 'just run'"
    @echo "  tmux split-window -h 'just run-sync'"
    @echo "  tmux attach-session -t rmi"

# Start both services using the startup script
start-services: swagger
    @echo "Starting both services using startup script..."
    ./scripts/start_services.sh

# Run the cache demo script
demo-cache:
    @echo "Running multi-level cache demo..."
    ./scripts/demo_cache.sh

# Test the cache system
test-cache:
    @echo "Testing RMI cache system..."
    ./scripts/test_cache_system.sh

# Debug cache issues
debug-cache:
    @echo "Debugging RMI cache system..."
    ./scripts/debug_cache.sh

# Run tests with a specific CPF
test-cpf cpf:
    go test -v ./... -cpf={{cpf}}

# Run tests with default CPF
test:
    go test -v ./...

# Run tests with coverage
test-coverage:
    go test -v -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out

# Run tests with race detection
test-race:
    go test -race -v ./...

# Run tests like GitHub Actions (with service containers)
test-ci:
    #!/usr/bin/env sh
    echo "Running tests with CI environment variables..."
    echo "Note: This requires MongoDB and Redis to be running (use 'just start-deps')"
    echo ""
    MONGODB_URI=${MONGODB_URI:-mongodb://localhost:27017} \
    MONGODB_DATABASE=${MONGODB_DATABASE:-rmi_test} \
    MONGODB_CITIZEN_COLLECTION=${MONGODB_CITIZEN_COLLECTION:-citizens} \
    MONGODB_SELF_DECLARED_COLLECTION=${MONGODB_SELF_DECLARED_COLLECTION:-self_declared} \
    MONGODB_PHONE_MAPPING_COLLECTION=${MONGODB_PHONE_MAPPING_COLLECTION:-phone_mapping} \
    MONGODB_OPT_IN_HISTORY_COLLECTION=${MONGODB_OPT_IN_HISTORY_COLLECTION:-opt_in_history} \
    MONGODB_AUDIT_LOG_COLLECTION=${MONGODB_AUDIT_LOG_COLLECTION:-audit_logs} \
    MONGODB_BETA_GROUPS_COLLECTION=${MONGODB_BETA_GROUPS_COLLECTION:-beta_groups} \
    MONGODB_WHATSAPP_MEMORY_COLLECTION=${MONGODB_WHATSAPP_MEMORY_COLLECTION:-whatsapp_memory} \
    MONGODB_AVATARS_COLLECTION=${MONGODB_AVATARS_COLLECTION:-avatars} \
    MONGODB_MAINTENANCE_REQUEST_COLLECTION=${MONGODB_MAINTENANCE_REQUEST_COLLECTION:-maintenance_requests} \
    MONGODB_LEGAL_ENTITY_COLLECTION=${MONGODB_LEGAL_ENTITY_COLLECTION:-legal_entities} \
    MONGODB_CHAT_MEMORY_COLLECTION=${MONGODB_CHAT_MEMORY_COLLECTION:-chat_memory} \
    MONGODB_CNAE_COLLECTION=${MONGODB_CNAE_COLLECTION:-cnae} \
    MONGODB_DEPARTMENT_COLLECTION=${MONGODB_DEPARTMENT_COLLECTION:-departments} \
    MONGODB_PET_COLLECTION=${MONGODB_PET_COLLECTION:-pets} \
    MONGODB_PETS_SELF_REGISTERED_COLLECTION=${MONGODB_PETS_SELF_REGISTERED_COLLECTION:-pets_self_registered} \
    REDIS_ADDR=${REDIS_ADDR:-localhost:6379} \
    REDIS_PASSWORD=${REDIS_PASSWORD:-} \
    REDIS_DB=${REDIS_DB:-0} \
    REDIS_CLUSTER_ENABLED=${REDIS_CLUSTER_ENABLED:-false} \
    JWT_ISSUER_URL=${JWT_ISSUER_URL:-http://localhost:8080} \
    PORT=${PORT:-8080} \
    CF_LOOKUP_ENABLED=${CF_LOOKUP_ENABLED:-false} \
    WHATSAPP_ENABLED=${WHATSAPP_ENABLED:-false} \
    MCP_SERVER_URL=${MCP_SERVER_URL:-http://localhost:8000} \
    MCP_AUTH_TOKEN=${MCP_AUTH_TOKEN:-test-token} \
    WHATSAPP_API_BASE_URL=${WHATSAPP_API_BASE_URL:-http://localhost:9000} \
    WHATSAPP_API_USERNAME=${WHATSAPP_API_USERNAME:-test} \
    WHATSAPP_API_PASSWORD=${WHATSAPP_API_PASSWORD:-test} \
    WHATSAPP_CAMPAIGN_NAME=${WHATSAPP_CAMPAIGN_NAME:-test} \
    WHATSAPP_COST_CENTER_ID=${WHATSAPP_COST_CENTER_ID:-1} \
    WHATSAPP_HSM_ID=${WHATSAPP_HSM_ID:-1} \
    go test -v -race -coverprofile=coverage.out ./...

# Run E2E tests
test-e2e:
    @echo "Running E2E tests..."
    @if [ -z "$TEST_BASE_URL" ]; then echo "Error: TEST_BASE_URL not set"; exit 1; fi
    @if [ -z "$TEST_KEYCLOAK_URL" ]; then echo "Error: TEST_KEYCLOAK_URL not set"; exit 1; fi
    @if [ -z "$TEST_CPF" ]; then echo "Error: TEST_CPF not set"; exit 1; fi
    go test -v ./tests/e2e/...

# Run tests with a specific CPF and race detection
test-cpf-race cpf:
    go test -race -v ./... -cpf={{cpf}}

# Run tests with a specific CPF and coverage
test-cpf-coverage cpf:
    go test -v -coverprofile=coverage.out ./... -cpf={{cpf}}
    go tool cover -html=coverage.out

# Clean test cache and coverage files
clean:
    go clean -testcache
    rm -f coverage.out coverage.html

# Format code
fmt:
    go fmt ./...

# Lint code
lint:
    #!/usr/bin/env sh
    if ! command -v golangci-lint > /dev/null; then
        echo "Installing golangci-lint..."
        go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    fi
    golangci-lint run

# Check for outdated dependencies
deps-check:
    go list -u -m all

# Update all dependencies
deps-update:
    go get -u ./...
    go mod tidy

# Build the application
build:
    #!/usr/bin/env sh
    VERSION=$(git rev-parse --short HEAD 2>/dev/null || echo "dev")
    go build -ldflags "-X github.com/prefeitura-rio/app-rmi/internal/handlers.Version=$VERSION" -o bin/api cmd/api/main.go

# Build the sync service
build-sync:
    #!/usr/bin/env sh
    VERSION=$(git rev-parse --short HEAD 2>/dev/null || echo "dev")
    go build -ldflags "-X github.com/prefeitura-rio/app-rmi/internal/handlers.Version=$VERSION" -o bin/sync cmd/sync/main.go

# Build both API and sync service
build-all: build build-sync

# Build Docker image
docker-build:
	#!/usr/bin/env sh
	VERSION=$(git rev-parse --short HEAD 2>/dev/null || echo "dev")
	docker build --build-arg VERSION=$VERSION -t rmi-service .

# Run Docker container (API service by default)
docker-run: docker-build
	docker run -p 8080:8080 --env-file .env rmi-service

# Run Docker container (API service)
docker-run-api: docker-build
	docker run -p 8080:8080 --env-file .env rmi-service ./api

# Run Docker container (Sync service)
docker-run-sync: docker-build
	docker run --env-file .env rmi-service ./sync

# Run both services in separate containers
docker-run-all: docker-build
	@echo "Starting API service..."
	docker run -d --name rmi-api -p 8080:8080 --env-file .env rmi-service ./api
	@echo "Starting Sync service..."
	docker run -d --name rmi-sync --env-file .env rmi-service ./sync
	@echo "Services started:"
	@echo "  API: http://localhost:8080"
	@echo "  Sync: running in background"
	@echo ""
	@echo "To stop both services:"
	@echo "  docker stop rmi-api rmi-sync"
	@echo "  docker rm rmi-api rmi-sync"

# Stop and remove all RMI containers
docker-stop-all:
	@echo "Stopping all RMI containers..."
	docker stop rmi-api rmi-sync 2>/dev/null || true
	docker rm rmi-api rmi-sync 2>/dev/null || true
	@echo "All RMI containers stopped and removed"

# Docker Compose commands
docker-compose-up:
	@echo "Starting all services with Docker Compose..."
	docker-compose up -d
	@echo "Services started:"
	@echo "  API: http://localhost:8080"
	@echo "  Sync: running in background"
	@echo "  MongoDB: localhost:27017"
	@echo "  Redis: localhost:6379"

docker-compose-down:
	@echo "Stopping all services..."
	docker-compose down

docker-compose-logs:
	@echo "Showing logs for all services..."
	docker-compose logs -f

docker-compose-logs-api:
	@echo "Showing API service logs..."
	docker-compose logs -f rmi-api

docker-compose-logs-sync:
	@echo "Showing Sync service logs..."
	docker-compose logs -f rmi-sync

# Setup development environment
setup:
    #!/usr/bin/env sh
    if [ ! -f .env ]; then
        echo "Creating .env file from example..."
        cp .env.example .env
    fi
    go mod download
    go mod tidy

# Create Docker network for services
docker-network:
    #!/usr/bin/env sh
    if ! docker network inspect rmi-network >/dev/null 2>&1; then
        docker network create rmi-network
    fi

# Start MongoDB container
mongodb-start: docker-network
    #!/usr/bin/env sh
    if ! docker ps | grep -q rmi-mongodb; then
        docker run -d \
            --name rmi-mongodb \
            --network rmi-network \
            -p 27017:27017 \
            -e MONGODB_DATABASE=rmi \
            mongo:latest
    else
        echo "MongoDB is already running"
    fi

# Start Redis container
redis-start: docker-network
    #!/usr/bin/env sh
    if ! docker ps | grep -q rmi-redis; then
        docker run -d \
            --name rmi-redis \
            --network rmi-network \
            -p 6379:6379 \
            redis:latest
    else
        echo "Redis is already running"
    fi

# Stop MongoDB container
mongodb-stop:
    #!/usr/bin/env sh
    if docker ps | grep -q rmi-mongodb; then
        docker stop rmi-mongodb
        docker rm rmi-mongodb
    fi

# Stop Redis container
redis-stop:
    #!/usr/bin/env sh
    if docker ps | grep -q rmi-redis; then
        docker stop rmi-redis
        docker rm rmi-redis
    fi

# Start all dependencies
start-deps: mongodb-start redis-start
    @echo "All dependencies started"

# Stop all dependencies
stop-deps: mongodb-stop redis-stop
    @echo "All dependencies stopped"

# Show API documentation in browser
docs:
    #!/usr/bin/env sh
    echo "Opening Swagger UI..."
    if command -v xdg-open > /dev/null; then
        xdg-open http://localhost:8080/swagger/index.html
    elif command -v open > /dev/null; then
        open http://localhost:8080/swagger/index.html
    else
        echo "Please open http://localhost:8080/swagger/index.html in your browser"
    fi

load-test base_url cpf_csv token_file oauth_config:
    #!/usr/bin/env sh
    k6 run -e BASE_URL={{base_url}} -e CPF_CSV={{cpf_csv}} -e TOKEN_FILE={{token_file}} -e OAUTH_CONFIG={{oauth_config}} scripts/load_test/load-test.js 