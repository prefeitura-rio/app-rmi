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
        go install github.com/cosmtrek/air@latest
    fi
    air

# Run tests
test:
    go test -v ./...

# Run tests with coverage
test-coverage:
    go test -v -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html

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
    go build -o bin/api cmd/api/main.go

# Build Docker image
docker-build:
    docker build -t rmi-api .

# Run Docker container
docker-run: docker-build
    docker run -p 8080:8080 --env-file .env rmi-api

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