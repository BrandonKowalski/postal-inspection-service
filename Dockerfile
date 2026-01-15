# Build stage
FROM golang:1.23-alpine AS builder

ARG COMMIT_SHA=unknown
ARG VERSION=dev

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the binary with optimizations and version info
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.CommitSHA=${COMMIT_SHA} -X main.Version=${VERSION}" \
    -o /postal-inspection-service ./cmd/server

# Runtime stage
FROM alpine:3.21

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata curl

# Create non-root user for security
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -h /app -D appuser

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /postal-inspection-service /app/postal-inspection-service

# Create data directory with proper ownership
RUN mkdir -p /data && chown -R appuser:appgroup /data /app

# Switch to non-root user
USER appuser

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

EXPOSE 8080

# Graceful shutdown signal
STOPSIGNAL SIGTERM

CMD ["/app/postal-inspection-service"]
