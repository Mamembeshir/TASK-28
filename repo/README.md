# EduExchange

Offline Learning Resource Exchange & Supplier Fulfillment Console for school districts.

## Prerequisites

- Docker
- Docker Compose

No other tools are required to run or test the application. All builds, migrations, and tests execute inside Docker containers.

## Quick Start

1. Create a `.env` file in the `repo/` directory with the required secrets:

```bash
cp .env.example .env   # if .env.example exists, or create manually:
# SESSION_SECRET=<random-32-char-string>
# ENCRYPTION_KEY=<exactly-32-byte-string>
```

`SESSION_SECRET` secures browser sessions and `ENCRYPTION_KEY` (must be exactly 32 bytes) is used for AES-256-GCM at-rest encryption of sensitive fields. Both are required; the server will refuse to start without them.

2. Start the application:

```bash
docker-compose up
```

Open [http://localhost:8080](http://localhost:8080).

The server automatically runs migrations and seeds a minimal set of demo users on first boot.

## Demo Accounts

The server seeds a minimal set of accounts on first boot. For a richer dataset (published resources, engagement data, supplier orders) run the full seed via Docker:

```bash
# Preferred: runs inside the already-running container (no local Go required)
make seed

# Alternative: reset the DB, apply migrations, then seed in one step
make fresh
```

If you need to run the rich seed locally (requires Go and a reachable Postgres):

```bash
DATABASE_URL=postgres://eduexchange:eduexchange@localhost:5432/eduexchange?sslmode=disable \
ENCRYPTION_KEY=change-me-32-byte-key-here!!!!!! \
go run ./cmd/seed
```

| Username | Password | Roles |
|---|---|---|
| admin | Admin12345!@ | Administrator |
| author1 | Author12345!@ | Author, Regular User |
| reviewer1 | Review12345!@ | Reviewer, Regular User |
| supplier1 | Supply12345!@ | Supplier |
| teacher1 | Teach12345!@ | Regular User |

## Running Tests

```bash
# All tests
./run_tests.sh

# Or via Makefile
make test

# Specific suites
make test-unit
make test-api
make test-frontend
make coverage
```

## Project Structure

```
cmd/
  server/            HTTP server entry point
  seed/              Development seed data command
internal/
  audit/             Append-only audit log service
  cron/              Scheduled jobs (retry, analytics, ranking, KPI)
  handler/           HTTP handlers by domain
  middleware/        Auth, ban check, RBAC, rate limit
  model/             Domain structs and enums
  repository/        DB interfaces + postgres implementations (pgx)
  service/           Business logic
  sse/               Server-Sent Events hub
  templ/             Templ components and page templates
migrations/          golang-migrate SQL up/down files
static/              CSS, JS, fonts
tests/
  frontend/          Templ render tests (no DB required)
  integration/       End-to-end HTTP tests (requires Docker DB)
```
