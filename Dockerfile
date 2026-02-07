# Multi-stage build for optimized image size

# Stage 1: Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies with retry logic for network issues
RUN --mount=type=cache,target=/go/pkg/mod \
    for i in 1 2 3; do \
      go mod download && go mod verify && break; \
      echo "Retry $i failed, waiting..."; \
      sleep 5; \
    done

# Copy source code
COPY . .

# Build the application with build cache
# - CGO_ENABLED=0: static binary, no gcc/musl needed
# - Removed -a flag: allows Go build cache to work (HUGE speedup on rebuilds)
# - -trimpath: reproducible builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-w -s" \
    -o /app/bin/crawler \
    ./cmd/server

# Stage 2: Runtime stage
FROM alpine:3.21

# Install CA certs + wget for healthcheck (with retry logic)
RUN for i in 1 2 3; do \
      apk update && apk add --no-cache ca-certificates tzdata wget && break; \
      echo "Retry $i failed, waiting..."; \
      sleep 5; \
    done

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/crawler .

# Copy .env.example as default .env
COPY --from=builder /app/.env.example ./.env

# Change ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 9000

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:9000/health || exit 1

# Run the application
CMD ["./crawler"]
