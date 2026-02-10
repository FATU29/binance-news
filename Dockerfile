# Multi-stage build for optimized image size

# Stage 1: Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
# - CGO_ENABLED=0: static binary, no gcc/musl needed
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o crawler ./cmd/server

# Verify binary was created
RUN test -f crawler && ls -lh crawler

# Stage 2: Runtime stage
FROM alpine:3.21

# Install CA certs + wget for healthcheck (with retry logic)
RUN for i in 1 2 3 4 5; do \
      apk update && apk add --no-cache ca-certificates tzdata wget && break || \
      (echo "APK install attempt $i failed, retrying in 10s..." && sleep 10); \
    done

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/crawler .

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
