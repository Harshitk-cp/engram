# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Tidy modules and build the binary
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /engram ./cmd/server/

# Final stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS calls and tzdata for timezones
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 engram && \
    adduser -u 1000 -G engram -s /bin/sh -D engram

# Copy binary from builder
COPY --from=builder /engram /app/engram

# Copy migrations
COPY --from=builder /app/migrations /app/migrations

# Set ownership
RUN chown -R engram:engram /app

# Switch to non-root user
USER engram

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the binary
ENTRYPOINT ["/app/engram"]
