# Build stage
FROM golang:1.24-alpine AS builder

# Install ca-certificates and git
RUN apk add --no-cache ca-certificates=20251003-r0 git=2.52.0-r0

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Generate Swagger docs
RUN go install github.com/swaggo/swag/cmd/swag@latest && \
    swag init -g cmd/api/main.go

# Build both binaries with version from git
ARG VERSION
RUN if [ -z "$VERSION" ]; then VERSION=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown"); fi && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X github.com/prefeitura-rio/app-rmi/internal/handlers.Version=$VERSION" -o api cmd/api/main.go && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X github.com/prefeitura-rio/app-rmi/internal/handlers.Version=$VERSION" -o sync cmd/sync/main.go

# Final stage
FROM scratch

WORKDIR /app

# Copy ca-certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy both binaries
COPY --from=builder /app/api .
COPY --from=builder /app/sync .

# Copy swagger docs
COPY --from=builder /app/docs ./docs

# Expose port
EXPOSE 8080

# Set environment variables
ENV GIN_MODE=release

# Default to API service
CMD ["./api"]
