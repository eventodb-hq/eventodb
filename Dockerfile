# Message DB Server Dockerfile
# Multi-stage build for minimal production image

# ============================================================================
# Stage 1: Builder
# ============================================================================
FROM golang:1.24-alpine AS builder

# Install git for Go module downloads if needed
RUN apk add --no-cache git

WORKDIR /app

# Copy Go module files first for layer caching
COPY golang/go.mod golang/go.sum ./
RUN go mod download

# Copy source code
COPY golang/ ./

# Build the binary
# CGO_ENABLED=0 for static binary (needed for alpine/scratch)
# -ldflags="-s -w" for smaller binary (strip debug info)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /eventodb \
    ./cmd/eventodb

# ============================================================================
# Stage 2: Production
# ============================================================================
FROM alpine:3.19

# Install ca-certificates for HTTPS (if needed) and create non-root user
RUN apk --no-cache add ca-certificates && \
    adduser -D -g '' eventodb

WORKDIR /app

# Copy binary from builder
COPY --from=builder /eventodb .

# Set ownership
RUN chown -R eventodb:eventodb /app

# Switch to non-root user
USER eventodb

# Expose default port
EXPOSE 8080

# Health check endpoint
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Default command
# Note: Override --port and --token via environment or command line
ENTRYPOINT ["./eventodb"]
CMD ["--port=8080"]
