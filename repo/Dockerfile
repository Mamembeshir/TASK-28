# ============================================
# Stage 1: Build
# ============================================
FROM golang:1.22-bookworm AS builder

WORKDIR /build

# Skip checksum DB — allows building without pre-committed go.sum
ENV GONOSUMDB=*
ENV GOFLAGS=-mod=mod

# Install templ CLI
RUN go install github.com/a-h/templ/cmd/templ@v0.2.778

# Download Tailwind CSS standalone CLI (linux-x64)
RUN curl -sLO https://github.com/tailwindlabs/tailwindcss/releases/download/v3.4.4/tailwindcss-linux-x64 \
    && chmod +x tailwindcss-linux-x64 \
    && mv tailwindcss-linux-x64 /usr/local/bin/tailwindcss

# Copy go.mod first for layer caching
COPY go.mod ./
RUN go mod download

# Copy source
COPY . .

# Generate templ files
RUN templ generate

# Build Tailwind CSS
RUN tailwindcss -i static/css/input.css -o static/css/app.css --minify

# Build Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /build/eduexchange ./cmd/server

# ============================================
# Stage 2: Runtime
# ============================================
FROM debian:bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    bash \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary
COPY --from=builder /build/eduexchange .

# Copy static assets
COPY --from=builder /build/static ./static

# Copy migrations
COPY --from=builder /build/migrations ./migrations

# Copy run_tests.sh (for test execution inside container)
COPY --from=builder /build/run_tests.sh .

# Create data directories
RUN mkdir -p data/uploads data/exports data/statements

EXPOSE 8080

# The binary handles wait-for-db, migrate, seed, and serve internally
CMD ["./eduexchange", "serve"]
