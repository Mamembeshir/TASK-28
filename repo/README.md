# EduExchange

Offline Learning Resource Exchange & Supplier Fulfillment Console for school districts.

## Prerequisites

- Docker
- Docker Compose

No other tools required. Everything builds and runs inside Docker.

## Quick Start

```bash
docker-compose up
```

Open [http://localhost:8080](http://localhost:8080).

## Demo Accounts

Seed the database with demo data first:

```bash
DATABASE_URL=postgres://eduexchange:eduexchange@localhost:5432/eduexchange go run ./cmd/seed
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
docs/
  api-spec.md        Full API endpoint reference
  design.md          Architecture, state machines, design decisions
  questions.md       Open questions and future considerations
```

See `docs/` for detailed documentation.
