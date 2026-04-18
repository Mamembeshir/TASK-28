# EduExchange

**Type: fullstack** — Go backend + server-rendered web UI (Templ components).

Offline Learning Resource Exchange & Supplier Fulfillment Console for school districts.

## Prerequisites

- Docker
- Docker Compose

No other tools are required to run or test the application. All builds, migrations, and tests execute inside Docker containers. Do **not** install Go, Postgres, or any runtime dependency on the host machine — the `docker-compose` configuration provides everything needed.

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

## Verifying the Application is Running

After `docker-compose up`, confirm the application is working with these checks:

```bash
# 1. Health endpoint — should return {"status":"ok"}
curl http://localhost:8080/health

# 2. Login page — should return HTTP 200
curl -o /dev/null -s -w "%{http_code}" http://localhost:8080/login

# 3. Authenticated access — log in and check home page
curl -c /tmp/cookies.txt -b /tmp/cookies.txt \
     -X POST http://localhost:8080/login \
     -d "username=admin&password=Admin12345!@" -L
# Expect: redirect to / with 200 home page
```

For UI verification, open [http://localhost:8080/login](http://localhost:8080/login) in a browser, log in with the `admin` credentials below, and confirm the dashboard loads.

## Demo Accounts

The server seeds a minimal set of accounts on first boot. For a richer dataset (published resources, engagement data, supplier orders) run the full seed inside Docker:

```bash
# Preferred: runs inside the already-running container (no local Go required)
make seed

# Alternative: reset the DB, apply migrations, then seed in one step
make fresh
```

| Username | Password | Roles |
|---|---|---|
| admin | Admin12345!@ | Administrator |
| author1 | Author12345!@ | Author, Regular User |
| reviewer1 | Review12345!@ | Reviewer, Regular User |
| supplier1 | Supply12345!@ | Supplier |
| teacher1 | Teach12345!@ | Regular User |

## Running Tests

All tests run inside Docker — no local Go installation required:

```bash
# All tests (unit + frontend render + integration)
./run_tests.sh

# Or via Makefile
make test

# Specific suites
make test-unit        # Service/model unit tests (no DB)
make test-api         # Integration tests (full HTTP, real DB)
make test-frontend    # Templ component render tests (no DB)
make coverage         # Coverage report
```

The integration suite (`make test-api`) spins up a real Postgres instance, runs migrations, and exercises all HTTP endpoints with a production router — no mocks.

## Architecture

```
cmd/
  server/            HTTP server entry point
  seed/              Development seed data command
internal/
  audit/             Append-only audit log service
  cron/              Scheduled jobs (retry, analytics, ranking, KPI)
  handler/           HTTP handlers by domain (auth, catalog, engagement,
                     gamification, search, supplier, messaging, moderation,
                     analytics, admin)
  middleware/        Auth, ban check, RBAC, rate limit, idempotency, CSRF
  model/             Domain structs and enums
  repository/        DB interfaces + postgres implementations (pgx)
  service/           Business logic
  sse/               Server-Sent Events hub
  templ/             Templ components and page templates
migrations/          golang-migrate SQL up/down files
static/              CSS, JS, fonts
tests/
  unit/              Service/model unit tests (no DB required)
  frontend/          Templ render tests (no DB required)
  integration/       Full HTTP integration tests (requires Docker DB)
```

### Request lifecycle

```
HTTP request
  → CSRF middleware
  → Auth middleware (session validation)
  → Ban check middleware
  → RBAC middleware (role enforcement)
  → Idempotency middleware
  → Handler (calls Service → Repository → Postgres)
  → Templ page render or JSON response
```

### Authorization model

| Role | Capabilities |
|---|---|
| `REGULAR_USER` | Browse published resources, vote, favorite, follow, report |
| `AUTHOR` | All of the above + create/edit/delete own drafts, upload files, create tags |
| `REVIEWER` | All of the above + review queue, approve/reject submissions, moderate reports |
| `SUPPLIER` | Supplier portal, view/confirm orders, submit ASN |
| `ADMIN` | Full access: publish, take down, restore, manage users, categories, tags, import/export, analytics |

## Troubleshooting

| Symptom | Fix |
|---|---|
| `docker-compose up` fails with port conflict | Stop conflicting service or change port in `docker-compose.yml` |
| DB connection refused on startup | Wait 5–10 s for Postgres to initialize; migrations run automatically |
| `SESSION_SECRET` or `ENCRYPTION_KEY` missing | Check `.env` file; both must be present and `ENCRYPTION_KEY` exactly 32 bytes |
| Tests fail with "failed to connect to test DB" | Ensure `docker-compose up` has fully started before running `./run_tests.sh` |
| Seed data not visible | Run `make seed` after `docker-compose up`; the minimal seed only creates users |
