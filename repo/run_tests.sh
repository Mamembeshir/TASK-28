#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Minimum Go version required by go.mod
REQUIRED_GO_MINOR=22

# ── Check whether the local Go toolchain is new enough ────────────────────────
go_is_new_enough() {
    command -v go &>/dev/null || return 1
    local ver
    ver="$(go version | grep -oP 'go1\.(\d+)' | head -1)"
    local minor="${ver#go1.}"
    [ -n "$minor" ] && [ "$minor" -ge "$REQUIRED_GO_MINOR" ] 2>/dev/null
}

# ── Decide whether to delegate to Docker ──────────────────────────────────────
# Run inside Docker when:
#   1. We are on the host (no /.dockerenv), OR
#   2. We are inside a container whose Go is too old.
USE_DOCKER=false
if [ ! -f /.dockerenv ]; then
    USE_DOCKER=true
elif ! go_is_new_enough; then
    echo "⚠  Container Go is too old (need ≥1.$REQUIRED_GO_MINOR). Delegating to Docker."
    USE_DOCKER=true
fi

if $USE_DOCKER; then
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

    # Run tests in the builder image (has Go 1.22+ toolchain + templ CLI).
    # --build ensures the image is rebuilt if the Dockerfile changed.
    docker-compose -f "$SCRIPT_DIR/docker-compose.yml" \
        --profile test \
        run --build --rm test bash run_tests.sh "$@"
    STATUS=$?

    exit $STATUS
fi

# ── Inside container with a suitable Go toolchain ────────────────────────────
echo "Go: $(go version)"

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
