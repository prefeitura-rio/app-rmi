# Build stage
FROM golang:1.24-alpine AS builder

# Install ca-certificates
RUN apk add --no-cache ca-certificates

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

# Build both binaries
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o api cmd/api/main.go && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o sync cmd/sync/main.go

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
