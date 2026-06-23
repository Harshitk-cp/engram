# Stage 1: build the React/Vite console → dist/ (JS + CSS assets).
# Done inside Docker so the embedded console is always present and consistent;
# we no longer depend on committing build artifacts (console/dist is gitignored).
FROM node:20-alpine AS console

WORKDIR /console

# Install deps against the lockfile first for layer caching.
COPY console/package.json console/package-lock.json ./
RUN npm ci

# Build the SPA (vite emptyOutDir cleans dist/ → fresh, hash-consistent assets).
COPY console/ ./
RUN npm run build

# Stage 2: build the Go binary, embedding the freshly built console.
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Overlay the freshly built console over whatever's in the build context
# (the committed console/dist is stale/incomplete because dist/ is gitignored).
COPY --from=console /console/dist ./console/dist

# Tidy modules and build the binary (go:embed all:dist picks up the real assets)
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /engram ./cmd/server/

# Stage 3: minimal runtime
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS calls and tzdata for timezones
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 engram && \
    adduser -u 1000 -G engram -s /bin/sh -D engram

# Copy binary and migrations from builder
COPY --from=builder /engram /app/engram
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
