#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ── Host: delegate everything to Docker ───────────────────────────────────────
if [ ! -f /.dockerenv ]; then
    echo "EduExchange Test Suite — delegating to Docker"

    # Copy .env.example → .env if .env is missing (needed by docker-compose)
    if [ ! -f "$SCRIPT_DIR/.env" ] && [ -f "$SCRIPT_DIR/.env.example" ]; then
        cp "$SCRIPT_DIR/.env.example" "$SCRIPT_DIR/.env"
    fi

    # Ensure the db service is up (idempotent — already running is fine)
    docker-compose -f "$SCRIPT_DIR/docker-compose.yml" up -d db

    # Wait for the db healthcheck to pass (already defined in docker-compose.yml)
    echo "Waiting for database to be healthy..."
    until docker-compose -f "$SCRIPT_DIR/docker-compose.yml" exec -T db \
        pg_isready -U eduexchange -q 2>/dev/null; do
        sleep 1
    done

    # Run tests in the builder image (has Go toolchain + templ CLI)
    docker-compose -f "$SCRIPT_DIR/docker-compose.yml" \
        --profile test \
        run --rm test bash run_tests.sh "$@"
    STATUS=$?

    exit $STATUS
fi

# ── Inside container ──────────────────────────────────────────────────────────
if ! command -v go &>/dev/null; then
    echo "ERROR: Go not found inside container." >&2
    exit 1
fi

# Generate templ files (no-op if already up to date)
templ generate 2>/dev/null || true

# Parse flags — no args means run all three layers
RUN_UNIT=false
RUN_INTEGRATION=false
RUN_FRONTEND=false
COVERAGE=false

if [ $# -eq 0 ]; then
    RUN_UNIT=true
    RUN_INTEGRATION=true
    RUN_FRONTEND=true
fi

for arg in "$@"; do
    case $arg in
        --unit)        RUN_UNIT=true ;;
        --integration) RUN_INTEGRATION=true ;;
        --frontend)    RUN_FRONTEND=true ;;
        --coverage)    COVERAGE=true; RUN_UNIT=true; RUN_INTEGRATION=true; RUN_FRONTEND=true ;;
    esac
done

EXIT=0

if $RUN_UNIT; then
    echo ""
    echo "━━━ Unit Tests ━━━"
    go test ./tests/unit/... -v -count=1 || EXIT=1
fi

if $RUN_FRONTEND; then
    echo ""
    echo "━━━ Frontend Render Tests ━━━"
    go test ./tests/frontend/... -v -count=1 || EXIT=1
fi

if $RUN_INTEGRATION; then
    echo ""
    echo "━━━ Integration Tests ━━━"
    go test ./tests/integration/... -v -count=1 -timeout 300s || EXIT=1
fi

if $COVERAGE; then
    echo ""
    echo "━━━ Coverage Report ━━━"
    go test ./tests/unit/... ./tests/frontend/... ./tests/integration/... \
        -coverprofile=coverage.out \
        -covermode=atomic \
        -count=1 2>/dev/null || true
    go tool cover -func=coverage.out | tail -1
    go tool cover -html=coverage.out -o coverage.html 2>/dev/null || true
    echo "Coverage HTML written to coverage.html"
fi

exit $EXIT
