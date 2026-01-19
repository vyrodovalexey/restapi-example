# syntax=docker/dockerfile:1

# ==============================================================================
# Stage 1: Build
# ==============================================================================
FROM golang:1.25-alpine AS builder

# Build arguments for version injection
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Install build dependencies
RUN apk add --no-cache ca-certificates git tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the binary with static linking
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /build/bin/server \
    ./cmd/server

# ==============================================================================
# Stage 2: Runtime
# ==============================================================================
FROM alpine:3.21

# OCI Labels
LABEL org.opencontainers.image.title="restapi-example" \
      org.opencontainers.image.description="REST API and WebSocket Server" \
      org.opencontainers.image.vendor="User" \
      org.opencontainers.image.source="https://github.com/vyrodovalexey/restapi-example" \
      org.opencontainers.image.licenses="MIT"

# Build arguments for labels
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Dynamic labels
LABEL org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_TIME}"

# Install runtime dependencies
RUN apk add --no-cache ca-certificates curl tzdata && \
    rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/bin/server /app/server

# Change ownership to non-root user
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Set entrypoint
ENTRYPOINT ["/app/server"]
