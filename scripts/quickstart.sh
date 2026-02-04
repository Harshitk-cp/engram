#!/usr/bin/env bash
#
# Engram Quickstart
# Sets up and runs Engram with a demo in under 2 minutes.
#
# Usage: ./quickstart.sh
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log()   { echo -e "${BLUE}==>${NC} $1"; }
success() { echo -e "${GREEN}✓${NC} $1"; }
warn()  { echo -e "${YELLOW}!${NC} $1"; }
error() { echo -e "${RED}✗${NC} $1"; exit 1; }

header() {
    echo ""
    echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║${NC}  $1"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

# ─────────────────────────────────────────────────────────────
header "ENGRAM QUICKSTART"

# Check prerequisites
log "Checking prerequisites..."

if ! command -v docker &> /dev/null; then
    error "Docker is required. Install from https://docker.com"
fi
success "Docker installed"

if ! command -v go &> /dev/null; then
    error "Go is required. Install from https://go.dev"
fi
success "Go installed ($(go version | awk '{print $3}'))"

if ! docker info &> /dev/null; then
    error "Docker daemon not running. Start Docker and retry."
fi
success "Docker running"

# ─────────────────────────────────────────────────────────────
header "STARTING DATABASE"

if docker ps --format '{{.Names}}' | grep -q '^engram-postgres'; then
    success "PostgreSQL already running"
else
    log "Starting PostgreSQL..."
    docker compose up -d postgres

    log "Waiting for database to be ready..."
    for i in {1..30}; do
        if docker compose exec -T postgres pg_isready -U engram &> /dev/null; then
            break
        fi
        sleep 1
    done
    success "PostgreSQL ready"
fi

# ─────────────────────────────────────────────────────────────
header "RUNNING MIGRATIONS"

log "Applying database migrations..."
if command -v mise &> /dev/null; then
    mise run db:migrate 2>/dev/null || go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
        -path migrations -database "postgres://engram:engram@localhost:5432/engram?sslmode=disable" up
else
    go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
        -path migrations -database "postgres://engram:engram@localhost:5432/engram?sslmode=disable" up 2>/dev/null || true
fi
success "Migrations complete"

# ─────────────────────────────────────────────────────────────
header "SEEDING DATABASE"

log "Creating demo tenant and agent..."
SEED_OUTPUT=$(go run ./scripts/seed.go 2>&1) || true

API_KEY=$(echo "$SEED_OUTPUT" | grep "API Key:" | awk '{print $3}')
if [ -z "$API_KEY" ]; then
    warn "Seed may have already run. Checking for existing data..."
    API_KEY="mz_demo_key_not_available"
fi
success "Demo data ready"

# ─────────────────────────────────────────────────────────────
header "STARTING SERVER"

# Environment
export DATABASE_URL="postgres://engram:engram@localhost:5432/engram?sslmode=disable"
export SERVER_PORT="8080"
export LLM_PROVIDER="${LLM_PROVIDER:-mock}"
export EMBEDDING_PROVIDER="${EMBEDDING_PROVIDER:-mock}"

# Check if server already running
if curl -s http://localhost:8080/health &> /dev/null; then
    success "Server already running on :8080"
else
    log "Building and starting server..."
    go build -o ./engram-server ./cmd/server

    ./engram-server &
    SERVER_PID=$!

    log "Waiting for server..."
    for i in {1..30}; do
        if curl -s http://localhost:8080/health &> /dev/null; then
            break
        fi
        sleep 1
    done
    success "Server running on :8080 (PID: $SERVER_PID)"
fi

# ─────────────────────────────────────────────────────────────
header "RUNNING DEMO"

if [ "$API_KEY" != "mz_demo_key_not_available" ]; then
    export ENGRAM_API_KEY="$API_KEY"
    log "Running learning agent example..."
    echo ""
    go run ./examples/01_learning_agent.go
else
    warn "Skipping demo (no API key available)"
    echo ""
    echo "To run examples manually:"
    echo "  1. Get an API key: go run ./scripts/seed.go"
    echo "  2. Export it: export ENGRAM_API_KEY=<key>"
    echo "  3. Run: go run ./examples/01_learning_agent.go"
fi

# ─────────────────────────────────────────────────────────────
header "QUICKSTART COMPLETE"

echo "Engram is running at http://localhost:8080"
echo ""
echo "API Key: $API_KEY"
echo ""
echo "Try these commands:"
echo ""
echo "  # Store a memory"
echo "  curl -X POST http://localhost:8080/v1/memories \\"
echo "    -H 'Authorization: Bearer $API_KEY' \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"agent_id\": \"<agent-id>\", \"content\": \"User prefers dark mode\", \"type\": \"preference\"}'"
echo ""
echo "  # Run more examples"
echo "  export ENGRAM_API_KEY=$API_KEY"
echo "  go run ./examples/02_memory_evolution_demo.go"
echo "  go run ./examples/03_benchmark_mistake_reduction.go"
echo ""
echo "  # Stop services"
echo "  docker compose down"
echo "  pkill -f engram-server"
echo ""
